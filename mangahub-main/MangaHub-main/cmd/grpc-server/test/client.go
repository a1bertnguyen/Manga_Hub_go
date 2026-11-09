package main

import (
	"context"
	"fmt"
	"log"
	"time"

	pb "mangahub/proto"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	// Connect to gRPC server
	conn, err := grpc.Dial("localhost:9092",
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	
	defer conn.Close()

	client := pb.NewMangaServiceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	fmt.Println("🧪 Testing gRPC Manga Service")

	// Test 1: GetManga
	fmt.Println("=== Test 1: Get Manga ===")
	getMangaResp, err := client.GetManga(ctx, &pb.GetMangaRequest{
		Id: "one-piece",
	})
	if err != nil {
		log.Printf("GetManga failed: %v", err)
	} else {
		manga := getMangaResp.Manga
		fmt.Printf("  Title: %s\n", manga.Title)
		fmt.Printf("  Author: %s\n", manga.Author)
		fmt.Printf("  Genres: %v\n", manga.Genres)
		fmt.Printf("  Chapters: %d\n\n", manga.TotalChapters)
	}

	// Test 2: SearchManga
	fmt.Println("=== Test 2: Search Manga ===")
	searchResp, err := client.SearchManga(ctx, &pb.SearchMangaRequest{
		Query: "naruto",
	})
	if err != nil {
		log.Printf("SearchManga failed: %v", err)
	} else {
		fmt.Printf(" Found %d manga(s)\n", searchResp.Count)
		for i, manga := range searchResp.Mangas {
			fmt.Printf("  %d. %s by %s\n", i+1, manga.Title, manga.Author)
		}
		fmt.Println()
	}

	// Test 3: Search by Genre
	fmt.Println("=== Test 3: Search by Genre ===")
	genreResp, err := client.SearchManga(ctx, &pb.SearchMangaRequest{
		Genre: "Action",
	})
	if err != nil {
		log.Printf("SearchManga (genre) failed: %v", err)
	} else {
		fmt.Printf(" Found %d Action manga(s)\n", genreResp.Count)
		for i, manga := range genreResp.Mangas {
			fmt.Printf("  %d. %s\n", i+1, manga.Title)
		}
		fmt.Println()
	}

	// Test 4: UpdateProgress
	fmt.Println("=== Test 4: Update Progress ===")
	progressResp, err := client.UpdateProgress(ctx, &pb.UpdateProgressRequest{
		UserId:         "usr_test123",
		MangaId:        "one-piece",
		CurrentChapter: 1095,
	})
	if err != nil {
		log.Printf("UpdateProgress failed: %v", err)
	} else {
		fmt.Printf(" %s\n\n", progressResp.Message)
	}

	// Test 5: Error handling (manga not found)
	fmt.Println("=== Test 5: Error Handling ===")
	_, err = client.GetManga(ctx, &pb.GetMangaRequest{
		Id: "nonexistent",
	})
	if err != nil {
		fmt.Printf(" Expected error: %v\n\n", err)
	}

	fmt.Println("✅ All tests completed!")
}
