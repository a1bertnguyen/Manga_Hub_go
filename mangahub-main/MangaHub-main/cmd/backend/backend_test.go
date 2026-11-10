package main

import (
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"mangahub/internal/config"
	"mangahub/internal/database"
	grpcServer "mangahub/internal/grpc"
	"mangahub/pkg/models"
	pb "mangahub/proto"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func setupBackendTestEnv(t *testing.T) (*gin.Engine, *config.Config) {
	gin.SetMode(gin.TestMode)

	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "backend_test.db")

	if err := database.Initialize(dbPath); err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}

	t.Cleanup(func() {
		_ = database.Close()
	})

	cfg := &config.Config{
		HTTPPort:      "8080",
		TCPPort:       "9090",
		UDPPort:       "9091",
		BackendDBPath: dbPath,
	}

	router := setupRouter(cfg)
	return router, cfg
}

func TestBackend_AuthFlow(t *testing.T) {
	router, _ := setupBackendTestEnv(t)

	// 1. Register new user -> 200 OK
	regBody, _ := json.Marshal(models.RegisterRequest{
		Username: "bob_marley",
		Email:    "bob@music.com",
		Password: "password123",
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/auth/register", bytes.NewBuffer(regBody))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK on register, got %d: %s", w.Code, w.Body.String())
	}

	var regResp models.LoginResponse
	if err := json.Unmarshal(w.Body.Bytes(), &regResp); err != nil {
		t.Fatalf("Failed to decode register response: %v", err)
	}
	if regResp.Token == "" || regResp.Username != "bob_marley" {
		t.Fatalf("Invalid register response payload: %+v", regResp)
	}

	// 2. Register duplicate username -> 409 Conflict
	wDup := httptest.NewRecorder()
	reqDup, _ := http.NewRequest("POST", "/api/auth/register", bytes.NewBuffer(regBody))
	reqDup.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(wDup, reqDup)

	if wDup.Code != http.StatusConflict {
		t.Errorf("Expected 409 Conflict for duplicate username, got %d", wDup.Code)
	}

	// 3. Login with correct password -> 200 OK
	loginBody, _ := json.Marshal(models.LoginRequest{
		Username: "bob_marley",
		Password: "password123",
	})
	wLogin := httptest.NewRecorder()
	reqLogin, _ := http.NewRequest("POST", "/api/auth/login", bytes.NewBuffer(loginBody))
	reqLogin.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(wLogin, reqLogin)

	if wLogin.Code != http.StatusOK {
		t.Errorf("Expected 200 OK on valid login, got %d", wLogin.Code)
	}

	// 4. Login with invalid password -> 401 Unauthorized
	badLoginBody, _ := json.Marshal(models.LoginRequest{
		Username: "bob_marley",
		Password: "wrongPassword!",
	})
	wBad := httptest.NewRecorder()
	reqBad, _ := http.NewRequest("POST", "/api/auth/login", bytes.NewBuffer(badLoginBody))
	reqBad.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(wBad, reqBad)

	if wBad.Code != http.StatusUnauthorized {
		t.Errorf("Expected 401 Unauthorized for invalid password, got %d", wBad.Code)
	}
}

func startTestGRPCServer(t *testing.T) (net.Listener, *grpc.Server) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to listen for test gRPC: %v", err)
	}

	srv := grpc.NewServer()
	mangaSrv := grpcServer.NewMangaServiceServer(database.DB)
	pb.RegisterMangaServiceServer(srv, mangaSrv)

	go func() {
		_ = srv.Serve(lis)
	}()

	return lis, srv
}

func TestBackend_GRPC_CatalogAndServiceUnavailable(t *testing.T) {
	router, _ := setupBackendTestEnv(t)

	// Seed one test manga in database
	_, _ = database.DB.Exec(`
		INSERT INTO manga (id, title, author, genres, status, total_chapters, description)
		VALUES ('solo-leveling', 'Solo Leveling', 'Chugong', 'Action,Fantasy', 'completed', 200, 'Hunter Sung')
	`)

	// 1. Start gRPC catalog server
	lis, srv := startTestGRPCServer(t)

	// Dial gRPC from backend client
	conn, err := grpc.Dial(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("Failed to dial gRPC: %v", err)
	}
	grpcCatalogClient = pb.NewMangaServiceClient(conn)
	catalogConn = conn

	t.Cleanup(func() {
		_ = conn.Close()
		srv.Stop()
	})

	// Register user to obtain valid JWT token
	regBody, _ := json.Marshal(models.RegisterRequest{
		Username: "hunter",
		Email:    "hunter@test.com",
		Password: "hunterPassword123",
	})
	wReg := httptest.NewRecorder()
	rReg, _ := http.NewRequest("POST", "/api/auth/register", bytes.NewBuffer(regBody))
	router.ServeHTTP(wReg, rReg)

	var regResp models.LoginResponse
	_ = json.Unmarshal(wReg.Body.Bytes(), &regResp)
	token := regResp.Token

	// 2. Query catalog when gRPC is running -> 200 OK
	wQuery := httptest.NewRecorder()
	rQuery, _ := http.NewRequest("GET", "/api/manga?q=Solo", nil)
	rQuery.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(wQuery, rQuery)

	if wQuery.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK from catalog search, got %d: %s", wQuery.Code, wQuery.Body.String())
	}

	// 3. Gracefully stop gRPC catalog server to simulate network / process failure
	srv.Stop()
	_ = lis.Close()
	_ = conn.Close()

	// 4. Query catalog when gRPC is DOWN -> Must return HTTP 503 Service Unavailable
	wDown := httptest.NewRecorder()
	rDown, _ := http.NewRequest("GET", "/api/manga?q=Solo", nil)
	rDown.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(wDown, rDown)

	if wDown.Code != http.StatusServiceUnavailable {
		t.Errorf("Grade A Verification Failed: Expected 503 Service Unavailable when gRPC is down, got %d (body: %s)",
			wDown.Code, wDown.Body.String())
	}
}

func TestBackend_ChapterValidationAndProgress(t *testing.T) {
	router, _ := setupBackendTestEnv(t)

	// Seed manga with total_chapters = 25
	_, _ = database.DB.Exec(`
		INSERT INTO manga (id, title, author, genres, status, total_chapters, description)
		VALUES ('chainsaw-man', 'Chainsaw Man', 'Tatsuki Fujimoto', 'Action,Supernatural', 'ongoing', 25, 'Denji')
	`)

	// Start test gRPC server
	lis, srv := startTestGRPCServer(t)
	conn, err := grpc.Dial(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("Failed to dial gRPC: %v", err)
	}
	grpcCatalogClient = pb.NewMangaServiceClient(conn)
	catalogConn = conn

	t.Cleanup(func() {
		_ = conn.Close()
		srv.Stop()
	})

	// Create user
	regBody, _ := json.Marshal(models.RegisterRequest{
		Username: "denji_fan",
		Email:    "denji@pochita.com",
		Password: "denjiPassword123",
	})
	wReg := httptest.NewRecorder()
	rReg, _ := http.NewRequest("POST", "/api/auth/register", bytes.NewBuffer(regBody))
	router.ServeHTTP(wReg, rReg)
	var regResp models.LoginResponse
	_ = json.Unmarshal(wReg.Body.Bytes(), &regResp)
	token := regResp.Token

	// Add Chainsaw Man to library
	addBody, _ := json.Marshal(AddToLibraryReq{
		MangaID: "chainsaw-man",
		Status:  "reading",
	})
	wAdd := httptest.NewRecorder()
	rAdd, _ := http.NewRequest("POST", "/api/library", bytes.NewBuffer(addBody))
	rAdd.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(wAdd, rAdd)
	if wAdd.Code != http.StatusOK {
		t.Fatalf("Failed to add to library: %d", wAdd.Code)
	}

	// 1. Grade A Requirement: Chapter > Total Chapters MUST be rejected with HTTP 400 Bad Request
	progBadBody, _ := json.Marshal(UpdateProgressReq{
		CurrentChapter: 30, // Exceeds 25!
		Status:         "reading",
	})
	wBadProg := httptest.NewRecorder()
	rBadProg, _ := http.NewRequest("PUT", "/api/library/chainsaw-man/progress", bytes.NewBuffer(progBadBody))
	rBadProg.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(wBadProg, rBadProg)

	if wBadProg.Code != http.StatusBadRequest {
		t.Errorf("Grade A Verification Failed: Expected 400 Bad Request when chapter exceeds total, got %d: %s",
			wBadProg.Code, wBadProg.Body.String())
	}

	// 2. Chapter <= Total Chapters MUST succeed with HTTP 200 OK
	progGoodBody, _ := json.Marshal(UpdateProgressReq{
		CurrentChapter: 20, // Valid (<= 25)
		Status:         "reading",
	})
	wGoodProg := httptest.NewRecorder()
	rGoodProg, _ := http.NewRequest("PUT", "/api/library/chainsaw-man/progress", bytes.NewBuffer(progGoodBody))
	rGoodProg.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(wGoodProg, rGoodProg)

	if wGoodProg.Code != http.StatusOK {
		t.Errorf("Expected 200 OK for valid chapter update, got %d: %s", wGoodProg.Code, wGoodProg.Body.String())
	}

	// 3. Verify library query returns chapter 20 and version incremented to 2
	wLib := httptest.NewRecorder()
	rLib, _ := http.NewRequest("GET", "/api/library", nil)
	rLib.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(wLib, rLib)

	var libResp struct {
		Library []struct {
			MangaID        string `json:"manga_id"`
			CurrentChapter int    `json:"current_chapter"`
			Version        int    `json:"version"`
		} `json:"library"`
	}
	_ = json.Unmarshal(wLib.Body.Bytes(), &libResp)
	if len(libResp.Library) != 1 || libResp.Library[0].CurrentChapter != 20 || libResp.Library[0].Version != 2 {
		t.Errorf("Expected chapter 20 and version 2, got: %+v", libResp.Library)
	}
}
