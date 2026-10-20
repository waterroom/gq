//go:build windows

package main

import "testing"

// The HRESULT mapping is the whole point of the window diagnostic: a wrong
// translation sends people looking in the wrong place. Pin the strings.
func TestDescribeHRESULT(t *testing.T) {
	cases := []struct{ in, want string }{
		{"80070490", "未安装 WebView2 运行时（0x80070490）"},
		{"80070002", "WebView2 运行时文件找不到（0x80070002），安装可能不完整"},
		{"80070005", "数据目录不可写，拒绝访问（0x80070005）"},
		{"80040154", "WebView2 组件未注册（0x80040154）"},
		{"8007139f", "WebView2 运行时版本不匹配（0x8007139F）"},
		{"8007139F", "WebView2 运行时版本不匹配（0x8007139F）"},
		{"80004005", "WebView2 初始化失败（0x80004005）"},
		{"", "WebView2 初始化失败"},
	}
	for _, c := range cases {
		if got := describeHRESULT(c.in); got != c.want {
			t.Errorf("describeHRESULT(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestFirstLine(t *testing.T) {
	cases := []struct{ in, want string }{
		{"  Result: 80070002\r\n2026/10/20 next", "Result: 80070002"},
		{"single", "single"},
		{"", ""},
	}
	for _, c := range cases {
		if got := firstLine(c.in); got != c.want {
			t.Errorf("firstLine(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestFirstLineTruncates(t *testing.T) {
	long := make([]byte, 300)
	for i := range long {
		long[i] = 'x'
	}
	if got := firstLine(string(long)); len(got) != 120 {
		t.Errorf("len(firstLine(300 x)) = %d, want 120", len(got))
	}
}
