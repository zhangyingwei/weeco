# weeco 团队模型实现

依据：[weeco-team-model.md](weeco-team-model.md)。

本文描述当前代码如何组织团队、启动成员、派发任务、关闭资源，以及对应的状态字段和测试。设计口径以团队模型文档为准；本文只记录代码事实。

## 1. 运行拓扑

```text
负责人 pane（原 workspace，保持不动）
团队 workspace（weeco 创建或 worktree 提供）
  ├─ 成员 A 的 tab
  ├─ 成员 B 的 tab
  └─ 成员 C 的 tab
```

- weeco 为每个成员分配一个独立 tab 和一个 Agent pane。
- 默认情况下，第一次 `start` 创建一个由 weeco 拥有的团队 workspace，标签为 `weeco <run-id>`。
- 第一个成员复用该 workspace 的 root tab，并把 tab 重命名为成员别名；后续成员各自 `tab create`。
- 传 `--team-pane` 时，成员 tab 建在该 pane 所属的 workspace 中（通常是 worktree workspace），weeco 不拥有这个 workspace。
- 负责人始终留在原 pane/workspace；成员命令通过 Herdr socket 和 pane 身份跨 workspace 工作。
- `close` 只关闭成员 tab。由 weeco 拥有的 workspace 会在成员全部关闭后移除；借用 workspace 永远不会被 weeco 关闭。

## 2. `start` 流程

入口：`internal/commands/start.go`。

1. 解析 `--name --role --task --agent --level --model --dir --team-pane --replace`。
2. 校验名称、角色、等级、agent、模型和任务文件；解析 `--dir`。
3. 解析 `--team-pane`，记录其 workspace 和 tab。
4. 从 `HERDR_PANE_ID` / `HERDR_WORKSPACE_ID` 读取当前负责人上下文，确认当前 pane 有 live agent。
5. 在 `store.RuntimeLock` 内解析或创建 run。
6. 校验团队 pane 和目录在已有 run 上不可变；校验调用者必须是负责人。
7. 解析等级候选，判断成员是否重复；需要替换时先确认原资源已释放。
8. 写任务文件、成员提示和 guard 配置。
9. 按候选顺序启动成员；每个候选记录一次 `Attempt`。
10. 成功后写入实际 agent、模型、强度、pane、tab、workspace 和 session。

新 run 的字段：

```go
store.Run{
    LeadPane:      ctx.Pane,
    LeadSession:   ctx.Session,
    Workspace:     ctx.Workspace,
    TabID:         ctx.Tab,
    Dir:           runDir,
    TeamPane:      teamPaneFlag,      // 仅 --team-pane
    TeamWorkspace: teamWorkspace,     // 仅 --team-pane
    TeamTab:       teamTab,           // 仅 --team-pane
}
```

默认模式首次启动成员时才创建团队 workspace，因此没有成员时不会留下空 workspace。

### 2.1 成员 tab 分配

`openMemberTab` 的规则：

```text
if 团队 workspace 为空:
    workspace create --cwd run.Dir --label "weeco <run-id>" --no-focus
    使用返回的 root tab/pane
    tab rename <tab> <成员别名>
else if workspace 由 weeco 拥有:
    确认 workspace 仍然存在
    存在 → tab create --workspace <ws> --cwd run.Dir --label <别名> --no-focus
    不存在 → 清空失效的 workspace 记录并重新创建
else:
    tab create --workspace <ws> --cwd run.Dir --label <别名> --no-focus
```

分配的 tab 和 pane 写回：

```go
m.WorkspaceID = workspace
m.TabID       = tab
m.PaneID      = pane
```

随后才执行 `agent start`，并把 agent 实际出现的 tab 与 `m.TabID` 核对。

## 3. 候选循环与失败处理

每个候选执行一次完整尝试：

```text
记录 Attempt
if agent 不在 PATH:
    记录错误，试下一个候选
生成 guard 配置和 native 参数
生成新的成员 agent 名
分配成员 tab
agent start
核对 live agent 的 tab 与身份
保存 run
注入任务
等待 working
成功 → m.State = running
失败 → 停止或转移
```

失败分类：

| 情况 | 处理 |
|---|---|
| agent 不在 PATH | 记录，试下一个候选 |
| `agent_pane_busy`、`not an available shell` | `herdr.AgentStart` 在本次尝试内部重试 |
| `agent_not_ready`、`blocked during startup` | 停止，保留成员 tab 供检查 |
| 注入返回 `agent_blocked` | 停止，保留成员 tab |
| 注入后等不到 `working` | 停止，保留成员 tab，不自动换模型 |
| 其他 `agent start` 错误 | 关闭成员 tab，记录尝试，试下一个候选 |
| 全部候选失败 | `State=failed`，清空 pane/tab 记录，汇总到 `LastError` |

停止类失败会保留成员 tab；负责人检查后用 `send` 重试或 `close --cancel` 结束。转移类失败关闭成员 tab；如果关闭失败，同样保留资源并停止，避免泄漏。

全部候选失败且没有其他成员 tab 时，启动流程会清理 weeco 创建的 workspace。

## 4. 成员身份与命令上下文

成员命令（`report`、`send lead`、`status` 等）在成员自己的 tab 中执行，workspace 和 tab 都与负责人不同。

`resolveRun` 的匹配规则：

- 负责人：`pane + session + workspace + tab` 全部匹配 `Run.LeadPane / LeadSession / Workspace / TabID`。
- 成员：只按 `pane` 和 session（或 agent 名）匹配，与所在 workspace/tab 无关。
- 自动解析时会跳过已关闭的 run；显式 `--run` 可以指定。
- 多个 run 同时匹配时报错，要求显式传 `--run`。

`currentRuntime` 仍要求：

```text
HERDR_ENV=1
HERDR_PANE_ID 和 HERDR_WORKSPACE_ID 存在
当前 pane 在那个 workspace 中存在
当前 pane 有 live agent
当前 tab 可解析
```

因此成员在独立 workspace 中调用 CLI 不会丢失 run 归属。

## 5. 关闭与清理

入口：`internal/commands/close.go`。

1. 必须是负责人调用。
2. 普通关闭要求成员处于 `submitted` 或 `closed`；`--cancel --reason` 允许关闭其他状态。
3. 逐个成员处理：
   - 拒绝关闭负责人 pane。
   - 如果成员 pane 上有 live agent，核对 session 或 agent 名，身份不一致则失败。
   - 有 `TabID` → `tab close`；`tab_not_found` 视为已关闭。
   - 没有 `TabID`（pane 直接管理）→ 在成员 workspace 中找到 pane，核对团队 tab 后 `pane close`。
   - 标记 `State=closed`，清空 pane、tab、workspace 和 session 字段。
4. 所有成员都关闭后，`Run.Closed=true`。
5. 没有关闭失败且团队 workspace 由 weeco 拥有时：
   - 关闭 workspace；
   - `workspace_not_found` 视为成功（最后一个 tab 关闭时 Herdr 会自动移除 workspace）；
   - 清空 `TeamWorkspace / TeamWorkspaceOwned / TeamTab / TeamPane`。
6. 借用 workspace：只关闭成员 tab，保留 workspace 和 root tab。
7. 任一成员关闭失败：保留 run 和 workspace，返回失败列表，允许重试。

## 6. 状态字段

`internal/store/run.go` 中与拓扑相关的字段：

```go
type Run struct {
    ID                 string
    LeadPane           string
    LeadSession        string
    Workspace          string
    TabID              string
    Dir                string
    TeamPane           string // --team-pane 的 root pane；owned 模式为空
    TeamWorkspace      string
    TeamTab            string // --team-pane 的 root tab；owned 模式为空
    TeamWorkspaceOwned bool
    Members            map[string]*Member
    Closed             bool
}

type Member struct {
    Name        string
    Agent       string
    Kind        string
    PaneID      string
    TabID       string
    WorkspaceID string
    Session     string
    Role        string
    Level       string
    Model       string
    Effort      string
    Task        string
    TaskSHA     string
    State       string
    Turn        int
    Report      string
    Revision    int
    Attempts    []Attempt
    // 验收字段：AcceptedTurn / AcceptedRev / AcceptNote / AcceptedAt
}
```

说明：

- `Level / Model / Effort`：普通成员记录实际候选；画师三项为空。
- `Attempts`：每次候选尝试的 agent、model、effort、错误和时间。
- `Revision` 与 `AcceptedRev` 不一致时，`status` 标记验收过期。
- 缺少 `TabID` 的成员使用 pane 关闭路径。

## 7. `status`

人类可读输出包含团队位置、成员状态和每个成员的 tab：

```text
run: r-...
lead: wK:p1
team_pane: -
team_workspace: w2
dir: /path/to/run-dir
reason: snapshot
writer  state=submitted live=idle turn=2 level=mid model=... role=写作 tab=w2:t1 attempts=1
```

`--json` 直接输出 `runtimeStatus`：

- `team_pane`、`team_tab`、`team_workspace`、`team_workspace_owned`；
- `members[]` 嵌入 `store.Member`，包含 `tab_id` 和 `workspace_id`；
- `live` 为 `missing / replaced / blocked / idle / working / ...`。

`status --wait` 等待新报告、阻塞或身份异常；idle 和超时不代表完成。

## 8. Agent、模型与权限参数

`internal/commands/agentstart.go` 负责 native 参数。

模型和强度映射：

| agent | model | effort |
|---|---|---|
| `claude` | `--model <name>` | `--effort <low\|medium\|high>`；`none` 省略 |
| `codex` | `-m <name>` | `-c model_reasoning_effort=<low\|medium\|high>` |
| `pi` | `--model <name>` | `--thinking <low\|medium\|high>` |
| `omp` | `--model=<name>` | `--thinking=<low\|medium\|high>` |

基础参数：

| agent | 参数 |
|---|---|
| `claude` | `--add-dir <run-dir>`、`--permission-mode bypassPermissions`、`--append-system-prompt-file <prompt>`、`--settings <guard>` |
| `codex` | `--sandbox workspace-write`、`--ask-for-approval never`、关闭启动更新检查、开启 `sandbox_workspace_write.network_access`、`--add-dir <runs-root>` |
| `pi` | `--approve`、`--extension <guard>` |
| `omp` | `--approval-mode=yolo`、`--add-dir=<run-dir>`、`--extension=<guard>` |

守卫边界：

- Claude 使用 PreToolUse hook 调用 `weeco guard`，只覆盖 Bash。
- PI/OMP 使用 `tool_call` 扩展调用同一个 guard。
- Codex 使用原生 `workspace-write` 沙箱；额外允许写 `~/.weeco/runs` 以便上报。
- guard 是护栏，不是完整沙箱；脚本混淆和文件写入工具不保证被拦截。

## 9. 等级与配置

配置文件优先级：

```text
内置默认 < ~/.weeco/config.json < <项目>/.weeco/config.json < 命令行参数
```

路径：

- 全局：`store.Home()/config.json`，受 `WEECO_HOME` 影响。
- 项目：`<项目>/.weeco/config.json`。
- 新 run 的项目目录取当前工作目录；已有 run 取 `run.Dir`。

结构：

```json
{
  "levels": {
    "senior": [
      { "model": "<model>", "effort": "high" },
      { "agent": "codex", "model": "<model>" }
    ],
    "mid":    [ { "model": "<model>", "effort": "medium" } ],
    "junior": [ { "model": "<model>", "effort": "low" } ],
    "temp":   [ { "agent": "pi", "model": "<model>" } ]
  }
}
```

约束：

| 字段 | 规则 |
|---|---|
| 顶层 | 只允许 `levels` |
| 等级 | `senior`、`mid`、`junior`、`temp` |
| `agent` | 可省略，默认 `claude`；允许 `claude`、`codex`、`pi`、`omp` |
| `model` | 用户配置中必填 |
| `effort` | 可省略；允许 `low`、`medium`、`high`、`none`；等级默认 senior=high、mid=medium、junior=low、temp=low |
| 候选 | 对象或数组；数组顺序就是失败转移顺序 |

解析使用 `json.Decoder.DisallowUnknownFields()`；未知字段报错并给出字段名。

内置默认：四个等级各一个 `{agent: claude, model: "", effort: "none"}`，不传模型参数。

配置中的 `worktree_root`、`agent_kind`、`ready_timeout_ms`、`wait_timeout_s`、`team_lead_permission_mode` 会被读取但忽略；运行参数由代码默认值和命令行控制。

`config init` / `config check`：

```bash
weeco config init              # ~/.weeco/config.json
weeco config init --project    # ./.weeco/config.json
weeco config init --force
weeco config check
```

## 10. `start` 参数行为

| 输入 | 行为 |
|---|---|
| `--role artist` 或 `--role 画师` | 固定 `agent=codex`，使用 Codex 默认模型；拒绝 `--level`、`--model` 和非 codex 的 `--agent` |
| `--level` | 使用该等级候选；默认 `mid` |
| `--model` | 在等级候选中精确过滤；无匹配报错并列出候选 |
| `--level` + `--agent` | 再按 agent 过滤；为空报错 |
| 只有 `--agent` | 直接使用该 kind，不套等级配置，不传模型参数 |
| 无选择参数 | `mid` 等级；内置默认 claude |

## 11. `accept`

```bash
weeco accept [ALIAS] --turn N --file NOTE [--run ID]
```

流程：

1. 必须是负责人调用。
2. 成员必须存在。
3. `--turn` 必须等于成员当前 `Turn`。
4. 成员状态必须是 `submitted`；`blocked` 不接受。
5. 备注写入 `members/<name>/accepts/turn-N-accept.md`。
6. 写入 `AcceptedTurn`、`AcceptedRev=Revision`、`AcceptNote`、`AcceptedAt`。
7. 后续同一轮出现新 revision 时，`status` 标记验收过期。

`accept` 不改变成员状态，不替代 `close`。

## 12. 文件清单

| 文件 | 职责 |
|---|---|
| `internal/herdr/herdr.go` | Herdr CLI 适配：workspace、tab、pane、agent、prompt、wait |
| `internal/store/run.go` | Run/Member/Attempt 持久化结构 |
| `internal/commands/start.go` | 参数解析、run 建立、候选循环、workspace/tab 分配 |
| `internal/commands/close.go` | 成员 tab 关闭、workspace 清理、身份校验 |
| `internal/commands/run.go` | 运行时上下文、run 解析、成员提示、成员校验 |
| `internal/commands/status.go` | 状态与等待 |
| `internal/commands/send.go` | 新 turn 派发 |
| `internal/commands/report.go` | 成员报告 |
| `internal/commands/accept.go` | 验收留痕 |
| `internal/commands/agentstart.go` | guard 配置和 native 参数 |
| `internal/config/config.go` | 等级候选加载、合并、校验、解析 |
| `internal/commands/testdata/fakeherdr/main.go` | 测试用 Herdr 状态机：workspace、tab、pane、agent |
| `internal/commands/runtime_test.go` | 生命周期、身份、权限 |
| `internal/commands/runtime_edge_test.go` | 失败转移、关闭重试、错误边界 |
| `internal/commands/team_pane_test.go` | `--team-pane` / 借用 workspace |
| `internal/commands/workspace_test.go` | owned workspace、每成员 tab、清理、无 TabID 的 pane 成员、替换 |
| `internal/commands/start_level_test.go` | 等级、模型、画师、候选转移 |
| `internal/assets/files/skills/weeco/` | 负责人 Skill 和成员提示 |
| `docs/weeco-quickstart.md`、`README.md`、`docs/weeco-team-model.md` | 使用说明和团队模型 |

## 13. 验证

自动检查：

```bash
go test ./...
go test -race ./...
go vet ./...
make build
```

拓扑相关覆盖：

- 默认启动创建 owned workspace，第一个成员复用 root tab，后续成员各自一个 tab；
- 成员 tab 标签为成员别名，pane cwd 为 `run.Dir`；
- 成员在自己的 tab 中执行 `report` / `send lead`；
- `close` 关闭成员 tab，并在最后一个 tab 后移除 owned workspace；
- `--team-pane` 模式在借用 workspace 中创建成员 tab，关闭后 workspace 和 root tab 保留；
- `--replace` 在成员原 tab 已关闭后创建新 tab，turn 递增；
- 首次启动设置失败会清理刚创建的 workspace；
- 第二个成员 tab 创建失败不影响已有成员和 workspace；
- 没有 `TabID` 的成员仍可按 pane 关闭，负责人 pane 不受影响。

真实 Herdr 冒烟：

- owned 模式：创建 `weeco <run-id>` workspace、1 个成员 tab、成员 pane 在指定目录；`status` 显示 `team_workspace_owned=true`；`close` 后 workspace 消失。
- 借用模式：成员 tab 建在已有 workspace 中，root tab 和 workspace 在 `close` 后保留。

## 14. 当前边界

- 人眼一次只能看一个 tab；程序化读取后台 tab 的 pane 输出不受影响。
- 关闭 owned workspace 会关闭里面所有 tab 和进程；成员不应自行创建额外 pane 或 tab。
- 借用 workspace 只关闭成员 tab，weeco 不清理该 workspace 中的其他内容。
- 用户手动关闭团队 workspace 后，成员 tab 会消失；`close` 将这些成员视为已关闭，但 run 状态可能停留在过时状态。
- 成员身份依赖 pane + session/agent 名；agent 被替换后，`send`、`report` 和 `close` 会拒绝继续操作。
- 所有 run 状态写入 `~/.weeco/runs/<id>/run.json`，不手工修改。
