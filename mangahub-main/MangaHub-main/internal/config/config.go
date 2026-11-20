package config

import (
	"errors"
	"os"

	"github.com/joho/godotenv"
)

// Config holds all service configuration values
type Config struct {
	HTTPPort        string
	TCPPort         string
	UDPPort         string
	GRPCPort        string
	CatalogGRPCAddr string
	BackendDBPath   string
	CatalogDBPath   string
	JWTSecret       string
	AdminAPIKey     string
}

// Load reads .env.local from the working directory, preserving existing process
// variables, then returns configuration with defaults for unset/empty values.
func Load() *Config {
	if err := godotenv.Load(".env.local"); err != nil && !errors.Is(err, os.ErrNotExist) {
		// Parser errors may include file contents. Do not expose secrets in logs.
		panic("cannot load .env.local: check its format and read permissions")
	}
	return &Config{
		HTTPPort:        getEnv("HTTP_PORT", "8080"),
		TCPPort:         getEnv("TCP_PORT", "9090"),
		UDPPort:         getEnv("UDP_PORT", "9091"),
		GRPCPort:        getEnv("GRPC_PORT", "9092"),
		CatalogGRPCAddr: getEnv("CATALOG_GRPC_ADDR", "localhost:9092"),
		BackendDBPath:   getEnv("BACKEND_DB_PATH", "./data/mangahub.db"),
		CatalogDBPath:   getEnv("CATALOG_DB_PATH", "./data/catalog.db"),
		JWTSecret:       getEnv("JWT_SECRET", "mangahub_secret_jwt_key_2026"),
		AdminAPIKey:     os.Getenv("ADMIN_API_KEY"),
	}
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
