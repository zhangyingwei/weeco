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
- 工作区
- 团队人数（不含负责人）和角色
- 拆分建议

需要机器检查时，再写一份 `acceptance.json`。

合同由老板写，负责人只读；负责人不能自己放宽验收标准。

## 招团队

```bash
weeco team recruit --requirement R-... \
  --purpose design \
  --contract docs/weeco/contracts/R-...-design.md \
  --size 2
```

需要机器检查时加：

```bash
--acceptance docs/weeco/contracts/R-...-acceptance.json
```

老板要按设计工作量决定团队大小：小任务小团队，大任务大团队。

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
