package applicationinteraction

import domaininteraction "GCFeed/internal/domain/interaction"

// ActionResult 是点赞/收藏写入后的对外结果。
type ActionResult struct {
	VideoID       int64
	ActionType    string
	Active        bool
	LikeCount     int
	FavoriteCount int
}

// CreateCommentResult 是创建评论后的对外结果。
type CreateCommentResult struct {
	Comment      *domaininteraction.Comment
	CommentCount int
}

// DeleteCommentResult 是删除评论后的对外结果。
type DeleteCommentResult struct {
	CommentID    int64
	Status       int
	CommentCount int
}

// CommentListResult 是评论游标分页结果。
type CommentListResult struct {
	Items      []*domaininteraction.Comment
	NextCursor string
	HasMore    bool
}
