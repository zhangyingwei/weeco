# fleet TUI（brizzai/fleet）参考

> 源码已获取：`git clone --depth 1 https://github.com/brizzai/fleet`，commit `1b13ff2`（2026-10-20），Apache-2.0，Go + Bubble Tea。以下引自仓库源码与 `docs/`。

## 1. 一句话与定位

fleet 是“多 coding agent 会话的终端驾驶舱”：每个会话跑在独立 tmux 中，可选 Claude Code / Codex / OpenCode / Copilot，按 repo/worktree 分组，实时显示 running/waiting/finished；按 Space 跳到需要人的会话，Enter 进入审批或纠偏。它不做调度、队列、多机，核心价值是“人机注意力路由”。开源 Apache-2.0，约 59 stars。

## 2. 架构与运行方式

- 状态检测三层：① Claude Code hooks（UserPromptSubmit / Stop / PermissionRequest / Notification）注入 `~/.claude/settings.json`，触发 `fleet hook-handler`，把状态写到 `~/.config/fleet/hooks/<session_id>.json`；② fsnotify watcher 监听目录（100ms 防抖），更新内存 map；③ 无 hook 时抓 tmux pane 做模式匹配（spinner、“(Y/n)”、“❯”）。规则：hook 对 waiting 有权威性，pane 不得覆盖，避免把 “❯ 1. Yes” 误判为已完成。
- worker goroutine 每约 2 秒一轮：刷新 tmux 缓存 → 同步 hook 状态 → 自动命名一个会话 → 轮转更新 5 个会话状态 → 刷新一个 repo 的 git/PR → 发 UI 消息。所有阻塞 I/O 不在 UI 线程。
- 持久化：SQLite（WAL）。`sessions` 表存 id/title/project_path/agent/account/status/tmux_session/claude_session_id/workspace_name/first_prompt/prompt_count/snoozed_until 等；另有 slot_bindings、pinned_repos、collapsed_groups、repo_pr_cache、shells。
- 运行：`fleet` 起 TUI（需常驻）；`fleet add|wt|send|status|read|wait|answer|interrupt|submit|clear|restart|suspend|resume|list|remove|skill` 是同一二进制的子命令，直接读写 SQLite + tmux，因此 TUI 未开也能被脚本或其他 agent 驱动。
- `fleet skill install` 把一份 SKILL.md 写入 `~/.claude/skills/fleet/` 与 `~/.agents/skills/fleet/`，教 agent 用 CLI 派活。

## 3. 角色与任务模型

- 没有角色枚举。会话本身就是任务载体：一个 session = 一个 agent CLI + 一个目录或 git worktree + 一个分支 + 一个首 prompt + 自动生成的标题。
- 创建方式：`fleet wt <branch> -p "<task>"`（建 worktree、起 agent、把 prompt 作为第一条消息，会话直接开工）；`fleet add <path> -p`（在已有目录）；`fleet wt --ticket BRZ-3182`（把 Linear/Jira issue 的描述、评论、截图物化进 worktree）。`--agent/--model/--effort/--account/--base` 在启动时指定，之后不再强加。
- 选择器（`send/status/read/wait/answer` 共用）按精确度排序：`@0`–`@9` 热键槽 → 精确 id → 精确标题 → workspace 名 → git 分支 → id 前缀 → 标题子串；多义匹配直接报错并列候选，绝不猜。

## 4. 状态、调度与恢复

- 状态枚举：starting / running / waiting / finished / idle / error / suspended；另有“snoozed”只影响展示，不改变真实状态。
- 无调度器、无作业队列、无重试退避；状态由 agent 自身事件驱动，fleet 只观测。
- 恢复：`r` / `fleet restart` 杀掉进程后用 `claude --resume <id>`（Codex `resume`、OpenCode `--session`、Copilot `--session-id`）续会话；内存压力下自动休眠最闲会话（每个约 400MB，`fleet suspend` 也可手动休眠），`resume`/`send`/Enter 唤醒；`f` fork 会话（Copilot 不支持）。
- 对外等待协议是最有价值的部分：`fleet wait <s...> --until done|idle|waiting|running [--any] [--timeout] [--json]`，退出码 0=条件满足、2=超时、3=卡在无人回答的 prompt、4=agent 退出/报错；`fleet send -wait` 一步完成“发消息 + 等回合结束”。文档明确：不要轮询 status，wait 就是为此设计。
- 幂等细节：`send` 交付后校验 pane，失败不自动重试；重复 send 会把任务提交两次，所以失败先 `fleet status <s>` 看 last prompt。

## 5. 隔离与权限

- 隔离靠 tmux + git worktree，不靠容器/沙箱：agent 以当前用户权限运行，能碰到的文件和网络就是用户的。`fleet add` 明确警告不要与正在编辑的树共用目录。
- 审批是“人肉 gate”：`Y` 仅在状态为 waiting 时可用，向 pane 发送 `y` + Enter，且先确认 pane 上确有权限提示；`fleet answer <s> <n|yes|no>` 用方向键移动光标、按前校验、按后复查，`-expect` 防止答错 prompt；`interrupt/submit/clear` 都先观察再动作。
- 拒绝错位输入：waiting（除非 `-force`）、pane 回到 shell（不可 force，只能 restart）、suspended（自动唤醒并作为首 prompt）。
- 配置：`~/.config/fleet/config.json`（默认 agent、tick、telemetry、suspend 策略、account_strategy/allowed_accounts 等）；repo 级 `.fleet.json` / `.fleet.local.json` 配 workspace 的 list/create/destroy、copy_files、PR 检查忽略、Linear/Jira 团队。`.claude/settings.local.json` 默认复制进新 worktree（`copy_claude_settings`）。

## 6. 可借鉴清单（映射 7 组件）

口径：C1 角色与载体矩阵、C2 终态协议、C3 事件驱动协调、C4 审查者工具白名单与审查台账、C5 单写租约与幂等派发、C6 上下文预算与状态包交接、C7 权限预检；下列简称均按此对应。

- 角色载体矩阵：用 `agent` 字段 + 启动参数 + SKILL.md 把“谁能干什么”外置；同一 fleet 可混四种 CLI，说明角色载体应是“可插拔 adapter”，不是固定枚举。
- 终态协议：状态枚举 + wait 的 0/2/3/4 退出码 + `--until` 四值，是一套可直接借用的“可编程终态”接口；尤其 exit 3 把“等人审批”与“失败”分开。
- 事件驱动：hooks → 状态文件 → fsnotify → 内存 map → UI/CLI，是低延迟、无轮询的 agent 状态总线；pane 仅作 fallback 且不得覆盖 hook。
- 审查白名单/台账：没有台账，但 `answer -expect`、act 前 observe、act 后 verify 的“三拍”流程，可移植为审批动作的防误触协议。
- 租约/幂等：无租约；但“selector 多义即报错”“send 不自动重试”“重复 send 会重复执行”的显式失败哲学，正好反衬我们需要补幂等键。
- 上下文交接：fork/restart/resume + `fleet read` 从 transcript 取最后 3 轮 + `fleet send` 给运行中会话追加指令，是“人/AI 交接同一会话”的完整工具面。
- 权限预检：`send` 在 waiting/shell/suspended 三种危险状态前拒绝或改写行为，可作为“动作前状态预检”的模板。

## 7. 不可照搬与风险

- 明确不相关之处：它是 TUI/人机驾驶舱，不含调度、队列、租约、审计、权限沙箱、多机，不能作为舰队控制平面。
- 依赖 tmux 与全局 `~/.claude/settings.json` 注入 hook，会污染用户环境；hook 契约随 agent 版本变化。
- pane 抓取是启发式，版本升级/自定义主题会误判；虽然 hook 优先，但非 Claude 的 agent 仍大量依赖 pane。
- 无持久任务账本与幂等键：进程/TUI 崩溃后只有 SQLite 会话行，无法恢复“进行中的派发”。
- 无跨机器、无密钥隔离、无审批记录；`Y` 是直接敲键，审计性弱。
- 会话休眠依赖内存压力策略，恢复时若 transcript 丢失会失败（代码已有 “no conversation found” 分支）。

## 8. 原文索引

- `README.md`、`docs/ARCHITECTURE.md`、`docs/vision.md`、`docs/background-agent-detection.md`
- `internal/session/session.go`（状态机）、`internal/session/storage.go`（SQLite schema）
- `internal/hooks/claude_hooks.go`（hook 注入）、`internal/hooks/hook_watcher.go`
- `cmd/fleet/{send,wait,status,control,skill}.go`、`internal/skill/SKILL.md`
- `internal/config/config.go`、`internal/workspace/repo_config.go`
