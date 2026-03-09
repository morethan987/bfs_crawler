# AGENTS.md — BFS LLM Scraper

Coding agent reference for the `github.com/morethan/bfs_scraper` project.
Go 1.26.1 · No test suite · No Makefile · No CI config.

---

## Project Layout

```
bfs_scraper/
├── cmd/main.go          # CLI entry point (package main)
├── config/config.go     # Config structs + LoadConfig (package config)
├── utils/
│   ├── types.go         # Shared types + GenerateSchema[T]
│   ├── sanitize.go      # SanitizePath, BuildOutputPath, NormalizeURL
│   ├── fetcher.go       # HTTP fetching + HTML link extraction
│   ├── converter.go     # HTML→Markdown conversion
│   ├── llm.go           # OpenAI-compatible LLM client
│   └── crawler.go       # BFS engine (concurrent workers)
├── config.example.yaml  # Fully documented config template
├── go.mod               # module github.com/morethan/bfs_scraper
└── go.sum
```

**Package rule**: all utility code lives in `package utils`. Config lives in `package config`. Only `cmd/main.go` is `package main`.

---

## Build & Run Commands

```bash
# Build
go build ./...

# Build binary
go build -o bfs_scraper ./cmd/...

# Vet (run after every change)
go vet ./...

# Race detector build (confirm no data races)
go build -race ./...

# Run
./bfs_scraper -config config.yaml https://example.com

# Add a dependency
go get github.com/some/pkg
go mod tidy
```

**No test suite.** There are no `*_test.go` files. Verification is done by building and running the binary manually.

---

## Code Style

### Imports — Two-Group Format

Always two import groups separated by a blank line: stdlib first, then third-party. Never three groups.

```go
import (
    "context"
    "fmt"
    "net/http"
    "strings"

    "github.com/openai/openai-go/v3"
    "github.com/openai/openai-go/v3/option"
)
```

### Struct Definitions

- No pointer fields in config structs (plain value types only).
- Unexported fields in implementation structs.
- One blank line between struct definitions.
- Exported structs get a doc comment on the line immediately above.

```go
// Fetcher handles HTTP fetching with retry and link extraction.
type Fetcher struct {
    client     *http.Client
    userAgent  string
    delay      time.Duration
    maxRetries int
}
```

### Struct Tags

- Config structs use `yaml:"snake_case"` tags only.
- JSON-serialized structs (LLM types) use `json:"snake_case"` tags only.
- **CRITICAL: Never add `omitempty` to `LLMResponse` or `LinkItem` JSON tags.** It breaks strict JSON schema mode with OpenAI.

```go
// Correct — no omitempty
type LLMResponse struct {
    Summary string     `json:"summary"`
    Links   []LinkItem `json:"links"`
}
```

### Naming Conventions

- **Constructors**: `NewXxx(params) *Xxx` or `NewXxx(params) (*Xxx, error)`.
- **Methods**: verb-first, e.g., `Fetch`, `Convert`, `Analyze`, `ExtractLinks`.
- **Config fields**: PascalCase in Go, `snake_case` in YAML tags.
- **Local variables**: short and clear — `cfg`, `conv`, `llmResp`, `htmlBody`, `rawURL`.
- **Unexported helpers**: lowercase, e.g., `doFetch`, `callLLM`.
- **Template data structs**: unexported, e.g., `templateData`.

### Error Handling

Always wrap errors with `fmt.Errorf("context: %w", err)`. Never discard errors silently.

```go
// Constructor errors — return (value, error)
func NewLLMClient(...) (*LLMClient, error) {
    tmpl, err := template.New("user_prompt").Parse(userPromptTemplate)
    if err != nil {
        return nil, fmt.Errorf("failed to parse user prompt template: %w", err)
    }
    ...
}

// Fatal at startup only (cmd/main.go)
cfg, err := config.LoadConfig(*configPath)
if err != nil {
    log.Fatalf("Failed to load config: %v", err)
}

// Warn and continue for non-fatal runtime errors (crawler/worker)
if err != nil {
    log.Printf("[WARN] LLM analysis failed for %s: %v", item.URL, err)
    return nil
}
```

**Pattern**: `log.Fatalf` only in `cmd/main.go` for initialization. All runtime errors in `utils/` either return the error or `log.Printf("[WARN] ...")` and continue.

### Logging

Use stdlib `log` package directly — no structured logging framework.

```go
log.Printf("[Processing] depth=%d %s", item.Depth, item.URL)
log.Printf("[Saved] %s", mdPath)
log.Printf("[SKIP] max depth reached for %s", item.URL)
log.Printf("[WARN] link extraction failed for %s: %v", item.URL, err)
log.Printf("[ERROR] processing %s: %v", item.URL, err)
log.Fatalf("Failed to load config: %v", err)   // cmd/main.go only
```

Log level prefixes: `[Processing]`, `[Saved]`, `[SKIP]`, `[WARN]`, `[ERROR]`.

### Context Usage

- `context.Context` is the **first parameter** on any function that does I/O or calls external services.
- Always pass context through — never use `context.Background()` inside utility functions.
- Signal-aware cancellation lives only in `cmd/main.go` via `signal.NotifyContext`.

```go
func (c *LLMClient) Analyze(ctx context.Context, ...) (*LLMResponse, error) { ... }
func (c *Crawler) Run(ctx context.Context, seedURLs []string) error { ... }
```

### Concurrency

- Use `sync.Mutex` for protecting shared state (`queue`, `visited`).
- Use `sync/atomic` for counters accessed by multiple goroutines (`pageCount int32`).
- Use `sync.WaitGroup` to track in-flight BFS items.
- Worker loops check `ctx.Done()` and a `stop` channel at the top of every iteration.

---

## Key Architectural Rules

### LLM Client (CRITICAL)

- **MUST use** `client.Chat.Completions.New()` — NOT `client.Responses.New()` (OpenAI-proprietary, not in openai-go/v3).
- `openai.NewClient(...)` returns a **value type** — store as `&client` in a pointer field.
- `Messages`, `Model`, `ResponseFormat` are **direct value fields**, not wrapped with `openai.F(...)`.
- `json_schema` mode: use `openai.Bool(true)` for the `Strict` field (type `param.Opt[bool]`).
- `json_object` mode: for Ollama/other providers that don't support strict JSON schema.

### Path Sanitization

- `SanitizePath` handles a **single path component** only — no directory separators.
- `BuildOutputPath` returns path **without `.md` extension** — callers must append `.md`.
- `BuildOutputPath` calls `os.MkdirAll` internally as a side effect.
- Seed items use `ParentPath: cfg.Output.BaseDir`, `FolderName: "root"`, `FileName: "index"`.
- Child `ParentPath`: `filepath.Join(item.ParentPath, SanitizePath(item.FolderName))` — do NOT prepend `baseDir` again.

### Config Defaults

Applied in `LoadConfig` when zero/empty — do not re-apply in constructors:
- `llm.max_content_length`: 32000
- `llm.max_retries`: 3
- `llm.json_mode`: `"json_schema"`
- `bfs.max_pages`: 100
- `bfs.concurrency`: 3
- `http.timeout`: 30 (seconds)
- `http.delay`: 1000 (milliseconds)
- `http.max_retries`: 3
- `output.base_dir`: `"output"`

### JSON Schema Generation

`GenerateSchema[T]()` in `utils/types.go` uses `invopop/jsonschema` with:
- `AllowAdditionalProperties: false`
- `DoNotReference: true`

Used for `json_schema` response format mode with OpenAI-compatible APIs.

---

## Dependencies

| Package | Purpose |
|---|---|
| `gopkg.in/yaml.v3` | YAML config parsing |
| `github.com/openai/openai-go/v3` | OpenAI-compatible LLM API |
| `github.com/JohannesKaufmann/html-to-markdown/v2` | HTML→Markdown conversion |
| `github.com/invopop/jsonschema` | JSON schema generation from Go types |
| `golang.org/x/net` | HTML tokenizer for link extraction |

---

## What NOT to Add

- No unit tests (`*_test.go` files)
- No JavaScript rendering (Playwright, Chrome, etc.)
- No proxy or authentication support
- No resume/checkpoint system
- No database or persistent storage
- No structured logging framework (zerolog, zap, slog)
- No exponential backoff (linear retry only)
- No `omitempty` on `LLMResponse`/`LinkItem` JSON fields
- No `client.Responses.New()` calls
