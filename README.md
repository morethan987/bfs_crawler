# bfs_scraper

**LLM 驱动的 BFS 网页爬虫** — 自动抓取网页、转换为 Markdown，并由大语言模型决策链接入队顺序，将结果以层级目录结构保存到本地。

---

## 工作原理

```
种子 URL
   │
   ▼
抓取 HTML ──► 转换为 Markdown ──► 写入 .md 文件
                                       │
                                       ▼
                              输入给 LLM（截断至 max_content_length）
                                       │
                          ┌────────────┴────────────┐
                          ▼                         ▼
                   写 index.md（摘要）      选择链接 + 生成目录/文件名
                                                    │
                                                    ▼
                                             加入 BFS 队列
                                                    │
                                        （直到队列为空或达到上限）
```

每个页面经过以下步骤：

1. HTTP 抓取 HTML 原文
2. 提取页面中所有 `<a href>` 链接
3. 将 HTML 转换为 Markdown
4. 保存 Markdown 到对应路径
5. 将 Markdown 内容（截断后）送给 LLM，LLM 返回：
   - `summary`：页面摘要，写入当前目录的 `index.md`
   - `links[]`：值得继续爬取的链接列表，附带建议的目录名和文件名
6. 将 LLM 选择的链接加入队列，深度 +1，循环继续

---

## 快速开始

### 1. 编译

```bash
git clone https://github.com/morethan/bfs_scraper
cd bfs_scraper
go build -o target/bfs_scraper ./cmd/...
```

### 2. 配置

```bash
cp config.example.yaml config.yaml
# 用你喜欢的编辑器打开 config.yaml，填写 LLM 参数和爬取目标
```

### 3. 运行

```bash
./bfs_scraper -config config.yaml https://example.com/target-page
```

支持多个种子 URL：

```bash
./bfs_scraper -config config.yaml https://site.com/page1 https://site.com/page2
```

按 `Ctrl+C` 可优雅退出（等待当前页面处理完成后停止）。

---

## 配置说明

### `llm` — 大语言模型

| 字段 | 默认值 | 说明 |
|------|--------|------|
| `base_url` | *(必填)* | OpenAI 兼容 API 端点，如 `https://api.openai.com/v1` |
| `api_key` | *(必填)* | API 密钥（Ollama 本地部署可填任意字符串） |
| `model` | *(必填)* | 模型名称，如 `gpt-4o`、`deepseek-chat`、`llama3` |
| `max_content_length` | `32000` | 发送给 LLM 的页面内容最大字符数 |
| `json_mode` | `"json_schema"` | 结构化输出模式：`"json_schema"`（OpenAI）或 `"json_object"`（Ollama 等） |
| `max_retries` | `3` | LLM 调用失败后的重试次数 |

### `bfs` — 爬取策略

| 字段 | 默认值 | 说明 |
|------|--------|------|
| `max_depth` | `0` | 最大爬取深度（0 = 仅种子页面，2 = 种子 + 两层子页面） |
| `max_pages` | `100` | 最大总页面数上限 |
| `concurrency` | `3` | 并发工作协程数 |
| `allowed_domains` | *(空，不限)* | 域名白名单，空列表表示允许所有域名 |

### `http` — HTTP 客户端

| 字段 | 默认值 | 说明 |
|------|--------|------|
| `user_agent` | `""` | 请求时使用的 User-Agent 头 |
| `timeout` | `30` | 单次 HTTP 请求超时时间（秒） |
| `delay` | `1000` | 请求间延迟（毫秒），用于礼貌爬取和重试间隔 |
| `max_retries` | `3` | HTTP 请求失败后的重试次数 |

### `prompt` — LLM 提示词

| 字段 | 说明 |
|------|------|
| `system_prompt` | *(必填)* 系统提示词，定义 LLM 的角色和任务 |
| `user_prompt_template` | *(必填)* 用户提示词模板，支持 Go `text/template` 语法 |

模板中可用变量：

| 变量 | 内容 |
|------|------|
| `{{.URL}}` | 当前页面 URL |
| `{{.Depth}}` | 当前爬取深度 |
| `{{.Content}}` | 页面 Markdown 内容（已截断） |
| `{{.Links}}` | 页面中提取的链接列表（换行分隔） |

提示词必须要求 LLM 返回如下 JSON 结构：

```json
{
  "summary": "页面摘要文字",
  "links": [
    {"url": "https://...", "folder_name": "目录名", "file_name": "文件名"}
  ]
}
```

### `output` — 输出设置

| 字段 | 默认值 | 说明 |
|------|--------|------|
| `base_dir` | `"output"` | 输出根目录路径 |

---

## 支持的 LLM 提供商

只要提供兼容 OpenAI Chat Completions API 的端点，均可使用。

| 提供商 | `base_url` | `json_mode` |
|--------|------------|-------------|
| **OpenAI** | `https://api.openai.com/v1` | `json_schema` |
| **DeepSeek** | `https://api.deepseek.com/v1` | `json_schema` |
| **Azure OpenAI** | `https://<resource>.openai.azure.com/openai/deployments/<deployment>` | `json_schema` |
| **Ollama（本地）** | `http://localhost:11434/v1` | `json_object` |
| **其他兼容服务** | 填写对应端点 | 视支持情况而定 |

> Ollama 等本地模型通常不支持 `json_schema` 严格模式，需将 `json_mode` 设为 `"json_object"`，并在提示词中明确要求 LLM 输出合法 JSON。

---

## 输出结构

爬取结果以层级目录形式保存，目录和文件名由 LLM 生成：

```
output/
└── root/
    ├── index.md              ← 种子页面的 LLM 摘要
    ├── seed_page.md          ← 种子页面的完整 Markdown
    ├── professors/
    │   ├── index.md          ← 该目录的 LLM 摘要
    │   ├── zhang_san.md
    │   └── li_si.md
    └── research/
        ├── index.md
        └── paper_nlp.md
```

- **`index.md`**：LLM 对该层页面的摘要描述
- **`*.md`**：原始页面 HTML 转换后的 Markdown 正文

---

## 命令行参数

```
./bfs_scraper -config <配置文件路径> <种子URL> [种子URL...]
```

| 参数 | 说明 |
|------|------|
| `-config` | 配置文件路径（默认：`config.yaml`） |
| 位置参数 | 一个或多个种子 URL，至少需要一个 |

---

## 注意事项

- **礼貌爬取**：`http.delay` 控制每次请求之间的等待时间，建议不低于 500ms，避免对目标站点造成压力。
- **域名限制**：`bfs.allowed_domains` 为空时允许跨域跟踪链接，建议填写目标站点域名防止爬虫跑偏。
- **深度 vs 页数**：`max_depth` 和 `max_pages` 同时生效，任一先触发上限即停止新链接入队。
- **并发安全**：爬虫使用互斥锁和原子计数器保护共享状态，`concurrency` 可根据目标站点响应速度适当调整。
- **LLM 费用**：每个页面都会调用一次 LLM，爬取大量页面时注意 Token 用量与 API 费用。
