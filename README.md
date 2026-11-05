# weeco

在 Herdr 中用真实成员协作完成目标，由当前会话负责组织、参与执行和检查结果。

v0.12.0 保留 v0.11.0 的公开技能 `/weeco <目标>` 和扁平协作命令，增加成员等级、画师角色和验收记录。当前 Herdr 会话是唯一负责人；成员在同一 tab 的独立 pane 中工作，主 pane 保留。人数包含负责人，简单任务由负责人加 1 名成员完成，复杂任务根据交付物、依赖和所需能力增加成员。负责人也可以执行具体工作。

成员分为 `senior`、`mid`、`junior`、`temp` 四个等级，默认 `mid`。每个等级的模型候选写在 `~/.weeco/config.json` 或项目 `.weeco/config.json`，用 `weeco config check` 查看实际生效的候选。需要生成图片时使用画师：`--role artist`，固定用 Codex，不单独配置图像模型。

## 安装与使用

从源码构建需要 Go 1.24+。运行团队需要可用的 `herdr`、已完成登录且可正常工作的 `claude`，以及 Herdr 中的负责人 pane。使用 `--agent codex` 或 `--agent pi` 时，还需要对应 CLI 已安装并登录。

```bash
make install
```

已有二进制可运行 `weeco install` 安装技能。默认二进制位于 `~/.local/bin/weeco`，唯一技能位于 `~/.claude/skills/weeco/`，成员提示随技能安装。

在 Herdr 中打开工作的目录，进入当前 Claude 会话并输入目标，例如：

```text
/weeco 把本地预览端口从5186改为5187，其他不动，验收能启动
```

首次在全新目录使用 Claude 时，会先询问是否信任该工作区；由负责人会话确认一次即可。成员复用同一目录，不会重复询问。

无需项目初始化。安装不写项目 CLAUDE.md，也不修改全局信任或权限；由 weeco 启动的成员默认获得最大授权：Claude 使用 bypassPermissions，PI 以最大工具权限运行并加载 guard 扩展，Codex 使用不弹审批的 workspace-write 沙箱（开启网络以访问 Herdr socket）。测试目录之外的破坏性命令会被拦截。技能只在显式调用 `/weeco` 时启用，普通对话按原有方式进行。

## 如何协作

负责人读取已有材料、明确目标和交付边界，按依赖与能力分工。动手前先用简短要点说明理解的目标、分工、执行顺序、验收方式和关键假设，让用户能尽早发现偏差；说明后直接开始，用户可随时打断。只有用户明确要求先确认，或涉及 UI 原型确认时才暂停等待。只有会改变行动的缺口才问用户；既有授权内继续工作，不在每一轮重复问开工。项目文档、设计、计划和交付物写在项目内或任务约定位置；`/tmp` 只用于临时文件，任务引用的共享材料必须能在项目或 run 目录中保留。

涉及用户可见界面、布局、视觉、交互、响应式或深色模式时，负责人必须先确认是否需要原型；这是“说明后直接开始”的例外。需要原型时，先在项目 `docs/prototypes/<feature>/` 产出可查看的静态原型和关键状态，用户确认后再写生产 UI；后端等非 UI 工作可以并行，UI 实现等待原型确认。

开工前还要按问题规模选择工作区：小型任务直接在当前 checkout 修改；中大型任务用 Herdr worktree 隔离。worktree 必须在第一个 `weeco start` 之前创建，并通过 `--dir` 和 `--team-pane` 指定；`--dir` 是成员工作目录，`--team-pane` 是 worktree 的 root pane，成员会实际出现在 worktree workspace，负责人仍留在原 pane。worktree 从 HEAD 创建，不会带上未提交改动，脏工作区要先和用户确认提交、stash 或直接在当前树做。交付前要在 worktree 内检查 `git status`，报告路径、分支、workspace 和未提交文件，由用户决定保留还是删除。删除使用 `herdr worktree remove --workspace <id>`；有未提交内容时不能自动 `--force`。`weeco close` 只关闭成员和 run，不删除 worktree，也不能用 `workspace close` 代替清理。

用户只要计划、方案或草稿时，团队检查该产物后交付，不执行其中列出的操作。

成员必须是真实 Herdr Agent/pane，不能用 Agent/Task 工具或会话里的角色扮演代替，也不能递归招人。共享目录中的写入先明确归属，避免相互覆盖。成员提交成果和核验依据后等待，负责人检查实际产物与原始来源，发现问题后让相应成员返工。

成员默认使用 Claude，也可通过 `--agent codex` 或 `--agent pi` 启动对应 Agent。Claude 以 `bypassPermissions` 启动并挂 PreToolUse guard；Codex 以 `workspace-write` 沙箱和 `--ask-for-approval never` 启动，写入限定在工作区和 `~/.weeco/runs`（运行状态，用于加锁和上报），并开启 `network_access` 以便成员通过 Herdr socket 上报；PI 以最大权限启动并挂 `tool_call` guard。测试目录之外的破坏性命令会被拦截；Claude/PI 的 guard 是护栏，脚本混淆仍可能绕过，且只覆盖 Bash 命令，Claude/PI 的文件写入工具不受其检查；Codex 由原生沙箱限制写入范围。该设置只作用于 weeco 启动的成员，不修改你的当前会话或全局权限配置；负责人必须在任务文件中写明可操作范围和交付边界。

独立检查需要独立读取原始要求和证据、尝试反例；换角色签字不能证明正确。检查深度随错误代价调整。idle、超时和“已完成”文字都不构成验收证据。

## 命令

| 命令 | 用途 |
|---|---|
| `start --name ALIAS --role TEXT --task FILE [--level LEVEL] [--model MODEL] [--agent claude\|codex\|pi] [--dir PATH] [--team-pane PANE] [--replace]` | 启动成员，首名成员自动建立 run；等级默认 `mid`，画师用 `--role artist`，`--dir` 指定 worktree 或其他工作目录，`--team-pane` 指定团队分屏的 worktree root pane |
| `status [ALIAS] [--wait --timeout SEC] [--json]` | 查询成员与当前工作，按需等待 |
| `send <ALIAS\|lead> --file FILE` | 给成员开启新 turn，或向负责人通信 |
| `report --turn N --file FILE [--blocked]` | 成员提交指定轮次的成果或阻塞 |
| `accept [ALIAS] --turn N --file NOTE` | 负责人记录验收内容；不替代 `close` |
| `close [ALIAS] [--cancel --reason TEXT]` | 关闭指定成员或当前团队；取消需说明原因；不删除 worktree |
| `config init [--project] [--force]` / `config check` | 生成最小模型配置 / 查看实际生效的等级与候选 |

以上命令均以 `weeco` 为前缀，支持 `--run ID` 显式定位。成员身份、任务、控制目录、当前 turn 和报告命令由 CLI 注入。报告归属成员 pane，必须携带收到的 turn，不能自动取最新轮次把旧结果冒充新结果。

`send lead` 只通信，送达不等于取得决定或授权。负责人在交付边界内检查并完成必要返工后再关闭成员；待用户决定或仍需返工时保留相关 pane。`close` 保留负责人主 pane，关闭动作本身不证明成果通过；它也不会删除 worktree，worktree 的保留或删除要单独确认。`accept` 只记录负责人核验了哪些产物、实际执行和未验证项，不改变 `close` 的条件；同一轮之后又有新报告时，验收标记为过期，需要重新核验。`start` 会确认成员真正开始任务；未确认时保留 pane 并报错，负责人检查后用 `send` 重试或 `close --cancel` 结束。`send` 会等待成员真正开始新 turn；如果成员仍在 `working`，命令会拒绝，负责人应先等当前报告或检查 pane；如果 pane 停在确认提示或模态向导上，命令返回未确认错误，负责人需检查 pane 后重试。

## 运行记录与边界

最小运行记录保存在 `~/.weeco/runs/<id>/run.json`，并保留任务、报告快照。CLI 管理成员与通信，成果仍由团队按实际任务交付；负责人核对当前版本、原始来源和实际观察，说明未验证项与剩余问题。

v0.12.0 沿用 v0.11.0 的扁平命令接口，增加等级、画师和验收记录，不提供旧流程兼容层。安装器自动归档并移除旧 manifest 拥有的技能、模板和同名 slash 命令，保留用户自定义内容。升级不会回滚历史信任设置；归档位置与处理结果见安装输出。

## 开发与验证

```bash
go test ./...
go vet ./...
make build
```

自动回归覆盖成员生命周期、会话隔离、轮次检查、过期报告、阻塞、投递失败、等待、清理失败、栅栏规则以及 Claude/Codex/PI 三种启动参数，并通过 `go test -race ./...`。真实 Herdr 环境用 Python + Vue 记事本项目验证了 Claude 成员的主要流程：负责人写需求与任务、两名成员并行实现、backend/checker 正常提交报告、负责人独立运行测试与 HTTP 集成、独立 checker 发现后端 id 越界缺陷、`send` 返工修复；当时前端成员因 `npm` 构建被审批拒绝而提交 blocked，构建由负责人完成并以 cancel 关闭，不是所有成员都正常提交。

v0.12.0 的等级、候选失败转移、画师和 `accept` 目前只经过自动回归（fake Herdr），尚未在真实 Herdr 环境验证；画师固定使用 Codex 后能否稳定产出图片，也需要单独做真实冒烟。

真实测试暴露并修复了两个审批相关问题：成员上报曾被 auto 分类器判为缺少授权；成员 pane 停在模态提示时 herdr 仍报告 prompt 成功。现在 Claude 成员以 `bypassPermissions` 启动并挂 PreToolUse guard，Codex 使用 `workspace-write` 沙箱，PI 使用 `tool_call` guard；`send` 会等待成员真正进入 `working`。Claude/PI 的 guard 是护栏，不是沙箱，脚本混淆或解释器内删除仍可能绕过；Codex 的沙箱限制写入范围。成员权限与栅栏边界必须在任务中写清；小样本也不能证明任意领域任务都可靠。

另外做了针对三种 agent 的定向真实验证：Claude 进程在 `bypassPermissions` 和生成的 settings 下，PreToolUse guard 拦截了工作目录外删除并放行目录内删除；PI 与 Codex 都能作为真实 Herdr 成员启动并通过 `weeco report` 上报，PI 的 `tool_call` guard 拦截目录外删除，Codex 的 workspace-write 沙箱拒绝工作区外写入。这验证的是启动、栅栏与上报链路，不等于完整的多人协作端到端结论。

完整操作示例见 [快速开始](docs/weeco-quickstart.md)。
