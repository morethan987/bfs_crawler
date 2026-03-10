package utils

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
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

// Fetch retrieves the body of the given URL with retry logic.
// Returns a FetchResult containing the raw body bytes and Content-Type info.
func (f *Fetcher) Fetch(rawURL string) (*FetchResult, error) {
	var lastErr error
	for attempt := 0; attempt <= f.maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(f.delay)
		}
		result, err := f.doFetch(rawURL)
		if err == nil {
			return result, nil
		}
		lastErr = err
	}
	return nil, fmt.Errorf("fetch failed after %d retries: %w", f.maxRetries, lastErr)
}

func (f *Fetcher) doFetch(rawURL string) (*FetchResult, error) {
	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	if f.userAgent != "" {
		req.Header.Set("User-Agent", f.userAgent)
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP error: %d %s", resp.StatusCode, resp.Status)
	}
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	contentType := resp.Header.Get("Content-Type")
	isHTML := strings.Contains(strings.ToLower(contentType), "text/html")

	return &FetchResult{
		Body:        bodyBytes,
		ContentType: contentType,
		IsHTML:      isHTML,
	}, nil
}

// skipExtensions contains file extensions that are not useful for crawling.
var skipExtensions = map[string]struct{}{
	".css": {}, ".js": {}, ".png": {}, ".jpg": {}, ".jpeg": {}, ".gif": {},
	".svg": {}, ".ico": {}, ".woff": {}, ".woff2": {}, ".ttf": {}, ".eot": {},
	".mp3": {}, ".mp4": {}, ".avi": {}, ".mov": {}, ".zip": {}, ".tar": {},
	".gz": {}, ".rar": {}, ".exe": {}, ".dmg": {}, ".apk": {}, ".deb": {},
	".rpm": {}, ".iso": {},
}

