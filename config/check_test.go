package config

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestCheckConfigRejectsInvalidNumericParams(t *testing.T) {
	base := Config{
		Version:         "test",
		Path:            os.Args[0], // 测试二进制自身一定存在且是文件
		Port:            11883,
		LogCapacity:     200,
		LogMaxLineBytes: 1048576,
		RestartTimes:    3,
		LocalLogLifeDay: 7,
	}
	if err := CheckConfig(base); err != nil {
		t.Fatalf("baseline config should pass, got: %v", err)
	}

	cases := []struct {
		name    string
		mutate  func(*Config)
		contain string
	}{
		{"negative log capacity", func(c *Config) { c.LogCapacity = -1 }, "log-capacity"},
		{"zero log max line bytes", func(c *Config) { c.LogMaxLineBytes = 0 }, "log-max-line-bytes"},
		{"negative log max line bytes", func(c *Config) { c.LogMaxLineBytes = -1 }, "log-max-line-bytes"},
		{"port too low", func(c *Config) { c.Port = 0 }, "port"},
		{"port too high", func(c *Config) { c.Port = 70000 }, "port"},
		{"invalid retry count", func(c *Config) { c.RestartTimes = -2 }, "max-retries"},
		{"negative log life day", func(c *Config) { c.LocalLogLifeDay = -1 }, "log-life-day"},
		{"probe without interval", func(c *Config) { c.ProbeCmd = "true"; c.ProbeInterval = 0; c.ProbeTimeout = 5; c.ProbeFailureLimit = 3 }, "probe-interval"},
		{"probe without timeout", func(c *Config) {
			c.ProbeCmd = "true"
			c.ProbeTimeout = 0
			c.ProbeInterval = 30
			c.ProbeFailureLimit = 3
		}, "probe-timeout"},
		{"probe without failure limit", func(c *Config) {
			c.ProbeCmd = "true"
			c.ProbeFailureLimit = 0
			c.ProbeInterval = 30
			c.ProbeTimeout = 5
		}, "probe-failure-limit"},
		{"missing path", func(c *Config) { c.Path = "" }, "Path"},
		{"nonexistent path", func(c *Config) { c.Path = "/definitely/not/exists/xyz" }, "不存在"},
		{"negative restart delay", func(c *Config) { c.RestartDelay = -time.Second }, "restart-delay"},
		{"negative restart delay max", func(c *Config) { c.RestartDelayMax = -time.Second }, "restart-delay-max"},
		{"restart delay max below base", func(c *Config) {
			c.RestartDelay = 2 * time.Second
			c.RestartDelayMax = time.Second
		}, "restart-delay-max"},
		{"restart delay max below the default base", func(c *Config) {
			// 未配 base(=0，按缺省 2s 生效) 却把 max 配成 1s：形同虚设，必须拒绝
			c.RestartDelay = 0
			c.RestartDelayMax = time.Second
		}, "restart-delay-max"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := base // 每个用例独立拷贝，避免前序用例的 ProbeCmd 泄漏
			tc.mutate(&cfg)
			err := CheckConfig(cfg)
			if err == nil {
				t.Fatalf("expected error for %s", tc.name)
			}
			if !strings.Contains(err.Error(), tc.contain) {
				t.Fatalf("error %q should mention %q", err.Error(), tc.contain)
			}
		})
	}
}

func TestCheckConfigSkippedWithoutVersion(t *testing.T) {
	// Version 为空时跳过检查（兼容旧行为）
	err := CheckConfig(Config{})
	if err != nil {
		t.Fatalf("empty version should skip checks, got: %v", err)
	}
}

func TestParseConfigListenDefaults(t *testing.T) {
	cfg := ParseConfigListen("v", "/bin/true", "", "", "", false, 3, false, 11883, "", "", 200, 1048576, false, "", 7)
	if cfg.ListenAddress != "127.0.0.1" {
		t.Fatalf("empty listen address should default to 127.0.0.1, got %q", cfg.ListenAddress)
	}
}

// 未显式配置退避时，必须回落到历史等价的 2s 固定基数，而不是零值。
func TestParseConfigListenDefaultsRestartDelay(t *testing.T) {
	cfg := ParseConfigListen("v", "/bin/true", "", "", "", false, 3, false, 11883, "", "", 200, 1048576, false, "", 7)
	if cfg.RestartDelay != DefaultRestartDelay {
		t.Fatalf("未配置退避基数时应为 %s, 实际 %s", DefaultRestartDelay, cfg.RestartDelay)
	}
	if cfg.RestartDelayMax != 0 {
		t.Fatalf("未配置退避上限时应为 0(不封顶), 实际 %s", cfg.RestartDelayMax)
	}
}

// 带退避的构造函数：显式值必须透传，零值/负值必须被规范化。
func TestParseConfigListenWithProbeAndBackoff(t *testing.T) {
	call := func(delay, max time.Duration) *Config {
		return ParseConfigListenWithProbeAndBackoff(
			"v", "/bin/true", "", "", "", false, 3, false,
			11883, "", "", 200, 1048576, false, "", 7,
			"true", 30, 5, 3, delay, max)
	}

	t.Run("explicit values pass through", func(t *testing.T) {
		cfg := call(500*time.Millisecond, 8*time.Second)
		if cfg.RestartDelay != 500*time.Millisecond {
			t.Fatalf("退避基数应透传, 实际 %s", cfg.RestartDelay)
		}
		if cfg.RestartDelayMax != 8*time.Second {
			t.Fatalf("退避上限应透传, 实际 %s", cfg.RestartDelayMax)
		}
	})

	t.Run("zero base falls back to default", func(t *testing.T) {
		cfg := call(0, 5*time.Second)
		if cfg.RestartDelay != DefaultRestartDelay {
			t.Fatalf("零值基数应回落到 %s, 实际 %s", DefaultRestartDelay, cfg.RestartDelay)
		}
	})

	t.Run("negative max is clamped to zero", func(t *testing.T) {
		cfg := call(time.Second, -1)
		if cfg.RestartDelayMax != 0 {
			t.Fatalf("负值上限应被规范为 0(不封顶), 实际 %s", cfg.RestartDelayMax)
		}
	})

	t.Run("legacy constructor still defaults to 2s", func(t *testing.T) {
		// 未改签名的旧入口必须保持历史行为，避免调用方因签名变更而行为漂移
		cfg := ParseConfigListenWithProbe("v", "/bin/true", "", "", "", false, 3, false,
			11883, "", "", 200, 1048576, false, "", 7, "true", 30, 5, 3)
		if cfg.RestartDelay != DefaultRestartDelay {
			t.Fatalf("旧构造函数退避基数应为 %s, 实际 %s", DefaultRestartDelay, cfg.RestartDelay)
		}
	})
}
