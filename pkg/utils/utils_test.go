package utils

import (
	"testing"
)

func TestCompareVersion(t *testing.T) {
	tests := []struct {
		v1       string
		v2       string
		expected int
	}{
		{"4.14.0", "4.14.0", 0},
		{"4.14.0", "4.14.1", -1},
		{"4.14.1", "4.14.0", 1},
		{"4.14", "4.13", 1},
		{"4.14.0", "4.14", 0},
		{"v4.14.0", "4.14.0", 0}, // 应该忽略前缀
		{"", "", 0},
		{"4.14.0-rc.1", "4.14.0", 0}, // 应该只比较数字部分
	}

	for _, test := range tests {
		result := CompareVersion(test.v1, test.v2)
		if result != test.expected {
			t.Errorf("CompareVersion(%s, %s) = %d, expected %d", test.v1, test.v2, result, test.expected)
		}
	}
}

func TestExtractNetworkBase(t *testing.T) {
	tests := []struct {
		cidr     string
		expected string
	}{
		{"192.168.1.0/24", "192.168.1.0"},
		{"10.0.0.0/8", "10.0.0.0"},
		{"172.16.0.0/16", "172.16.0.0"},
		{"192.168.1.1", "192.168.1.1"}, // 无前缀
	}

	for _, test := range tests {
		result := ExtractNetworkBase(test.cidr)
		if result != test.expected {
			t.Errorf("ExtractNetworkBase(%s) = %s, expected %s", test.cidr, result, test.expected)
		}
	}
}

func TestExtractPrefixLength(t *testing.T) {
	tests := []struct {
		cidr     string
		expected int
	}{
		{"192.168.1.0/24", 24},
		{"10.0.0.0/8", 8},
		{"172.16.0.0/16", 16},
		{"192.168.1.1/32", 32},
		{"192.168.1.1", 24}, // 默认值
		{"192.168.1.0/28", 28},
	}

	for _, test := range tests {
		result := ExtractPrefixLength(test.cidr)
		if result != test.expected {
			t.Errorf("ExtractPrefixLength(%s) = %d, expected %d", test.cidr, result, test.expected)
		}
	}
}

func TestExtractGateway(t *testing.T) {
	tests := []struct {
		cidr     string
		expected string
	}{
		{"192.168.1.0/24", "192.168.1.1"},
		{"10.0.0.0/8", "10.0.0.1"},
		{"172.16.0.0/16", "172.16.0.1"},
	}

	for _, test := range tests {
		result := ExtractGateway(test.cidr)
		if result != test.expected {
			t.Errorf("ExtractGateway(%s) = %s, expected %s", test.cidr, result, test.expected)
		}
	}
}

func TestIsValidVersionFormat(t *testing.T) {
	tests := []struct {
		version  string
		expected bool
	}{
		{"4.14.0", true},
		{"4.14", true},
		{"4", false},
		{"", false},
		{"4.14.0-rc.1", true},
		{"4.14.0+build123", true},
		{"v4.14.0", false}, // 前缀应该在调用前移除
		{"abc", false},
	}

	for _, test := range tests {
		result := IsValidVersionFormat(test.version)
		if result != test.expected {
			t.Errorf("IsValidVersionFormat(%s) = %v, expected %v", test.version, result, test.expected)
		}
	}
}

func TestExtractVersionFromOutput(t *testing.T) {
	output := `
openshift-install 4.14.0
Release Image: quay.io/openshift-release-dev/ocp-release@sha256:12345
`
	result := ExtractVersionFromOutput(output, "openshift-install")
	expected := "4.14.0"
	if result != expected {
		t.Errorf("ExtractVersionFromOutput() = %s, expected %s", result, expected)
	}
}

func TestExtractSHAFromOutput(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		expected string
	}{
		{
			name: "Valid SHA with Release Image:",
			output: `
openshift-install 4.14.0
Release Image: quay.io/openshift-release-dev/ocp-release@sha256:12345
`,
			expected: "@sha256:12345",
		},
		{
			name:     "Valid SHA with release image (lowercase)",
			output:   "release image quay.io/openshift-release-dev/ocp-release@sha256:fbad931c725b2e5b937b295b58345334322bdabb0b67da1c800a53686d7397da",
			expected: "@sha256:fbad931c725b2e5b937b295b58345334322bdabb0b67da1c800a53686d7397da",
		},
		{
			name: "Real openshift-install output",
			output: `/root/ocpack/dr/downloads/bin/openshift-install 4.17.0
built from commit dfd4c085a7210e49111fa8d6747016d78f98ecf2
release image quay.io/openshift-release-dev/ocp-release@sha256:fbad931c725b2e5b937b295b58345334322bdabb0b67da1c800a53686d7397da
release architecture unknown
default architecture amd64`,
			expected: "@sha256:fbad931c725b2e5b937b295b58345334322bdabb0b67da1c800a53686d7397da",
		},
		{
			name:     "No SHA",
			output:   "Some other output",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractSHAFromOutput(tt.output)
			if result != tt.expected {
				t.Errorf("ExtractSHAFromOutput() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestExtractMajorVersion(t *testing.T) {
	tests := []struct {
		version  string
		expected string
	}{
		{"4.14.0", "4.14"},
		{"4.14", "4.14"},
		{"4", "4.14"}, // 默认值
		{"", "4.14"},  // 默认值
	}

	for _, test := range tests {
		result := ExtractMajorVersion(test.version)
		if result != test.expected {
			t.Errorf("ExtractMajorVersion(%s) = %s, expected %s", test.version, result, test.expected)
		}
	}
}

func TestSupportsOcMirror(t *testing.T) {
	tests := []struct {
		version  string
		expected bool
	}{
		{"4.14.0", true},
		{"4.14.1", true},
		{"4.13.9", false},
		{"4.15.0", true},
	}

	for _, test := range tests {
		result := SupportsOcMirror(test.version)
		if result != test.expected {
			t.Errorf("SupportsOcMirror(%s) = %v, expected %v", test.version, result, test.expected)
		}
	}
}
