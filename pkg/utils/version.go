package utils

import (
	"fmt"
	"strconv"
	"strings"
)

// CompareVersion 比较两个版本号
// 返回值: -1 表示 v1 < v2, 0 表示 v1 = v2, 1 表示 v1 > v2
func CompareVersion(v1, v2 string) int {
	// 移除可能的前缀
	v1 = strings.TrimPrefix(v1, "v")
	v2 = strings.TrimPrefix(v2, "v")

	parts1 := ParseVersion(v1)
	parts2 := ParseVersion(v2)

	maxLen := len(parts1)
	if len(parts2) > maxLen {
		maxLen = len(parts2)
	}

	for i := 0; i < maxLen; i++ {
		p1, p2 := 0, 0
		if i < len(parts1) {
			p1 = parts1[i]
		}
		if i < len(parts2) {
			p2 = parts2[i]
		}

		if p1 != p2 {
			if p1 < p2 {
				return -1
			}
			return 1
		}
	}

	return 0
}

// ParseVersion 解析版本号为整数数组
func ParseVersion(version string) []int {
	if version == "" {
		return []int{0}
	}

	// 如果版本号包含后缀（如 -rc.1），只保留主要版本部分
	if idx := strings.IndexAny(version, "-+"); idx != -1 {
		version = version[:idx]
	}

	parts := strings.Split(version, ".")
	result := make([]int, 0, len(parts))

	for _, part := range parts {
		if part == "" {
			continue
		}

		// 提取数字部分
		var numStr strings.Builder
		for _, char := range part {
			if char >= '0' && char <= '9' {
				numStr.WriteRune(char)
			} else {
				break
			}
		}

		if numStr.Len() > 0 {
			if num, err := strconv.Atoi(numStr.String()); err == nil {
				result = append(result, num)
			}
		}
	}

	if len(result) == 0 {
		return []int{0}
	}

	return result
}

// ExtractVersionFromOutput 从命令输出中提取版本号
func ExtractVersionFromOutput(output, prefix string) string {
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		// 支持不区分大小写的前缀匹配
		lowerLine := strings.ToLower(line)
		lowerPrefix := strings.ToLower(prefix)
		if strings.HasPrefix(lowerLine, lowerPrefix) {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				// 提取版本号，去掉可能的前缀
				version := parts[1]
				// 如果版本号包含 "v" 前缀，去掉它
				if strings.HasPrefix(version, "v") {
					version = version[1:]
				}
				// 验证提取的版本号格式
				if IsValidVersionFormat(version) {
					return version
				}
			}
		}
	}
	return ""
}

// ExtractSHAFromOutput 从命令输出中提取 SHA 值
func ExtractSHAFromOutput(output string) string {
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		// 查找包含 "release image" 和 "@sha" 的行（不区分大小写）
		lowerLine := strings.ToLower(line)
		if strings.Contains(lowerLine, "release image") && strings.Contains(line, "@sha") {
			// 提取 @sha256:... 部分
			shaIndex := strings.Index(line, "@sha")
			if shaIndex != -1 {
				return line[shaIndex:]
			}
		}
	}
	return ""
}

// ExtractVersionWithRegex 使用简单模式匹配从输出中提取版本号
func ExtractVersionWithRegex(output string) string {
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		// 查找包含版本号的行
		if strings.Contains(line, "4.") {
			// 提取版本号模式 x.y.z
			parts := strings.Fields(line)
			for _, part := range parts {
				// 移除可能的前缀
				part = strings.TrimPrefix(part, "v")
				// 检查是否匹配版本号格式
				if IsValidVersionFormat(part) {
					return part
				}
			}
		}
	}
	return ""
}

// IsValidVersionFormat 检查字符串是否为有效的版本号格式
func IsValidVersionFormat(version string) bool {
	if version == "" {
		return false
	}

	parts := strings.Split(version, ".")
	if len(parts) < 2 {
		return false
	}

	// 检查每个部分是否为数字
	for _, part := range parts {
		if part == "" {
			continue
		}
		// 检查是否包含数字
		hasDigit := false
		for _, char := range part {
			if char >= '0' && char <= '9' {
				hasDigit = true
			} else if char != '.' && char != '-' && char != '+' {
				// 如果包含其他字符，只允许在末尾
				break
			}
		}
		if !hasDigit {
			return false
		}
	}

	return true
}

// ExtractMajorVersion 提取版本号的主要部分 (例如 "4.14.0" -> "4.14")
func ExtractMajorVersion(version string) string {
	parts := strings.Split(version, ".")
	if len(parts) >= 2 {
		return parts[0] + "." + parts[1]
	}
	return "4.14" // 默认值
}

// SupportsOcMirror 检查版本是否支持 oc-mirror 工具
func SupportsOcMirror(version string) bool {
	return CompareVersion(version, "4.14.0") >= 0
}

// ParseTimestamp 解析时间戳字符串为 int64
func ParseTimestamp(timestamp string) (int64, error) {
	// 尝试解析为整数时间戳
	if timeValue, err := strconv.ParseInt(timestamp, 10, 64); err == nil {
		return timeValue, nil
	}

	// 如果不是纯数字，返回错误
	return 0, fmt.Errorf("无效的时间戳格式: %s", timestamp)
}
