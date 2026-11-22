// ─────────────────────────────────────────────────────────────
// FasterEdge 开源项目
// Github: https://github.com/FasterEdge
// Gitee:  https://gitee.com/FasterEdge
// ─────────────────────────────────────────────────────────────
//
//go:build windows

// 平台差异层（Windows）：把"进程组/信号"从进程管理逻辑中剥离出来。
//
// Windows 没有 POSIX 信号，可用手段与 Unix 不同：
//   - 优雅停止：向子进程的控制台组投递 CTRL_BREAK_EVENT，子进程（Go 运行时）
//     会把它派发为 os.Interrupt，可被 signal.Notify 捕获。
//     前置条件是启动时带 CREATE_NEW_PROCESS_GROUP，见 applyLaunchGroup。
//   - 强杀收敛：taskkill /PID <pid> /T /F 终止整棵进程树。
//     不能用 Process.Kill：Windows 上它等价于 TerminateProcess，只终止单个进程，
//     会留下子进程派生的后代成为孤儿。
//
// 新增一个平台只需两步（详见 README「平台差异与移植」）：
//  1. 若该平台的 GOOS 属于 Unix 家族 → 交由 platform_unix.go 覆盖，无需改动；
//  2. 否则新建 platform_<goos>.go，实现 applyLaunchGroup / terminateGroup /
//     waitStatus 三个符号，并在 README 的「平台差异清单」登记。

package exec

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

// applyLaunchGroup 在 cmd.Start() 之前设置 CREATE_NEW_PROCESS_GROUP，
// 使 CTRL_BREAK_EVENT 能投递到该子进程的控制台组。
func applyLaunchGroup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= syscall.CREATE_NEW_PROCESS_GROUP
}

// taskkill 命令名单独抽出，便于在测试里断言调用形态。
const taskkillExe = "taskkill"

// terminateGroup 收敛子进程树。
// force=false：向控制台组投递 CTRL_BREAK_EVENT（优雅，子进程可捕获 os.Interrupt）；
// force=true ：taskkill /T /F 收敛整棵树，避免只杀主进程留下孤儿。
func terminateGroup(pid *os.Process, force bool) error {
	if pid == nil {
		return nil
	}

	if force {
		cmd := exec.Command(taskkillExe, "/PID", fmt.Sprint(pid.Pid), "/T", "/F")
		if err := cmd.Run(); err == nil {
			return nil
		}
		// taskkill 不可用（极简镜像/权限受限）时退化为单进程强杀，
		// 至少保证主进程不复活；此时后代可能成为孤儿，由调用方日志可见。
		return pid.Kill()
	}

	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	generateConsoleCtrlEvent := kernel32.NewProc("GenerateConsoleCtrlEvent")
	ret, _, callErr := generateConsoleCtrlEvent.Call(uintptr(syscall.CTRL_BREAK_EVENT), uintptr(pid.Pid))
	if ret == 0 {
		if callErr != nil && callErr != syscall.Errno(0) {
			return callErr
		}
		return fmt.Errorf("GenerateConsoleCtrlEvent 返回失败")
	}
	return nil
}

// waitStatus 是 syscall.WaitStatus 的类型别名，供 monitor 判断"是否被信号终止"。
// Windows 上 WaitStatus.Signaled() 恒为 false（无 POSIX 信号概念），
// 因此 LastExitBySignal 在 Windows 上不会为 true —— 这是平台事实，非缺陷。
type waitStatus = syscall.WaitStatus
