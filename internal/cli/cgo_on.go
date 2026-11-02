//go:build cgo

package cli

// isCgo 报告构建是否启用 cgo。
func isCgo() bool { return true }
