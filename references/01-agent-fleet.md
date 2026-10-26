# chankov/agent-fleet 参考笔记

> 来源：`github.com/chankov/agent-fleet`，本地 clone `/tmp/refs-agent-fleet`，commit `8391895`（package v2.0.23，2026-10-25）。引号内保留原文英文术语；以下只摘录对重建“Agent 舰队”有帮助的机制。

## 1. 项目一句话与定位

Agent Fleet 是 **pi-first 的多 Agent 编排系统**：一个 **thin dispatcher（agent-hub）** 把任务路由给 specialist，整个团队在 **Verification Contract** 下工作；团队可作为 Herdr tiled workspace 快照/恢复，agent 之间通过 **coms** 点对点通信，Hermes Desktop 面板展示全舰队状态。README 的口号是 “Many agents. One direction.”，明确区别于“单个 chat session”。

定位有三点：**BYO subscriptions**（Codex/Copilot/Claude/Ollama/本地模型混在一个 fleet）；**dispatcher 的上下文永不淹没**（研究输出、子 Agent dump 全部落盘）；**可观察**（Herdr pane、Hermes panel、手机 relay）。方法论见 `docs/PHILOSOPHY.md`：**deterministic process, room for judgment**——harness 拥有流程，模型只在流程内贡献判断；“完成”必须有证据，exit 0、文件存在或模型宣称都不算。安装目标是 **pi**，Claude Code 只作为 **coms peer** 接入，不是安装目标。

## 2. 运行架构

| 层 | 职责 |
|---|---|
| **agent-hub** | 一个 pi runtime，两种工作模式。**operator**：直接 `read/bash/edit/write`、orchestration、embedded coms，默认空 roster。**orchestrator**：移除直接编码工具，保留 dispatch、research、assertions、`ask_user`、只读 `filesystem` 与 coms/Herdr 工具；自读上限 **64 KiB/file、64 KiB/turn**。Hub 在磁盘上拥有 Verification Contract。 |
| **Workflows** | `just flow`：headless、code-owned 的 phase order、bounded retries、executable gates、writes enforcement、单一 acceptance。agent 只用于需要判断的 phase。 |
| **Herdr** | fleet control plane：tiled peer workspace、presence push、snapshot/resume。 |
| **coms** | peer data plane：envelope 形式的 P2P 消息，unix socket/named pipe，按 project 注册。 |
| **Claude Code bridge** | 把交互式 Claude Code pane 变成双向 coms peer。 |
| **Hermes** | 远程人类控制：`ask_user` 转发 Telegram，手机与本地答案竞速；Desktop fleet panel。 |
| **Skills + personas** | skills 是“怎么做”，personas 是“谁”，commands 是“何时”。 |

三个独立轴：**Work Mode**（operator/orchestrator）、**Native roster**（`teams.yaml`，本地 headless Pi 子进程）、**Peer topology**（`peers.yaml`，独立 Pi/Claude Code 进程）。task tier 只影响 rigor，不改变 work mode。启动规则：显式 `--work-mode` 优先；`--agents <roster>` 隐含 orchestrator；否则 operator；orchestrator 启动必须有 roster。

组成规则：**hub 或 slash command 负责编排；persona 不调用 persona**；specialist 只能 fan out 到自己声明的 sub-agents；research helper 写盘，hub 只拿路径。

## 3. 角色与载体

- **Roster（native）**：`teams.yaml` 的命名团队，`dispatch_agent` 启动本地 Pi specialist 子进程，一个 persona = 一个 specialist session。默认 `default` = builder / test-engineer / code-reviewer / documenter；另有 plan/debug/frontend/security/hotfix/release/info。
- **Peer**：`peers.yaml` 的长期独立进程，占 Herdr sibling pane。字段：`name`（coms 身份）、`persona`、`model`、`extensions`、`env_file`、`runner: claude-code`。
- **同名双载体**：`dispatch_agent(..., backend: "native" | "coms" | "auto")`。native 永远本地；coms 必须找到同名 live peer，失败不回退；auto 读 `.pi/agents/dispatch-policy.yaml`（`prefer`、`fallback: native|none`、`grace_s`、`timeout_s`）。原文强调：“HOW a peer runs stays in peers.yaml; dispatch-policy only decides WHEN”。
- **Persona 权限**：15 个 persona，`docs/agents.md` 标 rw / read-only：planner 仅写 plan 文件；builder/test-engineer/documenter/architect/releaser 为 rw；code-reviewer/plan-reviewer/security-auditor/web-performance-auditor/researcher/deep-researcher 只读。research helper 以 `--no-skills --no-context-files` 启动，工具限于 `read,grep,find,ls`。
- **Sub-agent（delegate）**：persona 在 frontmatter 声明 `subagents:` 后获得 `delegate(role, instruction, context?, allow_write?)`。每 dispatch 最多 4 个 child，`delegate_depth` 默认/上限 1；child 默认只读，只有单个 live child 可 `allow_write: true`，并发 child 强制只读；声明 role 的工具上限取交集。
- **Claude Code peer 不带 persona**：`runner: claude-code` 启动 Claude CLI + bridge，role 由它自己的 session 决定；`peer-launch` 会拒绝 `--persona`。

## 4. 任务与状态

- **Task tier**：`set_task_tier` 在第一次 dispatch 前分类 `trivial/small/feature/project`。跳过 = small；降级免费，升级必须给 `reason`（否则代码拒绝）；tier 是 task-scoped，跟随后续消息。tier 决定 child deadline（600/600/1800/1800 秒）、session recycle（3/3/5/5）、persona gate、review finding cap（1/2/5/uncapped）。Planner/plan-reviewer 在任何 tier 都只是可选。`set_task_tier` 还接受显式 `risk`（`unknown|low|high`）和 `scope`（`read-only|small|wide`）；risk 不靠任务措辞推断，`unknown` 允许只读研究但不能接受 changed work，high risk 加独立 review，wide 还要求独立 review。
- **Dispatch 输入**：`dispatch_agent` 接受 `scope: string[]`（advisory globs）、`deliverables: string[]`（精确产物路径）、`artifacts: string[]`（只传路径 + 首标题/一行预览，不传正文）、`backend`、`watchdog`、`review_reason`。pre-flight 校验免费：未知 persona、解析不了的 artifact、低 tier 的重 persona、docs-only reviewer 都在 spawn 前拒绝，不花预算。非法输入返回 `invalid_invocation_shape`；合法但无法持久化返回 `invocation_persistence_failed`，都不启动 executor。
- **断言**：ledger 由 `set_assertions` 建立，每条必须有 `source`；open ledger 软上限 8；sourceless batch 直接拒绝并点名 id。
- **终态**：schema `agent-fleet.runtime-result/v1` 把结果拆成四个正交维度——`execution`（`completed|failed|pending`）、`changes`（`changed|unchanged|unknown`，带 `certain|uncertain|not_observed` 归因）、`verification`（`passed|failed|missing|stale|unsupported`）、`acceptance`（`accepted|not_accepted`）。兼容展示为 `not_available|deliverable_failed|needs_verification|accepted`；flow 用 `accepted|rejected`。**exit 0 = completed，不等于 accepted**；没有 qualifying runtime checks 时是 `completed_unverified` 或 `verification_failed`，`accepted: false`。`needs_verification` 的含义是“未接受”，不是“快好了”。
- **取消**：operator cancellation 以 **agent identity** 为 fence，覆盖 disjoint scope、模型/措辞变化和 dispatch/research 输入；不取消其他 agent 已在跑的工作；task reset 不清除 fence。`/af-agents-kill` SIGTERM 整个 process group；coms-backed run 只能“放弃等待”，peer 继续跑。
- **失败分类**：T1 recovery contract——no automatic retry, queue, model fallback, or new numeric allowance。`not_started`/`started:false`/`effects:none` 是 preflight refusal，0 launch、0 indeterminate；busy 在计费与历史变更前拒绝；timeout without close 保持 uncertain；`turn_timeout` exit 124（保留 partial output），`drift_stop` exit 125。T3 是 tool protocol error：工具形文本被误当已执行动作时的对账失败。refusal 计数与真实 launch 计数分开。

## 5. Verification Contract

原文在 `skills/orchestration-verification/SKILL.md`。核心问题：多 Agent 运行会“静默丢需求”；传播通常不是问题，**验证**才是问题——没有任何东西真正执行需求并拒绝“done”。

1. **Acceptance assertions**（dispatcher-owned，先于任何 builder）：
   ```
   A<N> `<tag>`: <single checkable pass condition>
   ```
   tag 只能是 `test` | `runtime-ui` | `code-grep` | `manual`。复合需求必须拆成独立断言。dispatch 时把相关断言 **word-for-word** 放进任务，不能转述；只有 `proven` 才推进，`unproven` 和 `failed` 都算 not done。`runtime-ui` 必须由真实运行观察（DOM/screenshot/network）关闭，静态 review 不算。
2. **Parity / touchpoint inventory**：任何 “make X behave like existing Y” 请求，先让 `deep-researcher` 穷举 exemplar 被特判的每一个 site（feature flags、branches、display logic、validation、translations、fixtures、tests），每个 site 变成覆盖整个集合的断言；inventory 归 dispatcher 所有，下游 specialist 只消费、不重新推导。示例把 Walkover/Retired/Disqualified 拆成 A2/A3/A4 per-type 断言，防止“只修 exemplar”。
3. **Structured upward return**（原文格式）：
   ```
   changed_files:        [path:line — 3–6 word note, …]
   assertions_proven:    [A2: … ✓ — evidence: <test name | screenshot | grep result>, …]
   assertions_unproven:  [A4: … — NOT checked, …]
   assertions_failed:    [A3: … — evidence: <runtime observation>, …]
   tests_run:            [command → result, …]
   open_risks:           [non-blocking concerns, assumptions made, …]
   requires_user_decision:[questions only a human can resolve, …]
   ```
   没有 named evidence 的 “proven” 一律降级为 unproven；`runtime-ui` 的 evidence 必须是 `.pi/agent-sessions/artifacts/evidence/` 下已存在的 runtime artifact。hub 机器解析 return，完整原文写 `returns/<agentKey>-<dispatchId>.md`，只向 dispatcher 暴露 `details.structuredReturn` + `details.returnPath`；`contractNotices` 标记缺 assertion id 或无 evidence 的条目。`compilerDiagnostics` 发现 changed file 有编译错误时，只把该次 dispatch 的 proven id 降级，附 `compiler_diagnostics`，不会提升断言。
4. **Requirement-regression reset**：用户说 “wrong again” 时，先作废该区域所有旧 “proven/approved”，以最新 correction 为唯一事实源重建断言（反向指令是替换不是追加），必要时重跑 parity inventory，然后才重新 dispatch。

运行时：`set_assertions` 把 ledger 持久化到 `.pi/agent-sessions/assertions.json`，状态行 `Assertions: 2✓ 1○ 1✗ · open: A4` 让合同在 compaction 后仍可见；compaction 后 dispatcher 必须 `get_assertions` 读回全文再 gate。`test_command` 只声明、不执行；native child 走既有 bash 审批路径，`runtime-test-check` 把 command、真实 exit、before/after revision 绑定到 task。**Advisory by design**：hub 不会因断言未 proven 硬拒绝 dispatch；但 sourceless 和 >8 open 会被拒/警告。`manual`/`runtime-ui` 没有 trusted producer，fail closed 为 missing/unsupported。

## 6. 协调协议

1. **Sentinel（read-to-decide）**：生产者最后一行给机器可 grep 的 verdict。live 机制：`NEEDS_RESEARCH: <question>`（specialist 暂停，hub 研究后带路径 resume）、`ASK_USER:`、`EXTERNAL_BLOCKED:`、delegate 的 `DIGEST:`、Claude Code bridge 的 `<<COMS_DONE:msg_id>>`。缺失 sentinel 本身就是 verdict（按失败/unknown，绝不按成功）；只 grep 最后几行，防止 agent 在正文里“提到” sentinel。
2. **Push over polling**：`coms_await` 阻塞等 `msg_id` 的 response envelope；Herdr 下用 `events.subscribe`（`pane.agent_status_changed`、`pane.created/closed/exited`）替代 10 秒 ping；`herdr wait agent-status` 和 `pane.wait_for_output` 是 server-side wait。status wait 只在**变化**时触发，先读当前状态；reconnect 必须 resync snapshot；所有 wait 都要 timeout。
3. **Barrier**：`coms_send` + `coms_await` 是 peer 间 request/response barrier；hub 等所有并行 dispatch 的 structured return 到齐才进 contract gate；`/af-handoff` 是全 session barrier：dispatcher 写 self-contained brief，send 后 await，再交控制权。ack 只表示“收到”，不等于“完成”，要等 response envelope。
4. **Fan-out / fan-in digest**：`spawn_research` 把 findings 写 `.pi/agent-sessions/findings/*.md`，hub 只拿路径；artifact bus 用 kind 标记，输入只给 preview + path；delegate 必须返回 ≤30 行 `DIGEST:`，带 `path:line` 引用和 result-file path，缺 digest 则父级收到前 ~30 行 + 警告。merge 方只读 digest，按需拉全文。`/af-poll` 和 `/af-debate` 是同一问题的多模型 panel：voices 只读、并行、各自落盘，向上只回 digest；debate 2–5 轮，每轮把上一轮 labeled positions 喂给同一 voice session；**没有 judge**；`--apply` 时只有 integrator 能写，且必须持有 cross-process **writer lease**（0600、pid/owner，非 owner release 是 no-op）。
5. **Racing with cancellation**：参考文档明确写 **not implemented as a first-class mechanism**，输掉的 peer 会继续烧 token 直到 turn 结束。注意：当前源码 `coms-envelope.ts` 已定义 `CancelEnvelope`（`ref_msg_id`），说明文档落后于 wire schema；但 hub 文档仍说 coms-backed kill 只是 abandon wait。重建时不要把“有 schema”当成“端到端已实现”。
6. **Poll / debate**：`/af-poll [--panel]`、`just flow poll`；panel 来自 `.pi/agents/voices.yaml`；artifact 落 `.pi/agent-sessions/artifacts/polls/<runId>/`；debate 在 `debate/round-<n>/`。可选 `references/communication-contract.md`：一句话立场、每条 `case` 必须带证据、必须写 `would_change_my_mind`、辩论必须点名 voice、silent conversion 不允许、merge 必须给每个 consensus/divergence/minority 归属。

通信底座：envelope 字段含 `msg_id`、`sender_session`、`hops`、`prompt`、`response_schema`、`reply_timeout_ms`；`MAX_HOPS=5`，`DEFAULT_TIMEOUT_MS=1_800_000`；注册表 `~/.pi/coms/projects/<project>/agents/<name>.json`；heartbeat 30s、90s 内视为 fresh；peer status `idle|queued|working|booting`，`coms_list` 给出 `pane_id`、`queue_depth`、`status`。**Pool scope 是 reach boundary**：默认只看自己的 project，`--explicit` peer 默认不可见；只有人类能 `/af-coms --project` 或 `--all` 扩大，`coms_list` 参数不能扩大。

## 7. 上下文与交接

- **Thin dispatcher**：判断标准只有一句：“does this persistently enter the dispatcher context?” 会，就落盘或压成一行状态。研究全文进 findings 文件，dispatcher 只看一行 notice；Verification Contract 在 `assertions.json`，dispatcher 只看状态行。orchestrator 自读上限 64 KiB/file 和 64 KiB/turn，超过就 `spawn_research` 或 dispatch 一个带路径的 specialist，而不是把正文塞进上下文。
- **Bounded output**：`assist.bounded-output` 固定 500 inventory entries/page、64 KiB per content/reply、180 Unicode chars per preview；截断必须给出 retained full-output path 和 retrieval handle，不能静默截断。`filesystem` 工具做 deterministic inventory/excerpt/readback，opaque handle 绑定 canonical path、content hash、offset，stale bytes 拒绝，symlink escape 拒绝。
- **Artifact bus**：`.pi/agent-sessions/artifacts/` 下 `returns/`、`failures/`、`plans/`、`reviews/`、`inventories/`、`evidence/`。失败/超时写 `failures/`，绝不写成 return——一个 142 字节的 coms error stub 曾被当 code-reviewer 的 return，害得下一次 dispatch 花 103 秒调查一个其实已成功的 review。session start 把上一 session 的 artifacts **archive** 到 `.pi/agent-sessions/runs/<runId>/`（immutable `meta.json` + `runs/index.json`），而不是 `rm -rf`；archive 失败就留在原地（stale tree 可恢复，deleted 不可）。
- **Compaction recovery**：hub 采样 provider usage，60% warning，70% abort 下一次 provider request、等 tool result 落盘后执行一次自动 compaction，80% ceiling 停止；低于 60% 或 history 增长 10% 后 re-arm。recovery single-flight，启动/同 turn 期间收到的输入（含图片）保留在内存，成功后 replay once。compaction 后必须 `get_assertions`。roster 恢复 fail-closed：只持久化 team name，resume 时重新解析；team 缺失/改名/为空则保持 orchestrator、继续禁用直接工具、阻塞 model input，直到 `/af-agents-team`、`--agent-team` 或显式 operator。
- **Handoff**：`/af-handoff <peer>` 不让 LLM 提取 compaction summary，而是让 dispatcher 写 “everything the target needs, assume no shared history” 的 brief；send 时机器追加 `## Verification ledger (verbatim, machine-appended)` 和 `## Artifact index`（路径 + 首标题）。目标必须把机器追加的 ledger 当作 contract，而不是 brief 的转述。pi session 不可移植，所以没有 raw session copy。
- **Context hierarchy**：rules → spec/architecture → relevant source → error output → conversation；75% 开始 trim；先压缩再删除；把最任务关键内容放在最后（lost-in-the-middle）。

## 8. 审查与证据

- **Review evidence 绑定**：`review-reuse.ts` 的 reuse 条件是 exact match——同一 reviewer（whitelist：`code-reviewer`、`plan-reviewer`、`security-auditor`）、同一 `taskId`、非空且排序后的 `scope`、`scopeRevision`（worktree revision 的 64-hex）、`contractHash`（accepted input digest + risk + scope + changedFiles + additions + requirements declarations + input artifact hashes），以及 retained evidence 文件本身的 SHA-256 未变。命中则 `review_reused`，不启动 child，保留原 evidence path。任一 changed accepted input、source、requirements、task、scope、evidence、concurrent writes，或显式 `review_reason`，都必须 fresh review；blocking findings、unresolved questions、rejected reviews 不能提供 reusable approval。review closure 状态 `closed | unproven | invalidated`；`changedAfterReview` 与 scope 相交即 invalidated；evidence revision 必须等于 current revision；reviewed assertion path 变了，`updateAssertion` 拒绝。
- **Approval memory**：semantic decision 绑定 accepted task/input revision 和 exact path；task/session 变化会 fence 迟到的答案；只有“向人类展示了完整命令”的 exact-action dialog 才记录 delete decision；free-form `ask_user` 从不记录、从不自动回答（prose 无法证明人类选了哪条路径/极性）。remembered confirmation 不是可复用的 effect grant，真正的不可逆效果仍要消耗自己的 runtime authorization；相同并发 dialog 会 coalesce；cancellation 不是 consent。human dialog 的完整 JSON 有 **8 KiB（8192 bytes）** 上限，超限/敏感/不匹配直接拒绝，不截断、不做部分脱敏。
- **One-use authority**：源码用 `WeakMap` + frozen object 实现：hash 只是绑定数据，不是 authority；`consume` 必须对象身份和 key 同时匹配。
- **Evidence archived, never overwritten**：见上一节 `runs/<runId>`。`submit_result` 要求 `evidence: [{path}]`，path 必须在 workspace 内并通过 damage-control read policy，工具记录 SHA-256 + size；proven 声明必须引用其中之一。**hash 只证明文件身份，不证明断言为真**。
- **Runtime-ui 证据**：必须是真实运行观察；静态 review / 代码阅读不能关闭。README 明确 “accepted is not human Checkpoint A approval”；runtime 只证明 execution/state，不证明测试或 code-grep predicate 的语义充分性。

## 9. 权限与安全

- **Worktree targeting**：推荐布局 `root/af`（Fleet workspace）+ `root/repo`（主 checkout）+ `root/repo.worktree`（已注册 linked/detached checkout）+ `root/docs`（独立文档仓库）。`workspaceRoot/resourceRoot/configBase = af`，`repositoryRoot = repo`，`executionRoot/worktreeRoot = repo.worktree`；相对 selector 从 `af` 解析。`just fleet --worktree <registered-checkout>` 把 session 绑定到不可变的 canonical filesystem/Git registry identity，**independent of HEAD**；comms、retention、resume 携带 target identity；serialized transport 每次重新校验，绝不当作 permission。移除/替换 target、history 不匹配、缺 policy 都拒绝，不 fallback 到 main；没有 live switch。文档明确：这是 operational routing 和 bounded guards，**不是 OS sandbox**，也不隔离外部服务。
- **Workspace 边界**：Fleet 不得复制或 symlink 进 application checkout；外部 docs/rules 必须显式配置为相对 `af` 的路径，read grant 不等于 write；`--all-extensions`、自定义 `--session-dir` 和 worktree selector 在 lifecycle 命令中被拒绝。System 1 每个 consumer 要单独 consent，main 的授权不自动扩展 source export。
- **Writes policy（flow 层）**：每个 agent phase 必须从 persona frontmatter 或 call site 拿到显式 writes policy；缺失在 spawn 前拒绝，绝不默认为 unrestricted。`writes: []` = 仓库只读；非空 = allowlist；`*` 不跨 `/`，`**` 跨目录。`protectedGlobs` 只有在没有任何 `writes` glob 匹配时才拒绝——所以 `writes: ["**"]` 会连 protected path 一起允许。执行方式是 phase 前后 snapshot diff（tracked numstat + untracked），越权改动 **rollback 并 fail phase**；run 前已 dirty 的路径不重建，但 agent 把它 restore 回 HEAD 也算 change。Hub 的 `scope` 只是 advisory：`details.scopeViolations` + ⚠ notice，不 block、不 revert、不 escalate。
- **Sandbox**：`assist.write-isolation: true` 时 dispatch 先 snapshot，强制 native child（explicit coms 直接拒绝，因为无法声称 remote isolation）。Linux 用 bubblewrap，macOS 用 Seatbelt；repo/root 只读，只允许 existing exact relative files、显式允许的 existing recursive dirs、以及 separate current-run runtime/artifact/temp roots。glob scope、absolute/escaping scope、symlinked grant、空 writable grant、backend 缺失/平台不符、sandbox launch failure 全部 fail closed，**不做 unsandboxed retry**；approval 不能扩大已经创建的 OS boundary，要更宽路径必须重新授权 dispatch。exact-file grant 支持直接写 inode，但故意阻止 temp-file+rename 的 atomic save，调用方必须显式 grant 目录。sandboxed spawn 只允许 3 个 pipe stdio，拒绝额外 inherited FD；Linux 保留 Hub-owned process group（不加 PID namespace），假设 host ptrace/terminal injection 控制；Seatbelt 只对 `/dev/null` 和 `/dev/tty` 放开写。
- **Damage control**：`just fleet` 先加载 `damage-control-continue` 再加载 agent-hub；`.pi/damage-control-rules.yaml` 用 regex 拦 `rm -rf`、`git reset --hard`、`git push --force`、云资源删除、`herdr pane/tab/workspace close` 等；部分规则 `ask: true`（人类确认而非直接 block）。guardrail 也重新加载进每个 native specialist/research/delegate；**missing safety plumbing refuses child dispatch**。specialist 不能 spawn/close herdr pane；`herdr_close_pane` 每次都要人类确认。
- **Tool catalog**：`setActiveTools` 前后比较实际 catalog，产出 `agent-fleet.tool-catalog-delta/v1`（removed/added/active substitutes/`permissionExpansion: false`/SHA-256 identity），并持久化为 trusted session entry。`before_agent_start` latch catalog；`session_compact` 时与 Pi 当前 active tools 对账。unavailable tool refusal 按 normalized task+tool 计数，**exactly three counted refusals including the first**；mode switch、compaction、prose、真实 catalog 变化都不会 refill，只有 new-task lifecycle 重置。
- **Fail-closed 原则**：missing/off/invalid/root-off/unavailable-key 意味着“不再产生新的推断”，不是移除已有 fence；unknown effect 保持 uncertain；unavailable evidence 显式标注，不变成 0；tool availability 绝不是 gate admission。

## 10. 失败处理

- **Drift watchdog（in-flight）**：Layer 1 确定性规则、零 token——超出 dispatch 声明 `scope` 的 write/edit、同一 tool call 重复 4 次、连续 5 次 tool 失败、单 run 200 次 tool call。Layer 2 只在规则触发时升级到 one-shot cheap LLM judge（single-flight，90 秒 success cooldown），judge 读原始 task + scope + 最近 tool trail，输出 `VERDICT: ON_TRACK|DRIFTING|STUCK`。DRIFTING/STUCK 以 `drift_stop`（exit 125，保留 partial output）终止，指令是“用修正/收窄后的任务重新 dispatch **一次**，绝不原样重试”。judge 不可用记为 `judge_unavailable`，5 秒后带新 snapshot/attempt id 重试一次；第二次失败是 exhausted，不是成功的 cooldown。operator 也被 `operator-watchdog.ts` 监督；research helper 不跑 LLM judge（已有 per-tool watchdog + turn deadline，且不能写）。
- **Bounded recovery**：T1 只有一套合同——no automatic retry, queue, model fallback, or new numeric allowance。settled indeterminate failure 最多允许**一次**显式、带警告、人类授权的 retry per logical operation，前提是 prior process 确认结束且 executor idle；partial side effects 可能被重复。`/af-recover inspect|retry|reconcile|abandon`：inspect 只读；retry 不执行工作，只给 stored original-contract invocation 供另行显式调用；reconcile 只在 independent current-revision readback + nonblocking reviewer evidence 覆盖失败 attempt 时清技术 block，保留 immutable failure 和 open T2/T11；abandon 永不满足 dependent work。pending/unknown process、second grant、stale ID、missing contract 各自 fail closed，并给出 next actions。
- **Watchdog 与预算**：`agent-turn-timeout-s` 是整轮 deadline（tier 600/600/1800/1800），到期 `turn_timeout`（exit 124）并保留 partial output；`recon-search-timeout-s`（默认 120，1..3600|off）是 **per-tool** deadline；`session-recycle-runs`（3/3/5/5）或 ≥60% measured context 时 recycle；`run-history-keep` 默认 10。超时/取消时 hub 拥有并终止 child process group（SIGTERM，有限 grace 后 SIGKILL），有独立 settlement timer，`terminationConfirmed:false` 显式暴露无法确认死亡的情况。不再有 per-turn dispatch/research 配额，`max-dispatches-per-turn` 等 legacy key 被忽略并警告。
- **Blocked escalation**：`EXTERNAL_BLOCKED: <what is missing, who owns it, what it blocks>` 触发 hub 拒绝下一次 dispatch/research，返回 owner-escalation packet（缺什么、谁负责、哪些断言 blocked、已 proven 什么、人类选项：provide access / waive as UNPROVEN / drop from scope）。gate 在“人类已被 address”（`ask_user` 或下一条 user message）后打开；没有 `ask_user` 时只触发一次，避免 deadlock。`blocked_on_user` 是 paused clarification 的兼容状态，本身不强制升级；specialist 的 `ASK_USER` 先到 orchestrator，orchestrator 用 `ORCHESTRATOR_ANSWER`（basis: `existing_decision` | `repository_fact` | `routine_technical`）解决，解决不了才升级；human answer 用 `USER_ANSWER: <dispatchId> :: <question>`。
- **取消语义**：cancellation fence 绑定 agent identity，包括 disjoint scope、model/prose 变化；其他 agent 不受影响；task reset 不清除；native cancel 是 process-group kill，coms-backed 只是 abandon wait。`/af-agents-restart` 重跑最后一个 task fresh；research helper 不能 restart，每次新建 `rN`，handle 不重用。

## 11. 可借鉴清单（对应 7 组件）

口径：C1 角色与载体矩阵、C2 终态协议、C3 事件驱动协调、C4 审查者工具白名单与审查台账、C5 单写租约与幂等派发、C6 上下文预算与状态包交接、C7 权限预检；下表简称均按此对应。

| 组件 | 可直接借 | 需改造 | 不适用/暂缓 |
|---|---|---|---|
| **角色载体矩阵** | persona（who）× carrier（native subprocess / standing peer / Claude Code peer）× access（rw/read-only）× model tier 四维表；`teams.yaml` / `peers.yaml` / `dispatch-policy.yaml` 三文件分离（团队、如何跑、何时用 peer）；同名双 backend + `native\|coms\|auto` 显式语义。 | 把 pi frontmatter 换成我们的载体格式；peer 是否保留取决于我们是否有 pane/tiling 控制面；`subagents:`/`delegate_depth` 可先只做一层。 | Claude Code bridge、Herdr pane、Hermes phone relay 不照搬，除非确实要跨 CLI。 |
| **终态协议** | `execution × changes × verification × acceptance` 四维正交；exit 0 ≠ accepted；`completed_unverified`/`verification_failed`/`needs_verification` 口径；acceptance 绑定 task、revision、command、exit、evidence。 | 我们可能没有 native bash observer，可先用“声明命令 + 独立执行记录 + revision hash”实现弱化版；保留 `unsupported`/`stale`，不伪造 pass。 | 不必复制全部字段；不要引入没有 producer 的维度。 |
| **事件驱动** | sentinel 协议（`NEEDS_RESEARCH:`/`ASK_USER:`/`DIGEST:`/`EXTERNAL_BLOCKED:`）；push over polling（`coms_await`、`events.subscribe`）；barrier（send+await）；fan-out/fan-in digest（≤30 行 + path:line）；缺 sentinel = failure。 | 需要先有事件总线/阻塞等待原语；若我们的工具调用是同步 request/response，可先做 per-request await，不要假装有 push。 | racing with cancellation 目前非 first-class；没有 cancel 语义时不要并行竞速。poll/debate 可后置。 |
| **审查白名单/台账** | reviewer whitelist；review evidence 绑定 task/scopeRevision/contractHash/evidenceHash；exact match 才 reuse；changed input/source/requirements/scope/concurrent writes 强制重审；review closure `closed/unproven/invalidated`；immutable run namespace + `failures/` 与 `returns/` 分离。 | reviewer 名字和 hash 输入按我们的任务模型定义；没有 worktree revision 可用 git tree hash + 文件 SHA-256 替代；台账可先 JSON 文件。 | System 1 semantic review、proactive turn review 不照搬。 |
| **租约/幂等** | one-use authority（对象身份 + WeakMap，hash 不是授权）；writer lease（0600、pid/owner、非 owner release no-op、held 时 fail 而非等待）；duplicate `invocation_id` 相同输入幂等返回，stale/busy 不收费；refusal 与真实 launch 分开计数；`not_started` 不消耗配额。 | 未必要跨进程 lease；可先做 task-level 单写者 + 幂等 key；把“授权”和“绑定数据”分开，避免把 hash 当 token。 | Hermes monitor lease、diagnostic-series lease 等实验性机制不需要。 |
| **上下文交接** | thin dispatcher 判据；64 KiB/file + 64 KiB/turn 自读上限；artifact path + 首标题/一行预览，绝不内联正文；handoff 机器追加 ledger + artifact index；compaction 60/70/80 阈值、single-flight、queued input replay once；compaction 后 `get_assertions`；JSONL append-only 不可编辑；失败写 `failures/`。 | 阈值按我们的窗口调整；交接可能用文件/消息而非 coms，保留“机器追加、目标以 ledger 为准”的原则；artifact 归档先做只追加目录。 | Pi 的 context-continuity/System1 D7 全套不需要。 |
| **权限预检** | pre-flight 免费拒绝（未知 persona、不可解析 artifact、低 tier 重 persona、docs-only reviewer）；writes policy 缺失即拒绝；`writes: []`/allowlist/`*` vs `**`；`protectedGlobs` 语义；phase 前后 diff + rollback；scope advisory 与 enforced writes 的明确区分；worktree target 绑定 immutable identity；sandbox fail-closed、不 unsandboxed retry。 | 若没有 flow 层，可先把 writes policy 放进 dispatch 合同；scope 仍 advisory 但要显式标注“不是隔离”；sandbox 可后置，但 fail-closed 语义先定。 | Herdr worktree targeting、System 1 per-consumer consent、bubblewrap/Seatbelt 的完整实现可暂缓。 |

## 12. 不可照搬与风险

1. **体量与实验面积**：agent-hub README 单文件 1527 行，System 1 有 D6/D7/D9/D10 多个 consumer，很多能力标注 “acceptance open / uncalibrated / experimental”；`docs/PHILOSOPHY.md` 明确说文档不等于运行时已强制。照搬会得到一个我们维护不起的控制面。
2. **pi 绑定**：harness 是 `.pi/harnesses/` 的 TypeScript `ExtensionAPI` 扩展，persona 是 pi frontmatter；Claude Code 只是 coms peer。我们只能借机制，不能借代码。
3. **Herdr/Hermes 是外部产品**：pane 生命周期、presence push、snapshot/resume、Desktop panel、Telegram relay 都依赖外部服务；没有它们时很多路径降级为 files backend 或 unknown。
4. **验证是 advisory，不是强制**：hub 不会因为断言未 proven 硬拒绝 dispatch；runtime 只证明“命令执行了/状态变了”，不证明测试语义充分；specialist 自己写的测试可以通过 gate。`accepted` 不是人类 Checkpoint A。
5. **scope 不是隔离**：hub 的 `scope` 只做 violation 报告，不 block/revert；只有 flow 层有 writes enforcement 和 rollback；worktree targeting 是 routing，不是 OS sandbox。
6. **sandbox 有已知边界**：exact-file grant 阻止 temp+rename atomic save；Linux 不加 PID namespace，依赖 host ptrace；Seatbelt 仍需 Darwin runtime 验证；并发 writer 无法保护 allowlisted file。
7. **文档与源码漂移**：`fleet-coordination-patterns.md` 说 “coms has no cancel envelope”，但 `coms-envelope.ts` 已定义 `CancelEnvelope`；README 一处说 29 skills、一处说 30；多个 “removed” 配置仍出现在旧文档/版本目录。摘抄必须标注版本与文件，不能把文档当唯一事实源。
8. **并发与成本**：delegate 最多 4 child/dispatch、depth 1；并行 writable dispatch 的 scope violation 只能近似归因；racing 没有 cancel 时每个 lane 都要付费；coms peer 的 kill 只是 abandon wait，peer 会继续烧 token。
9. **本地信任模型**：coms 是同一台机器的 unix socket + 文件注册表，pool scope 是 reach boundary，但不是加密/强认证；`--explicit` 只是默认不可见，不是不可达；跨项目泄露曾发生过，后来用 `peersInScope()` 收紧。不要把 pool scope 当成多租户安全边界。
10. **品牌/资产/许可证**：代码 MIT，但 README 图片、logo、Herdr/Hermes 品牌不属于我们；派生文档要避免暗示官方关联。

## 13. 原文索引

本地 clone：`/tmp/refs-agent-fleet`（commit `8391895`，v2.0.23）。GitHub（main）：

- README — https://github.com/chankov/agent-fleet/blob/main/README.md
- 架构 — https://github.com/chankov/agent-fleet/blob/main/docs/ARCHITECTURE.md
- 哲学 — https://github.com/chankov/agent-fleet/blob/main/docs/PHILOSOPHY.md
- Workflows — https://github.com/chankov/agent-fleet/blob/main/docs/workflows.md
- 协调协议 — https://github.com/chankov/agent-fleet/blob/main/references/fleet-coordination-patterns.md
- 编排模式 — https://github.com/chankov/agent-fleet/blob/main/references/orchestration-patterns.md
- Verification Contract — https://github.com/chankov/agent-fleet/blob/main/skills/orchestration-verification/SKILL.md
- Context engineering — https://github.com/chankov/agent-fleet/blob/main/skills/context-engineering/SKILL.md
- Designing agents — https://github.com/chankov/agent-fleet/blob/main/skills/designing-agents/SKILL.md
- agent-hub — https://github.com/chankov/agent-fleet/blob/main/.pi/harnesses/agent-hub/README.md
- coms — https://github.com/chankov/agent-fleet/blob/main/.pi/harnesses/coms/README.md
- teams — https://github.com/chankov/agent-fleet/blob/main/.pi/agents/teams.yaml
- peers — https://github.com/chankov/agent-fleet/blob/main/.pi/agents/peers.yaml
- dispatch-policy — https://github.com/chankov/agent-fleet/blob/main/.pi/agents/dispatch-policy.yaml
- personas 矩阵 — https://github.com/chankov/agent-fleet/blob/main/docs/agents.md
- 项目覆盖配置 — https://github.com/chankov/agent-fleet/blob/main/docs/agent-fleet-setup.md
- worktree targeting — https://github.com/chankov/agent-fleet/blob/main/docs/session-worktree-targeting.md
- workflow authoring — https://github.com/chankov/agent-fleet/blob/main/references/workflow-authoring.md
- coms envelope 源码 — https://github.com/chankov/agent-fleet/blob/main/.pi/agent-fleet/scripts/lib/coms-envelope.ts
- review reuse 源码 — https://github.com/chankov/agent-fleet/blob/main/.pi/harnesses/agent-hub/review-reuse.ts
- review closure 源码 — https://github.com/chankov/agent-fleet/blob/main/.pi/harnesses/agent-hub/review-closure.ts
- one-use authority 源码 — https://github.com/chankov/agent-fleet/blob/main/.pi/harnesses/agent-hub/action-authorization.ts
- damage-control 规则 — https://github.com/chankov/agent-fleet/blob/main/.pi/damage-control-rules.yaml
