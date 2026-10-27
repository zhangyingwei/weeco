# 老板行为规范：落地草案

> 状态：v0.5，2026-10-27。P0、CLI 清理、P1 核心已执行；高风险并发 review 待做。  
> 用途：这是变更规格，不是运行时实现；确认后按 P0 修改 skill/模板，再跑 dry run。  
> 边界：资料中的 Magpie 数字、X 帖子效果均为自述，未独立审计；只取模式，不照搬提示词。

## 0. 结论

1. 资料支持的是“有边界的自治 + 外部状态 + 执行/验收分离 + 可复现证据 + 失败转规则”，不是“完全无人”。
2. 当前 weeco 骨架正确：老板不实现、两级验收、一个 active 团队、worktree、ledger、阶段 skill、成员 pane。
3. 主要缺口是**合同不够具体、老板验收证据不足、失败不复盘、返工/合并没有硬边界**。
4. 先做 P0：只改 skill 和模板，不动 Go；高风险 review 是 P1 必需项，不是可选。
5. 不把 `/loop` 当完成证明，不自动 merge/release，不让老板变成实现者。

## 1. 资料结论

| 经验 | 对老板的要求 |
|---|---|
| Loop 是系统，不是提示词 | 老板写合同、证据、停止条件，不只写“去做” |
| 证据优先于自述 | 独立跑/看真实结果；`/goal` 的评估器也只读证据，不替你执行 |
| 执行者与检查者分离 | 负责人验员工，老板验团队；老板不重复细审，也不橡皮图章 |
| 状态外置 | 合同、ledger、result、验收记录、lessons 可跨会话恢复 |
| 失败转规则 | 同类问题升级为合同/检查项；严重问题立即升级 |
| 反馈先分诊 | 老板先复现、分类、定验收，不把客户原话直接丢给团队 |
| 自治有边界 | 时间盒、返工上限、停止/升级、回滚、合并/发布闸门 |
| 不 cargo cult | 不照抄 Magpie 三循环、提示词和数字 |

关键教训：

- Magpie 复盘出现过“测试所依据的代码树与推送版本不一致”。
- weeco-test T-05 的 91 个测试通过，但 4 张验收截图 MD5 相同，不能作为 4 个不同 M 项证据；负责人 rework 后重建 13 张。
- T-03 的 wait 把负责人 idle 当结束，团队 submitted 后没人监听，直到客户提醒。
- 结论：**调度不等于完成，测试通过不等于证据可信。**

## 2. 当前差距

| 维度 | 当前 | 风险 | P0 改法 |
|---|---|---|---|
| 合同 | 只有目标/交付物/验收标准/约束/工作区/团队 | 验收标准抽象，无法判断“完成” | 加非目标、DoD、证据要求、停止/升级、返工上限、基线 |
| 老板验收 | review skill 只写“看实际交付物” | 容易只看 result 或重复负责人 | 改成最小证据包清单 |
| 证据 | result 的“证据”是自由文本 | 版本不一致、截图复用、不可复现 | 加证据表：命令/环境/哈希/执行人/时间 |
| 真实路径 | 未明确要求 | 单测过但应用不可用 | 要求关键路径 + 一个失败路径 |
| 范围 | 无强制 diff 审计 | 设计/计划/合同被改、越界依赖 | 验收检查只读文件、越界文件、依赖 |
| 返工/停止 | 自由文本 + wait 超时 | 无限返工、审批疲劳 | 结构化问题 + 默认 2 轮上限 + 停止条件 |
| 复盘 | 有 ledger/review 文件 | 同类问题跨团队重复 | P0 写规则；P1 加 lessons 模板 |
| 高风险 review | 只有老板验收，无独立 review 团队 | 高风险变更缺少第二视角 | P0 定规则；P1 加 review team |
| 交付 | deliver 说老板协调 merge | 无 post-merge smoke/rollback | 老板验收后合并；合并后冒烟；失败回滚 |

## 3. P0 变更清单

| 文件 | 位置 | 改法 |
|---|---|---|
| `internal/assets/files/skills/weeco-boss/SKILL.md` | `## 硬规则` 之后 | 插入“验收前必查 / 停止与升级 / 交付 / 高风险需求”四节 |
| `internal/assets/files/skills/weeco-review/SKILL.md` | `## submitted：老板验收` 第 4 步 | 用最小证据包替换“看实际交付物”；在 `## 验收规则` 前插入结论/返工/停止模板 |
| `internal/assets/files/skills/weeco-team-lead/SKILL.md` | `## 开始` 之后 / `## 提交给老板` / `## 老板驳回` | 加“开工前读 lessons、result 写经验候选、返工记根因” |
| `internal/assets/files/templates/contract.md` | 全文 | 用新模板替换 |
| `internal/assets/files/templates/result.md` | 全文 | 用新模板替换 |
| `internal/assets/files/claude-md-block.md` | “两级验收”条目之后 | 加 4 条老板硬规则 |
| `docs/weeco-quickstart.md` | §6 老板验收 | 加“最小证据包”段 |
| `references/weeco-company-model.md` | §3 两层验收、§6 合同 | 同步两条规则和合同字段 |
| `references/boss-norms.md` | 本文 | 作为依据；不重复写进 skill |

P0 改完后执行：

```bash
make build
./bin/weeco install
weeco doctor
go test ./...
```

## 4. P0 具体改法

### 4.1 `weeco-boss/SKILL.md`

在 `## 硬规则` 列表之后插入：

```markdown
## 验收前必查

验收时加载 `weeco-review`，按“最小证据包”执行；不接受只读 `result.md`，也不重复负责人的任务级细审。

- 合同、result、成员表齐全；成员均已 accepted/closed。
- 工作区/分支/commit/tree hash；`git status --porcelain` 为空。
- 验收命令、环境、输出；命令针对当前树。
- 真实关键路径 + 一个失败路径。
- 交付物/证据清单、哈希；截图/日志唯一。
- 范围：设计/计划/合同未改，无越界文件、密钥、额外依赖。
- 项目根 `docs/weeco/lessons.md` 已读；result 有经验候选/已应用 lessons/无新经验原因；本次验收/返工已更新 lessons。
- 记录“已验证/未验证/剩余风险”。

## 停止与升级

- 同一问题 2 轮返工无新证据、合同/范围冲突、安全/数据/合并风险：停止并升级。
- 客户只决定产品方向、范围、安全/不可逆、成本；实现细节老板在合同内决定。

## 交付

- 团队不 push/merge；老板验收后统一合并/发布。
- 合并后跑冒烟；失败可回滚；未交付不 close。
- 验收/返工后记录 lessons；严重问题立即升级为合同/检查项。

## 高风险需求

以下情况强制独立 review 团队：安全/权限/密钥、数据删除/迁移、公共 API/契约、支付、生产合并/发布、跨模块核心改动、客户指定。

- review 团队独立于实施团队，只读，不改代码；老板仍做最终验收。
- P1 前按顺序 review：实施团队提交 → 验收/解散实施团队 → 招 review 团队；review 失败开修复团队。
- P1 起使用只读 review worktree，允许 review 团队与 submitted 实施团队并存，保留原团队返工。
```

### 4.2 `weeco-review/SKILL.md`

把 `## submitted：老板验收` 第 4 步“看实际交付物。”替换为：

```markdown
4. 按“最小证据包”验收，不能只看 `result.md`：

- 合同、result、成员表齐全；成员均为 accepted/closed。
- 工作区/分支/commit/tree hash；`git status --porcelain` 为空。
- 验收命令、环境、输出；命令针对当前树。
- 真实关键路径 + 一个失败路径。
- 交付物/证据清单、哈希；证据与验收项一一对应，截图/日志不重复。
- 范围：设计/计划/合同未改；无越界文件、密钥、额外依赖。
- 测试质量：bug 修复有回归测试；关键测试抽查“故意破坏时会不会失败”。
- 未验证项/剩余风险。
- 经验：项目根 `docs/weeco/lessons.md` 已读；result 有经验候选/已应用 lessons/无新经验原因；本次验收/返工已更新。
- 高风险：独立 review 团队的报告、发现项/严重级别/证据。
```

在 `## 验收规则` 之前插入：

```markdown
## 验收结论模板

accept 时把以下内容写入 `--note`（P1 改为 `acceptance.md`）：

结论：accept
合同/基线：
已验证：
未验证/剩余风险：
范围检查：
证据：

## 返工问题模板

`--issues` 至少写：

现象：
期望：
复现/证据：
影响范围：
严重级别：
验收方式：

## 停止

同一问题 2 轮无新证据、合同/范围冲突、安全/数据/合并风险：停止，不继续重试。
```

### 4.3 `contract.md`

用以下内容替换：

```markdown
# 团队合同

## 目标

（这个团队要完成什么）

## 非目标

- 

## 交付物

- 

## 完成定义（DoD）

- 

## 验收标准

- 

## 证据要求

- 验收命令：
- 真实路径：
- 工件/哈希：
- 证据清单：

## 约束

- 只在指定工作区工作
- 不修改合同之外的文件
- 不 push，不碰主分支
- 开工前读项目根 docs/weeco/lessons.md（合同给出绝对路径；若存在）
- 只读文件：

## 停止与升级

- 必须停下来问老板：
- 直接停止团队：

## 返工上限

- 默认最多 2 轮；超过后：

## 工作区

- 路径：
- 分支 / base commit：

## 团队

- 目的：design / plan / implement / review / other
- 人数：N 名员工（不含负责人）
- 角色：
- 拆分建议：

## 验收权限

- 负责人验员工：
- 老板验团队：

## 独立 review（高风险）

- 是否强制：
- review 范围：
- review 团队只读，不改代码：

## 合并 / 交付

- 团队不 push / 不 merge：
- 老板验收后合并：
- 失败回滚：

## 备注

（老板给负责人的补充说明）
```

### 4.4 `result.md`

用以下内容替换：

```markdown
# 团队结果

## 摘要

（一句话说明团队交付了什么）

## 成员与验收

（每个成员都必须有正式名字，包括负责人；不要用“员工1”这类占位名。）

| 类型 | 正式名字 | agent name | pane | 角色 | 任务 | 负责人验收 |
|---|---|---|---|---|---|---|
| 负责人 |  |  |  | 负责人 | 管理、拆分、初验 | — |
| 员工 |  |  |  |  |  |  |

## 基线与最终状态

- 分支 / commit / tree hash：
- 工作区是否干净：

## 交付物

- 

## 证据清单

| ID | 验收标准 | 证据/命令 | 环境 | 哈希 | 结果 | 执行人 | 时间 |
|---|---|---|---|---|---|---|---|
| E1 |  |  |  |  |  |  |  |

## 已验证 / 未验证

- 已验证：
- 未验证：
- 剩余风险：

## 范围检查

- 设计/计划/合同是否修改：
- 越界文件：
- 额外依赖：

## 经验候选

- 已应用 lessons：
- 新候选：
- 无新经验（原因）：

## 独立 review（高风险）

- review 团队：
- 报告路径：
- 主要发现：

## 给老板的说明

（需要老板注意或决策的事项）
```

### 4.5 `claude-md-block.md`

在“两级验收：……”条目之后加：

```markdown
- 验收看证据：合同、树/commit、命令输出、真实路径、工件哈希、范围；不能只看 result.md。
- 验收≠合并：团队不 push/merge；老板验收后合并/发布，合并后跑冒烟。
- 验收/返工后记录 lessons；严重或重复问题升级为合同/检查项。
- 高风险需求（安全/数据/公共 API/支付/生产发布/跨模块核心改动）强制独立 review 团队；老板仍做最终验收。
```

### 4.6 文档同步

`docs/weeco-quickstart.md` §6 老板验收，在“先读”清单后加：

```markdown
老板不能只看 `result.md`。至少检查：合同/result/成员表；工作区/分支/commit；`git status` 干净；验收命令/环境/输出；真实关键路径 + 一个失败路径；工件/证据哈希；范围是否越界；未验证项/剩余风险。
```

`references/weeco-company-model.md`：

- §3 两层验收加一条：老板验收看合同、树/commit、命令输出、真实路径、工件哈希、范围；不能只看 `result.md`。
- §3 加高风险规则：高风险需求强制独立 review 团队；review 团队只读，老板仍做最终验收。
- §6 合同字段增加：非目标、完成定义、证据要求、停止/升级、返工上限、工作区基线、验收权限、独立 review、合并/交付。

### 4.7 `weeco-team-lead/SKILL.md`

在 `## 开始` 之后插入：

```markdown
## 开工前：读 lessons

- 读项目根 `docs/weeco/lessons.md`（合同给出绝对路径；若存在）。
- 在团队计划和员工任务里写明适用的 lessons。
```

在 `## 提交给老板` 的 result 清单中增加：

```markdown
- 经验候选：已应用 lessons / 新候选 / 无新经验原因
```

在 `## 老板驳回` 的返工步骤中增加：

```markdown
- 记录根因和 lesson 候选；同类问题再次出现时，在 result.md 标注。
```

## 5. 老板验收最小证据包

按顺序检查，缺一项就写“未验证”：

1. 合同、result、成员表；成员均已 accepted/closed。
2. 工作区、分支、commit/tree hash；`git status --porcelain` 为空。
3. 验收命令、环境、退出码、输出。
4. 真实关键用户路径 + 一个失败路径。
5. 交付物清单、哈希；证据与验收项一一对应，截图/日志不重复。
6. 范围：设计/计划/合同未改；无越界文件、密钥、额外依赖。
7. 测试质量：bug 修复有回归测试；关键测试抽查“故意破坏时会不会失败”。
8. 未验证项、剩余风险。
9. 高风险：独立 review 团队的报告、发现项/严重级别/证据、review 负责人结论。

结论只有 `accept` 或 `rework`。`--force` 必须写明理由并记录。

## 6. 停止、返工、升级

- 默认返工上限 2 轮；每轮必须带新证据。先按 2 轮运行，后续根据数据再调。
- 停止条件：同一问题无新证据；合同/范围冲突；安全/数据/合并风险；负责人/员工 lost；时间盒到期仍无有效进展。
- 升级客户：产品方向、范围、验收直觉、安全/不可逆、成本。
- 实现细节、文件划分、测试命令由老板在合同内决定。
- 复盘路径：项目 `docs/weeco/lessons.md`；P0 老板写，P1 加模板并要求团队开工前读。
- 高风险需求：按 4.1 的“高风险需求”执行；P1 前顺序 review，P1 起使用只读 review worktree。

### 6.1 如何保证 lessons 持续维护

不能只靠“写一个文件”。要让 lessons 出现在每个团队门禁里，并验证它是否真的减少了复发。

**Owner**

- 老板：项目根 `docs/weeco/lessons.md` 的唯一写者，负责筛选、升级、退休。
- 负责人：开工前读 lessons；`result.md` 写“经验候选”。
- 员工：在报告里写问题和证据；不直接编辑项目 lessons，避免并发冲突。
- 高风险 review 团队：检查 lessons 是否被应用。

**触发**

- 每个团队 accept/rework 后。
- 每次客户反馈、事故、返工。
- 需求 close 前。
- 同类问题再次出现时。

**四个门**

1. 合同：开工前读项目根 `docs/weeco/lessons.md`（合同给出绝对路径），写明适用项。
2. result：必须有“经验候选：已应用 / 新候选 / 无新经验原因”。
3. 老板 accept：检查 lessons 已更新；`--note` 写“lessons: updated / no new”。
4. 需求 close：晋升/退休规则，并抽样审计。

**Schema**

```markdown
## L-YYYYMMDD-NN 标题
- 日期 / 需求 / 团队 / 阶段：
- 类型：incident / rework / feedback / near-miss
- 严重级别：S1 / S2 / S3
- 现象 / 根因 / 证据：
- 规则（可执行）：
- 落地位置：contract / result / acceptance.json / skill / code / test
- 状态：candidate / active / retired
- 复发次数 / 最后验证：
```

**晋升规则**

- 只有能写成“触发 / 检查 / 证据”的候选才升 active。
- S1（安全/数据/合并事故）立即升级；S2（重复质量问题）出现 2–3 次后升级；S3 只记录。
- 规则必须落到一个可执行位置；落不下去的，不是规则。
- 规则会过期；close 或里程碑时退休无效规则。

**怎么判断有效**

- 复发率：同一 lesson 再次出现在后续团队/返工中的次数。
- 采用率：合同/result 引用 active lessons 的比例。
- 逃逸率：accept 后仍发现已知 lesson 的问题。
- 抽样审计：close 时抽 2 条 active lesson，检查合同、result、验收是否真的执行。
- 不用“lessons 数量”当成功指标。

**实际例子**

| 事件 | 候选 | 落地 |
|---|---|---|
| T-05 证据 MD5 重复 | 证据必须唯一、绑定验收项 | result 证据表、验收检查 |
| T-03 wait 提前返回 | idle 不等于 submitted | wait 代码、测试、review skill |
| T-01 多行 prompt 失败 | 多行 prompt 走文件 | herdr 调用、测试 |
| T-02 角色 ID 当名字 | 成员必须有正式名字 | 合同、member 校验、skill |

**工具（P1）**

- 不新增 `weeco lesson add`；老板直接写 Markdown，避免文件/ledger 双写。
- 可选只读 `weeco lesson check` 校验 schema；不是必需。
- `lead submit` 检查 result 有“经验候选”，或 `--no-lessons`。
- `team accept` 要求 `--lessons-updated` / `--no-lessons`；有 rework 却无 lesson 时警告。
- `requirement close` 检查 lessons reviewed。
- `status` 可选显示 lessons 最后更新时间。

**反模式**

- 让每个团队直接改项目 lessons → 并发冲突；只让老板写。
- 把聊天记录/流水账当 lesson → 必须写根因和规则。
- 每个事故都加规则 → 规则膨胀；按严重级别。
- 只写“注意/加强” → 不可执行，不算规则。

### 6.2 需求甄别与调研团队

澄清保证“说清楚”，甄别保证“值得做”。两者都过，才建需求。

**三类失真**

| 类型 | 判别问题 | 典型处理 |
|---|---|---|
| 假需求 | 推进哪个上游目标？不做会影响什么？ | 指出关系，给“不做/改写/缩小”选项 |
| 伪需求 | 预期收益、成本、更直接的路径？ | 先定义可观察收益；建议最小验证或调整 |
| 错误需求 | 前提依据是什么？谁验证过？ | 先核实事实，不进入设计 |

- 结论只有五种：真需求 / 待验证 / 假需求 / 伪需求 / 错误需求。
- 没有证据只能写“待验证”；不允许凭感觉判真伪。
- 老板不替客户否决：给证据、影响、选项，客户决定继续/调整/先验证/关闭。
- 假需求或错误需求建议 `requirement close`，理由留在简报。

**老板检索边界**

- 可做：问客户、读项目现状、单点核对一个可一句话回答的外部事实（标明来源）。
- 不可做：多次检索、多来源比对、竞品/市场/政策/收益分析、成本/工时估算、方案比较。
- 单点核对后仍不确定 → 写“待验证”并招研究团队；不得下“真/伪”结论。

**方向未知**

- 客户说“不知道做什么好”时，先建需求（goal=确定方向），再招 research 团队产出 3–5 个方向及证据、成本、风险。
- 老板不自己列方向、不自己查市场；客户选定后写回简报。

**调研团队**（证据不足、S1 或影响面大时）

- 仍在 `stage=requirement`，不新增阶段或命令。
- `weeco team recruit --requirement R-... --purpose research --contract ... --size 1..3`。
- 合同：要回答的问题、可用来源、时间盒、证据要求、禁止改代码、只写报告。
- 产出：事实核查、目标对齐、效果与成本、替代方案（含“不做”）、建议与置信度、未验证项。
- 老板按最小证据包验收；结论写回需求简报，再和客户决定下一步。

**落盘**（需求简报新增）

- 上游目标：这个需求服务于哪个更大的目标。
- 依据与验证：事实、数据、来源；已验证/未验证。
- 甄别结论：结论、依据、引导方式、客户确认。

**不做**

- 不新增 `discovery` 阶段或 `brainstorm` skill；调研是需求阶段的团队任务。
- 不在甄别阶段给架构、技术选型或工时估算。

### 6.3 团队定员

N 是员工数，不含负责人；负责人负责集成、统稿和验收，不计入 N。

定员只看工作图，不按任务类型（开发/测试/调研/设计）套人数。

**工作单元**：能独立交付、独立验收、可单独负责的最小块。开发是模块/接口/迁移，测试是风险面/测试层，调研是问题集，设计是设计决策，评审是审查维度。名字不同，标准相同：能单独给证据。

**工作图**：工作单元 + 依赖边。

- 依赖：B 需要 A 的产出，画 A→B。
- 链：顺序依赖的一组单元；链再长只影响关键路径，不增加 N。
- 并行宽度：互不依赖、可同时推进的链数，是 N 的基数。
- 耦合：共享文件、接口、数据、证据、环境或必须统一决策；耦合高就合并，N 减小。
- 独立验证：验证者不能是被验证单元的作者；高风险时必须有，算一条独立链。

**公式**：`N = 合并后的并行链数 + 必需的独立验证链（如果还没有独立的人承担）`。取让关键路径最短的最小值。

| 图形状 | 对 N 的影响 |
|---|---|
| 一条长链 | N=1；加人不缩短关键路径 |
| 多条独立链 | N = 链数 |
| 共享文件/接口/证据 | 合并，N 减小 |
| 高风险、需独立验证 | +1 验证者，不能是作者 |
| 协调成本高 | 取最小值，不取最大值 |

- N > 3：合同写 ownership、接口、合并顺序；N > 5：拆需求或分阶段。
- 示例一律写 `--size N`，不写死 2/3。
- 合同写工作单元、依赖、并行链、独立验证、定员依据；不要默认 2。

**同一模型，不同工作**（示例，不是按类型定人数）：

| 工作 | 工作单元 | 独立链例子 |
|---|---|---|
| 开发 | 模块/接口/迁移 | 后端、前端；E2E 依赖两者，不是独立链 |
| 测试 | 风险面/测试层 | API、UI；E2E 依赖实现，不是独立链 |
| 调研 | 问题集 | 政策、市场；B 等 A 的结论，就是同一条链 |
| 设计 | 设计决策/文档 | 数据模型、交互；共享接口就合并 |
| 评审 | 审查维度 | 正确性、安全；作者不能是唯一评审 |

## 7. P1 必需增强

| 文件 | 改动 | 产出 |
|---|---|---|
| `internal/assets/files/templates/acceptance.json` | 增加 `require_clean_tree`、`record_git`、`required_files` | 可选机器验收配置 |
| `internal/commands/check.go` | 记录 commit/tree、clean status、命令输出、工件 SHA-256 | 更完整的 `check.json` |
| `internal/commands/team.go` | `team accept` 写 `acceptance.md`；`team rework` 记录轮次 | 独立验收文件、结构化返工 |
| `internal/model/model.go` / `team.go` / `workspace` | 高风险 review：`--review-of`，创建只读 review worktree，允许 review 团队与 submitted 实施团队并存 | 独立 review 团队 |
| `internal/commands/lesson.go`（可选） | 只读 `weeco lesson check`，校验 Markdown schema；不写文件、不写 ledger | 可选校验 |
| `internal/commands/lead.go` / `team.go` | `lead submit` 检查 result 有“经验候选”或 `--no-lessons`；`team accept` 要求 `--lessons-updated` / `--no-lessons` | lessons 门禁 |
| `internal/commands/status.go` / `team.go` | 显示返工轮次、最近验收结果、lessons 最后更新时间 | 老板一眼看到状态 |
| `internal/commands/install.go` | 安装 `lessons.md` 模板 | 项目 `docs/weeco/lessons.md` |
| 集成测试 | 覆盖新字段、返工轮次、acceptance.md、review 团队、lessons 门禁 | 防止回归 |

P1 先做 warning，不做硬失败；dry run 后对高风险/有返工的团队改成硬门禁。

## 8. 反模式

- 不复制 Magpie 三循环、提示词、commit/版本数字。
- 不用 `/loop` 证明完成；`/goal` 评估器不等于独立验证。
- 不把“测试通过”当“质量通过”；证据必须绑定当前树。
- 不让老板写代码、不让负责人自验收、不让团队自动 merge。
- 不每次事故都加规则；按严重级别和重复次数升级。
- 不把“文档存在”当“执行遵守”；看 ledger、验收记录、证据。

## 9. 这次改动怎么验收

P0 完成后，用一个小需求 dry run：

1. 合同能填满，不产生歧义。
2. result 证据表能逐项映射验收标准。
3. 老板按最小证据包能完成验收；记录实际耗时，判断是否过重。
4. 团队知道“不 push/merge、验收后老板合并”。
5. `go test ./...` 通过（模板/安装/集成测试）。
6. 如果字段明显过重，先删减，不急着上 P1。

## 10. 参考来源

- Magpie：<https://github.com/yetone/magpie>；LESSONS：<https://github.com/yetone/magpie/blob/main/LESSONS.md>
- Claude Code scheduled tasks：<https://code.claude.com/docs/en/scheduled-tasks>
- Claude Code goal：<https://code.claude.com/docs/en/goal>
- Addy Osmani：<https://addyosmani.com/blog/loop-engineering/>
- LangChain：<https://www.langchain.com/blog/the-art-of-loop-engineering>
- Anthropic harness：<https://www.anthropic.com/engineering/effective-harnesses-for-long-running-agents>
- Anthropic harness design：<https://www.anthropic.com/engineering/harness-design-long-running-apps>
- Loop Engineering 论文：<https://arxiv.org/abs/2608.21884>

## 11. 已定

- `acceptance.md`：P1 独立文件；P0 继续用 `team accept --note`。
- 返工上限：默认 2 轮，先观察运行数据，后续再调。
- `lessons.md`：放项目 `docs/weeco/lessons.md`；按 6.1 的合同/result/accept/close 四个门维护。
- 不新增 `weeco lesson add`：老板直接写 Markdown；P2 再评估命令。
- 高风险需求：强制独立 review 团队。

## 12. 实现顺序

- P0：先落地 skill/模板；高风险需求在 P1 前按顺序 review（实施提交 → 验收/解散实施 → 招 review 团队；失败开修复团队）。
- P1：实现只读 review worktree，允许 review 团队与 submitted 实施团队并存，保留原团队返工。
- P2（仅当需要跨项目查询/复发统计时）：再加 `lesson add/list` + ledger；现在不做。
- 高风险清单按 4.1 的七类执行；客户可额外指定。

## 13. 下一批工作

> 本批已执行 P0、CLI 清理、P1 核心、需求澄清协议；剩余高风险 review 与 dry run。

### P0 老板行为规范

- [x] `weeco-boss`：验收前必查、停止/升级、交付、高风险需求。
- [x] `weeco-review`：最小证据包、验收结论/返工/停止模板、lessons 检查。
- [x] `contract.md`：非目标、DoD、证据、停止/升级、返工上限、基线、验收权限、独立 review、合并/交付。
- [x] `result.md`：基线、证据表、已验证/未验证、范围、经验候选、独立 review。
- [x] `weeco-team-lead`：开工前读 lessons、result 经验候选、返工根因。
- [x] `claude-md-block.md`：4 条硬规则。
- [x] `docs/weeco-quickstart.md`、`references/weeco-company-model.md`：同步。
- [x] 提交本批文档变更（`references/boss-norms.md`、`references/README.md`）。

### 需求澄清协议

- [x] `weeco-requirements`：快速/发散两种模式、必答清单、清晰度门禁、风险分级、待设计决定。
- [x] 需求简报模板 `requirements.md`，路径 `docs/weeco/requirements/R-....md`，`--goal` 只放摘要。
- [x] `weeco-design`：设计合同引用简报，团队只读，回答“待设计决定”。
- [x] `weeco-boss`、`claude-md-block.md`：澄清阶段不做技术决定。
- [x] 不新增 CLI 命令；需求变更追加到简报“变更记录”。
- [x] 安装器安装 `requirements.md`；安装测试覆盖。

### 需求甄别

- [x] `weeco-requirements`：三类失真判别、五种结论、引导方式、客户决定。
- [x] 证据不足/S1 招 `purpose=research` 团队；仍在 requirement 阶段，不新增阶段或命令。
- [x] 需求简报增加：上游目标、依据与验证、甄别结论、引导方式。
- [x] `weeco-design` 入口检查：假/伪/错误需求不进入设计；“待验证”需客户确认或调研结论。
- [x] `weeco-boss`、`claude-md-block.md`、quickstart、company model 同步。
- [x] 老板不替客户否决；只给证据、影响、选项。

### 调研边界（本批）

- [x] 老板可单点核对一个事实；不得下真伪结论，不得多来源/竞品/市场/收益分析。
- [x] 方向未知：先建需求，再招 `purpose=research` 团队产出方向；老板不自己列方向。
- [x] `weeco-requirements`、`weeco-boss`、`claude-md-block.md`：禁止使用通用 brainstorming/设计流程。
- [x] 测试项目刷新 CLAUDE.md；旧会话需重开才加载新规则。

### 团队定员（本批）

- [x] 示例从 `--size 2/3` 改为 `--size N`，不写死人数。
- [x] 定员改为工作图模型：工作单元、依赖、并行链、耦合、独立验证；不按开发/测试/调研分别写规则。
- [x] 合同模板增加工作单元、依赖、并行链、定员依据、ownership/合并顺序。
- [x] `weeco-boss`、阶段 skill、quickstart、company model 同步。
- [x] `size` 明确为员工数，不含负责人。

### 跨项目隔离（本批）

- [x] 负责人 agent 名带项目短哈希，避免同一天同 team ID 冲突。
- [x] 成员招募前检查 Herdr 活体 agent 名，重名直接报错。
- [x] `wait/prompt/status` 优先用 pane ID；agent 名只做显示。
- [x] fakeherdr 支持 pane ID 定位；补命名/重名测试。

### P1 工具

- [x] `acceptance.md` 独立文件，`team accept` 写入。
- [x] `acceptance.json` / `check.go`：commit/tree、clean status、命令输出、工件哈希。
- [x] `team accept/rework`：结构化记录、返工轮次；`status` 显示。
- [x] lessons 门禁：`lead submit` 检查“经验候选”；`team accept` 检查 `--lessons-updated` / `--no-lessons`；`close` 检查 reviewed；当前是 warning，后续再升级硬门禁。
- [x] `lessons.md` 模板；路径项目 `docs/weeco/lessons.md`；不新增 `lesson add`，可选只读 `lesson check`。
- [x] 高风险 review 政策：强制独立 review 团队；P0 顺序模式（实施提交 → 验收/解散 → review）。
- [ ] P1 并发 review：`--review-of`、只读 review worktree、与 submitted 实施团队并存。
- [x] 集成测试覆盖新字段、返工轮次、acceptance.md、lessons 门禁；含查询命令。
- [ ] 并发 review 团队测试；顺序模式已有 skill 规则。

### CLI 清理

- [x] 删除 `member prompt`：任意 prompt 绕过状态机，改用 `member rework` 或直接看 pane。
- [x] 删除 `member list`：改用 `lead status` 和 `member status`。
- [x] 删除 `team recruit --wait`：统一用 `team wait`。
- [x] 保留 `requirement list`；等 `status` 补齐 goal/workspace 后再评估合并。
- [x] 更新 `weeco-team-lead` skill、`team recruit` 初始 prompt、README、quickstart、company model。
- [x] 补查询命令测试：`lead status`、`member status`、`team list`、`requirement list/show`。

### 验证与约束

- [x] P0 后 `make build && ./bin/weeco install && weeco doctor && go test ./...`。
- [ ] 小需求 dry run：合同可填、证据可映射、验收不过重。
- [x] 返工上限默认 2 轮，观察后再调。
- [x] 高风险 review 强制；当前 P0 顺序执行；P1 并发只读 worktree 待做。
- [x] 不自动 push/merge；老板验收后合并。
- [x] 不新增 `lesson add`；P2 再评估。

### 待决

- [ ] `.ovis/` 资料集是否入库；当前未跟踪。
- [ ] lessons 门禁先 warning 还是直接硬失败；建议先 warning。
- [x] 查询命令测试：已覆盖 `lead status`、`member status`、`team list`、`requirement list/show`；后续按需扩展。
