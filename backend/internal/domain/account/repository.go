package domainaccount

import "context"

// Repository 定义账号领域需要的持久化能力，应用层只依赖这个接口。
type Repository interface {
	// Save 保存新用户，账号重复时返回 ErrAccountAlreadyExists。
	Save(ctx context.Context, user *User) error
	// FindByAccount 用于登录流程通过账号查找用户。
	FindByAccount(ctx context.Context, account string) (*User, error)
	// FindByID 用于根据登录态读取当前用户。
	FindByID(ctx context.Context, id int64) (*User, error)
	// UpdateProfile 只更新用户展示资料字段。
	UpdateProfile(ctx context.Context, user *User) error
}

// PasswordHasher 提供密码哈希与校验能力。
//
// 领域层只依赖该抽象，具体算法（bcrypt）由基础设施层实现并注入，
// 使领域实体不再编译期绑定某个加密库。
type PasswordHasher interface {
	// Hash 生成密码哈希。
	Hash(password string) (string, error)
	// Verify 校验明文密码是否匹配已保存的哈希。
	Verify(hashed string, password string) error
}
