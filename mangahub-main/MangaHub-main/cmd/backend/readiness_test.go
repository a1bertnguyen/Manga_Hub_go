package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"mangahub/internal/auth"
	"mangahub/internal/database"
	wsHub "mangahub/internal/websocket"
	pb "mangahub/proto"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type catalogResultClient struct {
	pb.MangaServiceClient
	code  codes.Code
	total int32
	empty bool
}

func (f catalogResultClient) GetManga(ctx context.Context, _ *pb.GetMangaRequest, _ ...grpc.CallOption) (*pb.GetMangaResponse, error) {
	if f.code != codes.OK {
		return nil, status.Error(f.code, "injected catalog failure")
	}
	if f.empty {
		return &pb.GetMangaResponse{}, nil
	}
	return &pb.GetMangaResponse{Manga: &pb.Manga{Id: "review-manga", Title: "Review manga", TotalChapters: f.total}}, nil
}

func readinessBackend(t *testing.T) (*gin.Engine, string) {
	t.Helper()
	router, _ := setupBackendTestEnv(t)
	previous := grpcCatalogClient
	grpcCatalogClient = catalogResultClient{total: 25}
	t.Cleanup(func() { grpcCatalogClient = previous })
	_, err := database.DB.Exec(`INSERT INTO users(id,username,email,password_hash) VALUES ('review-user','alice','alice@example.test','unused');
 INSERT INTO user_progress(user_id,manga_id,current_chapter,status,version) VALUES ('review-user','review-manga',5,'reading',1)`)
	if err != nil {
		t.Fatal(err)
	}
	token, err := auth.GenerateToken("review-user", "alice")
	if err != nil {
		t.Fatal(err)
	}
	return router, token
}

func readinessRequest(router http.Handler, method, path, token, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	router.ServeHTTP(w, r)
	return w
}

func TestReadinessCatalogFailureDoesNotMutateLibrary(t *testing.T) {
	router, token := readinessBackend(t)
	for _, test := range []struct {
		name   string
		client pb.MangaServiceClient
		want   int
	}{
		{"nil", nil, 503}, {"unavailable", catalogResultClient{code: codes.Unavailable}, 503},
		{"deadline", catalogResultClient{code: codes.DeadlineExceeded}, 503},
		{"not-found", catalogResultClient{code: codes.NotFound}, 404},
		{"invalid", catalogResultClient{code: codes.InvalidArgument}, 400},
		{"internal", catalogResultClient{code: codes.Internal}, 500},
		{"empty", catalogResultClient{empty: true}, 503},
	} {
		t.Run(test.name, func(t *testing.T) {
			grpcCatalogClient = test.client
			for _, path := range []string{"/api/library/review-manga/progress", "/users/progress"} {
				w := readinessRequest(router, "PUT", path, token, `{"manga_id":"review-manga","current_chapter":999}`)
				if w.Code != test.want {
					t.Errorf("%s: want %d got %d: %s", path, test.want, w.Code, w.Body)
				}
			}
			for _, path := range []string{"/api/library", "/users/library"} {
				w := readinessRequest(router, "POST", path, token, `{"manga_id":"new-title"}`)
				if w.Code != test.want {
					t.Errorf("%s: want %d got %d", path, test.want, w.Code)
				}
			}
			var chapter, version, count int
			_ = database.DB.QueryRow("SELECT current_chapter,version FROM user_progress WHERE user_id='review-user'").Scan(&chapter, &version)
			_ = database.DB.QueryRow("SELECT COUNT(*) FROM user_progress").Scan(&count)
			if chapter != 5 || version != 1 || count != 1 {
				t.Fatalf("rejected write mutated data: chapter=%d version=%d rows=%d", chapter, version, count)
			}
		})
	}
}

func TestReadinessProgressBoundariesAndIdentity(t *testing.T) {
	router, token := readinessBackend(t)
	for _, body := range []string{`{"current_chapter":-1}`, `{"current_chapter":26}`, `{"current_chapter":4294967296}`, `{"current_chapter":5,"status":"invented"}`} {
		w := readinessRequest(router, "PUT", "/api/library/review-manga/progress", token, body)
		if w.Code != 400 {
			t.Errorf("invalid update returned %d: %s", w.Code, w.Body)
		}
	}
	other, _ := auth.GenerateToken("other-user", "bob")
	w := readinessRequest(router, "PUT", "/api/library/review-manga/progress", other, `{"current_chapter":2,"user_id":"review-user"}`)
	if w.Code != 404 {
		t.Errorf("other user's write returned %d", w.Code)
	}
	grpcCatalogClient = catalogResultClient{total: 0}
	w = readinessRequest(router, "PUT", "/api/library/review-manga/progress", token, `{"current_chapter":1}`)
	if w.Code != 400 {
		t.Errorf("zero-chapter manga accepted chapter 1: %d", w.Code)
	}
	w = readinessRequest(router, "PUT", "/api/library/review-manga/progress", token, `{"current_chapter":0}`)
	if w.Code != 200 {
		t.Errorf("chapter 0 rejected: %d", w.Code)
	}
}

func TestReadinessConcurrentProgressVersions(t *testing.T) {
	router, token := readinessBackend(t)
	const clients = 12
	type result struct{ chapter, version, code int }
	results := make(chan result, clients)
	var wg sync.WaitGroup
	for chapter := 1; chapter <= clients; chapter++ {
		wg.Add(1)
		go func(ch int) {
			defer wg.Done()
			w := readinessRequest(router, "PUT", "/api/library/review-manga/progress", token, fmt.Sprintf(`{"current_chapter":%d}`, ch))
			var response struct {
				Version int `json:"version"`
			}
			_ = json.Unmarshal(w.Body.Bytes(), &response)
			results <- result{ch, response.Version, w.Code}
		}(chapter)
	}
	wg.Wait()
	close(results)
	versions := map[int]int{}
	for result := range results {
		if result.code != 200 {
			t.Fatalf("concurrent update failed: HTTP %d", result.code)
		}
		if _, exists := versions[result.version]; exists {
			t.Errorf("duplicate committed version: %d", result.version)
		}
		versions[result.version] = result.chapter
	}
	for version := 2; version <= clients+1; version++ {
		if _, ok := versions[version]; !ok {
			t.Errorf("missing version %d", version)
		}
	}
	var finalChapter, finalVersion int
	_ = database.DB.QueryRow("SELECT current_chapter,version FROM user_progress WHERE user_id='review-user'").Scan(&finalChapter, &finalVersion)
	if finalVersion != clients+1 || finalChapter != versions[finalVersion] {
		t.Errorf("stored state does not match the last committed version")
	}
}

func TestReadinessTCPRejectsUnverifiedFrames(t *testing.T) {
	for _, frame := range []string{`{"type":"auth","user_id":"victim"}`, `{"type":"auth","token":"invalid","user_id":"victim"}`, `victim`, `not-json`, `{"type":"chat","token":"invalid"}`} {
		t.Run(frame, func(t *testing.T) {
			server, client := net.Pipe()
			defer client.Close()
			done := make(chan struct{})
			go func() { handleTCPConnection(server); close(done) }()
			_ = client.SetDeadline(time.Now().Add(time.Second))
			_, _ = client.Write([]byte(frame + "\n"))
			response, err := bufio.NewReader(client).ReadString('\n')
			if err != nil || !strings.Contains(response, "authentication_failed") {
				t.Errorf("unverified frame accepted: %s (%v)", response, err)
			}
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("rejected connection was not closed")
			}
		})
	}
}

func TestReadinessWebSocketIdentityAndAuthorization(t *testing.T) {
	router, token := readinessBackend(t)
	previous := globalWSHub
	globalWSHub = wsHub.NewHub()
	go globalWSHub.Run()
	t.Cleanup(func() { globalWSHub = previous })
	server := httptest.NewServer(router)
	defer server.Close()
	url := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws?username=forged&room=general"
	for _, value := range []string{"", "Bearer invalid", "Basic " + token, token} {
		headers := http.Header{}
		if value != "" {
			headers.Set("Authorization", value)
		}
		conn, response, err := websocket.DefaultDialer.Dial(url, headers)
		if conn != nil {
			conn.Close()
		}
		if err == nil || response == nil || response.StatusCode != 401 {
			t.Errorf("invalid upgrade was not rejected: %q", value)
		}
		if response != nil {
			response.Body.Close()
		}
	}
	headers := http.Header{"Authorization": []string{"Bearer " + token}}
	conn, _, err := websocket.DefaultDialer.Dial(url, headers)
	if err != nil {
		t.Fatal(err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	if err := conn.WriteJSON(map[string]string{"type": "chat", "text": "verified identity", "username": "forged", "room": "other-room"}); err != nil {
		t.Fatal(err)
	}
	var message wsHub.Message
	if err := conn.ReadJSON(&message); err != nil {
		t.Fatal(err)
	}
	if message.Username != "alice" || message.Room != "general" {
		t.Errorf("client forged server identity/room: %+v", message)
	}
	conn.Close()
	deadline := time.Now().Add(time.Second)
	for globalWSHub.GetClientCount() != 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if globalWSHub.GetClientCount() != 0 {
		t.Fatal("disconnected WebSocket client not removed")
	}
}

func TestReadinessAdminReleaseProtection(t *testing.T) {
	router, cfg := setupBackendTestEnv(t)
	cfg.AdminAPIKey = "test-only-admin-key"
	token, _ := auth.GenerateToken("admin-demo", "teacher")
	for _, path := range []string{"/api/admin/manga/review/release", "/admin/manga/review/release"} {
		for _, chapter := range []int64{0, -1, 4294967296} {
			r := httptest.NewRequest("POST", path, strings.NewReader(fmt.Sprintf(`{"chapter":%d}`, chapter)))
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Authorization", "Bearer "+token)
			r.Header.Set("X-Admin-Key", cfg.AdminAPIKey)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, r)
			if w.Code != http.StatusBadRequest {
				t.Errorf("invalid release chapter %d: got %d", chapter, w.Code)
			}
		}
		for _, test := range []struct {
			token, key string
			want       int
		}{{"", "", 401}, {token, "", 403}, {token, "wrong", 403}, {token, cfg.AdminAPIKey, 503}} {
			previous := grpcCatalogClient
			grpcCatalogClient = nil
			r := httptest.NewRequest("POST", path, bytes.NewBufferString(`{"chapter":2}`))
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Authorization", "Bearer "+test.token)
			r.Header.Set("X-Admin-Key", test.key)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, r)
			grpcCatalogClient = previous
			if w.Code != test.want {
				t.Errorf("%s: want %d got %d", path, test.want, w.Code)
			}
		}
	}
}
