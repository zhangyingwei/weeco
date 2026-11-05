# weeco 任务路由与场景 Playbook 实施计划

**状态**：规划稿，尚未修改代码  
**目标版本**：v0.13.0  
**核心目标**：把“调研、审查、设计、编码、UI”从隐含在负责人判断里的东西，变成主 Skill 可路由、任务文件可携带、成员和验收可执行的任务画像。

---

## 1. 目标与非目标

### 1.1 目标

1. `/weeco <目标>` 仍是唯一入口，不新增多个用户需要记住的 slash 命令。
2. 主 Skill 在开工前做语义分析并完成路由：识别任务类型、阶段、写权限、UI 属性和歧义。
3. 调研、审查默认不改项目源码，设计默认只写 docs；编码才默认改项目源码。所有任务都允许在 run 目录的 scratch 写分析脚本、数据和临时产物。
4. 每个场景有独立的 playbook，只写该场景的差异；共享规则仍只写一份；`routing.md` 只做索引和语义边界，不做关键词命中。
5. 任务类型和写权限进入 `run.json`，成员提示、`status`、`send` 和验收都能看到。
6. UI 是修饰层，不是和调研/编码并列的类型；UI 任务必须经过原型确认。
7. 支持“先调研/审查/设计，再编码”的阶段切换，避免负责人一口气从调研滑到实现。
8. 安装器能安装 `routing.md` 和 playbook，升级和归档行为保持现有安全边界。

### 1.2 非目标

- 不做多个平级、可自动触发的独立 Skill；原因见第 2 节。
- 不引入独立 router subagent；路由由主 agent 基于 `routing.md` 做语义判断，原因见第 4.7 节。
- 不在 CLI 里自动做语义分类；路由由主 Skill 完成，CLI 只接收结构化结果。
- 本版不承诺“硬写拦截/沙箱”，也不把“只读”定义为禁止运行代码或写脚本。写权限约束的是**项目文件**，不是成员的分析能力；硬拦截另做设计，原因见第 12 节。
- 不新增“测试类型”“文档类型”“运维类型”等场景；先用四个主类型加 UI 修饰层，后续按同一机制扩展。
- 不改变现有成员招募、等级、模型候选、worktree、关闭和权限模型的语义。

---

## 2. 架构决策

### 2.1 一个入口 Skill + 语义路由 + 按需 playbook

采用：

```text
~/.claude/skills/weeco/
  SKILL.md                 # 入口、路由、共享规则
  routing.md               # playbook 索引、语义边界、RouteDecision 规则
  member-prompt.md         # 成员通用提示
  playbooks/
    research.md
    review.md
    design.md
    coding.md
    ui.md
```

不采用：

```text
~/.claude/skills/weeco/
~/.claude/skills/weeco-research/
~/.claude/skills/weeco-review/
~/.claude/skills/weeco-coding/
```

原因：

- Claude Code 的 Skill 不是函数调用，主 Skill 无法可靠地“调用”另一个 Skill；
- 子 Skill 若设 `disable-model-invocation: true`，主 Skill 路由不过去；
- 子 Skill 若不设，又可能在普通对话中被自动触发，破坏“只有显式 `/weeco` 才启用”的约定；
- playbook 文件可以由主 Skill 用 Read 确定性地加载，且只在需要时消耗上下文；
- 安装器只需多安装几个文件，不需要管理多个 Skill 目录、manifest 和升级归档。

### 2.2 类型与等级正交

- `senior/mid/junior/temp` 回答“任务要多强、多贵”；
- `research/review/design/coding` 回答“这次要产出什么、能不能写”；
- `role` 继续是自由文本，用来写领域，例如“实时渲染工程师”“产品设计师”；
- 同一名 `senior` 可以做调研、审查、设计或编码，等级不决定任务类型。

### 2.3 UI 是修饰层

UI 不是一个独立主类型。一个任务可以是：

- `design + ui`：先出静态原型；
- `coding + ui`：按已确认原型实现；
- `review + ui`：审查现有界面，不写代码；
- `research + ui`：调研界面方案，不写生产 UI。

主 Skill 检测到 UI 时，除主 playbook 外还必须加载 `playbooks/ui.md`。

---

## 3. 任务模型

### 3.1 主类型

| 类型 | 语义边界 | 默认项目写权限 | 交付物 | 默认终点 |
|---|---|---|---|---|
| `research` | 要了解、比较、评估、解释或判断；需要证据和来源 | `none`（可加 `analysis`） | 报告：问题、方法、来源、证据、反证、未知项、建议 | 报告；不自动进入实现 |
| `review` | 要检查现有产物，找问题、风险、缺陷或反例 | `none`（可加 `analysis`） | 问题清单：严重级别、位置、证据、影响、建议 | 审查结论；不自动修复 |
| `design` | 要方案、架构、规划、取舍或设计文档/原型 | `docs` | 设计文档：目标、非目标、约束、选项、取舍、失败路径、回滚、待定项 | 方案；不自动写生产代码 |
| `coding` | 要改变项目文件并验证行为 | `scoped` | 代码、测试、构建/运行结果 | 实现并验证 |

### 3.2 写权限

| 写权限 | 含义 | 允许写入 | 禁止写入 |
|---|---|---|---|
| `none` | 不改项目已有文件；允许分析脚本 | run 目录的 scratch（脚本、数据、临时报告） | 项目已有文件、源码、文档、配置、依赖、测试、git 状态 |
| `analysis` | 只写分析目录 | 任务指定的 `analysis/`、`research/`、`notebooks/` 等分析路径；scratch | 生产源码、测试、依赖、构建配置、其他项目文件 |
| `docs` | 只写文档/原型 | 任务指定的 `docs/`、`docs/prototypes/` 路径；scratch | 生产源码、测试、依赖、构建配置、其他项目文件 |
| `scoped` | 限定范围实现 | 任务文件列出的源码、测试、配置路径；scratch | 未列出的文件；其他人的文件；全局配置 |

**scratch** 固定放在 run 目录下，例如：

```text
~/.weeco/runs/<run>/members/<member>/scratch/
```

- 所有类型都可写 scratch；
- scratch 不是交付物，不进入项目 diff；
- 如果 scratch 里的脚本/数据要成为交付物，负责人必须明确提升为 `analysis` 或 `docs`，或切换到 `coding`；
- 成员报告中可以引用 scratch 路径、脚本和运行结果作为证据。

> 本文中的“只读”是“不改项目文件”的简写，不是“不能写脚本、不能跑分析”。调研和审查经常需要脚本，默认放 scratch 即可。

默认映射：

```text
research -> none   （需要项目内分析脚本时显式加 analysis）
review   -> none   （需要复现脚本时显式加 analysis）
design   -> docs
coding   -> scoped
```

允许的显式覆盖：

- `research/review` 可以是 `none`、`docs` 或 `analysis`，用于报告、项目内分析脚本或数据；不能改为 `scoped`。
- `design` 可以是 `none`、`docs` 或 `analysis`；不能改为 `scoped`。
- `coding` 只允许 `scoped`；如果只需要审查，应使用 `review`。

### 3.3 阶段

阶段不是新的数据类型，而是任务画像的时间序列：

```text
research -> coding
review   -> coding
design   -> coding
design(ui) -> coding(ui)
```

规则：

- 第一阶段默认不改项目源码；分析脚本放 scratch，必要时用 `analysis` 写项目内分析目录；
- 进入下一阶段前，负责人必须在交付说明中明确宣布阶段切换；
- 如果用户明确说“调研完直接做”，可以连续执行，但第一阶段仍不改项目源码，且报告要说明“已进入实现阶段”；
- 如果没有明确授权，调研/审查/设计以第一阶段交付物为终点，不自动进入编码；
- 阶段切换通过 `send` 更新成员的 `task_type` 和 `write_scope`，并开始新 turn。

---

## 4. 语义路由

主 Skill 不把关键词命中作为路由条件。它先读取 `routing.md`（playbook 索引和路由手册），再对用户目标、约束、材料、否定、阶段和当前项目状态做语义分析，输出结构化的 `RouteDecision`，最后只加载选中的 playbook。

### 4.1 决策方式

- **主 agent 自己路由**，不新起独立的 router subagent；
- 路由依据是意图、交付物、约束、风险和阶段，不是词表；
- `routing.md` 只提供 playbook 索引、边界、正反例和歧义规则；
- 关键词只作为示例、召回提示和兜底信号，不作为条件；
- 只有选中的 playbook 才读全文；UI 任务额外读 `ui.md`；
- 路由结果必须可解释、可复核，不能只给一个类型标签。

### 4.2 `routing.md` 的内容

`routing.md` 是路由手册，不是关键词表。每个 playbook 至少写：

- 目的；
- 交付物；
- 默认写权限；
- 什么时候用；
- 什么时候不用；
- 与相邻 playbook 的边界；
- 2–3 个正例；
- 1–2 个反例；
- 常见误判。

索引只写契约，不复制 playbook 的完整流程。流程留在对应 playbook。

### 4.3 `RouteDecision`

主 Skill 在开工说明前生成并在内部使用以下结构：

```text
primary_intent: research | review | design | coding
phases: [research, coding, ...]
current_phase: research | review | design | coding
ui: true | false
write_scope: none | analysis | docs | scoped
deliverable: 报告 | 审查结论 | 方案/原型 | 代码+测试
confidence: high | medium | low
evidence: 用户原话、约束、材料或项目状态
ambiguities: 还不确定、需要用户决定的事项
action: proceed | ask
rationale: 为什么选这个路由，以及为什么不选相邻路由
```

规则：

- `phases` 支持多个阶段；`current_phase` 是现在要执行的阶段；
- `ui=true` 时必须加载 `ui.md`，但不改变 `primary_intent`；
- `write_scope` 按主类型和用户约束确定，不按单个词确定；
- `confidence` 低或 `ambiguities` 会影响行动时，`action=ask`；
- `RouteDecision` 由主 Skill 生成和自检，不写入 `run.json`；持久化的是最终的 `task_type` 和 `write_scope`；
- 路由结果写入开工说明，让用户能尽早发现偏差。

### 4.4 路由步骤

1. **先看交付物**：用户最终要的是报告、审查结论、方案/原型，还是代码和测试。
2. **再读约束和否定**：`不要改代码`、`只交报告`、`先别实现`、`不用做界面` 会覆盖表面的动作词。
3. **识别阶段**：`先调研再实现`、`设计完落地`、`审查后修复` 拆成多个阶段，不只选一个类型。
4. **语义识别 UI**：判断是否涉及用户可见界面、布局、视觉、交互、原型、响应式、深色模式等；原型默认按 UI 处理。
5. **确定写权限**：按交付物和阶段确定 `none/analysis/docs/scoped`，不是按关键词决定。
6. **给出置信度**：明确单一意图为 high；存在多个合理解释为 medium；依赖关键假设或会改变行动为 low。
7. **处理歧义**：中低置信、混合意图、否定约束冲突或高风险写入时，只问一个会改变行动的问题，或采用安全默认并明确说明。
8. **加载 playbook**：只读选中的 `playbooks/<type>.md`；UI 额外读 `ui.md`。
9. **自检**：用 playbook 的“什么时候不用”反查；不匹配就重新路由或询问。
10. **输出任务画像**：把类型、阶段、写权限、UI 和交付物写入开工说明。

### 4.5 语义判断示例

这些是判断示例，不是关键词规则：

| 用户输入 | 路由结果 | 原因 |
|---|---|---|
| 不要改代码，先看看这个页面为什么卡，给我分析 | `research` + `ui`，`none`，当前 research | “不要改代码”覆盖了“页面/分析”的动作歧义；要的是分析，不是实现 |
| 先调研这个方案，再直接实现 | `research -> coding`，当前 research，`none`；下一阶段 `scoped` | 有明确阶段；第一阶段不改项目源码 |
| 帮我设计一下接口和数据模型 | `design`，`docs`，无 UI | “接口设计”是设计，不是用户界面 |
| 做个原型看看交互 | `design` + `ui`，`docs`，需要原型确认 | 原型默认属于 UI；交付物是原型，不是生产代码 |
| 审查一下这版实现，发现问题直接修 | `review -> coding`，当前 review，`none`；下一阶段 `scoped` | 先独立审查，再进入修复；不能把审查和修复混成一轮 |
| 优化一下 | `confidence=low`，先读现状或提问 | “优化”可能指性能、体验、架构或流程，不能直接判 coding |
| 这个方案值不值得做 | `research`，`none` | 要判断和证据，不是实现 |
| 接口页面加载慢，看看有没有问题 | `review` + `ui`，`none` | 要定位问题，不是直接改代码 |

### 4.6 安全默认

- 无法判断，但用户要信息或判断 → `research`；
- 无法判断，但用户要修改文件 → `coding`；
- 无法判断，但用户要规划或方案 → `design`；
- 无法判断，但用户要检查现有产物 → `review`；
- 涉及生产代码写入、迁移、删除、发布等高风险动作，且意图不明确 → 先问再做；
- 不根据单个名词推断类型；不以“页面”“接口”“脚本”“优化”等单字面直接定路由。

### 4.7 不采用独立 router subagent

本版不新增独立的 router subagent，原因：

- 主 agent 已经拥有用户目标、对话上下文、仓库状态和已有约束，转交给 subagent 会丢上下文；
- 多一次调用会增加延迟和成本；
- 路由 subagent 仍会误判，主 agent 不应盲信；
- 当前只有五个 playbook，独立路由的收益不足以抵消复杂度；
- weeco 的 Agent/Task 工具约定是真实成员不能用它替代，额外引入 router 会增加边界混乱。

如果未来 playbook 数量显著增加，先用一组真实输入对比“主 agent 语义路由”和“独立 router”的准确率、延迟、成本，再决定是否引入。


## 5. 主 `SKILL.md` 调整

### 5.1 结构调整

建议顺序：

1. 角色与入口
2. `## 任务路由`（新增）
   - 先读取 `routing.md`；
   - 做语义分析，生成 `RouteDecision`（类型、阶段、UI、写权限、置信度、歧义）；
   - 按 UI、阶段、主类型、歧义顺序路由；
   - 输出任务画像并加载对应 playbook。
3. `## 开工前说明`
4. `## 工作区选择`
5. `## UI 与原型确认`（保留短入口，细节移入 `playbooks/ui.md`；原型默认属于 UI）
6. `## 明确工作并组织成员`
7. `## 协作命令`
8. `## 检查、返工和交付`

### 5.2 共享规则保留在 `SKILL.md`

- 负责人是唯一有招募、验收、关闭权的人；
- 成员是真实 Herdr Agent，不能用 Agent/Task 工具替代；
- 等级模型候选、画师、共享文件归属；
- worktree 选择、创建、验证和清理；
- 权限、guard、报告 turn、`send`、`accept`、`close`；
- 验收的通用原则。

### 5.3 必须修改的现有表述

1. 把“用户只要计划、方案或草稿时”扩展为：

   > 用户只要计划、方案、草稿、调研、研究、评估、审查、比较、选型或预研时，以该交付物为终点，不执行其中的实际操作。

2. “写实现或执行其他实际工作之前”改为：

   > 在调用 `weeco start`、创建成员、写实现、修改项目文件或执行其他实际工作之前……

3. 工作区选择增加前置条件：

   - `research/review` 默认不创建 worktree；分析脚本放 run scratch，项目内分析脚本用 `analysis`；
   - `design` 只在需要写设计文档/原型时使用项目目录；代码探针放 scratch；
   - `coding` 才按规模选择当前 checkout 或 worktree；使用 worktree 时通过 `--dir` 和 `--team-pane` 让成员进入 worktree workspace；
   - 不修改项目文件的任务不要为了“中大型”而创建 worktree。

4. 验收增加：

   - `write_scope=none`：检查 `git status --porcelain`，项目不应有改动；scratch 和 run 目录报告不计入项目 diff；
   - `write_scope=analysis`：只允许任务指定的分析目录（如 `analysis/`、`research/`、`notebooks/`）；源码、测试、依赖、配置不能变；
   - `write_scope=docs`：只允许任务指定的文档/原型路径；
   - `write_scope=scoped`：检查 diff 是否越出任务文件列出的路径；
   - 出现越界时停止并询问，不把越界改动当成交付物。

5. UI 入口只保留：

   > 涉及 UI 时先加载 `playbooks/ui.md`，完成原型确认后再进入实现。原型、线框图、设计稿、mockup、wireframe、prototype 默认按 UI 处理。

---

## 6. Playbook 规格

所有 playbook 开头统一写：

> 共享规则见 `SKILL.md`；本文件只写本场景的差异。

### 6.1 `research.md`

必须包含：

- 适用场景和默认 `write_scope=none`（脚本默认放 scratch；需要项目内分析脚本时显式用 `analysis`）；
- 交付物结构：
  - 问题与范围
  - 方法与来源（文件路径/URL、版本、日期）
  - 结论与证据
  - 反证、冲突和未知项
  - 建议（仅在用户要求时）
  - 后续验证项
- 允许动作：
  - 读文件、搜索、`git status`、只读命令；
  - 在 run scratch 写分析脚本、Notebook、数据抽取脚本和临时数据；
  - 如用户要求脚本成为项目交付物，显式使用 `write_scope=analysis`，只写任务指定的 `analysis/`、`research/` 或 `notebooks/`；
  - 脚本运行结果、命令和输出路径写入报告；
- 禁止动作：
  - 修改项目生产源码、测试、依赖、配置、构建脚本；
  - 未经授权创建项目脚手架或把探针代码留在项目源码目录；
  - `git add/commit`、切换分支、创建 worktree；
  - 把“调研”当成“顺手实现”；
- 报告位置：
  - 优先 run 目录或任务指定位置；
  - 如用户要求项目内报告，使用 `docs/` 并显式设为 `write_scope=docs`；
- 验收：
  - 每条结论可回查来源；
  - 脚本/数据路径和运行结果可复现；
  - 未验证项明确；
  - 项目 diff 只出现在允许的 `analysis/`/`docs/` 路径，其他项目文件无改动；
- 反模式：
  - 把 scratch 中的探针代码直接当成产品实现；
  - 用代码实现代替调研结论；
  - 未授权修改生产测试或源码来“证明”结论。

### 6.2 `review.md`

必须包含：

- 适用场景和默认 `write_scope=none`（复现脚本默认放 scratch；需要项目内复现脚本时显式用 `analysis`）；
- 审查对象：代码、方案、设计、文档、运行结果；
- 问题分级：
  - `BLOCKING`：会导致目标失败、数据错误、不可逆风险；
  - `SHOULD FIX`：明显缺陷或维护风险；
  - `NICE TO HAVE`：优化项；
  - `OPEN QUESTIONS`：需要用户/负责人决定；
- 每项包含：位置、现象、证据、影响、建议、验证方式；
- 独立性：
  - 从原始要求和实际产物重新核验；
  - 不替作者签字，不把作者结论当证据；
  - 尝试反例；
- 允许动作：
  - 在 scratch 写复现脚本、测试驱动、日志分析脚本；
  - 必要时用 `analysis` 写项目内复现脚本或数据；
- 禁止：修改生产源码/测试/配置、直接修复、只读报告里夹带实现；
- 验收：
  - 问题可复现或可回查；
  - 复现脚本和命令可复现；
  - 项目 diff 只出现在允许的 `analysis/`/`docs/` 路径；
  - 结论区分“已确认”和“推测”。

### 6.3 `design.md`

必须包含：

- 适用场景和默认 `write_scope=docs`；
- 交付物结构：
  - 目标与非目标
  - 约束和假设
  - 方案选项与取舍
  - 推荐方案
  - 失败模式、降级、回滚
  - 验收与验证计划
  - 待定问题和需要用户决定的事项
- 写权限：
  - 只写 `docs/` 或任务指定的设计/原型路径；
  - 不写生产代码、测试、依赖、构建配置；
  - 需要在 scratch 写代码探针验证方案时允许，但探针不是交付物；
  - 如需把探针提升为生产实现，必须单独进入 `coding` 阶段或获得明确授权；
- UI：
  - 涉及 UI 时加载 `ui.md`，先确认是否需要原型；
- 验收：
  - 方案完整不等于已执行；
  - 每个关键约束有对应验证方式；
  - 未验证依赖和风险明确；
  - 无生产代码改动。

### 6.4 `coding.md`

必须包含：

- 适用场景和默认 `write_scope=scoped`；
- 写权限：
  - 只改任务文件列出的文件/目录；
  - 不修改其他人的文件、全局配置、无关模块；
  - 共享文件指定唯一写者；
- 交付物：
  - 代码、测试、构建/运行结果；
  - 实际执行的命令和结果；
  - 未验证项和剩余问题；
- 验证：
  - 针对当前版本运行测试、构建、启动或 HTTP 联调；
  - 检查实际 diff；
  - 不把“代码写完”当成“已验证”；
- UI：
  - 涉及 UI 时加载 `ui.md`；
  - 实现前引用已确认的原型路径；
- 返工：
  - 按现象、期望、依据、修复边界处理；
  - 不顺手扩大范围。

### 6.5 `ui.md`（修饰层）

必须包含：

- 触发条件：用户可见界面、布局、视觉、交互、响应式、深色模式、原型、页面、组件；
- 不是独立主类型，必须与 `design/coding/review/research` 组合；
- 原型确认流程：
  - 在写生产 UI 前问用户是否需要原型；
  - 需要：原型作为独立交付物，放 `docs/prototypes/<feature>/`，只写静态原型、关键状态、交互和 `notes.md`；不写生产 UI、不接真实后端；
  - 复杂流程用 `senior`，简单组件可用 `mid`；
  - 负责人检查后由用户确认，`accept` 记录确认范围、未定项和实现约束；
  - 实现任务引用已确认原型；
- 不需要原型时：
  - 记录用户决定；
  - 任务仍写清布局、状态、交互、窄屏、深色模式等验收标准；
- 审查/调研 UI 时：
  - 不自动创建原型；
  - 按现有界面和证据审查；
- 验收：
  - 关键页面、状态、交互、窄屏、深色模式与原型/要求一致；
  - 原型与实现不一致时记录偏差或返工。

---

## 7. CLI 与数据结构

### 7.1 `store.Member` 新字段

```go
type Member struct {
    // ...existing fields...
    TaskType   string `json:"task_type,omitempty"`
    WriteScope string `json:"write_scope,omitempty"`
}
```

旧 run 没有这两个字段时视为空值，读取和展示不能报错。

### 7.2 任务画像校验

新增 `internal/commands/taskprofile.go`：

```go
const (
    TaskResearch = "research"
    TaskReview   = "review"
    TaskDesign   = "design"
    TaskCoding   = "coding"
)

const (
    WriteNone     = "none"
    WriteAnalysis = "analysis"
    WriteDocs     = "docs"
    WriteScoped   = "scoped"
)

type taskProfile struct {
    Type  string
    Scope string
}

func resolveTaskProfile(taskType, writeScope string, artist bool) (taskProfile, error)
```

规则：

- `taskType` 为空时默认 `coding`，并给出 warning：`--task-type not set; defaulting to coding (write scope scoped)`；
- 非法类型/权限报错；
- 默认权限按第 3.2 节映射；
- `research/review` 允许 `none/docs/analysis`，不能 `scoped`；
- `design` 允许 `none/docs/analysis`，不能 `scoped`；
- `coding` 只能 `scoped`；
- 画师默认 `coding/scoped`；画师不接受 `research/review/design` 作为主类型，避免把出图任务伪装成无写权限任务；
- `none` 只约束项目文件，不阻止 run 目录 scratch 的脚本和数据。

### 7.3 `start` 参数

```text
weeco start --name ALIAS --role TEXT --task FILE \
  [--task-type research|review|design|coding] \
  [--write-scope none|analysis|docs|scoped] \
  [--level LEVEL] [--model MODEL] [--agent claude|codex|pi] \
  [--dir PATH] [--team-pane PANE] [--replace] [--run ID]
```

行为：

- 主 Skill 路由后必须始终传 `--task-type`；CLI 参数是运行时的权威值，任务文件里的类型/权限是给成员看的副本，两者不一致时负责人必须先修正；
- 新成员写入 `TaskType`、`WriteScope`，并创建成员 scratch 目录；
- 已有同名成员比较任务、角色、等级、模型、类型和权限；任一不同且没有 `--replace`，报错并提示用 `send` 改阶段或用 `--replace` 换人；
- 旧成员的 `TaskType`/`WriteScope` 为空时，比较按 `coding/scoped` 兼容处理，不因升级导致已有 run 无法继续；
- `start` 输出增加 `type` 和 `write`；
- 缺少 `--task-type` 时打印 warning，不静默。

### 7.4 `send` 参数

```text
weeco send ALIAS --file FILE \
  [--task-type research|review|design|coding] \
  [--write-scope none|analysis|docs|scoped]
```

行为：

- 未提供时保留当前画像；
- 提供类型但未提供权限时，使用该类型的默认权限；
- 提供权限但未提供类型时，按当前类型校验；旧成员空类型按 `coding` 校验；
- 更新成员画像后再注入新 turn；注入失败时恢复旧画像，不能留下“状态已切换但消息未送达”的假记录；
- 用于阶段切换，例如 `research -> coding`；
- 新 turn 的任务文件/指令必须写明当前阶段和交付边界。

### 7.5 `status`

文本输出增加：

```text
type=research write=none
```

JSON 通过嵌入的 `Member` 自动输出 `task_type`、`write_scope`。

旧 run 空值显示 `-`，不报错。

### 7.6 成员提示

`memberPrompt` 增加：

```text
Task type: research
Write scope: none
Write policy: do not modify existing project files; use scratch for analysis scripts.
Scratch workspace: /Users/.../.weeco/runs/<run>/members/<name>/scratch
```

按权限生成：

- `none`：不改项目已有文件；可以在 scratch 写脚本、数据、Notebook 和临时报告；只读命令和临时探针允许；报告写 run 目录/任务指定位置。
- `analysis`：只写任务指定的 `analysis/`、`research/`、`notebooks/` 等分析路径和 scratch；不写生产源码、测试、依赖、配置。
- `docs`：只写任务指定的文档/原型路径和 scratch；不写源码、测试、依赖、配置。
- `scoped`：只写任务文件列出的路径和 scratch；不碰其他文件。

成员提示仍保留“最大权限”说明，但明确权限是能力上限，不是任务授权；任务边界优先。scratch 的脚本和数据不自动成为交付物；要交付时由负责人提升为 `analysis`/`docs` 或切换到 `coding`。

---

## 8. 安装器与资源

### 8.1 `internal/assets/assets.go`

```go
//go:embed \
//  files/skills/weeco/SKILL.md \
//  files/skills/weeco/routing.md \
//  files/skills/weeco/member-prompt.md \
//  files/skills/weeco/playbooks/*.md
var FS embed.FS
```

新增资源列表：

```go
var Routing = "files/skills/weeco/routing.md"

var Playbooks = []string{
    "files/skills/weeco/playbooks/research.md",
    "files/skills/weeco/playbooks/review.md",
    "files/skills/weeco/playbooks/design.md",
    "files/skills/weeco/playbooks/coding.md",
    "files/skills/weeco/playbooks/ui.md",
}
```

### 8.2 `skillFiles()`

返回顺序固定：

```text
SKILL.md
routing.md
member-prompt.md
playbooks/research.md
playbooks/review.md
playbooks/design.md
playbooks/coding.md
playbooks/ui.md
```

`installFile` 已支持创建父目录，`manifest.SkillFiles` 也已支持嵌套路径；主要改测试和期望文件列表。

### 8.3 安装测试

更新/新增：

- `TestInstallCopiesOnlyWeecoSkillAndMemberPrompt` 改名为 `TestInstallCopiesWeecoSkillRoutingMemberPromptAndPlaybooks`；
- 顶层目录检查从只有 `SKILL.md`/`member-prompt.md` 改为包含 `routing.md` 和 `playbooks/`；
- `TestEmbeddedFilesContainOnlyWeecoSkillAndMemberPrompt` 改为完整资源集合；
- `TestInstallUpdatesOwnedSkillFiles` 覆盖 routing/playbook；
- 新增：routing/playbook 被用户修改后重装能更新；
- 新增：旧 manifest 中的 routing/playbook 被移除后能归档；
- 新增：symlink 目录下的 routing/playbook 不被写入；
- 保持现有 manifest、归档、dry-run、权限测试不变。

---

## 9. 文档

### 9.1 `README.md`

- 版本改为 v0.13.0；
- 增加“任务路由”小节：一个入口、语义路由手册、playbook、四个主类型 + UI 修饰；
- 增加 `start` 的 `--task-type` / `--write-scope`；
- 说明旧用法不传类型时默认 `coding` 并 warning；
- 说明原型、线框图、设计稿、mockup、wireframe、prototype 默认按 UI 处理；
- 说明调研/审查不改项目源码，但允许在 scratch 写分析脚本；需要项目内脚本时用 `analysis`；
- 说明验收检查 `git status` 和 diff 边界。

### 9.2 `docs/weeco-quickstart.md`

新增“任务类型与路由”章节，放在工作区选择之前：

```bash
weeco start --name researcher --role "调研" \
  --task-type research --write-scope none \
  --task "$PROJECT/.weeco/tasks/research-task.md"

# 需要把分析脚本/数据放进项目分析目录时：
weeco start --name analyst --role "数据分析" \
  --task-type research --write-scope analysis \
  --task "$PROJECT/.weeco/tasks/analysis-task.md"

weeco start --name reviewer --role "独立审查" \
  --task-type review --write-scope none \
  --task "$PROJECT/.weeco/tasks/review-task.md"

weeco start --name designer --role "方案设计" \
  --task-type design --write-scope docs \
  --task "$PROJECT/.weeco/tasks/design-task.md"

weeco start --name coder --role "实现" \
  --task-type coding --write-scope scoped \
  --task "$PROJECT/.weeco/tasks/coding-task.md"
```

补充：

- 调研/审查/设计以报告/方案为终点；
- `none` 不是“不能写代码”，而是“不改项目已有文件”；分析脚本写 scratch；需要项目内脚本时显式用 `analysis`；
- 原型、原型图、线框图、设计稿、mockup、wireframe、prototype 默认按 UI 交付物处理；
- 阶段切换用 `send --task-type coding --write-scope scoped`；
- UI 任务加载 `ui.md` 并先确认原型；
- 验收检查 `git status --porcelain`，只允许任务画像内的路径有改动。

### 9.3 `docs/weeco-team-model.md`

新增“任务类型与写权限”章节，说明：

- 类型与等级正交；
- 四个主类型和权限表；
- routing 语义索引和 RouteDecision 示例；
- UI 是修饰层，原型默认属于 UI；
- 阶段切换；
- 任务文件模板增加：

```text
任务类型：
写权限：
是否 UI：
原型决定：
原型路径：
交付物：
```

任务文件里的类型/权限必须与 `weeco start --task-type/--write-scope` 一致；CLI 是权威值。文档示例要同时写出两处，避免负责人只改一处。

更新“任务拆分”和“验收”章节，明确调研/审查不改项目源码，但允许 scratch/analysis 脚本；验收按写权限检查 diff 边界。

### 9.4 `docs/weeco-team-model-implementation.md`

更新：

- 实现状态和本期范围；
- 任务画像字段、CLI 参数、成员提示；
- `routing.md` 和 playbook 安装；
- 新增测试、语义路由评测和真实冒烟；
- 实施顺序和风险。

### 9.5 新增 `docs/weeco-task-routing.md`（可选但建议）

集中写：

- 语义路由规则和 `RouteDecision`；
- playbook 契约和“何时不用”；
- 类型/权限矩阵；
- 阶段切换；
- 常见误判、否定和反模式；
- 语义评测用例的维护方式。

README、quickstart、team model 链接到它，避免三处重复维护。

---

## 10. 测试计划

### 10.1 单元测试

`internal/commands/taskprofile_test.go`：

- 默认映射；
- 非法类型/权限；
- `research + analysis` 允许；
- `research + scoped` 报错；
- `design + scoped` 报错；
- `coding + none/analysis/docs` 报错；
- 画师默认和非法类型；
- 空类型默认 `coding`；
- `none` 仍允许 scratch，不把“不能写项目源码”误判为“不能运行脚本”。

`internal/commands/runtime_test.go` / 新增 `task_type_test.go`：

- `start --task-type research` 写入成员字段，`status` 显示 `type=research write=none`；
- 成员提示包含 `Task type: research`、scratch 路径和“不改项目源码”策略；
- `start --task-type research --write-scope analysis` 允许写分析目录，提示包含 analysis 策略；
- `start --task-type coding` 允许写，提示包含 scoped 策略；
- `start --task-type review` / `design` 的默认权限正确；
- `start` 创建成员 scratch 目录，提示中的路径可写；
- 同名成员类型不同且无 `--replace` 报错；
- `send --task-type coding` 能把 research 成员切到 coding，新 turn 提示更新；
- 旧 run 没有字段时 `status` 正常显示 `-`；
- 缺少 `--task-type` 时输出 warning 并默认 coding。

`internal/commands/install_test.go`：

- 安装 playbook；
- 重装更新 playbook；
- 归档旧 playbook；
- symlink 安全；
- embedded 资源集合完整。

Playbook 内容测试（可放 `internal/assets` 或 `install_test.go`）：

- 每个 playbook 存在且包含必需标题/要点；
- `routing.md` 包含 research/review/design/coding 四个索引项、语义边界、正反例和 RouteDecision 规则；
- `routing.md` 明确“关键词只作示例，不作路由条件”，并包含原型默认按 UI 处理的边界说明；
- 新增 `evals/routing/cases.md` 或 `docs/weeco-routing-cases.md`，用真实输入记录期望的 `RouteDecision`；至少覆盖否定、混合阶段、UI 原型、接口设计、模糊“优化”五类；
- `research.md` 包含“不改项目源码”、scratch、来源、`git status`；
- `review.md` 包含分级、证据、反例、复现脚本；
- `design.md` 包含目标/非目标、取舍、回滚、scratch 探针；
- `coding.md` 包含写权限、测试、diff；
- `ui.md` 包含原型确认、`docs/prototypes`、用户确认。

### 10.2 回归命令

```bash
go test ./...
go test -race ./...
go vet ./...
make build
make install
weeco install
```

### 10.3 真实 Herdr 验证

1. **调研任务**：`/weeco 调研 X，只交报告，不改项目源码` → 成员类型 research，项目无越界 diff，报告有来源；允许在 scratch 写分析脚本并记录路径。
2. **审查任务**：`/weeco 审查 X` → 不改项目源码，问题分级，项目无 diff；允许 scratch 复现脚本。
3. **分析脚本任务**：`/weeco 调研 X，并把分析脚本放到 analysis/` → `write_scope=analysis`，项目 diff 只出现在 `analysis/`，源码/测试/配置不变。
4. **设计任务**：`/weeco 设计 X 方案` → 只写 `docs/`，无生产代码；代码探针只在 scratch。
5. **编码任务**：`/weeco 实现 X` → 有实际 diff、测试/构建通过。
6. **混合任务**：`/weeco 先调研 X，再实现` → 先 research，报告后进入 coding；`status` 显示类型变化。
7. **UI 任务**：`/weeco 实现这个页面` → 先问原型；需要时只出静态原型，用户确认后再 coding。
8. **否定约束**：`/weeco 不要改代码，只看看这个页面为什么卡` → research + ui，`none`，不进入 coding。
9. **歧义输入**：`/weeco 优化一下` → 先读现状或问一个会改变行动的问题，不直接判 coding。
10. **边界输入**：`/weeco 设计一下接口和数据模型` → design，不叠加 UI。
11. **安装验证**：全局 `~/.claude/skills/weeco/routing.md` 和 `~/.claude/skills/weeco/playbooks/` 五个文件存在，`/weeco` 能加载。

---

## 11. 实施顺序

| 阶段 | 内容 | 完成标准 |
|---|---|---|
| 1 | 任务画像模型与校验 | `taskprofile` 单测全过；默认/非法组合正确 |
| 2 | `Member` 字段、`start`、`send`、`status`、成员提示 | 类型/权限可写入、切换、展示；旧 run 兼容 |
| 3 | 资源嵌入与安装器 | `routing.md` 和五个 playbook 安装、更新、归档、symlink 测试通过 |
| 4 | 主 `SKILL.md` 语义路由和共享规则 | 路由、开工说明、工作区、验收规则自洽 |
| 5 | 四个主 playbook + `ui.md` | 内容覆盖第 6 节；asset 测试通过 |
| 6 | 文档同步 | README、quickstart、team model、implementation、routing 文档一致 |
| 7 | 全量回归与真实 Herdr 冒烟 | 第 10 节全部通过 |
| 8 | 版本与安装 | v0.13.0；`make install` 后全局 Skill 含 routing 和 playbooks |

---

## 12. 安全边界与风险

### 12.1 本版不承诺硬写拦截

本版通过以下方式降低“调研变编码”：

- 路由和 playbook 明确调研/审查不改项目源码，但允许 scratch/analysis 脚本；
- 任务画像进入成员提示；
- 验收检查 `git status` 和 diff 范围；
- 调研/审查任务不创建 worktree。

但不承诺硬拦截项目写入：

- Claude/PI 的 guard 目前只覆盖 Bash 的破坏性命令，不拦截 Write/Edit 文件工具；
- Codex 的 read-only 沙箱会阻断 `weeco report` 向控制目录写入；
- 不同 agent 的写入工具名和沙箱能力不一致。

因此，若需要“绝对不允许修改项目文件”，应另做写入 guard/沙箱设计，并分别验证 Claude/PI/Codex 的上报链路。本版先把策略和验收做对，不把提示当沙箱。

### 12.2 主要风险

1. **路由误判**：模型可能把“调研并实现”只判成 coding。缓解：先识别阶段；歧义时只问一个会改变行动的问题；第一阶段默认不改项目源码。
2. **CLI 默认 coding 的 footgun**：不传 `--task-type` 仍可能编码。缓解：warning；Skill 必须始终传；后续可改为必填。
3. **Playbook 加载失败**：Skill 目录不可用或安装不完整。缓解：固定 fallback 路径；读不到时说明并退回“不改项目源码 + scratch”默认。
4. **共享规则漂移**：playbook 重复写等级/权限/关闭规则。缓解：playbook 只写差异；主 Skill 保留共享规则；测试检查关键短语。
5. **UI 修饰层被跳过**：coding/design playbook 忘记加载 `ui.md`。缓解：主路由统一检测 UI；coding/design playbook 开头写“UI 任务必须加载 ui.md”。
6. **语义路由过度自信**：模型可能把混合意图压成单一类型，或把“优化”直接判成 coding。缓解：输出 `RouteDecision` 和置信度；中低置信或高风险写入时问一个会改变行动的问题；用真实场景评测回归。
7. **设计类任务的写权限**：`docs` 可能被扩大成源码。缓解：任务文件列允许路径；验收检查 diff；成员提示明确禁止源码。
8. **旧 run/旧任务文件**：没有 `task_type`。缓解：空值显示 `-`；成员提示要求按任务文本和只读约束处理；新任务必须写画像。
9. **阶段切换没有报告**：`send` 直接进入 coding。缓解：Skill 要求在阶段切换前说明；CLI 只提供机制，不替负责人做阶段判断。
10. **安装器嵌套路径回归**：现有测试假设扁平文件。缓解：更新测试并增加嵌套/symlink/归档用例。
11. **画师任务类型**：画师写文件，不适合 `none`/`analysis`。缓解：画师默认 `coding/scoped`；不接受无写权限类型。
12. **把“只读”误解成“不能写脚本”**：调研/审查需要数据分析脚本。缓解：所有类型都有 scratch；需要项目内脚本时显式 `analysis`；文档和成员提示写清区别。

---

## 13. 验收标准

全部满足才算完成：

- [ ] `/weeco` 仍是唯一入口，没有新增用户必须记住的场景 slash 命令。
- [ ] 主 Skill 能输出任务类型、阶段、写权限、UI 属性和交付物。
- [ ] 主 Skill 先做语义分析并输出结构化 `RouteDecision`，不把关键词命中作为路由条件。
- [ ] `routing.md` 是 playbook 索引和语义边界手册，包含四个主类型 + UI 修饰的正反例、歧义规则和“何时不用”。
- [ ] 语义评测覆盖否定、混合阶段、UI 原型、接口设计、模糊“优化”等场景，结果符合期望。
- [ ] 调研/审查默认不改项目源码，但允许在 scratch 写分析脚本；需要项目内脚本时使用 `analysis`；设计默认只写 docs；编码默认只在 scoped 范围写。
- [ ] 任务文件、成员提示、`status`、`send`、验收都能看到任务画像和 scratch 路径。
- [ ] “调研并实现”能按阶段执行，调研阶段不写项目源码。
- [ ] UI 任务先确认原型；原型只写 `docs/prototypes/`；用户确认后才实现。
- [ ] playbook 和 `routing.md` 随 `weeco install` 安装，升级/归档/symlink 行为安全。
- [ ] 旧 run 和旧 CLI 用法不崩溃；缺少类型时有 warning。
- [ ] 单元测试、race、vet、build 全过。
- [ ] 真实 Herdr 验证覆盖调研、审查、分析脚本、设计、编码、混合、UI 七类场景。
- [ ] 原型、设计稿、线框图、mockup、wireframe、prototype 等词能触发 UI 原型确认。
- [ ] README、quickstart、team model、implementation、routing 文档一致。

---

## 14. 完成定义

完成后，用户输入：

```text
/weeco 调研一下这个项目的认证实现，给我报告，不要改代码
```

应看到：

```text
任务类型：research
阶段：调研报告；不进入实现
写权限：none（项目不改；脚本放 scratch）
UI：否
```

成员只做项目文件不改的取证，报告有来源和未知项；可以在 scratch 写分析脚本，项目没有越界代码改动。

用户输入：

```text
/weeco 先调研认证实现，再补一个缺失的测试
```

应看到：

```text
任务类型：research -> coding
当前阶段：research（不改项目源码；可在 scratch/analysis 写脚本）
后续阶段：coding（在报告后或明确授权后进入）
```

调研阶段不改项目源码、不写生产代码，但可在 scratch/analysis 写分析脚本；进入 coding 后通过 `send --task-type coding --write-scope scoped` 切换，并检查实际 diff 和测试结果。
