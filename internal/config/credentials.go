// Package config 负责内置 Telegram 应用凭据及其覆盖规则。
package config

import (
	"os"
	"strconv"
)

// 项目公开内置的 Telegram 应用凭据（编译进二进制，公开仓库同样可见）。
//
// 这是应用标识，不是账号密钥；用户可用环境变量、配置文件、
// 命令行 flag 或 Web UI 直接替换。
const (
	BuiltinAPIID   = 34353880
	BuiltinAPIHash = "35edeb69a4182e939b3ed765cd5524be"
)

// Credentials 是一组 Telegram 应用凭据。
type Credentials struct {
	APIID   int    `json:"api_id"`
	APIHash string `json:"api_hash"`
	Source  string `json:"source"`
}

// BuiltinCredentials 返回项目内置凭据。
func BuiltinCredentials() Credentials {
	return Credentials{APIID: BuiltinAPIID, APIHash: BuiltinAPIHash, Source: "builtin"}
}

// EnvCredentials 从环境变量读取 API 凭据（TELEGRAM_API_ID / TELEGRAM_API_HASH）。
// 返回 ok=false 表示环境变量不完整。
func EnvCredentials() (id int, hash string, ok bool) {
	idStr := os.Getenv("TELEGRAM_API_ID")
	hash = os.Getenv("TELEGRAM_API_HASH")
	if idStr == "" || hash == "" {
		return 0, "", false
	}
	n, err := strconv.Atoi(idStr)
	if err != nil || n <= 0 {
		return 0, "", false
	}
	return n, hash, true
}

// EnvCredentialPair 返回带来源标记的环境变量凭据。
func EnvCredentialPair() (Credentials, bool) {
	id, hash, ok := EnvCredentials()
	if !ok {
		return Credentials{}, false
	}
	return Credentials{APIID: id, APIHash: hash, Source: "env"}, true
}

// ResolveCredentials 按优先级解析凭据：
//  1. 环境变量 TELEGRAM_API_ID / TELEGRAM_API_HASH
//  2. 调用方显式提供的 api_id / api_hash
//  3. 项目公开内置的默认凭据
//
// 允许只覆盖其中一项，另一项自动取内置值。
func ResolveCredentials(apiID int, apiHash string) Credentials {
	if env, ok := EnvCredentialPair(); ok {
		return env
	}
	if apiID > 0 && apiHash != "" {
		return Credentials{APIID: apiID, APIHash: apiHash, Source: "configured"}
	}
	if apiID > 0 {
		return Credentials{APIID: apiID, APIHash: BuiltinAPIHash, Source: "configured"}
	}
	if apiHash != "" {
		return Credentials{APIID: BuiltinAPIID, APIHash: apiHash, Source: "configured"}
	}
	return BuiltinCredentials()
}

// IsBuiltinCredentials 判断某组凭据是否为内置默认值。
func IsBuiltinCredentials(id int, hash string) bool {
	return id == BuiltinAPIID && hash == BuiltinAPIHash
}

// ApplyCredentials 把「运行时实际生效」的凭据应用到账号，
// 包含环境变量覆盖；返回是否使用了内置默认值。
func ApplyCredentials(acc *Account) bool {
	if acc == nil {
		return false
	}
	cred := ResolveCredentials(acc.APIID, acc.APIHash)
	acc.APIID = cred.APIID
	acc.APIHash = cred.APIHash
	return cred.Source == "builtin"
}

// FillBuiltinCredentials 只补空值，不把环境变量写进持久化配置。
// 环境变量是临时覆盖：用户撤销后配置应自动回到内置/自定义值。
func FillBuiltinCredentials(acc *Account) {
	if acc == nil {
		return
	}
	if acc.APIID <= 0 {
		acc.APIID = BuiltinAPIID
	}
	if acc.APIHash == "" {
		acc.APIHash = BuiltinAPIHash
	}
}
