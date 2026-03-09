package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/morethan/bfs_scraper/config"
	"github.com/morethan/bfs_scraper/utils"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to config YAML file")
	flag.Parse()

	seedURLs := flag.Args()
	if len(seedURLs) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: bfs_scraper -config <config.yaml> <seed_url> [seed_url...]\n")
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
