package interfaceshttprouter

import (
	interfaceshttpaccount "GCFeed/internal/interfaces/http/account"
	interfaceshttpexposure "GCFeed/internal/interfaces/http/exposure"
	interfaceshttpfeed "GCFeed/internal/interfaces/http/feed"
	interfaceshttpinteraction "GCFeed/internal/interfaces/http/interaction"
	interfaceshttpmessage "GCFeed/internal/interfaces/http/message"
	interfaceshttpmiddleware "GCFeed/internal/interfaces/http/middleware"
	interfaceshttpplayback "GCFeed/internal/interfaces/http/playback"
	interfaceshttprecommendation "GCFeed/internal/interfaces/http/recommendation"
	interfaceshttprelation "GCFeed/internal/interfaces/http/relation"
	interfaceshttpupload "GCFeed/internal/interfaces/http/upload"
	interfaceshttpvideo "GCFeed/internal/interfaces/http/video"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Deps 汇总路由注册所需的 Handler 与中间件。
// 依赖装配由 internal/bootstrap 完成；本包不感知任何 infra 实现。
type Deps struct {
	Account        *interfaceshttpaccount.Handler
	Feed           *interfaceshttpfeed.Handler
	Video          *interfaceshttpvideo.Handler
	Interaction    *interfaceshttpinteraction.Handler
	Relation       *interfaceshttprelation.Handler
	Message        *interfaceshttpmessage.Handler
	MessageStream  *interfaceshttpmessage.StreamHandler
	Exposure       *interfaceshttpexposure.Handler
	Playback       *interfaceshttpplayback.Handler
	Recommendation *interfaceshttprecommendation.Handler
	Upload         *interfaceshttpupload.Handler

	JWTAuth         gin.HandlerFunc
	OptionalJWTAuth gin.HandlerFunc
	InternalToken   string
}

// Register 只做路由注册：路径、方法与中间件顺序与此前版本逐条保持一致。
func Register(g *gin.Engine, deps Deps) {
	// 局部别名让路由注册代码保持原样，避免逐行替换引入偏差。
	accountHandler := deps.Account
	feedHandler := deps.Feed
	videoHandler := deps.Video
	interactionHandler := deps.Interaction
	relationHandler := deps.Relation
	messageHandler := deps.Message
	messageStreamHandler := deps.MessageStream
	exposureHandler := deps.Exposure
	playbackHandler := deps.Playback
	recommendationHandler := deps.Recommendation
	uploadHandler := deps.Upload
	authMiddleware := deps.JWTAuth
	optionalAuthMiddleware := deps.OptionalJWTAuth

	g.GET("/health", HealthCheck)
	g.GET("/metrics", gin.WrapH(promhttp.Handler()))
	// 静态文件路由让上传后的文件可以通过 /uploads/... 访问。
	g.Static("/uploads", "./uploads")

	api := g.Group("/api")

	// RESTful 路由约定：路径表达资源，HTTP 方法表达动作。
	// 会话资源用于登录态：创建会话表示登录，删除当前会话表示登出。
	// 登录与注册对匿名请求开放且成本较高，按 IP 限流以减少爆破和灌水。
	sessions := api.Group("/sessions")
	sessions.POST("", interfaceshttpmiddleware.NewLoginRateLimit(), accountHandler.Login)
	sessions.DELETE("/current", authMiddleware, accountHandler.Logout)

	// 用户资源承载注册、当前用户资料和用户作品列表。
	users := api.Group("/users")
	users.POST("", interfaceshttpmiddleware.NewRegisterRateLimit(), accountHandler.Register)
	users.GET("/me", authMiddleware, accountHandler.Me)
	users.PATCH("/me", authMiddleware, accountHandler.UpdateMe)
	users.GET("/me/videos", authMiddleware, videoHandler.ListMine)
	users.PUT("/me/following/:targetUserId", authMiddleware, relationHandler.Follow)
	users.DELETE("/me/following/:targetUserId", authMiddleware, relationHandler.Unfollow)
	users.GET("/me/following", authMiddleware, relationHandler.ListFollowing)
	users.GET("/me/followers", authMiddleware, relationHandler.ListFollowers)
	users.GET("/:userId", accountHandler.Get)
	users.GET("/:userId/videos", videoHandler.ListByAuthor)

	// 视频是互动资源的父资源，点赞、收藏和评论都挂在具体视频下。
	videos := api.Group("/videos")
	videos.POST("", authMiddleware, videoHandler.Create)
	videos.GET("/:videoId", videoHandler.Get)
	videos.DELETE("/:videoId", authMiddleware, videoHandler.Delete)
	videos.PUT("/:videoId/like", authMiddleware, interactionHandler.Like)
	videos.DELETE("/:videoId/like", authMiddleware, interactionHandler.Unlike)
	videos.PUT("/:videoId/favorite", authMiddleware, interactionHandler.Favorite)
	videos.DELETE("/:videoId/favorite", authMiddleware, interactionHandler.Unfavorite)
	videos.POST("/:videoId/comments", authMiddleware, interactionHandler.CreateComment)
	videos.GET("/:videoId/comments", interactionHandler.ListComments)

	uploads := api.Group("/uploads", authMiddleware)
	uploads.POST("", uploadHandler.Create)

	// Feed 暴露为条目集合，客户端通过游标和 limit 控制分页。
	api.GET("/feed-items", optionalAuthMiddleware, feedHandler.ListFeedItems)
	api.POST("/feed-queries", optionalAuthMiddleware, feedHandler.Query)
	api.POST("/video-view-events", authMiddleware, exposureHandler.CreateViewEvent)
	// 删除评论只需要评论自身 ID，所以放在顶层 comments 资源下。
	api.DELETE("/comments/:commentId", authMiddleware, interactionHandler.DeleteComment)
	api.GET("/messages", authMiddleware, messageHandler.List)
	api.POST("/messages/stream-ticket", authMiddleware, messageStreamHandler.Ticket)
	api.GET("/messages/stream", messageStreamHandler.Stream)
	api.PATCH("/messages", authMiddleware, messageHandler.MarkRead)
	api.GET("/message-stats/unread", authMiddleware, messageHandler.CountUnread)
	api.GET("/playback-config", authMiddleware, playbackHandler.GetConfig)
	api.GET("/preload-videos", authMiddleware, playbackHandler.ListPreloadVideos)
	api.POST("/playback-qos-reports", authMiddleware, playbackHandler.CreateQoSReport)

	internal := g.Group("/internal")
	internal.POST("/recommendation-candidates", interfaceshttpmiddleware.NewInternalTokenAuth(deps.InternalToken), recommendationHandler.ListCandidates)
	internal.POST("/exposure-decisions", interfaceshttpmiddleware.NewInternalTokenAuth(deps.InternalToken), recommendationHandler.DecideExposures)
	internal.POST("/exposures", interfaceshttpmiddleware.NewInternalTokenAuth(deps.InternalToken), recommendationHandler.SaveExposures)
	internal.POST("/messages", interfaceshttpmiddleware.NewInternalTokenAuth(deps.InternalToken), messageHandler.Create)
	internal.POST("/playback-qos-reports", interfaceshttpmiddleware.NewInternalTokenAuth(deps.InternalToken), playbackHandler.CreateInternalQoSReport)
}

// HealthCheck 提供基础健康检查接口，方便本地调试和容器探活。
func HealthCheck(c *gin.Context) {
	c.JSON(200, gin.H{
		"message": "All is well",
	})
}
