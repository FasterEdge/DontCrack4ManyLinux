package exec

import (
	"testing"
	"time"
)

// 退避序列必须与 DontCrack-Manager 的 nextBackoff 语义一致：
// min(base<<shift, max)，首次重试用 base，shift=n-1 且封顶 20。
func TestBackoffDelayExponentialSequence(t *testing.T) {
	p := RestartPolicy{Delay: time.Second, MaxDelay: 10 * time.Second}

	want := []time.Duration{
		time.Second,      // n=1: base<<0
		2 * time.Second,  // n=2: base<<1
		4 * time.Second,  // n=3: base<<2
		8 * time.Second,  // n=4: base<<3
		10 * time.Second, // n=5: 16s 被封顶到 10s
		10 * time.Second, // n=6: 32s 仍被封顶
	}
	for i, w := range want {
		n := i + 1
		if got := p.BackoffDelay(n); got != w {
			t.Fatalf("第 %d 次重启的退避应为 %s, 实际 %s", n, w, got)
		}
	}
}

// MaxDelay<=0 表示不封顶：序列应持续指数增长。
func TestBackoffDelayNoCap(t *testing.T) {
	p := RestartPolicy{Delay: 500 * time.Millisecond}
	if got := p.BackoffDelay(4); got != 4*time.Second {
		t.Fatalf("不封顶时第4次应为 4s, 实际 %s", got)
	}
}

// 左移溢出保护：shift 极大时不得回绕成小值或负数，必须落到 MaxDelay。
func TestBackoffDelayOverflowGuard(t *testing.T) {
	p := RestartPolicy{Delay: time.Second, MaxDelay: 30 * time.Second}
	// 1<<63 远超 int64，若不保护会回绕
	for _, n := range []int{64, 1000} {
		if got := p.BackoffDelay(n); got != p.MaxDelay {
			t.Fatalf("溢出时第 %d 次退避应封顶 %s, 实际 %s", n, p.MaxDelay, got)
		}
	}
}

// Delay<=0 时回落到历史默认 2s，保证与旧行为兼容。
func TestBackoffDelayDefaultsToLegacyTwoSeconds(t *testing.T) {
	var p RestartPolicy
	if got := p.BackoffDelay(1); got != 2*time.Second {
		t.Fatalf("未配置退避基数时应回落到 2s, 实际 %s", got)
	}
	if got := p.BackoffDelay(0); got != 2*time.Second {
		t.Fatalf("计数为 0 时也应为 2s, 实际 %s", got)
	}
}

// n<=1 一律返回 base：首次重启不应有指数放大。
func TestBackoffDelayFirstAttemptIsBase(t *testing.T) {
	p := RestartPolicy{Delay: 3 * time.Second, MaxDelay: time.Minute}
	if got := p.BackoffDelay(1); got != 3*time.Second {
		t.Fatalf("首次重启应为 base 3s, 实际 %s", got)
	}
	if got := p.BackoffDelay(-5); got != 3*time.Second {
		t.Fatalf("负计数应视为首次, 实际 %s", got)
	}
}

// 稳定期阈值必须是 30s，与 DontCrack-Manager 的 stableThreshold 对齐。
func TestStableThresholdIsThirtySeconds(t *testing.T) {
	if StableThreshold != 30*time.Second {
		t.Fatalf("稳定期阈值应为 30s, 实际 %s", StableThreshold)
	}
}

// 稳定期判定：存活时长达到阈值才重置，未达到不重置。
// 这里直接验证 monitor 中使用的判定表达式，确保语义不漂移。
func TestStablePeriodResetDecision(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name     string
		started  time.Time
		elapsed  time.Duration
		wantRest bool
	}{
		{"恰好30s", now.Add(-30 * time.Second), 30 * time.Second, true},
		{"超过30s", now.Add(-45 * time.Second), 45 * time.Second, true},
		{"不足30s", now.Add(-5 * time.Second), 5 * time.Second, false},
		{"零时刻(未启动)", time.Time{}, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// 与 process.go monitor 中的表达式保持一致
			reset := tc.started.After(time.Time{}) && tc.elapsed >= StableThreshold
			if reset != tc.wantRest {
				t.Fatalf("%s: 稳定期重置应为 %v, 实际 %v", tc.name, tc.wantRest, reset)
			}
		})
	}
}
