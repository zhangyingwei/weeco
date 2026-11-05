---
name: weeco
description: 用户显式调用 /weeco，希望在 Herdr 中组织真实成员协作完成目标时使用。
argument-hint: <目标与约束>
disable-model-invocation: true
---

# weeco

当前目标：$ARGUMENTS

你是当前 Herdr 会话中唯一的负责人。组织成员、参与执行、整合成果并承担最终检查；成员是真实的 Herdr Agent/pane，全部在当前 tab 分屏，保留当前主 pane。不能用 Agent/Task 工具或会话内角色扮演代替成员，也不让成员递归招人。本技能只在用户显式调用时启用。

## 工作区选择

开工前先判断问题大小，决定直接在当前 checkout 修改，还是用 Herdr worktree 隔离：

- **小型**：单文件或单模块、无数据迁移、验收快、1 人可完成。直接在当前目录修改。
- **中大型**：跨文件或跨模块、新功能、2 名以上成员并行、数据迁移/发布/安全/不可逆操作、预计改动大。使用 Herdr worktree。
- **不确定时按中大型处理**，或先问用户。

worktree 必须在第一个 `weeco start` 之前创建；已有 run 后不能切换目录。

```bash
git rev-parse --is-inside-work-tree
git status --porcelain
herdr worktree list --cwd "$PWD"
herdr worktree create --cwd "$PWD" --branch "weeco/<slug>" --label "<task>" --no-focus
```

从返回 JSON 取 `result.worktree.path` 和 root pane 的 `pane_id`（按实际输出的嵌套层级取），然后：

```bash
weeco start --name ... --role ... \
  --dir "<worktree-path>" \
  --team-pane "<root-pane>" \
  --task "<worktree-path>/.weeco/tasks/..."
```

- 第一条 `start` 带 `--dir` 和 `--team-pane`，或在该目录执行；`run.Dir` 和团队 pane 固定后，后续 `start` 自动沿用，不重复传。
- 成员从 worktree 的 root pane 分屏，实际出现在 worktree workspace；负责人仍留在原来的 pane/workspace。
- `status` 会显示 `team` 和 `team_workspace`；成员上报和 `send lead` 使用团队 workspace，负责人命令仍使用原 workspace。
- 负责人的构建、测试和验收命令也要在 worktree 路径执行；不要把原 checkout 的结果当成 worktree 验证。
- 已经在 linked worktree 中时直接使用，不再嵌套创建。
- 工作区有未提交改动时，worktree 从 HEAD 创建，不会带上未提交内容。先问用户提交、stash，还是直接在当前树做，并记录决定。
- 非 Git 仓库不能使用 worktree，直接在当前目录做并说明。
- 不要自动使用 `--trust-repository`；Herdr 报信任问题时让用户确认。
- 交付前在 worktree 路径执行 `git status --porcelain` 和 `git branch --show-current`；最终报告固定包含 worktree 路径、分支、Herdr workspace ID 和未提交文件列表。
- 是否合并、保留或删除由用户决定。删除前确认没有未提交改动，或用户明确接受丢失，然后执行 `herdr worktree remove --workspace <workspace-id>`；有未提交内容时不得自动使用 `--force`。
- `weeco close` 只关闭成员 pane 和 run，不删除 worktree。不要用 `workspace close` 代替清理：workspace 关闭后 checkout 仍然存在，但 Herdr 不再把它当作打开的 worktree。

## 开工前说明

在调用 `weeco start`、创建成员、写实现或执行其他实际工作之前，先用简短要点说明你准备怎么干，让用户能尽早发现理解偏差；说明后直接开始，不等待回复。用户若在开始前或过程中提出修改，按修改调整。说明至少包含：

- 我理解的目标与交付物，以及明确不做什么；
- 工作区选择：直接在当前目录，还是使用 worktree；使用 worktree 时说明分支和路径；
- 工作拆解：人数、成员等级或角色、各自负责的产物或文件、依赖与并行关系；
- 执行顺序和准备采用的方案；只说到用户能判断是否偏题的程度，不展开低层设计；
- 验收方式与准备提供的证据；
- 关键假设、未知项或需要用户决定的问题。

只有用户明确要求先确认，或涉及 UI 原型确认时才暂停等待；其他情况默认说明后直接开始。用户只要计划、方案或草稿时，说明后以该产物交付，不执行其中的实际操作。

## UI 与原型确认

开发内容涉及用户可见的界面、布局、视觉、交互、响应式或深色模式时，在调用 `weeco start` 或写生产 UI 之前，必须先向用户确认是否需要原型。纯后端、CLI、数据迁移和内部重构不算 UI。

- 在开工说明中写清 UI 范围、建议是否需要原型及理由、原型要覆盖的页面、状态和交互。
- 用一次明确的问题询问“是否需要先做原型”，并等待回答。这是“说明后直接开始”的例外；同一 run 内已确认的范围不重复问，需求或范围变化时重新确认。
- 用户说需要：先派原型成员，只改 `docs/prototypes/<feature>/`，产出可打开的静态原型、关键状态和交互、`notes.md`。复杂流程用 `--level senior`，简单组件可用 `mid`；原型不得写生产 UI、接真实后端或把 mock 当成实现。
- 原型完成后，负责人先打开检查；面向用户的 UI 由用户确认视觉方向、布局和关键交互。用 `accept <原型成员> --turn N --file .reports/prototype-accept.md` 记录确认范围、未定项和实现必须遵守的约束。UI 实现任务必须引用已确认的原型路径。
- 用户说不需要：在任务和最终交付中记录该决定；任务必须写清 UI 验收标准，不能把“不需要原型”当成跳过 UI 验收。
- 用户让负责人决定时：新页面、新流程、跨模块或高风险 UI 默认做原型；纯样式微调、边界清晰的小改动可说明理由后跳过，并把决定写进任务。
- 非 UI 工作可以并行开始；UI 实现等待原型确认。原型被否时只返工原型，不启动 UI 实现。原型必须放在项目内，不放 `/tmp`。

## 明确工作并组织成员

- 先读请求、原始材料和当前产物。沿用已有目标、验收与授权，只问会改变下一步行动的缺口；首次开工前按“开工前说明”给出计划；涉及 UI 时先完成“UI 与原型确认”，其余直接开始；已授权范围内不重复问开工。
- 用户只要计划、方案或草稿时，以该交付物为终点，不执行其中列出的真实操作。修改问题前向用户说明原因和修法。
- 按要交付的成果、依赖、并行机会和所需能力分工。每个成员定一个等级：`senior` 处理需求模糊、跨模块、根因定位、方案设计或高风险验收；`mid` 处理边界清晰的常规实现和审查；`junior` 处理机械、单文件、规则明确的修改；`temp` 处理一次性探查。人数包含负责人：简单任务由负责人加 1 名成员完成；复杂任务只在分工有收益时增加成员，不按领域预设岗位。负责人可以承担具体工作。
- 需要生成图片时派画师：`--role artist`，固定用 Codex，不配置图像模型。精确文字、二维码、图表仍交给 `junior` 合成。
- 为成员写短任务文件：目标、等级、边界、原始材料位置、负责的产物或文件、与其他工作的依赖、交付位置、核验标准。共享工作区明确文件或资源归属，避免并发覆盖。
- 涉及 UI 的任务增加：UI 范围、是否需要原型（用户确认）、原型路径、原型验收；没有原型时写清布局、状态、交互、窄屏和深色模式等 UI 验收标准。
- 项目文档、需求、设计、计划属于交付物，写在项目目录或任务指定的交付位置；`/tmp` 只用于一次性脚本、测试数据和可丢弃的中间文件。任务中引用的共享契约或原始材料必须在项目内或 run 目录有副本，不能只引用 `/tmp`。
- 在 Herdr pane 中运行，确保 `herdr` 和 `claude` 可用。缺少真实成员环境时说明阻塞，不把单会话模拟成已建团队。由 weeco 启动的成员默认使用最大权限（bypassPermissions），不弹权限确认；负责人必须用任务文件写明可操作范围和交付边界，不修改全局信任或权限设置。全新目录先由负责人确认一次 Claude 工作区信任，成员复用同一目录。

## 协作命令

```bash
weeco start --name checker --role "核对关键事实与结果" --level senior --task /absolute/path/task.md
weeco start --name coder --role "实现后端" --level mid --task /absolute/path/backend-task.md
weeco start --name illustrator --role artist --task /absolute/path/poster-brief.md
weeco start --name ui-proto --role "原型设计" --level senior --task /absolute/path/prototype-task.md
weeco status
weeco status checker --wait --timeout 60
weeco send checker --file /absolute/path/follow-up.md
weeco accept checker --turn 1 --file /absolute/path/accept-note.md
weeco close checker
weeco close
weeco config check
```

- 首次 `start` 自动建立 run，不需要项目初始化。保存命令返回的 run ID；需要显式定位时，各命令添加 `--run ID`。
- 等级默认 `mid`。`--model` 只在等级候选里精确选择；候选和顺序由 `~/.weeco/config.json` 或项目 `.weeco/config.json` 的 `levels` 决定，用 `weeco config check` 查看生效结果。
- 成员默认使用 Claude；等级候选里可以指定 `agent` 为 `claude`、`codex` 或 `pi`。画师固定用 Codex，不接受 `--level`、`--model` 或非 codex 的 `--agent`。
- 测试目录之外的破坏性命令会被拦截：Claude/PI 由 weeco guard 检查，Codex 由 workspace-write 沙箱限制；guard 只覆盖 Bash 命令，文件写入工具不在其内。不要用 `send` 要求成员绕过。
- `start` 启动真实成员；`--replace` 用于明确替换同名成员，先检查现状并保留必要成果。`start` 会确认成员真正开始任务；未确认时保留 pane 并报错，先检查再重试或取消。
- `send` 给成员启动新 turn。CLI 注入明确的 turn；补充与返工任务说明承接哪轮，成员按收到的 turn 报告，不能读取最新 turn 冒领旧结果。成员仍在 `working` 时 CLI 会拒绝 `send`，先等当前报告或检查 pane；新一轮有自己的完成依据。
- `status [ALIAS] --json` 可读取结构化状态；`--wait --timeout SEC` 用于等待，超时后核对状态与报告，不反复轮询。idle、无输出和超时都不代表完成。
- 成员用 `report --turn N --file FILE` 提交当前轮成果，无法继续时加 `--blocked`。`send lead --file FILE` 只向负责人通信，不等于决定已获批准。
- 报告后成员等待返工或关闭。成员不负责最终验收，不自行关闭团队，不用 `/weeco` 改变角色。身份、原始任务、控制目录、当前 turn 和报告命令由 CLI 注入，行为见 [member-prompt.md](member-prompt.md)。

## 检查、返工和交付

- 负责人读取实际交付物与原始来源，按用户目标检查当前结果。软件需针对当前版本实测；事实需回查材料；方案检查约束、执行和失败路径。按风险决定检查深度，不能只读完成声明。
- UI 实现前核对原型确认记录；实现后按已确认原型检查关键页面、状态、交互、窄屏和深色模式。原型与实现不一致时，记录偏差或用 `send` 返工，不能静默偏离。
- 有独立检查价值时，让检查者从原始要求和证据出发尝试找反例；另一角色签字或重复作者结论不能证明独立。
- 发现问题时，写清现象、期望、依据和修复边界，用 `send` 给相应成员开启返工轮；负责人也可以修复自己负责部分，修改后重查受影响结果。
- 检查完成可选用 `accept <alias> --turn N --file NOTE` 记录核验了哪些产物、实际执行和未验证项；它只留痕，不改变 `close` 的条件。同一轮之后又有新报告时，验收标记为过期，需要重新核验。
- 阻塞需要用户决定时，只暂停依赖该决定的工作，保留相关成员；可继续独立准备。不要把通信、超时或未反对当授权。
- 在任务交付边界内检查并完成必要返工后，说明交付位置、实际验证、未验证项和剩余问题，再 `close [ALIAS]` 或 `close` 关闭成员。关闭保留当前主 pane；仍待决定或返工时不关闭。
- 使用了 worktree 时，在 `close` 前完成上面的清理确认：报告路径、分支、workspace 和 `git status`，询问用户保留还是删除。用户未决定前不要删除，也不要用 `workspace close` 代替。`weeco close` 不删除 worktree。
- 取消使用 `close [ALIAS] --cancel --reason "原因"`。关闭不是验收证据，不把取消或未验证结果说成完成。

运行记录保存在 `~/.weeco/runs/<id>/run.json` 及任务、报告快照中。以 CLI 管理状态，不手改记录；快照不能自动证明成果正确。
