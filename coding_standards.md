# Coding Standards & Rules

This document defines the coding conventions and rules to follow throughout the project. All contributors must adhere to these standards for consistency and maintainability.

---

## 1. Language & Tooling

| Item | Standard |
|---|---|
| Language | Go 1.21+ |
| Formatter | `gofmt` (mandatory — no unformatted code gets committed) |
| Linter | `go vet` + `staticcheck` |
| Module name | `hw1` (matches the project directory) |
| Build | `go build .` must pass with zero warnings |

---

## 2. Project Layout

Follow the structure defined in `system_architecture.md`. Key rules:

- **One package per directory.** Each folder (`crawler/`, `parser/`, `index/`, `search/`, `server/`) is its own Go package.
- **No circular imports.** Dependency direction is strictly: `main → server → crawler/search → index → parser`. Never import upward.
- **`main.go`** is the only file in the root package. It only wires things together and starts the server.
- **Static frontend files** go in `static/`. No Go code in that directory.
- **Runtime data** goes in `storage/`. This directory is `.gitignore`d.

---

## 3. Naming Conventions

### Go Code

| Element | Convention | Example |
|---|---|---|
| Packages | Short, singular, lowercase | `crawler`, `index`, `parser` |
| Exported types | PascalCase, noun | `CrawlJob`, `InvertedIndex` |
| Exported functions | PascalCase, verb-first | `StartJob()`, `Search()`, `Parse()` |
| Unexported helpers | camelCase | `resolveURL()`, `tokenizeText()` |
| Constants | PascalCase | `DefaultRateLimit`, `MaxQueueCapacity` |
| Interfaces | PascalCase, `-er` suffix when possible | `Searcher`, `Parser` |
| Acronyms | All caps in identifiers | `URL`, `HTTP`, `ID` (not `Url`, `Http`, `Id`) |
| File names | snake_case | `crawl_job.go`, `inverted_index.go` |
| Test files | `*_test.go` | `crawler_test.go` |

### Frontend (JS/CSS/HTML)

| Element | Convention | Example |
|---|---|---|
| File names | kebab-case | `app.js`, `style.css` |
| JS variables | camelCase | `queueDepth`, `jobId` |
| JS functions | camelCase, verb-first | `fetchStatus()`, `renderResults()` |
| CSS classes | kebab-case | `.search-input`, `.job-card` |
| HTML IDs | kebab-case, unique per page | `#crawl-form`, `#search-results` |

---

## 4. Error Handling

- **Never ignore errors.** Every function call that returns an error must be checked.
  ```go
  // ✅ Correct
  resp, err := http.Get(url)
  if err != nil {
      return fmt.Errorf("fetching %s: %w", url, err)
  }

  // ❌ Wrong
  resp, _ := http.Get(url)
  ```
- **Wrap errors with context** using `fmt.Errorf("context: %w", err)`.
- **Log at the top level, return errors everywhere else.** Internal packages return errors; `main.go` and HTTP handlers decide whether to log or respond with an error.
- **HTTP handlers** return proper status codes:
  - `400` for bad input
  - `404` for not found
  - `500` for internal errors
  - Always return JSON error bodies: `{"error": "message"}`

---

## 5. Concurrency Rules

- **Never use a bare goroutine.** Every goroutine must have a way to be stopped (via `context.Context`, a `stopCh`, or a `sync.WaitGroup`).
  ```go
  // ✅ Correct
  go func() {
      defer wg.Done()
      select {
      case <-stopCh:
          return
      case url := <-frontier:
          process(url)
      }
  }()

  // ❌ Wrong — no way to stop or wait
  go process(url)
  ```
- **Protect shared state explicitly.** Document which mutex/channel guards which data with a comment:
  ```go
  // mu guards the data map. Acquire before reading or writing.
  mu    sync.RWMutex
  data  map[string][]Entry
  ```
- **Prefer channels for communication, mutexes for state protection.** Don't mix the two for the same resource.
- **Keep critical sections small.** Lock, do the minimum work, unlock. Never do I/O while holding a lock.
- **Use `sync.RWMutex`** when reads vastly outnumber writes (e.g., the inverted index).
- **Use `atomic` operations** for simple counters (e.g., `job.Processed`).

---

## 6. HTTP & API Standards

- All API routes live under `/api/`.
- Use **JSON** for all request/response bodies.
- Set `Content-Type: application/json` on all API responses.
- Use standard HTTP methods: `GET` for reads, `POST` for actions.
- API responses always have a consistent structure:
  ```json
  // Success
  {"data": { ... }}

  // Error
  {"error": "descriptive message"}
  ```
- **No hardcoded ports.** Use a constant or flag: `const DefaultPort = 8080`.

---

## 7. Code Documentation

- **Every exported function, type, and constant** must have a GoDoc comment.
  ```go
  // Search takes a query string, tokenizes it, and returns
  // ranked results from the inverted index.
  func Search(idx *InvertedIndex, query string) []Result {
  ```
- **Package-level comments** in a `doc.go` file or at the top of the main file in each package.
- **Non-obvious logic** must have inline comments explaining *why*, not *what*.
- **No commented-out code.** Delete it; Git has history.

---

## 8. Testing

- Every package must have at least one `_test.go` file.
- Test function naming: `TestFunctionName_Scenario`.
  ```go
  func TestParse_ExtractsLinksFromHTML(t *testing.T) { ... }
  func TestSearch_RanksResultsByFrequency(t *testing.T) { ... }
  ```
- Use **table-driven tests** for functions with multiple input/output cases.
  ```go
  tests := []struct {
      name  string
      input string
      want  int
  }{
      {"empty query", "", 0},
      {"single word", "hello", 1},
  }
  for _, tt := range tests {
      t.Run(tt.name, func(t *testing.T) { ... })
  }
  ```
- Tests must not depend on network access or external state. Use `httptest.NewServer` for HTTP tests.
- Run all tests before committing: `go test ./...`

---

## 9. Git & Version Control

- **Commit messages:** Imperative mood, 50-char subject line max.
  ```
  Add inverted index persistence to disk
  Fix race condition in frontier queue
  ```
- **One logical change per commit.** Don't mix refactoring with feature work.
- **`.gitignore`** must include:
  ```
  storage/
  *.data
  hw1          # compiled binary
  ```
- **No binary files** in the repo.

---

## 10. Performance Guidelines

- **Buffer I/O:** Use `bufio.Scanner` / `bufio.Writer` for file operations, never raw `os.Read/Write`.
- **Close response bodies:** Always `defer resp.Body.Close()` after `http.Get`.
- **Set timeouts on HTTP clients:**
  ```go
  client := &http.Client{
      Timeout: 10 * time.Second,
  }
  ```
- **Limit response body size** to prevent memory exhaustion on large pages:
  ```go
  body := io.LimitReader(resp.Body, 10*1024*1024) // 10 MB max
  ```
- **Reuse the HTTP client.** Create one `*http.Client` per crawler, not per request.

---

## 11. Security Considerations

- **Validate all user input** in API handlers (origin URL format, depth > 0, rate limit > 0).
- **Only crawl HTTP/HTTPS URLs.** Reject `file://`, `javascript:`, `data:` schemes.
- **Set a maximum crawl depth** (e.g., cap at 10) regardless of user input to prevent runaway crawls.
- **Respect `robots.txt`** — optional but recommended. If not implemented, document the limitation.

---

## Quick Reference Checklist

Before submitting any code, verify:

- [ ] `gofmt` applied
- [ ] `go vet` passes
- [ ] All errors handled
- [ ] No goroutines without shutdown mechanism
- [ ] Exported symbols have GoDoc comments
- [ ] Tests exist and pass (`go test ./...`)
- [ ] No hardcoded values — use constants or config
- [ ] HTTP client has timeouts and body limits set
