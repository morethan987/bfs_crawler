package utils

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/morethan/bfs_crawler/config"
)

type Crawler struct {
	fetcher   *Fetcher
	converter *Converter
	llm       *LLMClient
	cfg       *config.Config
	queue     []QueueItem
	visited   map[string]struct{}
	mu        sync.Mutex
	wg        sync.WaitGroup
	pageCount int32
}

func NewCrawler(cfg *config.Config) (*Crawler, error) {
	fetcher := NewFetcher(
		cfg.HTTP.UserAgent,
		cfg.HTTP.Timeout,
		cfg.HTTP.Delay,
		cfg.HTTP.MaxRetries,
	)
	conv := NewConverter()
	llmClient, err := NewLLMClient(
		cfg.LLM.BaseURL,
		cfg.LLM.APIKey,
		cfg.LLM.Model,
		cfg.LLM.JSONMode,
		cfg.LLM.MaxRetries,
		cfg.Prompt.SystemPrompt,
		cfg.Prompt.UserPromptTemplate,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create LLM client: %w", err)
	}
	return &Crawler{
		fetcher:   fetcher,
		converter: conv,
		llm:       llmClient,
		cfg:       cfg,
		visited:   make(map[string]struct{}),
	}, nil
}

func (c *Crawler) Run(ctx context.Context, seedURLs []string) error {
	seedItems := make([]QueueItem, 0, len(seedURLs))
	for _, rawURL := range seedURLs {
		normalized := NormalizeURL(rawURL)
		seedItems = append(seedItems, QueueItem{
			URL:        normalized,
			Depth:      0,
			ParentPath: c.cfg.Output.BaseDir,
			FolderName: "root",
			FileName:   "index",
		})
	}
	c.enqueue(seedItems)

	workerCount := c.cfg.BFS.Concurrency
	if workerCount <= 0 {
		workerCount = 1
	}

	stop := make(chan struct{})

	var workerWg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		workerWg.Add(1)
		go func() {
			defer workerWg.Done()
			c.worker(ctx, stop)
		}()
	}

	c.wg.Wait()
	close(stop)
	workerWg.Wait()

	return nil
}

func (c *Crawler) worker(ctx context.Context, stop <-chan struct{}) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-stop:
			return
		default:
		}

		c.mu.Lock()
		if len(c.queue) == 0 {
			c.mu.Unlock()
			time.Sleep(10 * time.Millisecond)
			select {
			case <-stop:
				return
			default:
				continue
			}
		}
		item := c.queue[0]
		c.queue = c.queue[1:]
		c.mu.Unlock()

		err := c.processPage(ctx, item)
		if err != nil {
			log.Printf("[ERROR] processing %s: %v", item.URL, err)
		}
		c.wg.Done()
	}
}

func (c *Crawler) enqueue(items []QueueItem) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, item := range items {
		normalized := NormalizeURL(item.URL)
		if normalized == "" {
			continue
		}
		if len(c.cfg.BFS.AllowedDomains) > 0 {
			allowed := false
			for _, domain := range c.cfg.BFS.AllowedDomains {
				if strings.Contains(normalized, domain) {
					allowed = true
					break
				}
			}
			if !allowed {
				continue
			}
		}
		if _, seen := c.visited[normalized]; seen {
			continue
		}
		if int(atomic.LoadInt32(&c.pageCount)) >= c.cfg.BFS.MaxPages {
			continue
		}
		c.visited[normalized] = struct{}{}
		item.URL = normalized
		c.queue = append(c.queue, item)
		c.wg.Add(1)
	}
}

func (c *Crawler) processPage(ctx context.Context, item QueueItem) error {
	if item.Depth > c.cfg.BFS.MaxDepth {
		log.Printf("[SKIP] max depth reached for %s", item.URL)
		return nil
	}

	newCount := atomic.AddInt32(&c.pageCount, 1)
	if int(newCount) > c.cfg.BFS.MaxPages {
		atomic.AddInt32(&c.pageCount, -1)
		log.Printf("[SKIP] max pages reached, skipping %s", item.URL)
		return nil
	}

	log.Printf("[Processing] depth=%d %s", item.Depth, item.URL)

	result, err := c.fetcher.Fetch(item.URL)
	if err != nil {
		return fmt.Errorf("fetch failed: %w", err)
	}

	// Route based on Content-Type
	if !result.IsHTML {
		return c.handleNonHTML(result, item)
	}

	return c.handleHTML(ctx, result, item)
}

// handleNonHTML saves downloadable files directly and skips unknown content types.
// Downloads do not count toward max_pages.
func (c *Crawler) handleNonHTML(result *FetchResult, item QueueItem) error {
	// Decrement page count — downloads don't count toward max_pages
	atomic.AddInt32(&c.pageCount, -1)

	if IsDownloadableContentType(result.ContentType) {
		ext := ExtensionFromContentType(result.ContentType, item.URL)
		if ext == "" {
			ext = ".bin"
		}
		outputPath := BuildOutputPath(c.cfg.Output.BaseDir, item.ParentPath, item.FolderName, item.FileName)
		filePath := outputPath + ext
		if err := os.WriteFile(filePath, result.Body, 0644); err != nil {
			return fmt.Errorf("failed to save file: %w", err)
		}
		log.Printf("[Downloaded] %s (%s) -> %s", item.URL, result.ContentType, filePath)
		return nil
	}

	// Unknown content type — skip
	log.Printf("[SKIP] unsupported content type %q for %s", result.ContentType, item.URL)
	return nil
}

// handleHTML processes an HTML page: convert to markdown, annotate links, save, LLM analyze, enqueue.
func (c *Crawler) handleHTML(ctx context.Context, result *FetchResult, item QueueItem) error {
	htmlBody := string(result.Body)

	markdown, err := c.converter.Convert(htmlBody, item.URL)
	if err != nil {
		return fmt.Errorf("HTML conversion failed: %w", err)
	}

	// Annotate links in markdown with IDs (⟨L1⟩, ⟨L2⟩, ...)
	// Links stay in their natural page context for better LLM comprehension
	annotated, linkRefs := c.converter.AnnotateLinks(markdown)

	outputPath := BuildOutputPath(c.cfg.Output.BaseDir, item.ParentPath, item.FolderName, item.FileName)
	mdPath := outputPath + ".md"
	metadata := fmt.Sprintf("---\nsource_url: %s\ndepth: %d\ncrawl_time: %s\n---\n\n", item.URL, item.Depth, time.Now().Format(time.RFC3339))
	if err := os.WriteFile(mdPath, []byte(metadata+markdown), 0644); err != nil {
		return fmt.Errorf("failed to save markdown: %w", err)
	}
	log.Printf("[Saved] %s", mdPath)

	truncated := c.converter.TruncateContent(annotated, c.cfg.LLM.MaxContentLength)

	llmResp, err := c.llm.Analyze(ctx, item.URL, item.Depth, truncated, item.Reason)
	if err != nil {
		log.Printf("[WARN] LLM analysis failed for %s: %v", item.URL, err)
		return nil
	}

	folderPath := filepath.Dir(mdPath)
	indexPath := filepath.Join(folderPath, "index.md")
	if err := os.WriteFile(indexPath, []byte(llmResp.Summary+"\n"), 0644); err != nil {
		log.Printf("[WARN] failed to write index.md for %s: %v", item.URL, err)
	}

	if item.Depth < c.cfg.BFS.MaxDepth {
		// Build link ref lookup map
		refMap := make(map[string]LinkRef, len(linkRefs))
		for _, ref := range linkRefs {
			refMap[ref.ID] = ref
		}

		// Filter links by relevance score
		minScore := c.cfg.BFS.MinRelevanceScore
		maxLinks := c.cfg.BFS.MaxLinksPerPage

		// Depth-based tightening: reduce max links and increase min score at deeper levels
		if item.Depth >= 3 {
			minScore = max(minScore, 70)
			maxLinks = max(1, maxLinks/2)
		}
		if item.Depth >= 4 {
			minScore = max(minScore, 80)
			maxLinks = 1
		}

		// Filter by score threshold and resolve link IDs
		var qualified []LinkItem
		for _, link := range llmResp.Links {
			if _, exists := refMap[link.LinkID]; !exists {
				log.Printf("[WARN] LLM returned unknown link_id %q for %s, skipping", link.LinkID, item.URL)
				continue
			}
			if link.RelevanceScore >= minScore {
				qualified = append(qualified, link)
			} else {
				log.Printf("[SKIP] link %s score=%d < min=%d for %s", link.LinkID, link.RelevanceScore, minScore, item.URL)
			}
		}

		// Sort by score descending and take top N
		sort.Slice(qualified, func(i, j int) bool {
			return qualified[i].RelevanceScore > qualified[j].RelevanceScore
		})
		if len(qualified) > maxLinks {
			qualified = qualified[:maxLinks]
		}

		childPath := filepath.Join(item.ParentPath, SanitizePath(item.FolderName))
		childItems := make([]QueueItem, 0, len(qualified))
		for _, link := range qualified {
			ref := refMap[link.LinkID]
			log.Printf("[Enqueue] %s score=%d reason=%q %s", link.LinkID, link.RelevanceScore, link.Reason, ref.URL)
			childItems = append(childItems, QueueItem{
				URL:        ref.URL,
				Depth:      item.Depth + 1,
				ParentPath: childPath,
				FolderName: link.FolderName,
				FileName:   link.FileName,
				Reason:     link.Reason,
			})
		}
		c.enqueue(childItems)
	}

	time.Sleep(time.Duration(c.cfg.HTTP.Delay) * time.Millisecond)

	return nil
}
