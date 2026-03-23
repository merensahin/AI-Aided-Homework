// Package server provides the HTTP server that serves the web UI
// and exposes REST API endpoints for crawler control and search.
package server

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	"hw1/crawler"
	"hw1/index"
	"hw1/search"
)

// Server holds the dependencies for the HTTP handlers.
type Server struct {
	crawler *crawler.Crawler
	idx     *index.InvertedIndex
}

// New creates a new Server instance.
func New(c *crawler.Crawler, idx *index.InvertedIndex) *Server {
	return &Server{
		crawler: c,
		idx:     idx,
	}
}

// Handler returns an http.Handler with all routes registered.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// API routes.
	mux.HandleFunc("/api/crawl/start", s.handleCrawlStart)
	mux.HandleFunc("/api/crawl/stop", s.handleCrawlStop)
	mux.HandleFunc("/api/crawl/status", s.handleCrawlStatus)
	mux.HandleFunc("/api/search", s.handleSearch)

	// Static file serving for the frontend.
	mux.Handle("/", http.FileServer(http.Dir("./static")))

	return mux
}

// startRequest is the expected JSON body for POST /api/crawl/start.
type startRequest struct {
	Origin       string `json:"origin"`
	Depth        int    `json:"depth"`
	RateLimit    int    `json:"rate_limit"`
	QueueCapacity int   `json:"queue_capacity"`
}

// handleCrawlStart starts a new crawl job.
func (s *Server) handleCrawlStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req startRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	jobID, err := s.crawler.StartJob(req.Origin, req.Depth, req.RateLimit, req.QueueCapacity)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"job_id": jobID})
}

// stopRequest is the expected JSON body for POST /api/crawl/stop.
type stopRequest struct {
	JobID string `json:"job_id"`
}

// handleCrawlStop stops a running crawl job.
func (s *Server) handleCrawlStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req stopRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if err := s.crawler.StopJob(req.JobID); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
}

// handleCrawlStatus returns the status of all crawl jobs.
func (s *Server) handleCrawlStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	statuses := s.crawler.GetStatus()
	writeJSON(w, http.StatusOK, statuses)
}

// handleSearch processes search queries and returns ranked results.
func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	q := r.URL.Query().Get("q")
	if q == "" {
		writeError(w, http.StatusBadRequest, "query parameter 'q' is required")
		return
	}

	// Pagination parameters.
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	size, _ := strconv.Atoi(r.URL.Query().Get("size"))
	if size < 1 || size > 100 {
		size = 10
	}

	results := search.Search(s.idx, q)
	total := len(results)

	// Apply pagination.
	start := (page - 1) * size
	if start >= total {
		results = nil
	} else {
		end := start + size
		if end > total {
			end = total
		}
		results = results[start:end]
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"results": results,
		"total":   total,
		"page":    page,
		"size":    size,
	})
}

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("Error encoding JSON response: %v", err)
	}
}

// writeError writes a JSON error response.
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
