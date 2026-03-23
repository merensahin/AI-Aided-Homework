// Package search provides query processing and result ranking for the
// search engine. It tokenizes queries, looks up words in the inverted index,
// and ranks results using keyword frequency plus title matching.
package search

import (
	"sort"
	"strings"
	"unicode"

	"hw1/index"
)

// Result represents a single search result with its relevancy score.
type Result struct {
	URL       string `json:"url"`
	OriginURL string `json:"origin_url"`
	Depth     int    `json:"depth"`
	Score     int    `json:"score"`
}

// TitleBonus is the score bonus applied when a query word appears in the page title.
const TitleBonus = 10

// Search takes a query string, tokenizes it, looks up each word in the
// inverted index, aggregates scores per URL, and returns ranked results.
// Ranking: score descending, then depth ascending (shallower pages first).
func Search(idx *index.InvertedIndex, query string) []Result {
	words := tokenizeQuery(query)
	if len(words) == 0 {
		return nil
	}

	// Aggregate scores per URL.
	type urlInfo struct {
		OriginURL string
		Depth     int
		Score     int
	}
	scores := make(map[string]*urlInfo)

	for _, word := range words {
		entries := idx.Lookup(word)
		for _, entry := range entries {
			info, exists := scores[entry.URL]
			if !exists {
				info = &urlInfo{
					OriginURL: entry.OriginURL,
					Depth:     entry.Depth,
				}
				scores[entry.URL] = info
			}

			// Add keyword frequency to score.
			info.Score += entry.Frequency

			// Title bonus: check if the query word is in the page title.
			if strings.Contains(strings.ToLower(entry.PageTitle), word) {
				info.Score += TitleBonus
			}
		}
	}

	// Convert to result slice.
	results := make([]Result, 0, len(scores))
	for pageURL, info := range scores {
		results = append(results, Result{
			URL:       pageURL,
			OriginURL: info.OriginURL,
			Depth:     info.Depth,
			Score:     info.Score,
		})
	}

	// Sort by score descending, then depth ascending.
	sort.Slice(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		return results[i].Depth < results[j].Depth
	})

	return results
}

// tokenizeQuery splits a query string into lowercase words, filtering
// out very short tokens (less than 2 characters).
func tokenizeQuery(query string) []string {
	var words []string
	var current strings.Builder

	for _, r := range query {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			current.WriteRune(unicode.ToLower(r))
		} else {
			if current.Len() >= 2 {
				words = append(words, current.String())
			}
			current.Reset()
		}
	}

	if current.Len() >= 2 {
		words = append(words, current.String())
	}

	return words
}
