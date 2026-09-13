package contract

// ActionStateResult 是点赞/收藏快速写路径（Redis Watch 事务）的返回结果。
type ActionStateResult struct {
	VideoID        int64
	ActionType     string
	Active         bool
	LikeCount      int
	FavoriteCount  int
	Delta          int
	IdempotencyKey string
}
