package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/morethan/bfs_crawler/config"
	"github.com/morethan/bfs_crawler/utils"
)

//go:embed config.example.yaml
var exampleConfigFS embed.FS

const version = "0.1.0"

const usage = `bfs_crawler — LLM-driven BFS web crawler

USAGE:
    bfs_crawler <command> [options]

COMMANDS:
    crawl   Start crawling from one or more seed URLs
    init    Write an example config file to disk

Run 'bfs_crawler <command> -help' for command-specific options.

QUICK START:
    bfs_crawler init                        # create config.yaml from template
    # edit config.yaml — set API key, model, goal
    bfs_crawler crawl https://example.com   # start crawling

VERSION:
    ` + version + `
`

const crawlUsage = `bfs_crawler crawl — crawl from one or more seed URLs

USAGE:
    bfs_crawler crawl [options] <url> [url...]

OPTIONS:
    -config <path>   Path to config YAML file (default: config.yaml)
    -help            Show this help

EXAMPLES:
    bfs_crawler crawl https://example.com/about
    bfs_crawler crawl -config my.yaml https://site.com/a https://site.com/b

Press Ctrl+C to stop gracefully (waits for the current page to finish).
`

const initUsage = `bfs_crawler init — write an example config file

USAGE:
    bfs_crawler init [options]

OPTIONS:
    -out <path>   Output path for the config file (default: config.yaml)
    -force        Overwrite the file if it already exists
    -help         Show this help

EXAMPLES:
    bfs_crawler init                  # writes config.yaml in current directory
    bfs_crawler init -out my.yaml     # writes to my.yaml
    bfs_crawler init -force           # overwrite existing config.yaml
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(1)
	}

	switch os.Args[1] {
	case "crawl":
		runCrawl(os.Args[2:])
	case "init":
		runInit(os.Args[2:])
	case "-version", "--version", "version":
		fmt.Println("bfs_crawler version", version)
	case "-help", "--help", "help", "-h":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %q\n\n%s", os.Args[1], usage)
		os.Exit(1)
	}
}

func runCrawl(args []string) {
	fs := flag.NewFlagSet("crawl", flag.ContinueOnError)
	fs.Usage = func() { fmt.Fprint(os.Stderr, crawlUsage) }
	configPath := fs.String("config", "config.yaml", "path to config YAML file")

	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	seedURLs := fs.Args()
	if len(seedURLs) == 0 {
		fmt.Fprint(os.Stderr, crawlUsage)
		os.Exit(1)
	}

	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	crawler, err := utils.NewCrawler(cfg)
	if err != nil {
		log.Fatalf("Failed to create crawler: %v", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := crawler.Run(ctx, seedURLs); err != nil {
		log.Printf("Scraping error: %v", err)
		os.Exit(1)
	}

	log.Printf("Scraping complete.")
}

func runInit(args []string) {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.Usage = func() { fmt.Fprint(os.Stderr, initUsage) }
	outPath := fs.String("out", "config.yaml", "output path for the config file")
	force := fs.Bool("force", false, "overwrite if file already exists")

	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	if !*force {
		if _, err := os.Stat(*outPath); err == nil {
			fmt.Fprintf(os.Stderr, "error: %q already exists (use -force to overwrite)\n", *outPath)
			os.Exit(1)
		}
	}

	data, err := exampleConfigFS.ReadFile("config.example.yaml")
	if err != nil {
		log.Fatalf("Failed to read embedded config template: %v", err)
	}

	if err := os.WriteFile(*outPath, data, 0644); err != nil {
		log.Fatalf("Failed to write config file: %v", err)
	}

	fmt.Printf("Config written to %q\n", *outPath)
	fmt.Println("Next steps:")
	fmt.Println("  1. Open the file and set your LLM API key, model, and goal")
	fmt.Printf("  2. Run: bfs_crawler crawl -config %s <seed-url>\n", *outPath)
}
