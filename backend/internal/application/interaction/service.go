package applicationinteraction

import (
	"errors"

	domainfeed "GCFeed/internal/domain/feed"
	domaininteraction "GCFeed/internal/domain/interaction"
)

const defaultCommentLimit = 20

// 互动热度增量权重统一引用 domain/feed 的权威定义，避免与热榜排序口径分叉。
const hotScoreLikeWeight = domainfeed.HotScoreLikeWeight
const hotScoreFavoriteWeight = domainfeed.HotScoreFavoriteWeight
const hotScoreCommentWeight = domainfeed.HotScoreCommentWeight

var ErrLoadInteractionFailed = errors.New("failed to load interaction")
var ErrSaveInteractionFailed = errors.New("failed to save interaction")
var ErrUpdateInteractionFailed = errors.New("failed to update interaction")

// Service 编排点赞、收藏与评论的读写，并通过可选端口触发缓存、事件与通知副作用。
type Service struct {
	repo             domaininteraction.Repository
	hotScoreRecorder HotScoreRecorder
	statCache        StatCache
	actionStateStore ActionStateStore
	actionPublisher  ActionEventPublisher
	messageWriter    MessageWriter
}

type Option func(*Service)

func New(repo domaininteraction.Repository, options ...Option) *Service {
	service := &Service{repo: repo}
	for _, option := range options {
		option(service)
	}
	return service
}

// WithHotScoreRecorder 为互动服务启用热榜增量写入。
func WithHotScoreRecorder(recorder HotScoreRecorder) Option {
	return func(s *Service) {
		s.hotScoreRecorder = recorder
	}
}

// WithStatCache 为评论写入后的 Feed 计数展示启用缓存同步。
func WithStatCache(cache StatCache) Option {
	return func(s *Service) {
		s.statCache = cache
	}
}

// WithAsyncActionPipeline 为点赞收藏启用 Redis 快速写和 MQ 异步落库。
func WithAsyncActionPipeline(store ActionStateStore, publisher ActionEventPublisher) Option {
	return func(s *Service) {
		s.actionStateStore = store
		s.actionPublisher = publisher
	}
}

// WithMessageWriter 为点赞和评论成功后的通知写入启用消息中心。
func WithMessageWriter(writer MessageWriter) Option {
	return func(s *Service) {
		s.messageWriter = writer
	}
}
