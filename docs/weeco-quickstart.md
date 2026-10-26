# weeco 快速开始

`weeco` 是“老板 + 临时团队”的控制面。本文只讲一条最短路径：初始化、招一个设计团队、招一个实施团队、验收、解散。

完整设计见 [references/weeco-company-model.md](../references/weeco-company-model.md)。

## 0. 前提

- macOS / Linux
- Go 1.24+（从源码构建）
- Herdr ≥ 0.9.3
- Claude Code
- git 可选：有 git 时可以用需求级 worktree；没有 git 时用普通目录

## 1. 构建和安装

```bash
git clone <weeco-repo-url> ~/weeco
cd ~/weeco
make build
./bin/weeco install
```

`install` 会写：

| 内容 | 位置 |
|---|---|
| 二进制 | `~/.local/bin/weeco` |
| 老板 skill | `~/.claude/skills/weeco-boss/SKILL.md` |
| 团队负责人 skill | `~/.claude/skills/weeco-team-lead/SKILL.md` |
| slash command | `~/.claude/commands/weeco.md` |
| 模板 | `~/.weeco/templates/{contract.md,result.md,acceptance.json}` |

检查：

```bash
weeco doctor
```

## 2. 初始化项目

```bash
mkdir ~/code/demo && cd ~/code/demo
weeco init
```

`init` 会：

- 在 `~/.weeco/projects/` 注册这个项目
- 在项目根目录 `CLAUDE.md` 写入 `<!-- weeco:begin -->` 块
- 有 git 时记录默认分支；没有 git 也能初始化

这个块告诉主 Claude：

> 你是老板，不做具体任务；需要设计、计划、实施、评审时招团队。

## 3. 提需求

在项目目录里启动 Claude，直接说需求。老板会先和你澄清，然后：

```bash
weeco requirement new --goal "..."
weeco status
```

得到一个需求号，例如 `R-20261026-01`。`weeco status` 会显示当前阶段和 `next_skill`，老板据此加载阶段 skill：

| Skill | 阶段 |
|---|---|
| `weeco-requirements` | 需求澄清 |
| `weeco-design` | 设计团队 |
| `weeco-plan` | 计划团队 |
| `weeco-implement` | 实施团队 |
| `weeco-review` | 等待、验收、返工 |
| `weeco-deliver` | 交付、关闭 |

这些 skill 只规定怎么调用 weeco；具体产出由团队完成。

## 4. 招设计团队

老板写一份设计团队合同，例如 `docs/weeco/contracts/design.md`：

```markdown
# 设计团队合同

## 目标
做一个 Web 记事本的需求/交互设计。

## 交付物
- 需求说明
- 页面和交互说明

## 验收标准
- 覆盖新建、编辑、删除、搜索
- 说明数据如何保存

## 约束
- 不写代码
- 只在指定工作区工作

## 工作区
/Users/me/code/demo

## 团队
- 目的：design
- 人数：2 名员工（不含负责人）
- 角色：产品、交互
```

招团队：

```bash
weeco team recruit --requirement R-20261026-01 \
  --purpose design --contract docs/weeco/contracts/design.md --size 2
```

`team recruit` 会：

- 创建一个临时团队目录 `~/.weeco/projects/<key>/teams/T-.../`
- 把合同复制进去
- 在 Herdr 当前 workspace 开一个团队 tab（负责人 pane）
- 启动团队负责人 agent（bypass 权限）
- 把 team id、合同路径、工作区、团队人数发给负责人

一个团队是一个 tab/panel；员工是负责人用 `weeco member recruit` 招的 Herdr agent，在同一个 tab 内分屏加入，不新开 tab。员工数量不设工具上限。

负责人会为每个员工写任务文件、split 一个 pane、启动员工 agent；员工读任务、写 `members/<agent>/report.md`，负责人用 `weeco member wait/status/accept/rework` 初验。老板可以用 `weeco team status T-...` 看成员表，或在 Herdr 里直接看团队 tab 的多个 pane。

如果这个团队需要机器检查，再写一份 `acceptance.json`，并在 recruit 时加 `--acceptance <file>`：

```bash
weeco team recruit --requirement R-20261026-01 \
  --purpose implement --contract implement.md --acceptance acceptance.json --size 3
```

它会被复制到团队目录，`team check` 在那里运行。

## 5. 等负责人提交

```bash
weeco team wait T-20261026-01
```

在后台运行；它只在 submitted/blocked/lost 或超时时返回。负责人 idle 不代表团队结束，不要因为 agent idle 就停止等待。

可能的结果：

- `submitted`：负责人认为团队交付达到合同要求，等你验收
- `blocked`：负责人需要你决策
- `lost`：负责人 agent 不在了
- `active` 但 agent idle：负责人没有 submit 也没有 ask，需要看 pane

查看：

```bash
weeco team status T-20261026-01
```

## 6. 老板验收

先读：

- 合同：`weeco team status` 里的 `contract`
- 成员表：`weeco team status`；每个员工应该是 accepted 或 closed
- 负责人写的 `result.md`
- 交付物

如果合同目录里有 `acceptance.json`：

```bash
weeco team check T-20261026-01
```

通过：

```bash
weeco team accept T-20261026-01 --note "检查了需求覆盖和交互说明"
```

`accept` 默认关闭整个团队 tab（负责人 + 所有员工 pane），团队进入 `disbanded`；worktree 不会删。

不通过：

```bash
weeco team rework T-20261026-01 --issues "缺少删除确认；搜索范围没说清楚"
```

问题退回原团队。负责人协调员工返工后重新提交；旧 `check.json` 会失效，返工后要重新 check。

## 7. 计划和实施团队

设计验收并解散后，确定要落地实施：

```bash
weeco requirement stage R-20261026-01 --set plan
weeco workspace create --requirement R-20261026-01 --base main
```

- git 项目：创建需求级 worktree（仓库至少要先有一个 commit）
- 非 git 项目：使用项目根目录或 `--path` 指定目录

然后按同样方式招 `plan` 团队、`implement` 团队。同一个需求同一时间只允许一个 active 团队；上一个团队验收解散后再招下一个。

## 8. 关闭需求

全部团队验收后，向客户交付，然后：

```bash
weeco requirement close R-20261026-01 --remove-worktree
```

只有这一步会删除需求级 worktree。外部 `--path` 工作区不会被删除。

## 常用命令

```bash
weeco status
weeco requirement list
weeco requirement show R-...
weeco team list
weeco team status T-...
weeco team nudge T-...
weeco team repair T-...
weeco team answer T-... --decision "..."
weeco team disband T-...
```

## 排查

- `not running inside a Herdr pane`：老板的 `team recruit` 必须从 Herdr 里的主 Claude 会话运行。
- 负责人找不到项目：从团队 pane 里，`weeco lead/member ...` 会用 `HERDR_PANE_ID` 自动解析项目；如果提示没有上下文，再带 `--project <项目根>`。
- 负责人 idle 但没有提交：`weeco team nudge T-...`。
- 负责人丢失或状态对不上：`weeco team repair T-...`，再决定是否 `team disband --force`。
- 员工 pane/agent 丢失：`weeco team repair T-...` 会把成员标为 lost；负责人在团队 pane 里 `weeco member close <agent>`，再用新名字招募替代员工。
- `submitted` 但验收失败：先 `team rework`，不要直接 `accept --force`。
- 负责人丢失：`weeco team disband T-... --force`，然后按同一合同重新招团队。
