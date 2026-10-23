package core

import (
	"strings"
	"testing"
)

func TestTrimLogsToBudget(t *testing.T) {
	// 预算内: 不截断
	logs := []string{"a", "bb", "ccc"}
	got := trimLogsToBudget(logs, 100)
	if len(got) != 3 || got[0] != "a" || got[2] != "ccc" {
		t.Fatalf("budget-ok case: got %v", got)
	}
	// 超预算: 保留尾部(最近的), 丢弃头部(最旧的)
	logs = []string{"x" + strings.Repeat("x", 10000), "short", "tail"}
	got = trimLogsToBudget(logs, 20) // 9字节(short+tail)装得下, 10001字节的头部行放不下
	if len(got) != 2 || got[0] != "short" || got[1] != "tail" {
		t.Fatalf("trim case: got %v", got)
	}
	// 空输入: 返回 nil 语义(不变)
	if trimLogsToBudget(nil, 100) != nil {
		t.Fatal("nil in -> non-nil out")
	}
	if len(trimLogsToBudget([]string{}, 100)) != 0 {
		t.Fatal("empty in -> non-empty out")
	}
	// 预算<=0: 原样返回
	if trimLogsToBudget(logs, 0) != nil && len(trimLogsToBudget(logs, 0)) != len(logs) {
		t.Fatalf("budget<=0 case: got %v", trimLogsToBudget(logs, 0))
	}
}
