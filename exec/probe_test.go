package exec

import (
	"errors"
	"testing"
	"time"
)

// 阈值判定：limit<=0 视为未启用，恒不触发（fail-closed，不会误杀进程）。
func TestHealthProbeShouldConverge(t *testing.T) {
	var p HealthProbe
	if p.ShouldConverge(3, 0) {
		t.Fatal("limit=0 时不应触发收敛")
	}
	if p.ShouldConverge(2, 3) {
		t.Fatal("失败次数未达阈值不应触发收敛")
	}
	if !p.ShouldConverge(3, 3) {
		t.Fatal("失败次数达到阈值必须触发收敛")
	}
	if !p.ShouldConverge(4, 3) {
		t.Fatal("超过阈值同样应触发收敛")
	}
}

// 探针收敛必须复用 StopManagedProcess 的同一条路径：
// 先标记主动停止(stoppedByReq)并递增代际，使 monitor 的重启计划过期，
// 避免"探针杀了进程、monitor 又把它拉起来"的自愈失效。
func TestHealthProbeConvergeMarksRequestAndBumpsGeneration(t *testing.T) {
	var p Process
	probe := HealthProbe{}
	before := p.generation

	// 进程未运行时，收敛等价于幂等停止：返回 ErrProcessNotRunning，
	// 但必须已标记主动停止（否则等待窗口后的自动重启会复活子进程）。
	err := probe.Converge(&p, 200*time.Millisecond)
	if !errors.Is(err, ErrProcessNotRunning) {
		t.Fatalf("未运行时收敛应返回 ErrProcessNotRunning, 实际: %v", err)
	}
	p.ProcessMu.Lock()
	defer p.ProcessMu.Unlock()
	if !p.stoppedByReq {
		t.Fatal("探针收敛未运行进程也必须标记 stoppedByReq")
	}
	if p.generation != before+1 {
		t.Fatalf("generation 必须 +1(使重启计划过期), got %d -> %d", before, p.generation)
	}
}

// 探针对 nil 进程必须返回错误而不是 panic（日志轮转/竞态下的防御）。
func TestHealthProbeConvergeNilProcess(t *testing.T) {
	probe := HealthProbe{}
	if err := probe.Converge(nil, time.Second); err == nil {
		t.Fatal("nil 进程应返回错误")
	}
}

// 宽限为 0/负值时回落到默认 5s，避免"零宽限直接强杀"这种静默降级。
func TestHealthProbeConvergeFallsBackToDefaultGrace(t *testing.T) {
	var p Process
	probe := HealthProbe{}
	start := time.Now()
	// 进程未运行时立即返回，但必须仍然走同一条路径（返回幂等错误而非 panic）
	err := probe.Converge(&p, 0)
	if !errors.Is(err, ErrProcessNotRunning) {
		t.Fatalf("期望 ErrProcessNotRunning, 实际: %v", err)
	}
	if time.Since(start) > time.Second {
		t.Fatal("未运行进程的收敛必须立即返回，不应等待宽限")
	}
	if decideConverge().GraceTimeout != 5*time.Second {
		t.Fatalf("零宽限应回落到 5s, 实际 %s", decideConverge().GraceTimeout)
	}
}

// 默认处置策略：优雅优先 + 5s 宽限，与 core 的 /shutdown 宽限一致。
func TestDecideConvergeDefaults(t *testing.T) {
	d := decideConverge()
	if !d.GracefulFirst {
		t.Fatal("探针收敛必须优雅优先(先发信号再强杀)")
	}
	if d.GraceTimeout != 5*time.Second {
		t.Fatalf("优雅宽限应为 5s, 实际 %s", d.GraceTimeout)
	}
	if d.Reason == "" {
		t.Fatal("处置策略必须记录原因，便于日志与审计")
	}
}
