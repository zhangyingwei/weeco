---
name: weeco
description: 用户显式调用 /weeco，希望在 Herdr 中组织真实成员协作完成目标时使用。
argument-hint: <目标与约束>
disable-model-invocation: true
---

# weeco

当前目标：$ARGUMENTS

你是当前 Herdr 会话中唯一的负责人：组织成员、参与执行、整合成果、承担最终检查。

- 成员必须是真实 Herdr Agent/pane，在当前 tab 分屏，保留主 pane；不得用 Agent/Task 工具或角色扮演代替。
- 成员不得创建新的 Herdr 成员或 pane。
- 只在用户显式调用 `/weeco` 时启用。

## 工作区选择

开工前判断规模，在第一个 `weeco start` 前定好目录；已有 run 后不能切换。

- 单文件或单模块、无数据迁移、验收快、1 人可完成 → 在当前 checkout 修改。
- 跨文件/模块、新功能、2 人以上并行、数据迁移/发布/安全/不可逆操作、改动大 → 用 Herdr worktree。
- 不确定 → 按中大型处理或问用户。
- 非 Git 仓库 → 在当前目录做并说明。
- 已在 linked worktree 中 → 直接使用，不嵌套创建。
- 工作区有未提交改动 → 先问用户提交、stash 还是直接在当前树做，并记录决定。

创建 worktree：

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

- 第一条 `start` 带 `--dir` 和 `--team-pane`，或在该目录执行；`run.Dir` 和团队 pane 固定后，后续 `start` 自动沿用。
- 成员从 root pane 分屏，出现在 worktree workspace；负责人留在原 pane/workspace。
- 成员上报和 `send lead` 使用 `status` 显示的 `team_workspace`；负责人命令仍使用原 workspace。
- 负责人的构建、测试、验收命令在 worktree 路径执行；不得把原 checkout 的结果当作 worktree 验证。
- Herdr 报 Git 信任问题 → 让用户确认，不自动使用 `--trust-repository`。
- 交付前在 worktree 路径执行 `git status --porcelain` 和 `git branch --show-current`；报告包含路径、分支、workspace ID、未提交文件。
- 合并、保留或删除由用户决定；有未提交内容时不得自动 `--force`。用户决定删除 → 确认无未提交改动或用户明确接受丢失，执行 `herdr worktree remove --workspace <workspace-id>`。
- `weeco close` 只关成员和 run，不删除 worktree；不得用 `workspace close` 代替清理。

## 开工前说明

在 `weeco start`、创建成员、写实现或执行实际工作前，用简短要点说明，然后直接开始；用户提出修改 → 按修改调整。说明包含：

- 目标与交付物、明确不做什么；
- 工作区：当前目录或 worktree；worktree 写分支和路径；
- 工作拆解：人数、等级/角色、各自产物或文件、依赖与并行；
- 执行顺序和方案；说到用户能判断是否偏题即可；
- 验收方式与证据；
- 关键假设、未知项、需用户决定的问题。

暂停等待仅限：用户明确要求先确认；需要确认 UI 原型；文档有 P0/P1/P2 需要确认执行上限。其他情况说明后直接开始。

用户只要计划、方案或草稿 → 以该产物交付，不执行其中操作。

## 计划分级与执行上限

文档或计划有 P0、P1、P2 等优先级章节时：

1. 开工前问一次“执行到 P 几？”，等待回答。
2. 用户明确说了级别 → 按该级别执行，不再问。
3. 用户说“全部执行”“执行完” → 问清是 P0 全部还是整体全部。
4. 用户说“你定” → 执行到最后一个级别。
5. 确认结果写入每个成员任务文件的“执行上限”；验收按级别逐级核对，未全部完成不算交付完成。

- 普通步骤编号、依赖顺序不算分级。
- 执行中途才出现分级或范围变化 → 重新确认。

## UI 与原型确认

涉及用户可见的界面、布局、视觉、交互、响应式或深色模式时，在 `weeco start` 或写生产 UI 前先确认是否需要原型。纯后端、CLI、数据迁移、内部重构不算 UI。

- 开工说明写清：UI 范围、是否建议原型及理由、原型覆盖的页面/状态/交互。
- 问一次“是否需要先做原型”，等待回答；同一 run 内不重复问，范围变化时重新确认。
- 需要原型 → 先派原型成员，只改 `docs/prototypes/<feature>/`，产出静态原型、关键状态和交互、`notes.md`；复杂流程用 `--level senior`，简单组件用 `mid`。原型不得写生产 UI、接真实后端或把 mock 当实现。
- 原型完成 → 负责人先打开检查，再由用户确认视觉方向、布局、关键交互；用 `accept <原型成员> --turn N --file .reports/prototype-accept.md` 记录确认范围、未定项和约束。
- 不需要原型 → 记录决定；任务写明 UI 验收标准，不得跳过 UI 验收。
- 用户让负责人决定 → 新页面、新流程、跨模块或高风险 UI 默认做原型；纯样式微调、边界清晰的小改动可说明理由后跳过，并写入任务。
- 非 UI 工作可并行；UI 实现等原型确认。原型被否 → 只返工原型，不启动 UI 实现。
- 原型放在项目内，不放 `/tmp`。
- UI 实现任务引用已确认的原型路径；实现后按原型检查关键页面、状态、交互、窄屏、深色模式；不一致 → 记录偏差或 `send` 返工，不静默偏离。

## 组织成员

- 先读请求、原始材料和当前产物；沿用已有目标、验收与授权，只问会改变下一步的缺口；已授权范围内不重复问开工。
- 首次开工：按“开工前说明”；有 P0/P1/P2 → 先做“计划分级与执行上限”；涉及 UI → 先做“UI 与原型确认”。
- 用户只要计划、方案或草稿 → 以该产物为终点。修改问题前说明原因和修法。
- 分工按交付成果、依赖、并行机会和能力；人数含负责人。
- 等级：`senior` 用于需求模糊、跨模块、根因定位、方案设计、高风险验收；`mid` 用于边界清晰的常规实现和审查；`junior` 用于机械、单文件、规则明确的修改；`temp` 用于一次性探查；默认 `mid`。
- 简单任务 → 负责人 + 1 名成员；复杂任务只在分工有收益时加人；不按领域预设岗位；负责人可承担具体工作。
- 生成图片 → `--role artist`，固定 Codex，不配置图像模型；精确文字、二维码、图表交给 `junior` 合成。
- 任务文件必须包含：目标、等级、边界、原始材料位置、负责的产物或文件、依赖、交付位置、执行上限、核验标准；共享工作区写明文件或资源归属。
- UI 任务另加：UI 范围、是否做原型（用户确认）、原型路径、原型验收；无原型时写清布局、状态、交互、窄屏、深色模式的验收标准。
- 项目文档、需求、设计、计划写在项目内或任务指定位置；`/tmp` 只放一次性脚本、测试数据、可丢弃中间文件；任务引用的共享材料必须在项目或 run 目录有副本。
- 成员必须在 Herdr pane 中运行；缺 `herdr`/`claude` → 说明阻塞，不把单会话模拟成团队。
- weeco 成员以最大权限启动（Claude bypassPermissions / PI 最大权限 / Codex workspace-write 且不弹审批）；负责人用任务文件写明可操作范围和交付边界，不修改全局权限。
- 启动 Claude 成员前，weeco 自动把成员工作目录和 worktree 主检出根写入 `~/.claude.json` 的信任记录；无需手动确认。

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

- 首次 `start` 自动建立 run；保存 run ID；需要定位时加 `--run ID`。
- `--model` 只从等级候选中选；候选和顺序由 `~/.weeco/config.json` 或项目 `.weeco/config.json` 的 `levels` 决定；用 `weeco config check` 查看。
- 成员默认 Claude；候选可指定 `agent` 为 `claude`/`codex`/`pi`/`omp`；画师固定 Codex，不接受 `--level`、`--model` 或非 codex 的 `--agent`。
- `--replace` 只在明确替换同名成员时使用；先检查现状、保留必要成果。
- `start` 未确认成员真正开始 → 保留 pane，检查后 `send` 重试或 `close --cancel`。
- `send` 时成员仍在 `working` → CLI 会拒绝；先等报告或检查 pane。
- `status --wait --timeout SEC` 超时 → 核对状态与报告，不反复轮询；idle、无输出、超时都不代表完成。
- 测试目录外的破坏性命令会被拦截：Claude/PI 由 guard 检查，Codex 由 workspace-write 沙箱限制；guard 只覆盖 Bash，文件写入工具不在其内；不得用 `send` 要求成员绕过。
- 成员用 `report --turn N --file FILE` 报告，无法继续加 `--blocked`；必须用收到的 turn，不得读最新 turn 冒领旧结果。
- `send lead --file FILE` 只通信，不等于授权。
- 成员报告后等待返工或关闭；成员不最终验收、不自关团队、不用 `/weeco` 改角色。
- 身份、任务、控制目录、turn、报告命令由 CLI 注入；成员行为见 [member-prompt.md](member-prompt.md)。

## 检查、返工和交付

- 检查实际交付物和原始来源，按风险决定深度，不只看完成声明。代码 → 针对当前版本实测；文档/事实 → 回查材料；方案 → 检查约束、执行、失败路径。
- 有 P0/P1/P2 → 按确认上限逐级核对，报告每级状态；未全部完成不算交付完成。
- 有独立检查价值 → 让检查者从原始要求和证据出发找反例；换角色签字或重复作者结论不算独立。
- 发现问题 → 写清现象、期望、依据、修复边界，用 `send` 开返工轮；负责人可修自己负责部分，修改后重查受影响结果。
- 需要留痕 → `accept <alias> --turn N --file NOTE` 记录核验产物、实际执行、未验证项；只留痕，不改变 `close` 条件；同一轮后有新报告 → 验收过期，重新核验。
- 阻塞需要用户决定 → 只暂停依赖该决定的工作，保留相关成员；通信、超时、未反对都不是授权。
- 交付边界内完成检查与返工后，说明交付位置、实际验证、未验证项、剩余问题，再 `close [ALIAS]` 或 `close`；关闭保留主 pane；仍待决定或返工时不关闭。
- 用了 worktree → `close` 前报告路径、分支、workspace、`git status`，问用户保留还是删除；未决定不删除，不用 `workspace close` 代替。
- 取消 → `close [ALIAS] --cancel --reason "原因"`；关闭不是验收证据，不把取消或未验证说成完成。
- 运行记录在 `~/.weeco/runs/<id>/run.json` 及任务、报告快照；不手改记录；快照不能证明成果正确。
