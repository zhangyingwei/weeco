# apra-fleet 参考

> 源码已获取：`git clone --depth 1 https://github.com/Apra-Labs/apra-fleet`，commit `8a46d59`（2026-10-23），Apache-2.0，v0.4.5，TypeScript monorepo（约 2400 个文件）。

## 1. 一句话与定位

apra-fleet 是“agent 舰队的控制平面”：把多台真实机器注册为 member，用 MCP 工具向任意 member 派发 prompt/命令，用持久化 workflow 编排多角色长任务，并用 supervisor 做预留、看门狗、历史与仪表盘。自述 “Kubernetes for AI agents”。与 herdctl 的单机 cron 不同，它解决多机、多 provider、长时程、可审计。开源 Apache-2.0，约 105 stars。

## 2. 架构与运行方式

- 三层：Fleet（所有 member）→ Member（一台机器 + 工作目录；本地 child_process 或远程 SSH）→ Session（member 上的对话线程，可 resume/fork）。
- MCP server 走 stdio（Claude Code 默认），同时在本机 `127.0.0.1` 暴露 Streamable HTTP `/mcp`，供 member 反向连接；JWT 的 `workspace_id` 是硬安全边界。
- `execute_prompt` 双路径：member 无活会话时起子进程/SSH；有活会话时走 MCP channel，用 `respond_to_message({reply_to, content})` 在内存中按 msgid 关联，超时同 `timeout_s`。
- Provider 适配器：Claude / Antigravity(agy) / Codex / Copilot / OpenCode / none；统一 `ProviderAdapter`（cliCommand、buildPromptCommand、parseResponse、classifyError、可选 supportsFork/forkFlag）。
- 包：`apra-fleet-workflow`（workflow 引擎）、`apra-fleet-se`（fleet-sprint、角色、supervisor）、`apra-fleet-client`、`fleet-api-contract`。
- 命令：`apra-fleet install [--llm ...]`、`start`、`workflow hello-world`、`workflow fleet-sprint --issue ... --members ...`。

## 3. 角色与任务模型

- 角色是**文件**：`packages/apra-fleet-se/apra-pm/agents/<role>.md`，YAML frontmatter 给 `name/description/tools` 白名单，正文是步骤、输入、输出契约与 Rules。角色枚举：planner、plan-reviewer、doer、reviewer、deployer、integ-test-runner、regression-test-runner、ci-watcher、harvester（另有 backlog groomer / orchestrator 伪角色）。
- 每个角色有 JSON Schema 输出契约 `agents/schemas/<role>-output.json`，`$id` 带 `@1` 主版本；`contracts.mjs` 在模块加载时校验版本，漂移即报错；缺失则回退内置字面 schema 并告警。
- 典型 verdict：reviewer 为 `APPROVED | CHANGES_NEEDED`，附 `reopenIds / replanIds / newTasks / buildFailing`；doer 用 `status` + `closedIds` + `notes`；最终 sprint verdict 是应用自有 schema。
- 任务源是 beads（`bd`）DAG：`parent-child` 表分组、`blocks` 表顺序。planner 产 DAG，doer 按分配 bead 工作，reviewer 只审被引擎确认关闭的 bead 并集，integ-test-runner 负责验证集关闭，harvester 收尾。
- 派发参数：`member_name`、`model`（premium/standard/cheap 或具体 id）、`max_turns`、`timeout_s`（无输出即杀，滚动重置）、`max_total_s`（硬上限）、`fail_on_permission_denial`、`substitutions`、`expected_context_tokens`/`context_size`、`sprint_id`（校验预留）、`resume`/`fork`。

## 4. 状态、调度与恢复

- Workflow 引擎把 workflow 当持久程序：`agent/command/sequential/pipeline/parallel/transform/phase/group/publishState/setPauseGuard`；默认 fail-fast，`continueOnError` 可选，失败时 `err.partialResults` 保留已完成项；`command({failSoft:true})` 返回 `{ok,output,error}` 而不抛。
- 结构化输出：`opts.schema` 先编译 ajv，schema 作为指令附在 prompt 后；解析先找 ```json 围栏，再括号匹配扫描；校验失败做有界修复，不是无限重试。
- 恢复：可选 JSONL journal 记录 activity:start/end/run:end；resume 时按 `(序号, 类型, member, prompt 哈希)` 计算 replay key，命中成功的活动直接返回、不重复派发；首个不匹配即切回实时执行（部分回放）。parallel 分支用 `barrier:branch:localSeq` 保证与调度无关。启动未完成的活动只发 `journal:ambiguous`，不自动判定。
- 暂停：`requestPause` 延迟生效，必须“零在途 + `setPauseGuard` 返回真”才真正 paused；guard 抛错 fail-closed；`setPreResumeHook` 在恢复前串行执行，避免重新获取资源与下一次派发竞态。
- Supervisor：预留台账（members + issueRoots 同锁步、原子 temp+rename）；6 态看门狗 `running-healthy / paused / running-unresponsive / crashed / finished / launch-failed`，PID 复用用命令行 marker 校验，只观测不自动杀。`long_running` 命令返回 `task_id`，`monitor_task` 读 status.json/PID/log。
- 可靠性契约：push 必须观测到远端 ref 移动才算 landed；孤儿 claim 在运行中回收，重读 bead、杀活进程、用 `--if-status in_progress` 状态守卫写入，每 bead 有回收上限；重试前把未推提交/改动 pin 到 `fleet-salvage/`，pin 失败则拒绝 reset（fail closed）；每个失败结果都带 sessionId 以便续跑。
- 终态错误是类型化枚举（20 余个 reason：busy/reserved/dispatch_failed/nonzero_exit/max_turns_exhausted/empty_response/orphan_recovery_timeout/workspace_not_trusted/auth/server/overloaded/insufficient_context_headroom/budget_exhausted/session_not_found/fork_unsupported/stalled/agent_never_started/max_total_time/preflight_*/usage_limit/permission_denied/secret_delivery_unavailable）。
- 并发：每 member 一把 in-flight busy 锁；拒绝前先做 pid 存活探测，确认死才自愈释放，不确定一律当 busy。

## 5. 隔离与权限

- `compose_permissions` 组合唯一 canonical allow list：角色 base profile（base-dev/base-reviewer）+ 栈探测（node/python/go/rust…）+ tags（gpu/devops）+ 项目 permissions.json 台账；再按 provider 投递到原生文件（Claude `.claude/settings.local.json`、Codex `.codex/config.toml`、Copilot `.github/copilot/settings.local.json`、OpenCode `.opencode/settings.json`、agy 项目 json）。写入后读回结构比对，全部落地才更新台账。
- 反应式授权：denial 后 `grant`，先用 `dry_run` 看是否在策略内；`healable:false` 的拒绝（auto/bypass 的 safety classifier、policy_deny）永不自动授权。永久 denylist：sudo/su/doas、bash -c/sh -c/eval、env/printenv、nc/nmap、chmod 777、`Bash(*)`、含 `| ; && 反引号 $(` 的 payload。
- 密钥：out-of-band 输入 credential store；`{{secret.NAME}}` 只在少数参数服务端解析（register_member.password、provision_llm_auth.api_key、execute_command.command 等）；`execute_prompt` 明文拒绝 secret token；输出中脱敏。
- `unattended: auto|dangerous` 控制绕过权限；`fail_on_permission_denial` 决定拒绝是失败还是 warning。另有 JWT workspace 隔离、SSH 主机密钥校验、credential 按 workspace 分域。

## 6. 可借鉴清单（映射 7 组件）

口径：C1 角色与载体矩阵、C2 终态协议、C3 事件驱动协调、C4 审查者工具白名单与审查台账、C5 单写租约与幂等派发、C6 上下文预算与状态包交接、C7 权限预检；下列简称均按此对应。

- 角色载体矩阵：md 文件（frontmatter tools）+ 输出 schema + 输入 schema（已定义、尚未接入派发）+ role-scoped memories（`+all+` / `+<role>+`）+ provider 原生 agent 文件，多层共同定义角色，比单一 prompt 稳。
- 终态协议：typed reason 枚举 + 每角色 verdict schema + `@major` 版本钉死 + “失败也带 sessionId/usage” + “deploy 失败不得 PASS” 的 honest-failure gate，可直接作为我们的终态规范。
- 事件驱动：MCP notification `notifications/claude/channel`、`report_status` 的 online/idle、fleet event、supervisor `/state` 与 watchdog 三信号，构成可观测事件面。
- 审查白名单/台账：permissions 台账（grant 持久化）、reviewer 的 `reopenIds/replanIds/newTasks`、KB 的 CONFIRMED/INFERRED 晋升-丢弃、untrusted block 包装，都是审查与知识台账的现成设计。
- 租约/幂等：member reservation（reserve/release/force_release、outcome 枚举、sprint_id owner 校验）+ supervisor ledger（两轴同锁步、原子写）+ 孤儿回收的 `--if-status in_progress` + push landed 校验 + journal replay key，是一整套“租约 + 幂等”范式。
- 上下文交接：`resume`/`fork` 的显式 id 语义（显式 id 失败即 terminal，不静默换 session）、`kb_session_prime` 只回 stale 文件、role memories、journal 回放，都是“交接可验证”的做法。
- 权限预检：`compose_permissions dry_run`、preflight（无 bd 直接失败且不重试）、dispatch 前 context headroom 检查、never-auto-grant 名单，都是启动前拒绝而非运行中补救。

## 7. 不可照搬与风险

- 体量与假设：2400+ 文件、Node 22.16+、member 需装 provider CLI/bd/dolt/git；Windows 还有 CreateProcess 环境块上限、MAX_PATH 等坑，小团队照搬成本极高。
- 预留不是 TTL 租约：没有自动过期，卡死要靠 `force_release` / “recovering a wedged member reservation”；需补心跳与过期。
- workflow 脚本是 trusted code，vetting 只是 advisory，不是安全边界，不能当沙箱。
- fleet-sprint 与 beads/Dolt/git 强耦合，换领域要重写 workflow 与角色契约。
- 大量行为依赖 provider CLI 细节（max_turns 仅 Claude、fork 仅部分 provider、Codex NDJSON、OpenCode yargs strict），升级易碎。
- 观测面很宽（dashboard/watchdog/journal），但仍是单 supervisor 进程，历史与台账落本地盘。

## 8. 原文索引

- `README.md`、`docs/architecture.md`、`docs/mcp-tools.md`、`docs/vocabulary.md`
- `docs/compose-permissions-design.md`、`docs/secret-variables.md`、`docs/sprint-reliability-gates.md`、`docs/stall-detector-resilience.md`
- `packages/apra-fleet-workflow/docs/apra-fleet-workflow-architecture.md`
- `packages/apra-fleet-se/docs/role-contracts.md`、`packages/apra-fleet-se/apra-pm/agents/*.md`、`agents/schemas/*-output.json`
- `packages/apra-fleet-se/src/supervisor/{watchdog,ledger,api}.mjs`、`src/tools/{execute-prompt,member-reservation,monitor-task}.ts`
