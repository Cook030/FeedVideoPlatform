package bootstrap

import (
	"context"
	"errors"

	applicationembedding "GCFeed/internal/application/embedding"
	applicationexposure "GCFeed/internal/application/exposure"
	applicationinteraction "GCFeed/internal/application/interaction"
	applicationrecommendation "GCFeed/internal/application/recommendation"
	applicationvideo "GCFeed/internal/application/video"
	infracache "GCFeed/internal/infra/cache"
	infraconfig "GCFeed/internal/infra/config"
	infradatabase "GCFeed/internal/infra/database"
	infravector "GCFeed/internal/infra/embedding"
	inframetrics "GCFeed/internal/infra/metrics"
	inframq "GCFeed/internal/infra/mq"
	infraembedding "GCFeed/internal/infra/persistence/embedding"
	infrafeed "GCFeed/internal/infra/persistence/feed"
	infrainteraction "GCFeed/internal/infra/persistence/interaction"
	migration "GCFeed/internal/infra/persistence/migration"
	infrarecommendation "GCFeed/internal/infra/persistence/recommendation"

	"gorm.io/gorm"
)

var errWorkerRabbitMQRequired = errors.New("rabbitmq url is required for worker")
var errWorkerRedisRequired = errors.New("redis addr is required for worker")

// StartWorkers 装配数据库、消息与缓存依赖并启动全部后台消费者。
// 依赖装配从 cmd/worker 上移到这里，与 API 侧 BuildAPI 保持同一入口风格。
func StartWorkers(ctx context.Context, cfg *infraconfig.Config) error {
	if cfg.RabbitMQ.URL == "" {
		return errWorkerRabbitMQRequired
	}
	if cfg.Redis.Addr == "" {
		return errWorkerRedisRequired
	}

	sqlDB, err := infradatabase.New(cfg.Database)
	if err != nil {
		return err
	}
	defer func() { _ = sqlDB.Close() }()

	gormDB, err := openGORM(sqlDB)
	if err != nil {
		return err
	}
	if err := migration.AutoMigrate(gormDB); err != nil {
		return err
	}

	rabbitMQ, err := inframq.NewRabbitMQ(cfg.RabbitMQ)
	if err != nil {
		return err
	}
	defer func() { _ = rabbitMQ.Close() }()

	return startWorkers(ctx, cfg, gormDB, rabbitMQ)
}

func startWorkers(ctx context.Context, cfg *infraconfig.Config, gormDB *gorm.DB, rabbitMQ *inframq.RabbitMQ) error {
	redisClient := infracache.NewRedisClient(cfg.Redis)
	feedCache := infracache.NewFeedCache(redisClient)

	// 观测端口统一在装配层注入，application 不直接依赖 infra/metrics。
	observer := inframetrics.NewAdapter()

	interactionRepo := infrainteraction.New(gormDB)
	actionWorker := applicationinteraction.NewActionWorker(interactionRepo, rabbitMQ, feedCache).WithObserver(observer)
	if err := actionWorker.Start(ctx); err != nil {
		return err
	}

	feedRepo := infrafeed.New(gormDB)
	feedPreheater := applicationvideo.NewFeedPreheater(feedRepo, feedCache)
	fanoutWorker := applicationvideo.NewFanoutWorker(feedRepo, rabbitMQ, feedCache, feedPreheater).WithObserver(observer)
	if err := fanoutWorker.Start(ctx); err != nil {
		return err
	}

	embeddingRepo := infraembedding.New(gormDB)
	embeddingService := applicationembedding.New(embeddingRepo, infravector.NewHashNgramVectorizer())
	embeddingWorker := applicationembedding.NewVideoEmbeddingWorker(embeddingService, rabbitMQ).WithObserver(observer)
	if err := embeddingWorker.Start(ctx); err != nil {
		return err
	}

	// 观看行为事件消费链路。
	interestCache := infracache.NewUserInterestCache(redisClient)
	recommendationRepo := infrarecommendation.New(gormDB, infrarecommendation.WithUserInterestCache(interestCache))
	recommendationService := applicationrecommendation.New(recommendationRepo)
	// 兴趣向量是幂等累加，重投会重复累加，因此按观看记录 ID 做事件级去重。
	eventDedupCache := infracache.NewEventDedupCache(redisClient)
	viewEventWorker := applicationexposure.NewViewEventWorker(recommendationService, rabbitMQ, eventDedupCache).WithObserver(observer)
	return viewEventWorker.Start(ctx)
}
