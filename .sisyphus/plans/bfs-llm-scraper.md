# BFS LLM-Driven Web Scraper (Go)

## TL;DR

> **Quick Summary**: Build a Go CLI tool that performs BFS web crawling driven by LLM decisions — fetching HTML, converting to Markdown, and using an LLM (via OpenAI-compatible API) to decide which links to follow, generate folder/file names, and write one-line summaries. Configurable via YAML for reuse across different scraping purposes.
> 
> **Deliverables**:
> - Go CLI binary `bfs_scraper` that accepts `-config config.yaml <seed_urls...>`
> - YAML config system covering LLM, BFS, HTTP, and prompt settings
> - Hierarchical markdown output with `index.md` summaries per page
> - Concurrent BFS engine with configurable depth, max pages, and worker count
> 
> **Estimated Effort**: Medium
> **Parallel Execution**: YES — 4 waves
> **Critical Path**: Task 1 → Task 2 → Task 5 → Task 7 → Task 8

---

## Context

### Original Request
Build a BFS web scraper in Go driven by LLM decisions. The tool should:
1. Crawl web pages using BFS (breadth-first search) traversal
2. Fetch full HTML → convert to clean Markdown → save to disk
3. Feed Markdown to LLM which decides what links to enqueue (with rules from prompt config)
4. LLM also generates folder/file names and index.md summaries
5. Continue until queue is empty or limits reached
6. Be configurable via YAML for different use cases (teacher info, research papers, etc.)

Target output structure example:
```
tsinghua/
└── AI/
    ├── chenyongchao/
    │   ├── index.md          (one-line summary)
    │   ├── 陈勇超.md
    │   └── Yongchao-Chen.md
    ├── lijia/
    │   ├── index.md
    │   ├── 李佳.md
    │   └── Jia-Lis-Homepage.md
    ...
```

### Interview Summary
**Key Discussions**:
- **LLM Provider**: OpenAI-compatible interface — `option.WithBaseURL()` supports Ollama, DeepSeek, Azure, etc.
- **BFS Strategy**: Configurable max_depth + concurrent goroutine workers + max_pages safety cap
- **Naming**: LLM generates folder names (english/pinyin) and file names (chinese/original title)
- **Prompt Config**: Full system_prompt + user_prompt_template in YAML, with Go `text/template` placeholders
- **LLM Output**: JSON Schema structured output via `invopop/jsonschema` — forced structured response with link list, names, summary
- **No unit tests** — Agent-executed QA scenarios only

**Research Findings**:
- `JohannesKaufmann/html-to-markdown` v2 — used by Hugo (gohugoio), PandaWiki; `converter.WithDomain()` for relative URL resolution
- `openai/openai-go` v3 — official library; `Chat.Completions.New()` for OpenAI-compatible providers; `Responses.New()` is OpenAI-proprietary only
- `invopop/jsonschema` — `GenerateSchema[T]()` pattern for structured output schema generation
- BFS in Go: slice-based queue + `map[string]struct{}` visited set + `sync.WaitGroup` for concurrent termination detection

### Metis Review
**Identified Gaps** (addressed):
- **MUST use Chat.Completions API, NOT Responses API** — Responses API is OpenAI-proprietary, won't work with Ollama/DeepSeek
- **Structured output fallback**: Not all providers support `Strict: true` JSON schema — add `json_mode` config option (`"json_schema"` vs `"json_object"`)
- **Link extraction strategy**: Extract all `<a href>` links from HTML into structured list for LLM, rather than relying on LLM to parse markdown links
- **BFS queue item must carry metadata**: `{URL, Depth, ParentPath, FolderName, FileName}` — not just URL+depth
- **Safety caps**: Need both `max_depth` AND `max_pages` AND `allowed_domains` whitelist
- **Path sanitization**: All LLM-generated folder/file names must be sanitized (strip `../`, illegal chars, truncate)
- **Content truncation**: `max_content_length` config to prevent sending 50K+ token pages to LLM
- **BFS termination with concurrency**: `sync.WaitGroup` pattern — `wg.Add(1)` on enqueue, `wg.Done()` on full processing
- **Separate rate limits**: HTTP fetch delay vs LLM API delay
- **URL normalization**: Strip fragments, trailing slashes, tracking params
- **HTTP client config**: `user_agent` + `http_timeout` in config
- **No `omitempty`** in LLM response schema struct tags — breaks strict JSON schema

---

## Work Objectives

### Core Objective
Build a reusable Go CLI tool that performs BFS web crawling where an LLM (via OpenAI-compatible API) drives the link-following decisions, naming, and summarization — producing organized markdown files in a hierarchical folder structure.

### Concrete Deliverables
- `cmd/main.go` — CLI entry point with flag parsing
- `config/config.go` — YAML config loading and validation
- `utils/fetcher.go` — HTTP client with configurable User-Agent, timeout, retry
- `utils/converter.go` — HTML to Markdown conversion with link extraction
- `utils/llm.go` — OpenAI-compatible LLM client with structured JSON output
- `utils/crawler.go` — BFS engine orchestrating fetch→convert→LLM→save→enqueue loop
- `utils/sanitize.go` — Path sanitization for LLM-generated names
- `config.example.yaml` — Example config file with all options documented
- `go.mod` / `go.sum` — Go module files

### Definition of Done
- [ ] `go build -o bfs_scraper ./cmd` compiles without errors
- [ ] `go vet ./...` passes with no issues
- [ ] Running with a valid config and seed URL produces markdown files in output directory
- [ ] `index.md` files contain one-line summaries
- [ ] BFS respects `max_depth` and `max_pages` limits
- [ ] LLM-generated folder/file names are properly sanitized
- [ ] Concurrent workers process pages in parallel without race conditions (`go build -race`)
- [ ] Invalid config or unreachable URLs produce clear error messages, not panics

### Must Have
- BFS queue-based traversal with visited URL tracking
- HTML → Markdown conversion with relative URL resolution
- LLM-driven link selection via structured JSON output
- LLM-generated folder names (english/pinyin) and file names
- `index.md` per-page with one-line LLM-generated summary
- Configurable: `max_depth`, `max_pages`, `concurrency`, `delay`, `allowed_domains`
- YAML config with full `system_prompt` and `user_prompt_template`
- OpenAI-compatible LLM interface (base_url + api_key + model)

### Must NOT Have (Guardrails)
- **NO JavaScript rendering** — `net/http` only, no headless browser
- **NO Responses API** — MUST use `client.Chat.Completions.New()`, NOT `client.Responses.New()`
- **NO `omitempty` in LLM response schema struct tags** — breaks strict JSON schema
- **NO proxy support** — out of scope for v1
- **NO resume/checkpoint** — in-memory state only, no persistent state
- **NO database** — files and in-memory maps only
- **NO structured logging framework** — use `log.Printf` / `fmt.Fprintf(os.Stderr, ...)`
- **NO exponential backoff** — simple fixed-delay retry with configurable count
- **NO custom HTML preprocessing** — use html-to-markdown defaults + plugins
- **NO link filtering regex or CSS selectors** — LLM decides what to follow
- **NO progress bars or web dashboards** — log each URL as processed
- **NO over-abstracted interfaces** — keep it concrete; this is a CLI tool, not a framework

---

## Verification Strategy

> **ZERO HUMAN INTERVENTION** — ALL verification is agent-executed. No exceptions.
> Acceptance criteria requiring "user manually tests/confirms" are FORBIDDEN.

### Test Decision
- **Infrastructure exists**: NO (greenfield project)
- **Automated tests**: None — QA scenarios only
- **Framework**: N/A

### QA Policy
Every task MUST include agent-executed QA scenarios.
Evidence saved to `.sisyphus/evidence/task-{N}-{scenario-slug}.{ext}`.

- **CLI**: Use Bash — Run `go build`, `go vet`, execute binary, check output files
- **API/Integration**: Use Bash (curl) — Test with mock/real endpoints if available
- **File output**: Use Bash — `ls`, `cat`, `wc`, `grep` to verify file existence and content

---

## Execution Strategy

### Parallel Execution Waves

```
Wave 1 (Start Immediately — foundation + types):
├── Task 1: Go module init + config struct + YAML loading [quick]
├── Task 2: Type definitions (BFS queue item, LLM response schema) [quick]
└── Task 3: Path sanitization utility [quick]

Wave 2 (After Wave 1 — I/O modules, MAX PARALLEL):
├── Task 4: HTTP fetcher with retry + link extraction [unspecified-high]
├── Task 5: HTML-to-Markdown converter [quick]
└── Task 6: LLM client with structured JSON output [deep]

Wave 3 (After Wave 2 — core engine + CLI):
├── Task 7: BFS crawler engine [deep]
└── Task 8: CLI entry point + example config [quick]

Wave FINAL (After ALL tasks — verification):
├── Task F1: Plan compliance audit [oracle]
├── Task F2: Code quality review [unspecified-high]
├── Task F3: Real QA — build & run [unspecified-high]
└── Task F4: Scope fidelity check [deep]

Critical Path: Task 1 → Task 2 → Task 6 → Task 7 → Task 8
Parallel Speedup: ~50% faster than sequential
Max Concurrent: 3 (Waves 1 & 2)
```

### Dependency Matrix

| Task | Depends On | Blocks | Wave |
|------|-----------|--------|------|
| 1 | — | 2, 3, 4, 5, 6 | 1 |
| 2 | 1 | 6, 7 | 1 |
| 3 | 1 | 7 | 1 |
| 4 | 1 | 7 | 2 |
| 5 | 1 | 7 | 2 |
| 6 | 1, 2 | 7 | 2 |
| 7 | 2, 3, 4, 5, 6 | 8 | 3 |
| 8 | 7 | F1-F4 | 3 |
| F1-F4 | 8 | — | FINAL |

### Agent Dispatch Summary

- **Wave 1**: **3** — T1 → `quick`, T2 → `quick`, T3 → `quick`
- **Wave 2**: **3** — T4 → `unspecified-high`, T5 → `quick`, T6 → `deep`
- **Wave 3**: **2** — T7 → `deep`, T8 → `quick`
- **Wave FINAL**: **4** — F1 → `oracle`, F2 → `unspecified-high`, F3 → `unspecified-high`, F4 → `deep`

---

## TODOs

- [ ] 1. Go Module Init + Config Struct + YAML Loading

  **What to do**:
  - Run `go mod init github.com/morethan/bfs_scraper`
  - Create `config/config.go` with the complete YAML config struct:
    ```go
    type Config struct {
        LLM     LLMConfig     `yaml:"llm"`
        BFS     BFSConfig     `yaml:"bfs"`
        HTTP    HTTPConfig    `yaml:"http"`
        Prompt  PromptConfig  `yaml:"prompt"`
        Output  OutputConfig  `yaml:"output"`
    }
    type LLMConfig struct {
        BaseURL          string `yaml:"base_url"`     // e.g. https://api.openai.com/v1
        APIKey           string `yaml:"api_key"`
        Model            string `yaml:"model"`         // e.g. gpt-4o
        MaxContentLength int    `yaml:"max_content_length"` // chars to truncate before LLM, default 32000
        JSONMode         string `yaml:"json_mode"`    // "json_schema" or "json_object"
        MaxRetries       int    `yaml:"max_retries"`  // LLM call retries, default 3
    }
    type BFSConfig struct {
        MaxDepth       int      `yaml:"max_depth"`       // 0 = seed only
        MaxPages       int      `yaml:"max_pages"`       // total page cap, default 100
        Concurrency    int      `yaml:"concurrency"`     // worker count, default 3
        AllowedDomains []string `yaml:"allowed_domains"` // domain whitelist
    }
    type HTTPConfig struct {
        UserAgent  string `yaml:"user_agent"`
        Timeout    int    `yaml:"timeout"`     // seconds, default 30
        Delay      int    `yaml:"delay"`       // ms between requests, default 1000
        MaxRetries int    `yaml:"max_retries"` // HTTP retries, default 3
    }
    type PromptConfig struct {
        SystemPrompt      string `yaml:"system_prompt"`
        UserPromptTemplate string `yaml:"user_prompt_template"` // Go text/template with {{.URL}}, {{.Depth}}, {{.Content}}, {{.Links}}
    }
    type OutputConfig struct {
        BaseDir string `yaml:"base_dir"` // output root directory, default "output"
    }
    ```
  - Implement `LoadConfig(path string) (*Config, error)` — read YAML file, unmarshal, apply defaults for missing optional fields, validate required fields (base_url, api_key, model, system_prompt, user_prompt_template must be non-empty)
  - Install dependency: `go get gopkg.in/yaml.v3`

  **Must NOT do**:
  - No env var overrides — YAML only
  - No JSON Schema validation of the config file itself
  - No config hot-reloading

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Straightforward struct definitions + YAML parsing, no complex logic
  - **Skills**: []
  - **Skills Evaluated but Omitted**:
    - None relevant

  **Parallelization**:
  - **Can Run In Parallel**: YES (Wave 1)
  - **Parallel Group**: Wave 1 (with Tasks 2, 3)
  - **Blocks**: Tasks 2, 3, 4, 5, 6
  - **Blocked By**: None (can start immediately)

  **References**:
  - **Pattern References**: None (greenfield project)
  - **External References**:
    - `gopkg.in/yaml.v3` — standard Go YAML library
    - Config struct conventions: follow `yaml:"field_name"` tag pattern
  - **WHY**: Config is the foundation — all other modules depend on it for settings

  **Acceptance Criteria**:
  - [ ] `go build ./...` compiles successfully
  - [ ] `config/config.go` contains all struct types listed above
  - [ ] `LoadConfig("config.example.yaml")` returns populated Config struct
  - [ ] Missing `base_url` in config file → returns error with clear message

  **QA Scenarios:**
  ```
  Scenario: Valid config loading
    Tool: Bash
    Preconditions: config.example.yaml exists with all required fields
    Steps:
      1. go build ./...
      2. Write a small Go test-main that calls LoadConfig("config.example.yaml") and prints result
    Expected Result: All fields populated, no error
    Evidence: .sisyphus/evidence/task-1-valid-config.txt

  Scenario: Missing required field
    Tool: Bash
    Preconditions: Create a config missing api_key field
    Steps:
      1. Call LoadConfig with incomplete config
    Expected Result: Error message containing "api_key" or "required"
    Evidence: .sisyphus/evidence/task-1-missing-field.txt
  ```

  **Commit**: YES (group with Tasks 2, 3 — Wave 1)
  - Message: `feat(init): scaffold project with config, types, and path sanitization`
  - Files: `go.mod`, `go.sum`, `config/config.go`
  - Pre-commit: `go build ./...`

- [ ] 2. Type Definitions (BFS Queue Item, LLM Response Schema)

  **What to do**:
  - Create `utils/types.go` with:
    ```go
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
        Summary  string      `json:"summary"`  // one-line summary for index.md
        Links    []LinkItem  `json:"links"`    // links to enqueue
    }

    type LinkItem struct {
        URL        string `json:"url"`         // absolute URL to enqueue
        FolderName string `json:"folder_name"` // suggested folder name (english/pinyin)
        FileName   string `json:"file_name"`   // suggested file name (chinese/original)
        Reason     string `json:"reason"`      // why this link is worth following
    }

    // PageResult holds the processing result of a single page
    type PageResult struct {
        URL       string
        Markdown  string
        Links     []string // extracted <a href> links from HTML
        LLMResp   *LLMResponse
        SavePath  string
    }
    ```
  - CRITICAL: Do NOT use `omitempty` in any JSON tag on LLMResponse or LinkItem structs
  - Add the `GenerateSchema[T]()` helper function in this file:
    ```go
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
    ```
  - Install dependency: `go get github.com/invopop/jsonschema`

  **Must NOT do**:
  - No `omitempty` in LLMResponse/LinkItem JSON tags
  - No complex inheritance or embedding — flat structs

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Pure type definitions + one utility function
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES (Wave 1)
  - **Parallel Group**: Wave 1 (with Tasks 1, 3)
  - **Blocks**: Tasks 6, 7
  - **Blocked By**: Task 1 (needs go.mod)

  **References**:
  - **External References**:
    - `github.com/invopop/jsonschema` — JSON schema generation from Go structs
    - openai-go structured output pattern: define struct → GenerateSchema → use in ResponseFormat
  - **WHY**: LLMResponse struct shape drives the entire LLM interaction and downstream data flow

  **Acceptance Criteria**:
  - [ ] `go build ./...` compiles
  - [ ] `utils/types.go` contains QueueItem, LLMResponse, LinkItem, PageResult structs
  - [ ] GenerateSchema[LLMResponse]() returns valid map with "properties" key
  - [ ] No `omitempty` in any JSON tag (verify with `grep omitempty utils/types.go` → 0 matches)

  **QA Scenarios:**
  ```
  Scenario: Schema generation produces valid JSON
    Tool: Bash
    Preconditions: utils/types.go exists with GenerateSchema function
    Steps:
      1. Write small Go main that calls GenerateSchema[LLMResponse]() and prints JSON
      2. Verify output contains "summary", "links" as properties
    Expected Result: Valid JSON schema with all fields present, no omitempty artifacts
    Evidence: .sisyphus/evidence/task-2-schema.json

  Scenario: No omitempty in schema structs
    Tool: Bash
    Steps:
      1. grep -c 'omitempty' utils/types.go
    Expected Result: Output is 0
    Evidence: .sisyphus/evidence/task-2-no-omitempty.txt
  ```

  **Commit**: YES (group with Tasks 1, 3 — Wave 1)
  - Message: `feat(init): scaffold project with config, types, and path sanitization`
  - Files: `utils/types.go`
  - Pre-commit: `go build ./...`

- [ ] 3. Path Sanitization Utility

  **What to do**:
  - Create `utils/sanitize.go` with:
    - `SanitizePath(name string) string` — sanitize a single path component (folder or file name):
      - Strip `..` and `.` path traversal
      - Replace OS-illegal characters (`\/:*?"<>|`) with `_`
      - Trim leading/trailing whitespace and dots
      - Truncate to 200 characters (leave room for extension)
      - If result is empty after sanitization, return `"unnamed"`
    - `BuildOutputPath(baseDir, parentPath, folderName, fileName string) string` — construct full output path:
      - Join `baseDir/parentPath/SanitizePath(folderName)/SanitizePath(fileName)`
      - Ensure directory exists (`os.MkdirAll`)
    - `NormalizeURL(rawURL string) string` — URL normalization:
      - Parse with `url.Parse()`
      - Strip fragment (`#section`)
      - Strip trailing slash
      - Strip common tracking params (`utm_*`, `ref`, `source`)
      - Return canonical URL string

  **Must NOT do**:
  - No complex URL deduplication beyond normalization
  - No custom encoding/escaping schemes

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Pure utility functions with string manipulation
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES (Wave 1)
  - **Parallel Group**: Wave 1 (with Tasks 1, 2)
  - **Blocks**: Task 7
  - **Blocked By**: Task 1 (needs go.mod)

  **References**:
  - **External References**:
    - Go `path/filepath`, `net/url`, `strings`, `unicode` standard libraries
  - **WHY**: LLM-generated names are untrusted input — sanitization prevents path traversal and filesystem errors

  **Acceptance Criteria**:
  - [ ] `SanitizePath("../../../etc/passwd")` returns `"etcpasswd"` or similar safe string
  - [ ] `SanitizePath("")` returns `"unnamed"`
  - [ ] `SanitizePath("valid-name")` returns `"valid-name"` unchanged
  - [ ] `NormalizeURL("https://example.com/page#section")` strips fragment
  - [ ] `NormalizeURL("https://example.com/page/")` strips trailing slash

  **QA Scenarios:**
  ```
  Scenario: Path traversal sanitization
    Tool: Bash
    Steps:
      1. Write Go main calling SanitizePath with malicious inputs: "../../../etc/passwd", "", "a]b[c", string of 500 chars
      2. Print results
    Expected Result: No ".." in any output, empty input returns "unnamed", long string truncated to <=200 chars
    Evidence: .sisyphus/evidence/task-3-sanitize-paths.txt

  Scenario: URL normalization
    Tool: Bash
    Steps:
      1. Write Go main calling NormalizeURL with: "https://example.com/page#section", "https://example.com/page/", "https://example.com/page?utm_source=x&real=y"
    Expected Result: Fragments stripped, trailing slashes stripped, utm params stripped but real params kept
    Evidence: .sisyphus/evidence/task-3-normalize-urls.txt
  ```

  **Commit**: YES (group with Tasks 1, 2 — Wave 1)
  - Message: `feat(init): scaffold project with config, types, and path sanitization`
  - Files: `utils/sanitize.go`
  - Pre-commit: `go build ./...`

---

- [ ] 4. HTTP Fetcher with Retry + Link Extraction

  **What to do**:
  - Create `utils/fetcher.go` with:
    - `type Fetcher struct` — holds `*http.Client` and config settings
    - `NewFetcher(cfg config.HTTPConfig) *Fetcher` — create http.Client with configured timeout and transport
    - `Fetch(url string) (htmlBody string, err error)` — GET request with:
      - Custom User-Agent header from config
      - Configurable timeout
      - Simple retry: loop up to `MaxRetries` times with `Delay` ms between retries
      - Return full HTML body as string
      - Return descriptive error on non-200 status codes
    - `ExtractLinks(htmlBody string, baseURL string) ([]string, error)` — parse HTML, find all `<a href>` tags:
      - Use `golang.org/x/net/html` tokenizer to extract all `<a>` elements
      - Resolve relative URLs against `baseURL` using `net/url.ResolveReference()`
      - Deduplicate links
      - Return list of absolute URLs
  - Install dependency: `go get golang.org/x/net`

  **Must NOT do**:
  - No proxy support
  - No cookie jar / session management
  - No HEAD request pre-check
  - No content-type filtering (process whatever comes back)
  - No exponential backoff — fixed delay only

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high`
    - Reason: HTTP client + HTML parsing involves I/O patterns and error handling nuances
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES (Wave 2)
  - **Parallel Group**: Wave 2 (with Tasks 5, 6)
  - **Blocks**: Task 7
  - **Blocked By**: Task 1 (needs config types)

  **References**:
  - **External References**:
    - `net/http` — Go standard HTTP client
    - `golang.org/x/net/html` — HTML tokenizer for link extraction
    - `net/url` — URL parsing and resolution for relative → absolute links
  - **WHY**: Fetcher provides raw HTML input; link extraction gives LLM a structured list to pick from (more reliable than LLM parsing markdown)

  **Acceptance Criteria**:
  - [ ] `Fetch("https://example.com")` returns HTML string containing `<html`
  - [ ] `Fetch("https://httpstat.us/404")` returns error mentioning 404
  - [ ] `ExtractLinks(html, "https://example.com")` returns absolute URLs
  - [ ] Relative links like `/page` are resolved to `https://example.com/page`

  **QA Scenarios:**
  ```
  Scenario: Fetch a live page successfully
    Tool: Bash
    Steps:
      1. Write Go main that creates Fetcher with default config
      2. Call Fetch("https://example.com")
      3. Print first 200 chars of result
    Expected Result: Output contains "<!doctype html>" or "<html" (case-insensitive)
    Evidence: .sisyphus/evidence/task-4-fetch-live.txt

  Scenario: Extract and resolve links
    Tool: Bash
    Steps:
      1. Call ExtractLinks with HTML containing <a href="/relative"> and <a href="https://absolute.com">
      2. Print extracted links
    Expected Result: Relative link resolved to full URL, absolute link preserved
    Evidence: .sisyphus/evidence/task-4-extract-links.txt

  Scenario: HTTP error handling
    Tool: Bash
    Steps:
      1. Call Fetch("https://httpstat.us/500")
    Expected Result: Non-nil error, error message contains "500"
    Evidence: .sisyphus/evidence/task-4-http-error.txt
  ```

  **Commit**: YES (group with Tasks 5, 6 — Wave 2)
  - Message: `feat(core): add HTTP fetcher, markdown converter, and LLM client`
  - Files: `utils/fetcher.go`
  - Pre-commit: `go build ./...`

- [ ] 5. HTML-to-Markdown Converter

  **What to do**:
  - Create `utils/converter.go` with:
    - `type Converter struct` — holds `*converter.Converter` instance
    - `NewConverter() *Converter` — initialize with plugins:
      ```go
      conv := converter.NewConverter(
          converter.WithPlugins(
              base.NewBasePlugin(),
              commonmark.NewCommonmarkPlugin(),
              table.NewTablePlugin(),
          ),
      )
      ```
    - `Convert(htmlBody string, sourceURL string) (string, error)` — convert HTML to Markdown:
      - Use `converter.WithDomain(sourceURL)` to resolve relative URLs in images/links
      - Call `conv.ConvertString(htmlBody)` (note: WithDomain is per-call option, use `htmltomarkdown.ConvertString(html, converter.WithDomain(url))` convenience)
      - Return clean markdown string
    - `TruncateContent(markdown string, maxLength int) string` — truncate to maxLength characters (for LLM context window)
      - Truncate at last complete line before maxLength
      - Append `\n\n[Content truncated...]` if truncated
  - Install dependencies:
    - `go get github.com/JohannesKaufmann/html-to-markdown/v2`
    - `go get github.com/JohannesKaufmann/html-to-markdown/v2/plugin/base`
    - `go get github.com/JohannesKaufmann/html-to-markdown/v2/plugin/commonmark`
    - `go get github.com/JohannesKaufmann/html-to-markdown/v2/plugin/table`

  **Must NOT do**:
  - No custom HTML preprocessing or filtering
  - No CSS selector-based content extraction
  - No custom renderer plugins beyond base + commonmark + table

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Thin wrapper around html-to-markdown library, minimal custom logic
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES (Wave 2)
  - **Parallel Group**: Wave 2 (with Tasks 4, 6)
  - **Blocks**: Task 7
  - **Blocked By**: Task 1 (needs go.mod)

  **References**:
  - **External References**:
    - `github.com/JohannesKaufmann/html-to-markdown/v2` — HTML to Markdown library
    - v2 API: `converter.NewConverter(converter.WithPlugins(...))` then `conv.ConvertString(html)`
    - `converter.WithDomain(url)` resolves relative URLs to absolute in output markdown
    - Used by Hugo (gohugoio) and PandaWiki (chaitin) in production
  - **WHY**: Converter bridges raw HTML to LLM-readable markdown; WithDomain ensures links are absolute

  **Acceptance Criteria**:
  - [ ] `Convert("<strong>Bold</strong>", "https://example.com")` returns `**Bold**`
  - [ ] Relative image `<img src="/img.png">` converted to `![](https://example.com/img.png)`
  - [ ] `TruncateContent(longString, 100)` returns string of <=100 chars + truncation notice
  - [ ] Tables in HTML are preserved as markdown tables

  **QA Scenarios:**
  ```
  Scenario: Basic HTML to Markdown conversion
    Tool: Bash
    Steps:
      1. Write Go main that calls Convert with HTML: "<h1>Title</h1><p>Text with <a href='/link'>link</a></p>"
      2. Print result
    Expected Result: Output contains "# Title" and "[link](https://example.com/link)"
    Evidence: .sisyphus/evidence/task-5-basic-convert.txt

  Scenario: Content truncation
    Tool: Bash
    Steps:
      1. Generate 1000-char markdown string
      2. Call TruncateContent(content, 200)
    Expected Result: Output length <= 200 + truncation notice length, ends with "[Content truncated...]"
    Evidence: .sisyphus/evidence/task-5-truncation.txt
  ```

  **Commit**: YES (group with Tasks 4, 6 — Wave 2)
  - Message: `feat(core): add HTTP fetcher, markdown converter, and LLM client`
  - Files: `utils/converter.go`
  - Pre-commit: `go build ./...`

---

- [ ] 6. LLM Client with Structured JSON Output

  **What to do**:
  - Create `utils/llm.go` with:
    - `type LLMClient struct` — holds `*openai.Client`, config, and pre-generated schema
    - `NewLLMClient(cfg config.LLMConfig, promptCfg config.PromptConfig) *LLMClient`:
      ```go
      client := openai.NewClient(
          option.WithBaseURL(cfg.BaseURL),
          option.WithAPIKey(cfg.APIKey),
      )
      ```
      - Pre-generate LLMResponse JSON schema using `GenerateSchema[LLMResponse]()`
      - Parse `promptCfg.UserPromptTemplate` as `text/template.Template`
    - `Analyze(ctx context.Context, pageURL string, depth int, markdown string, links []string) (*LLMResponse, error)`:
      - Render user_prompt_template with template data: `{URL, Depth, Content, Links}`
        - `Content` = markdown (already truncated by caller)
        - `Links` = joined list of extracted links from HTML
      - Build `openai.ChatCompletionNewParams` with:
        - `openai.SystemMessage(systemPrompt)`
        - `openai.UserMessage(renderedUserPrompt)`
        - `openai.ChatModel(cfg.Model)` for model name
        - ResponseFormat depending on `cfg.JSONMode`:
          - If `"json_schema"`: use `OfJSONSchema` with strict schema
          - If `"json_object"`: use `OfJSONObject` (looser, for Ollama/etc.)
      - Call `client.Chat.Completions.New(ctx, params)` — MUST use Chat.Completions, NOT Responses
      - Parse response `Choices[0].Message.Content` as JSON into `LLMResponse`
      - Retry on JSON parse failure up to `MaxRetries` times
      - Return parsed `*LLMResponse`
  - Install dependency: `go get github.com/openai/openai-go/v3`

  **Must NOT do**:
  - **MUST NOT use `client.Responses.New()`** — this is OpenAI-proprietary and won't work with Ollama/DeepSeek/Azure
  - No streaming — wait for full response
  - No function calling / tools — structured output via ResponseFormat only
  - No token counting or cost tracking
  - No caching of LLM responses

  **Recommended Agent Profile**:
  - **Category**: `deep`
    - Reason: Complex API integration with structured output, dual JSON mode support, template rendering, retry logic, and error handling
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES (Wave 2)
  - **Parallel Group**: Wave 2 (with Tasks 4, 5)
  - **Blocks**: Task 7
  - **Blocked By**: Task 1 (config types), Task 2 (LLMResponse type + GenerateSchema)

  **References**:
  - **API/Type References**:
    - `utils/types.go:LLMResponse` — the struct that defines the JSON schema for structured output
    - `utils/types.go:GenerateSchema[T]()` — converts Go struct to JSON schema map
    - `config/config.go:LLMConfig` — base_url, api_key, model, json_mode, max_retries
    - `config/config.go:PromptConfig` — system_prompt, user_prompt_template
  - **External References**:
    - `github.com/openai/openai-go/v3` — official OpenAI Go client
    - Chat.Completions.New API: `openai.ChatCompletionNewParams{Messages, Model, ResponseFormat}`
    - `openai.SystemMessage(text)`, `openai.UserMessage(text)` — message constructors
    - `openai.ChatModel(modelName)` — cast string to model type for custom models
    - `option.WithBaseURL(url)`, `option.WithAPIKey(key)` — client construction options
    - JSON schema response format:
      ```go
      ResponseFormat: openai.ChatCompletionNewParamsResponseFormatUnion{
          OfJSONSchema: &openai.ResponseFormatJSONSchemaParam{
              JSONSchema: openai.ResponseFormatJSONSchemaJSONSchemaParam{
                  Name:   "llm_response",
                  Schema: schema, // from GenerateSchema[LLMResponse]()
                  Strict: openai.Bool(true),
              },
          },
      }
      ```
    - JSON object mode (for providers without schema support):
      ```go
      ResponseFormat: openai.ChatCompletionNewParamsResponseFormatUnion{
          OfJSONObject: &openai.ResponseFormatJSONObjectParam{},
      }
      ```
  - **WHY**: This is the brain of the system — it determines what links to follow and how to organize output. The dual JSON mode support is critical for provider compatibility.

  **Acceptance Criteria**:
  - [ ] `go build ./...` compiles with openai-go dependency
  - [ ] NewLLMClient constructs without panic given valid config
  - [ ] Code uses `client.Chat.Completions.New()` NOT `client.Responses.New()` (verify with grep)
  - [ ] JSON mode switch: `json_schema` sets OfJSONSchema, `json_object` sets OfJSONObject
  - [ ] User prompt template renders with URL, Depth, Content, Links variables
  - [ ] JSON parse failure triggers retry (up to MaxRetries)

  **QA Scenarios:**
  ```
  Scenario: LLM client construction
    Tool: Bash
    Steps:
      1. Write Go main creating LLMClient with test config (any base_url/key)
      2. Verify no panic
    Expected Result: LLMClient created successfully
    Evidence: .sisyphus/evidence/task-6-client-construction.txt

  Scenario: Verify Chat.Completions used (not Responses)
    Tool: Bash
    Steps:
      1. grep -n 'Responses.New' utils/llm.go
      2. grep -n 'Chat.Completions.New' utils/llm.go
    Expected Result: Responses.New = 0 matches, Chat.Completions.New >= 1 match
    Evidence: .sisyphus/evidence/task-6-api-check.txt

  Scenario: Template rendering
    Tool: Bash
    Steps:
      1. Create LLMClient with template "URL: {{.URL}}, Depth: {{.Depth}}, Links: {{.Links}}"
      2. Render with test data
    Expected Result: Placeholders replaced with actual values
    Evidence: .sisyphus/evidence/task-6-template.txt

  Scenario: JSON mode configuration
    Tool: Bash
    Steps:
      1. grep -A5 'json_schema' utils/llm.go | grep 'OfJSONSchema'
      2. grep -A5 'json_object' utils/llm.go | grep 'OfJSONObject'
    Expected Result: Both modes handled in code
    Evidence: .sisyphus/evidence/task-6-json-modes.txt
  ```

  **Commit**: YES (group with Tasks 4, 5 — Wave 2)
  - Message: `feat(core): add HTTP fetcher, markdown converter, and LLM client`
  - Files: `utils/llm.go`
  - Pre-commit: `go build ./...`

---

- [ ] 7. BFS Crawler Engine

  **What to do**:
  - Create `utils/crawler.go` with:
    - `type Crawler struct` — holds all dependencies:
      - `fetcher *Fetcher`
      - `converter *Converter`
      - `llm *LLMClient`
      - `cfg *config.Config`
      - `queue []QueueItem` (protected by mutex)
      - `visited map[string]struct{}` (protected by mutex)
      - `mu sync.Mutex`
      - `wg sync.WaitGroup`
      - `pageCount int32` (atomic counter)
    - `NewCrawler(cfg *config.Config) *Crawler` — initialize all components
    - `Run(ctx context.Context, seedURLs []string) error` — main BFS loop:
      1. Enqueue seed URLs at depth 0 with base output dir as parent path
      2. Start `cfg.BFS.Concurrency` worker goroutines
      3. Each worker loops:
         - Lock mutex, dequeue from queue (if empty, unlock and check WaitGroup)
         - Check depth <= MaxDepth and pageCount < MaxPages
         - Unlock mutex
         - Call `processPage(ctx, item)`
         - `wg.Done()` after processing
      4. Main goroutine: `wg.Wait()` then signal workers to stop (close channel or context cancel)
      5. Return nil on success
    - `enqueue(items []QueueItem)` — thread-safe enqueue:
      - Lock mutex
      - For each item: normalize URL, check visited + check allowed_domains
      - If not visited and domain allowed: mark visited, append to queue, `wg.Add(1)`
      - Unlock mutex
    - `processPage(ctx context.Context, item QueueItem) error` — single page pipeline:
      1. Log: `"Processing [depth=%d] %s"` to stderr
      2. `fetcher.Fetch(item.URL)` → get HTML
      3. `fetcher.ExtractLinks(html, item.URL)` → get link list
      4. `converter.Convert(html, item.URL)` → get markdown
      5. Save markdown file to `BuildOutputPath(baseDir, item.ParentPath, item.FolderName, item.FileName)` with `.md` extension
      6. `converter.TruncateContent(markdown, cfg.LLM.MaxContentLength)` → truncate for LLM
      7. `llm.Analyze(ctx, item.URL, item.Depth, truncatedMD, links)` → get LLMResponse
      8. Write `index.md` in the item's folder: single line with `LLMResponse.Summary`
      9. Convert `LLMResponse.Links` to `[]QueueItem` — each link becomes:
         - URL: `link.URL`
         - Depth: `item.Depth + 1`
         - ParentPath: current item's folder path
         - FolderName: `SanitizePath(link.FolderName)`
         - FileName: `SanitizePath(link.FileName)`
      10. `enqueue(newItems)`
      11. Handle errors at each step: log error, skip page, don't crash
    - Rate limiting: `time.Sleep(time.Duration(cfg.HTTP.Delay) * time.Millisecond)` after each fetch

  **Must NOT do**:
  - No channel-based queue (use mutex-protected slice — simpler for BFS ordering)
  - No persistent state / checkpointing
  - No recursive crawling — strict BFS with queue
  - No goroutine leak — ensure all workers exit when done

  **Recommended Agent Profile**:
  - **Category**: `deep`
    - Reason: Concurrent BFS with mutex, WaitGroup, atomic operations, rate limiting, and multi-step page processing pipeline. This is the most complex component.
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Parallel Group**: Wave 3 (sequential within wave)
  - **Blocks**: Task 8
  - **Blocked By**: Tasks 2, 3, 4, 5, 6 (needs all utility modules)

  **References**:
  - **Pattern References**:
    - BFS pattern from research: `queue = append(queue, item)`, `item = queue[0]; queue = queue[1:]`, `visited map[string]struct{}`
    - Termination detection: `sync.WaitGroup` — `wg.Add(1)` on enqueue, `wg.Done()` after full processing
    - Race condition prevention: `mu.Lock()` around visited check + enqueue (atomic check-and-set)
  - **API/Type References**:
    - `utils/types.go:QueueItem` — queue item struct
    - `utils/types.go:LLMResponse` — LLM output structure
    - `utils/fetcher.go:Fetcher` — HTTP fetching + link extraction
    - `utils/converter.go:Converter` — HTML→Markdown + truncation
    - `utils/llm.go:LLMClient` — LLM analysis
    - `utils/sanitize.go:SanitizePath, BuildOutputPath, NormalizeURL` — path safety
    - `config/config.go:Config` — all configuration
  - **WHY**: This is the orchestrator that ties everything together. The concurrent BFS with proper termination detection is the hardest part. All references are needed because this module calls every other module.

  **Acceptance Criteria**:
  - [ ] `go build ./...` compiles
  - [ ] `go build -race ./...` compiles (no race condition detection issues at compile time)
  - [ ] Crawler processes seed URL and creates output files
  - [ ] BFS stops at configured max_depth
  - [ ] BFS stops at configured max_pages
  - [ ] Visited URLs are not re-processed
  - [ ] Worker count matches config.BFS.Concurrency
  - [ ] Errors in individual pages are logged but don't crash the entire crawl

  **QA Scenarios:**
  ```
  Scenario: Build with race detector
    Tool: Bash
    Steps:
      1. go build -race -o bfs_scraper_race ./cmd
    Expected Result: Compiles successfully, exit 0
    Evidence: .sisyphus/evidence/task-7-race-build.txt

  Scenario: BFS depth limiting
    Tool: Bash
    Steps:
      1. Create config with max_depth=0 (seed only)
      2. Run crawler with single seed URL
      3. Check output directory — should have only seed page files, no child pages
    Expected Result: Only seed page processed, no deeper pages
    Evidence: .sisyphus/evidence/task-7-depth-limit.txt

  Scenario: Error resilience
    Tool: Bash
    Steps:
      1. Add an unreachable URL to seed list alongside a valid URL
      2. Run crawler
    Expected Result: Error logged for bad URL, valid URL still processed, no crash
    Evidence: .sisyphus/evidence/task-7-error-resilience.txt
  ```

  **Commit**: YES (group with Task 8 — Wave 3)
  - Message: `feat(crawler): implement BFS engine and CLI entry point`
  - Files: `utils/crawler.go`
  - Pre-commit: `go build ./...`

- [ ] 8. CLI Entry Point + Example Config

  **What to do**:
  - Create `cmd/main.go`:
    - Parse flags: `-config <path>` (required)
    - Remaining args are seed URLs (at least 1 required)
    - Load config with `config.LoadConfig(configPath)`
    - Create `Crawler` with config
    - Run crawler with seed URLs
    - Handle SIGINT/SIGTERM for graceful shutdown (context cancellation)
    - Exit with code 0 on success, 1 on error
    - Usage message on bad args:
      ```
      Usage: bfs_scraper -config <config.yaml> <seed_url> [seed_url...]
      ```
  - Create `config.example.yaml` with all fields documented:
    ```yaml
    llm:
      base_url: "https://api.openai.com/v1"  # OpenAI-compatible endpoint
      api_key: "sk-your-key-here"
      model: "gpt-4o"
      max_content_length: 32000  # chars to send to LLM
      json_mode: "json_schema"   # "json_schema" (strict) or "json_object" (loose, for Ollama)
      max_retries: 3
    
    bfs:
      max_depth: 2         # 0 = seed page only
      max_pages: 100       # total page safety cap
      concurrency: 3       # parallel workers
      allowed_domains:     # only follow links to these domains
        - "example.com"
    
    http:
      user_agent: "BFS-Scraper/1.0"
      timeout: 30          # seconds
      delay: 1000          # ms between requests per worker
      max_retries: 3
    
    prompt:
      system_prompt: |
        You are a web page analyzer. Given a web page's content and links,
        you must decide which links are worth following for research purposes.
        Return a JSON response with:
        - summary: a one-line summary of this page
        - links: array of links to follow, each with url, folder_name (english/pinyin),
          file_name (original title), and reason
        Only select links that are directly relevant to the research topic.
        Do not follow navigation links, login pages, or unrelated content.
      user_prompt_template: |
        Page URL: {{.URL}}
        Crawl Depth: {{.Depth}}
        
        ## Page Content
        {{.Content}}
        
        ## Available Links
        {{.Links}}
        
        Analyze this page and decide which links to follow.
    
    output:
      base_dir: "output"
    ```

  **Must NOT do**:
  - No interactive mode / REPL
  - No web server / API mode
  - No --verbose / --debug flags (use stderr logging by default)

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Thin CLI wrapper with flag parsing and signal handling
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Parallel Group**: Wave 3 (after Task 7)
  - **Blocks**: F1-F4
  - **Blocked By**: Task 7 (needs crawler)

  **References**:
  - **API/Type References**:
    - `config/config.go:LoadConfig()` — config loading function
    - `utils/crawler.go:NewCrawler(), Run()` — crawler construction and execution
  - **WHY**: CLI is the user-facing entry point; example config is the documentation

  **Acceptance Criteria**:
  - [ ] `go build -o bfs_scraper ./cmd` produces binary
  - [ ] `./bfs_scraper` with no args prints usage and exits with code 1
  - [ ] `./bfs_scraper -config nonexistent.yaml https://example.com` prints config error
  - [ ] `config.example.yaml` contains all config fields with comments
  - [ ] SIGINT during crawl triggers graceful shutdown

  **QA Scenarios:**
  ```
  Scenario: Binary builds and runs
    Tool: Bash
    Steps:
      1. go build -o bfs_scraper ./cmd
      2. ./bfs_scraper 2>&1 || true
    Expected Result: Build succeeds (exit 0), running without args prints usage
    Evidence: .sisyphus/evidence/task-8-build-run.txt

  Scenario: Config file missing
    Tool: Bash
    Steps:
      1. ./bfs_scraper -config nonexistent.yaml https://example.com 2>&1 || true
    Expected Result: Error message about config file not found, exit code 1
    Evidence: .sisyphus/evidence/task-8-missing-config.txt

  Scenario: End-to-end with example config
    Tool: Bash
    Preconditions: Valid API key in config or mock endpoint
    Steps:
      1. ./bfs_scraper -config config.example.yaml https://example.com
      2. ls -R output/
      3. cat output/*/index.md
    Expected Result: Output directory has markdown files, index.md has summary text
    Evidence: .sisyphus/evidence/task-8-e2e.txt
  ```

  **Commit**: YES (group with Task 7 — Wave 3)
  - Message: `feat(crawler): implement BFS engine and CLI entry point`
  - Files: `cmd/main.go`, `config.example.yaml`
  - Pre-commit: `go build ./...`

---

## Final Verification Wave (MANDATORY — after ALL implementation tasks)

> 4 review agents run in PARALLEL. ALL must APPROVE. Rejection → fix → re-run.

- [ ] F1. **Plan Compliance Audit** — `oracle`
  Read the plan end-to-end. For each "Must Have": verify implementation exists (read file, run command). For each "Must NOT Have": search codebase for forbidden patterns (e.g., `Responses.New`, `omitempty` in schema structs, headless browser imports). Check evidence files exist in `.sisyphus/evidence/`. Compare deliverables against plan.
  Output: `Must Have [N/N] | Must NOT Have [N/N] | Tasks [N/N] | VERDICT: APPROVE/REJECT`

- [ ] F2. **Code Quality Review** — `unspecified-high`
  Run `go build ./...` + `go vet ./...`. Review all files for: unused imports, panic in production paths, unchecked errors, race conditions (shared map without mutex). Check for AI slop: excessive comments, over-abstraction, generic variable names. Verify no `omitempty` in LLM response struct. Verify `Chat.Completions.New` used (not `Responses.New`).
  Output: `Build [PASS/FAIL] | Vet [PASS/FAIL] | Files [N clean/N issues] | VERDICT`

- [ ] F3. **Real QA — Build & Run** — `unspecified-high`
  Build the binary. Run with `config.example.yaml` against a known URL (e.g., a simple static page). Verify: markdown files created, `index.md` contains summary, folder structure is sane, invalid URL produces error log not panic, missing config field produces clear error. Save evidence to `.sisyphus/evidence/final-qa/`.
  Output: `Build [PASS/FAIL] | Run [PASS/FAIL] | Scenarios [N/N pass] | VERDICT`

- [ ] F4. **Scope Fidelity Check** — `deep`
  For each task: read "What to do", read actual code. Verify 1:1 — everything in spec was built (no missing), nothing beyond spec was built (no creep). Check "Must NOT do" compliance. Flag: JavaScript rendering code, Responses API usage, structured logging imports, exponential backoff, database drivers.
  Output: `Tasks [N/N compliant] | Scope Violations [CLEAN/N issues] | VERDICT`

---

## Commit Strategy

- **Commit 1** (after Wave 1): `feat(init): scaffold project with config, types, and path sanitization` — config/config.go, utils/types.go, utils/sanitize.go, go.mod, go.sum
- **Commit 2** (after Wave 2): `feat(core): add HTTP fetcher, markdown converter, and LLM client` — utils/fetcher.go, utils/converter.go, utils/llm.go
- **Commit 3** (after Wave 3): `feat(crawler): implement BFS engine and CLI entry point` — utils/crawler.go, cmd/main.go, config.example.yaml
- **Commit 4** (after Final): `chore: final QA fixes if any`

---

## Success Criteria

### Verification Commands
```bash
go build -o bfs_scraper ./cmd            # Expected: binary created, exit 0
go vet ./...                              # Expected: no issues, exit 0
go build -race -o bfs_scraper_race ./cmd  # Expected: compiles, exit 0
./bfs_scraper -config config.example.yaml https://example.com  # Expected: output directory created with .md files
ls output/                                # Expected: at least 1 directory
find output/ -name "index.md" | wc -l    # Expected: >= 1
```

### Final Checklist
- [ ] All "Must Have" present
- [ ] All "Must NOT Have" absent
- [ ] `go build` and `go vet` pass
- [ ] Binary runs without panic on valid input
- [ ] Binary exits cleanly with error message on invalid input
- [ ] Concurrent execution has no race conditions
