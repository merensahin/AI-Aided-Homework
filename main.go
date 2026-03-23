// Package main is the entry point for the web crawler and search engine.
// It wires together all components and starts the HTTP server.
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"hw1/crawler"
	"hw1/index"
	"hw1/server"
)

// DefaultPort is the port the HTTP server listens on.
const DefaultPort = ":8080"

// StorageDir is the directory for persisted index and job state files.
const StorageDir = "storage"

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// Initialize the inverted index.
	idx := index.New()

	// Try to load persisted index data from disk.
	if err := idx.LoadFromDisk(StorageDir); err != nil {
		log.Printf("No existing index data loaded: %v", err)
	} else {
		log.Printf("Loaded index from disk (%d words)", idx.Size())
	}

	// Initialize the crawler.
	c := crawler.New(idx)

	// Initialize the HTTP server.
	srv := server.New(c, idx)
	httpServer := &http.Server{
		Addr:         DefaultPort,
		Handler:      srv.Handler(),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Start the server in a goroutine.
	go func() {
		log.Printf("Server starting on http://localhost%s", DefaultPort)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	// Wait for interrupt signal for graceful shutdown.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	// Graceful shutdown with timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}

	// Persist the inverted index to disk.
	log.Println("Saving index to disk...")
	if err := idx.SaveToDisk(StorageDir); err != nil {
		log.Printf("Error saving index: %v", err)
	} else {
		log.Println("Index saved successfully.")
	}

	log.Println("Server stopped.")
}
