package tui

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
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
	if c.state.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.state.Token)
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
			return errors.New(errResp.Error)
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

	c.state.Token = res.Token
	c.state.UserID = res.UserID
	c.state.Username = res.Username
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

	c.state.Token = res.Token
	c.state.UserID = res.UserID
	c.state.Username = res.Username
	return nil
}

func (c *APIClient) FetchManga(query, genre string) ([]*models.Manga, error) {
	url := "/api/manga"
	if query != "" {
		url += "?q=" + query
	}
	if genre != "" {
		if query != "" {
			url += "&genre=" + genre
		} else {
			url += "?genre=" + genre
		}
	}

	var res struct {
		Manga []*models.Manga `json:"manga"`
		Count int             `json:"count"`
	}
	err := c.doRequest("GET", url, nil, &res)
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

