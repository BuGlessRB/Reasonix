# 子代理 handoff 协议基线（首次可测量）

> 日期：2026-08-29
> 状态：冻结的观测结果，不是设计提案
> 前置条件：`3f02f9fc7` 注册 SubagentHandoff audit 通道，`3723ff454` 修好丢弃它的子代理 sink。在这两个提交之前，本文的数据在物理上无法采集——所有子代理 audit 都被一个只实现 `Emit` 的包装器吞掉了。

## 研究问题

被委派的子代理，在没有任何额外提示压力的情况下，是否自然遵守 `complete_subtask` 协议？

## 结果

判定分母是 **Judged = `expected && exit == "completed"`**：被 provider 杀掉的子代理从未走到收尾那一步，把它算成拒绝会让分母回答另一个问题。

| 指标 | 结果 |
| --- | --- |
| Judged / Expected | 14/14 |
| Attempted / Judged | 14/14 |
| NeverAttempted / Judged | 0/14 |
| Accepted / Attempted | 14/14 |
| Malformed / Attempted | 1/14 |
| HostDowngrade / Accepted | 0/14 —— 见下方更正 |
| 报告后仍调用其他工具 | 10/14 |
| **重复调用 `complete_subtask`** | **11/14** |
| 报告真正结束了运行 | 3/14 |

调用次数分布 `{1:3, 2:4, 3:3, 4:1, 5:2, 6:1}`，且与运行长度单调相关：

```text
final_round= 2 → 1 次报告，之后 0 次工具调用
final_round= 7 → 3 次报告，之后 2 次
final_round=12 → 4 次报告，之后 5 次
final_round=16 → 5 次报告，之后 12 次
final_round=24 → 5 次报告，之后 12 次
```

## 更正（2026-08-29，同日）

上表两处需要更正，两处都由后续的 terminal-barrier 重放查出：

**一、`HostDowngrade 0/14` 只描述最后一份报告。** `observeSubagentHandoff` 取的是
`LatestCompletionReport()`，所以早先报告上的降级不可见。按全部 39 次被接受的调用重算：
**25 次被裁决为 `partial`，25 次输出含「the host lowered N unbacked criterion claim(s)」**，
只有 14 次维持 `complete`。审计字段本身没写错，但把它当作「子代理的声明都被证据支撑」会读反。

**二、「模型把报告当里程碑」这个解释被更强的证据取代。** 见下节。

## 解释

**措辞不是观测到的瓶颈。** 每一个被判定的子代理都尝试并最终产出了被接受的报告（14/14）。

重复调用不是模型误解了终止语义，而是**主机自己要求的**。首份报告的裁决结果与之后是否继续工作完全对应，14 个子代理零例外：

| | 首份报告被降级 | 未被降级 |
| --- | --- | --- |
| 之后继续工作 | 10 | 0 |
| 之后就此停止 | 0 | 4 |

`complete_subtask` 在降级时返回的正是：

> complete_subtask accepted: status=partial … — the host lowered N unbacked criterion claim(s); **run the check or cite what you really did**

而提示里的契约同时说：

> End this sub-task by calling complete_subtask **exactly once, as your final tool call**.

两句话互相矛盾：工具让子代理回去补检查再报一次，契约说只能报一次。模型服从了更晚、更具体的那一条，这是合理行为，不是协议失灵。`complete_subtask.go` 的注释说明这是刻意设计——「The submission always succeeds: the parent is better served by a claim it can see the host lower than by a rejection the parent never learns about」——但「总是 accepted」加上「去把检查跑了」，等于用 accepted 这个词表达了一次拒绝。

缺陷因此不是「契约缺少结构强制」，而是**裁决结果不是一个状态**：一份被降级的报告与一份原样通过的报告，返回的都是 `accepted`。

按事先约定的门槛（`Attempted / Judged ≥ 95%` 即不加 nudge），本轮结论是**不修改子代理提示**。

## Terminal-barrier 反事实（同一数据集的派生研究）

问题：子代理**首次被接受的** `complete_subtask` 是否已经是安全的终止点？

以首次被接受的报告为锚点，用宿主自己的分类器（`evidence.ToolCallMutationClass` / `CommandRunsVerification`）判定其后的每一次调用：

| 判定 | 子代理数 |
| --- | --- |
| PREMATURE_MUTATION（其后有路径可证的改动） | 1 |
| PREMATURE_OPAQUE_WORK（其后有范围不明的命令） | 7 |
| PREMATURE_VERIFICATION | 0 |
| SEMANTIC_REPORT_EVOLUTION（后续报告实质变化） | 2 |
| SAFE_REDUNDANT | 4 |

**结论：首次被接受即无条件终止是错的。** 它会在 10/14 的情况下截掉**宿主自己要求的**工作。

报告后的 47 次 bash 绝大多数是即席断言与巡查（`python3 -c "… assert …"`、`grep`、`git diff`、`[ … ] && echo PASS`），路径可证的真实改动只有 4 次且集中在一个子代理。宿主的 `CommandRunsVerification` 只认识测试运行器，认不出这些断言，所以 opaque 一栏是**未能分类工作的上界**，不是「它们写了东西」的断言。

两个未决的设计方向由此产生，本轮不做选择：让被降级的报告**返回拒绝而不是 accepted**（这样「exactly once」重新为真），或者引入一个由宿主裁决可关闭性的子代理纪元。

原始与派生证据：`<state-home>/research/subagent-terminal-barrier/2026-08-29/`，其 manifest 引用上游数据集的 manifest 哈希，不复制原始数据。

一个未能回答的问题：`protocol_recovery` 记录不带子代理 id，所以「后续工作是宿主恢复导致」这一类无法归因，判定集合里没有 HOST_RECOVERY。

## 限制

- 只有 14 个被判定的子代理；不适合做跨版本百分比比较。
- 其中 10 个来自被明确要求委派的任务。上述合规率是「子代理一旦存在」时的合规率，不能外推到模型自发委派的场景。
- read-only 子代理只有 3 个，且全部来自同一个 fanout 任务。「writer 因终止链更长而更容易漏掉最后一次转换」这一假设在现有数据中**未被支持**（两组 attempted/accepted 均为满分），但样本不足以否定它。
- 每个任务单次运行，语料本身是随机的。

## 单独登记的开放问题：自发委派率

同一批运行里，6 个未作要求的任务只产生了 1 个子代理，5 个明确要求委派的任务产生了 10 个。

这是一个独立的研究问题（委派策略），**不要和 handoff 终止语义混进同一个实验**。

## 原始证据

不进仓库：trajectory 按其包注释「与会话记录同等敏感」，四个批次合计约 3.2 MB。

```text
<state-home>/research/subagent-handoff/2026-08-29/
  batch-01-fanout-pilot/      batch-03-delegation-next2/
  batch-02-delegation-first7/ batch-04-delegation-last2/
  manifest.json               handoff.py
```

四个批次分开保存，因为预算截断本身就是 provenance：批次 02（800k 默认预算）和 03（1.2M）都被中止并跳过了剩余任务，批次 04 取消上限才跑完语料。`manifest.json` 钉住 revision、模型、判定口径与每个原始文件的 sha256；`handoff.py` 对任一批次目录重跑即可复现上表。
