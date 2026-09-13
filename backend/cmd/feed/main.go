package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	"GCFeed/internal/bootstrap"
	infraconfig "GCFeed/internal/infra/config"
	infradatabase "GCFeed/internal/infra/database"
	infrahttpgin "GCFeed/internal/infra/httpgin"
)

const configPath = "./configs/config.yaml"

func main() {
	// 监听中断信号，用于触发 HTTP 服务优雅退出。
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// 启动顺序保持简单：配置 -> 数据库 -> Gin -> 装配与路由 -> 启动服务。
	cfg, err := infraconfig.LoadConfig(configPath)
	if err != nil {
		log.Fatalf("load config failed: %v", err)
	}
	log.Printf(
		"config loaded: port=%d database=%s:%d/%s jwt_access_ttl=%s",
		cfg.Port,
		cfg.Database.Host,
		cfg.Database.Port,
		cfg.Database.Name,
		cfg.JWT.AccessTTL,
	)

	// 数据库连接使用 database/sql 连接池，后续会被 GORM 复用。
	db, err := infradatabase.New(cfg.Database)
	if err != nil {
		log.Fatalf("init database failed: %v", err)
	}
	log.Println("database connection initialized")

	// Gin 引擎只负责 HTTP 入口，业务依赖在 bootstrap.BuildAPI 中装配。
	g := infrahttpgin.Init()
	log.Println("gin engine initialized")

	// 依赖装配与路由注册统一由 bootstrap 完成。
	if err := bootstrap.BuildAPI(ctx, g, cfg, db); err != nil {
		log.Fatalf("init router failed: %v", err)
	}
	log.Println("router registered")

	// Run 会阻塞当前进程，直到收到退出信号或启动失败。
	log.Println("server is running")
	if err := infrahttpgin.Run(ctx, cfg, g); err != nil {
		log.Fatalf("run server failed: %v", err)
	}
	log.Println("server stopped")
}
