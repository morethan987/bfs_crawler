package utils

import (
	"encoding/json"
	"github.com/invopop/jsonschema"
)

// QueueItem represents a URL to be processed in the BFS queue
type QueueItem struct {
	URL        string
	Depth      int
	ParentPath string // filesystem path of parent (e.g., "output/tsinghua/AI")
	FolderName string // LLM-suggested folder name for this item
	FileName   string // LLM-suggested file name for this item
	Reason     string // LLM reason for enqueuing this link (context propagation)
}

// FetchResult holds the HTTP response body and detected content type.
type FetchResult struct {
	Body        []byte
	ContentType string // e.g. "text/html", "application/pdf"
	IsHTML      bool   // true if Content-Type indicates HTML
}

// LinkRef represents an annotated link reference within converted markdown.
// Each link in the content is tagged with an ID (e.g., ⟨L1⟩) and resolved here.
type LinkRef struct {
	ID         string // e.g. "L1", "L2"
	URL        string // normalized absolute URL
	AnchorText string // original anchor text (empty for bare URLs)
}

// LLMResponse is the structured JSON response from the LLM
// CRITICAL: No omitempty tags — breaks strict JSON schema
type LLMResponse struct {
	Summary string     `json:"summary"`
	Links   []LinkItem `json:"links"`
}

type LinkItem struct {
	LinkID         string `json:"link_id"`
	FolderName     string `json:"folder_name"`
	FileName       string `json:"file_name"`
	Reason         string `json:"reason"`
	RelevanceScore int    `json:"relevance_score"`
}

// GenerateSchema generates a JSON schema map from a Go type T.
// Used for structured output with OpenAI-compatible APIs.
func GenerateSchema[T any]() map[string]any {
	reflector := jsonschema.Reflector{
		AllowAdditionalProperties: false,
		DoNotReference:            true,
	}
	var v T
	schema := reflector.Reflect(v)
	data, _ := json.Marshal(schema)
	var result map[string]any
	json.Unmarshal(data, &result)
	return result
}
