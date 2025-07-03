package utils

import (
	"strings"
)

// JoinStringSlice 连接字符串切片
func JoinStringSlice(slice []string, sep string) string {
	return strings.Join(slice, sep)
}

// SplitString 分割字符串
func SplitString(s, sep string) []string {
	return strings.Split(s, sep)
}

// ContainsString 检查字符串是否包含子串
func ContainsString(s, substr string) bool {
	return strings.Contains(s, substr)
}

// TrimString 去除字符串两端的空白字符
func TrimString(s string) string {
	return strings.TrimSpace(s)
}

// ReplaceString 替换字符串中的子串
func ReplaceString(s, old, new string) string {
	return strings.Replace(s, old, new, -1)
} 