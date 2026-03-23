# Real-Time Web Crawler and Search Engine

A fully functional, single-machine web crawler and real-time search engine built from scratch in Go. 
This project strictly relies on language-native features (e.g., `net/http`, goroutines) without high-level scraping frameworks.

## Features
- **Concurrent Crawling:** Goroutine-based crawler with 3 layers of back-pressure (queue limit, concurrency semaphore, rate limiting).
- **Thread-safe Indexing:** In-memory inverted index safely accessed via `sync.RWMutex` for concurrent reads/writes.
- **Live Search:** Search for keywords while indexing is actively running.
- **Persistence (Bonus):** The inverted index and crawler states are automatically written to a `storage/` directory and resumed on restart.
- **Clean UI:** Responsive, vanilla JavaScript dashboard for managing crawlers and performing searches.

## Prerequisites
- **Go:** 1.21 or higher installed on your system.

## Running the Application
1. Clone the repository and navigate to the project root.
2. Run the application using the Go CLI:
   ```bash
   go run .
   ```
3. Open your web browser and navigate to `http://localhost:8080`.

## Usage
- **Crawler:** Go to the Crawler tab and initiate a job (e.g., `https://go.dev` with a depth of 2).
- **Dashboard:** Monitor active crawls, processes vs queued URLs, and throttling status.
- **Search:** Go to the Search tab, type a keyword, and view immediately ranked and paginated results.
