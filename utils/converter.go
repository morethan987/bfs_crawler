package utils

import (
	"fmt"
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
