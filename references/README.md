# Agent 舰队参考文档

本目录收集“以 fleet（舰队）方式编排 AI Agent”的开源项目与产品资料，全部按同一套 7 组件口径整理，供重建 Vibe Coding System 的舰队内核使用。

## 7 组件口径

| 编号 | 组件 | 一句话 |
|---|---|---|
| C1 | 角色与载体矩阵 | 谁干活、跑在 Sub Agent 还是 Herdr、能读写什么 |
| C2 | 终态协议 | 怎样算完成/失败/阻塞，结论如何送达，idle 不算结论 |
| C3 | 事件驱动协调 | 完成/阻塞主动通知，禁止轮询，超时与失联判定 |
| C4 | 审查者工具白名单与审查台账 | 审查者只读、禁派生，证据绑定代码版本 |
| C5 | 单写租约与幂等派发 | 一个工作包一个写者，重复投递只生效一次 |
| C6 | 上下文预算与状态包交接 | 上下文到阈值换载体，交接单可恢复 |
| C7 | 权限预检 | 派发前确认权限模式、工具白名单、高风险人工批准 |

## 文档索引

| 文档 | 项目 | 许可证 / 版本 | 参考价值 | 核心机制 |
|---|---|---|---|---|
| [01-agent-fleet.md](01-agent-fleet.md) | [chankov/agent-fleet](https://github.com/chankov/agent-fleet) | MIT / v2.0.23 | **极高** | Verification Contract、sentinel 终态、push over polling、thin dispatcher、runtime-result 四维语义、review evidence 复用、writer lease |
| [04-adlc-fleet.md](04-adlc-fleet.md) | [voodootikigod/adlc](https://github.com/voodootikigod/adlc) | MIT | **高** | 确定性 ticket DAG、单写者分区、封闭失败原因集、双平面沙箱、集成分支、strike/墙钟 |
| [05-fengshen-fleet.md](05-fengshen-fleet.md) | [edisonshen/fleet](https://github.com/edisonshen/fleet) | MIT | **高** | 每项目 coordinator、coord/worker/reviewer/finisher、Scenario contract、alpha/beta 双席审查、自动交接、learnings |
| [02-fleetq.md](02-fleetq.md) | [escapeboy/agent-fleet-o](https://github.com/escapeboy/agent-fleet-o) | AGPLv3 | 中 | 四层验收、22 态状态机、ActionProposal 审批、审计哈希链、预算账本、风险分级 |
| [06-herdctl.md](06-herdctl.md) | [edspencer/herdctl](https://github.com/edspencer/herdctl) | MIT / CLI 1.5.28 | 中 | YAML 舰队、JobStatus/ExitReason、强类型事件、Docker 隔离、allowed/denied tools、resume/fork |
| [08-apra-fleet.md](08-apra-fleet.md) | [Apra-Labs/apra-fleet](https://github.com/Apra-Labs/apra-fleet) | Apache-2.0 / v0.4.5 | 中到高（机制密度最高，依赖重） | 角色 verdict schema、member reservation 租约、journal replay、permissions dry_run、preflight headroom |
| [07-fleet-tui.md](07-fleet-tui.md) | [brizzai/fleet](https://github.com/brizzai/fleet) | Apache-2.0 | 低到中 | hooks → fsnotify 状态总线、wait 退出码语义、fork/resume/read；无台账与租约 |
| [03-luxuzhou-agent-fleet.md](03-luxuzhou-agent-fleet.md) | [Luxuzhou/agent-fleet](https://github.com/Luxuzhou/agent-fleet) | MIT / v0.1.0 | 低 | Claude 调 Gemini/Codex 的可见委托原型；无验收、无持久化、无权限边界 |
| [09-fleet-agents.md](09-fleet-agents.md) | [fleetagents.dev](https://fleetagents.dev/) | 源码未获取 | 低 | 持久助理的委托人/能力授权思路；无公开协议 |

## 建议阅读顺序

1. **01-agent-fleet**：先看 Verification Contract 与 fleet-coordination-patterns，建立“断言 + 证据 + 事件”的框架。
2. **04-adlc-fleet**：看确定性门禁、单写者、沙箱与失败原因集，补上“控制流是代码”。
3. **05-fengshen-fleet**：看 coordinator 分工、自动交接、双席审查，补上“换人不丢状态”。
4. **02-fleetq**：按需看四层验收、审批与审计，不要整体照搬。
5. **06 / 08 / 07 / 03 / 09**：作为具体机制词典，按问题查阅。

## 使用约定

- 每份文档开头记录来源 commit / 版本与抓取日期；项目更新后可能漂移，引用前先核对。
- 文档中的“可借鉴清单”统一使用 C1–C7；若某项目没有对应机制，明确写“无/不适用”。
- 摘抄保留原文英文术语并给中文解释；不要把这些文档当成某项目的完整复刻说明。
- 跨项目结论与最小内核建议见 [00-fleet-kernel-synthesis.md](00-fleet-kernel-synthesis.md)。
