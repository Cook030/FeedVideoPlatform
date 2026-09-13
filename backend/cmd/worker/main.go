package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	"GCFeed/internal/bootstrap"
	infraconfig "GCFeed/internal/infra/config"
	inframetrics "GCFeed/internal/infra/metrics"
)

const configPath = "./configs/config.yaml"

func main() {
	cfg, err := infraconfig.LoadConfig(configPath)
	if err != nil {
		log.Fatalf("load config failed: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		if err := inframetrics.RunServer(ctx, ":9091"); err != nil {
			log.Printf("metrics server failed: %v", err)
		}
	}()

	// 数据库、消息与缓存依赖的装配统一由 bootstrap 完成，与 API 侧同源。
	if err := bootstrap.StartWorkers(ctx, cfg); err != nil {
		log.Fatalf("start workers failed: %v", err)
	}
	log.Println("gcfeed worker is running")
	<-ctx.Done()
	log.Println("gcfeed worker stopped")
}
