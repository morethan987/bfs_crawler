package utils

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// Fetcher handles HTTP fetching with retry and link extraction.
type Fetcher struct {
	client     *http.Client
	userAgent  string
	delay      time.Duration
	maxRetries int
}

// NewFetcher creates a new Fetcher with the given HTTP config.
func NewFetcher(userAgent string, timeoutSec int, delayMs int, maxRetries int) *Fetcher {
	return &Fetcher{
		client: &http.Client{
			Timeout: time.Duration(timeoutSec) * time.Second,
		},
		userAgent:  userAgent,
		delay:      time.Duration(delayMs) * time.Millisecond,
		maxRetries: maxRetries,
	}
}

// Fetch retrieves the HTML body of the given URL with retry logic.
func (f *Fetcher) Fetch(rawURL string) (string, error) {
	var lastErr error
	for attempt := 0; attempt <= f.maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(f.delay)
		}
		body, err := f.doFetch(rawURL)
		if err == nil {
			return body, nil
		}
		lastErr = err
	}
	return "", fmt.Errorf("fetch failed after %d retries: %w", f.maxRetries, lastErr)
}

func (f *Fetcher) doFetch(rawURL string) (string, error) {
	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	if f.userAgent != "" {
		req.Header.Set("User-Agent", f.userAgent)
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("HTTP error: %d %s", resp.StatusCode, resp.Status)
	}
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}
	return string(bodyBytes), nil
}

// ExtractLinks parses HTML and returns a deduplicated list of absolute URLs
// found in <a href="..."> elements, resolved against baseURL.
func (f *Fetcher) ExtractLinks(htmlBody string, baseURL string) ([]string, error) {
	base, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}

	seen := make(map[string]struct{})
	var links []string

	tokenizer := html.NewTokenizer(strings.NewReader(htmlBody))
	for {
		tt := tokenizer.Next()
		if tt == html.ErrorToken {
			break
		}
		if tt == html.StartTagToken || tt == html.SelfClosingTagToken {
			token := tokenizer.Token()
			if token.Data != "a" {
				continue
			}
			for _, attr := range token.Attr {
				if attr.Key != "href" {
					continue
				}
				href := strings.TrimSpace(attr.Val)
				if href == "" || strings.HasPrefix(href, "#") || strings.HasPrefix(href, "javascript:") || strings.HasPrefix(href, "mailto:") {
					continue
				}
				ref, err := url.Parse(href)
				if err != nil {
					continue
				}
				abs := base.ResolveReference(ref).String()
				if _, exists := seen[abs]; !exists {
					seen[abs] = struct{}{}
					links = append(links, abs)
				}
			}
		}
	}
	return links, nil
}
