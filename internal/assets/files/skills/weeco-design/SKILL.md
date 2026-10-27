---
name: weeco-design
description: Use at the weeco design stage: write a design team contract, recruit the team, review the result, and advance to plan.
---

# weeco：设计阶段

## 入口检查

先运行：

```bash
weeco status R-...
```

要求：

- stage=requirement 或 design
- 没有 open team
- 需求简报存在，甄别结论不是“假需求/伪需求/错误需求”；“待验证”必须有客户确认或调研结论（旧需求没有简报时，先补写或记录跳过理由）

如果 stage 还是 requirement，先切到设计：

```bash
weeco requirement stage R-... --set design
```

其他情况不满足就加载 `next_skill` 指向的 skill，不要硬跑。

## 写设计合同

在项目里写一份合同，例如 `docs/weeco/contracts/R-...-design.md`：

- 目标
- 交付物
- 验收标准
- 约束
- 需求简报：`docs/weeco/requirements/R-...md`（设计团队只读）
- 工作区
- 团队人数（不含负责人）和角色
- 拆分建议

需求简报里的“待设计决定”就是设计团队要回答的问题；合同只引用，不复制。

需要机器检查时，再写一份 `acceptance.json`。

合同由老板写，负责人只读；负责人不能自己放宽验收标准。

## 招团队

```bash
weeco team recruit --requirement R-... \
  --purpose design \
  --contract docs/weeco/contracts/R-...-design.md \
  --size N
```

N 按工作图定：工作单元、依赖、并行链、独立验证；链长不增加 N，耦合会减少 N；N > 3 写 ownership 和合并顺序，N > 5 拆需求。见 `weeco-boss` 定员。

需要机器检查时加：

```bash
--acceptance docs/weeco/contracts/R-...-acceptance.json
```

老板要按工作图的并行链定人数，不按“设计”这个类型默认 2 人。

## 等结果

```bash
weeco team wait T-...
weeco team status T-...
```

在后台运行 `team wait`；它只在 submitted/blocked/lost 或超时时返回，负责人 idle 不代表团队结束。退出后先运行 `team status`。

- `submitted`：加载 `weeco-review`
- `blocked`：用 `weeco team answer` 回答，或看 pane 后回答
- `active`（wait 超时）：看 pane，或 `weeco team nudge T-...`，再重新挂 wait
- `lost`：`weeco team disband T-... --force`，再按同一合同重新招团队

## 出口

设计团队验收并解散后：

```bash
weeco requirement stage R-... --set plan
```

加载 `weeco-plan`。

## 不要

- 不要自己写设计。
- 不要在设计阶段写实现代码。
- 不要在有 open team 时招下一个团队。
- 不要跳过 submitted 直接 accept。
