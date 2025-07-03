package utils

import (
	"fmt"
	"strings"
)

// ExtractNetworkBase 提取网络基地址
func ExtractNetworkBase(cidr string) string {
	parts := strings.Split(cidr, "/")
	if len(parts) > 0 {
		return parts[0]
	}
	return cidr
}

// ExtractPrefixLength 提取前缀长度
func ExtractPrefixLength(cidr string) int {
	parts := strings.Split(cidr, "/")
	if len(parts) == 2 {
		switch parts[1] {
		case "8":
			return 8
		case "16":
			return 16
		case "24":
			return 24
		case "32":
			return 32
		default:
			// 尝试转换为整数
			var prefix int
			if _, err := fmt.Sscanf(parts[1], "%d", &prefix); err == nil {
				return prefix
			}
			return 24 // 默认值
		}
	}
	return 24 // 默认值
}

// ExtractGateway 提取网关地址（假设是网络的第一个地址）
func ExtractGateway(cidr string) string {
	networkBase := ExtractNetworkBase(cidr)
	parts := strings.Split(networkBase, ".")
	if len(parts) == 4 {
		// 假设网关是 .1
		return fmt.Sprintf("%s.%s.%s.1", parts[0], parts[1], parts[2])
	}
	return networkBase
}

// GetNetworkClass 获取网络类别 (A, B, C)
func GetNetworkClass(cidr string) string {
	ip := ExtractNetworkBase(cidr)
	parts := strings.Split(ip, ".")
	if len(parts) != 4 {
		return "Unknown"
	}
	
	var firstOctet int
	if _, err := fmt.Sscanf(parts[0], "%d", &firstOctet); err != nil {
		return "Unknown"
	}
	
	if firstOctet >= 1 && firstOctet <= 126 {
		return "A"
	} else if firstOctet >= 128 && firstOctet <= 191 {
		return "B"
	} else if firstOctet >= 192 && firstOctet <= 223 {
		return "C"
	} else if firstOctet >= 224 && firstOctet <= 239 {
		return "D"
	} else if firstOctet >= 240 && firstOctet <= 255 {
		return "E"
	}
	
	return "Unknown"
} 