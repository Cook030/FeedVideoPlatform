// Package kernel 提供领域层跨聚合共享的词汇与无依赖工具。
//
// kernel 不依赖任何 internal 包，只承载被多个限界上下文共同使用的
// 常量与纯函数，避免领域子包之间互相 import。
package kernel

// 用户角色词汇，账户与互动等上下文共同使用。
const (
	RoleUser  = "user"
	RoleAdmin = "admin"
)

// 观看行为事件类型，由曝光上下文产生，推荐上下文消费。
// 放在 kernel 是为了避免 domain/recommendation 直接依赖 domain/exposure。
const (
	EventTypeExposed  = "exposed"
	EventTypePlay     = "play"
	EventTypeComplete = "complete"
	EventTypeSkip     = "skip"
)

// ClampCount 把计数夹逼到非负，避免删除类操作把展示计数压到负数。
func ClampCount(value int) int {
	if value < 0 {
		return 0
	}
	return value
}
