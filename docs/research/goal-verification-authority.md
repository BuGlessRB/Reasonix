# 谁拥有一个运行中 Goal 的验收权

> 日期：2026-08-29
> 状态：设计契约，尚未实现。四条规则先钉死，测试围绕它们写。
> 定级：P1 correctness / low-frequency integrity boundary

## 已证明可达的状态

Goal 在声明 A 下启动，捕获 A 并随 goal 状态落盘；进程重启后新进程读到声明 B；恢复该 Goal 时旧判据被恢复进来。

```text
Goal frozen contract = A
current project declaration = B
```

证据：`internal/control/goal_resume_project_check_test.go`（全程只走 `Submit` 与 `resume` 两个生产入口，未对任何字段赋值）。同一进程内与 `--continue` 无 goal 两条路径均已在真机证伪，见 `benchmarks/project-check/README.md`。

**已证到的是两次 Goal 执行之间的外部声明变更。** agent 会改写声明、且改写能存活到下一进程，这一点 benchmark 单独证明过；把两半拼成"agent 自删考卷"合理但尚未在一次运行里证明。

## 当前两种实现都在替用户猜

```text
legacy:      看到 B → 自动忘掉 A
obligation:  看到 A 与 B → 自动要求 A + B
```

同一个状态既可能来自 agent 改写自己被考的判据（保留 A 才对），也可能来自 owner 在两次运行之间改了项目政策（A 可能确实该废止）。**现有 durable 状态不足以区分这两者**，所以任何自动策略都是在猜。

## 四条规则

1. **一个 Goal 拥有自己的冻结验收契约。** 创建时按 `VerificationIdentity` 集合冻结，随 Goal checkpoint 持久化，带 epoch。
2. **工作区声明的变化不会自动改变一个活跃 Goal 的验收权。** 声明变化之后只是"当前项目声明"。
3. **baseline ≠ current 产生的是 authority conflict，不是义务的自动并集。** 此时欠的不是"跑 A"，而是"这个冲突尚未解决"，finalization 因此受阻。
4. **只有显式的 host-owned rebase 能取代冻结契约，且取代不等于满足。** A 被 supersede 不使 B 通过；B 仍需在新 epoch 下有效的证据。

## 这把 provenance 难题换成了 capability 问题

不要去建立「改动由 agent 发起 → preserve，由人发起 → rebase」的判定。IDE 外部编辑、`git checkout`、后台进程、启动脚本、崩溃后未落盘的 receipt——provenance 在这些情形下都不可靠。

更可信的是一条边界：

> **文件写权限 ≠ authority 写权限。**

写 `REASONIX.md` 只改变工作区，不改变活跃 Goal 的冻结验收契约；owner 若确实想改这个 Goal 的验收，走显式的 Goal rebase 动作。这样 agent 自改与 owner 自改的模糊性一并消失，而且不需要知道是谁改的。

同理，V1 **不给 agent** 任何 `rebase_verification_contract` 能力。将来至多允许"请求"，由 host/user 批准后才发生 epoch 转移。

## 最小状态机

```text
Goal starts → Epoch 1 {A}
                  │  restart, current declaration {B}
                  ▼
                Drift  ── finalization blocked, authority unresolved
            ┌─────┴─────┐
        Preserve      Rebase
        Epoch 1 {A}   Epoch 2 {B}，A superseded
            │             │
         verify A      verify B
            └─────┬─────┘
               Complete
```

## 故意不做的

- **Merge（A+B 同时要求）**：没有 observed requirement，Preserve 与 Rebase 两个明确动作已足够。
- **把 `ObligationBaselineCheck` 直接升为最终权威**：它当前的形状隐含"A 一定继续有效"，而可达性证明之后我们知道这不一定对。它应当逐步变成携带 baseline identities、current identities 与 goal epoch 的 drift 义务。
- **切换 `missingProjectChecks` 到 obligation derivation**：原来的迁移提案建立在"已确认 bypass"之上，那个前提已被更正。

## 随后要写的测试

| 测试 | 断言 |
| --- | --- |
| 可达性（已有） | frozen=A 与 current=B 生产可达 |
| drift 需要裁决 | 恢复后 finalization 受阻，理由是 authority unresolved，**既不是欠 A 也不是欠 B** |
| Preserve 能清债 | 裁决为 Preserve 后跑 A 即可完成——把 bypass 修成 deadlock 不算修好 |
| Rebase 能清债 | 裁决为 Rebase 后 A superseded、B 被欠，只跑 B 即可完成 |
| 不变量 | 没有显式 host rebase 时，工作区内容变更**永远**不能替换冻结契约 |

最后一条不需要知道文件是谁改的，因此它是这套设计里最稳的一条。

## 定级理由

不到 P0：没有高频或普通路径的证据。高于普通 P2：Goal durability 是明确支持的生产 lifecycle，而恢复后当前实现会**静默选择**一套验收权——静默更换正在执行任务的验收规则是 integrity 问题，不是边角情形。
