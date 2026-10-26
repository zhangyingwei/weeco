---
name: weeco-review
description: Use when a weeco team is active, blocked, or submitted: wait, inspect, run checks, answer, accept, or rework.
---

# weeco：团队验收

## 入口

先运行：

```bash
weeco team status T-...
```

根据状态处理：

```text
active / rework → 等待或看 pane
submitted       → 验收
blocked         → 回答
lost            → 解散后重招
accepted        → 返回阶段出口
disbanded       → 返回阶段出口
```

## submitted：老板验收

1. 读合同：

```bash
weeco team status T-...
```

2. 读团队结果，检查成员表的正式名字：

```text
~/.weeco/projects/<key>/teams/T-.../result.md
```

3. 看 `team status` 的成员表：每个员工都应该是 accepted 或 closed；还在 active/blocked/rework 的成员说明负责人还没完成初验。

> 0.8 之前招募的团队可能没有 member 记录（员工是 subagent）。这类团队按 `result.md` 的成员表验收，不要求 member pane；0.8 起新团队使用 pane 模式。

4. 看实际交付物。

5. 如果合同有 `acceptance.json`：

```bash
weeco team check T-...
```

6. 通过：

```bash
weeco team accept T-... --note "验收依据：..."
```

`accept` 默认关闭整个团队 tab（负责人 + 所有员工 pane）并记录解散；worktree 保留。

7. 不通过：

```bash
weeco team rework T-... --issues "具体问题 1；具体问题 2"
```

问题退回原团队，负责人协调员工返工后重新提交。`rework` 会让旧 `check.json` 失效。

## blocked：回答负责人

```bash
weeco team answer T-... --decision "..."
```

回答后团队回到 active。

## active / rework：等待

```bash
weeco team wait T-...
```

在后台运行；负责人 idle 不代表团队结束。`wait` 只在 submitted/blocked/lost 或超时时返回。退出后先运行 `team status`，再决定 accept/rework/nudge。

如果 wait 超时且仍是 active：

```bash
weeco team nudge T-...
```

它会提醒负责人提交或提问。不要反复重发初始 prompt；也不要老板自己接手干活。

## lost：负责人丢失

先对账：

```bash
weeco team repair T-...
```

如果负责人丢失但员工 pane 还在，`team repair` 会把负责人标为 lost；随后：

```bash
weeco team disband T-... --force
```

`disband` 会关闭整个团队 tab（包括所有员工 pane）。然后按同一合同重新招团队。不要假装团队还在。

## 验收后推进阶段

`accept` 成功后团队会解散，但阶段不会自动前进。按团队 purpose 执行：

```bash
# purpose=design
weeco requirement stage R-... --set plan

# purpose=plan
weeco requirement stage R-... --set implement

# purpose=implement
weeco requirement stage R-... --set delivery
```

然后运行：

```bash
weeco status R-...
```

加载 `next_skill` 指向的阶段 skill。

## 验收规则

- 不接受负责人的口头“done”；看合同、result、证据。
- 检查 `result.md` 的成员表：负责人和每个员工都有正式名字，没有“员工1/agent-1”这类占位名。
- 检查 `team status` 的成员状态：负责人已经接受或关闭每个员工。
- 员工是独立的 Herdr agent，在团队 tab 内分屏；老板不要自己进 pane 替负责人管理员工。
- 不跳过 submitted 直接 accept。
- `--force` 只用于明确知道自己在覆盖什么。
- 只有老板验收；负责人不能验收自己的团队。
- 验收完成后，回到当前阶段 skill 的出口；不确定就运行 `weeco status`。
