package config

import (
	"os"
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
}

// Load returns configuration populated from environment variables with fallback defaults
func Load() *Config {
	return &Config{
		HTTPPort:        getEnv("HTTP_PORT", "8080"),
		TCPPort:         getEnv("TCP_PORT", "9090"),
		UDPPort:         getEnv("UDP_PORT", "9091"),
		GRPCPort:        getEnv("GRPC_PORT", "9092"),
		CatalogGRPCAddr: getEnv("CATALOG_GRPC_ADDR", "localhost:9092"),
		BackendDBPath:   getEnv("BACKEND_DB_PATH", "./data/mangahub.db"),
		CatalogDBPath:   getEnv("CATALOG_DB_PATH", "./data/catalog.db"),
		JWTSecret:       getEnv("JWT_SECRET", "mangahub_secret_jwt_key_2026"),
	}
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
