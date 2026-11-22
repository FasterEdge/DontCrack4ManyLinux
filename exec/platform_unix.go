// ─────────────────────────────────────────────────────────────
// FasterEdge 开源项目
// Github: https://github.com/FasterEdge
// Gitee:  https://gitee.com/FasterEdge
// ─────────────────────────────────────────────────────────────
//
//go:build unix

// 平台差异层（Unix）：把"进程组/信号"从进程管理逻辑中剥离出来。
// 覆盖 linux / android / darwin(macOS) / ios / *BSD / solaris / illumos。
// 注意：OpenHarmony 版 release.yml 使用 GOOS=linux 编译，因此同样落入本文件。
//
// 新增一个平台只需两步（详见 README「平台差异与移植」）：
//  1. 若该平台的 GOOS 属于 Unix 家族 → 本文件已覆盖，无需改动；
//  2. 否则新建 platform_<goos>.go，实现 applyLaunchGroup / terminateGroup /
//     waitStatus 三个符号，并在 README 的「平台差异清单」登记。

package exec

import (
	"os"
	"os/exec"
	"syscall"
)

// applyLaunchGroup 在 cmd.Start() 之前把子进程放进独立进程组。
// 必须在这里设置：setpgid 由 fork/exec 内部完成，进程起来后再调用
// syscall.Setpgid 会因已 exec 而返回 EACCES，从而静默退化为"只杀主进程"。
func applyLaunchGroup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true // Pgid 取零值 → 进程组 ID = 子进程 PID
}

// terminateGroup 向子进程所在进程组发信号。
// force=false 为优雅阶段：SIGTERM；
// force=true 为强杀阶段：SIGKILL，且对整组发送以收敛子进程派生的后代。
func terminateGroup(pid *os.Process, force bool) error {
	if pid == nil {
		return nil
	}
	sig := syscall.SIGTERM
	if force {
		sig = syscall.SIGKILL
	}
	// 优先对进程组(-pid)发；若进程组未建立则退化为仅对主进程发。
	if err := syscall.Kill(-pid.Pid, sig); err != nil {
		return syscall.Kill(pid.Pid, sig)
	}
	return nil
}

// waitStatus 是 syscall.WaitStatus 的类型别名，供 monitor 判断"是否被信号终止"。
type waitStatus = syscall.WaitStatus
