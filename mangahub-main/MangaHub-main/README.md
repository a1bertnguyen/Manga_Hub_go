# MangaHub - Manga Tracking System

## Features
- User authentication: register/login with JWT tokens
- Manga catalog with search by title, author or genre
- Personal library management
- Reading progress tracking with real-time sync
- Real-time global chat (websocket) with typing indicator
- Progress broadcasting via TCP (reliable) and UDP (fast)
- gRPC service ready for microservices
- Responsive web interface

## Project Structure
MangaHub-main/
├── cmd/
│   ├── api-server/          # REST API + Web server (:8080)
│   ├── tcp-server/          # TCP progress sync: port 9090 + internal: port 9091 (clients: telnet localhost 9090)
│   ├── udp-server/          # UDP notifications: port 9091 (client: go run cmd/udp-client/main.go)
│   ├── websocket-server/    # Real-time chat (:9093)
│   └── grpc-server/         # gRPC service server (:9092)
├── internal/                # Private application code
│   ├── auth/                # Authentication logic
│   ├── shared/              # Update message
│   ├── database/            # Database initialization
│   ├── tcp/                 # TCP hub and client
│   ├── udp/                 # UDP hub and client
│   ├── websocket/           # WebSocket hub and client
│   └── grpc/                # gRPC service implementation
├── pkg/models/              # Data models
├── web/                     # Frontend HTML
├── data/mangahub.db         # SQLite database 
├── docs/                    # Swagger generated files
├── proto/                   # Protocol Buffer definitions
├── go.mod   
├── go.sum                 
└── README.md                


## Prerequisites
- Go 1.21 or higher
- Git

## Setup Instructions
1. **Clone the repository**
   ```bash
   git clone https://your-repo-url/MangaHub-main.git
   cd MangaHub-main
2. **Install dependencies**
   go mod tidy

## Run the System
go run cmd/api-server/main.go &
go run cmd/tcp-server/main.go &
go run cmd/udp-server/main.go &
go run cmd/websocket-server/main.go &
go run cmd/grpc-server/main.go &   

## API Documentation
Interactive Swagger docs: http://localhost:8080/swagger/index.html

- Auth: Use Bearer token from login
- Try endpoints directly in browser

## Code Documentation
Run `godoc -http=:6060` for full GoDoc.
http://localhost:6060/pkg/mangahub/internal/database/ → show Initialize and SeedManga comments
http://localhost:6060/pkg/mangahub/pkg/models/ → show Manga struct and methods
http://localhost:6060/pkg/mangahub/internal/auth/ → show HashPassword, etc.
All key functions have comments explaining purpose.



