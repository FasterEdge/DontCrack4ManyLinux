// ─────────────────────────────────────────────────────────────
// FasterEdge 开源项目
// Github: https://github.com/FasterEdge
// Gitee:  https://gitee.com/FasterEdge
// ─────────────────────────────────────────────────────────────
package exec

import (
	"fmt"
	"time"
)

// HealthProbe 封装健康探针的一次"判不健康 → 收敛进程"的动作。
// 与 core 层解耦：探针节奏（间隔/超时/阈值）由 core 决定，
// 而"如何收敛进程"复用 platform_*.go 的信号路径，保证探针自愈与
// /shutdown 走同一条语义（优雅信号优先，超时后强杀整棵树）。
type HealthProbe struct{}

// HealthProbeDecision 是探针连续失败达到阈值后的一次处置决策。
type HealthProbeDecision struct {
	// GracefulFirst 为 true 表示应先发优雅信号，等待 GraceTimeout 后再强杀。
	GracefulFirst bool
	// GraceTimeout 是优雅阶段的等待时长。
	GraceTimeout time.Duration
	// Reason 记录本次处置的原因，便于日志与测试断言。
	Reason string
}

// ShouldConverge 判断"连续失败次数已达阈值"是否成立。
// limit<=0 视为未启用阈值，恒不触发（fail-closed 交给上层配置校验）。
func (h HealthProbe) ShouldConverge(consecutiveFails, limit int) bool {
	return limit > 0 && consecutiveFails >= limit
}

// Converge 在探针连续失败达阈值时收敛进程：先优雅信号，超时后强杀整棵树。
// gracefulTimeout<=0 时使用默认宽限（与 /shutdown 的 5s 一致）。
// 返回 nil 表示进程已在该时限内退出（优雅或强杀生效）。
func (h HealthProbe) Converge(p *Process, gracefulTimeout time.Duration) error {
	if p == nil {
		return fmt.Errorf("进程对象为空")
	}
	if gracefulTimeout <= 0 {
		gracefulTimeout = decideConverge().GraceTimeout
	}
	// 与 /shutdown 完全同一条停止路径：优雅信号 → 超时 → 强杀进程树
	return p.StopManagedProcess(gracefulTimeout)
}

// decideConverge 给出探针触发时的处置策略。
// 默认"优雅优先 + 5s 宽限"，与 core 的 /shutdown 宽限一致。
func decideConverge() HealthProbeDecision {
	return HealthProbeDecision{
		GracefulFirst: true,
		GraceTimeout:  5 * time.Second,
		Reason:        "健康探针连续失败达到阈值",
	}
}
