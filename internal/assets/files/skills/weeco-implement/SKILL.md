---
name: weeco-implement
description: Use at the weeco implement stage: recruit an implementation team, review the implementation, and advance to delivery.
---

# weeco：实施阶段

## 入口检查

```bash
weeco status R-...
```

要求：

- stage=implement
- 没有 open team
- git 项目必须已经有需求工作区

如果 git 项目还没有 workspace：

```bash
weeco workspace create --requirement R-... --base main
```

## 写实施合同

例如 `docs/weeco/contracts/R-...-implement.md`：

- 目标
- 交付物
- 验收标准
- 约束：只在工作区改，不 push，不碰主分支
- 工作区
- 团队人数和角色
- 和计划阶段的对应关系

需要机器检查时写 `acceptance.json`，recruit 时用 `--acceptance` 传入。

## 招实施团队

```bash
weeco team recruit --requirement R-... \
  --purpose implement \
  --contract docs/weeco/contracts/R-...-implement.md \
  --size N
```

N 按工作图定：工作单元、依赖、并行链、独立验证；链长不增加 N，耦合会减少 N；N > 3 写 ownership 和合并顺序，N > 5 拆需求。见 `weeco-boss` 定员。

团队负责人负责拆分、分配、监督、初验和返工；老板不替负责人拆任务，也不在 pane 里写代码。

## 等结果

```bash
weeco team wait T-...
weeco team status T-...
```

在后台运行 `team wait`；它只在 submitted/blocked/lost 或超时时返回，负责人 idle 不代表团队结束。退出后先运行 `team status`。

- `submitted`：加载 `weeco-review`
- `blocked`：`weeco team answer`
- `active`（wait 超时）：看 pane，或 `weeco team nudge T-...`，再重新挂 wait
- `lost`：`weeco team disband --force`，重新招团队

## 出口

实施团队验收并解散后：

```bash
weeco requirement stage R-... --set delivery
```

加载 `weeco-deliver`。

## 不要

- 不要自己写代码。
- 不要在验收前 merge/push。
- 不要在有人工 open team 时招下一个团队。
- 不要把“负责人说完成”当成完成；必须走 submitted + accept。
