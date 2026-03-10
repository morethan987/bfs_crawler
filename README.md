# bfs_crawler

**LLM 驱动的 BFS 网页爬虫** — 自动抓取网页、转换为 Markdown，并由大语言模型决策链接入队顺序，将结果以层级目录结构保存到本地。

---

## 工作原理

### 整体架构

```
种子 URL(s)
    │
    ▼
┌─────────────────────────────────────────────────────────────┐
│                        BFS 引擎                             │
│                                                             │
│  队列 ──► 取出 QueueItem ──► 检查深度/页数上限              │
│              │                                              │
│              ▼                                              │
│          HTTP 抓取                                          │
│              │                                              │
│     ┌────────┴──────────┐                                   │
│     ▼                   ▼                                   │
│  text/html          PDF/图片/Office                         │
│  （HTML 处理流程）   （直接下载保存）    其他类型 → 忽略    │
│     │                   │                                   │
└─────┼───────────────────┼───────────────────────────────────┘
      │                   │
      ▼                   ▼
  HTML 处理流程         {文件名}.pdf / .xlsx / .png ...
      │
      ├─① HTML → Markdown 转换
      │
      ├─② 链接标注（⟨L1⟩, ⟨L2⟩, ... 内联标记 + 裸URL检测）
      │
      ├─③ 写入 .md 文件（含 YAML front matter 元数据）
      │
      └─④ 调用 LLM 分析（附带上游 Reason 上下文）
              │
          ┌───┴──────────────────────────┐
          ▼                              ▼
   写 index.md（摘要）        返回 link_id 列表
                                    （含 relevance_score 0-100）
                                         │
                              ┌──────────┤
                              ▼          ▼
                         分数过滤    深度收紧策略
                         ≥ min_score  depth≥3 → 阈值↑ 数量↓
                              │
                              ▼
                         link_id → URL 解析
                              │
                              ▼
                         URL 去重（NormalizeURL）
                              │
                              ▼
                         入队（深度 +1, 携带 Reason）
                              │
                    （直到队列为空或达到上限）
```

### 逐步说明

#### 1. 初始化与队列启动

程序从命令行读取一个或多个**种子 URL**，将它们以深度 `0` 放入 BFS 队列。
每个队列项（`QueueItem`）携带：URL、当前深度、父目录路径、LLM 建议的目录名和文件名，以及上游传递的 `Reason`（爬取原因，用于 LLM 上下文传递）。
每个队列项（`QueueItem`）携带：URL、当前深度、父目录路径、LLM 建议的目录名和文件名。

#### 2. URL 规范化与去重

每个 URL 入队前都经过 `NormalizeURL` 处理：
- 统一小写 scheme 和 host
- 将 `/index.html`、`/index.htm` 合并为目录路径（`/`），避免同一页面被重复抓取
- 排序 query 参数，确保参数顺序不同的相同 URL 只爬一次
- 剥离 `#fragment` 和常见追踪参数（`utm_*`、`ref`、`source`）

已访问的规范化 URL 记录在 `visited` 集合中，重复 URL 直接丢弃。

#### 3. HTTP 抓取与内容类型路由

抓取器发出 HTTP GET 请求，读取响应头中的 `Content-Type`，然后**按内容类型分三条路径处理**：

| Content-Type | 处理方式 |
|---|---|
| `text/html` | 进入 HTML 处理流程（规则 4） |
| PDF / Office / 图片 / 压缩包 / 媒体文件 | 直接以原始字节写入磁盘，保留 LLM 建议的路径，**不计入** `max_pages` 页数限制 |
| 其他未知类型 | 跳过，记录日志，**不计入** `max_pages` |

> **为什么下载文件不计入 `max_pages`？**  
> 二进制文件不产生新链接，不消耗 LLM 调用，属于"捎带下载"，不应占用页面配额。

#### 4. HTML 处理流程

**4-1. HTML → Markdown 转换**

使用 `html-to-markdown` 库将 HTML 转为 Markdown，保留标题、列表、表格等结构。

**4-2. 链接标注（Link Annotation）**

转换后的 Markdown 经过 `AnnotateLinks` 处理，系统会：

1. **扫描 Markdown 链接**：找到所有 `[text](url)` 形式的链接，在其后插入内联标记 `⟨L1⟩`、`⟨L2⟩`、...
2. **检测裸 URL**：找到文本中以 `http://` 或 `https://` 开头的、不在 Markdown 链接语法中的裸 URL（例如 `<p>` 标签中的纯文本 URL），同样标记
3. **去重**：同一 URL 规范化后只分配一个 ID，重复出现时复用
4. **过滤**：静态资源（.css, .js, .png 等）不标记

标注后的 Markdown 示例：

```markdown
访问 [李佳教授主页](https://example.com/lijia) ⟨L1⟩ 了解更多。
论文列表见 [Publications](https://example.com/pubs) ⟨L2⟩。
个人网站：https://lijia.github.io ⟨L3⟩
```

系统同时返回一个**引用映射表**（`[]LinkRef`），记录每个 ID 对应的规范化 URL 和锚文字。

**与旧方案的区别**：旧方案单独提取 `<a href>` 链接列表，链接与页面内容脱节。新方案将链接保留在原文语境中，LLM 可以看到链接出现的位置（比如在「教授团队」标题下还是在「学生社团」段落中），从而做出更准确的相关性判断。

**4-3. 写入 .md 文件**

每个 HTML 页面保存为一个 `.md` 文件（保存的是**未标注**的原始 Markdown），文件开头附有 YAML front matter：

```yaml
---
source_url: https://example.com/page
depth: 2
crawl_time: 2026-03-10T19:04:00+08:00
---
```

标注后的 Markdown 按 `max_content_length`（默认 32000 字符）在行边界截断后发送给 LLM，避免超出上下文窗口。

文件路径由 LLM 在上一步建议，经 `SanitizePath` 清理后写入 `output/` 下的对应层级目录。

**4-4. LLM 分析**

将以下内容发送给 LLM：
- 当前页面 URL 和爬取深度
- 上游传递的爬取原因（`ParentReason`），告诉 LLM「为什么来到这个页面」
- 截断后的**标注 Markdown**（链接已内联标记 `⟨Lxx⟩`）

LLM 返回结构化 JSON：

```json
{
  "summary": "页面摘要（2-4句）",
  "links": [
    {
      "link_id": "L1",
      "folder_name": "目录名（snake_case）",
      "file_name": "文件名",
      "reason": "选择原因",
      "relevance_score": 85
    }
  ]
}
```

注意 LLM 返回的是 `link_id`（如 `"L1"`）而非完整 URL。代码通过引用映射表将 `link_id` 解析回真实 URL。
这种设计避免了 LLM 在输出中复制长 URL 时常见的拼写错误问题。

摘要写入当前目录的 `index.md`。

#### 5. 相关性过滤与深度收紧

LLM 返回的链接经过两层代码级过滤，防止爬虫偏离目标：

**第一层：分数阈值过滤**

只有 `relevance_score ≥ min_relevance_score`（默认 60）的链接才进入候选。

**第二层：深度收紧策略**

| 当前深度 | 最低分数 | 最多入队数 |
|---|---|---|
| 0–2 | `min_relevance_score`（默认 60） | `max_links_per_page`（默认 5） |
| 3 | max(70, min_relevance_score) | max(1, max_links_per_page / 2) |
| 4+ | max(80, min_relevance_score) | 1 |

越深入爬取，标准越严格，防止相关性随深度衰减导致的"爬虫跑偏"。

过滤后的链接先通过 `link_id → URL` 映射解析出真实 URL，再按分数降序排列，取前 N 个，经 URL 去重后入队。
入队时，`QueueItem.Reason` 字段携带 LLM 给出的选择理由，传递给下一层页面的 LLM 分析，形成上下文链。
即：每个页面的 LLM 不仅知道「页面内容是什么」，还知道「为什么来到这个页面」。

#### 6. 终止条件

以下任一条件满足时，停止向队列添加新链接：
- 当前深度 ≥ `max_depth`
- 已抓取 HTML 页面数 ≥ `max_pages`
- 队列为空

按 `Ctrl+C` 可触发优雅退出，等待当前正在处理的页面完成后停止。

---

## 快速开始

### 1. 编译

```bash
git clone https://github.com/morethan/bfs_crawler
cd bfs_crawler

# 动态链接编译
go build -o target/bfs_crawler ./cmd/...

# 静态链接编译
CGO_ENABLED=0 go build -o target/bfs_crawler ./cmd/...

# 交叉编译 for windows
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o target/bfs_crawler.exe ./cmd/...
```

### 2. 配置

```bash
cp config.example.yaml config.yaml
# 用你喜欢的编辑器打开 config.yaml，填写 LLM 参数和爬取目标
```

### 3. 运行

```bash
./bfs_crawler -config config.yaml https://example.com/target-page
```

支持多个种子 URL：

```bash
./bfs_crawler -config config.yaml https://site.com/page1 https://site.com/page2
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
| `min_relevance_score` | `60` | LLM 返回的链接最低相关性分数（0-100），低于此分数的链接不入队 |
| `max_links_per_page` | `5` | 每页最多入队链接数（按相关性分数降序取前 N 个） |

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
| `{{.Content}}` | 页面 Markdown 内容（已标注链接 `⟨Lxx⟩`，已截断） |
| `{{.ParentReason}}` | 上游页面传递的爬取原因（种子页面为空） |

提示词必须要求 LLM 返回如下 JSON 结构：

```json
{
  "summary": "页面摘要文字",
  "links": [
    {"link_id": "L1", "folder_name": "目录名", "file_name": "文件名", "reason": "入队原因", "relevance_score": 85}
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
- **`*.md`**：原始页面 HTML 转换后的 Markdown 正文，文件开头包含 YAML front matter 元数据（`source_url`、`depth`、`crawl_time`）

---

## 命令行参数

```
./bfs_crawler -config <配置文件路径> <种子URL> [种子URL...]
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
