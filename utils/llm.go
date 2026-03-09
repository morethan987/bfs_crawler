package utils

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"text/template"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

type LLMClient struct {
	client       *openai.Client
	model        string
	jsonMode     string
	maxRetries   int
	systemPrompt string
	userTmpl     *template.Template
	schema       map[string]any
}

func NewLLMClient(baseURL, apiKey, model, jsonMode string, maxRetries int, systemPrompt, userPromptTemplate string) (*LLMClient, error) {
	client := openai.NewClient(
		option.WithBaseURL(baseURL),
		option.WithAPIKey(apiKey),
	)

	tmpl, err := template.New("user_prompt").Parse(userPromptTemplate)
	if err != nil {
		return nil, fmt.Errorf("failed to parse user prompt template: %w", err)
	}

	schema := GenerateSchema[LLMResponse]()

	return &LLMClient{
		client:       &client,
		model:        model,
		jsonMode:     jsonMode,
		maxRetries:   maxRetries,
		systemPrompt: systemPrompt,
		userTmpl:     tmpl,
		schema:       schema,
	}, nil
}

type templateData struct {
	URL     string
	Depth   int
	Content string
	Links   string
}

func (c *LLMClient) Analyze(ctx context.Context, pageURL string, depth int, markdown string, links []string) (*LLMResponse, error) {
	data := templateData{
		URL:     pageURL,
		Depth:   depth,
		Content: markdown,
		Links:   strings.Join(links, "\n"),
	}
	var buf bytes.Buffer
	if err := c.userTmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("failed to render user prompt template: %w", err)
	}
	userPrompt := buf.String()

	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		resp, err := c.callLLM(ctx, userPrompt)
		if err != nil {
			lastErr = err
			continue
		}
		return resp, nil
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("unknown error")
	}

	return nil, fmt.Errorf("LLM call failed after %d retries: %w", c.maxRetries, lastErr)
}

func (c *LLMClient) callLLM(ctx context.Context, userPrompt string) (*LLMResponse, error) {
	responseFormat := openai.ChatCompletionNewParamsResponseFormatUnion{}
	switch c.jsonMode {
	case "json_schema":
		responseFormat = openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONSchema: &openai.ResponseFormatJSONSchemaParam{
				JSONSchema: openai.ResponseFormatJSONSchemaJSONSchemaParam{
					Name:   "llm_response",
					Schema: c.schema,
					Strict: openai.Bool(true),
				},
			},
		}
	case "json_object":
		responseFormat = openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONObject: &openai.ResponseFormatJSONObjectParam{},
		}
	default:
		return nil, fmt.Errorf("unsupported json mode: %s", c.jsonMode)
	}

	resp, err := c.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(c.systemPrompt),
			openai.UserMessage(userPrompt),
		},
		Model:          openai.ChatModel(c.model),
		ResponseFormat: responseFormat,
	})
	if err != nil {
		return nil, fmt.Errorf("chat completion failed: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("chat completion returned no choices")
	}

	content := resp.Choices[0].Message.Content
	if strings.TrimSpace(content) == "" {
		return nil, fmt.Errorf("chat completion returned empty content")
	}

	var llmResp LLMResponse
	if err := json.Unmarshal([]byte(content), &llmResp); err != nil {
		return nil, fmt.Errorf("failed to parse LLM JSON response: %w", err)
	}

	return &llmResp, nil
}
