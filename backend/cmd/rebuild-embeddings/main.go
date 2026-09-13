package main

import (
	"context"
	"flag"
	"log"

	"GCFeed/internal/bootstrap"
	domainembedding "GCFeed/internal/domain/embedding"
	infraconfig "GCFeed/internal/infra/config"
)

func main() {
	configPath := flag.String("config", "./configs/config.yaml", "configuration file")
	batchSize := flag.Int("batch-size", 200, "number of videos processed per batch")
	flag.Parse()

	cfg, err := infraconfig.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("load config failed: %v", err)
	}
	rebuilt, err := bootstrap.RebuildVideoEmbeddings(context.Background(), cfg, *batchSize)
	if err != nil {
		log.Fatalf("rebuild embeddings failed after %d videos: %v", rebuilt, err)
	}
	log.Printf(
		"rebuilt %d video embeddings with model=%s dimension=%d",
		rebuilt,
		domainembedding.HashNgramModel,
		domainembedding.HashNgramDimension,
	)
}
