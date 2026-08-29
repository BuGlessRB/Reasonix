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
| HostDowngrade / Accepted | 0/14 |
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

## 解释

**措辞不是观测到的瓶颈。** 每一个被判定的子代理都尝试并最终产出了被接受的报告（14/14），但 11/14 反复调用它，10/14 在报告之后继续正常使用工具。因此观测到的失败形态是终止语义的丢失：`complete_subtask` 表现得像一次里程碑汇报，因为主机接受它却不关闭子代理的执行纪元。

这排除了「把提示写得更清楚」这条解释——契约原文已经是最直接的措辞：

> End this sub-task by calling complete_subtask exactly once, as your final tool call.

而 `complete_subtask` 工具本身没有任何防重或终止逻辑，每一次重复调用都被接受。缺陷在于契约只存在于散文里，没有结构强制。

按事先约定的门槛（`Attempted / Judged ≥ 95%` 即不加 nudge），本轮结论是**不修改子代理提示**。

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
