# 这些项目到底怎么实现 Agent 舰队

`references/` 里的笔记回答的是“有什么机制可以抄”；这个目录回答一个更朴素的问题：**它到底是怎么转起来的**。

每篇按同一套问题写：谁指挥谁、一次任务怎么从进入到验收、状态记在哪、Agent 之间怎么说话、怎么判断干完了、跑在什么隔离里、失败换人怎么办。不写许可证、文件数、功能清单和营销话术；文档与源码冲突时以源码为准，并在文首标注。

每篇末尾的 `## 12. 详细流程图` 用 Mermaid 画出了完整路径，按“人 / 调度 / 执行 / 状态与证据 / 验收门 / 失败与终态”分层，节点上标了具体文件、命令、状态值和失败原因。FleetQ 有 Experiment 和 Crew 两条独立入口，为了避免一张图挤成两条并排竖链，拆成 12.1 和 12.2 两张。

## 一句话看完全部

| 文档 | 它把自己当什么 | 一句话 |
|---|---|---|
| [01-agent-fleet](01-agent-fleet.md) | 薄调度员 + 契约验收 | hub 守住断言清单，把活派给上下文独立的 specialist，只收结构化 return 和证据路径 |
| [02-adlc](02-adlc.md) | 代码调度器 + 确定性流水线 | ticket DAG 按 scope 分区并行，每票过门禁和对抗审查后串行合入集成分支，最后只开一个 PR |
| [03-fengshen](03-fengshen.md) | 每个项目一个包工头 | coordinator 只拆活不写代码，worker 在自己 worktree 干，alpha/beta 双审，finisher 验证据才开 PR |
| [04-fleetq](04-fleetq.md) | 平台化任务控制台 | Experiment 22 态状态机 + 队列作业推进，审批不是挂起而是“拒绝后重放” |
| [05-herdctl](05-herdctl.md) | 单机 cron + 作业队列 | YAML 定义 agent，到点跑一个 Job，落盘状态；agent 之间基本不协作 |
| [06-apra-fleet](06-apra-fleet.md) | 多机舰队控制平面 | member 注册 + 预留台账 + workflow journal，把“谁在什么时候干什么”钉死 |
| [07-fleet-tui](07-fleet-tui.md) | 人机注意力路由 | tmux 里一堆 agent 会话，hooks 汇总状态，Space 跳到正在等你的那个 |
| [08-luxuzhou](08-luxuzhou.md) | 可见委托原型 | Claude 当 orchestrator，Gemini/Codex 在可见 pane 里干活；能看，但不能信 |

## 对照表

| 项目 | 谁决定下一步 | 状态记在哪 | 怎么算干完 | 隔离的真实程度 |
|---|---|---|---|---|
| agent-fleet | hub（LLM）按断言和 return 判断 | `.pi/agent-sessions/` 文件 + 断言台账 | 结构化 return + runtime 证据 + deliverables 读回 | 可选 OS 沙箱；scope 只是 advisory |
| ADLC | 纯代码调度器 | `.adlc/fleet-status.json` + git 事实 | 确定性门禁 + 阻断审查 + post-merge gate | 每票 worktree + 双平面 OS 沙箱 |
| Fengshen | coordinator 每 tick 重算 | `~/.fleet` 扁平文件 + `tasks.md` | Scenario contract + verification.md + finisher | worktree 真隔离；安全隔离几乎为零 |
| FleetQ | 状态机 + 队列作业 | PostgreSQL 表 + 事件 | 阶段 Verifier + QA rubric + 人工审批 | 取决于部署；默认不一定有沙箱 |
| herdctl | 时钟和并发上限 | `.herdctl/` 文件 | 进程正常退出（没有独立验收） | 默认本机权限；Docker 可选 |
| apra-fleet | workflow 脚本 | beads DAG + 预留台账 + journal | 角色输出 schema + 引擎回读 + 独立测试角色 | member 本机/SSH；靠权限白名单 |
| fleet TUI | 人 | SQLite + hook 状态文件 | 没有验收，“停了”不等于“对了” | tmux + worktree，无沙箱 |
| Luxuzhou | orchestrator（LLM） | 内存 Map | worker 自报 submit（没有验收） | 无隔离、无鉴权 |

## 按需求读

- 想看**验收怎么做硬**：01、03、02
- 想看**调度和冲突怎么消**：02、06、04
- 想看**崩溃后怎么接着跑**：06、02、03
- 想看**权限和工具白名单**：06、04、01
- 只想**让人看得见多个 agent**：07

## 三条共同结论

1. **没有一个是靠 agent 互相聊天把活干完的。** 成熟项目都是“共享账本 + 代码判断 + 独立验收”，消息只是辅助。
2. **“进程退出/任务完成”和“业务验收”必须分开。** 越成熟的项目，这道分离越硬；越像 demo 的项目，越容易把 agent 自报当成完成。
3. **隔离要分三种看**：git worktree 防冲突，OS 沙箱防破坏，权限白名单防越权。多数项目只做了第一种，却以为自己安全。

## 没有纳入的

Fleet Agents（`09-fleet-agents.md`）没有可获取的源码（原 GitHub 仓库 404），公开材料只有产品页和博客，回答不了“怎么实现”，因此不写实现分析。
