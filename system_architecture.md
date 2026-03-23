# System Architecture: Real-Time Web Crawler and Search Engine

## 1. Technology Choice

### Language: Go (Golang)

Go is selected as the sole backend language for the following reasons:

| Requirement | Go Feature |
|---|---|
| Native HTTP requests (no Scrapy/BS4) | `net/http` (stdlib) |
| HTML parsing without high-level libraries | `golang.org/x/net/html` (low-level tokenizer) |
| Thread-safe concurrency | Goroutines, channels, `sync.Mutex`, `sync.Map` |
| Back-pressure / rate limiting | Buffered channels, semaphore pattern |
| Single-machine scalability | Lightweight goroutines (thousands concurrent with minimal memory) |
| File-based persistence | `os`, `encoding/json`, `bufio` (stdlib) |
| Web UI server | `net/http` serving static files + JSON API |

### Frontend: Vanilla HTML / CSS / JavaScript

No framework (React, Vue, etc.) is used. The three UI views (Crawler Init, Status Dashboard, Search Page) are built with plain HTML files served by Go's `net/http.FileServer`, communicating with the backend via `fetch()` calls to JSON API endpoints.

---

## 2. High-Level Architecture

```
┌─────────────────────────────────────────────────────────┐
│                      Web Browser                        │
│   ┌──────────────┬──────────────┬─────────────────┐     │
│   │ Crawler Init │   Dashboard  │   Search Page   │     │
│   └──────┬───────┴──────┬───────┴────────┬────────┘     │
└──────────┼──────────────┼────────────────┼──────────────┘
           │ HTTP/JSON    │ SSE/Polling    │ HTTP/JSON
           ▼              ▼                ▼
┌─────────────────────────────────────────────────────────┐
│                   Go HTTP Server                        │
│                  (server/server.go)                      │
│                                                         │
│   POST /api/crawl/start     GET /api/crawl/status       │
│   GET  /api/search?q=...    GET /api/crawl/stop         │
└────┬────────────────┬───────────────────┬───────────────┘
     │                │                   │
     ▼                ▼                   ▼
┌──────────┐   ┌─────────────┐    ┌────────────┐
│ Crawler  │──▶│  Inverted   │◀───│   Search   │
│ (Worker  │   │   Index     │    │   Engine   │
│  Pool)   │   │ (In-Memory  │    │            │
│          │   │  + Files)   │    │            │
└──────────┘   └─────────────┘    └────────────┘
     │                │
     ▼                ▼
┌──────────────────────────┐
│   storage/               │
│   ├── [jobId].data       │
│   ├── a.data ... z.data  │
│   └── visited_urls.data  │
└──────────────────────────┘
```

The system is a **single Go binary** that starts:
1. A pool of crawler goroutines (the indexer).
2. An HTTP server that serves the UI and exposes JSON API endpoints.
3. Both share access to a **thread-safe in-memory inverted index** protected by mutexes.

---

## 3. Project Structure

```
hw1/
├── main.go                  # Entry point: parses flags, starts server + crawler
├── crawler/
│   └── crawler.go           # Crawl orchestration, URL frontier, visited set, back-pressure
├── parser/
│   └── parser.go            # HTML tokenization, link extraction, text extraction
├── index/
│   └── index.go             # Inverted index: in-memory map + file persistence
├── search/
│   └── search.go            # Query processing, ranking, result formatting
├── server/
│   └── server.go            # HTTP handlers, API routes, static file serving
├── static/
│   ├── index.html           # Crawler init + dashboard + search (SPA or multi-page)
│   ├── style.css            # Styling
│   └── app.js               # Frontend logic (fetch calls, DOM updates)
├── storage/                 # Runtime directory (created at runtime)
│   ├── [jobId].data         # Crawler job state (bonus)
│   ├── a.data ... z.data    # Inverted index persistence (bonus)
│   └── visited_urls.data    # Visited URL set persistence (bonus)
├── product_prd.md
├── system_architecture.md
├── recommendation.md
└── readme.md
```

---

## 4. Component Details

### 4.1 Crawler (`crawler/crawler.go`)

#### Responsibilities
- Accept a crawl job defined by `(originURL, maxDepth, rateLimit, queueCapacity)`.
- Perform BFS/DFS traversal starting from the origin URL up to depth `k`.
- Feed discovered page content into the Inverted Index.
- Expose live metrics (processed count, queue depth, throttle status) for the dashboard.

#### Internal Data Structures

```go
type CrawlJob struct {
    ID            string    // Unique job ID: EpochTime_ThreadID
    OriginURL     string
    MaxDepth      int
    RateLimit     int       // Max requests per second
    QueueCapacity int       // Max URLs in the frontier queue
    Status        string    // "running", "paused", "completed", "stopped"
    Processed     int64     // Atomic counter of processed URLs
    StartedAt     time.Time
}

type Crawler struct {
    mu          sync.Mutex
    jobs        map[string]*CrawlJob
    visitedURLs sync.Map          // map[string]bool — thread-safe visited set
    frontier    chan URLEntry      // Buffered channel acts as the work queue
    semaphore   chan struct{}      // Controls max concurrent HTTP fetches
    index       *index.InvertedIndex // Shared reference to the inverted index
    stopCh      chan struct{}      // Signal to gracefully stop crawling
}

type URLEntry struct {
    URL       string
    OriginURL string
    Depth     int
}
```

#### Concurrency Model

```
                      ┌─────────────────────┐
                      │   Dispatcher Loop    │
                      │   (1 goroutine)      │
                      │                      │
                      │  Reads from frontier │
                      │  channel, respects   │
                      │  rate limiter        │
                      └──────────┬───────────┘
                                 │
              ┌──────────────────┼──────────────────┐
              ▼                  ▼                   ▼
        ┌───────────┐     ┌───────────┐      ┌───────────┐
        │  Worker 1 │     │  Worker 2 │ ...  │  Worker N │
        │ goroutine │     │ goroutine │      │ goroutine │
        └─────┬─────┘     └─────┬─────┘      └─────┬─────┘
              │                 │                    │
              │  1. HTTP GET    │                    │
              │  2. Parse HTML  │                    │
              │  3. Extract links + text             │
              │  4. Add new URLs to frontier          │
              │  5. Update inverted index             │
              ▼                 ▼                    ▼
        ┌─────────────────────────────────────────────┐
        │           Inverted Index (mutex-protected)  │
        └─────────────────────────────────────────────┘
```

- **Frontier (buffered channel):** The `frontier` channel has a capacity equal to `QueueCapacity`. When full, producers (workers discovering new links) block — this **is** the back-pressure mechanism. No URLs are dropped; workers simply wait until space is available.
- **Semaphore (buffered channel):** A `chan struct{}` of size `N` limits how many HTTP requests can be in-flight simultaneously. Before each fetch, a worker sends to the semaphore channel (blocks if full). After the fetch completes, it receives from the channel to release the slot.
- **Rate Limiter:** A `time.Ticker` in the dispatcher loop enforces a maximum request rate (e.g., 10 req/s). The dispatcher only dequeues from the frontier at the tick rate.
- **Visited Set (`sync.Map`):** Before enqueuing any URL, workers check `visitedURLs.LoadOrStore(url, true)`. If the URL was already present, it is skipped. This guarantees each URL is crawled at most once.

#### Crawl Flow (per URL)

1. Dispatcher reads a `URLEntry` from the `frontier` channel.
2. Dispatcher acquires a semaphore slot.
3. Dispatcher launches a worker goroutine for that URL.
4. Worker performs `http.Get(url)` using Go's `net/http`.
5. Worker passes the response body to `parser.Parse()`.
6. Parser returns `([]Link, []Word)`.
7. Worker calls `index.Add(words, url, originURL, depth)` — acquires mutex internally.
8. For each discovered link where `depth + 1 <= maxDepth`:
   - Check `visitedURLs.LoadOrStore(link, true)`.
   - If new, push `URLEntry{link, originURL, depth+1}` into `frontier`.
9. Worker releases the semaphore slot.
10. Worker increments `job.Processed` atomically.

---

### 4.2 HTML Parser (`parser/parser.go`)

#### Responsibilities
- Tokenize raw HTML using `golang.org/x/net/html`.
- Extract all `<a href="...">` links (absolute and relative → resolved to absolute).
- Extract visible text content from the page for indexing.
- Extract the page `<title>` for title-match boosting in search ranking.

#### Key Functions

```go
// Parse takes an io.Reader (HTTP response body) and the base URL.
// Returns extracted links and a word frequency map.
func Parse(body io.Reader, baseURL *url.URL) (links []string, title string, wordFreq map[string]int, err error)
```

#### Implementation Details

- Uses `html.NewTokenizer(body)` to iterate over tokens.
- On `html.StartTagToken` with tag `a`: extract `href` attribute, resolve against `baseURL` using `url.Parse()` / `url.ResolveReference()`.
- On `html.TextToken`: tokenize the text into words (split on whitespace/punctuation, lowercase, strip non-alphanumeric). Count frequencies.
- On `<title>` tag: capture inner text for the title field.
- Filters out non-HTTP(S) links, fragments, and mailto links.

---

### 4.3 Inverted Index (`index/index.go`)

#### Responsibilities
- Maintain an in-memory mapping from **word → list of occurrences**.
- Each occurrence records: `(frequency, relevantURL, originURL, depth)`.
- Support concurrent reads (search) and writes (indexing) safely.
- Persist index to disk as `storage/[letter].data` files (bonus feature).

#### Data Structures

```go
type Entry struct {
    Frequency  int    `json:"frequency"`
    URL        string `json:"url"`
    OriginURL  string `json:"origin_url"`
    Depth      int    `json:"depth"`
    PageTitle  string `json:"page_title"`
}

type InvertedIndex struct {
    mu    sync.RWMutex
    data  map[string][]Entry  // word → list of entries
}
```

#### Thread Safety

- **Writes** (`Add`): Acquire `mu.Lock()` (exclusive write lock). Append entries to the word's slice.
- **Reads** (`Search`): Acquire `mu.RLock()` (shared read lock). Multiple search queries can run concurrently without blocking each other. They only block during a write.

This `sync.RWMutex` approach is chosen over `sync.Map` because search requires iterating and ranking across entries — operations better suited to a regular map with an RWMutex.

#### Persistence (Bonus)

##### Inverted Index Files (`storage/[letter].data`)

- On a periodic interval (e.g., every 30 seconds) or on graceful shutdown, the index is flushed to disk.
- Words are grouped by their first letter: all words starting with `a` go to `storage/a.data`, etc.
- File format: one JSON object per line (newline-delimited JSON / JSONL):
  ```
  {"word":"apple","entries":[{"frequency":3,"url":"...","origin_url":"...","depth":2,"page_title":"..."}]}
  ```
- On startup, if `storage/*.data` files exist, they are loaded back into the in-memory index to resume a previous session.

##### Job State Files (`storage/[jobId].data`)

Each crawl job persists its full state to `storage/[jobId].data` so it can be resumed after an interruption. The file contains:

```json
{
  "job_id": "1679500000_1",
  "origin_url": "https://example.com",
  "max_depth": 3,
  "rate_limit": 10,
  "queue_capacity": 1000,
  "status": "running",
  "processed_count": 150,
  "queued_urls": [
    {"url": "https://example.com/page2", "origin_url": "https://example.com", "depth": 2}
  ],
  "logs": [
    {"timestamp": "2026-03-23T12:00:00Z", "message": "Started crawling https://example.com"},
    {"timestamp": "2026-03-23T12:00:05Z", "message": "Throttling activated: queue at capacity"}
  ]
}
```

- **Queued URLs:** The current contents of the frontier channel are drained and written to the `queued_urls` array on shutdown or periodic flush.
- **Logs:** Significant crawler events (start, stop, throttle activations, errors) are appended to an in-memory log buffer and flushed to the job file.
- **Flush frequency:** Same interval as the inverted index (every 30 seconds + on shutdown).

##### Resume After Interruption

On startup, the system checks `storage/` for existing job files:

1. Load all `[jobId].data` files where `status == "running"`.
2. Reload the visited URLs set from `storage/visited_urls.data`.
3. Reload the inverted index from `storage/[letter].data` files.
4. Re-seed the frontier channel with the `queued_urls` from each job file.
5. Resume the dispatcher loop — crawling continues from where it left off.

This ensures no work is repeated and no discovered data is lost.

---

### 4.4 Search Engine (`search/search.go`)

#### Responsibilities
- Accept a query string, tokenize it into individual words.
- Look up each word in the inverted index.
- Aggregate and rank results.
- Return a sorted list of `(relevant_url, origin_url, depth)` triples.

#### Ranking Heuristic

A simple scoring formula combining keyword frequency and title matching:

```
score(url) = Σ (frequency_of_query_word_in_url) + title_bonus
```

Where:
- `frequency_of_query_word_in_url` is the count of that query word on the page.
- `title_bonus` = **10** if any query word appears in the page title, **0** otherwise.
- If a page matches multiple query words, scores are summed.

Results are sorted by `score` descending. Ties are broken by `depth` ascending (shallower pages ranked higher).

#### Key Function

```go
// Search takes a query string and returns ranked results.
func Search(idx *index.InvertedIndex, query string) []Result

type Result struct {
    URL       string `json:"url"`
    OriginURL string `json:"origin_url"`
    Depth     int    `json:"depth"`
    Score     int    `json:"score"`
}
```

---

### 4.5 HTTP Server (`server/server.go`)

#### Responsibilities
- Serve the static frontend files (HTML/CSS/JS).
- Expose REST API endpoints for crawler control and search.

#### API Endpoints

| Method | Path | Description | Request Body / Params | Response |
|---|---|---|---|---|
| `POST` | `/api/crawl/start` | Start a new crawl job | `{"origin": "...", "depth": 3, "rate_limit": 10, "queue_capacity": 1000}` | `{"job_id": "..."}` |
| `POST` | `/api/crawl/stop` | Stop a running crawl job | `{"job_id": "..."}` | `{"status": "stopped"}` |
| `GET` | `/api/crawl/status` | Get status of all active jobs | — | `[{"job_id": "...", "status": "running", "processed": 150, "queue_depth": 42, "throttled": false}]` |
| `GET` | `/api/search` | Search the index | `?q=keyword&page=1&size=10` | `{"results": [...], "total": 85}` |

#### Static File Serving

```go
http.Handle("/", http.FileServer(http.Dir("./static")))
```

All API routes are registered under `/api/` to keep them separate from static file routes.

---

### 4.6 Frontend (`static/`)

Three views, implemented as sections in a single-page HTML file (or as 3 separate HTML files):

#### View 1: Crawler Initialization
- Form fields: Origin URL, Max Depth, Rate Limit (req/s), Queue Capacity.
- Submit button calls `POST /api/crawl/start`.
- Displays the returned Job ID on success.

#### View 2: Status Dashboard
- Polls `GET /api/crawl/status` every 2 seconds via `setInterval` + `fetch()`.
- Displays a card per active job showing:
  - Job ID, Origin URL, Status.
  - Processed URLs count vs. Queue Depth (progress indicator).
  - Whether back-pressure/throttling is active.
- Stop button per job calls `POST /api/crawl/stop`.

#### View 3: Search Page
- Text input + Search button.
- Calls `GET /api/search?q=...&page=1&size=10`.
- Displays results as a list of `(URL, Origin URL, Depth, Score)`.
- Pagination controls (Previous / Next) update the `page` param.

---

## 5. Data Flow Summary

```
User submits crawl job via UI
        │
        ▼
POST /api/crawl/start
        │
        ▼
Crawler creates CrawlJob, seeds frontier with origin URL
        │
        ▼
Dispatcher loop dequeues URLs at rate-limited pace
        │
        ▼
Worker goroutines fetch pages (net/http.Get)
        │
        ├──▶ parser.Parse() extracts links + word frequencies
        │
        ├──▶ New links enqueued to frontier (if not visited, depth < max)
        │
        └──▶ index.Add() updates inverted index (mutex-protected)
        
Meanwhile, concurrently:

User submits search query via UI
        │
        ▼
GET /api/search?q=keyword
        │
        ▼
search.Search() acquires RLock on inverted index
        │
        ▼
Looks up query words, aggregates scores, sorts results
        │
        ▼
Returns ranked (url, origin_url, depth) triples as JSON
```

---

## 6. Back-Pressure Mechanism (Detailed)

Back-pressure is implemented via **three layers**:

1. **Frontier Queue Capacity:** The `frontier` channel has a fixed buffer size (e.g., 1000). When full, any goroutine trying to enqueue a new URL blocks until a slot opens. This naturally throttles discovery when processing can't keep up.

2. **Concurrency Semaphore:** A `chan struct{}` of fixed size (e.g., 20) limits the number of simultaneous HTTP connections. This prevents the machine from running out of file descriptors or memory.

3. **Rate Limiter:** A `time.Ticker` ensures the dispatcher dequeues at most N URLs per second (e.g., 10/s). This prevents overwhelming target servers and the local machine.

All three parameters (`queue_capacity`, `max_concurrent`, `rate_limit`) are user-configurable at job creation time.

---

## 7. Thread Safety Summary

| Shared Resource | Protection Mechanism | Readers | Writers |
|---|---|---|---|
| Inverted Index (`map[string][]Entry`) | `sync.RWMutex` | Search goroutines (RLock) | Worker goroutines (Lock) |
| Visited URLs (`map[string]bool`) | `sync.Map` | Workers (LoadOrStore) | Workers (LoadOrStore) |
| Frontier queue | Buffered channel | Dispatcher (receive) | Workers (send) |
| Job metadata (status, counters) | `sync.Mutex` + `atomic` | Server handlers | Crawler workers |

---

## 8. Graceful Shutdown

When the user stops a job or the process receives `SIGINT`/`SIGTERM`:

1. Close the `stopCh` channel to signal all goroutines.
2. Workers finish their current fetch, then exit.
3. Dispatcher drains remaining frontier URLs into a slice for persistence.
4. Each job's state (including queued URLs and logs) is written to `storage/[jobId].data`.
5. Inverted index is flushed to disk (`storage/[letter].data`).
6. Visited URLs are written to `storage/visited_urls.data`.
7. HTTP server calls `server.Shutdown(ctx)` with a timeout.

On next startup, the system detects these files and resumes interrupted jobs automatically (see §4.3 — Resume After Interruption).

---

## 9. External Dependencies

Only **two** non-stdlib packages are used:

| Package | Purpose | Justification |
|---|---|---|
| `golang.org/x/net/html` | HTML tokenization | Low-level tokenizer, not a scraping framework. Complies with the PRD constraint. |
| `golang.org/x/time/rate` | Token-bucket rate limiter (optional) | Cleaner than manual `time.Ticker`. Can be replaced with stdlib if purity is preferred. |

Everything else uses the Go standard library.

---

## 10. How to Run

```bash
# Clone and enter the project
cd hw1

# Run the application (Go 1.21+)
go run .

# The server starts on http://localhost:8080
# Open the browser to access the UI
```

No Docker, no database, no external services. One command, one binary, one port.
