# weeco 公司模型

> 状态：v0.8 实现。本文描述当前代码实际采用的模型，不是未来设想。

## 1. 一句话

把项目组织成一家软件公司：**老板招临时团队，团队负责人招员工；负责人验收员工，老板验收团队；合格解散，不合格退回原团队返工。**

## 2. 角色

| 角色 | 是谁 | 职责 | 不做什么 |
|---|---|---|---|
| 客户 | 人 | 提需求、最终确认 | 不逐项验收员工 |
| 老板 | 主 Claude 会话 | 需求澄清、主流程、任务书、招团队、最终验收、解散、交付 | 不做设计、计划、代码 |
| 团队 | 临时执行单元 | 一个 Herdr tab/panel | 不常驻 |
| 负责人 | 被老板启动的 Herdr agent | 招员工、拆分、分配、监督、初验、协调返工 | 不替员工产出；不验收自己的团队 |
| 员工 | 团队 tab 内的 Herdr agent（独立 pane） | 执行并交付 | 不直接找老板 |

老板和负责人都是管理者，不是执行者。小任务找小团队，大任务找大团队；老板不自己做任务。

### 成员命名

每个成员（包括负责人）都有正式名字。负责人在招募前给自己和员工定名，并把名字用于任务分配、员工报告路径和 `result.md` 的成员表。正式名字在团队内唯一，返工保持不变；换人时新成员取新名字，并记录替换关系。

Herdr agent 名只接受 ASCII，因此每个员工另有一个 ASCII agent name（如 `lin-yi`），正式名字本身可以是中文。负责人技术 agent 名仍是 weeco 分配的 `t-...-lead`。员工 pane 的标题用正式名字。

`weeco member recruit` 会校验正式名字（非占位、团队内唯一）和 agent name（ASCII、唯一）；`lead submit` 要求负责人先接受或关闭所有员工。老板在验收时检查 `team status` 和 `result.md` 的成员表。

## 3. 两层验收

```text
负责人验收员工
  ├─ 不合格 → 负责人协调员工返工
  └─ 合格 → 提交老板
       老板验收
         ├─ 合格 → 团队解散
         └─ 不合格 → 问题退回原团队 → 负责人协调返工 → 再提交
```

- 返工期间团队不解散。
- 老板验收不合格，默认退回原团队，不重新招团队。
- 只有团队确实不胜任、负责人丢失或方向错误，才换团队。
- 老板不能只听说“done”；必须按合同看结果和证据：合同、树/commit、命令输出、真实路径、工件哈希、范围；不能只看 `result.md`。
- 高风险需求（安全/数据/公共 API/支付/生产发布/跨模块核心改动）强制独立 review 团队；review 团队只读，老板仍做最终验收。
- 负责人用 `weeco member accept/rework` 验收员工，成员状态进入台账；`lead submit` 会检查没有 active/blocked/rework 的成员。

## 4. 实体

```text
Project        一个项目
Requirement    一个需求；持有 workspace/worktree
Team           临时团队；一个 tab/panel；purpose、contract、size、state
Member         团队员工；Herdr agent、pane、正式名字、角色、报告
Contract       任务书（team 目录下的 contract.md）
Result         团队交付报告（team 目录下的 result.md）
Check          可选的机器验收结果（team 目录下的 check.json）
Event          项目级 append-only 事件
```

### 团队状态

```text
draft → active → submitted → accepted → disbanded
          ↑  ↓         │
       blocked         ↓
                    rework
```

- `draft`：团队目录和合同已建，负责人未启动
- `active`：负责人和员工在工作
- `blocked`：负责人向老板提问
- `submitted`：负责人提交，等老板验收
- `rework`：老板驳回，退回原团队
- `accepted`：老板验收通过
- `disbanded`：团队关闭，workspace/worktree 保留
- `lost`：负责人 agent 丢失

### 成员状态

```text
active → accepted
   │  ↕
 blocked / rework
   │
 closed / lost
```

- `active`：pane 和 agent 在运行，负责人尚未验收
- `accepted`：负责人验收通过
- `blocked`：员工在 pane 里等待负责人处理
- `rework`：负责人要求返工
- `closed`：pane 已关闭（换人/清理）
- `lost`：员工 agent 或 pane 丢失

`lead submit` 要求团队内没有 active/blocked/rework 的成员；`team accept` 会再次检查。

## 5. 工作区

- worktree 按**需求**创建，不按团队创建。
- 设计、计划、实施团队可以先后在同一个需求工作区里工作。
- 同一个需求同一时间只允许一个 active 团队。
- 同一个团队的所有成员在同一个 workspace 工作；负责人负责划分文件范围，避免并发写冲突。
- 团队解散关闭整个团队 tab（负责人 + 员工 pane），不删工作区。
- `requirement close --remove-worktree` 才删除工具创建的 git worktree。
- 没有 git 时，工作区就是项目根目录或 `workspace create --path` 指定的普通目录。

## 6. 合同

`contract.md` 是老板写的任务书，负责人只读。至少包含：

- 目标
- 非目标
- 交付物
- 完成定义（DoD）
- 验收标准
- 证据要求
- 约束（含开工前读项目根 `docs/weeco/lessons.md`）
- 停止与升级
- 返工上限
- 工作区/分支/base commit
- 团队人数（不含负责人；工具不设上限）和角色
- 成员命名/角色约束（可选）
- 验收权限
- 独立 review（高风险）
- 合并/交付
- 拆分建议

合同在 `team recruit` 时复制到团队目录，负责人只读。老板要改合同，先 `team disband` 后用新合同重招；如果只是返工要求变化，可以用 `team rework --issues` 写明，但合同文件不会自动改。负责人不能自行放宽。合同只是 Markdown，不强制任何字段格式。

### 可选机器检查

如果合同目录有 `acceptance.json`：

```json
{
  "commands": [
    {"id": "A1", "cmd": "npm test", "expect_exit": 0, "timeout_s": 300}
  ],
  "artifacts": ["dist/app"],
  "required_files": ["result.md"],
  "require_clean_tree": true,
  "record_git": true
}
```

`weeco team check T-...` 会在团队工作区运行命令、检查工件，写 `check.json`；`record_git` 记录 commit/tree/branch/status，`require_clean_tree` 要求工作区干净，`required_files` 检查文件并记录 SHA-256。

- 没有 `acceptance.json` 也能验收，老板人工评审。
- `team rework` 会删除旧 `check.json`，返工后必须重新 check。
- `team accept` 在 `acceptance.json` 存在时要求 `check.json` 存在且通过（`--force` 可覆盖），并写团队目录下的 `acceptance.md`。
- 验收/返工后老板维护项目根 `docs/weeco/lessons.md`；`team accept` 记录 `--lessons-updated` / `--no-lessons`，`requirement close --lessons-reviewed` 表示已检查。

这不是旧的 `verify`：没有强制 checks.json、没有 commit trailer、没有 revision 预算/缓存、没有 base/head 字段。

## 7. 命令

### 老板

```bash
weeco init
weeco status [R-...]
weeco requirement new|list|show|stage|close
weeco workspace create --requirement R-...
weeco team recruit --requirement R-... --contract FILE --size N
weeco team list|status|wait|nudge|repair|check|accept|rework|answer|disband
```

关键命令语义：

- `team recruit`：建团队目录、复制合同、开一个 Herdr tab（负责人 pane）、启动负责人、发送初始 prompt。
- `team wait`：轮询团队状态，直到 submitted/blocked/lost 或超时；不会因为负责人 idle 就返回。老板在后台运行它，退出后处理状态。
- `team check`：可选的机器证据。
- `team status`：显示负责人和成员表（正式名字、agent name、状态、live、pane、report）。
- `team accept`：老板验收；默认关闭整个团队 tab（负责人 + 员工 pane）、记录 disbanded；工作区保留。
- `team rework`：老板驳回；写 review 文件、清掉旧 check、把问题发给原负责人。
- `team answer`：回答负责人的 blocked 问题。
- `team disband`：解散团队；`--force` 可用于 lost 团队。
- `requirement close`：关闭需求；`--remove-worktree` 才删工作区。

### 负责人

```bash
weeco member recruit --name "正式名字" --agent ascii-name --role "角色" --task FILE
weeco member status|wait|accept|rework|close
weeco lead status|submit|ask
```

负责人运行在团队 tab 的 lead pane 里。`member recruit` 会在同一个团队 tab 内 split 一个新 pane、启动员工的 Herdr agent；第一个员工在负责人右侧，后续员工继续分屏。员工数量不设工具上限。

负责人命令通过 `HERDR_PANE_ID` 自动解析 project/team，通常不用带 flags。上下文缺失时再带 `--project` / `--team`。`member` 命令同时校验正式名字/agent name，并记录成员事件。

`member recruit` 的 `--task` 是负责人写的任务文件；命令会把它复制到团队目录的 `members/<agent>/task.md`，并生成员工系统提示。员工在独立 pane 里运行，读任务、写 `report.md`，完成后停下来等负责人验收。

### 阶段 Skill

`weeco status` 输出 `next_skill`，老板按阶段加载：

| Skill | 阶段 |
|---|---|
| `weeco-boss` | 老板总入口 / 阶段路由 |
| `weeco-requirements` | 需求澄清 |
| `weeco-design` | 设计团队 |
| `weeco-plan` | 计划团队 + 需求工作区 |
| `weeco-implement` | 实施团队 |
| `weeco-review` | 等待、验收、返工、回答 |
| `weeco-deliver` | 交付、关闭 |
| `weeco-team-lead` | 负责人通用 skill |

阶段 skill 是调用 runbook，不是领域 playbook：它只规定这个阶段什么时候调用哪条 weeco 命令、谁验收、失败怎么办。具体设计/计划/代码由合同和团队完成。

当前版本提供成员级 CLI；员工是 Herdr agent，不是子代理。成员级台账只记录身份、pane、状态和验收，不回到旧的任务级 Task 模型。

> 兼容说明：0.8 之前招募的团队使用 subagent，没有 member 事件；这类团队仍按 `result.md` 的成员表验收。0.8 起新团队使用 pane 模式。

## 8. 存储

```text
~/.weeco/projects/<key>/
  project.json
  ledger.jsonl
  teams/<T-id>/
    contract.md
    result.md
    acceptance.json      # 可选
    check.json           # team check 生成
    acceptance.md        # 老板验收记录
    review-<n>.md
    lead-system-prompt.md  # 负责人 system prompt，经 --append-system-prompt-file 传入
    members/<agent-name>/
      task.md              # 负责人写的任务文件
      report.md            # 员工报告
      system-prompt.md     # 员工 system prompt，经 --append-system-prompt-file 传入
    logs/
  requirements/<R-id>/
```

```text
~/.weeco/worktrees/<key>/<R-id>/
docs/weeco/lessons.md           # 项目根；老板唯一写者
~/.weeco/panes/<pane>.json      # pane -> project/team；负责人和员工上下文
```

- `project.json` 记录项目根、key、默认分支。
- `ledger.jsonl` 是唯一状态源；事件按 `entity`（requirement/team/member）+ `id` 折叠。
- 项目通过 `~/.weeco/projects/` 注册；`FindProject` 从 cwd 向上查找，不依赖 git。
- git worktree 只是 workspace 的一种实现。

### 事件示例

```json
{"ts":"...","entity":"team","id":"T-20261026-01","event":"team_submitted","by":"lead","data":{"result_path":"..."}}
{"ts":"...","entity":"member","id":"T-20261026-01/lin-zhiyuan","event":"member_accepted","by":"lead","data":{"note":"..."}}
```

## 9. 代码结构

```text
cmd/weeco/main.go
internal/model/model.go         Project, Requirement, Team, Member, Event, 状态迁移判定
internal/store/paths.go         项目注册、路径、配置、锁
internal/store/ledger.go        事件追加、fold、ID、列表
internal/store/member.go        成员事件 fold、成员列表
internal/herdr/herdr.go         Herdr tab / pane split / agent 适配
internal/workspace/workspace.go git worktree / 普通目录
internal/commands/              CLI 命令（team / member / lead）
internal/assets/                嵌入的 skill、prompt、slash command、模板
```

`internal/assets/files/skills/` 下有 boss 路由、阶段 skill 和 team lead skill；`weeco install` 统一安装，`weeco doctor` 统一检查。

旧的 `internal/app` 任务模型已删除：`Task`、`Check`、`Report`、`VerifyResult`、`Budget`、`checks.json`、commit trailers 都不再属于核心。

## 10. 设计选择

- **一个团队一个 tab/panel，员工是 Herdr agent。** 负责人和员工各占一个 pane；第一个员工 split 在负责人右侧，后续员工继续分屏；不新开 tab。员工数量不设工具上限。
- **老板不做任务，负责人不做员工的工作。** 角色边界写在 prompt/skill，不用工具权限硬砍；老板、负责人和员工都用 bypass。
- **大流程在老板 skill/CLAUDE.md，不写死进工具。** 工具管团队和成员生命周期，老板管需求、设计、计划、实施、交付的阶段。
- **人的门在需求和最终交付。** 团队合同和团队验收是公司内部管理，不需要每个团队找人批。
- **一个需求一个 active 团队。** 先保证工作区不冲突；并行团队以后扩展。
- **上下文由工具注入，不靠 agent 记 flag。** 负责人通过 pane 上下文自动解析 project/team；成员命令从 lead pane 上下文解析团队；`team status` 直接给下一步命令；`team nudge` / `team repair` 处理 idle/lost。
- **负责人/员工 system prompt 走文件参数。** 多行 prompt 写入团队目录，用 `--append-system-prompt-file` 传入；Herdr 拒绝含换行等控制字符的 agent 启动参数。
- **成员命名在招募时校验。** 负责人给自己和员工取正式名字，员工另有 ASCII agent name；`member recruit` 校验非占位、唯一和 Herdr 命名规则；`lead submit` 要求成员已验收。
- **员工级台账只到成员级。** 记录身份、pane、状态、报告和验收，不记录任务级拆分，避免回到旧的 Task 模型。
- **合同 Markdown + 可选检查。** 不把开发流程写死；设计/计划/评审团队用同一套机制。

## 11. 非目标

- 不做 Dev Playbook / Design Playbook。
- 不强制 git。
- 不强制 shell checks、commit trailer、report 字段。
- 不做多项目公司、并行团队、自动 merge。
- 不做员工任务级台账；成员级有，任务级不回到旧 Task 模型。
- 不做大团队可读性优化；一个 tab 内 pane 数量不设工具上限，屏幕空间交给 Herdr 缩放和用户布局。
