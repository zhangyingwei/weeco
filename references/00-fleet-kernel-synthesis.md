# 舰队内核 v0：跨项目综合

口径：C1 角色与载体矩阵、C2 终态协议、C3 事件驱动协调、C4 审查者工具白名单与审查台账、C5 单写租约与幂等派发、C6 上下文预算与状态包交接、C7 权限预检。

本文把 `references/` 下 9 份项目笔记收敛成一份可执行结论：哪些机制直接借、哪些只改造、哪些不要碰。每一条都能在对应文档里找到出处。

## 结论

1. **舰队的本质是控制面，不是角色扮演。** 成熟项目都在做四件事：显式状态机、独立验证、隔离工作区、可恢复交接；“多几个 Agent 聊天”本身不产生价值。
2. **先做状态与租约（C2/C5），再做角色（C1）。** FleetQ/Luxuzhou 的对比最明显：前者靠 22 态状态机、执行表、审计哈希链支撑重试与恢复；后者全内存 Map，重启即丢，只能算可见委托原型。
3. **验证必须是“断言 + 具名证据”，不能是散文结论。** chankov 的 Verification Contract、ADLC 的确定性门禁、Fengshen 的 Scenario contract 是三种可组合的实现。
4. **多 Agent 只在独立工作面成立。** 编码是紧耦合任务；并行应给研究、独立模块、独立审查，而不是让几个 Agent 同时改同一批文件。

## 一、最值得直接照抄的 5 个机制

### 1. 派发前先写断言，完成时逐条交证据

**出处**：`01-agent-fleet.md` §5；`04-adlc-fleet.md` §5；`05-fengshen-fleet.md` §5。

chankov 的格式可直接用：

```
A<N> <tag>: <single checkable pass condition>
tag ∈ test | runtime-ui | code-grep | manual
```

- 没有断言就不派发；派发时把断言原文附给执行者，不允许转述。
- 执行者只能返回 `assertions_proven / unproven / failed`，每条必须带具名证据（测试名、命令输出、文件:行、运行观察）。
- 无证据的“已证明”自动降级为未证明；`unproven` 和 `failed` 都算没做完。
- UI/可见性断言必须用运行观察，不接受静态审查。

**最小实现**：`fleet/acceptance/<task>.md` 存断言；`fleet/reports/<task>-r<n>.json` 存逐条状态与证据。

### 2. 终态用封闭状态机 + 固定失败原因，不用自由文本

**出处**：`04-adlc-fleet.md` §4；`05-fengshen-fleet.md` §4；`06-herdctl.md` §4。

- ADLC 的失败原因是封闭枚举：`quota-paused / lock-held / wall-clock / strikes-exhausted / ticket-blocked / flail / review-unavailable / mirror-fetch-failed`；未知原因必须走人工。
- herdctl 用 `JobStatus` 五态 + `ExitReason` 五值，终态不可变，重启靠新 job 而不是改旧 job。
- `exit 0 ≠ accepted`：进程成功、测试通过、文件存在都不能单独作为验收。

**最小实现**：每个任务一个 JSON 状态机：`pending → dispatched → working → review → accepted | failed | blocked | paused`；失败必须落在枚举里，禁止写“差不多好了”。

### 3. 一个工作包一个写者：租约 + epoch + 过期回收

**出处**：`04-adlc-fleet.md` §4/§9（单写者分区、`fleet.lock` owner + stale 回收）；`05-fengshen-fleet.md` §4/§9（文件租约、generation fence）；`08-apra-fleet.md` §6（member reservation、journal replay key）；`01-agent-fleet.md` §11（writer lease 0600、非 owner release no-op）。

- 租约记录：`owner`、`epoch`、`expires_at`、`last_heartbeat`。
- 写入必须携带 epoch；旧 epoch 的写入一律拒绝（fencing）。
- 重复派发使用幂等键：同一 key + 相同输入返回已有结果；输入变化则拒绝，不重新执行。
- 换主之前旧主必须确认不可寻址；不能“先启新主、再停旧主”。

**最小实现**：`fleet/leases/<package>.json` + 一个扫描器；所有任务写入先校验 epoch。

### 4. 上下文到阈值就换人，交接单必须可恢复

**出处**：`05-fengshen-fleet.md` §6（源码 40%/50%，官网 50%/70%，以源码为准）；`01-agent-fleet.md` §7（thin dispatcher、64 KiB 自读上限、artifact 路径）；`04-adlc-fleet.md` §6（一票一新上下文）。

交接单至少包含：

```
goal:
done:            # 已完成 + 证据
not_done:        # 未完成 + 原因
decisions:       # 已做决策 + 理由
files:           # path + sha256
evidence:        # 报告/测试/命令输出路径
next:            # 下一步与停止条件
```

- 研究结果、长输出写磁盘，父级只传路径 + 摘要（digest 不超过 30 行，带 `path:line`）。
- 压缩/换载体后先读账本与断言全文，不能只看计数摘要。

### 5. 审查者工具白名单 + 精确动作审批

**出处**：`01-agent-fleet.md` §8/§9；`04-adlc-fleet.md` §7/§9；`02-fleetq.md` §7；`06-herdctl.md` §5。

- 审查者只读：禁 `Skill / Agent / SendMessage / Edit / Write`，只允许 Read 与只读 Bash。
- 证据绑定版本：`task + scopeRevision + contractHash + evidenceHash` 全部匹配才可复用审查；代码改动、范围变化、并发写入都必须重审。
- 高风险动作走一次性授权：批准绑定到具体命令/路径/输入，不能用一句“可以”当长期通行证；自由文本回答不能当授权。
- 预检失败直接拒绝（未知角色、缺 writes policy、缺沙箱、缺权限模式），不做“先跑起来再补”。

## 二、C1–C7 最佳来源对照

| 组件 | 首选来源 | 直接借 | 需改造 |
|---|---|---|---|
| C1 角色与载体 | chankov（persona × carrier × access × model；teams/peers/dispatch-policy 三文件）、ADLC（WorkerAdapter 六种 harness） | 角色四维表；同一角色多载体 | 先只支持 Sub Agent + Herdr 两种载体 |
| C2 终态协议 | chankov（runtime-result v1 四维正交）、ADLC（封闭 reason + exit code）、Fengshen（tasks.md 七态） | 四维分离；`exit 0 ≠ accepted`；终态不可复活 | 字段按我们的账本裁剪 |
| C3 事件驱动 | chankov（sentinel、push、barrier、digest）、fleet TUI（hooks → fsnotify） | 终态哨兵；阻塞等待；fan-in digest | 没有事件总线时先做 per-request await，不假装有 push |
| C4 审查白名单/台账 | chankov（review reuse 四 hash）、ADLC（gate-manifest、对抗审查）、Fengshen（alpha/beta 双席） | 只读白名单；版本绑定；改则重审 | 台账可先 JSONL + hash |
| C5 租约/幂等 | ADLC（单写者分区、lock owner、stale 回收）、apra（reservation、replay key）、chankov（one-use authority） | epoch/fencing；幂等键；过期回收 | 从单机文件租约起步 |
| C6 上下文交接 | Fengshen（阈值交接、learnings）、chankov（thin dispatcher、artifact 路径） | 交接单字段；结果落盘；最近教训注入 | 阈值按我们的窗口定 |
| C7 权限预检 | ADLC（双平面沙箱、operator-local、fail-closed）、chankov（writes policy、protectedGlobs）、FleetQ（风险分级 + ActionProposal）、apra（compose_permissions dry_run、preflight headroom） | 预检清单；工具白名单；高风险人工；dry-run | 无 bwrap 时至少做到目录/命令 allowlist + fail-closed |

## 三、这些项目也没解决的 5 个缺口

1. **派发前权限预检**：多数项目在工具调用时才检查；没有“这个角色在这个任务上是否可执行”的一次性预检。
2. **统一交接 schema**：Fengshen 有交接文档，chankov 有 artifact 路径，但没有跨项目标准；字段需要我们自己定。
3. **租约过期与 fencing 的完整实现**：ADLC 有 lock + stale 回收，多数项目没有；Agent 长任务失联后的安全接管仍是空白。
4. **“检查本身会失败”的证明**：只有 myFleet 公开复盘过“检查永远通过”的事故；需要 canary/mutation 证明检查能变红。
5. **状态真相 vs 摘要**：所有项目都依赖账本/文件，但“摘要先于事实”仍会发生；需要状态只能由产物触发的硬规则。

## 四、舰队内核 v0 最小形态

建议只用文件 + 一个扫描器，不引入框架：

```
fleet/
├── ledger.jsonl              # 只追加事件：created/dispatched/status/report/lease/handoff
├── tasks/<task>.json         # 状态机 + 断言 + 证据 + owner epoch
├── leases/<package>.json     # owner / epoch / expires_at / heartbeat
├── reports/<task>-r<n>.json  # 结构化终态：proven/unproven/failed + 证据
├── handoff/<agent>-<n>.md    # 交接单（字段见上）
├── acceptance/<task>.md      # A1..An 断言
├── checks/                   # 能证明会失败的检查（canary/mutation）
└── policy.yaml               # 角色、工具白名单、权限模式、审批规则
```

调度规则：

1. 派发前检查：角色/载体、工具白名单、权限模式、断言存在、租约空闲。
2. 只派发一次；等待终态事件或报告文件，不轮询。
3. 超时先读账本与报告；确认无终态再替换一次；绝不默认放行。
4. 报告缺证据的断言一律视为未完成。
5. 上下文到阈值写交接单并换实例；新实例先读账本再开工。
6. 一个工作包同时只有一个 owner epoch；换主先停旧主。

## 五、不要照搬

- **FleetQ 的平台化**：DAG 编辑器、670+ MCP 工具、多租户、计费，不是我们当前需要的。
- **Fengshen 的默认 skip permissions / 无 OS 沙箱**：协作层可借，安全模型必须反过来做。
- **Luxuzhou 的可见委托原型**：无验收、无持久化、无鉴权，只能看交互思路。
- **fleet TUI 的多开会话**：解决可见性，不解决交付质量。
- **herdctl 的定时/聊天优先**：适合助理与运维，不是交付系统。
- **高频发版与 15× token 并行**：成本和审查带宽会反噬。

## 六、下一步

先用一个玩具任务（1 个项目主 + 1 个舰队主 + 1 个实现 + 1 个审查）验证内核 v0 的五个指标：零轮询、报告必达、零非法派生、零重复派发、单子任务 ≤20 分钟。指标不过，不进入流程层。

项目细节与原文索引见各参考文档；本文只做取舍，不替代原文。
