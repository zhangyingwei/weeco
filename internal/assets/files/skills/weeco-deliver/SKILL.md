---
name: weeco-deliver
description: Use at the weeco delivery stage: confirm all teams are done, deliver the result, and close the requirement.
---

# weeco：交付和关闭

## 入口检查

```bash
weeco status R-...
weeco requirement show R-...
```

要求：

- stage=delivery，或所有团队都已 accepted/disbanded
- 没有 open team

如果有 open team，先加载 `weeco-review`，不要强行关闭。

## 交付

1. 汇总各团队结果，向客户交付。
2. 需要合并/合并请求时，由老板协调；weeco 不自动 merge。
3. 确认实施结果在需求工作区里，且没有未处理的返工。

## 关闭需求

```bash
weeco requirement close R-... --remove-worktree --lessons-reviewed
```

- `--lessons-reviewed` 表示已检查/更新 `docs/weeco/lessons.md`；不加会警告。
- 只有这一步会删除工具创建的需求级 git worktree
- 外部 `--path` 工作区不会被删除
- 团队目录和 ledger 保留，便于追溯

## 不要

- 不要为了关闭需求跳过未验收的团队。
- 不要在关闭前手动删 worktree 或改 ledger。
- 不要在交付阶段自己改代码；需要修复就开新的修复团队。
