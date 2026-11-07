# weeco 快速开始

本页介绍 v0.12.2 的接口：在 v0.11.0 的扁平命令上增加成员等级、画师角色和验收记录。下方示例描述操作方法，实际验证范围与限制见页末。

## 安装

从 Release 一键安装：

```bash
curl -fsSL https://github.com/zhangyingwei/weeco/releases/download/v0.12.2/install.sh | bash
```

Windows PowerShell：

```powershell
irm https://github.com/zhangyingwei/weeco/releases/download/v0.12.2/install.ps1 | iex
```

脚本会校验 `SHA256SUMS`，安装二进制并执行 `weeco install` 安装技能；`herdr`、`claude` 只检测并提示，不会自动安装或登录。

从源码安装需要 Go 1.24+：

```bash
make install
```

已有二进制可执行 `weeco install` 安装技能。默认二进制为 `~/.local/bin/weeco`，技能和成员提示为 `~/.claude/skills/weeco/{SKILL.md,member-prompt.md}`。确认 `weeco`、`herdr` 和 `claude` 在 PATH 中，Claude 已能正常登录和工作；使用 `--agent codex` 或 `--agent pi` 时，还需要对应 CLI 已安装并登录。

运行时进入 Herdr 的 pane，在要工作的目录中使用 Claude。无需项目初始化，不需要写项目 CLAUDE.md 或变更全局信任、权限；由 weeco 启动的成员默认获得最大授权：Claude 使用 bypassPermissions，PI 以最大工具权限运行并加载 guard 扩展，Codex 使用不弹审批的 workspace-write 沙箱（开启 network_access 以访问 Herdr socket）。首次在全新目录使用 Claude 时会询问是否信任该工作区，由负责人会话确认一次即可；成员复用同一目录，不会重复询问。当前 pane 是唯一负责人，成员都在当前 tab 中分屏，结束团队时保留主 pane。

升级时，安装器自动归档并移除旧 manifest 拥有的技能、模板和同名 slash 命令，保留用户自定义内容；不会回滚历史信任设置。归档位置和处理结果以安装输出为准。

## 配置等级模型

不配置也能用，默认全部等级使用 Claude 的 CLI 默认模型。需要指定模型时：

```bash
weeco config init
weeco config check
```

`config init` 写入 `~/.weeco/config.json`，`--project` 写入当前目录的 `.weeco/config.json`。文件里只需要 `levels`：

```json
{
  "levels": {
    "senior": [
      { "model": "<模型名>", "effort": "high" },
      { "agent": "codex", "model": "<另一个模型名>" }
    ],
    "mid":    [ { "model": "<模型名>", "effort": "medium" } ],
    "junior": [ { "model": "<模型名>", "effort": "low" } ],
    "temp":   [ { "agent": "pi", "model": "<模型名>" } ]
  }
}
```

- `agent` 省略时默认 `claude`，只能是 `claude`、`codex`、`pi`；
- `effort` 省略时按等级取默认：资深 high、中级 medium、初级和临时 low；写 `none` 表示不传强度参数；
- 候选按顺序失败转移，只在一轮工作开始前尝试下一个；
- 未知字段会直接报错，不会静默忽略；
- 画师不读等级配置，固定使用 Codex。

## 用目标启动协作

在负责人会话输入：

```text
/weeco 用我提供的三段材料写一份200字以内的活动通知，缺失信息留空，不发布
```

负责人使用已有目标和授权，按成果与依赖分配工作。这个简单任务可以由负责人写稿，1 名真实成员逐条核对材料、字数和空缺；人数共 2 人。任务复杂时按实际工作增加成员，不按固定领域岗位凑人数。

动手前，负责人会先用简短要点说明：我理解的目标与交付物、不做什么、准备怎么分工、执行顺序、验收方式和关键假设。说明后直接开始，不等待确认；如果发现理解偏差，可以直接打断并提出修改。只有明确要求“先确认”，或涉及 UI 原型确认时，负责人才会暂停等待。

每个成员定一个等级：`senior` 处理模糊、跨模块、根因或高风险验收；`mid` 处理边界清晰的常规实现；`junior` 处理机械、单文件修改；`temp` 处理一次性探查。需要生成图片时直接派画师，画师固定用 Codex。

只有缺失信息会改变行动时才询问。用户已允许事实缺项留空，就按该边界成稿；用户只要计划或方案，就以方案交付为止，不实施其中的真实操作。

## 按规模选择工作区

小型任务（单文件或单模块、无迁移、验收快）直接在当前 checkout 修改。中大型任务（跨模块、新功能、多人并行、数据迁移/发布/安全/不可逆操作）用 Herdr worktree 隔离：

```bash
PROJECT=/absolute/path/to/project
herdr worktree create --cwd "$PROJECT" --branch "weeco/<slug>" --label "<task>" --no-focus

# 从返回 JSON 取 result.worktree.path 和 root pane 的 pane_id，然后：
weeco start --name coder --role "实现功能" --level mid \
  --dir "<worktree-path>" \
  --team-pane "<root-pane>" \
  --task "<worktree-path>/.weeco/tasks/coder.md"
```

`--dir` 只在创建 run 时生效；已有 run 时传入不同目录会报错。`--team-pane` 指定 worktree 的 root pane，成员会从它分屏，实际出现在 worktree workspace；负责人仍留在原来的 pane/workspace。`status` 会显示实际目录、团队 pane 和团队 workspace。worktree 必须在第一个 `weeco start` 前创建，之后不能切换。

worktree 从 HEAD 创建，不会带上未提交改动；工作区不干净时先确认提交、stash，还是直接在当前树做。已经在 linked worktree 中就直接使用，不再嵌套。

### 结束时清理 worktree

交付前在 worktree 路径检查实际状态：

```bash
cd "<worktree-path>"
git status --porcelain
git branch --show-current
```

最终报告写清 worktree 路径、分支、Herdr workspace ID 和未提交文件列表，然后询问用户保留还是删除：

- **保留**：不执行 remove；用户可能还要合并、检查或继续开发。
- **删除**：确认没有未提交改动，或用户明确接受丢失后执行：
  ```bash
  herdr worktree remove --workspace <workspace-id>
  ```
  有未提交内容时不得自动使用 `--force`；只有用户明确接受丢失未提交改动时才允许。
- `weeco close` 只关闭成员 pane 和 run，不删除 worktree。
- 不要用 `workspace close` 代替清理：关闭 workspace 后 checkout 仍然存在，但 Herdr 不再把它当作打开的 worktree。

## UI 项目先确认原型

涉及用户可见界面、布局、视觉、交互、响应式或深色模式时，负责人必须先问用户是否需要原型；这是“说明后直接开始”的例外。纯后端、CLI、数据迁移和内部重构不需要。

需要原型时，先启动原型成员：

```bash
PROJECT=/absolute/path/to/project
weeco start --name ui-proto --role "原型设计" --level senior --task "$PROJECT/.weeco/tasks/prototype-task.md"
```

原型任务只写 `docs/prototypes/<feature>/`，产出可打开的静态原型、关键状态和交互、`notes.md`；不写生产 UI，不接真实后端。复杂流程用 `--level senior`，简单组件可用 `mid`。负责人检查后，由用户确认视觉方向、布局和关键交互，再用 `weeco accept` 记录；UI 实现任务引用原型路径。后端等非 UI 工作可以并行，UI 实现等待原型确认。

用户说不需要原型时，把决定写进任务；任务仍要写清布局、状态、交互、窄屏和深色模式等 UI 验收标准。

## 负责人启动成员

通常由 `/weeco` 中的负责人代为调用命令。需要了解底层操作时，先在项目内写一份任务文件。下文用 `$PROJECT` 表示项目根目录，例如 `/absolute/path/to/project`；任务文件例如 `$PROJECT/.weeco/tasks/notice-check-task.md`：

```text
目标：核对负责人交付的活动通知。
原始材料与通知位置：填写本次工作使用的真实路径。
范围：只核对，不改正文，不发布。
交付：逐项列出事实与原文对应、字数结果、应保留的缺项及具体问题。
验收：每条结论给出材料位置；不能确认的内容明确标注。
依赖：通知草稿由负责人提供。当前 turn 以 CLI 注入值为准。
```

示例中的路径说明须替换为实际材料位置，再执行：

```bash
PROJECT=/absolute/path/to/project
weeco start --name checker --role "核对材料与成稿" --level senior --task "$PROJECT/.weeco/tasks/notice-check-task.md"
weeco start --name coder --role "实现后端" --level mid --task "$PROJECT/.weeco/tasks/backend-task.md"
weeco start --name illustrator --role artist --task "$PROJECT/.weeco/tasks/poster-brief.md"
weeco status
weeco status checker --wait --timeout 60
```

任务文件、共享契约、报告和验收备注应放在项目内稳定路径（如 `.weeco/tasks/`、`.reports/`）；`/tmp` 只用于可丢弃的临时文件。`weeco start` 只复制 `--task` 指向的任务文件进 run 目录，任务里引用的共享材料必须在项目内或 run 目录有副本。

首名成员自动建立 run，记下输出中的 run ID。等级默认 `mid`；`--model` 在等级候选里精确选择，用 `weeco config check` 查看可用候选。成员默认使用 Claude；配置里的候选可以指定 `agent` 为 `claude`、`codex` 或 `pi`。画师固定用 Codex，不接受 `--level`、`--model` 或非 codex 的 `--agent`。成员是同 tab 的真实 Herdr Agent，不能用 Agent/Task 工具代替。共享文件需明确写入归属。需要替换同名成员时，先检查当前状态并保存必要成果，再使用 `start` 的 `--replace`。

测试目录之外的破坏性命令会被拦截：Claude/PI 由 weeco guard 检查，Codex 由 workspace-write 沙箱限制写入。Codex 成员会开启 `sandbox_workspace_write.network_access`，否则它无法通过 Herdr socket 上报；同时允许写 `~/.weeco/runs` 以便加锁和提交报告，其它位置仍不可写。Claude/PI 的 guard 是护栏，脚本混淆仍可能绕过，且只覆盖 Bash 命令，文件写入工具不受其检查；不要把栅栏当成绝对沙箱。

`start` 发送任务后会等待成员进入 `working`；如果未确认，会保留成员 pane 并报错，先检查 pane，再用 `send` 重试或 `close --cancel` 结束，不会把成员当作启动失败直接清理。

`status [ALIAS] --json` 输出结构化状态。每次 `status --wait` 只进行一次等待：等新报告到达或到达超时时间后返回，不会持续替负责人跟进后续轮次。返回后读取实际报告和状态；等待超时时核对 pane 与任务进展，idle、超时或没有输出都不能当作完成。

## 成员提交指定轮次

CLI 为成员注入身份、原始任务、控制目录、当前 turn 及报告命令。`report` 必须显式提供 `--turn N`，不能省略或自动使用最新轮次。成员将实际结果与依据写入报告文件，使用收到的明确轮次，例如当前收到的是第 1 轮：

```bash
weeco report --turn 1 --file "$PROJECT/.reports/notice-check-report.md"
```

成员会话在各自权限模型下运行：Claude/PI 有最大权限，Codex 使用 workspace-write 沙箱；测试目录之外的破坏性命令会被拦截。负责人必须在任务文件中写明可操作范围，不能依赖权限系统兜底。

不能读取最新 turn 替旧结果报数。新一轮任务需要本轮的工作与核验依据，不能复用旧报告冒充完成。

无法继续时，报告说明阻塞、证据、需要的决定和可继续部分：

```bash
weeco report --turn 1 --file "$PROJECT/.reports/notice-check-blocked.md" --blocked
```

需要向负责人通信可用：

```bash
weeco send lead --file "$PROJECT/.reports/notice-question.md"
```

`send lead` 只传递消息，不代表问题已经获批。成员不创建新的 Herdr 成员、不自行验收或关闭团队，报告后等待负责人返工或关闭。

## 返工与实际验收

负责人读实际通知和原始材料，不能只读成员的完成声明。发现问题时，将现象、期望、依据与修复边界写入补充文件，再执行：

```bash
weeco send checker --file "$PROJECT/.reports/notice-follow-up.md"
weeco status checker --wait --timeout 60
```

向成员 `send` 会开启新 turn，CLI 在消息中给出轮次。补充文件说明承接哪轮与要解决的问题，成员随后使用新轮次报告。`send` 发送后会等待成员真正进入工作状态；如果 pane 停在确认提示或模态向导上，命令会返回未确认错误，先检查并处理该 pane，再重试。

按实际任务选择证据：端口修改检查当前进程与新端口能否启动；通知检查事实、字数和缺项；迁移方案检查步骤、对账、回滚、失败重跑和未验证依赖。方案完整不能证明生产迁移已执行，模拟检查也不能冒称真实用户反馈。独立检查从原始目标和来源重新核验，并尝试找反例。

检查完成可记录验收内容：

```bash
weeco accept checker --turn 1 --file "$PROJECT/.reports/notice-accept.md"
```

备注里写核验了哪些产物、实际执行的命令或观察、未验证项和剩余问题。`accept` 只留痕，不改变 `close` 条件；同一轮之后成员又提交了新 revision，`status` 会显示验收已过期，需要重新核验。

## 交付与关闭

交付边界内检查完成后，负责人说明成果位置、实际核验、未验证项和剩余问题，再关闭。如果本轮使用了 worktree，先按“结束时清理 worktree”报告状态并确认保留或删除；`weeco close` 本身不删除 worktree。

```bash
weeco close checker
```

省略成员名会关闭当前团队的成员，保留负责人主 pane：

```bash
weeco close
```

仍待用户决定或需要返工时，先保留相关成员。取消用明确原因记录：

```bash
weeco close checker --cancel --reason "用户取消本次核对"
```

关闭动作不是成果通过的证明。所有协作命令支持 `--run ID`；需要显式定位时，将 `RUN_ID` 替换为 `start` 返回的实际值：

```bash
weeco status --run RUN_ID
```

运行记录位于 `~/.weeco/runs/<id>/run.json`，任务和报告另存快照。不手改记录；任务产物放在约定的交付位置。

## 遇到阻塞

- 无法启动真实成员：先确认当前会话位于 Herdr pane，`herdr` 与 `claude` 可用，并检查启动命令的实际错误；如果报 `blocked during startup`，通常是新目录尚未确认 Claude 工作区信任，先在负责人会话里确认一次；不要用单会话角色模拟替代。
- 成员没有报告：查看指定成员与 turn，核对 pane 和任务进展，不能靠 idle 推定完成。
- `send` 被拒绝：如果提示成员仍在 `working`，先等当前报告或检查 pane，不能把旧 turn 的完成当成新 turn 已送达。
- 报告轮次不符：成员按自己收到的明确 turn 报告；过期结果不能改成最新轮次补报。
- 需要用户决定：负责人只暂停依赖该决定的工作，保留相关成员，其余已授权工作可以继续。

v0.12.2 已通过自动回归、race 检查、静态检查和构建。等级、候选失败转移、画师和 `accept` 目前只在 fake Herdr 测试桩上验证，尚未在真实 Herdr 环境验证；画师固定使用 Codex 后能否稳定产出图片，需要单独做真实冒烟。v0.11.0 的真实 Herdr 验证仍可作为基础参考：用 Python + Vue 记事本项目验证了 Claude 成员的主要协作流程：负责人写需求与任务、两名成员并行实现、backend/checker 正常提交报告、负责人独立测试与 HTTP 集成、独立 checker 发现后端 id 越界缺陷、`send` 返工修复。前端成员当时因 `npm` 构建被审批拒绝而提交 blocked，构建由负责人完成并以 cancel 关闭，不是全部成员正常提交。

真实测试暴露并修复了两个审批相关问题：成员上报曾被 auto 分类器判为缺少授权；成员 pane 停在模态提示时 herdr 仍报告 prompt 成功。现在 Claude 成员以 `bypassPermissions` 启动并挂 PreToolUse guard，Codex 使用 `workspace-write` 沙箱，PI 使用 `tool_call` guard；`send` 会等待成员真正进入 `working`。Claude/PI 的 guard 是护栏，脚本混淆或解释器内删除仍可能绕过；Codex 的沙箱限制写入范围。测试时必须写清成员可操作范围；小样本也不代表所有任务质量已获证明。

另外做了针对三种 agent 的定向真实验证：Claude 进程在 `bypassPermissions` 和生成的 settings 下，PreToolUse guard 拦截了工作目录外删除并放行目录内删除；PI 与 Codex 都能作为真实 Herdr 成员启动并通过 `weeco report` 上报，PI 的 `tool_call` guard 拦截目录外删除，Codex 的 workspace-write 沙箱拒绝工作区外写入。这验证的是启动、栅栏与上报链路，不是完整多人协作 e2e。
