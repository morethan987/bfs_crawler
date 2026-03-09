## [2026-03-09] Task 1: Config

### Module & Dependency
- Module: github.com/morethan/bfs_scraper
- YAML library: gopkg.in/yaml.v3 (v3.0.1)

### Config Structure
- Root Config struct contains 5 sub-configs: LLM, BFS, HTTP, Prompt, Output
- All fields use yaml struct tags for YAML unmarshaling
- No pointers used in struct definitions (simple value types)

### LoadConfig Function
- Reads YAML from file path
- Validates and applies defaults
- Returns *Config or error
- Errors are wrapped with fmt.Errorf for context

### Validation Rules
Required fields (no empty string/zero allowed):
- llm.base_url
- llm.api_key
- llm.model
- prompt.system_prompt
- prompt.user_prompt_template

Default Values Applied:
- llm.max_content_length: 32000
- llm.max_retries: 3
- llm.json_mode: "json_schema"
- bfs.max_pages: 100
- bfs.concurrency: 3
- http.timeout: 30 (seconds)
- http.delay: 1000 (milliseconds)
- http.max_retries: 3
- output.base_dir: "output"

### Build Status
- go build ./... passes successfully
- go mod tidy resolves all dependencies
- No linting or diagnostic issues

### Test Evidence
✓ Valid config loads successfully with all defaults applied
✓ Missing api_key is caught and rejected with clear error message

## [2026-03-09] Task 2: Types
- GenerateSchema uses invopop/jsonschema with AllowAdditionalProperties:false, DoNotReference:true
- CRITICAL: NO omitempty in LLMResponse/LinkItem JSON tags - breaks strict JSON schema mode
- QueueItem has NO json tags (internal struct, not serialized)
- LLMResponse.Links field name is "links" (lowercase), LLMResponse.Summary is "summary"

## [2026-03-09] Task 3: Sanitize
- SanitizePath uses FieldsFunc to split on / and \, then joins with _ to handle path traversal
- Truncation at 200 chars (not runes) - fine for ASCII-heavy names
- NormalizeURL strips: fragment, trailing slash (except root), utm_* params, ref, source
- BuildOutputPath: parentPath="" means use baseDir/safeFolder directly
- os.MkdirAll called inside BuildOutputPath (side effect, but needed for convenience)
- Path components "..", ".", "" are filtered by FieldsFunc - prevents path traversal
- Empty strings default to "unnamed" (fallback for unnamed files)
- All OS-illegal characters (\/:*?"<>|) replaced with underscores
- Root path "/" preserved by NormalizeURL to avoid stripping protocol-only URLs

## [2026-03-09] Task 5: Converter

### HTML-to-Markdown Library
- Library: github.com/JohannesKaufmann/html-to-markdown/v2 (v2.5.0)
- Main packages: converter, plugin/base, plugin/commonmark, plugin/table

### API Details (Actual Implementation)
- NewConverter takes variadic converterOption arguments
- WithPlugins(plugins ...Plugin) is a converterOption factory function
- Plugin constructors:
  - base.NewBasePlugin() returns converter.Plugin
  - commonmark.NewCommonmarkPlugin(opts ...OptionFunc) returns converter.Plugin
  - table.NewTablePlugin(opts ...option) returns converter.Plugin

### Converter Methods
- ConvertString(htmlInput string, opts ...ConvertOptionFunc) (string, error)
- ConvertNode(doc *html.Node, opts ...ConvertOptionFunc) ([]byte, error)
- ConvertReader(r io.Reader, opts ...ConvertOptionFunc) ([]byte, error)
- WithDomain(domain string) ConvertOptionFunc for URL resolution

### Implementation Pattern (utils/converter.go)
- Converter struct wraps *converter.Converter from the library
- NewConverter() creates instance with all 3 plugins registered via WithPlugins
- Convert(htmlBody string, sourceURL string) uses ConvertString with WithDomain for absolute URL resolution
- TruncateContent(markdown string, maxLength int) truncates at line boundary and appends "[Content truncated...]"

### URL Resolution
- WithDomain resolves relative URLs to absolute URLs correctly
- Example: <a href="/link"> with domain "https://example.com" becomes [link](https://example.com/link)

### Build & Testing
- go build ./... passes without errors
- Basic conversion test: <h1>Title</h1> → # Title ✓
- Link resolution test: /link with baseURL https://example.com → https://example.com/link ✓
- Truncation test: 1471 chars → 169 chars with "[Content truncated...]" appended ✓


## [2026-03-09] Task 4: Fetcher
- NewFetcher takes: userAgent string, timeoutSec int, delayMs int, maxRetries int (NOT config struct)
- Retry loop: attempt 0..maxRetries (so maxRetries=3 means 4 total attempts including first)
- ExtractLinks uses golang.org/x/net/html tokenizer (NOT html.Parse tree - more memory efficient)
- Links filtered: empty, "#", "javascript:", "mailto:" are skipped
- Relative URLs resolved via url.Parse + base.ResolveReference(ref).String()
- Deduplication via seen map[string]struct{}
- delay is used BETWEEN retries (not before first attempt)
- Environment TLS note: This dev env has x509 cert issues with HTTPS; Fetcher works fine with HTTP
- golang.org/x/net upgraded to v0.51.0

## [2026-03-09] Task 6: LLM Client (openai-go/v3)
- Installed github.com/openai/openai-go/v3 v3.26.0
- NewClient returns value type `openai.Client`; when storing in struct pointer field, take address (`&client`)
- Required chat call path is `client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{...})`
- Message helpers verified in package: `openai.SystemMessage(...)`, `openai.UserMessage(...)`
- `ChatCompletionNewParams` core fields are direct values (not `openai.F(...)` in this version):
  - `Messages []openai.ChatCompletionMessageParamUnion`
  - `Model openai.ChatModel`
  - `ResponseFormat openai.ChatCompletionNewParamsResponseFormatUnion`
- `openai.ChatModel` is alias to string (`type ChatModel = string`), so `openai.ChatModel(c.model)` is valid
- JSON modes supported via `ChatCompletionNewParamsResponseFormatUnion` variants:
  - `json_schema`: `OfJSONSchema: &openai.ResponseFormatJSONSchemaParam{ JSONSchema: openai.ResponseFormatJSONSchemaJSONSchemaParam{ Name, Schema, Strict } }`
  - `json_object`: `OfJSONObject: &openai.ResponseFormatJSONObjectParam{}`
- For json_schema strict flag type is `param.Opt[bool]`; set via `openai.Bool(true)`
- Response content extraction verified: `resp.Choices[0].Message.Content`
- No `Responses.New` usage in llm.go (Chat Completions only)


## [2026-03-09] Task 7: Crawler
- Run() uses wg (sync.WaitGroup) tracking BFS items: Add(1) on enqueue, Done() after processPage
- Worker pattern: lock→dequeue→unlock→process→Done
- Queue empty + stop channel closed when wg.Wait() completes
- pageCount is int32 used with atomic.AddInt32/LoadInt32 (safe for concurrent workers)
- enqueue() does: NormalizeURL, AllowedDomains check, visited check, MaxPages check - all under mutex
- processPage() depth check is FIRST (before incrementing pageCount) to avoid counting skipped pages
- rate limiting: time.Sleep(delay) in processPage AFTER all file writes (not in worker loop)
- BuildOutputPath returns the full file path WITHOUT .md extension; processPage appends ".md"
- folderPath for index.md: filepath.Dir(mdPath) - gets the directory containing the markdown file
- childPath for child items: filepath.Join(cfg.Output.BaseDir, item.ParentPath, SanitizePath(item.FolderName))
