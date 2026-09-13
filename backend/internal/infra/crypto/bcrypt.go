// Package infracrypto 提供密码哈希的基础设施实现。
package infracrypto

import "golang.org/x/crypto/bcrypt"

// BcryptHasher 用 bcrypt 实现领域层的 PasswordHasher 端口。
type BcryptHasher struct{}

// NewBcryptHasher 创建 bcrypt 哈希器。
func NewBcryptHasher() BcryptHasher {
	return BcryptHasher{}
}

// Hash 生成 bcrypt 哈希，数据库中不会保存明文密码。
func (BcryptHasher) Hash(password string) (string, error) {
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hashed), nil
}

// Verify 校验明文密码是否匹配已保存的哈希。
func (BcryptHasher) Verify(hashed string, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hashed), []byte(password))
}
