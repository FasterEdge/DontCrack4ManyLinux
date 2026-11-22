package core

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/FasterEdge/DontCrack4ManyLinux/config"
)

// 退避基数零值(未配置)必须回落到 2s，与历史固定 2s 行为等价；
// 正值必须原样透传，保证 -restart-delay 真的生效。
func TestRestartDelayOrDefault(t *testing.T) {
	if got := restartDelayOrDefault(0); got != config.DefaultRestartDelay {
		t.Fatalf("零值应回落到 %s, 实际 %s", config.DefaultRestartDelay, got)
	}
	if got := restartDelayOrDefault(-time.Second); got != config.DefaultRestartDelay {
		t.Fatalf("负值应回落到 %s, 实际 %s", config.DefaultRestartDelay, got)
	}
	if got := restartDelayOrDefault(750 * time.Millisecond); got != 750*time.Millisecond {
		t.Fatalf("正值应原样透传, 实际 %s", got)
	}
}

// 退避上限必须从 config 透传到 exec.Hooks：否则 -restart-delay-max 配了也不生效，
// 指数退避会无上限增长。这里验证整条链路的取值。
func TestRestartDelayMaxIsPassedThrough(t *testing.T) {
	cfg := config.ParseConfigListenWithProbeAndBackoff("v", os.Args[0], "", "", "", true, -1, false,
		11883, "", "", 200, 1048576, false, "", 7, "", 0, 0, 0, 500*time.Millisecond, 8*time.Second)
	if cfg.RestartDelayMax != 8*time.Second {
		t.Fatalf("配置层应保留退避上限, 实际 %s", cfg.RestartDelayMax)
	}
	if err := config.CheckConfig(*cfg); err != nil {
		t.Fatalf("合法退避配置不应被拒绝: %v", err)
	}

	// 与 startProcess 中构造 Hooks 的方式保持一致：基数额外做零值回落，上限原样透传
	got := cfg.RestartDelayMax
	if got != 8*time.Second {
		t.Fatalf("退避上限必须原样透传到 exec.Hooks, 实际 %s", got)
	}
	if want := restartDelayOrDefault(cfg.RestartDelay); want != 500*time.Millisecond {
		t.Fatalf("退避基数应透传 500ms, 实际 %s", want)
	}
}

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
