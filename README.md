# weeco

**把项目组织成一家软件公司：老板招团队，团队负责人招员工。**

`weeco` 是一个跑在 Herdr + Claude 上的公司/团队控制面。它不规定工作内容，只规定一种工作模式：

```text
客户
  ↓ 需求
老板（主 Claude 会话）
  ↓ 招临时团队
团队（一个 Herdr tab/panel）
  ├─ 负责人 pane（Herdr agent）：招员工、拆分、分配、监督、初验、协调返工
  └─ 员工 panes（Herdr agent，同一 tab 内分屏）：干活
```

- 老板不做具体任务；设计、计划、实施、评审都招团队。
- 团队规模按工作图（并行链 + 必需独立验证）定，不按任务类型默认 2 人。
- 每个团队成员（包括负责人）都有正式名字；员工另有 ASCII agent name，名字写进任务和 `result.md`。
- 两级验收：负责人验收员工；老板验收团队。不合格退回原团队返工，合格才解散。
- 团队是临时的；一个团队一个 Herdr tab/panel，负责人和员工都是 Herdr agent，员工在同一个 tab 内分屏加入；worktree 按需求拆，团队解散不删 worktree。
- 没有 Dev Playbook / Design Playbook；[team 的机制是通用的](internal/assets/files/skills/weeco-boss/SKILL.md)。

## 快速开始

```bash
# 1. 构建并安装
make build
./bin/weeco install

# 2. 初始化项目（git 可选）
mkdir ~/code/demo && cd ~/code/demo
weeco init

# 3. 在项目里启动 Claude，直接说需求
#    CLAUDE.md 会告诉 Claude：它是老板，不自己做任务
```

安装后：

- 老板 / 阶段路由：`weeco-boss`
- 阶段 skill：`weeco-requirements`、`weeco-design`、`weeco-plan`、`weeco-implement`、`weeco-review`、`weeco-deliver`
- 团队负责人：`weeco-team-lead`
- slash command：`/weeco`
- 模板：`~/.weeco/templates/{requirements.md,contract.md,result.md,acceptance.json,lessons.md}`

## 命令

老板：

```bash
weeco init
weeco trust .
weeco status
weeco requirement new --goal "..."
weeco requirement list|show|stage|close
weeco workspace create --requirement R-...
weeco team recruit --requirement R-... --contract contract.md --size N
weeco team list|status|wait|nudge|repair|check|accept|rework|answer|disband
```

## 阶段 Skill

`weeco status` 会给出 `next_skill`，老板按阶段加载：

| Skill | 什么时候用 |
|---|---|
| `weeco-requirements` | 需求澄清、建 requirement |
| `weeco-design` | 设计团队 |
| `weeco-plan` | 计划团队 + 需求工作区 |
| `weeco-implement` | 实施团队 |
| `weeco-review` | 等待、验收、返工、回答 |
| `weeco-deliver` | 交付、关闭需求 |

这些是**调用 runbook**，只规定什么时候调用哪条 weeco 命令；具体设计/计划/代码由团队完成，不是领域 playbook。

团队负责人（由 `team recruit` 启动，在自己的 pane 里运行）：

```bash
weeco member recruit --name "正式名字" --agent ascii-name --role "角色" --task <task-file>
weeco member status|wait|accept|rework|close
weeco lead status|submit|ask
```

负责人命令通过 `HERDR_PANE_ID` 自动解析 project/team；`member recruit` 会在团队 tab 内 split 员工 pane、启动 Herdr agent；上下文缺失时才带 `--project` / `--team`。

## 状态

```text
团队：draft → active → submitted → accepted → disbanded
                 ↑  ↓
             blocked / rework

成员：active → accepted / rework / closed / lost
```

- `team accept` 默认关闭整个团队 tab（负责人 + 所有员工 pane）并记录解散；worktree 保留；同时写 `acceptance.md`，并记录 lessons 是否更新。
- `team rework` 把问题退回原团队，并清掉旧检查结果；返工后必须重新 check。
- `lead submit` 要求负责人先接受或关闭所有员工。
- `team wait` 在后台运行，等待 submitted/blocked/lost 或超时；不会因为负责人 idle 就返回。
- 负责人在团队 pane 里运行 `weeco lead submit/ask` 时，通过 pane 上下文自动解析 project/team，不用重复带 flags。
- 负责人 idle 但没提交时用 `team nudge`；负责人丢失或状态不一致时用 `team repair`。
- `requirement close --remove-worktree --lessons-reviewed` 才删除需求级 worktree；不加 `--lessons-reviewed` 会警告。

## 目录

```text
cmd/weeco               CLI 入口
internal/model             Project / Requirement / Team / Member 状态
internal/store             项目注册、账本、fold、路径、锁、pane 上下文
internal/herdr             Herdr pane split / tab / agent 适配
internal/workspace         可选 git worktree / 普通目录
internal/commands          init / requirement / team / member / lead / install / doctor
internal/assets            嵌入的 skill、prompt、slash command、模板
```

旧的任务模型（`Task`、`checks.json`、commit trailers）已删除。新版本不强制 git、不强制 checks、不强制 commit trailer；这些只是某个合同里的可选约定。

## 文档

- [公司模型设计](references/weeco-company-model.md)
- [快速开始](docs/weeco-quickstart.md)
- [Agent 舰队参考文档](references/README.md)

## 测试

```bash
go test ./...
```

集成测试用 `internal/commands/testdata/fakeherdr` 模拟 Herdr，不需要真实 Claude。
