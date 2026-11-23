package main

import (
	"testing"

	"mangahub/internal/auth"
	"mangahub/internal/database"
)

func TestClosedDatabaseReturnsControlledErrors(t *testing.T) {
	router, _ := setupBackendTestEnv(t)
	token, err := auth.GenerateToken("db-review-user", "review")
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct{ method, path, body string }{
		{"POST", "/api/auth/register", `{"username":"review","email":"review@example.test","password":"Review-password-123"}`},
		{"POST", "/api/auth/login", `{"username":"review","password":"Review-password-123"}`},
		{"GET", "/api/library", ""},
	} {
		response := readinessRequest(router, test.method, test.path, token, test.body)
		if response.Code != 500 {
			t.Errorf("%s: closed SQLite returned %d", test.path, response.Code)
		}
	}
}
