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
}

// LLMResponse is the structured JSON response from the LLM
// CRITICAL: No omitempty tags — breaks strict JSON schema
type LLMResponse struct {
	Summary string     `json:"summary"`
	Links   []LinkItem `json:"links"`
}

type LinkItem struct {
	URL        string `json:"url"`
	FolderName string `json:"folder_name"`
	FileName   string `json:"file_name"`
	Reason     string `json:"reason"`
}

// PageResult holds the processing result of a single page
type PageResult struct {
	URL      string
	Markdown string
	Links    []string // extracted <a href> links from HTML
	LLMResp  *LLMResponse
	SavePath string
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
