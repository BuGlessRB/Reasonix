# Windows 关闭与 transcript 诊断验收说明

[English](WINDOWS_CLOSE_TRANSCRIPT_VALIDATION.md)

本文记录 `main-v2` 上 Windows 重复关闭竞态修复及 transcript follow 诊断的验收边界。
未知的 transcript 同步根因尚未定位，因此本文不声称已经修复该未知根因。

## 已实现行为

- 标题栏最小化、最大化、最大化状态查询和关闭直接通过 preload IPC 交给 Electron。
  仍使用旧 Go 窗口方法名的调用方由通用 invoke 层转到同一个原生窗口所有者。
- `QuitSequencer` 统一处理窗口关闭、应用退出、系统退出和更新重启。重复请求复用同一次
  草稿准备和服务收尾；后台关闭策略检查期间到达的应用退出会把当前事务升级为真实退出。
- 服务开始 shutdown 时立即发布 `stopping`，公开的 `ready` 同时变为 false；新的业务调用
  返回“正在退出”，shutdown/status 仍通过当前服务会话完成。
- 服务进入 `stopping` 后，transcript follower 停止且不再发送订阅清理 RPC；旧世代的在途
  响应不能更新已经显示的聊天内容。
- transcript 故障以受限字段记录 stage、reason、错误类型、传输类型、revision、commit、活动 attempt
  数、失败次数、持续时间、服务阶段和服务 generation。相同故障每 30 秒最多输出一次可见
  汇总，原因变化和恢复立即记录。renderer → shell 端点拒绝自由文本、未知字段、超过 2 KiB
  的载荷、不可信发送者，以及每秒超过十条的已接收事件。

长期诊断以轮转的 `shell.log` 为准。启动日志包含 shell 版本、channel、commit、PID 和运行
generation；握手成功日志包含服务构建、PID 和 generation；退出日志包含 shutdown requestId、
触发原因、草稿保存耗时、服务收尾耗时、总耗时和结果。

`%APPDATA%\reasonix\diagnostics\lifecycle\` 中的文件是临时生命周期证据。正常退出会删除
当前运行对应的文件；策略关闭诊断或使用开发构建时也可能不创建文件。因此正常退出后目录
为空属于预期行为。退出后的调查应采集 `%APPDATA%\reasonix\logs\shell.log` 和 `service.log`。

## Windows 正式包验收

使用同一个最终安装包或便携包，隔离 `REASONIX_HOME`，仅使用包内服务，不启用开发握手绕过。

1. 确认严格 shell/service 握手成功，并完成真实 renderer `Version` 调用。
2. 交叉执行连续标题栏关闭、Alt+F4、托盘退出、后台隐藏和重新打开；使用测试夹具暂停草稿
   准备后重复这些动作。
3. 在存在未发送草稿、活动会话和多个标签时重复测试，重启后核对所有持久状态。
4. 使用确定性夹具暂停服务收尾，再次重复关闭和退出；确认只有一个 requestId 和一次收尾事务。
5. 确认 shell 与服务均退出，无崩溃覆盖层、未处理 rejection 或残留进程。
6. 分别注入本地与远程 transcript 故障；确认 `shell.log` 和复制的崩溃文本包含稳定 stage/reason，
   并能通过构建 commit、服务 generation 和绝对发生时间关联。
7. 确认正常退出后 lifecycle 目录可以为空，而轮转日志仍然保留。

在 Windows 原生矩阵完成前，状态只能写为“实现和本地测试完成”，不能写为“Windows 问题验证通过”。

## 独立迁移调查交接

历史归属迁移不属于本次改动。最小脱敏模型是：3 个会话登记在 workspace **A**，一个待处理
导入操作指向 workspace **B**。后续调查需要对照 workspace registry 中的 membership/lifecycle
记录，以及 migration ledger 中的来源映射、目标、操作 revision 和完成回执。在目标归属得到
证明前，不移动、复制或删除会话及待处理操作。
