---
name: weeco-plan
description: Use at the weeco plan stage: create the requirement workspace, recruit a planning team, review the plan, and advance to implement.
---

# weeco：计划阶段

## 入口检查

```bash
weeco status R-...
```

要求：

- stage=plan
- 没有 open team

## 准备需求工作区

计划需要看代码时，先在 git 项目里建需求级 worktree：

```bash
weeco workspace create --requirement R-... --base main
```

- git 项目：worktree 在 `~/.weeco/worktrees/<key>/R-...`
- 非 git 项目：使用项目根目录或 `--path` 指定目录
- worktree 属于需求，不属于团队；团队解散不删

## 写计划合同

例如 `docs/weeco/contracts/R-...-plan.md`：

- 目标
- 交付物：实施计划、任务拆分、风险
- 验收标准
- 约束
- 工作区
- 团队人数和角色

需要机器检查时写 `acceptance.json`，recruit 时用 `--acceptance` 传入。

## 招计划团队

```bash
weeco team recruit --requirement R-... \
  --purpose plan \
  --contract docs/weeco/contracts/R-...-plan.md \
  --size 2
```

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

计划团队验收并解散后：

```bash
weeco requirement stage R-... --set implement
```

加载 `weeco-implement`。

## 不要

- 不要自己写计划。
- 不要在计划阶段改业务代码。
- 不要跳过设计阶段而没有记录理由。
- 不要在有 open team 时招下一个团队。
