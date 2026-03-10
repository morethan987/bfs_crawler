package utils

import (
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
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
// - lowercases scheme and host
// - strips fragments
// - strips common tracking parameters
// - sorts query parameters for consistent ordering
// - collapses /index.html and /index.htm to /
// - strips trailing slashes (except root path)
func NormalizeURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}

	// Lowercase scheme and host
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)

	// Strip fragment
	u.Fragment = ""

	// Strip tracking params and sort remaining params
	trackingParams := map[string]struct{}{
		"utm_source":   {},
		"utm_medium":   {},
		"utm_campaign": {},
		"utm_term":     {},
		"utm_content":  {},
		"ref":          {},
		"source":       {},
	}
	q := u.Query()
	for param := range trackingParams {
		q.Del(param)
	}
	// Sort query parameters for consistent ordering
	if len(q) > 0 {
		keys := make([]string, 0, len(q))
		for k := range q {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var parts []string
		for _, k := range keys {
			vals := q[k]
			sort.Strings(vals)
			for _, v := range vals {
				parts = append(parts, url.QueryEscape(k)+"="+url.QueryEscape(v))
			}
		}
		u.RawQuery = strings.Join(parts, "&")
	} else {
		u.RawQuery = ""
	}

	// Clean the path and collapse index files to directory
	u.Path = path.Clean(u.Path)
	base := path.Base(u.Path)
	if base == "index.html" || base == "index.htm" {
		u.Path = path.Dir(u.Path)
		if u.Path != "/" {
			u.Path += "/"
		}
	}

	result := u.String()

	// Strip trailing slash (but not from root path)
	if strings.HasSuffix(result, "/") && u.Path != "/" && u.Path != "" {
		result = strings.TrimRight(result, "/")
	}

	return result
}

// downloadableMIME maps Content-Type prefixes/values to file extensions for
// binary files that should be saved directly instead of parsed as HTML.
var downloadableMIME = map[string]string{
	// Documents
	"application/pdf":                                                          ".pdf",
	"application/msword":                                                       ".doc",
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document":   ".docx",
	"application/vnd.ms-excel":                                                 ".xls",
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":         ".xlsx",
	"application/vnd.ms-powerpoint":                                            ".ppt",
	"application/vnd.openxmlformats-officedocument.presentationml.presentation": ".pptx",
	// Images
	"image/png":  ".png",
	"image/jpeg": ".jpg",
	"image/gif":  ".gif",
	"image/webp": ".webp",
	"image/svg+xml": ".svg",
	"image/bmp":  ".bmp",
	"image/tiff": ".tiff",
	// Archives
	"application/zip":              ".zip",
	"application/gzip":             ".gz",
	"application/x-tar":            ".tar",
	"application/x-rar-compressed": ".rar",
	"application/x-7z-compressed":  ".7z",
	// Media
	"audio/mpeg":  ".mp3",
	"audio/wav":   ".wav",
	"video/mp4":   ".mp4",
	"video/webm":  ".webm",
	// Data
	"application/json": ".json",
	"text/csv":         ".csv",
	"application/xml":  ".xml",
	"text/xml":         ".xml",
}

// ExtensionFromContentType returns a file extension for the given Content-Type.
// Falls back to the URL path extension if the MIME type is not in the known map.
// Returns empty string if no extension can be determined.
func ExtensionFromContentType(contentType string, rawURL string) string {
	// Strip parameters (e.g. "text/html; charset=utf-8" → "text/html")
	mime := strings.ToLower(strings.TrimSpace(contentType))
	if idx := strings.Index(mime, ";"); idx != -1 {
		mime = strings.TrimSpace(mime[:idx])
	}

	if ext, ok := downloadableMIME[mime]; ok {
		return ext
	}

	// Fallback: extract extension from URL path
	if rawURL != "" {
		if u, err := url.Parse(rawURL); err == nil {
			ext := strings.ToLower(path.Ext(u.Path))
			if ext != "" && ext != ".html" && ext != ".htm" {
				return ext
			}
		}
	}

	return ""
}

// IsDownloadableContentType returns true if the Content-Type indicates a file
// that should be downloaded rather than parsed as HTML.
func IsDownloadableContentType(contentType string) bool {
	mime := strings.ToLower(strings.TrimSpace(contentType))
	if idx := strings.Index(mime, ";"); idx != -1 {
		mime = strings.TrimSpace(mime[:idx])
	}
	_, ok := downloadableMIME[mime]
	return ok
}
