package bootstrap

import (
	applicationembedding "GCFeed/internal/application/embedding"
	infraconfig "GCFeed/internal/infra/config"
	infradatabase "GCFeed/internal/infra/database"
	infravector "GCFeed/internal/infra/embedding"
	infraembedding "GCFeed/internal/infra/persistence/embedding"
	migration "GCFeed/internal/infra/persistence/migration"
	"context"
)

// RebuildVideoEmbeddings 重算当前模型下所有已发布视频的语义向量。
func RebuildVideoEmbeddings(ctx context.Context, cfg *infraconfig.Config, batchSize int) (int, error) {
	sqlDB, err := infradatabase.New(cfg.Database)
	if err != nil {
		return 0, err
	}
	defer func() { _ = sqlDB.Close() }()

	gormDB, err := openGORM(sqlDB)
	if err != nil {
		return 0, err
	}
	if err := migration.AutoMigrate(gormDB); err != nil {
		return 0, err
	}

	repository := infraembedding.New(gormDB)
	service := applicationembedding.New(repository, infravector.NewHashNgramVectorizer())
	return service.RebuildPublishedVideos(ctx, repository, batchSize)
}
