package main

import (
	"log"
	"os"

	"github.com/GoogleCloudPlatform/functions-framework-go/funcframework"
	"github.com/joho/godotenv"

	// Blank import to trigger the init() in function.go which registers the handler
	_ "github.com/vinm0/gmail-handler"
)

func main() {
	// Load .env file if it exists to simplify local development
	_ = godotenv.Load()

	// Use PORT environment variable, or default to 8080
	port := "8080"
	if envPort := os.Getenv("PORT"); envPort != "" {
		port = envPort
	}
	if err := funcframework.Start(port); err != nil {
		log.Fatalf("funcframework.Start: %v\n", err)
	}
}
