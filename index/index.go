// Package index provides a thread-safe inverted index for the search engine.
// It maps words to their occurrences across crawled pages, supporting
// concurrent reads (search) and writes (indexing) via sync.RWMutex.
package index

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode"
)

// Entry represents a single occurrence of a word on a crawled page.
type Entry struct {
	Frequency int    `json:"frequency"`
	URL       string `json:"url"`
	OriginURL string `json:"origin_url"`
	Depth     int    `json:"depth"`
	PageTitle string `json:"page_title"`
}

// wordRecord is used for JSON serialization of index data to disk.
type wordRecord struct {
	Word    string  `json:"word"`
	Entries []Entry `json:"entries"`
}

// InvertedIndex is a thread-safe mapping from words to their page occurrences.
// It uses sync.RWMutex to allow concurrent search reads while indexing writes.
type InvertedIndex struct {
	// mu guards the data map. Acquire before reading or writing.
	mu   sync.RWMutex
	data map[string][]Entry
}

// New creates and returns a new empty InvertedIndex.
func New() *InvertedIndex {
	return &InvertedIndex{
		data: make(map[string][]Entry),
	}
}

// Add inserts word frequency data from a crawled page into the index.
// It acquires an exclusive write lock to ensure thread safety.
func (idx *InvertedIndex) Add(wordFreq map[string]int, pageURL, originURL string, depth int, title string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	for word, freq := range wordFreq {
		entry := Entry{
			Frequency: freq,
			URL:       pageURL,
			OriginURL: originURL,
			Depth:     depth,
			PageTitle: title,
		}
		idx.data[word] = append(idx.data[word], entry)
	}
}

// Lookup returns all entries for a given word.
// It acquires a shared read lock, allowing concurrent lookups.
func (idx *InvertedIndex) Lookup(word string) []Entry {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	entries := idx.data[strings.ToLower(word)]
	if entries == nil {
		return nil
	}

	// Return a copy to avoid holding the lock after return.
	result := make([]Entry, len(entries))
	copy(result, entries)
	return result
}

// Size returns the number of unique words in the index.
func (idx *InvertedIndex) Size() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return len(idx.data)
}

// SaveToDisk persists the inverted index to disk, grouping words by
// their first letter into separate files (e.g., storage/a.data).
// File format is newline-delimited JSON (JSONL).
func (idx *InvertedIndex) SaveToDisk(dir string) error {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating storage dir: %w", err)
	}

	// Group words by first letter.
	groups := make(map[rune][]wordRecord)
	for word, entries := range idx.data {
		if len(word) == 0 {
			continue
		}
		firstChar := unicode.ToLower(rune(word[0]))
		if !unicode.IsLetter(firstChar) {
			firstChar = '#' // Non-letter words grouped under #.data
		}
		groups[firstChar] = append(groups[firstChar], wordRecord{
			Word:    word,
			Entries: entries,
		})
	}

	// Write each group to its own file.
	for letter, records := range groups {
		filename := filepath.Join(dir, fmt.Sprintf("%c.data", letter))
		f, err := os.Create(filename)
		if err != nil {
			return fmt.Errorf("creating %s: %w", filename, err)
		}

		writer := bufio.NewWriter(f)
		encoder := json.NewEncoder(writer)
		for _, rec := range records {
			if err := encoder.Encode(rec); err != nil {
				f.Close()
				return fmt.Errorf("encoding word %q: %w", rec.Word, err)
			}
		}

		if err := writer.Flush(); err != nil {
			f.Close()
			return fmt.Errorf("flushing %s: %w", filename, err)
		}
		f.Close()
	}

	return nil
}

// LoadFromDisk reads previously persisted index data from the storage
// directory and loads it into the in-memory index.
func (idx *InvertedIndex) LoadFromDisk(dir string) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	files, err := filepath.Glob(filepath.Join(dir, "*.data"))
	if err != nil {
		return fmt.Errorf("globbing data files: %w", err)
	}

	for _, filename := range files {
		// Skip non-index files (e.g., job state files, visited_urls.data).
		base := filepath.Base(filename)
		if base == "visited_urls.data" || len(base) > len("x.data") {
			continue
		}

		f, err := os.Open(filename)
		if err != nil {
			return fmt.Errorf("opening %s: %w", filename, err)
		}

		scanner := bufio.NewScanner(f)
		// Increase scanner buffer for large lines.
		scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)

		for scanner.Scan() {
			var rec wordRecord
			if err := json.Unmarshal(scanner.Bytes(), &rec); err != nil {
				f.Close()
				return fmt.Errorf("decoding line in %s: %w", filename, err)
			}
			idx.data[rec.Word] = append(idx.data[rec.Word], rec.Entries...)
		}

		if err := scanner.Err(); err != nil {
			f.Close()
			return fmt.Errorf("reading %s: %w", filename, err)
		}
		f.Close()
	}

	return nil
}
