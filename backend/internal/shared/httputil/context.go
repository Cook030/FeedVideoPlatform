// Package httputil 提供表现层共用的请求上下文读取与参数解析工具。
//
// 它不属于四层中的任何一层，是并列于四层的通用辅助目录：
// 只依赖 gin，不依赖 domain/application/infra/interfaces。
package httputil

import "github.com/gin-gonic/gin"

// 鉴权中间件写入 gin.Context 的键名。
// 定义在此处是为了让中间件与各 handler 双方都只依赖 shared/httputil，保持依赖单向。
const (
	ContextUserIDKey         = "auth_user_id"
	ContextRoleKey           = "auth_role"
	ContextTokenExpiresAtKey = "auth_token_expires_at"
)

// UserIDFromContext 读取当前登录用户 ID；未登录或值非法时返回 (0,false)。
func UserIDFromContext(c *gin.Context) (int64, bool) {
	value, exists := c.Get(ContextUserIDKey)
	if !exists {
		return 0, false
	}
	userID, ok := value.(int64)
	return userID, ok && userID > 0
}

// ViewerIDFromContext 读取可选登录身份（Feed 个性化场景），语义与 UserIDFromContext 一致。
func ViewerIDFromContext(c *gin.Context) (int64, bool) {
	return UserIDFromContext(c)
}

// RoleFromContext 读取当前登录用户的角色，未登录返回空串。
func RoleFromContext(c *gin.Context) string {
	value, exists := c.Get(ContextRoleKey)
	if !exists {
		return ""
	}
	role, _ := value.(string)
	return role
}
