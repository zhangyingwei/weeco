# Fengshen Fleet 参考

> 源码公开：github.com/edisonshen/fleet（MIT），官方页 fengshen.dev/fleet。注意版本漂移：官网写 50%/70%，当前源码与 README 是 40%/50%；下文以源码为准并标注。

## 1. 一句话与定位

Fleet 是"每个人的管理台"：一个 Go 单二进制 + TUI，把多个仓库里的多个 Claude Code 会话收敛成一个看板。每个项目一个 coordinator 拥有任务列表并派发 worker；每个 worker 在独立 git worktree/分支；上下文热了自动交接。目标是放大"一个人的监督容量"，而不是替代 Claude Code。MIT 开源。

## 2. 架构与运行方式

- 安装 `brew install edisonshen/tap/fleet`；`fleet init` 安装 `fleet-guard` 与 `coordinator` skills 到 `~/.claude/skills/`、合并 hooks、写 `~/.fleet/standards.md`，并询问默认并行度（默认 3）。
- 运行时依赖 `tmux` + `claude`（codex 可选，作第二意见 reviewer）。
- 全部状态是 `~/.fleet/` 下的扁平文件：projects/<name>.yaml、agents/<id>.json、handoffs/、inbox/、progress/、queue/、logs/；每个聚合一个写者，原子 rename + fsync，项目级 flock。
- 流程：DISCUSS → PLAN-DOC → SPLIT → TASK LIST → TASK-PLAN-DOC → IMPLEMENT → PR-TRACK → DONE；唯一人工批准门在 DISCUSS 之后（G2），之后自动跑到 PR。
- coordinator 是崩溃安全的文件租约（NB-flock）；`[a]` 可恢复死掉的 coord，先合成交接再归档旧记录；以 `claude --remote-control` 启动，手机 claude.ai 可直接接管。
- dashboard 是 bubbletea TUI，fsnotify 监听状态文件；worker 跑在 detached tmux 会话里。

## 3. 角色与职责

| 角色 | 职责 |
| --- | --- |
| operator | 经理：描述问题、批准计划、决定升级项；不直接调度 |
| coordinator | 每项目一个；拥有 tasks.md（唯一写入者，只经 `fleet tasks` CLI）；规划、拆分、派发、盯 PR；**从不实现、不内联 review** |
| worker | 子代理；场景优先：spec-repro → spec-encode → verify；写 verification.md；本地 commit；不 push、不 review |
| reviewer | alpha/beta 两席；beta 是主引擎锚点必须通过，alpha 是另一 provider（或第二主引擎模型）；修 P0/P1 并复验 |
| finisher | tick 内的确定性步骤；校验 review 终态与证据后 push + `gh pr create`；缺 verification evidence 直接 block |
| fix/rebase subagent | PR autopilot 用；CI 红修、BEHIND/DIRTY 重基；实质性冲突升级给人 |

- 拆分：≤10 个任务 coord 自己写，>10 派一个 planner 子代理；拆分本身无二次批准（operator 批准的是计划）。每个任务先出 `TASK-PLAN-<slug>.md`（含 Scenario contract 表），经双模型 plan review（记录 sha256，文档一改就失效）后才 promote。
- coord 的"只派发不实现"由 fleet-guard 的 PreToolUse 钩子机械执行：拒绝 `docs/` 外的内联编辑与改文件的 shell；子代理豁免；`FLEET_COORD_GUARD=off` 可关。

## 4. 任务状态与调度

- tasks.md 状态：`todo / ready / in-progress / in-review / done / blocked / abandoned`；字段含 priority P0–P3、worker_pid、worktree、pr_url、branch、depends_on、created/updated、started_at/finished_at、dispatch_generation、parked、complexity、spawned_by。
- 就绪：`status=ready` + 所有 depends_on 都存在于 tasks.md 且 `done`（abandoned 不算完成）+ 并行槽未满 + 文本路径重叠启发式不冲突。
- 默认并行 3，可 per-project 配 `parallelism`；`parallelism: 1` 退化为串行就地执行。并行 >1 时每 worker 独立 worktree，分支 `worker/<slug>`。
- 每个 tick 重算；每 slug 有 `dispatch_generation` 栅栏，防旧 worker 写回；启动用 `mark-launch-attempted` 三态 CAS（ok / predicate_fail / contention）保证至多一次派发。
- 没有 ADLC 式 strike 预算；替代的是分项上限：review slot 每条 2 次尝试；CI 修复每任务 3 次；PR remediation 每 watch 3 次、总量 6 次、退避最多 8 tick；超过即 raise-hand。
- 冲突检测是便宜的文件路径文本匹配（`path:`/`file:` 与内联路径），只减少冲突，不保证正确；误报代价是一个 tick 的串行。

## 5. 验证与门禁

- 任务计划里的 Scenario contract：每行一个可观察结果（响应体、文件内容、屏幕文本、exit code、DB 行、PR 状态），禁止"调用了函数 X"；level 取项目 standards 的 `## Sandbox` 最高档（e2e/replay/integ/unit），低于档位必须写理由。
- worker 证据：verification.md 的 `## Scenario contract`、`## Evidence — before`（证明原本会失败）、`## Evidence — after`、`## Gates`、`## Baseline`、`## Unit tests`；gates 必须全过，scenarios_verified == scenarios_total。
- reviewer 用 Scenario-contract 透镜：缺行、降档无理由、可跑 e2e 却 mock 都是 P1；每席独立跑，beta 不能跳过（限流就是 blocked，不是 skip）。
- finisher 校验终态字段后才 push/开 PR；PR 描述附 verification.md 原文。
- PR autopilot：tick 为每个带 pr_url 的任务维护 durable watch；CI 红 → fix-subagent；STALE/BEHIND/DIRTY → 隔离 worktree 里 rebase；CHANGES_REQUESTED → fix-subagent 只做机械修改，实质性意见 raise；MERGED 后任务置 done。
- 合并仍由 GitHub PR 与人工 review 完成；Fleet 不替代 CI。

## 6. 上下文与交接

- `fleet-guard` 从 transcript 读 context 百分比，按阈值自动交接。官网：50% 软 / 70% 硬；当前源码：`auto-yellow` ≥40%、`auto-red` ≥50%；budget profile 用绝对 token 250k/300k，quality profile 才用 40%/50%。
- 交接文档 frontmatter：agent_id、task_id、project、previous_handoff、handoff_number、timestamp、handoff_type、context_pct_at_handoff；正文：Status、Completed、Key Decisions、Docs (this session)、Open Questions、Next Steps、Active Subagents、Open PRs。
- soft 阈值只停止派发新 worker/PR fixer，reviewer/finisher/reconcile 继续，等在飞工作落地；hard 阈值强制交接，把在飞项列给继任者。继任者首轮收到固定提示："Read your handoff doc at <path> and continue the task."
- coordinator 还会写 checkpoint（next-step / decision）与会话文档列表，供交接时补齐占位段。
- `learnings.md` 是项目级 append-only 日志（`## 时间 · 作者 · 任务 · tag`）；每次派发把最近 5 条注入 worker prompt，其余可 `fleet learnings list --limit=20` 查。

## 7. 隔离与权限

- 隔离单位是 worker 的独立 worktree + `worker/<slug>` 分支；coordinator 由文件租约保证每项目一个。
- 权限：Claude worker 默认 `claude --dangerously-skip-permissions`（理由：fire-and-forget，权限提示会卡住 N 个 agent）；codex 默认 `--dangerously-bypass-approvals-and-sandbox`。**没有 OS 沙箱、没有网络/文件系统边界**；真正的边界只有 worktree 约定 + coord 的写钩子（且子代理豁免）。
- 状态：单机单 operator；每个聚合单写者；原子 rename + fsync；项目级 flock；handoff 文件名本身是完成信号（fsnotify CREATE），不解析自由文本。
- remote control：coord argv 内置 `claude --remote-control`，移动端可配对；可用 `fleet rc down <project>` 关闭。

## 8. 失败与恢复

- coord 崩溃：租约是 crash-safe 文件租约，不靠 staleness 猜活；`[a]` 生成替身并合成交接，替身拿到租约后才归档旧 coord。
- worker 崩溃/失联：tick 用 worker state、journal、worktree 三方对账；旧 generation 的写入被拒；dirty worktree 可 `parked` 保留，避免误删。
- 启动修复：queue 中的 spawn 记录若 tmux 会话已存在则丢弃；agents/*.json 对应会话不存在则归档；handoff 写一半的 `.tmp` 丢弃。
- PR 事件集合：`stale / behind / dirty / ci-failed / changes-requested / closed-unmerged / not-found / merged`；DIRTY 的阶梯是"干净 rebase → 从未批准才 re-derive → 仍失败则 raise"；已人工 APPROVED 的 PR 冲突只升级，不自动重写已审 diff。
- 卡住/无进展：review、CI、PR remediation 都有次数上限；blocked 需要 operator ack；没有 quota-paused 概念（本地单机）。
- 恢复不依赖 `--resume`：状态都在文件里，下一次 tick 继续。

## 9. 可借鉴清单（映射到我们 7 组件）

口径：C1 角色与载体矩阵、C2 终态协议、C3 事件驱动协调、C4 审查者工具白名单与审查台账、C5 单写租约与幂等派发、C6 上下文预算与状态包交接、C7 权限预检。

| 组件 | 机制 | 判断 |
| --- | --- | --- |
| C1 | coord/worker/reviewer/finisher 四段分工 | 直接借 |
| C1 | 双席 review（beta 锚点 + alpha 异源）+ 模型/effort 路由 | 直接借 |
| C1 | tmux 会话承载 agent、TUI 看板 | 需改造（我们的运行宿主不同） |
| C2 | tasks.md 七态 + 字段 + depends_on | 直接借 |
| C2 | Scenario contract + before/after 证据 + finisher 终态校验 | 直接借 |
| C2 | review/CI/PR 分项上限 + blocked 需 operator ack | 直接借 |
| C3 | 每项目一个 coordinator + tick + 依赖 + 并行槽 | 直接借 |
| C3 | 文本路径重叠启发式串行化 | 需改造（应换成显式 scope） |
| C3 | PR autopilot 阶梯（fix/rebase/rederive/raise）+ 事件集合 + durable watch | 直接借 |
| C4 | 计划评审 + sha256 失效 + 双模型 | 直接借 |
| C4 | reviewer 的 Scenario-contract 透镜 + P0/P1 修后复验 | 直接借 |
| C5 | 文件租约 + 单写者 + 原子写 + 启动 reconcile | 直接借 |
| C5 | mark-launch-attempted 三态 + journal + generation 栅栏 | 直接借 |
| C6 | 自动交接 + 结构化交接文档 + resume prompt | 需改造（阈值按模型窗口定） |
| C6 | learnings.md + 最近 5 条注入 | 直接借 |
| C7 | "coord 只派发不实现"的 PreToolUse 钩子 | 直接借 |
| C7 | worker 独立 worktree + `worker/<slug>` 分支 | 直接借 |
| C7 | 默认 skip permissions + 无 OS 沙箱 | 不适用（必须反过来做） |

## 10. 不可照搬与风险

- 最大风险：`--dangerously-skip-permissions` + 无 OS 沙箱。Fleet 的假设是单机、单 operator、可信仓库；我们若接外部仓库或共享机器，必须先加沙箱/网络与密钥边界。
- coord 的写钩子只约束 coord，子代理（worker/reviewer/finisher）豁免；"隔离"主要是 worktree 约定，不是安全边界。
- 文本路径重叠是启发式，false negative 会真的互相覆盖；我们的并行调度应有显式 scope/分区。
- 单机单 operator、文件状态、tmux 会话：多机/多用户不适用。
- 官网与源码阈值不一致（50/70 vs 40/50），且 profile 会切换绝对 token 与百分比；移植时要固定一套口径并写测试。
- 40% 交接对 1M 窗口可能过早；预算档用绝对 token 更稳。建议我们按"剩余可用 token"而非百分比触发。
- PR 自动修复可能掩盖设计问题；实质性冲突必须升级，不能自动化。
- 无封闭失败原因枚举，恢复逻辑分散在多个组件；我们应补一个 ADLC 式 reason 集合。

## 11. 原文索引

- https://fengshen.dev/fleet/
- https://github.com/edisonshen/fleet
- 仓库内路径：`README.md`、`docs/COORDINATOR-WORKFLOW.md`、`docs/STATE.md`、`docs/DESIGN.md`、`docs/FLOW.md`、`skills/coordinator/SKILL.md`、`skills/coordinator/pr_watch.py`、`internal/handoff/handoff.go`、`internal/tasks/tasks.go`、`internal/workers/workers.go`、`internal/learnings/learnings.go`、`cmd/fleet/dispatch.go`
