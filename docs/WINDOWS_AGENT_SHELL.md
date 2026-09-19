# Windows Agent shell / Windows Agent 解释器

Windows Agent commands use PowerShell. PowerShell 7 is preferred; Windows PowerShell 5.1 remains supported. Valid configured PowerShell paths are retained. Legacy Git Bash preferences remain readable without rewriting the saved configuration, but do not select the Windows Agent interpreter. Remote execution uses the remote host's operating system.

Windows Agent 命令使用 PowerShell，优先选择 PowerShell 7，同时支持 Windows PowerShell 5.1。有效的 PowerShell 配置路径会保留。历史 Git Bash 偏好仍可读取且不会自动改写，但不再决定 Windows Agent 的解释器。远程执行根据远端主机操作系统选择解释器。

The provider-visible Windows shell tool is `pwsh`. Each foreground call runs in a fresh PowerShell process, so directories, variables, functions, and environment changes do not carry into the next call. Use `run_in_background=true` for servers and watchers; it returns a `pwsh-*` job id that can be read with `job_output` and stopped with `job_kill`. PowerShell 5.1 does not support `&&` or `||`; portable calls should use `;` or `if ($?) { ... }`.

Windows 向 provider 暴露的 shell 工具名为 `pwsh`。每次前台调用都使用全新的 PowerShell 进程，因此目录、变量、函数和环境修改不会带入下一次调用。服务器和 watcher 应设置 `run_in_background=true`，调用会返回 `pwsh-*` job id，可用 `job_output` 读取、用 `job_kill` 停止。PowerShell 5.1 不支持 `&&` 或 `||`；兼容写法应使用 `;` 或 `if ($?) { ... }`。

For a local OpenMAIC checkout whose start command is `npm run start`, a
background call looks like this (replace the directory and start command for
the deployment):

```json
{"command":"Set-Location 'C:\\OpenMAIC'; $env:PORT='3000'; npm run start","description":"Start OpenMAIC service","run_in_background":true}
```

Read its incremental log or wait for a terminal state with
`job_output({"job_id":"pwsh-1","wait":false})`, and stop the whole process
tree with `job_kill({"job_id":"pwsh-1","reason":"service no longer needed"})`.

如果本地 OpenMAIC 的启动命令是 `npm run start`，可以按上面的方式后台启动；
请按实际部署替换目录和启动命令。返回 `pwsh-*` 后，使用 `job_output` 读取增量
日志或等待终态，使用 `job_kill` 停止完整进程树。

Reasonix no longer runs a nested shell/child-process preflight before each Windows command. The restricted-token runner executes the requested command directly. A private inherited report handle carries pre-command `dependency`, `authorization`, and `launch` failures; command stdout/stderr cannot claim that execution never started. The handle is not inherited by the restricted child, and its temporary backing file cannot be reopened by name and is deleted on close. Foreground and background runtime permission denials offer the same exact-command, single-use `denial_id` retry after explicit approval; runtime failures remain `execution` with possible partial effects. Reasonix never retries outside the selected sandbox by itself.

Reasonix 不再在每条 Windows 命令前运行嵌套 shell／子进程预检。restricted-token
runner 直接执行请求的命令，通过独立继承的诊断句柄报告启动前的 `dependency`、
`authorization` 和 `launch` 失败。命令输出无法伪造“未执行”状态；受限子进程
不会继承诊断句柄，临时诊断文件禁止按路径重新打开，并在关闭时删除。
前台和后台的运行期权限拒绝均返回绑定原命令、单次使用的 `denial_id`，供用户
明确批准后精确重试；运行期失败仍保持 `execution` 和可能已部分修改的状态。
Reasonix 不会自动绕过沙箱。

An unconfirmed submission is not evidence that sending failed. Reconnect and inspect the session before sending again. Tool timeouts and cancellations take priority over legacy zero exit codes; missing results are shown as unknown.

提交结果未确认不代表发送失败。请先重连并检查会话，再决定是否重新发送。工具超时和取消状态优先于历史零退出码；缺少结果时显示未知状态。
