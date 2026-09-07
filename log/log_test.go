// FasterEdge 开源项目 - Github: https://github.com/FasterEdge - Gitee: https://gitee.com/FasterEdge
package log

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// parseTimestampFromName 必须正确提取 "YYYYMMDD-HHMMSS" 段
// (历史暗病: 只取 "HHMMSS" 段导致 time.Parse 永远失败, 过期清理从不生效)。
func TestParseTimestampFromName(t *testing.T) {
	cases := []struct {
		name, proc string
		ok         bool
		want       string // 期望解析出的时间(格式 20060102-150405)
	}{
		{"myapp-20260907-123456-01.log", "myapp", true, "20260907-123456"},
		{"my-app-20260907-123456-01.log", "my-app", true, "20260907-123456"}, // procName 含 '-'
		{"myapp-20260907-123456-0123.log", "myapp", true, "20260907-123456"}, // seq 多位
		{"myapp-20260907-123456.log", "myapp", false, ""},                    // 缺 seq
		{"other-20260907-123456-01.log", "myapp", false, ""},                 // procName 不匹配
		{"myapp-20260907.log", "myapp", false, ""},                           // 太短
		{"myapp-20260907-123456-XX.log", "myapp", false, ""},                 // seq 非数字
	}
	for _, c := range cases {
		got, ok := parseTimestampFromName(c.name, c.proc)
		if ok != c.ok {
			t.Fatalf("%q: ok=%v, 期望 %v", c.name, ok, c.ok)
		}
		if !ok {
			continue
		}
		if got.Format(timeLayout) != c.want {
			t.Fatalf("%q: 时间 %s, 期望 %s", c.name, got.Format(timeLayout), c.want)
		}
	}
}

// cleanupLocked 必须删除超过 lifeDays 的旧日志, 保留新日志。
func TestCleanupRemovesExpired(t *testing.T) {
	dir := t.TempDir()
	fl := &FileLogger{dir: dir, procName: "app", lifeDays: 2}

	now := time.Now()
	// 三个文件: 3 天前(过期)、昨天(保留)、今天(保留)
	old := now.AddDate(0, 0, -3).Format(timeLayout)
	yesterday := now.AddDate(0, 0, -1).Format(timeLayout)
	today := now.Format(timeLayout)
	names := []string{
		"app-" + old + "-01.log",
		"app-" + yesterday + "-01.log",
		"app-" + today + "-01.log",
	}
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	fl.cleanupLocked(now)

	for _, n := range names {
		_, err := os.Stat(filepath.Join(dir, n))
		exists := err == nil
		if n == "app-"+old+"-01.log" {
			if exists {
				t.Fatalf("过期日志 %s 应被删除", n)
			}
		} else if !exists {
			t.Fatalf("有效日志 %s 不应被删除", n)
		}
	}
}
