package utils

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// SanitizePath sanitizes a single path component (folder or file name).
// Prevents path traversal, removes illegal characters, and truncates to safe length.
func SanitizePath(name string) string {
	// Trim whitespace
	name = strings.TrimSpace(name)
	if name == "" {
		return "unnamed"
	}

	// Replace OS-illegal characters (including path separators and shell special chars)
	illegal := `\/:*?"<>|`
	for _, ch := range illegal {
		name = strings.ReplaceAll(name, string(ch), "_")
	}

	// Strip path traversal components: split by path separators and join clean parts
	parts := strings.FieldsFunc(name, func(r rune) bool {
		return r == '/' || r == '\\'
	})
	var cleanParts []string
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == ".." || part == "." || part == "" {
			continue
		}
		cleanParts = append(cleanParts, part)
	}
	name = strings.Join(cleanParts, "_")

	// Trim leading/trailing dots
	name = strings.Trim(name, ".")

	// Truncate to 200 characters
	if len(name) > 200 {
		name = name[:200]
	}

	if name == "" {
		return "unnamed"
	}

	return name
}

// BuildOutputPath constructs the full output path for a scraped page.
// Creates the directory if it does not exist.
func BuildOutputPath(baseDir, parentPath, folderName, fileName string) string {
	safeFolder := SanitizePath(folderName)
	safeFile := SanitizePath(fileName)

	var dir string
	if parentPath == "" {
		dir = filepath.Join(baseDir, safeFolder)
	} else {
		dir = filepath.Join(parentPath, safeFolder)
	}

	os.MkdirAll(dir, 0755)

	return filepath.Join(dir, safeFile)
}

// NormalizeURL normalizes a URL for deduplication:
// strips fragments, trailing slashes, and common tracking parameters.
func NormalizeURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}

	// Strip fragment
	u.Fragment = ""

	// Strip tracking params
	trackingParams := []string{"utm_source", "utm_medium", "utm_campaign", "utm_term", "utm_content", "ref", "source"}
	q := u.Query()
	for _, param := range trackingParams {
		q.Del(param)
	}
	u.RawQuery = q.Encode()

	result := u.String()

	// Strip trailing slash (but not from root path)
	if strings.HasSuffix(result, "/") && u.Path != "/" && u.Path != "" {
		result = strings.TrimRight(result, "/")
	}

	return result
}
