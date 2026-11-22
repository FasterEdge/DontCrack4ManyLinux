<div align="center">
<img src="./Logo.png" alt="logo" width="100"/>
<h2>DontCrack4ManyLinux</h2>
<h3>Generic Linux (manylinux) Process Manager</h3>
</div>


### 1. Features

- A process manager for generic Linux distributions (manylinux), designed to improve the robustness, availability and timing stability of background/daemon processes
- Host a target process directly via a systemd unit or the command line, avoiding service unavailability caused by application crashes
- Supports managing these process types: `binary executable`, `sh script`
- Maps processes to port numbers; RESTful API for getting logs, starting/stopping processes, etc.
- Each managed process can be configured independently with: program path, environment variables, startup arguments, pre-processing script, auto-restart toggle, max crash-retry count, restart backoff base and cap, start-now flag, port number, log cache line limit, per-line byte limit, local log storage path, log retention period, etc.
- Auto-restart uses exponential backoff (`min(base<<(n-1), max)`, aligned with DontCrack-Manager) with a stable-period reset: once the child has stayed alive for 30s the retry counter resets, so occasional crashes never exhaust the retry budget
- Graceful stop and probe convergence signal the whole **process group** (Unix `SIGTERM` / Windows `CTRL_BREAK_EVENT`) and force-kill the entire tree only after the grace period, so no orphaned grandchildren are left behind
- Cross-architecture, no CGO required; supports any architecture that the Go compiler can target (amd64 / arm64 / armv7 etc.)
- Shares the same CLI, HTTP API and file structure with the OpenHarmony and Android versions

### 2. Basic Usage

```
./DontCrack \
  -path "/opt/DontCrack4ManyLinux/example/childproc/childproc" \
  -args "-mode normal -interval 500ms -lifetime 5s" \
  -env "EXTRA_INFO=from_manager RESTART_ENV_COUNT=0" \
  -file-log -log-path ./example/logs/ -log-life-day 7 \
  -auto-restart -max-retries 2 \
  -start-now \
  -password 123456
```

| Config | Type | Default | Description |
|--------|------|---------|-------------|
| path | string | "" | Path of the program to manage (supports executables, shell scripts etc.) |
| args | string | "" | Arguments passed to the program (optional) |
| pre | string | "" | Command to run before startup (executed in sh; multiple commands can be chained with && / ; / ||, default empty) |
| env | string | "" | Environment variables to append for the child process, e.g. "PATH=/usr/local/bin:/usr/bin FOO=bar"; separated by spaces or semicolons |
| auto-restart | bool | false | Whether to auto-restart on crash |
| max-retries | int | 3 | Max retry count (-1 means unlimited, default 3) |
| restart-delay | duration | 2s | Backoff base: the nth restart waits `min(base<<(n-1), restart-delay-max)`; the first restart waits exactly base |
| restart-delay-max | duration | 0 | Backoff cap (default 0 = uncapped; must not be smaller than restart-delay) |
| start-now | bool | false | Whether to start immediately |
| probe-cmd | string | "" | Health check command for the child process (empty disables; on consecutive failures the manager **gracefully stops and converges the whole process group**) |
| probe-interval | int | 30 | Probe interval in seconds |
| probe-timeout | int | 5 | Probe timeout in seconds |
| probe-failure-limit | int | 3 | Consecutive failures that mark unhealthy (default 3) |
| password | string | "" | Password for managing the process (optional; no password protection if empty) |
| listen-address | string | 127.0.0.1 | HTTP listen address (default loopback only; external listen requires a password) |
| port | int | 11883 | HTTP service port |

> **Probe interaction (important)**: once the probe fails consecutively up to the
> limit, the manager takes the exact same stop path as `/shutdown`: it first sends a
> graceful signal to the child's **process group** (Unix: `SIGTERM`; Windows:
> `CTRL_BREAK_EVENT`), and if the child is still alive after 5 seconds it force-kills the
> **whole process tree** (Unix: `SIGKILL` to the group; Windows: `taskkill /T /F`).
> Whether it is restarted afterwards depends on
> `-auto-restart` — **enable `-auto-restart` (e.g. `-max-retries -1` for unlimited)
> whenever the probe is meant for health self-healing**, otherwise the child will not
> recover after being stopped by the probe and needs a manual `/startup`. If the probe
> is only meant to "detect and take down", you may leave it disabled.
| log-capacity | int | 200 | Max lines of cached logs (default 200) |
| log-max-line-bytes | int | 1048576 | Max bytes per log line (for bufio.Scanner, default 1 MiB) |
| file-log | bool | false | Whether to enable file logging (default false) |
| log-path | string | ./logs/proc_manager/ | Local log file directory (default ./logs/proc_manager/, subdirectories created per process name) |
| log-life-day | int | 7 | Log retention in days (default 7; expired logs cleaned when new logs are written) |

### 3. API Documentation

> `/`

- Description: Root path; returns a plain-text banner (`DontCrack_linux By FasterEdge`), handy as a connectivity check
- Method: GET
- Request parameters: none
- Response type: text
- Example response:
  ```
  DontCrack_linux By FasterEdge
  ```

> `/ui` (and `/ui/`)

- Description: Embedded single-file HTML console (`core/webui/index.html`, compiled into the
  binary via `go:embed`) offering status view, start/stop buttons and log refresh; no external
  static files needed
- Method: GET
- Request parameters: none
- Response type: HTML
- **Authentication note (stated honestly)**: neither `/ui` nor `/ui/` checks the password, so
  anyone can fetch this static page. The page itself contains no process data; its data
  requests (`/heartbeat`) and actions (`/startup`, `/shutdown`) are still password-checked, so
  with `-password` configured an unauthenticated visitor only gets an empty shell
- Note: the page polls `/heartbeat` every 3 seconds; a `?password=` in the URL is only read
  into the input box on load, real requests always use the `Authorization: Bearer` header, so
  the password never lands in browser history or access logs

> `/healthz`

- Description: Health check (K8s `livenessProbe` / load-balancer health check). The manager's
  own liveness is implied by its HTTP server answering at all; it returns 200 only when the
  managed child process is running
- Method: GET
- Request parameters:
  ```
  password: secret key (optional params parameter; required when -password is configured)
  ```
- Response type: text
- Example response:
  ```
  ok
  ```
  Returns `503` + `process not running` when the child is not running

> `/readyz`

- Description: Readiness check (K8s `readinessProbe`). **Semantically identical to `/healthz`
  with exactly the same condition** (it likewise requires the child process to be running for
  a 200). The two paths exist only so a K8s manifest can point `livenessProbe` and
  `readinessProbe` at different addresses and tune each independently. The only difference is
  the response body: `/healthz` returns `ok`, `/readyz` returns `ready`
- Method: GET
- Request parameters:
  ```
  password: secret key (optional params parameter; required when -password is configured)
  ```
- Response type: text
- Example response:
  ```
  ready
  ```
  Returns `503` + `not ready` when the child is not running

> `/metrics`

- Description: Prometheus text-format metrics (`text/plain; version=0.0.4`); GET only, any
  other method returns 405
- Method: GET
- Request parameters:
  ```
  password: secret key (optional params parameter; required when -password is configured)
  ```
- Response type: text (Prometheus exposition format)
- Metrics:

  | Metric | Type | Meaning |
  | --- | --- | --- |
  | `dontcrack_up` | gauge | manager is running (always 1) |
  | `dontcrack_process_state` | gauge | child state, 1=running, 0=stopped |
  | `dontcrack_process_pid` | gauge | child PID (0 when not running) |
  | `dontcrack_restart_count` | counter | cumulative auto-restart count |
  | `dontcrack_last_exit_code` | gauge | last exit code of the child |
  | `dontcrack_last_exit_time_seconds` | gauge | unix timestamp of the last exit (0 = never exited) |
  | `dontcrack_log_lines` | gauge | number of log lines cached in memory |
  | `dontcrack_info` | gauge | version/platform info, labels `version`, `file_type`, `process_path` |

- Example response:
  ```
  # HELP dontcrack_up 管理器是否在运行(始终 1)
  # TYPE dontcrack_up gauge
  dontcrack_up 1
  # HELP dontcrack_process_state 子进程运行状态 1=running 0=stopped
  # TYPE dontcrack_process_state gauge
  dontcrack_process_state 0
  # HELP dontcrack_process_pid 子进程 PID
  # TYPE dontcrack_process_pid gauge
  dontcrack_process_pid 0
  # HELP dontcrack_restart_count 累计自动重启次数
  # TYPE dontcrack_restart_count counter
  dontcrack_restart_count 0
  # HELP dontcrack_last_exit_code 子进程上次退出码
  # TYPE dontcrack_last_exit_code gauge
  dontcrack_last_exit_code 0
  # HELP dontcrack_last_exit_time_seconds 子进程上次退出 unix 时间戳
  # TYPE dontcrack_last_exit_time_seconds gauge
  dontcrack_last_exit_time_seconds 0
  # HELP dontcrack_log_lines 内存中缓存的日志行数
  # TYPE dontcrack_log_lines gauge
  dontcrack_log_lines 0
  # HELP dontcrack_info 版本/平台信息
  # TYPE dontcrack_info gauge
  dontcrack_info{version="1.0.20261123",file_type="shell_script",process_path="/opt/.../childproc"} 1
  ```

> /startup

- Description: Starts the process and resets the retry count
- Method: POST
- Request parameters:
  ```
  password: secret key (optional params parameter)
  ```
- Response type: text
- Example response:
    ```
    ok
    ```

> /heartbeat

- Description: Returns heartbeat information, including startup status and cached logs (logs are cleared after reading)
- Method: GET
- Request parameters:
  ```
  password: secret key (optional params parameter)
  ```
- Response type: JSON
- Example response:
     ```
	{
	"version": "1.0.20261123",
	"state": "stopped",
	"info": "Process manager running normally",
	"timestamp": "2026-08-25 15:28:04",
	"logs": [
	"[STDERR] 2026/08/25 15:27:55.647714 env restart count -> 1",
	"[STDERR] 2026/08/25 15:27:55.648316 childproc start | pid=32054 | mode=normal | interval=1s | lifetime=5s | msg=",
	"[STDERR] 2026/08/25 15:27:55.648345 args: /opt/.../childproc -mode normal -interval 1s -lifetime 5s",
	"[STDERR] 2026/08/25 15:27:55.648352 env EXTRA_INFO=from_manager",
	"[STDERR] 2026/08/25 15:27:55.648353 env RESTART_ENV_COUNT=0",
	"[STDERR] 2026/08/25 15:27:56.668982 tick at 2026-08-25T15:27:56.64879725+08:00",
	"[STDERR] 2026/08/25 15:28:00.686300 lifetime reached, exiting normally"
	],
	"process_pid": 0,
	"process_path": "/opt/.../childproc",
	"restart_count": 0,
	"file_type": "binary_executable",
	"last_exit_time": "2026-08-25 15:28:00",
	"program_args": "-mode normal -interval 1s -lifetime 5s",
	"extra_env_raw": "PATH=/usr/local/bin EXTRA_INFO=from_manager RESTART_ENV_COUNT=0"
	}
	```

> /shutdown

- Description: Terminates the process
- Method: POST
- Request parameters:
  ```
  password: secret key (optional params parameter)
  ```
- Response type: text
- Example response:
  ```
  ok
  ```

### 4. Details

- Use full paths for the managed process's Path whenever possible
- The standard Linux default shell is `/bin/sh`; this version uses it directly, without probing multiple paths like the Android version
- Files ending with `.sh` or containing `#!` on the first line are recognized as scripts and executed by `/bin/sh`
- When password protection is enabled, credential sources take precedence: `Authorization: Bearer <password>` header > `X-DontCrack-Password` header > URL `?password=` query parameter (kept for backward compatibility, to be deprecated); e.g. `xxx/startup?password=123456`
- Differences from the OpenHarmony / Android versions:
  - Startup banner and root path messages changed to `DontCrack_linux`
  - No longer probes Android default shell paths; hardcodes `/bin/sh`
  - Default log directory changed to relative path `./logs/proc_manager/` (instead of the absolute path `/data/...`)
  - Examples changed to systemd units (`example/dontcrack-edgecore.service`)

#### Platform boundary and porting (exec/ build tags)

The shared logic — process lifecycle, generation checks, exponential backoff, stable-period
reset, restart decisions — exists exactly once, in `exec/process.go`. The only genuinely
cross-platform part is "how to signal the process group", which lives in per-GOOS files:

| File | Build tag | Provides |
| --- | --- | --- |
| `exec/process.go` | none (shared) | `Process` lifecycle, `monitor`, `RestartPolicy.BackoffDelay`, `HealthProbe` |
| `exec/platform_unix.go` | `//go:build unix` | `applyLaunchGroup` (setpgid), `terminateGroup` (SIGTERM/SIGKILL to the group) |
| `exec/platform_windows.go` | `//go:build windows` | `applyLaunchGroup` (CREATE_NEW_PROCESS_GROUP), `terminateGroup` (CTRL_BREAK_EVENT / taskkill /T /F) |
| `exec/probe.go` | none (shared) | probe threshold decision and "graceful stop → force kill", reusing `StopManagedProcess` |

The platform layer exposes only three symbols, so **what adding a platform requires is visible
at a glance**:

1. `applyLaunchGroup(cmd *exec.Cmd)` — put the child into a convergent process group before
   `cmd.Start()` (Unix: `SysProcAttr.Setpgid=true`; Windows:
   `CreationFlags |= CREATE_NEW_PROCESS_GROUP`);
2. `terminateGroup(pid *os.Process, force bool) error` — `force=false` sends the graceful
   signal, `force=true` force-kills and converges the whole process tree;
3. `waitStatus` — a type alias for `syscall.WaitStatus`, used by `monitor` to detect
   termination by signal.

The rule is a single line: **if the platform's GOOS belongs to the Unix family
(linux / android / darwin / ios / *BSD / solaris / illumos …) no change is needed at all**,
because `platform_unix.go` already covers it; otherwise create `exec/platform_<goos>.go`
implementing the three symbols above.

> On Windows the graceful signal is `CTRL_BREAK_EVENT`, which depends on the child's console
> group. `-pre` and probe commands still run through `/bin/sh`, so they do not work on
> Windows — this repo targets Linux; the Windows build exists for development-time
> cross-checking.

### 5. systemd Integration

Put the compiled `DontCrack` into `/usr/local/bin/`, then copy the example service to `/etc/systemd/system/`:

```bash
sudo cp example/dontcrack-edgecore.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now dontcrack-edgecore.service
sudo systemctl status dontcrack-edgecore
```

See the full example at `example/dontcrack-edgecore.service`; remember to adjust the paths, ports, pre-processing script and other arguments in `ExecStart` to match your environment.

### 6. Tips

- When used standalone, if `&` is used at the end of a command in a session, the operation may be killed when the session ends
- Go's `log.Printf` writes to `os.Stderr` by default, so log entries from child processes will show as `[STDERR]`; switch to `fmt.Println` to get non-`[STDERR]` messages
- Since processes can be controlled via encrypted HTTP, you can combine this with system roles, AI + MCP (or Skills) to implement various automation operations
- Besides using direct functionality for process robustness, the repeated restart mechanism can also be used for process polling, and the pre-script can implement delayed startup, waiting for dependent processes, etc.
- `-restart-delay` is the backoff base: during a crash storm the wait grows exponentially
  (`2s → 4s → 8s …`); cap it with `-restart-delay-max` so the log is not flooded. Once the
  child has stayed alive for more than 30 seconds the retry counter resets, so occasional
  crashes never exhaust the `-max-retries` budget.

### 7. Relationship with the other platform repos (stated honestly)

The DontCrack family has **7 repositories locally**: **5 platform repos**
(`DontCrack4ManyLinux` — this one — plus `DontCrack4Windows`, `DontCrack4Mac`,
`DontCrack4OpenHarmonyLinuxKernelSide`, `DontCrack4AndroidLinuxKernelSide`) and 2
non-platform repos (`DontCrack-Manager`, `DontCrack-Desktop`). The 5 platform repos are still
**five independent copies of the same code**, not a shared module.

**This refactor only touched this repository; it was not propagated to the other four platform
repos**, whose code is still pre-refactor. Latest commit per repo (measured with
`git log -1 --date=format:'%Y-%m-%d %H:%M' --format='%cd | %h | %s'`):

| Repository | Latest commit | Commit subject |
| --- | --- | --- |
| `DontCrack4ManyLinux` (this repo) | 2026-11-22 17:35 · `823b7ec` | this refactor: build-tag platform split, restart backoff, graceful probe |
| `DontCrack-Manager` | 2026-11-22 17:35 · `c869c7c` | probe semantics split, Windows process-tree containment, e2e in CI |
| `DontCrack4Windows` | 2026-11-16 19:48 · `1db7a62` | chore: update dependencies for v1.0.20261116 |
| `DontCrack4Mac` | 2026-11-16 19:48 · `20c5eb6` | chore: update dependencies for v1.0.20261116 |
| `DontCrack4OpenHarmonyLinuxKernelSide` | 2026-11-16 19:48 · `67e3c0f` | chore: update dependencies for v1.0.20261116 |
| `DontCrack4AndroidLinuxKernelSide` | 2026-11-16 19:47 · `157b343` | chore: update dependencies for v1.0.20261116 |
| `DontCrack-Desktop` | 2026-11-16 22:09 · `1b8f55c` | chore(deps): update @tauri-apps/api to 2.12.5 |

> So "the other repos are untouched" would be wrong — all four platform repos have their own
> 2026-11-16 dependency-update commits; what they lack is this refactor's backoff / probe /
> process-group convergence work.

Measured `exec/process.go` md5 values (`md5sum exec/process.go`), which show the copy relationship:

| Repository | `exec/process.go` md5 |
| --- | --- |
| `DontCrack4ManyLinux` (this repo, refactored) | `a22250fa854f68a843fcf6ed6cf52b10` |
| `DontCrack4AndroidLinuxKernelSide` | `3cf063ca3e4621fa06d16cefdd6bdee6` |
| `DontCrack4OpenHarmonyLinuxKernelSide` | `3cf063ca3e4621fa06d16cefdd6bdee6` (identical to Android) |
| `DontCrack4Windows` | `1e098e0d985db99d4ca56b44b35a5493` |
| `DontCrack4Mac` | `aebf56a34622078fa8687b44fc18729d` |

This repo's `exec/process.go` md5 was `69f19f353aa5c32c5b8f80957c54afd2` before the refactor
and is now `a22250fa…` as listed above.

Four repos share the same `exec/process_test.go` md5 (`2058cb89…`); only this repo's tests
differ, because of this refactor (this repo: `ee93a139…`). Historically the only real
cross-platform branch was the signal difference inside `StopManagedProcess` (Unix `SIGTERM`
vs Windows `os.Interrupt`).

**Convergence TODO (not done; each needs a separate verified change in its own repo):**

1. `DontCrack4AndroidLinuxKernelSide`: adopt `exec/platform_unix.go` for the signal path, and
   add the graceful probe stop + process-group convergence and exponential backoff with
   stable-period reset.
2. `DontCrack4OpenHarmonyLinuxKernelSide`: same as above (its release.yml builds with
   `GOOS=linux`, so it naturally falls under `//go:build unix`).
3. `DontCrack4Windows`: adopt `exec/platform_windows.go` — replace `os.Interrupt` with
   `CTRL_BREAK_EVENT` and replace `Process.Kill` with `taskkill /T /F`.
4. `DontCrack4Mac`: same as Android (Unix family, covered by `platform_unix.go`).
5. Longer term: extract `exec/` into its own Go module (e.g.
   `github.com/FasterEdge/dontcrack-exec`) and have all five platform repos depend on it,
   removing the five copies at the root. Not started; it needs a migration window agreed with
   each repo's release pipeline and go.mod.

> This change deliberately did **not** sync the other four platform repos "to look consistent" —
> that would present unverified code as verified. The verification results below hold for
> this repository only.

### 8. How this change was verified

```bash
go vet ./...
go test ./...
go test -race ./...
```

Tests covering the new behaviour (package `exec`):

- `backoff_test.go`: backoff sequence `base, 2·base, 4·base`, capping, left-shift overflow
  guard, fallback to the legacy 2s default, stable threshold pinned to 30s
- `probe_test.go`: threshold decision (with `limit<=0` fail-closed), convergence must set
  `stoppedByReq` and bump the generation (otherwise monitor would restart the process),
  nil-defence
- `e2e_signal_test.go`: real child processes prove the graceful signal path (the child exits
  by itself rather than being force-killed), force-kill escalation when the child ignores the
  signal, and process-group convergence (the grandchild receives the same signal). The test
  children listen for both `os.Interrupt` and `SIGTERM`, so the cases hold equivalently on
  Windows and Unix
- `e2e_backoff_test.go`: backoff gaps derived from child start timestamps are exponential and
  the cap really takes effect; the retry counter resets after 35s (>30s) of uptime

Three of the e2e cases were mutation-checked (deliberately breaking the implementation must
make them fail), so the assertions are not vacuous.