---
name: weeco-requirements
description: Use at the start of a weeco requirement: clarify the client's need, create the requirement, and hand off to design/plan/implement.
---

# weeco：需求澄清

## 入口

- 没有 requirement，或用户刚提出一个新需求。
- 先运行 `weeco status`，确认没有正在处理的同一需求。

## 做什么

1. 和用户澄清需求：目标、范围、不做什么、验收直觉。
2. 用简短的话复述需求，确认没有理解错。
3. 创建需求：

```bash
weeco requirement new --goal "..."
```

4. 记录 `R-...`，把阶段设为设计：

```bash
weeco requirement stage R-... --set design
```

5. 加载 `weeco-design`。

## 可以跳过设计吗

可以，但要有理由，并记录阶段：

```bash
weeco requirement stage R-... --set plan
# 或
weeco requirement stage R-... --set implement
```

然后加载对应阶段 skill。不要因为“想快”跳过澄清。

## 不要

- 不要在需求没确认前招团队。
- 不要自己写设计方案或代码。
- 不要把用户的每一句话都扩写成需求；先确认目标。
