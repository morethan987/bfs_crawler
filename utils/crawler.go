package utils

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/morethan/bfs_scraper/config"
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

	html, err := c.fetcher.Fetch(item.URL)
	if err != nil {
		return fmt.Errorf("fetch failed: %w", err)
	}

	links, err := c.fetcher.ExtractLinks(html, item.URL)
	if err != nil {
		log.Printf("[WARN] link extraction failed for %s: %v", item.URL, err)
		links = []string{}
	}

	markdown, err := c.converter.Convert(html, item.URL)
	if err != nil {
		return fmt.Errorf("HTML conversion failed: %w", err)
	}

	outputPath := BuildOutputPath(c.cfg.Output.BaseDir, item.ParentPath, item.FolderName, item.FileName)
	mdPath := outputPath + ".md"
	if err := os.WriteFile(mdPath, []byte(markdown), 0644); err != nil {
		return fmt.Errorf("failed to save markdown: %w", err)
	}
	log.Printf("[Saved] %s", mdPath)

	truncated := c.converter.TruncateContent(markdown, c.cfg.LLM.MaxContentLength)

	llmResp, err := c.llm.Analyze(ctx, item.URL, item.Depth, truncated, links)
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
		childPath := filepath.Join(item.ParentPath, SanitizePath(item.FolderName))
		childItems := make([]QueueItem, 0, len(llmResp.Links))
		for _, link := range llmResp.Links {
			childItems = append(childItems, QueueItem{
				URL:        link.URL,
				Depth:      item.Depth + 1,
				ParentPath: childPath,
				FolderName: link.FolderName,
				FileName:   link.FileName,
			})
		}
		c.enqueue(childItems)
	}

	time.Sleep(time.Duration(c.cfg.HTTP.Delay) * time.Millisecond)

	return nil
}
