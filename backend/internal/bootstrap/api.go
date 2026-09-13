package bootstrap

import (
	"context"
	"database/sql"
	"log"

	applicationaccount "GCFeed/internal/application/account"
	applicationexposure "GCFeed/internal/application/exposure"
	applicationfeed "GCFeed/internal/application/feed"
	applicationinteraction "GCFeed/internal/application/interaction"
	applicationmessage "GCFeed/internal/application/message"
	applicationplayback "GCFeed/internal/application/playback"
	applicationrecommendation "GCFeed/internal/application/recommendation"
	applicationrelation "GCFeed/internal/application/relation"
	applicationupload "GCFeed/internal/application/upload"
	applicationvideo "GCFeed/internal/application/video"
	domainembedding "GCFeed/internal/domain/embedding"
	infracache "GCFeed/internal/infra/cache"
	infraconfig "GCFeed/internal/infra/config"
	infracrypto "GCFeed/internal/infra/crypto"
	infrajwt "GCFeed/internal/infra/jwt"
	inframedia "GCFeed/internal/infra/media"
	inframetrics "GCFeed/internal/infra/metrics"
	inframq "GCFeed/internal/infra/mq"
	infraaccount "GCFeed/internal/infra/persistence/account"
	infraexposure "GCFeed/internal/infra/persistence/exposure"
	infrafeed "GCFeed/internal/infra/persistence/feed"
	infrainteraction "GCFeed/internal/infra/persistence/interaction"
	inframessage "GCFeed/internal/infra/persistence/message"
	migration "GCFeed/internal/infra/persistence/migration"
	infraplayback "GCFeed/internal/infra/persistence/playback"
	infrarecommendation "GCFeed/internal/infra/persistence/recommendation"
	infrarelation "GCFeed/internal/infra/persistence/relation"
	infravideo "GCFeed/internal/infra/persistence/video"
	inforealtime "GCFeed/internal/infra/realtime"
	infrastorage "GCFeed/internal/infra/storage"
	interfaceshttpaccount "GCFeed/internal/interfaces/http/account"
	interfaceshttpexposure "GCFeed/internal/interfaces/http/exposure"
	interfaceshttpfeed "GCFeed/internal/interfaces/http/feed"
	interfaceshttpinteraction "GCFeed/internal/interfaces/http/interaction"
	interfaceshttpmessage "GCFeed/internal/interfaces/http/message"
	interfaceshttpmiddleware "GCFeed/internal/interfaces/http/middleware"
	interfaceshttpplayback "GCFeed/internal/interfaces/http/playback"
	interfaceshttprecommendation "GCFeed/internal/interfaces/http/recommendation"
	interfaceshttprelation "GCFeed/internal/interfaces/http/relation"
	interfaceshttprouter "GCFeed/internal/interfaces/http/router"
	interfaceshttpupload "GCFeed/internal/interfaces/http/upload"
	interfaceshttpvideo "GCFeed/internal/interfaces/http/video"

	"github.com/gin-gonic/gin"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// openGORM 复用已有的 database/sql 连接池初始化 GORM，避免维护两套数据库连接。
func openGORM(db *sql.DB) (*gorm.DB, error) {
	return gorm.Open(gormmysql.New(gormmysql.Config{Conn: db}), &gorm.Config{})
}

// BuildAPI 完成 API 进程的全部依赖装配并注册路由。
//
// 这是全项目唯一允许同时引用 infra 实现与 application/interfaces 的位置
// （与 cmd/ 同级），表现层因此不需要感知任何基础设施细节。
func BuildAPI(ctx context.Context, g *gin.Engine, cfg *infraconfig.Config, db *sql.DB) error {
	gormDB, err := openGORM(db)
	if err != nil {
		return err
	}

	// AutoMigrate 根据模型创建或补齐表结构，适合教学项目快速启动。
	if err := migration.AutoMigrate(gormDB); err != nil {
		return err
	}

	// JWT Manager 同时被账号服务用于签发 token，也被鉴权中间件用于校验 token。
	jwtManager, err := infrajwt.NewManager(cfg.JWT.Secret, cfg.JWT.AccessTTL)
	if err != nil {
		return err
	}

	// 下面按领域模块组装依赖：Repository -> Service -> Handler。
	videoRepo := infravideo.New(gormDB)
	feedRepo := infrafeed.New(gormDB)
	var interestCache *infracache.UserInterestCache
	feedOptions := []applicationfeed.Option{}
	videoOptions := []applicationvideo.Option{}
	interactionOptions := []applicationinteraction.Option{}
	exposureOptions := []applicationexposure.Option{}
	messageOptions := []applicationmessage.Option{}
	var feedCache *infracache.FeedCache
	var rabbitMQ *inframq.RabbitMQ
	var messageStream *inforealtime.MessageStream
	if cfg.Redis.Addr != "" {
		redisClient := infracache.NewRedisClient(cfg.Redis)
		feedCache = infracache.NewFeedCache(redisClient)
		interestCache = infracache.NewUserInterestCache(redisClient, domainembedding.HashNgramModel, domainembedding.HashNgramDimension)
		// 消息实时通道使用独立 Redis 客户端做 Pub/Sub 扇出，支持 API 多实例。
		messageStream = inforealtime.NewMessageStream(cfg.Redis)
		feedOptions = append(feedOptions, applicationfeed.WithFeedCache(feedCache))
		messageOptions = append(messageOptions, applicationmessage.WithNotifier(messageStream))
		interactionOptions = append(interactionOptions, applicationinteraction.WithHotScoreRecorder(feedCache))
		interactionOptions = append(interactionOptions, applicationinteraction.WithStatCache(feedCache))
	}
	recommendationOptions := []infrarecommendation.Option{
		infrarecommendation.WithEmbeddingSpec(domainembedding.HashNgramModel, domainembedding.HashNgramDimension),
	}
	if interestCache != nil {
		recommendationOptions = append(recommendationOptions, infrarecommendation.WithUserInterestCache(interestCache))
	}
	recommendationRepo := infrarecommendation.New(gormDB, recommendationOptions...)
	recommendationService := applicationrecommendation.New(recommendationRepo)
	recommendationHandler := interfaceshttprecommendation.New(recommendationService)
	feedOptions = append(feedOptions, applicationfeed.WithRecommender(recommendationService))
	accountRepo := infraaccount.New(gormDB)
	if feedCache != nil {
		// 资料变更后失效卡片缓存；视频删除后失效对应卡片缓存。
		videoOptions = append(videoOptions, applicationvideo.WithCardInvalidator(feedCache))
	}
	accountOptions := []applicationaccount.Option{}
	if feedCache != nil {
		accountOptions = append(accountOptions, applicationaccount.WithCardInvalidator(feedCache))
	}
	accountService := applicationaccount.New(accountRepo, jwtManager, infracrypto.NewBcryptHasher(), accountOptions...)
	accountHandler := interfaceshttpaccount.New(accountService)
	// 观测端口在装配层注入，application 不直接依赖 infra/metrics。
	feedOptions = append(feedOptions, applicationfeed.WithObserver(inframetrics.NewAdapter()))
	feedService := applicationfeed.New(feedRepo, feedOptions...)
	feedHandler := interfaceshttpfeed.New(feedService)
	interactionRepo := infrainteraction.New(gormDB)
	messageRepo := inframessage.New(gormDB)
	messageService := applicationmessage.New(messageRepo, messageOptions...)
	messageHandler := interfaceshttpmessage.New(messageService)
	messageHub := inforealtime.NewHub()
	messageStreamHandler := interfaceshttpmessage.NewStreamHandler(messageService, messageHub, messageStream)
	if messageStream != nil {
		// 订阅全部用户频道，把事件投递给本实例持有的 SSE 连接。
		go func() {
			if err := messageStream.Run(ctx, messageHub.OnEvent); err != nil && ctx.Err() == nil {
				log.Printf("message stream stopped: %v", err)
			}
		}()
		// 进程退出时主动关闭 SSE 连接，让客户端尽快重连到其它实例。
		go func() {
			<-ctx.Done()
			messageHub.Close()
		}()
	}
	playbackRepo := infraplayback.New(gormDB)
	playbackService := applicationplayback.New(playbackRepo)
	playbackHandler := interfaceshttpplayback.New(playbackService)
	if cfg.RabbitMQ.URL != "" {
		rabbitMQ, err = inframq.NewRabbitMQ(cfg.RabbitMQ)
		if err != nil {
			log.Printf("rabbitmq disabled: %v", err)
		} else {
			videoOptions = append(videoOptions, applicationvideo.WithPublishedEventPublisher(rabbitMQ))
			exposureOptions = append(exposureOptions, applicationexposure.WithViewEventPublisher(rabbitMQ))
			if feedCache != nil {
				interactionOptions = append(interactionOptions, applicationinteraction.WithAsyncActionPipeline(feedCache, rabbitMQ))
			}
		}
	}
	messageWriter := NewMessageWriter(messageService)
	interactionOptions = append(interactionOptions, applicationinteraction.WithMessageWriter(messageWriter))
	relationOptions := []applicationrelation.Option{applicationrelation.WithMessageWriter(messageWriter)}
	videoService := applicationvideo.New(videoRepo, videoOptions...)
	videoHandler := interfaceshttpvideo.New(videoService)
	interactionService := applicationinteraction.New(interactionRepo, interactionOptions...)
	interactionHandler := interfaceshttpinteraction.New(interactionService)
	exposureRepo := infraexposure.New(gormDB)
	exposureService := applicationexposure.New(exposureRepo, exposureOptions...)
	exposureHandler := interfaceshttpexposure.New(exposureService)
	relationRepo := infrarelation.New(gormDB)
	if feedCache != nil {
		relationOptions = append(relationOptions, applicationrelation.WithFollowFeedBackfiller(NewFollowFeedBackfiller(feedRepo, feedCache)))
	}
	relationService := applicationrelation.New(relationRepo, relationOptions...)
	relationHandler := interfaceshttprelation.New(relationService)
	uploadService := applicationupload.New(infrastorage.New("./uploads"), inframedia.NewFFmpegProcessor())
	uploadHandler := interfaceshttpupload.New(uploadService)

	interfaceshttprouter.Register(g, interfaceshttprouter.Deps{
		Account:         accountHandler,
		Feed:            feedHandler,
		Video:           videoHandler,
		Interaction:     interactionHandler,
		Relation:        relationHandler,
		Message:         messageHandler,
		MessageStream:   messageStreamHandler,
		Exposure:        exposureHandler,
		Playback:        playbackHandler,
		Recommendation:  recommendationHandler,
		Upload:          uploadHandler,
		JWTAuth:         interfaceshttpmiddleware.NewJWTAuth(jwtManager),
		OptionalJWTAuth: interfaceshttpmiddleware.NewOptionalJWTAuth(jwtManager),
		InternalToken:   cfg.Internal.Token,
	})
	return nil
}
