package config

import (
	"os"
	"testing"
)

// Environment changes are process-wide, so these tests must remain sequential.
func localConfigDir(t *testing.T) {
	t.Helper()
	t.Chdir(t.TempDir())
	for _, key := range []string{
		"HTTP_PORT", "TCP_PORT", "UDP_PORT", "GRPC_PORT", "CATALOG_GRPC_ADDR",
		"BACKEND_DB_PATH", "CATALOG_DB_PATH", "JWT_SECRET", "ADMIN_API_KEY",
		"API_URL", "TCP_ADDR", "UDP_ADDR", "WS_URL",
	} {
		previous, present := os.LookupEnv(key)
		t.Cleanup(func() {
			if present {
				_ = os.Setenv(key, previous)
			} else {
				_ = os.Unsetenv(key)
			}
		})
		if err := os.Unsetenv(key); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLoadLocalEnv(t *testing.T) {
	localConfigDir(t)
	contents := "# Local settings\r\nHTTP_PORT=18080\r\nTCP_PORT=19090\r\nUDP_PORT=19091\r\nGRPC_PORT=19092\r\n" +
		"CATALOG_GRPC_ADDR=127.0.0.1:19092\r\nBACKEND_DB_PATH=./data/local.db\r\nCATALOG_DB_PATH=./data/catalog-local.db\r\n" +
		"JWT_SECRET='local-test#with=characters'\r\nexport ADMIN_API_KEY=local-admin\r\n" +
		"API_URL=http://127.0.0.1:18080\r\nTCP_ADDR=127.0.0.1:19090\r\nUDP_ADDR=127.0.0.1:19091\r\nWS_URL=ws://127.0.0.1:18080/ws\r\n"
	if err := os.WriteFile(".env.local", []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := Load()
	want := Config{
		HTTPPort: "18080", TCPPort: "19090", UDPPort: "19091", GRPCPort: "19092",
		CatalogGRPCAddr: "127.0.0.1:19092", BackendDBPath: "./data/local.db", CatalogDBPath: "./data/catalog-local.db",
		JWTSecret: "local-test#with=characters", AdminAPIKey: "local-admin",
	}
	if *cfg != want {
		t.Fatal("local configuration was not loaded correctly")
	}
	for key, value := range map[string]string{
		"API_URL": "http://127.0.0.1:18080", "TCP_ADDR": "127.0.0.1:19090",
		"UDP_ADDR": "127.0.0.1:19091", "WS_URL": "ws://127.0.0.1:18080/ws",
	} {
		if os.Getenv(key) != value {
			t.Errorf("TUI override %s was not loaded", key)
		}
	}
}

func TestProcessEnvironmentTakesPriority(t *testing.T) {
	localConfigDir(t)
	t.Setenv("HTTP_PORT", "28080")
	t.Setenv("JWT_SECRET", "process-test-secret")
	t.Setenv("ADMIN_API_KEY", "") // Explicitly disabled, even if the file has a key.
	if err := os.WriteFile(".env.local", []byte("HTTP_PORT=18080\nJWT_SECRET=file-secret\nADMIN_API_KEY=file-admin\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := Load()
	if cfg.HTTPPort != "28080" || cfg.JWTSecret != "process-test-secret" || cfg.AdminAPIKey != "" {
		t.Fatal("local file overrode an existing process variable")
	}
}

func TestLoadWithoutLocalEnv(t *testing.T) {
	localConfigDir(t)
	cfg := Load()
	if cfg.HTTPPort != "8080" || cfg.CatalogGRPCAddr != "localhost:9092" || cfg.AdminAPIKey != "" {
		t.Fatal("missing optional file changed defaults")
	}
	if _, exists := os.Stat(".env.local"); !os.IsNotExist(exists) {
		t.Fatal("loading config created a local file")
	}
}

func TestMalformedLocalEnvFailsWithoutLeakingContents(t *testing.T) {
	localConfigDir(t)
	if err := os.WriteFile(".env.local", []byte("JWT_SECRET='malformed-private-value\n"), 0600); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if value := recover(); value != "cannot load .env.local: check its format and read permissions" {
			t.Fatal("malformed configuration must stop startup with a secret-free error")
		}
	}()
	Load()
}
