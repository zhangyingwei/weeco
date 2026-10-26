# FleetQ（escapeboy/agent-fleet-o）参考

> 抓取：commit `f8aab5a`（2026-10-25），AGPLv3。仓库：https://github.com/escapeboy/agent-fleet-o 。项目是 Laravel 13 / PHP 8.4 单体，约 5100 文件、45 个 bounded context。本文按 7 组件口径映射：①角色与载体矩阵 ②终态协议 ③事件驱动协调 ④审查者工具白名单与审查台账 ⑤单写租约与幂等派发 ⑥上下文预算与状态包交接 ⑦权限预检。

## 1 一句话与定位

自托管的"Agent 任务控制台"：三个等价控制面（Livewire UI、`/api/v1` REST、MCP `AgentFleetServer` 约 680 个工具）共用同一领域层；**Experiment 是核心聚合根**，由 20 余状态机驱动服务器端队列作业。它编排的是平台内 Agent/crew/workflow，不是本地 CLI 面板。

## 2 架构与运行方式

- 主链路：Signal → `IngestSignalAction`（按 `source_native_id`/`content_hash` 去重，风暴限流 60 条/分）→ `TriggerRule`（conditions、cooldown_seconds、max_concurrent）→ Project/Experiment → `ExperimentStateMachine` → `ExperimentTransitioned` 事件 → `BaseStageJob`（Horizon/Redis，6 队列）→ AI Gateway（6 层中间件）与 Tool → Artifact/Outbound。
- **Experiment**：`ExperimentStatus` 枚举实际 22 个值（README 称 20），含 `draft/scoring/planning/building/awaiting_approval/executing/awaiting_children/collecting_metrics/evaluating/iterating/completed/killed/discarded/expired` 及各阶段 `*_failed`；`ExperimentTransitionMap` 显式列出前向边；终态仅 completed/killed/discarded/expired。
- **终态不可复活**：`ExperimentTransitionMap` 没有任何终态出边，`PAUSABLE_STATES`/`KILLABLE_STATES` 也排除终态；CrewTaskStatus 的终态是 validated/qa_failed/skipped。
- **Project**：`one_shot|continuous`，cron 调度，`overlap_policy=skip|queue|allow`、`max_consecutive_failures=3`、补跑、`budget_config`、milestones；`project_dependencies` 用 alias + `latest_run|specific_run` + `extract_config` 把上游产物注入下游。
- **Workflow DAG**：27 种节点（agent/crew/human_task/conditional/switch/dynamic_fork/do_while/merge/time_gate/sub_workflow/compensation 等）；图快照写入 `experiments.constraints.workflow_graph`，每个可执行节点物化为一条 `playbook_steps`。汇合语义：普通节点要求**全部前驱完成**，`merge` 节点为 OR；`activation_mode=all|any|n_of_m`；`max_loop_iterations` 默认 10。`GraphValidator` 校验 start/end、孤儿、可达性、循环是否有出口、补偿节点同图且不可递归、数据类型兼容，以及 **T4 分支到 `bitbucket_pr_merge` 必须经过 HumanTask** 的服务端硬下限。
- **Crew**：7 种 process type（见 §3）；**MCP**：stdio 与 `POST /mcp` 双传输，compact 端点把约 680 个工具收敛为 34 个带 `action` 的元工具，`/mcp/full` 暴露全量；Sanctum 或 OAuth2/PKCE，全部按 team 隔离。

## 3 角色与编排

- `CrewMemberRole`：`coordinator / qa / worker / process_reviewer / output_reviewer / judge`。
- `CrewProcessType`：`sequential / parallel / hierarchical / self_claim / adversarial / fanout / chat_room`。`hierarchical` 由 coordinator 每轮决定下一步；`adversarial` 每 worker 领一个假设并互相质疑，最后由 judge 综合；`chat_room` 有轮次与收敛检测。
- coordinator 用一次 LLM 调用把 goal 分解为 JSON 任务数组：`title/description/assigned_to/dependencies(0 基下标)/expected_output/skip_condition`，第二遍把下标重映射为 UUID；支持 `self` 与 `union_contributions`。
- `crew_members.config` 是成员级权限与成本边界：`tool_allowlist`、`max_steps`、`max_credits`、`model_override`、`context_scope`。`ProviderResolver` 按 skill → agent → team → config 的层级解析 provider/model，支持每成员模型覆盖与本地 Codex/Claude Code。
- 载体：每个 Agent 绑定云端 provider 或本地 CLI agent，crew 成员可单独覆盖模型；但载体只是执行后端，没有 Pane、Sub Agent 或 Herdr 的生命周期与责任边界。
- Experiment 流水线阶段：scoring → planning → building → executing → collecting_metrics → evaluating；`stage_model_tiers` 把阶段映射到 cheap/expensive/standard，控制成本。

## 4 任务状态与生命周期

- **Experiment**：可暂停集合与可 kill 集合分离；暂停时记 `paused_from_status`，恢复回原状态。`ExperimentStateMachine` 强制：每阶段重试 `max_retries_per_stage=3`、迭代 `max_iterations=10`、审批拒绝循环 `max_rejection_cycles=3`。
- **Crew**：`CrewExecutionStatus=planning|executing|paused|completed|failed|terminated`；用户终止写 `terminated`，`CrewExecutionScope` 在每个节点前检查，抛 `CrewExecutionCancelledException`。
- **CrewTaskStatus**：`pending|blocked|assigned|running|completed|failed|needs_revision|validated|qa_failed|skipped`；终态为 validated/qa_failed/skipped，blocked 明确不算终态。表 `crew_task_executions` 关键字段：`depends_on uuid[]`、`attempt_number`、`max_attempts=3`、`qa_score`、`input_context/output`、`batch_id`。
- 依赖：`TaskDependencyResolver.resolveReady` 只派发依赖均 validated/skipped 的 pending 任务；`hasDeadlock` 检测依赖已 failed/qa_failed 的任务。`ClaimNextTaskAction` 用 `lockForUpdate` 原子认领，供 self_claim 模式。
- 重试：QA 拒绝 → `needs_revision`，`onTaskRejected` 在 `canRetry()` 内重置为 pending 并递增 attempt，否则 `qa_failed`；worker 异常走 `onTaskFailed`。ProjectRun 另有 `pending|running|completed|failed|skipped|cancelled`。

## 5 验证与验收

- **任务级 QA**：`ValidateTaskOutputAction` 让 QA agent 返回严格 JSON：`passed/score/feedback/issues/unverified/criterion_scores`；阈值 `quality_threshold=0.70`，rubric 按任务标题/描述关键词匹配并带权重。`executeParallel` 做 3 轮独立 commit-reveal，按多数票 + 均分聚合，记录 `agreement_ratio`；QA 输出无法解析按失败处理。
- **Crew 级质量**：`ComputeCrewQualityAction` 输出 `coherence 0.3 + quality 0.3 + efficiency 0.2 + diversity 0.2`；`resolveTrustMode` 给出 `full_consensus/majority_consensus/single_agent/llm_judge`。
- **产物级审查**：综合后若有 `output_reviewer` 成员，再做一次 review；`approved=false && score<0.7` 时可替换为 `revised_result`。
- **阶段级闸门**：`BaseStageJob` 调 `ScoringVerifier` 等验证器；失败最多在本作业内重试 2 次，并把 `_verification_errors` 注入下一轮 prompt；作业本身 `tries=3, backoff=60`。
- **人工闸门**：`ApprovalRequest` 状态 `pending|approved|rejected|expired`；HumanTask 用 `form_schema`；审批模式 `in_loop|on_loop`，后者有干预窗口；SLA 到期沿 `escalation_chain` 升级，每级 SLA 递减，最后过期。

## 6 协调与消息

- MCP JSON-RPC 工具覆盖 `experiment_*`、`crew_*`、`approval_*`、`budget_*` 等；错误统一为 gRPC 风格码（`UNAVAILABLE/PERMISSION_DENIED/RESOURCE_EXHAUSTED/DEADLINE_EXCEEDED/INVALID_ARGUMENT/FAILED_PRECONDITION/NOT_FOUND/INTERNAL`），带 `retryable`、`retry_after_ms`、`details`。每个工具可传 `deadline_ms`，`DeadlineContext` 钳制在 100ms–600s，嵌套调用继承同一 deadline。
- Crew 内消息落 `crew_agent_messages`（round、message_type），chat_room 用轮次收敛；coordinator 的决策日志可注入下一次分解 prompt。
- **Steering**：`SteerExperimentAction` 把操作者消息写入 `orchestration_config.steering_queue`（上限 10 条、2000 字），中间件在下次 LLM 调用前按序消费并移除；"queued" 与 "applied" 分开审计。
- 子实验：父进入 `awaiting_children`；全部终态后按 `failure_policy=abort_all|retry_failed|continue_on_partial` 决定聚合、重试或终止。
- **事件驱动与心跳**：状态转换发 `ExperimentTransitioned`，监听器直接派下一阶段作业，正常路径不靠轮询；实时观察走 Reverb WebSocket。`playbook_steps.last_heartbeat_at`、fleetq-bridge 每 30 秒 heartbeat、`DetectStaleBridgeConnections` 用于失联判定。但 ProjectScheduler 仍是 cron、TimeGate 靠 `PollWorkflowTimeGatesCommand`，也没有 `await/push` 式协调原语。

## 7 状态与持久化

- PostgreSQL 17 + pgvector，UUIDv7 主键、JSONB/GIN、部分索引（活跃 experiment、pending approval）。核心表：`experiments/experiment_stages/experiment_state_transitions`、`crews/crew_members/crew_executions/crew_task_executions`、`workflows/workflow_nodes/workflow_edges`、`playbook_steps`、`approval_requests`、`credit_ledgers`、`audit_entries`、`artifacts/artifact_versions`。
- **预算账本**：`credit_ledgers` 追加式记账，`type=reservation|release|deduction`，每行保存 `balance_after`；`ReserveBudgetAction` 在事务内 `lockForUpdate` 校验 experiment cap 与团队余额，`SettleBudgetAction` 按实际成本补扣或释放差额。未购买额度的自托管团队跳过强制。
- **审计**：`audit_entries` 记录 `event`、subject morph、`properties`、`ocsf_class_uid/severity_id`、`decision_context`；异步按 team 生成 SHA-256 哈希链（`prev_hash/entry_hash`，按 UUIDv7 排序，120 秒结算窗口），命令 `audit:chain`、`audit:verify-chain`。已知局限：有 DB 写权限者可重写断点之后整链，尚无外部锚定。
- 断点：`playbook_steps` 有 `checkpoint_data/last_heartbeat_at/worker_id/idempotency_key/checkpoint_version`。上下文预算由 `PipelineContextCompressor`/`CrewContextCompactor` 在前序上下文超阈值时压缩，`stage_model_tiers` 控制每阶段模型档位。Redis 承担队列、缓存、锁。

## 8 权限与安全

- **工具风险**：`ToolRiskLevel=safe|read|write|destructive`；`ToolApprovalMode=auto|ask|deny`；超时动作 `ApprovalTimeoutAction`。`ask` 不执行工具，而是创建 `target_type=agent_tool_call` 的 `ActionProposal`，模型收到"待审批"结果；批准后 `ExecuteActionProposalJob` 用一次性 bypass 精确重放该调用。**平台无法挂起一次 LLM 回合，所以是"拒绝后重放"，不是暂停/恢复。**
- **ActionProposal**：`pending|approved|rejected|expired|executed|execution_failed`，风险 `low|medium|high`。`AgentPolicyVersion` 是不可变规则快照：`denied/allowed_target_types`、`risk_ceiling`、`auto_execute{enabled,threshold=18}`、`spend_cap`、`frequency_cap`、`sensitive_paths`。`PolicyEvaluator` 只能收窄自治：命中敏感路径/超 cap/风险超 ceiling 一律转人工；`critical` 永不自动；自动执行还需 rubric 总分达阈值。总开关 `AGENT_POLICIES_ENABLED` 默认关。
- **命令五层策略**：平台 → 组织 → 工具 `allowed_commands` → 项目 → Agent，取最严；平台永久阻断 `rm -rf /`、`mkfs`、`shutdown`、管道进解释器、`$()` 等；敏感路径（`~/.ssh`、`/etc/shadow`）阻断；破坏性 SQL 不硬拒但强制人工。SSH 用 TOFU 指纹。
- **租户与沙箱**：`TeamScope` 全局作用域 + `BelongsToTeam` 自动填 team + 约 50 张表的 PostgreSQL RLS；控制台默认绕过 scope，MCP 激活时例外。凭证加密保管；本地 LLM 有 SSRF 防护；入站 webhook HMAC-SHA256；仓库提供 `docker/sandbox` 镜像与 `seccomp.json`，注释声称运行时用 `--network none/--cap-drop ALL/--read-only`，但 compose 的 `bash_sidecar` 仍接 bridge 网络，硬化需自行补。
- **审查者白名单**：`crew_members.config.tool_allowlist/max_steps/max_credits` 是成员级工具与成本白名单，QA/output_reviewer 角色据此受限；但没有"审查者禁 Skill/Agent/SendMessage"这类工具类别硬禁，也没有把审查证据绑定到代码版本。

## 9 失败处理

- **CircuitBreaker**：以 provider 为缓存键、状态落在 agent 行上，维护 `closed|open|half_open`，`failure_threshold=5`、`cooldown_seconds=60`；半开成功闭合、失败重开；`ResetCircuitBreakersCommand`。
- **作业中间件**：`CheckKillSwitch`（killed 跳过、paused `release(60)`）、`CheckBudgetAvailable`、`EnforceExecutionTtl`（默认 120 分钟）、`EnforceExecutionDepth`（50 个已完成 stage）、`EnforceConcurrencyLimit`（每用户 5，超限 `release(30)`）、`WithoutOverlapping`、租户限流。
- **卡死恢复**：`tasks:recover-stuck --timeout=10` 把超时 running/queued 任务标 failed、修复 stage、恢复 experiment；每状态有恢复超时（executing 30 分、awaiting_approval 72 小时），最多 4 次，第 3 次通知、第 4 次暂停。`StuckPattern` 包括 `state_oscillation/repeated_stage_failure/tool_call_loop/progress_stall/budget_drain`，默认动作 pause 或 notify。
- **Saga 补偿**：`RunCompensationChainAction` 仅在 `execution_failed` 触发，按完成时间倒序执行有 `compensation_node_id` 的已完成节点；补偿节点不得再有补偿；最大深度 20；用 24 小时 cache 锁做幂等。
- 预算超支由 `PauseOnBudgetExceeded` 监听状态转换自动暂停；MCP 错误码带可重试提示，便于 Agent 区分重试与快速失败。
- **单写与幂等**：`ClaimNextTaskAction` 用 `lockForUpdate` 保证一个 crew task 只被一个 Agent 认领；`WithoutOverlapping(experimentId)`、`playbook_steps.idempotency_key`、补偿链的 cache 锁防重复执行。但没有 epoch/fencing token，重复投递主要靠队列与数据库锁。

## 10 可借鉴清单（映射到 7 组件）

> Legend：C1 角色与载体矩阵；C2 终态协议；C3 事件驱动协调；C4 审查者工具白名单与审查台账；C5 单写租约与幂等派发；C6 上下文预算与状态包交接；C7 权限预检。

| 组件 | FleetQ 对应机制 | 判定 |
|---|---|---|
| C1 角色与载体矩阵 | `CrewMemberRole` 六角色 + 7 种 `CrewProcessType`；`ProviderResolver` 按 skill→agent→team→config 选 provider/model，支持本地 Codex/Claude Code | **需改造**：角色是 LLM Agent 配置，不是"项目主/舰队主/实现/审查/验证/浏览器"的载体矩阵；无 Sub Agent 与 Herdr 之分 |
| C2 终态协议 | Experiment 终态无出边、不可暂停/复活；CrewTaskStatus 三终态；`output_snapshot/final_output/artifacts/error_message` 可当 REPORT 载体 | **需改造**：借终态不可复活；没有每回合 STATUS 行与固定 REPORT 文件，idle 也不是显式结论 |
| C3 事件驱动协调 | `ExperimentTransitioned` 事件直派下一作业；Reverb 推送；`last_heartbeat_at`、bridge 30 秒心跳、`DetectStaleBridgeConnections` | **需改造**：核心链路是事件驱动，但 ProjectScheduler/TimeGate 仍轮询；无 Agent 侧 `await/push` 协调原语 |
| C4 审查者工具白名单与审查台账 | 成员级 `tool_allowlist/max_steps/max_credits`；QA/output_reviewer 角色；`audit_entries.decision_context` + ActionProposal lineage + OCSF | **需改造**：无"审查者禁 Skill/Agent/SendMessage"硬白名单，也没有强制的证据-版本绑定 |
| C5 单写租约与幂等派发 | `ClaimNextTaskAction` 的 `lockForUpdate`；`WithoutOverlapping(experimentId)`；`playbook_steps.idempotency_key`；补偿 cache 锁；ActionProposal 一次性 bypass | **需改造**：有锁与幂等键，但无 epoch/fencing；认领后没有统一的租约过期与接管协议 |
| C6 上下文预算与状态包交接 | `PipelineContextCompressor`/`CrewContextCompactor`；`stage_model_tiers`；`checkpoint_data/worker_id`；`child_outputs` 与 project dependency `extract_config` | **需改造**：有压缩与检查点，无"50% 换载体"阈值与标准交接单 |
| C7 权限预检 | 工具 `risk_level` + `approval_mode`；项目 `allowed_tool_ids/allowed_credential_ids`；Agent pivot `command_policy`；`ToolCallGovernor`；`ActionProposal` 的 risk ceiling/caps/auto threshold | **直接借**：机制可直接借，去掉五层命令策略、SSH TOFU 等重型设施即可 |

## 11 不可照搬与风险

- 体量与运维：PHP 8.4 + PG17 + Redis + Horizon + Reverb + 可选 sidecar，远超"重建 Agent 舰队"的最小需要。
- 文档与代码不一致：README 说 20 态，枚举 22 个；"670+ 工具"在服务器类中约 681 个注册项。不要把营销数字当架构依据。
- 审批不是挂起：ActionProposal 批准后是**另起作业重放**，原调用已返回"待审批"。若我们要求人批后同一会话继续，必须自建 suspend/resume。
- `TeamScope` 在 console 默认关闭、依赖 `auth()->user()`；RLS 是另一层。照搬容易产生"测试通过、队列越权"的盲区。
- 预算余额取"最后一行 + 悲观锁"，结算后仍可能为负；高并发多 experiment 共享团队余额时需重新设计。
- 哈希链只能发现"断点之后"的篡改，无法防有 DB 写权限者重构全链；外部锚定未实现。
- AGPLv3：直接复制代码到闭源产品有传染性风险；建议只借机制。
- 它是**平台编排**，不解决本地多 CLI pane、终端可见性与 Herdr 式人工监督；与我们的舰队形态是互补而非替代。

## 12 原文索引

- `/tmp/refs-fleetq/README.md`（定位、概念、架构图）、`CLAUDE.md`（目录/路由/约定）
- `app/Domain/Experiment/Enums/ExperimentStatus.php`、`States/ExperimentTransitionMap.php`、`States/ExperimentStateMachine.php`
- `app/Domain/Crew/Enums/CrewTaskStatus.php`、`CrewProcessType.php`、`CrewMemberRole.php`；`Services/CrewOrchestrator.php`、`Services/TaskDependencyResolver.php`、`Actions/ValidateTaskOutputAction.php`、`Actions/ComputeCrewQualityAction.php`
- `app/Domain/Workflow/Enums/WorkflowNodeType.php`、`Services/GraphValidator.php`、`Services/WorkflowGraphExecutor.php`、`Actions/RunCompensationChainAction.php`
- `app/Domain/Approval/Services/PolicyEvaluator.php`、`Actions/CreateActionProposalAction.php`、`app/Domain/Tool/Services/ToolApprovalGate.php`、`CommandSecurityPolicy.php`
- `app/Domain/Budget/Actions/ReserveBudgetAction.php`、`SettleBudgetAction.php`、`Listeners/PauseOnBudgetExceeded.php`
- `app/Infrastructure/AI/Services/CircuitBreaker.php`、`app/Jobs/Middleware/*`、`app/Console/Commands/RecoverStuckTasks.php`
- `app/Domain/Audit/Services/AuditChainService.php`、`app/Domain/Shared/Scopes/TeamScope.php`
- `app/Mcp/ErrorCode.php`、`ErrorClassifier.php`、`DeadlineContext.php`、`docs/mcp.md`
- `config/experiments.php`、`config/agent_policies.php`、`docs/architecture-claude-code-vps.md`、`docs/bridge-architecture.md`
