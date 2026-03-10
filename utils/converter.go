package utils

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/JohannesKaufmann/html-to-markdown/v2/converter"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/base"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/commonmark"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/table"
)

// Converter handles HTML to Markdown conversion.
type Converter struct {
	conv *converter.Converter
}

// NewConverter creates a new Converter with base, commonmark, and table plugins.
func NewConverter() *Converter {
	conv := converter.NewConverter(
		converter.WithPlugins(
			base.NewBasePlugin(),
			commonmark.NewCommonmarkPlugin(),
			table.NewTablePlugin(),
		),
	)
	return &Converter{conv: conv}
}

// Convert converts HTML to Markdown, resolving relative URLs against sourceURL.
func (c *Converter) Convert(htmlBody string, sourceURL string) (string, error) {
	md, err := c.conv.ConvertString(htmlBody, converter.WithDomain(sourceURL))
	if err != nil {
		return "", fmt.Errorf("HTML to Markdown conversion failed: %w", err)
	}
	return md, nil
}

// TruncateContent truncates markdown to maxLength characters at a line boundary.
// Appends a truncation notice if content was cut.
func (c *Converter) TruncateContent(markdown string, maxLength int) string {
	if len(markdown) <= maxLength {
		return markdown
	}
	// Find last newline before maxLength
	truncated := markdown[:maxLength]
	lastNewline := strings.LastIndex(truncated, "\n")
	if lastNewline > 0 {
		truncated = truncated[:lastNewline]
	}
	return truncated + "\n\n[Content truncated...]"
}

// mdLinkRe matches markdown links: [text](url)
var mdLinkRe = regexp.MustCompile(`\[([^\]]*)\]\(([^)]+)\)`)

// bareURLRe matches bare http(s) URLs not already inside markdown link parentheses.
// Negative lookbehind for '(' ensures we don't match URLs already in [text](url).
var bareURLRe = regexp.MustCompile(`(?:^|[^(])\b(https?://[^\s)\]>]+)`)

// AnnotateLinks scans converted markdown for links (both markdown-syntax and bare URLs),
// tags each with an inline marker like ⟨L1⟩, and returns the annotated markdown plus a
// reference map from link ID to LinkRef. Static asset URLs are excluded.
func (c *Converter) AnnotateLinks(markdown string) (string, []LinkRef) {
	var refs []LinkRef
	seen := make(map[string]struct{})
	counter := 0

	// Phase 1: Annotate markdown-syntax links [text](url) → [text](url) ⟨L1⟩
	annotated := mdLinkRe.ReplaceAllStringFunc(markdown, func(match string) string {
		subs := mdLinkRe.FindStringSubmatch(match)
		if len(subs) < 3 {
			return match
		}
		anchorText := subs[1]
		rawURL := subs[2]

		if !isHTTPURL(rawURL) || isStaticAsset(rawURL) {
			return match
		}

		normalized := NormalizeURL(rawURL)
		if normalized == "" {
			return match
		}

		if _, exists := seen[normalized]; exists {
			// Find existing ref ID for dedup
			for _, ref := range refs {
				if ref.URL == normalized {
					return fmt.Sprintf("%s ⟨%s⟩", match, ref.ID)
				}
			}
			return match
		}

		counter++
		id := fmt.Sprintf("L%d", counter)
		seen[normalized] = struct{}{}
		refs = append(refs, LinkRef{
			ID:         id,
			URL:        normalized,
			AnchorText: anchorText,
		})
		return fmt.Sprintf("%s ⟨%s⟩", match, id)
	})

	// Phase 2: Annotate bare URLs not already inside markdown link syntax
	annotated = bareURLRe.ReplaceAllStringFunc(annotated, func(match string) string {
		// Extract the URL part (group 1)
		subs := bareURLRe.FindStringSubmatch(match)
		if len(subs) < 2 {
			return match
		}
		rawURL := subs[1]

		if isStaticAsset(rawURL) {
			return match
		}

		// Skip if already annotated (has ⟨ immediately after)
		if strings.Contains(match, "⟨") {
			return match
		}

		normalized := NormalizeURL(rawURL)
		if normalized == "" {
			return match
		}

		if _, exists := seen[normalized]; exists {
			for _, ref := range refs {
				if ref.URL == normalized {
					return fmt.Sprintf("%s ⟨%s⟩", match, ref.ID)
				}
			}
			return match
		}

		counter++
		id := fmt.Sprintf("L%d", counter)
		seen[normalized] = struct{}{}
		refs = append(refs, LinkRef{
			ID:         id,
			URL:        normalized,
			AnchorText: "",
		})
		return fmt.Sprintf("%s ⟨%s⟩", match, id)
	})

	return annotated, refs
}

func isHTTPURL(rawURL string) bool {
	return strings.HasPrefix(rawURL, "http://") || strings.HasPrefix(rawURL, "https://")
}

func isStaticAsset(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	lowerPath := strings.ToLower(parsed.Path)
	for ext := range skipExtensions {
		if strings.HasSuffix(lowerPath, ext) {
			return true
		}
	}
	return false
}
