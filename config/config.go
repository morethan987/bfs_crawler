package config

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"os"
)

type Config struct {
	LLM    LLMConfig    `yaml:"llm"`
	BFS    BFSConfig    `yaml:"bfs"`
	HTTP   HTTPConfig   `yaml:"http"`
	Prompt PromptConfig `yaml:"prompt"`
	Output OutputConfig `yaml:"output"`
}

type LLMConfig struct {
	BaseURL          string `yaml:"base_url"`
	APIKey           string `yaml:"api_key"`
	Model            string `yaml:"model"`
	MaxContentLength int    `yaml:"max_content_length"`
	JSONMode         string `yaml:"json_mode"`
	MaxRetries       int    `yaml:"max_retries"`
}

type BFSConfig struct {
	MaxDepth       int      `yaml:"max_depth"`
	MaxPages       int      `yaml:"max_pages"`
	Concurrency    int      `yaml:"concurrency"`
	AllowedDomains []string `yaml:"allowed_domains"`
}

type HTTPConfig struct {
	UserAgent  string `yaml:"user_agent"`
	Timeout    int    `yaml:"timeout"`
	Delay      int    `yaml:"delay"`
	MaxRetries int    `yaml:"max_retries"`
}

type PromptConfig struct {
	SystemPrompt       string `yaml:"system_prompt"`
	UserPromptTemplate string `yaml:"user_prompt_template"`
}

type OutputConfig struct {
	BaseDir string `yaml:"base_dir"`
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}
	// Apply defaults
	if cfg.LLM.MaxContentLength == 0 {
		cfg.LLM.MaxContentLength = 32000
	}
	if cfg.LLM.MaxRetries == 0 {
		cfg.LLM.MaxRetries = 3
	}
	if cfg.LLM.JSONMode == "" {
		cfg.LLM.JSONMode = "json_schema"
	}
	if cfg.BFS.MaxPages == 0 {
		cfg.BFS.MaxPages = 100
	}
	if cfg.BFS.Concurrency == 0 {
		cfg.BFS.Concurrency = 3
	}
	if cfg.HTTP.Timeout == 0 {
		cfg.HTTP.Timeout = 30
	}
	if cfg.HTTP.Delay == 0 {
		cfg.HTTP.Delay = 1000
	}
	if cfg.HTTP.MaxRetries == 0 {
		cfg.HTTP.MaxRetries = 3
	}
	if cfg.Output.BaseDir == "" {
		cfg.Output.BaseDir = "output"
	}
	// Validate required fields
	if cfg.LLM.BaseURL == "" {
		return nil, fmt.Errorf("config validation error: llm.base_url is required")
	}
	if cfg.LLM.APIKey == "" {
		return nil, fmt.Errorf("config validation error: llm.api_key is required")
	}
	if cfg.LLM.Model == "" {
		return nil, fmt.Errorf("config validation error: llm.model is required")
	}
	if cfg.Prompt.SystemPrompt == "" {
		return nil, fmt.Errorf("config validation error: prompt.system_prompt is required")
	}
	if cfg.Prompt.UserPromptTemplate == "" {
		return nil, fmt.Errorf("config validation error: prompt.user_prompt_template is required")
	}
	return &cfg, nil
}
