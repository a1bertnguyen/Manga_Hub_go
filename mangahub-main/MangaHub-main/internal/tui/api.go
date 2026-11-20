package tui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"mangahub/pkg/models"
)

type APIClient struct {
	state *AppState
	http  *http.Client
}

func NewAPIClient(state *AppState) *APIClient {
	return &APIClient{
		state: state,
		http: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

func (c *APIClient) doRequest(method, endpoint string, body interface{}, dest interface{}) error {
	var bodyReader io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return err
		}
		bodyReader = bytes.NewBuffer(jsonData)
	}

	req, err := http.NewRequest(method, c.state.BaseURL+endpoint, bodyReader)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	if method == http.MethodPut && strings.HasPrefix(endpoint, "/api/library/") && strings.HasSuffix(endpoint, "/progress") {
		if sessionID := c.state.tcpSessionID(); sessionID != "" {
			req.Header.Set("X-Session-ID", sessionID)
		}
	}
	if strings.HasPrefix(endpoint, "/api/admin/") {
		req.Header.Set("X-Admin-Key", os.Getenv("ADMIN_API_KEY"))
	}
	if c.state.token() != "" {
		req.Header.Set("Authorization", "Bearer "+c.state.token())
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("connection error: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode >= 400 {
		var errResp struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(respBytes, &errResp); err == nil && errResp.Error != "" {
			return fmt.Errorf("HTTP %d: %s", resp.StatusCode, errResp.Error)
		}
		return fmt.Errorf("HTTP error %d: %s", resp.StatusCode, string(respBytes))
	}

	if dest != nil {
		return json.Unmarshal(respBytes, dest)
	}
	return nil
}

func (c *APIClient) Login(username, password string) error {
	var res models.LoginResponse
	err := c.doRequest("POST", "/api/auth/login", map[string]string{
		"username": username,
		"password": password,
	}, &res)
	if err != nil {
		return err
	}

	c.state.setSession(res.Token, res.UserID, res.Username)
	_ = SaveSession(res.Token, res.UserID, res.Username)
	return nil
}

func (c *APIClient) Register(username, email, password string) error {
	var res models.LoginResponse
	err := c.doRequest("POST", "/api/auth/register", map[string]string{
		"username": username,
		"email":    email,
		"password": password,
	}, &res)
	if err != nil {
		return err
	}

	c.state.setSession(res.Token, res.UserID, res.Username)
	_ = SaveSession(res.Token, res.UserID, res.Username)
	return nil
}

func (c *APIClient) FetchManga(query, genre string) ([]*models.Manga, error) {
	params := url.Values{}
	params.Set("q", query)
	if genre != "" {
		params.Set("genre", genre)
	}
	endpoint := "/api/manga?" + params.Encode()

	var res struct {
		Manga []*models.Manga `json:"manga"`
		Count int             `json:"count"`
	}
	err := c.doRequest("GET", endpoint, nil, &res)
	if err != nil {
		return nil, err
	}
	return res.Manga, nil
}

func (c *APIClient) GetMangaDetail(id string) (*models.Manga, error) {
	var m models.Manga
	err := c.doRequest("GET", "/api/manga/"+id, nil, &m)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (c *APIClient) FetchLibrary() ([]LibraryItem, error) {
	var res struct {
		Library []LibraryItem `json:"library"`
		Count   int           `json:"count"`
	}
	err := c.doRequest("GET", "/api/library", nil, &res)
	if err != nil {
		return nil, err
	}
	return res.Library, nil
}

func (c *APIClient) AddToLibrary(mangaID, status string) error {
	return c.doRequest("POST", "/api/library", map[string]string{
		"manga_id": mangaID,
		"status":   status,
	}, nil)
}

func (c *APIClient) UpdateProgress(mangaID string, chapter int, status string) error {
	return c.doRequest("PUT", "/api/library/"+mangaID+"/progress", map[string]interface{}{
		"current_chapter": chapter,
		"status":          status,
	}, nil)
}

// TriggerRelease sends an admin request to release a new chapter (updating gRPC and triggering UDP broadcast)
func (c *APIClient) TriggerRelease(mangaID string, newChapter int, title string) error {
	return c.doRequest("POST", "/api/admin/manga/"+mangaID+"/release", map[string]interface{}{
		"chapter": newChapter,
		"title":   title,
	}, nil)
}

// FetchHealth retrieves the live status of all integrated backend services
func (c *APIClient) FetchHealth() (map[string]interface{}, error) {
	var res map[string]interface{}
	err := c.doRequest("GET", "/api/health", nil, &res)
	if err != nil {
		return nil, err
	}
	return res, nil
}
