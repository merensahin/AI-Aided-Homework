// Package parser provides HTML tokenization and content extraction.
// It uses golang.org/x/net/html to parse HTML documents, extracting
// links, page titles, and word frequencies for indexing.
package parser

import (
	"io"
	"net/url"
	"strings"
	"unicode"

	"golang.org/x/net/html"
)

// Parse takes an io.Reader (typically an HTTP response body) and the base URL
// of the page. It returns extracted links, the page title, and a word frequency
// map for indexing purposes.
func Parse(body io.Reader, baseURL *url.URL) (links []string, title string, wordFreq map[string]int, err error) {
	wordFreq = make(map[string]int)
	seen := make(map[string]bool)

	tokenizer := html.NewTokenizer(body)
	var inTitle bool
	var inScript bool
	var inStyle bool

	for {
		tt := tokenizer.Next()

		switch tt {
		case html.ErrorToken:
			err := tokenizer.Err()
			if err == io.EOF {
				return links, strings.TrimSpace(title), wordFreq, nil
			}
			return links, strings.TrimSpace(title), wordFreq, err

		case html.StartTagToken:
			tn, hasAttr := tokenizer.TagName()
			tagName := string(tn)

			switch tagName {
			case "title":
				inTitle = true
			case "script":
				inScript = true
			case "style":
				inStyle = true
			case "a":
				if hasAttr {
					href := extractHref(tokenizer)
					if href != "" {
						resolved := resolveURL(href, baseURL)
						if resolved != "" && !seen[resolved] {
							seen[resolved] = true
							links = append(links, resolved)
						}
					}
				}
			}

		case html.EndTagToken:
			tn, _ := tokenizer.TagName()
			tagName := string(tn)

			switch tagName {
			case "title":
				inTitle = false
			case "script":
				inScript = false
			case "style":
				inStyle = false
			}

		case html.TextToken:
			text := string(tokenizer.Text())

			if inTitle {
				title += text
			}

			// Skip text inside script and style tags.
			if inScript || inStyle {
				continue
			}

			words := tokenizeText(text)
			for _, w := range words {
				wordFreq[w]++
			}
		}
	}
}

// extractHref reads through the attributes of the current token
// and returns the value of the "href" attribute, if present.
func extractHref(tokenizer *html.Tokenizer) string {
	for {
		key, val, moreAttr := tokenizer.TagAttr()
		if string(key) == "href" {
			return string(val)
		}
		if !moreAttr {
			break
		}
	}
	return ""
}

// resolveURL resolves a potentially relative href against the base URL.
// It returns an empty string for non-HTTP(S) schemes, fragment-only refs,
// and mailto/javascript links.
func resolveURL(href string, baseURL *url.URL) string {
	href = strings.TrimSpace(href)
	if href == "" || strings.HasPrefix(href, "#") {
		return ""
	}

	// Filter out non-HTTP schemes early.
	lower := strings.ToLower(href)
	if strings.HasPrefix(lower, "mailto:") ||
		strings.HasPrefix(lower, "javascript:") ||
		strings.HasPrefix(lower, "data:") ||
		strings.HasPrefix(lower, "file:") ||
		strings.HasPrefix(lower, "tel:") {
		return ""
	}

	parsed, err := url.Parse(href)
	if err != nil {
		return ""
	}

	resolved := baseURL.ResolveReference(parsed)

	// Only allow HTTP and HTTPS.
	if resolved.Scheme != "http" && resolved.Scheme != "https" {
		return ""
	}

	// Strip fragment from the resolved URL to avoid duplicate entries.
	resolved.Fragment = ""

	return resolved.String()
}

// tokenizeText splits a text string into lowercase words,
// filtering out non-alphanumeric characters and short tokens.
func tokenizeText(text string) []string {
	var words []string
	var current strings.Builder

	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			current.WriteRune(unicode.ToLower(r))
		} else {
			if current.Len() >= 2 {
				words = append(words, current.String())
			}
			current.Reset()
		}
	}

	// Flush remaining word.
	if current.Len() >= 2 {
		words = append(words, current.String())
	}

	return words
}
