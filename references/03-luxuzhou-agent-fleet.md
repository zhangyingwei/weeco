# Luxuzhou/agent-fleet 参考

> 抓取：commit `7b930b2`（2026-03-26），MIT，v0.1.0，单提交原型，53 个文件。仓库：https://github.com/Luxuzhou/agent-fleet 。按 7 组件口径：①角色与载体矩阵 ②终态协议 ③事件驱动协调 ④审查者工具白名单与审查台账 ⑤单写租约与幂等派发 ⑥上下文预算与状态包交接 ⑦权限预检。

## 1 一句话与定位

一个 MCP server：Claude Code 当 orchestrator，Gemini CLI/Codex CLI 当 worker，各自在可见 pane 工作。核心是角色化 MCP 工具 + 内存队列 + 多 pane 可观察，非生产调度平台。

## 2 架构与运行方式

- `agent-fleet start` 读 `fleet.yaml`，起 Express + MCP Streamable HTTP（默认 4600），向 `CLAUDE.md` 追加"舰队模式"指令，再开 4 个 pane：`claude`、`gemini -i <workerPrompt>`、`codex <workerPrompt>`。pane 只是布局，不承担调度。
- 每个 CLI 独立 MCP session；server 按 session 身份注册 `AgentRegistry` 并挂不同工具集。Claude 经 stdio `channel-bridge` 回连 HTTP，Gemini 经 `stdio-bridge`，Codex 直接填 HTTP URL。
- 组件：`TaskQueue`（内存 Map + 1 秒定时器）、`AgentRegistry`（name↔session、60 秒断开宽限）、`ContextBuilder`（读 references 文件）、`NotificationManager`/`transport.send` 推送。
- 调度逻辑不在 server，而在 Claude 的推理里：Claude 拆任务、选 Agent、评审结果。

## 3 角色与编排

- 角色只有 `orchestrator|worker`；`workerRole`（README 示例 designer/developer；server 实际填 agent 名）仅用于展示，不参与授权。
- 角色在 MCP initialize 时由 `clientInfo.metadata.role` 或名字含 `claude` 判定。
- Orchestrator 工具：`fleet_delegate/status/result/agents/cancel`；Worker 工具：`fleet_poll/context/progress/submit`。
- 分工写死在提示：架构/规划 Claude 自己做，UI 给 gemini，实现给 codex；"不要问用户用哪个 Agent"。
- 载体：Claude/Gemini/Codex 各占一个 pane；无审查/验证/浏览器角色，无 Sub Agent/Herdr 区分。
- 无 coordinator/QA/reviewer，无动态组队、无并行 fan-out（README roadmap 才列 DAG）。

## 4 任务状态与生命周期

- `TaskStatus=pending|running|completed|failed|cancelled`。
- `FleetTask` 字段：8 位 id、agent、description、references、constraints、deliverables、upstream、progress、result{result,filesChanged}、timeout、createdAt/startedAt/updatedAt。
- `fleet_delegate` 创建 pending；worker `fleet_poll` 取到后自动 start；`fleet_submit` 置 completed（空结果拒绝）；超时（从 startedAt 起算，progress 不续期）置 failed；`fleet_cancel` 只改状态，不杀 worker 进程。
- 无依赖、优先级、重试、attempt、人工任务、子任务、取消传播。`fleet_delegate` 同步阻塞，每 1 秒查状态，自身无超时；worker 不 poll 就永久挂起。
- **终态不成立**：`complete()/fail()/cancel()` 都不校验当前状态，cancel 后仍可 complete，终态会被覆盖；没有 STATUS 行/REPORT 文件，"idle"也没有结论语义。

## 5 验证与验收

- 无 QA、无 gate、无独立验收。完成判定仅是 worker 调用 `fleet_submit` 且 result 非空；orchestrator 事后读文本自行判断。`filesChanged` 是 worker 自报，不校验文件是否真的改动，不能作为验收证据。

## 6 协调与消息

- MCP Streamable HTTP + JSON-RPC `tools/call`；任务创建时 push `notifications/fleet/task_assigned` 给 worker，完成/失败/进度 push `notifications/claude/channel` 给 orchestrator。
- `fleet_poll` 是 30 秒长轮询，监听 TaskQueue 的 `created` 事件。
- `channel-bridge` 另每 2 秒调 `fleet_status` 轮询；但 server 返回文本、bridge 按 JSON 解析，实际失败；`notifications/claude/channel` 的 params 形状两边也不一致（server 发 `{channel,message}`，bridge 发 `{content}`）。实时推送未闭环。
- 无消息持久化、ack、重放、去重。

## 7 状态与持久化

- 全部在内存 `Map`；无数据库、审计、预算账本、会话恢复。server 退出即丢任务与 Agent 状态。
- `ContextBuilder` 把 references 文件内容、constraints、upstream 打包返回，是状态包交接的雏形；缺失写 `[File not found]`，路径无 containment（`../../` 可越界）。无上下文预算、50% 换载体或交接单格式。
- 无 artifact、版本、checkpoint。

## 8 权限与安全

- `/mcp` 无认证；任何能连端口的客户端都可 initialize。角色由客户端自称，冒充 `claude` 即得 orchestrator 工具。
- 无 per-agent 工具白名单；worker 可用任意 task_id 调 `fleet_submit/progress`，没有归属校验，可篡改他人任务。
- 无沙箱、审批、预算、租户；安全边界完全依赖各 CLI 自身权限。`.claude/settings.local.json` 只对 4 个 fleet_* 工具做本地 allow，不构成强制。

## 9 失败处理

- 超时置 failed；进程退出丢状态；Agent 断开仅 60 秒后标 disconnected，不重排其 running 任务。
- 无重试、circuit breaker、deadline 传播、结构化错误码；错误以 tool text / `isError` 返回。
- `fleet_delegate` 的 timeout 取 `agentTimeouts[agent]`，但 server 从未用 fleet.yaml 填充该表，实际恒 300 秒。
- `src/workers/*` 是未被 server 引用的死代码（进程内直跑 gemini/codex）；当前实现走 CLI 自轮询路线。

## 10 可借鉴清单（7 组件）

> Legend：C1 角色与载体矩阵；C2 终态协议；C3 事件驱动协调；C4 审查者工具白名单与审查台账；C5 单写租约与幂等派发；C6 上下文预算与状态包交接；C7 权限预检。

| 组件 | Luxuzhou 对应机制 | 判定 |
|---|---|---|
| C1 角色与载体矩阵 | orchestrator/worker 两档；Claude/Gemini/Codex 各占一个 pane；`FleetAdapter` 负责 detect/configure/prompt/launch | **需改造**：只有实现载体，缺其余角色与 Sub Agent/Herdr 区分 |
| C2 终态协议 | `TaskStatus` 的 completed/failed/cancelled，但状态可被覆盖 | **不适用**：终态不可复活、STATUS 行、REPORT 文件均无 |
| C3 事件驱动协调 | `TaskQueue` EventEmitter + push；worker 30 秒长轮询；delegate 每 1 秒轮询；心跳检查被禁用 | **需改造**：可借事件源，但"禁轮询"不满足；60 秒断开宽限只标记、不重排任务 |
| C4 审查者工具白名单与审查台账 | 无 | **不适用** |
| C5 单写租约与幂等派发 | 无锁、无 epoch、无幂等键；重复 submit 直接覆盖 | **不适用** |
| C6 上下文预算与状态包交接 | `ContextBuilder` 汇总 references 文件内容 + constraints + upstream | **需改造**：是交接单雏形；无上下文预算与换载体机制 |
| C7 权限预检 | 无；只有角色工具集，角色由客户端自证 | **不适用**：必须在派发前重建预检 |

## 11 不可照搬与风险

- 角色自证 + `/mcp` 无认证，不是安全边界；worker 可越权改他人任务，ContextBuilder 可越界读。
- 无持久化，重启即丢；无独立验证，worker 自报完成，不能支撑真实验收。
- 阻塞 delegate 无超时，worker 掉线永久挂起；4-pane 也不等于并行调度。
- channel push 与 bridge 协议不一致，实时性未验证。
- 单提交原型，不能当生产依赖；可借的只是"薄 MCP 角色协议 + 可见性"。
- 假设 Claude 是唯一决策者，未覆盖 Herdr 式多独立实例与人工接管；与我们的舰队形态只部分重合。

## 12 原文索引

- `README.md`（定位、工具表、配置、roadmap）
- `src/types.ts`（TaskStatus/FleetTask/AgentInfo）
- `src/core/task-queue.ts`、`src/core/agent-registry.ts`（五态/超时、session/宽限）
- `src/tools/orchestrator.ts`、`src/tools/worker.ts`（角色工具语义）
- `src/server.ts`（Streamable HTTP、角色判定、push）
- `src/cli/start.ts`（pane 布局、worker prompt、CLAUDE.md 注入）
- `src/channel-bridge.ts`、`src/stdio-bridge.ts`（两种桥接）
- `src/workers/*`（未接入的进程内 worker）
- `test/integration/e2e.test.ts`（delegate→poll→submit 闭环）
