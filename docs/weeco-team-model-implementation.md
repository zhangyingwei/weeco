# weeco 团队模型实施文档

依据：[weeco-team-model.md](weeco-team-model.md)。

本文只描述怎么改代码、改哪些文件、按什么顺序验证。设计口径以团队模型文档为准。

**实现状态**：v0.12.0 已按本文完成代码和自动测试；真实 Herdr 冒烟（包括 Codex 画师出图）尚未执行。

**本期范围**

- `start --level`、`start --model`；
- 等级模型候选与单次启动内的失败转移；
- 画师角色：`--role artist` 固定用 Codex；
- `weeco config init` / `weeco config check`；
- `weeco accept`；
- `status` 展示等级、模型、画师、验收状态；
- 技能、快速开始、README 同步。

**本期不做**

- 文生视频；
- 独立的图像能力/供应商系统；
- `round_robin`、`random` 策略；
- 跨 `start` 的模型冷却；
- inbox、ack、stale 检测、负责人恢复；
- Windows 支持。

---

## 1. 现状

当前 `start` 的流程在 `internal/commands/start.go`：

1. 解析 `--name --role --task --agent --replace`；
2. 解析或新建 run；
3. 校验成员是否重复；
4. 生成成员身份，写入任务文件和成员提示；
5. 按 agent 类型生成 guard 配置；
6. 分屏、启动 agent、核对 live session；
7. 注入任务，等待 `working`。

模型参数目前完全由 `memberNativeArgs` 写死，只有三种 agent 的固定参数。

---

## 2. 动手前验证

必须先确认以下两点，否则后面的实现会返工：

1. **各 CLI 的模型与强度参数**
   - `claude --help`：确认模型参数和强度参数；
   - `codex --help`：确认模型参数和 reasoning effort 的写法；
   - `pi --help`：同上。

   把结果写进实现里的映射表；未确认前不要猜参数。

2. **Codex 作为画师能产出图片**
   - 用真实 Codex 成员跑一个最小任务，确认它能生成图片文件并给出路径。
   - 如果只能生成代码或提示词、不能出图，画师方案需要先调整，不能进入实现。

---

## 3. 配置

### 3.1 文件与优先级

```text
内置默认 < ~/.weeco/config.json < <项目>/.weeco/config.json < 命令行参数
```

- 全局路径：`store.Home()/config.json`，受 `WEECO_HOME` 影响。
- 项目路径：`<项目>/.weeco/config.json`。
- 新建 run 时项目目录取当前工作目录；已有 run 时取 `run.Dir`。
- 项目配置按等级覆盖全局；项目没写的等级继承全局。
- 文件不存在 → 使用内置默认。
- 文件存在但为空或非法 → 报错，不带着错误配置运行。

### 3.2 结构

```json
{
  "levels": {
    "senior": [
      { "model": "<资深模型名>", "effort": "high" },
      { "agent": "codex", "model": "<另一个资深模型名>" }
    ],
    "mid":    [ { "model": "<中级模型名>", "effort": "medium" } ],
    "junior": [ { "model": "<初级模型名>", "effort": "low" } ],
    "temp":   [ { "agent": "pi", "model": "<临时模型名>" } ]
  }
}
```

字段约束：

| 字段 | 约束 |
|---|---|
| 顶层 | 只允许 `levels` |
| 等级键 | 只允许 `senior`、`mid`、`junior`、`temp` |
| `agent` | 可省略，默认 `claude`；只允许 `claude`、`codex`、`pi` |
| `model` | 用户配置中必填且非空 |
| `effort` | 可省略；允许 `low`、`medium`、`high`、`none`；省略时取等级默认：senior=high，mid=medium，junior=low，temp=low |
| 候选 | 必须是对象或对象数组；数组顺序就是失败转移顺序 |

用 `json.Decoder.DisallowUnknownFields()` 解析。未知字段必须报错并指出字段名。

### 3.3 内置默认

四个等级各一个候选：

```go
{Agent: "claude", Model: "", Effort: "none"}
```

不传模型和强度参数，保持当前版本行为。

### 3.4 校验

- 顶层、等级、候选的未知字段报错；
- `agent` 非法报错；
- 用户配置的 `model` 为空报错；
- `effort` 非法报错；
- 某个等级在配置里缺失时警告，并使用内置默认；
- `effort` 与 `agent` 的组合不被该 CLI 支持时报错，提示改 `effort` 或换 agent；
- 未配置任何文件时不警告。

### 3.5 `config init` / `config check`

```bash
weeco config init              # 写 ~/.weeco/config.json
weeco config init --project    # 写 ./.weeco/config.json
weeco config init --force      # 覆盖已有文件
weeco config check             # 输出最终解析结果
```

`config init` 写入合法的最小文件：

```json
{
  "levels": {}
}
```

同时打印一段可复制的配置示例。用户填完后再跑 `config check`。

`config check` 输出示例：

```text
source: built-in + global + project
senior: 2 candidates → <模型名> (agent=claude, effort=high), <模型名> (agent=codex, fallback)
mid:    1 candidate  → <模型名> (agent=claude, effort=medium)
junior: 1 candidate  → <模型名> (agent=claude, effort=low)
temp:   1 candidate  → <模型名> (agent=pi, effort=low)
```

---

## 4. 状态字段

`internal/store/run.go` 的 `Member` 增加：

```go
type Member struct {
    // 现有字段不变
    Level        string    `json:"level,omitempty"`
    Model        string    `json:"model,omitempty"`
    Effort       string    `json:"effort,omitempty"`
    Attempts     []Attempt `json:"attempts,omitempty"`
    AcceptedTurn int       `json:"accepted_turn,omitempty"`
    AcceptedRev  int       `json:"accepted_revision,omitempty"`
    AcceptNote   string    `json:"accept_note,omitempty"`
    AcceptedAt   string    `json:"accepted_at,omitempty"`
}

type Attempt struct {
    Agent  string `json:"agent"`
    Model  string `json:"model,omitempty"`
    Effort string `json:"effort,omitempty"`
    Error  string `json:"error,omitempty"`
    At     string `json:"at"`
}
```

说明：

- 普通成员：`Level` 为 `senior/mid/junior/temp`，`Model`、`Effort` 为实际使用的值。
- 画师：`Role` 存 `画师`，`Kind` 存 `codex`，`Level`、`Model`、`Effort` 留空。
- 旧 `run.json` 没有这些字段，反序列化为零值，`status` 显示 `-`，不做迁移。
- `AcceptedRev` 用于判断验收后是否又有新报告：`Revision` 与 `AcceptedRev` 不一致时，`status` 标记验收已过期。

---

## 5. `start` 改动

### 5.1 新参数

```bash
weeco start --name coder --role "实现后端" --level mid --task FILE
weeco start --name coder --role "实现后端" --level mid --model <模型名> --task FILE
weeco start --name illustrator --role artist --task FILE
```

保留现有 `--agent`。

### 5.2 参数行为

| 输入 | 行为 |
|---|---|
| `--role artist` 或 `--role 画师` | 固定 `agent=codex`，使用 Codex 默认模型；拒绝 `--level`、`--model`；`--agent` 只能是 `codex` |
| 有 `--level` | 使用该等级候选；默认 `mid` |
| 有 `--model` | 在等级候选中按模型名精确过滤；没有匹配就报错并列出候选 |
| 有 `--agent` 且同时有 `--level`/`--model` | 再按 agent 过滤；过滤后为空就报错 |
| 只有 `--agent`，没有 `--level`/`--model` | 保留旧行为：kind 用 `--agent`，不套用等级配置，不传模型参数 |
| 没有任何选择参数 | 使用 `mid` 等级；内置默认是 `claude` 且不传模型参数 |

`artist` 和 `画师` 是保留字，不能再作为普通成员的 `--role` 文本使用。

### 5.3 跳过的校验

以下情况直接报错，不进入启动：

- `--level` 不存在；
- `--model` 在该等级候选中没有匹配；
- `--agent` 过滤后没有候选；
- 画师传了 `--level`、`--model` 或非 codex 的 `--agent`；
- 配置非法；
- 选中的 agent 不在 PATH 上（如果所有候选都不可用，可以记录后统一失败）。

### 5.4 候选循环

把现有“分屏 → 启动 → 注入 → 等 working”抽成单次尝试，外层按候选循环：

```text
for candidate in candidates:
    attempt = 记录候选
    if agent 不在 PATH:
        记录错误，继续下一个
    m.Kind, m.Model, m.Effort, m.Level = candidate
    生成该 kind 的 guard 配置
    生成新的成员 agent 名
    分屏
    agent start
    核对 live session 和 tab
    保存 run
    注入任务
    等待 working
    成功 → m.State = running，结束
    失败 → 分类：
        停止类 → 保留 pane，记录错误，返回
        转移类 → 关闭 pane，记录错误，继续下一个
全部失败 → m.State = failed，汇总错误，不留 pane
```

每次尝试写入 `m.Attempts`，时间用 `sysutil.NowISO()`。

### 5.5 失败分类

| 情况 | 处理 |
|---|---|
| agent 不在 PATH | 记录，试下一个候选 |
| `agent_pane_busy`、`not an available shell` | 同一个候选内部重试，由现有 `herdr.AgentStart` 处理 |
| `agent_not_ready`、`blocked during startup` | 停止，保留 pane，让人检查信任提示或模态框 |
| 注入返回 `agent_blocked` | 停止，保留 pane |
| 注入后等不到 `working` | 停止，保留 pane，不自动换模型 |
| 其他 `agent start` 错误 | 关闭 pane，记录尝试，试下一个候选 |
| 全部候选失败 | `State=failed`，错误汇总写到 `LastError` |

### 5.6 pane 和身份处理

- 每次尝试重新分屏；失败的 pane 必须关闭后再试下一个。
- 每次尝试生成新的 agent 名，成功后写回 `m.Agent`，避免 Herdr 侧残留同名 agent。
- 任务文件、成员提示、claude settings、pi extension 的路径保持按成员固定；kind 变化时覆盖对应文件。
- `--replace` 继续要求旧 pane 没有 live agent；新成员 turn 为旧 turn + 1。
- 重复成员判断增加 `Level`、`Model`、`Effort`：任务相同但等级或模型不同，必须用 `--replace` 或 `send`。

### 5.7 输出

`start` 成功输出增加：

```text
level: mid
model: <实际模型名>
effort: medium
attempts: 1
```

失败时输出每次尝试的 agent/model 和错误摘要。

---

## 6. `agentstart.go` 改动

新增映射函数：

```go
func modelArgs(kind, model, effort string) ([]string, error)
```

| kind | model | effort |
|---|---|---|
| claude | `--model <name>` | 按 `claude --help` 实测结果映射 |
| codex | `-m <name>` 或实测结果 | 按 `codex --help` 实测结果映射 |
| pi | 按 `pi --help` 实测结果 | 按实测结果映射 |

规则：

- `model` 为空 → 不传模型参数。
- `effort` 为 `none` 或空 → 不传强度参数。
- agent + effort 组合不支持 → 返回错误，由 `config check` 和 `start` 共同拦截。
- 这些参数追加在现有权限、sandbox、guard 参数之后。

`memberNativeArgs` 改为接收 `Candidate`，其余现有参数不变。

---

## 7. `status` 改动

人类可读输出增加：

```text
coder  state=submitted live=idle turn=2 level=mid model=<模型名> role=实现后端 attempts=1
```

有验收记录时：

```text
  accepted: turn 2 (2026-11-01T10:00:00+08:00) note=/path/to/note
```

- `Revision != AcceptedRev` 时输出 `accepted: stale (turn N, revision changed)`。
- `--json` 不需要额外结构，`runtimeMemberView` 已嵌入 `Member`，新字段自动出现。
- 不改变 `status --wait` 的现有判定逻辑。

---

## 8. `accept` 命令

```bash
weeco accept [ALIAS] --turn N --file NOTE [--run ID]
```

流程：

1. 必须是负责人调用；
2. 成员必须存在；
3. `--turn` 必须等于成员当前 `Turn`；
4. 成员状态必须是 `submitted`，`blocked` 不接受；
5. 备注写入 `members/<name>/accepts/turn-N-accept.md`；
6. 写入 `AcceptedTurn`、`AcceptedRev=Revision`、`AcceptNote`、`AcceptedAt`；
7. 输出成员、turn、备注路径。

`accept` 不改变成员状态，不替代 `close`，不改变 `close` 的条件。验收后若同一轮又提交了新 revision，`status` 标记验收过期，负责人需要重新验收。

---

## 9. 文件清单

### 新增

| 文件 | 内容 |
|---|---|
| `internal/config/config.go` | 配置结构、默认值、加载、合并、校验、候选解析 |
| `internal/config/config_test.go` | 配置单测 |
| `internal/commands/config.go` | `config init` / `config check` |
| `internal/commands/accept.go` | `accept` |
| `internal/commands/start_level_test.go` | 等级、失败转移、画师测试 |

### 修改

| 文件 | 改动 |
|---|---|
| `internal/store/run.go` | `Member`、`Attempt` 新字段 |
| `internal/commands/root.go` | 注册 `config`、`accept`；usage |
| `internal/commands/start.go` | 新参数、候选循环、画师、尝试记录 |
| `internal/commands/agentstart.go` | `modelArgs`、`memberNativeArgs` 接收候选 |
| `internal/commands/status.go` | 等级、模型、验收展示 |
| `internal/commands/run.go` | `memberPrompt` 带等级和模型 |
| `internal/commands/testdata/fakeherdr/main.go` | 按模型注入启动失败；保留 `native_args` |
| `internal/commands/runtime_test.go` | 快照字段、启动用例 |
| `internal/commands/runtime_edge_test.go` | 失败转移、验收边界 |
| `internal/assets/files/skills/weeco/SKILL.md` | 负责人使用等级、画师、accept |
| `docs/weeco-quickstart.md` | 新参数示例 |
| `README.md` | 命令表和使用说明 |
| `Makefile` | `VERSION ?= 0.12.0` |
| `internal/commands/root.go` | `Version = "0.12.0"` |

安装逻辑不需要改；`install` 会自动安装更新后的技能文件。

---

## 10. 兼容性

- 不配置任何文件：行为与当前版本一致，`claude` 成员，不传模型参数。
- 现有 `--agent codex` / `--agent pi` 用法保持不变。
- 旧 `run.json` 可直接读取，新字段为空。
- 旧成员没有 `Attempts`，`status` 显示 `attempts=0`。
- `--replace` 的旧 pane 检查逻辑不变。
- `go.mod` 不变，不引入新依赖；JSON 用标准库。

---

## 11. 测试计划

### 11.1 配置

- 无配置文件 → 四个内置默认等级；
- 只有全局配置 → 生效；
- 项目配置覆盖一个等级，其他等级继承全局；
- 未知顶层字段、未知等级、未知候选字段 → 报错并指出字段名；
- 用户配置 `model` 为空 → 报错；
- 非法 `agent` / `effort` → 报错；
- 缺某个等级 → 警告并使用默认；
- `--model` 过滤无匹配 → 报错并列出候选；
- `--agent` 过滤无匹配 → 报错。

### 11.2 启动

- 默认启动：fakeherdr 记录的 `native_args` 不含模型参数；
- `--level senior`：选中第一个候选，`native_args` 含对应模型参数；
- `--level` + `--model`：选中指定候选；
- `--level` + `--agent codex`：只在 codex 候选中选；
- 只有 `--agent codex`：旧行为，不套等级配置；
- 第一个候选启动失败，第二个成功：`Attempts` 有两条，`Kind/Model` 为第二个；
- 信任提示/`blocked during startup`：停止，不尝试下一个；
- 注入后等不到 `working`：停止，保留 pane；
- 全部候选失败：成员 `failed`，`LastError` 汇总，无残留 pane；
- `--replace` 且等级变化：允许替换，turn 递增；
- 画师：`role artist` → `Kind=codex`；传 `--level`/`--model`/`--agent claude` 报错。

### 11.3 验收

- 负责人接受成功，写入备注和字段；
- 成员调用被拒绝；
- turn 不符被拒绝；
- 状态不是 `submitted` 被拒绝；
- 验收后同一轮又有新 revision，`status` 标记过期。

### 11.4 回归

```bash
go test ./...
go test -race ./...
go vet ./...
make build
```

真实 Herdr 冒烟：

1. 无配置启动一个 `mid` 成员；
2. 配置一个 `senior` 候选，`--level senior` 启动，确认参数生效；
3. `--role artist` 启动画师，确认是 Codex 并能产出图片；
4. 人为让第一个候选启动失败，确认转移到第二个；
5. 提交报告后 `accept`，再提交一次，确认验收过期提示。

---

## 12. 实施顺序

| 阶段 | 内容 | 完成标准 |
|---|---|---|
| 0 | 验证 CLI 参数和 Codex 画师 | 参数映射表确定；Codex 能出图 |
| 1 | `internal/config` + `config init/check` | 配置单测全过；非法配置报错正确 |
| 2 | `Member` 新字段 + `start --level/--model` + `status` 展示 | 默认行为不变；等级参数能透传 |
| 3 | 候选失败转移 + 画师预设 | 失败转移和画师测试通过 |
| 4 | `accept` | 验收测试通过 |
| 5 | `SKILL.md`、快速开始、README、版本号 | 文档与命令一致 |
| 6 | 全量回归 + 真实冒烟 | 第 11.4 节全部通过 |

---

## 13. 风险与待确认

1. **CLI 参数不确定**：三家 CLI 的模型/强度写法必须实测，不能猜；如果某家不支持强度，用 `effort: "none"` 或报配置错误。
2. **Codex 画师能力**：如果 Codex 不能直接产出图片，画师方案需要重新定义，本实施文档的画师部分暂停。
3. **agent 名复用**：Herdr 关闭 pane 后是否立即释放 agent 名需要实测；不释放就每次尝试生成新名字。
4. **`--agent` 兼容路径**：只有 `--agent` 时绕过等级配置，这是为了不破坏现有用法；如果希望它也从配置取模型，需要另行确认。
5. **项目配置覆盖**：按等级整体替换，不合并候选数组；避免顺序歧义。
6. **画师保留字**：`artist` 和 `画师` 不能再作为普通 `--role` 文本。
7. **验收过期**：同一轮重新提交会让已有验收失效，需要负责人重新 `accept`。

---

## 14. 完成定义

满足以下全部条件才算实施完成：

- 不配置任何文件时，现有命令行为不变；
- `--level`、`--model`、画师、`accept`、`config init/check` 按本文工作；
- 所有新增分支有自动化测试；
- `go test ./...`、`go test -race ./...`、`go vet ./...`、`make build` 全过；
- 真实 Herdr 冒烟按第 11.4 节走通；
- `SKILL.md`、`docs/weeco-quickstart.md`、`README.md` 与命令一致。
