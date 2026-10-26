# herdctl 参考

> 源码已获取：`git clone --depth 1 https://github.com/edspencer/herdctl`，commit `0c0a1b3`（2026-10-25），MIT，CLI v1.5.28 / core v5.34.1。以下引自仓库源码与 `docs/`。

## 1. 一句话与定位

herdctl 是 Claude Code 的编排层：用 YAML 定义 agent，用 interval/cron/webhook/chat 触发无人值守运行，把作业、会话、输出落盘到 `.herdctl/`。仓库自述是 "thin orchestration layer, not a sandbox"——能力等同本机 Claude Code，Docker 隔离可选。单进程 `herdctl start` 拉起整个 fleet，另有 web dashboard 与 Discord/Slack 连接器；不做多机调度，也不替 agent 做审批。

## 2. 架构与运行方式

- 配置：根 `herdctl.yaml` 仅 `version: 1` 必填；`agents[]` 每项为 `path` + 可选 `overrides`。支持 super-fleet 组合，合并优先级从低到高：super-fleet defaults → sub-fleet defaults → agent 自身 → 子 fleet per-agent overrides → super-fleet per-fleet overrides；对象深合并，数组整体替换。
- 运行时：`packages/core` 的 FleetManager 串联 ConfigLoader、Scheduler、StateManager、Runner、JobManager。Scheduler 是 1 秒轮询循环，不是队列式调度器。
- 两套 runtime：`sdk` 走 API 计费；`cli` 复用 Claude Max 订阅，单价更低；两者都可套 Docker。
- 状态目录：`state.yaml`（每 agent/schedule 状态）、`jobs/job-<id>.yaml`、`jobs/<id>.jsonl`（逐行 append 的流式输出）、`sessions/`、`logs/`。
- 命令：`herdctl start [--web]`、`status`、`trigger <agent>`、`cancel`、`jobs`、`logs`、`sessions resume [id]`、`agent`、`config`。
- 连接器：Discord 每 agent 一个 bot token；Slack 每 workspace 一套 app token；聊天会话按 channel 隔离，默认 24h 过期。

## 3. 角色与任务模型

- agent 不是强类型“角色”，而是配置单元：`name`（正则限制，防路径穿越）、`identity{name, role, personality}`、`system_prompt`、`working_directory`、`repo`、`work_source`、`schedules`、`hooks`、`mcp_servers`、`plugins`、`setting_sources`、`session`、`docker`、权限三元组、`instances.max_concurrent`。
- 任务来源分两层：schedule 决定“何时跑”，work_source 决定“跑什么”。WorkSource 是适配器接口：`fetchAvailableWork / claimWork / completeWork / releaseWork / getWork`；目前仅 GitHub，靠 label 状态机 `ready` → `agent-working`，可配 `exclude_labels` 与失败时 `cleanup_on_failure` 回退 ready。
- 每次执行生成 job，`trigger_type` 枚举 manual/schedule/webhook/chat/discord/slack/web/fork/spawned。fork 从既有 session 分叉出新 job，`forked_from` 记录父作业；resume 则继续原 session。
- schedule 类型 interval/cron/webhook/chat，可带独立 prompt、work_source、`enabled`、`resume_session`、`outputToFile`。

## 4. 状态、调度与恢复

- Job 状态机：pending → running → completed/failed/cancelled，终态不可再更新；`exit_reason` 为 success/error/timeout/cancelled/max_turns；`id/agent/trigger_type/started_at` 创建后不可变；ID 形如 `job-2026-01-15-abc123`。记录按模型的四类 token 用量，但不持久化价格表。
- 调度判定短路顺序：`unsupported_type` → `disabled` → `already_running` → `at_capacity` → `not_due`。停机期间错过的 cron 不补跑，直接从当前时间算下一次，避免重启风暴。
- 并发：`JobQueue` 类提供优先级 1–10（数字小优先）、同优先级 FIFO 与可选 fleet 上限，但主触发路径实际用 `instances.max_concurrent`（默认 1）配合 scheduler 的在跑计数直接判定，超限抛 `ConcurrencyLimitError`。scheduled 触发满员时不排队，而是发 `schedule:skipped`；即“队列”目前更像库能力而非调度默认路径。
- 会话恢复：默认每次全新 session；schedule 可开 `resume_session`。复用前校验四项：未过期（默认 24h）、工作目录一致、runtime/docker 上下文一致、CLI transcript 仍存在；任一失败自动清指针。`--resume` 服务端报 session 不存在时，清指针并用新 session 重试一次。
- 持久化：schedule 状态写在共享 `state.yaml`，用 per-file promise 锁串行化读改写，并用 tombstone 防止已删除 schedule 被在途任务“复活”。
- Hooks：`after_run` / `on_error`，可按 `on_events: [completed|failed|timeout|cancelled]` 过滤，`when: metadata.shouldNotify` 读取 agent 写的 `metadata_file` 做条件；类型 shell/webhook/discord/slack，`continue_on_error` 默认 true。

## 5. 隔离与权限

- `permission_mode`：default / acceptEdits（默认）/ bypassPermissions / plan / delegate / dontAsk。`dontAsk` 最适合无人值守：未在 `allowed_tools` 的一律拒绝而非挂起；`delegate` 把审批交给协调 session。
- 工具三层：`tools` 是可用白名单，`allowed_tools` 是免审批，`denied_tools` 是显式拒绝（如 `Bash(rm -rf *)`）。
- Docker 配置刻意分两级：agent 级只允许“安全项”（enabled/ephemeral/memory/cpu/pids_limit/tmpfs/workspace_mode 等，schema 用 `.strict()` 拒绝未知项）；镜像、network、volumes、user、ports、env、host_config 只能在 fleet 级设置。理由是 agent 配置就在其工作目录里，若 agent 能改自己的配置，就能给自己加 `network: host` 或挂敏感卷。
- 自动加固：`--cap-drop=ALL`、`no-new-privileges`、非 root（宿主 UID:GID）、内存/CPU 限额、凭证走环境变量不挂文件。`network: bridge` 仍可访问任意外网，官方建议配 Squid 域名白名单防密钥外泄。
- 注意：herdctl 不替 agent 做权限裁决，`bypassPermissions` 只应在隔离环境使用。

## 6. 可借鉴清单（映射 7 组件）

口径：C1 角色与载体矩阵、C2 终态协议、C3 事件驱动协调、C4 审查者工具白名单与审查台账、C5 单写租约与幂等派发、C6 上下文预算与状态包交接、C7 权限预检；下列简称均按此对应。

- 角色载体矩阵：agent YAML + `identity.role` + `setting_sources/plugins/mcp_servers` 组合，说明角色可由“配置文件、技能、MCP、权限”四层共同承载；`overrides` 让同一角色在不同 fleet 中换装。
- 终态协议：JobStatus 五态 + ExitReason 五值 + 终态不可变 + “重启靠 fork 不靠改旧 job”，可作为我们作业终态的最小集。
- 事件驱动：`job:created/output/completed/failed/cancelled`、`schedule:triggered/skipped`、`config:reloaded` 等强类型事件，配合 webhook hook 外发，是 UI/连接器的唯一数据源。
- 审查白名单/台账：`tools/allowed_tools/denied_tools` 与 `permission_mode` 是声明式白名单；hooks 的 `metadata_file` + `when` 提供“agent 自报结果 → 条件触发”的轻量台账钩子。
- 租约/幂等：GitHub work_source 的 claim（打 in-progress label）是弱租约，无 TTL/owner 校验，只适合单 fleet；schedule 的 `already_running`/`at_capacity` 跳过与 tombstone 是幂等/防复活的好例子。
- 上下文交接：session 四要素校验 + resume/fork 二选一 + 每 channel 独立会话 + `herdctl sessions resume` 可在终端接管，是“交接不丢上下文”的完整链路。
- 权限预检：Docker 安全/危险两级 schema 与 `.strict()`、agent 名正则防穿越、`dontAsk` 的 fail-fast，都是“启动前拒绝不可能/不允许组合”。

## 7. 不可照搬与风险

- 无真正的任务租约：label claim 无 TTL、无 owner、无心跳，多 fleet 并发会抢同一 issue；需补 claim token 与过期时间。
- 无重试/退避策略：job 失败即终态，重跑要人工 trigger 或 fork；调度层也没有 misfire 补跑。
- 状态是单机文件（`state.yaml` + promise 锁），无跨进程/跨机一致性，只适合单主机。
- “不是沙箱”：权限最终由 Claude Code 自身的 permission 系统执行；`bypassPermissions`、chat 连接器、webhook 都扩大攻击面。
- 会话与 job 仅靠 `session_id` 关联，没有独立消息总线；高并发下文件轮询与 JSONL 解析会成为瓶颈（仓库已用 job index 缓存缓解）。
- `delegate` 模式依赖上游协调 session，herdctl 本身不实现仲裁。

## 8. 原文索引

- `README.md`；`docs/src/content/docs/architecture/{overview,scheduler,job-system,state-management,runner}.md`
- `docs/src/content/docs/concepts/{agents,schedules,jobs,sessions,work-sources,hooks,fleet-composition}.md`
- `docs/src/content/docs/configuration/{agent-config,fleet-config,permissions,docker}.md`、`security.mdx`
- `packages/core/src/config/schema.ts`、`state/schemas/job-metadata.ts`、`fleet-manager/job-queue.ts`、`scheduler/schedule-state.ts`、`fleet-manager/event-types.ts`
- `examples/multi-agent/`；`.herdctl` 状态布局
