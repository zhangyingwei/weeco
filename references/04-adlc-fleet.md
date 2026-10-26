# ADLC Fleet 参考

> 源码公开：github.com/voodootikigod/adlc（MIT），核心在 `packages/fleet`；官方文档 agenticlifecycle.ai/docs/toolkit/fleet。版本以仓库 `packages/fleet/README.md`、源码与 `docs/specs/fleet-orchestration.md` 为准。

## 1. 一句话与定位

ADLC 是 Agentic Development Lifecycle 工具链；`fleet` 是 P4 并行构建 + P5 逐票起诉的执行器。它把 ticket DAG 变成真正的并行执行：每个 ready ticket 交给一个无头 Claude Code worker，在独立 worktree 中构建，通过确定性门禁与阻断式跨模型对抗审查后，顺序合入单次运行的集成分支。核心原则 D0：控制流是代码，判断是模型——排序、重试、合并都不由 LLM 决定。MIT 开源。

## 2. 架构与运行方式

- 计划源是既有 `.adlc/tickets.json`，不引入新格式。`completed:true` 从活动集过滤，但仍满足依赖边；边方向为前置 → 后继。
- 就绪条件：未完成、本轮未合并/失败、所有前置已合并或 completed、scope 与任何 in-flight ticket 不重叠、有空闲并发槽。
- 事件驱动调度：每次合并后重算 ready 集；默认并发 2；合并严格串行（进程内 merge lock），构建与门禁可重叠。
- 每个 ticket 使用 `.worktrees/fleet-<id>/`，分支 `fleet/<id>`，从当前集成分支 tip 切出并记录 `startSha` 作为后续 diff 基准；orchestrator 负责 commit。
- 完成票以 rebase-first 顺序合入 `fleet/run-<runId>`，绝不写 base；run 结束最多开一个 PR，fleet 从不 push。合并后在集成分支跑 post-merge gate，失败则回滚（先断言 HEAD 仍是该 merge commit，否则停下交人）。
- 状态持久化在 `.adlc/fleet-status.json`（schemaVersion=1）；git 是事实源，状态文件是缓存。

## 3. 角色与职责

| 角色 | 职责 | 边界 |
| --- | --- | --- |
| operator | 配置、批准、最终 P6 合并 | 危险开关只能 operator-local argv |
| fleet orchestrator | 读 DAG、调度、commit、merge、写状态 | 不含任何 LLM 决策 |
| worker（builder） | 在 worktree 内实现 ticket | 只改 scope；不 commit；结束必须输出 `TICKET-DONE` 或 `TICKET-BLOCKED: <reason>` |
| deterministic gates | build/test、rails-guard、scope、protected-path | 代码判定，失败消耗 strike |
| prosecutor | 新鲜上下文的跨模型 `adversarial-review` | 阻断式；无 provider 则 fail closed |
| merge-forecast / quartermaster | 预检证据 / 可选按 ticket 选 adapter+model | 前者 advisory，后者 fail-closed |

谁写 ticket：人/ticket shaping 工具（tickets、spec-lint、premortem）；fleet 只消费 DAG，并在 post-merge gate 通过后走 `adlc ticket complete` 同一路径把票标 completed（幂等、有门禁）。没有 per-run 人工拆分门，人工门在 ticket 进入与 P6 合并。

## 4. 任务状态与调度

- 状态机：`pending → building → gating → prosecuting → fixing → merging → merged`，旁支 `failed | blocked | paused`；`blocked` 不消耗 strike。
- 单写者分区：同一 scope 同时只有一个 worker；scope 重叠的票被串行化——冲突是被调度掉的，不是被解决的。
- 预算：并发默认 2；strike 上限默认 2（可 1..50）；`--wall-clock-minutes` 是整轮绝对截止时间，超时杀进程组、记 `wall-clock`、exit 2 且可恢复。
- `flail-detector` 判定真 flail 时跳过下一 strike（该工具 fail-open）。
- 失败原因是封闭集合：`quota-paused`、`lock-held`、`wall-clock`、`strikes-exhausted`、`ticket-blocked`、`flail`、`review-unavailable`、`mirror-fetch-failed`；运行级另有 `quarantined`、`pr-open-failed`、`preflight`、`resume-refused`、`dispatch-refused`，均表示需要人。
- exit：0 全部合并/干净 dry-run；1 操作错误；2 至少一个票失败/阻塞/暂停。

## 5. 验证与门禁

1. 预检：repo lock + 干净树；canary dispatch 证明 allowlist 管道可用；rail-hook 探针缺失则大声警告（只剩确定性门禁）。
2. 预跑：`adlc merge-forecast --json` 作为证据，不构成第二套强制。
3. worker 内：`ADLC_P4_ENFORCEMENT=1` + `ADLC_TICKET` 启用插件 PreToolUse，拦 rails 与 trust root 写入。
4. 合并前确定性门禁（Bash 拼写也逃不掉）：`fleet.gate` build/test；`adlc rails-guard --base <startSha> --ticket`；scope 检查（`git diff <startSha>..HEAD` 每路径匹配 ticket scope）；protected-control-file 完整性扫描（`.claude/**`、`.adlc/**`，含信任根 `.adlc/tickets.json`、`current-ticket.json`；唯一惰性白名单 `.adlc/fleet-logs/**`；"被 git 列为改动但缺失/不可读"也失败）。
5. 起诉：对 ticket-local diff 跑真实 `adversarial-review`，新鲜上下文、优先不同 provider；达到 `prosecuteFailOn`（默认 medium）即转 fixing，修复后重跑整条门禁链；无 provider 则 fail closed。
6. 合并后 gate 失败 → 回滚；证据写入 `fleet-status.json`，可用 `gate-manifest record` 写入 `.adlc/manifest.jsonl`（prev 为上一行 SHA-256 的哈希链，可 HMAC 签名）。

## 6. 上下文与交接

- fleet 的基本单位是"一票一新上下文"：每个 ticket 新开 worker，天然规避上下文腐化。
- 失败信息不直接回灌：strike 2 的 regeneration charter 把上轮诊断包在不可猜测围栏 `<<UNTRUSTED:PRIOR_FAILURE:<uuid>>>` 中并声明为惰性数据，防 prompt injection。
- 长会话由工具箱 `context-handoff` 提供绝对阈值（pct/depth/bytes 三信号 OR 连接）：warn ≥50% / 20 / 128KiB；handoff ≥60% / 30 / 192KiB；hard ≥80% / 40 / 256KiB。会话终态写操作 deny 分 D1（本会话）、D2（重入仍粘滞）、D3（其他会话未消费也阻断）；`adlc handoff write/resume/consume/bypass/repair/unlock` 默认 dry-run，写操作需 `ADLC_MANIFEST_KEY` 并落 `.adlc/manifest.jsonl`。
- trace/manifest：`fleet-status.json` 记录每票状态、strike、worktree、lastExit；`fleet status --json` 供外部只读观测；`gate-manifest` 哈希链供 CI/P6 复核；auto-complete 的证据随单个 PR 走。

## 7. 隔离与权限

双平面，网络与文件系统是两个独立轴：

| | repo-command plane | model plane |
| --- | --- | --- |
| 网络 | 拒绝 | 允许（worker 要连 provider） |
| 写 | 仅 worktree + synthetic HOME | worktree + adapter 声明的 `homeState` |
| 读 | worktree + 只读运行时白名单 | 宿主机范围（已记录的残余风险） |
| HOME | worktree 内合成目录 | operator 真实 HOME |

- 后端：Linux bwrap、macOS sandbox-exec；没有后端就拒绝 dispatch（fail closed）；`--i-am-in-a-disposable-container` 只能由 operator CLI 指定，仓库配置设置会被警告并忽略。
- worker 权限：`claude -p --permission-mode acceptEdits` + settings.local.json allowlist，只含 gate 命令、allowedCommands、只读 git；不允许 `dangerously-skip-permissions`；默认不允许 commit/install（install 只有显式 allowlist 才可）。
- 环境：repo-command plane 只留 PATH、ADLC_*、配置命名键，provider key 与 `*_TOKEN`/`*_SECRET` 全部剥离；model plane 只留自身模型认证，不继承任何 ambient `ADLC_*`；`ADLC_MANIFEST_KEY` 硬拒绝，任何 modelAuthKey 都解不开。
- 模型/adapter/凭据都是 operator-local：`.adlc/config.json` 里的相关值会被警告忽略，防止候选树选择自己的裁判。

## 8. 失败与恢复

- `quota-paused` 与 `wall-clock` 是可恢复暂停：原样重跑同一命令即可，无需也不存在 `--resume`；strike 会交回。
- `lock-held` 是稍后重试；`strikes-exhausted`、`ticket-blocked`、`flail`、`review-unavailable`、`mirror-fetch-failed` 对该票终态。
- `paused` 既非 in-flight 也非终态，reconcile 时回到 `pending` 且保留 strike 数。
- 锁：`.adlc/fleet.lock/owner.json` 记录 pid/host/runId/startedAt/procStartTime；只有 pid 存活且进程启动时间匹配才算 live（防 PID 复用），否则回收；`fleet unlock` 是兜底。
- 恢复：同 run 中票分支是否 `merge-base --is-ancestor` 集成分支判断 merged；有 worktree 无 in-flight 进程则回 pending；死 run 的 worktree/分支清理，中断的 rebase/merge 先 abort；集成分支丢失或 baseSha 移动则拒绝 resume，要求新 run。
- strike 间重置：`git checkout -- . && git clean -fd`；受控文件删除后按模板重建，只有 `.adlc/fleet-logs/**` 保留。

## 9. 可借鉴清单（映射到我们 7 组件）

口径：C1 角色与载体矩阵、C2 终态协议、C3 事件驱动协调、C4 审查者工具白名单与审查台账、C5 单写租约与幂等派发、C6 上下文预算与状态包交接、C7 权限预检。

| 组件 | 机制 | 判断 |
| --- | --- | --- |
| C1 | builder 与 prosecutor 分离；控制流不含 LLM | 直接借 |
| C1 | WorkerAdapter 六种 harness 抽象（角色 × 载体 × 模型层） | 需改造（先支持 1–2 种） |
| C1 | quartermaster 按 ticket 路由 adapter/model | 不适用（当前规模不需要） |
| C2 | 状态机 `pending→…→merged` + `paused/blocked/failed` + 封闭 reason 集合 + exit 0/1/2 | 直接借 |
| C2 | 确定性门禁链（build/test/rails/scope）作为终态验收条件 | 直接借 |
| C2 | strike/墙钟/quota 暂停 + 可恢复；exit 0 ≠ accepted | 直接借 |
| C3 | tickets.json DAG + 事件驱动重算 ready + 依赖满足 | 直接借 |
| C3 | scope 重叠串行 + 合并严格串行、构建/门禁并行 | 直接借 |
| C4 | 阻断式跨模型审查 + severity 阈值 + 修复后全链重跑 | 直接借 |
| C4 | gate-manifest 哈希链/HMAC + 证据绑定 ticket/startSha | 需改造（可先 JSONL+hash） |
| C5 | 单写者分区 + `.adlc/fleet.lock` owner（pid+startTime）+ stale 回收 | 直接借 |
| C5 | paused 恢复 + git 事实源 + ancestor 对账 + 原子状态 | 直接借 |
| C6 | 一票一新上下文 + 失败信息围栏化 | 直接借 |
| C6 | context-handoff 绝对阈值 + D1–D3 deny | 需改造（我们可能用交接文档而非 deny） |
| C7 | 双平面 OS 沙箱 + env allowlist + 密钥剥离 + operator-local | 需改造（执行环境可能无 bwrap） |
| C7 | 每票独立 worktree + per-run 集成分支 + 绝不写 base | 直接借 |
| C7 | protected-path 完整性扫描 + 预检 canary/rail-hook 探针 | 直接借 |

## 10. 不可照搬与风险

- fail-closed 依赖 OS 沙箱后端；无 bwrap/sandbox-exec 的机器无法直接运行。macOS Seatbelt 表达力有限，bounded read、私有 tmpfs 等能力不可用。
- model plane 的读与出网仍未封闭，设计假设是"仓库可信"；不可用于不可信仓库。
- 仓库配置是攻击面：adapter/model/allowlist/extension 开关一律 operator-local，否则候选树可自选裁判或放宽边界。移植时必须保留"仓库配置只能收紧、不能放宽"。
- 门禁质量取决于 ticket scope 的准确性；scope 写得太松，单写者串行化与 scope check 同时失效。
- 跨模型审查依赖第二个 provider 与额外成本；provider 不可达时整个 run 不能合并，需准备降级策略但不要静默放行。
- 合并到集成分支后仍需人工 P6；fleet 不 push、不写 base，这是刻意设计，不是缺陷。

## 11. 原文索引

- https://agenticlifecycle.ai/docs/toolkit/fleet
- https://agenticlifecycle.ai/docs/toolkit/tickets
- https://agenticlifecycle.ai/docs/toolkit/context-handoff
- https://agenticlifecycle.ai/docs/toolkit/gate-manifest
- https://github.com/voodootikigod/adlc/tree/main/packages/fleet
- https://github.com/voodootikigod/adlc/blob/main/docs/specs/fleet-orchestration.md
- 仓库内路径：`packages/fleet/README.md`、`packages/fleet/lib/{plan,scheduler,gates,protected-paths,complete,sandbox,lock,resume}.mjs`、`docs/specs/fleet-orchestration.md`
