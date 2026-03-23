// Package crawler provides the web crawling engine that traverses pages
// starting from an origin URL up to a maximum depth. It uses goroutine-based
// concurrency with three layers of back-pressure: frontier queue capacity,
// concurrency semaphore, and rate limiting.
package crawler

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"hw1/index"
	"hw1/parser"
)

// DefaultTimeout is the HTTP client timeout for fetching pages.
const DefaultTimeout = 10 * time.Second

// MaxBodySize limits response body reads to prevent memory exhaustion.
const MaxBodySize = 10 * 1024 * 1024 // 10 MB

// JobStatus represents the current state of a crawl job for the dashboard.
type JobStatus struct {
	JobID      string `json:"job_id"`
	OriginURL  string `json:"origin_url"`
	Status     string `json:"status"`
	Processed  int64  `json:"processed"`
	QueueDepth int    `json:"queue_depth"`
	Throttled  bool   `json:"throttled"`
}

// urlEntry is an internal item in the crawl frontier queue.
type urlEntry struct {
	URL       string
	OriginURL string
	Depth     int
}

// UserAgent is sent with every HTTP request to avoid 403 blocks.
const UserAgent = "hw1-crawler/1.0 (educational project)"

// crawlJob holds the state and control channels for a single crawl job.
type crawlJob struct {
	ID            string
	OriginURL     string
	MaxDepth      int
	RateLimit     int
	QueueCapacity int
	Status        string // "running", "stopped", "completed"
	Processed     atomic.Int64
	activeWorkers atomic.Int64 // tracks in-flight workers for completion detection
	frontier      chan urlEntry
	stopCh        chan struct{}
	wg            sync.WaitGroup
}

// Crawler manages crawl jobs and coordinates workers.
type Crawler struct {
	mu          sync.Mutex
	jobs        map[string]*crawlJob
	visitedURLs sync.Map
	idx         *index.InvertedIndex
	client      *http.Client
}

// New creates a new Crawler instance that writes to the given inverted index.
func New(idx *index.InvertedIndex) *Crawler {
	return &Crawler{
		jobs: make(map[string]*crawlJob),
		idx:  idx,
		client: &http.Client{
			Timeout: DefaultTimeout,
		},
	}
}

// StartJob creates and starts a new crawl job. Returns the generated job ID.
func (c *Crawler) StartJob(origin string, maxDepth, rateLimit, queueCap int) (string, error) {
	// Validate input.
	if origin == "" {
		return "", fmt.Errorf("origin URL is required")
	}
	parsed, err := url.Parse(origin)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", fmt.Errorf("invalid origin URL: must be http or https")
	}
	if maxDepth < 0 || maxDepth > 10 {
		return "", fmt.Errorf("depth must be between 0 and 10")
	}
	if rateLimit <= 0 {
		rateLimit = 10 // Default: 10 requests/second
	}
	if queueCap <= 0 {
		queueCap = 1000 // Default queue capacity
	}

	jobID := fmt.Sprintf("%d_%d", time.Now().UnixMilli(), time.Now().Nanosecond()%1000)

	job := &crawlJob{
		ID:            jobID,
		OriginURL:     origin,
		MaxDepth:      maxDepth,
		RateLimit:     rateLimit,
		QueueCapacity: queueCap,
		Status:        "running",
		frontier:      make(chan urlEntry, queueCap),
		stopCh:        make(chan struct{}),
	}

	c.mu.Lock()
	c.jobs[jobID] = job
	c.mu.Unlock()

	// Seed the frontier with the origin URL.
	c.visitedURLs.Store(origin, true)
	job.frontier <- urlEntry{URL: origin, OriginURL: origin, Depth: 0}

	// Launch the dispatcher goroutine.
	go c.dispatch(job)

	log.Printf("[Job %s] Started crawling %s (depth=%d, rate=%d/s, queue=%d)",
		jobID, origin, maxDepth, rateLimit, queueCap)

	return jobID, nil
}

// StopJob signals a running crawl job to stop gracefully.
func (c *Crawler) StopJob(jobID string) error {
	c.mu.Lock()
	job, ok := c.jobs[jobID]
	c.mu.Unlock()

	if !ok {
		return fmt.Errorf("job %s not found", jobID)
	}

	if job.Status != "running" {
		return fmt.Errorf("job %s is not running (status: %s)", jobID, job.Status)
	}

	close(job.stopCh)
	job.wg.Wait()
	job.Status = "stopped"

	log.Printf("[Job %s] Stopped. Processed %d URLs.", jobID, job.Processed.Load())
	return nil
}

// GetStatus returns the current status of all crawl jobs.
func (c *Crawler) GetStatus() []JobStatus {
	c.mu.Lock()
	defer c.mu.Unlock()

	statuses := make([]JobStatus, 0, len(c.jobs))
	for _, job := range c.jobs {
		statuses = append(statuses, JobStatus{
			JobID:      job.ID,
			OriginURL:  job.OriginURL,
			Status:     job.Status,
			Processed:  job.Processed.Load(),
			QueueDepth: len(job.frontier),
			Throttled:  len(job.frontier) >= job.QueueCapacity-1,
		})
	}
	return statuses
}

// dispatch is the main dispatcher loop for a crawl job. It reads URLs from
// the frontier at a rate-limited pace and launches worker goroutines.
func (c *Crawler) dispatch(job *crawlJob) {
	// Semaphore to limit concurrent HTTP fetches.
	maxConcurrent := job.RateLimit * 2
	if maxConcurrent < 5 {
		maxConcurrent = 5
	}
	if maxConcurrent > 50 {
		maxConcurrent = 50
	}
	semaphore := make(chan struct{}, maxConcurrent)

	// Rate limiter: tick at the configured rate.
	interval := time.Second / time.Duration(job.RateLimit)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-job.stopCh:
			// Wait for all in-flight workers to finish.
			job.wg.Wait()
			return

		case <-ticker.C:
			// Try to dequeue a URL from the frontier.
			select {
			case entry := <-job.frontier:
				// Acquire semaphore slot.
				semaphore <- struct{}{}
				job.wg.Add(1)
				job.activeWorkers.Add(1)

				go func(e urlEntry) {
					defer job.wg.Done()
					defer job.activeWorkers.Add(-1)
					defer func() { <-semaphore }()

					c.processURL(job, e)
				}(entry)

			default:
				// Frontier is empty. If no workers are active, crawl is complete.
				if job.activeWorkers.Load() == 0 {
					job.Status = "completed"
					log.Printf("[Job %s] Completed. Processed %d URLs.", job.ID, job.Processed.Load())
					return
				}
			}
		}
	}
}

// processURL fetches a single URL, parses its HTML, updates the index,
// and enqueues discovered links.
func (c *Crawler) processURL(job *crawlJob, entry urlEntry) {
	// Build request with User-Agent header.
	req, err := http.NewRequest("GET", entry.URL, nil)
	if err != nil {
		log.Printf("[Job %s] Error creating request for %s: %v", job.ID, entry.URL, err)
		job.Processed.Add(1)
		return
	}
	req.Header.Set("User-Agent", UserAgent)

	// Fetch the page.
	resp, err := c.client.Do(req)
	if err != nil {
		log.Printf("[Job %s] Error fetching %s: %v", job.ID, entry.URL, err)
		job.Processed.Add(1)
		return
	}
	defer resp.Body.Close()

	// Skip non-OK responses.
	if resp.StatusCode != http.StatusOK {
		log.Printf("[Job %s] Skipping %s (status %d)", job.ID, entry.URL, resp.StatusCode)
		job.Processed.Add(1)
		return
	}

	// Only process HTML responses.
	contentType := resp.Header.Get("Content-Type")
	if !isHTML(contentType) {
		job.Processed.Add(1)
		return
	}

	// Limit body size to prevent memory exhaustion.
	body := io.LimitReader(resp.Body, MaxBodySize)

	// Parse the page.
	baseURL, err := url.Parse(entry.URL)
	if err != nil {
		job.Processed.Add(1)
		return
	}

	links, title, wordFreq, err := parser.Parse(body, baseURL)
	if err != nil {
		log.Printf("[Job %s] Error parsing %s: %v", job.ID, entry.URL, err)
		job.Processed.Add(1)
		return
	}

	// Update the inverted index.
	if len(wordFreq) > 0 {
		c.idx.Add(wordFreq, entry.URL, entry.OriginURL, entry.Depth, title)
	}

	// Enqueue newly discovered links.
	if entry.Depth < job.MaxDepth {
		for _, link := range links {
			// Check if already visited.
			if _, loaded := c.visitedURLs.LoadOrStore(link, true); loaded {
				continue
			}

			newEntry := urlEntry{
				URL:       link,
				OriginURL: entry.OriginURL,
				Depth:     entry.Depth + 1,
			}

			// Non-blocking enqueue: if frontier is full, skip this link
			// to avoid blocking the worker (back-pressure).
			select {
			case job.frontier <- newEntry:
			default:
				// Frontier full — back-pressure in effect.
			}
		}
	}

	job.Processed.Add(1)
}

// isHTML checks if a Content-Type header indicates an HTML response.
func isHTML(contentType string) bool {
	if contentType == "" {
		return true // Assume HTML if no content type.
	}
	ct := strings.ToLower(contentType)
	return strings.Contains(ct, "text/html") || strings.Contains(ct, "application/xhtml")
}
