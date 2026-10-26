# weefleet：用 Herdr + Claude 跑一个最小开发舰队

> 目标：一个人，一个协调者 Claude，一个 worker Claude，一次跑一个开发任务；任务做完必须能被独立验证。
>
> 本文只描述**最小可跑环境**，不追求并行、多机、自动合并。先把一个任务的闭环跑通，再决定要不要加东西。
>
> 最小实现见 [`cmd/weefleet`](../cmd/weefleet)（Go 单二进制，运行时资产编进二进制）；安装与第一个任务见 [`docs/weefleet-quickstart.md`](../docs/weefleet-quickstart.md)。

## 1. 机制，四句话

1. 一个任务 = 一个 git worktree + 一个 Claude pane + 一份 brief + 一份 report。
2. 协调者只做三件事：写 brief、派活、验收；**不写代码**。
3. Herdr 只做三件事：开 pane、起 Claude、告诉我们它停没停。
4. 完成不看 Herdr 的 `done`，只看 `weefleet verify` 重跑验收命令的结果。

记住一句话：**Herdr 是执行面和传感器，不是控制面。控制面是账本 + 命令 + 验收。**

这套设计对“验证成本”有硬约束：**一个 revision 只验一次，验证结果按 commit 复用；完整测试套件不放进每个任务。** 具体做法见第 10 节。

## 2. 最小环境

只需要这些：

| 东西 | 要求 | 检查命令 |
|---|---|---|
| Herdr | ≥ 0.9.3，server 在跑 | `herdr status` |
| Claude 集成 | `claude: current` | `herdr integration status` |
| Claude Code | 能在 pane 里正常启动 | `claude --version` |
| git | 仓库有 `main` 分支 | `git status` |
| weefleet | 单个二进制，`weefleet install` 装好入口 | `weefleet doctor` |

不需要：数据库、常驻服务、Web UI、MCP、消息队列、npm 依赖、运行时解释器。

本机验证过的版本：`herdr 0.9.3`，protocol 22，`claude` 集成 v10。

所有 `weefleet` 命令都在 Herdr 管理的 pane 里运行；`HERDR_WORKSPACE_ID`、`HERDR_PANE_ID` 这些变量由 Herdr 注入。不要在 Herdr 外面操控一个会话。

### 文件放在哪

```
~/.weefleet/
  repos/<repo-key>/
    ledger.jsonl              # 追加式事件账本
    lock                      # 单写锁（flock(2)）
    tasks/
      T-20261026-01/
        brief.md              # 任务合同
        checks.json           # 验收命令
        report.json           # worker 写
        verify.json           # weefleet verify 写
        logs/A1.log
        evidence/
  worktrees/<repo-key>/
    T-20261026-01/            # git worktree
```

任务文件放在仓库外，用绝对路径传给 worker。这样 worktree 里不会混进控制面的文件，也不会被 `git clean` 清掉。

## 3. 三个角色

| 角色 | 在哪 | 能做什么 | 不能做什么 |
|---|---|---|---|
| 人 | 任何 pane | 下目标、回答问题、最终接受/合并 | —— |
| 协调者 Claude | 一个固定 pane | 写 brief、跑 `weefleet` 命令、读 report 摘要、问你 | 改代码、自己宣布完成、自动合并 |
| worker Claude | 每个任务一个 pane | 在自己的 worktree 里改代码、跑测试、写 report | 碰别的 worktree、push main、关别人的 pane |

协调者用一个专用会话启动：`weefleet-claude` 会禁用 `Edit/Write/NotebookEdit`，并在系统提示里要求加载 `weefleet-coordinator` skill。普通 Claude 会话里则用 `/weefleet <需求>` 显式触发。这两种入口都不依赖项目里有没有 `CLAUDE.md`。

项目初始化可以直接用 `/weefleet init`，它等价于 `weefleet init --repo . --write-claude-md` 加一次 `weefleet doctor`。如果还想让没有 `/weefleet` 的普通请求也走 weefleet，`--write-claude-md` 会在项目 `CLAUDE.md` 里追加一个带标记的块；已有块就跳过。新项目没有 `CLAUDE.md` 是正常的，这一层只是兜底。

worker 是 `weefleet dispatch` 另起的进程，不受协调者工具限制。

协调者的约束写进它的 `CLAUDE.md` 或 skill：

```text
你是协调者。你不改代码。
你只能通过 weefleet 命令推进任务；需要人决定时停下来问。
不要根据终端输出判断完成，只根据 weefleet verify 的结果。
```

## 4. 一个任务的完整流程

下面这些命令都由**协调者 Claude 执行**；人只在三个门做决定。协调者把人的原话记进 `--note` / `--decision` / `--reason`，所以“人没敲命令”不等于“没有记录”。

### 第 0 步：人给目标

一句话就够，例如“修复登录接口偶发 500”。协调者把它写成 `brief.md`，你确认。

### 第 1 步：建任务

```bash
weefleet new --repo ~/code/app \
  --goal-file /tmp/goal.md \
  --checks /tmp/checks.json
# 输出 T-20261026-01
```

`weefleet new` 做四件事：

1. 在 `~/.weefleet/repos/<repo-key>/tasks/<id>/` 建目录；
2. 写 `brief.md` 模板（如果没给现成的）；
3. 校验 `checks.json.checks` 里每条命令非空、有 `id`，并检查预算字段合法；
4. 往 `ledger.jsonl` 追加一条 `created`。

这时任务还在 `draft`。把合同摘要给人看；人明确说批准后，把人的原话记下来：

```bash
weefleet approve T-20261026-01 --by "$(git config user.name)" --note "批准，按这个做"
```

批准前 `dispatch` 会拒绝。人不需要自己敲这条命令，协调者代跑并记录原话。

### 第 2 步：派活

```bash
weefleet dispatch T-20261026-01
```

它按顺序做：

```bash
# 1. 记下基线
BASE_SHA=$(git -C "$REPO" rev-parse main)

# 2. 建 worktree（一个任务一个，绝不共用）
git -C "$REPO" worktree add -b "task/$TASK_ID" "$WT" "$BASE_SHA"

# 3. 建一个独立 tab 和 pane，cwd 指向 worktree
OUT=$(herdr tab create \
  --workspace "$HERDR_WORKSPACE_ID" \
  --cwd "$WT" --label "$TASK_ID" --no-focus)
# PANE 取 $OUT 里 result.root_pane.pane_id；weefleet dispatch 已封装这一步

# 4. 起 Claude，等它真的可以输入
NAME="t-${TASK_ID#T-}-impl"        # 例如 t-20261026-01-impl
herdr agent start "$NAME" --kind claude --pane "$PANE"
herdr agent wait "$NAME" --until idle --timeout 60000

# 5. 发任务：只发路径，不发正文
herdr agent prompt "$NAME" \
  "Read $TASK_DIR/brief.md and do the task. Read $TASK_DIR/checks.json too. \
Write $TASK_DIR/report.json. Put evidence in $TASK_DIR/evidence/. \
Your final line must be: REPORT: $TASK_DIR/report.json"
```

这里**不加 `--wait`**。dispatch 只负责把任务送到，送完就返回；否则协调者会被一个几十分钟的命令卡住，人说话它听不见，`blocked` 也处理不了。

启动 worker 时还要把任务目录加进 Claude 的可写范围，否则它可能写不出 `report.json` 和 `evidence/`：

```bash
herdr agent start "$NAME" --kind claude --pane "$PANE" -- --add-dir "$TASK_DIR"
```

这不会自动批准写操作，只是让任务目录成为可写位置；真正的高风险操作仍然走 `blocked` 人类门。

派活时记三样东西进账本：`pane_id`、`agent_name`、`prompt 前的 completion_seq`。最后一样用来防止把上一轮的结束当成这一轮的完成。

### 第 3 步：等它停

dispatch 返回后，用非阻塞的 `weefleet status` 看状态：

```bash
weefleet status
```

需要阻塞等待时，再显式运行（可以放在后台，避免卡住协调者）：

```bash
weefleet wait T-20261026-01 --timeout 1800000
```

`weefleet wait` 调用 `herdr agent wait` 做 server 端等待，不自己 `sleep` 轮询。返回后，再用 `herdr agent list` 按 agent 名读一次最终状态。

最小版的推荐做法：协调者 dispatch 完就回到对话；人偶尔看 `weefleet status` 或 Herdr 的 tab 状态。看到 `blocked` 就处理，看到 `done` 就让协调者 `weefleet verify`。不要让人盯着 worker 的输出。

| Herdr 状态 | 含义 | 我们怎么做 |
|---|---|---|
| `working` | 正在干 | 继续等 |
| `blocked` | 在等你批准/回答 | 记 `blocked`，把 pane 指给人 |
| `idle` / `done` | 停下来等输入 | 看 report 是否存在 |
| `unknown` | 识别不了 | 不判成功，读 `agent get` / `agent explain` |

**`done` 只表示“它停下来了”，不表示“它做对了”。**

如果 report 存在，状态记为 `reported`；如果它停了但没 report，状态记为 `uncertain`（不是失败，也不是成功）。

### 第 4 步：独立验收

```bash
weefleet verify T-20261026-01
```

这一步**不相信 report 里的自述**，只做六件事：

1. `report.json` 存在、能解析、`task_id` 对得上；
2. 解析 HEAD 的 commit trailers，`Task / Attempt / Base / Checks / Report` 和账本、`checks.json` 对得上（格式见第 11 节）；
3. worktree 是干净的（`git status --porcelain` 为空），`report.head_sha` 等于 `git rev-parse HEAD`，且它不等于 `base_sha`；
4. 把 `checks.json.checks` 里每条命令**在 worktree 里重跑一遍**，记录真实 exit code 和日志；
5. 报告里列的每个 `evidence` 文件存在，且在 worktree 或任务目录内，记 SHA-256 和大小；
6. 对比 `git diff --name-only base..HEAD` 和报告的 `changed_files`。

只有“worktree 干净 + 有新 commit + 所有 checks 符合预期”才写 `verified`。否则写 `failed`，并把原因写进 `verify.json`。

`verify` 按 `(task_id, head_sha, sha256(checks.json))` 缓存：同一个 key 已经验过就直接读 `verify.json`，不再跑命令。只有新 commit、checks 改了，或人显式 `--force` 时才重跑。

要求 worker 先 commit 再写 report，是为了让验收面对一个确定的 revision；否则 checks 跑的是未提交的工作区，报告里的 `head_sha` 也没有意义。

范围越界（改了 `scope_paths` 之外的文件）在最小版里只记警告，不阻断。要阻断，等有了 writes policy 再加。

### 第 5 步：人接受

```bash
weefleet accept T-20261026-01
```

命令打印三样东西：report 摘要、`git diff --stat`、verify 结果。你必须明确输入任务 id 或 `--yes`，它才记 `accepted`。

接受之后，合并、开 PR、部署都还是你手动做。最小版**不自动合并**。

### 第 6 步：收尾

```bash
weefleet close T-20261026-01 --keep-branch
```

worker 停下来**不会自动关** tab/pane；Herdr 只改 `agent_status`，不收拾布局。只有 `weefleet close` 会关，而且只关 dispatch 时记录的那个 pane/tab。

关闭 tab/pane，删除 worktree；**分支和任务目录保留**，方便你看 diff、审计或手动合并。只有你显式运行才会删 worktree；任务目录永远不会被 `close` 自动删。

合并之后，运行一次 `weefleet record T-... --merge-commit <sha>`，把精简记录写进仓库；完整记录继续留在 `~/.weefleet`。

## 5. 状态机

```text
draft
  │ weefleet dispatch
  ▼
dispatched
  │ prompt 发出
  ▼
running ──blocked──▶ blocked ──人回答──▶ running
  │
  ├─ 有 report ─────────────▶ reported
  ├─ 停了但没 report ───────▶ uncertain
  ├─ 超时 / stalled ────────▶ uncertain
  └─ pane 没了 ─────────────▶ lost

reported
  │ weefleet verify
  ├─ 全部通过 ──────────────▶ verified
  └─ 有失败 ────────────────▶ failed

verified
  │ weefleet accept（人）
  ▼
accepted
```

`failed` / `uncertain` / `lost` 都**不会自动重试**。人看过现场后运行：

```bash
weefleet retry T-20261026-01 --reason "prompt 超时，已确认未送达"
```

`retry` 只增加 `attempt`，复用原 worktree 或按参数新建；它不会自己再发一遍 prompt。

## 6. 三个文件格式

### brief.md

```markdown
# T-20261026-01

## 目标
修复登录接口偶发 500。

## 只改这些路径
- src/auth/**
- tests/auth/**

## 不要做
- 不 push，不碰 main
- 不安装新依赖
- 不改数据库 schema

## 验收
weefleet verify 会重跑同目录的 checks.json，不要自己改它。

## 交付
- 代码改动（必须 commit，不要留着未提交的改动）
- report.json
- 证据文件放在 evidence/

## 结束时
1. `git add -A && git commit`，message 必须带这些 trailers（最后一段）：
   Task: <任务 id>
   Attempt: <第几次>
   Checks: A1,A2
   Report: report.json
   Base: <base_sha>
   Status: done|blocked|failed
2. 把 commit sha 写进 report 的 `head_sha`
3. 最后一行输出：REPORT: <绝对路径>/report.json
```

worker 不需要知道 trailers 的全部语义；它只要照模板填。解析和校验由 `weefleet verify` 做。

### checks.json

用 JSON 不用 YAML，少一个依赖。

```json
{
  "checks": [
    {"id": "A1", "cmd": "pnpm vitest run tests/auth", "expect_exit": 0, "timeout_s": 120},
    {"id": "A2", "cmd": "node scripts/check-session.mjs", "expect_exit": 0, "timeout_s": 60}
  ],
  "budget": {
    "max_total_seconds": 300,
    "max_runs_per_check": 2,
    "max_output_lines": 80
  }
}
```

验收命令必须是**可重复执行**的。不要写“看日志确认没问题”这种要人判断的条目；要判断的放到人工接受那一步。

这里只放**证明这个任务**的命令，不要放 `pnpm test` 这种全量套件。全量套件属于合并前的 L3，见第 10 节。

### report.json

```json
{
  "task_id": "T-20261026-01",
  "status": "done",
  "summary": "把 refresh token 的空值分支补上了，补了 2 个回归测试",
  "base_sha": "abc123",
  "head_sha": "def456",
  "changed_files": ["src/auth/session.ts", "tests/auth/session.test.ts"],
  "checks": [
    {"id": "A1", "cmd": "pnpm test -- auth", "exit": 0, "log": "evidence/A1.log"}
  ],
  "evidence": ["evidence/A1.log"],
  "open_risks": [],
  "needs_decision": null
}
```

规则：

- `status` 只能是 `done` / `blocked` / `failed`；
- 每条 check 必须指到 `evidence` 里的文件；
- 没有证据的 `done` 会被 `weefleet verify` 忽略，不算通过；
- `base_sha` / `head_sha` 必须和最终 commit、账本一致；
- 最后一行必须是 `REPORT: <路径>`；没有这行，按 `uncertain` 处理。

## 7. 人机交互

只保留三个入口：

| 入口 | 用什么 | 什么时候 |
|---|---|---|
| 下目标 / 改需求 | 和协调者对话 | 任务开始前 |
| 回答 worker 的问题 | 按 `weefleet status` 给的 pane 去回答 | `blocked` 时 |
| 接受结果 | `weefleet accept` | `verified` 后 |

其余时间人不看 worker 的 pane。`weefleet status` 一屏回答两个问题：

```text
T-20261026-01  running   t-20261026-01-impl  w12:p3  12m
T-20261026-02  blocked   t-20261026-02-impl  w13:p1   3m  ← needs you
T-20261025-04  verified  -                     -      ready to accept
```

## 8. `weefleet` 命令

一个 Go 二进制，只用标准库：`flag` 风格的子命令解析 + `os/exec` + `encoding/json` + `syscall.Flock`。skill、slash command、模板用 `//go:embed` 编进二进制，由 `weefleet install` 写出。所有写操作先拿 `~/.weefleet/repos/<repo-key>/lock`。

| 命令 | 作用 |
|---|---|
| `weefleet install` | 复制自身到 bin 目录、建 `weefleet-claude` 软链、写出内嵌的 skill/命令/模板 |
| `weefleet doctor` | 检查 git / herdr / claude / Claude 集成 / skill / 命令 / PATH |
| `weefleet init` | 注册仓库、建运行时目录、记录默认分支 |
| `weefleet new` | 建任务目录、写模板、校验 checks、记 `created` |
| `weefleet approve` | 记录人对 draft 合同的批准；人不需要自己敲命令 |
| `weefleet dispatch` | 建 worktree、开 tab/pane、起 Claude、发 brief；发完返回，不等 |
| `weefleet status` | 从账本 + `herdr agent list` 拼出一屏状态 |
| `weefleet wait` | 用 `herdr agent wait` 等到 `blocked` / `idle` / `done` / 超时；需要阻塞等待时用 |
| `weefleet verify` | 解析 commit trailers 并交叉校验；按 `(task, head_sha, checks hash)` 跑一次 checks；命中缓存就复用；写 `verify.json`。`--force` 才重跑 |
| `weefleet accept` | 人确认后记 `accepted` |
| `weefleet reject` | 人拒绝后记 `rejected`，不自动重试 |
| `weefleet record` | merge 后把精简记录写到 `docs/weefleet/tasks/<id>.md` 并提交 |
| `weefleet resolve` | 记录 `blocked` 的人工决定；`--send` 时把决定转达给 worker |
| `weefleet retry` | 增加 attempt，不自动重发 |
| `weefleet close` | 关 pane、删 worktree、保留分支和任务目录 |
| `weefleet gc` | 列出/清理旧任务目录；默认 dry-run，必须显式确认才删 |

最小实现先做 `doctor / init / new / approve / dispatch / status / wait / verify / accept`；`resolve` 和 `reject` 在第一次遇到 `blocked` 或拒绝前补；`retry`、`record`、`close`、`gc` 可以跑完第一个真实任务后再补。

### 账本事件

```json
{"ts":"2026-10-26T14:00:00+08:00","task":"T-20261026-01","event":"created","data":{}}
{"ts":"...","task":"T-20261026-01","event":"dispatched","data":{"worktree":"...","pane_id":"w12:p3","agent":"t-20261026-01-impl","attempt":1,"base_sha":"abc123"}}
{"ts":"...","task":"T-20261026-01","event":"prompt_sent","data":{"completion_seq":123}}
{"ts":"...","task":"T-20261026-01","event":"settled","data":{"agent_status":"done"}}
{"ts":"...","task":"T-20261026-01","event":"verified","data":{"passed":true}}
{"ts":"...","task":"T-20261026-01","event":"accepted","data":{"by":"zhangyw"}}
```

状态由事件折叠得出，不单独维护。一次只跑一个任务，所以 JSONL + 文件锁足够；等真的需要并行，再换 SQLite。

## 9. 失败和阻塞怎么处理

| 现象 | 判定 | 做法 |
|---|---|---|
| `agent start` 返回 `agent_not_ready` | 启动时就被审批/提问卡住 | 看 pane，回答；不要连续重试 |
| `agent prompt` 返回 `agent_prompt_stalled` | 5 秒内没观察到活动 | 先 `agent get` + `agent read`，确认没送达再决定重发 |
| `agent prompt` timeout | **不代表没送达** | 记 `uncertain`，读现场；绝不盲目重发 |
| `blocked` | 等人 | `weefleet status` 指到 pane，人回答后继续等 |
| `pane_exited` / pane 不见了 | `lost` | 先看 worktree 和 diff，确认副作用，再 `retry` |
| report 缺失 | `uncertain` | 读终端末尾，找 `REPORT:`；没有就按失败/未知处理 |
| checks 失败 | `failed` | 让 worker 修；同一 attempt 内可继续对话，不新建任务。同一 revision 不重复跑，等新 commit |
| 超过测试预算 | `budget_exceeded` | 停下来问人；不自动放宽预算，不继续跑 |
| 第二次 `dispatch` | 拒绝 | 除非显式 `retry`，否则不允许重复派发 |

## 10. 验证成本控制

这一节专门解决“一个小功能跑了很多遍测试”的问题。

### 10.1 先定一条原则

**验证的单位是 revision，不是 turn，也不是阶段。**

一个 commit 对应一次验收结果。只要 commit 没变、checks 没变，任何人都不应该再跑一遍。下游的 review、accept、merge 只读 `verify.json`，不重跑测试。

### 10.2 测试阶梯

| 层 | 谁跑 | 跑什么 | 什么时候 | 允许多跑吗 |
|---|---|---|---|---|
| L0 | worker | 语法、类型、单文件 | 每次编辑后 | 可以，但输出要短 |
| L1 | worker | 任务相关的测试文件 | 提交前 | 可以，属于 worker 自己的反馈循环 |
| L2 | `weefleet verify` | `checks.json` 里的验收命令 | 每个新 `head_sha` 一次 | 不允许重复 |
| L3 | 人 / CI / 集成任务 | 全量测试、集成测试 | 合并前或每批一次 | 不属于单任务 |

关键不是“少测试”，而是**把测试放在正确的层**：

- L0/L1 是 worker 的快速反馈，跑多少次由它自己控制；
- L2 是唯一验收，只跑一次；
- L3 是合并前的批次门，不能塞进每个任务的 `checks.json`。

`checks.json` 里只写证明这个任务的命令，例如：

```json
{
  "checks": [
    {"id": "A1", "cmd": "pnpm vitest run tests/auth", "expect_exit": 0, "timeout_s": 120}
  ],
  "budget": {
    "max_total_seconds": 300,
    "max_runs_per_check": 2,
    "max_output_lines": 80
  }
}
```

不要写 `pnpm test` 这种全量命令；全量套件放到 L3。

### 10.3 `verify` 按 revision 缓存

`checks.json` 在创建任务时记一个 SHA-256。`verify` 每次都先比对；worker 改过验收命令就直接拒绝，不跑 checks。要改验收，重新建任务，不要在原任务上放宽。

`verify.json` 的缓存 key 是：

```text
(task_id, head_sha, sha256(checks.json))
```

行为：

| 情况 | 行为 |
|---|---|
| key 相同，上次通过 | 直接复用，不再跑 |
| key 相同，上次失败 | 直接复用失败，不再跑；等新 commit |
| 新 commit | 新 key，跑一次 |
| 人工要求重跑 | `weefleet verify --force --reason "..."`，记录原因 |
| checks.json 改了 | 新 key，跑一次 |

下游阶段一律读 `verify.json`：

- reviewer 只读 `verify.json` + diff，不跑测试；
- `weefleet accept` 只读 `verify.json`，不跑测试；
- 合并前如果跑 L3，那是另一条记录，不复用 L2 的结果，也不重复 L2 的命令。

### 10.4 输出只回摘要，日志落盘

每次跑 check，完整输出写到：

```text
~/.weefleet/repos/<repo>/tasks/<id>/logs/<check-id>.log
```

回给模型和命令行的只有：

- exit code；
- 失败用例名（能提取就提取）；
- 最后 `max_output_lines` 行；
- 完整日志路径。

协调者永远不读完整日志。worker 自己要看可以读文件，但不要把它贴回上下文。

### 10.5 预算超了就停

每个任务有硬预算：

- 单个 check 最多跑 `max_runs_per_check` 次；
- 所有 check 累计最多 `max_total_seconds` 秒；
- 超过任意一条，状态记为 `budget_exceeded`，停下来问人。

不自动继续，也不自动放宽预算。预算由写 `checks.json` 的人定，不由 worker 定。

### 10.6 账本记录每次运行

每次 check 运行都追加一条：

```json
{"ts":"...","task":"T-...","event":"check_run","data":{"check":"A1","head_sha":"def456","exit":0,"duration_s":12.4,"output_bytes":1832,"cached":false}}
```

`weefleet status` 显示每个任务的累计测试时间和运行次数。超预算的任务标红。这样“测试跑太多”是可见的，不靠事后回忆。

### 10.7 谁可以要求重跑

| 角色 | 权限 |
|---|---|
| worker | 可以反复跑 L0/L1；L2 由 `weefleet verify` 跑 |
| verifier | 每个 revision 跑一次 L2 |
| reviewer（以后） | 只读，不跑测试；要重跑必须向 verifier 提理由，并记录 |
| 人 | `--force` 重跑，必须给 reason |
| 合并门 | 只跑 L3，不重跑 L2 |

一句话：**L2 的结果一旦产生，就绑定在这个 commit 上，后面的人只能引用，不能重算。**

### 10.8 这套规则能避免什么

- 不会出现 worker、协调者、reviewer、finisher 各跑一遍同一套测试；
- 不会因为 agent 反复 `done` 就反复触发验证；
- 不会把全量套件塞进每个小任务；
- 不会让几千行测试日志反复进入模型上下文；
- 同一个 commit 的第二次 `weefleet verify` 是零成本。

它不能避免的是：worker 自己在 L0/L1 反复跑。那部分属于它的工作循环，只能靠 brief 里的指令和它自己的预算约束，不能靠控制面完全消灭。

## 11. 用 git 承载代码事实和交接

git 已经在记录 revision、diff、作者、时间、分支和 worktree。不要再把这些抄进账本；账本只记 git 不适合记的控制事件。

### 11.1 三样东西各管什么

| 事实 | 存在哪 | 为什么 |
|---|---|---|
| 代码 revision、diff、改了哪些文件 | git | 天然、可复现，比 report 里的自述可靠 |
| 任务 id、attempt、checks 覆盖、报告路径 | 最终 commit 的 trailers | 跟 revision 绑定，能机器解析，能随分支走 |
| 验证结果、接受记录 | `git notes`（或 `verify.json`） | 绑到具体 commit，不改 SHA |
| dispatch / blocked / wait / timeout | `ledger.jsonl` | 不是代码事实，git 不适合 |
| 人给 worker 的指令 | `brief.md` | commit message 是记录，不是命令通道 |

### 11.2 最终 commit 的固定格式

worker 提交时，最后一个 commit 的 message 必须带这些 trailers：

```text
fix(auth): handle null refresh token

把 refresh token 的空值分支补上了，补了两个回归测试。
没有改数据库 schema。

Task: T-20261026-01
Attempt: 1
Checks: A1,A2
Report: report.json
Base: abc123
Status: done
```

规则：

- trailers 必须在**最后一段**，每行 `Key: value`；
- `Task` 必须和分支、账本对得上；
- `Attempt` 必须和账本里的 attempt 对得上；
- `Checks` 必须覆盖 `checks.json.checks` 里的所有 id，少一个就拒绝；
- `Report` 是相对 task 目录的路径（例如 `report.json`），文件必须存在；用 `Task` 定位 task 目录，不要写机器相关的绝对路径；
- `Base` 必须等于派活时记录的 `base_sha`；
- `Status` 只能是 `done` / `blocked` / `failed`。

用 `git interpret-trailers --parse HEAD` 解析，不要自己写正则。

### 11.3 `weefleet verify` 怎么用这些信息

`verify` 先解析 HEAD 的 trailers，再和 report、账本、checks 交叉校验：

| 校验 | 不一致时 |
|---|---|
| `Task` = 账本 task id | 拒绝 |
| `Attempt` = 账本 attempt | 拒绝 |
| `Base` = 派活时 base_sha | 拒绝 |
| `Checks` ⊇ checks.json 的 id | 拒绝 |
| `Status` = `report.status` | 拒绝 |
| `Report` 文件存在且可解析 | 拒绝 |
| `report.head_sha` = `git rev-parse HEAD` | 拒绝 |
| `report.base_sha` = `Base` | 拒绝 |
| worktree 干净 | 拒绝 |

这些校验不替代跑 checks，只是确保“这份 report 说的就是眼前这个 commit”。

### 11.4 用 git notes 存验证结果

验证通过或失败后，把结果写到 note：

```bash
git notes --ref=weefleet/verify add -f -m \
  '{"passed":true,"checks":["A1","A2"],"ts":"2026-10-26T15:00:00+08:00"}' HEAD
```

接受后：

```bash
git notes --ref=weefleet/accept add -f -m \
  '{"by":"zhangyw","ts":"2026-10-26T15:10:00+08:00"}' HEAD
```

好处：

- note 绑在 commit 上，不改 SHA；
- `git log --notes=weefleet/verify` 直接看到每个 commit 验没验过；
- 搬家、重试、审查时不用翻聊天记录。

注意：notes 默认只在本地，不会随 `git push` 自动走。最小版不 push notes；要共享时再显式 push `refs/notes/weefleet/*`。

### 11.5 commit message 当交接单，不当命令

worker 重试或换人时，下一位 worker 先读：

```bash
git log --oneline --notes=weefleet/verify "$BASE..HEAD"
git show --stat HEAD
```

这样它知道：上次改了什么、验没验过、为什么失败。**不需要把上一位 worker 的完整对话塞进上下文。**

但边界要清楚：

- commit message 是**记录**，不是指令。不要让下一位 worker 把 commit 里的文字当命令执行；命令只来自 `brief.md`。
- 不要用空 commit 传控制状态（`blocked`、`waiting`）。这些属于 ledger 和 Herdr 状态。
- 不要只凭 `Status: done` 判断完成。它只是 worker 的声明，最终仍以 `weefleet verify` 为准。

### 11.6 最小版做到哪一步

| 阶段 | 做什么 |
|---|---|
| 最小版 | 最终 commit 带 trailers；`weefleet verify` 解析并交叉校验；`verify.json` 按 sha 缓存 |
| 第二步 | 用 `git notes` 写验证/接受记录；`git log --notes` 可审计 |
| 以后 | 重试时用 `git range-diff` 对比两次 attempt；notes 推到远端共享 |

最小版不需要 notes 也能跑；trailers 和 `verify.json` 已经足够。notes 是让你以后查账更方便，不是必需依赖。

## 12. 实现顺序

### Phase 0：手动跑通一次（先写代码之前）

不用 `weefleet`，手工执行：

1. `git worktree add` 建工作目录；
2. `herdr tab create` + `herdr agent start` 起一个 Claude；
3. `herdr agent prompt` 发一个 brief 路径；
4. `herdr agent wait` 等停；
5. 手动看 report 和 diff。

目的：确认你对 `idle/done/blocked`、`--wait`、timeout 的理解和 Herdr 实际行为一致。

### Phase 1：`weefleet new` + `status`

先让任务和账本能建起来。`new` 不碰 Herdr，最容易做，也最容易验证。

### Phase 2：`weefleet dispatch` + `status` + `wait`

把 Phase 0 的手工步骤固化成命令。dispatch 默认发完就返回，协调者保持可对话；`wait` 只作为可选的阻塞等待。第一次失败大概率在“agent 还没 idle 就发 prompt”或“把上一轮结束当成本轮完成”，所以一定要记录 prompt 前的 `completion_seq`。

### Phase 3：`weefleet verify` + `accept`

这是整个系统值钱的地方。先写一个**故意造假的 report**（声称通过但 checks 实际失败），确认 `verify` 会拒绝。

### Phase 4：跑三个真实任务

连续跑三个小任务，记录每次卡在哪。只有出现下面这些信号，才加下一层：

| 信号 | 再考虑加 |
|---|---|
| 任务之间要并行 | 任务队列、并发槽 |
| worker 经常虚报完成 | 独立 reviewer agent |
| 任务多到记不住 | board / cockpit |
| 要跨机器 | 容量表、远程 dispatch |
| worker 越界改文件 | writes policy / 沙箱 |
| 经常被 `blocked` 卡很久 | 通知桥（Telegram/Slack） |

在那之前，**不要加**。

## 13. 最小系统自己的验收标准

满足这九条，就算最小环境跑通：

1. 给一个 5 行的目标，能建出任务和 checks；
2. `dispatch` 能建 worktree、起一个可见的 Claude pane；
3. worker 写 report 后，`weefleet status` 能看到 `reported`；
4. 伪造一份“通过”的 report，`weefleet verify` 必须拒绝；
5. 超时后状态是 `uncertain`，不会自动重发；
6. `accept` 必须由人显式执行；没有自动合并；
7. 同一个 commit 第二次 `verify` 复用结果，不再跑测试；
8. 超过预算时状态是 `budget_exceeded`，不会继续；
9. 最终 commit 缺 trailers，或 `Checks` 少覆盖一条，`verify` 必须拒绝。

另外两条底线：

- 全程不需要常驻进程；
- 二进制不引入第三方 Go 依赖。

## 14. 明确不做

最小版不做：多 worker 并行、多机、自动合并、自动开 PR、reviewer agent、模型路由、配额、沙箱、Web UI、MCP、事件订阅守护进程、长期记忆。

这些不是不重要，而是**现在做会让闭环跑不通**。先把“一个任务从目标到可验证的 diff”跑顺。

## 15. 参考实现，按顺序读

| 项目 | 先读什么 | 学什么 |
|---|---|---|
| [chetanunadkat-lang/herdr-fleet](https://github.com/chetanunadkat-lang/herdr-fleet) | README | orchestrator/worker 分工、brief 文件、worktree 隔离、收据到手才关 pane |
| [jmcjm/Hivemind](https://github.com/jmcjm/Hivemind) | README 的 “Why this way and not another” | coordinator + drone、文件式 mail、不轮询、failed prompt 的 attempts |
| [Casper5253/herdr-orchestration](https://github.com/Casper5253/herdr-orchestration) | `docs/ARCHITECTURE.md` | 确定性路由、checkpoint、compact 前后 hook（Phase 4 以后再看） |
| [eliasstravik/herdr-projects](https://github.com/eliasstravik/herdr-projects) | README + `docs/get-started.md` | board、Coordinator、第二 agent 审查、合并门（以后再看） |

先读前两个就够了。后两个是给你判断“以后要不要加”的，不是现在就抄。

## 16. 已知限制

- `done` 不是成功，`unknown` 不是完成；系统只认 `weefleet verify`。
- 终端输出不可靠（alternate screen、软换行）；report 文件才是事实源。
- 最小版对越界改动只警告，不阻断。
- 没有沙箱；worker 的权限靠 Claude 的权限模式和 prompt 约束。
- 一次只跑一个任务；`blocked` 时需要人自己去 pane 回答。
- Claude 升级后要重跑 `herdr integration install claude`，否则状态可能退化成 `unknown`。
- Herdr 升级后要重启 server；client/server 版本不一致会报 `protocol_mismatch`。

## 17. 合并后：brief / checks 怎么留

### 17.1 会不会丢

`weefleet close` 只做三件事：关 pane、关 tab、删 worktree。**它不删任务目录。**

所以本机上不会丢：

```text
~/.weefleet/repos/<repo-key>/tasks/T-20261026-01/
  brief.md
  checks.json
  report.json
  verify.json
  logs/
  evidence/
```

但任务目录不在 git 里。换机器、清 `~/.weefleet`、或者别人 clone 仓库，都看不到它。合并后的代码历史里也只有 commit trailers，没有 brief 和 checks 的正文。

结论：**本地全量记录 + 仓库内精简记录，两层。**

### 17.2 两层记录

| 层 | 位置 | 内容 | 生命周期 |
|---|---|---|---|
| 全量 | `~/.weefleet/repos/<repo>/tasks/<id>/` | brief、checks、report、verify、日志、证据 | `close` 不删；手动 `gc` 才清 |
| 精简 | `docs/weefleet/tasks/<id>.md` | 目标、范围、checks、verify 摘要、base/head/merge sha、人的决定 | 跟代码走，永久 |

日志和证据不进仓库：体积大、可能含敏感信息、 review 时也没人看。它们留在本地或 CI artifact 里。

### 17.3 `weefleet close` 的语义

```bash
# 合并前：保留分支和任务目录，只删 worktree
weefleet close T-20261026-01 --keep-branch

# 合并后：连分支一起删，任务目录仍然保留
weefleet close T-20261026-01 --delete-branch
```

- 永远不自动删任务目录；
- ledger 的 `closed` 事件记录 `branch`、`head_sha`、`merge_commit`（如果已知）；
- 如果 close 时还不知道 merge commit，就记 `null`，等 `weefleet record` 补。

### 17.4 `weefleet record`：合并后写精简记录

```bash
weefleet record T-20261026-01 --merge-commit 789abc
```

它在主工作区写一个文件并提交：

```text
docs/weefleet/tasks/T-20261026-01.md
```

内容模板：

```markdown
# T-20261026-01 修复登录偶发 500

- 状态：accepted
- 分支：task/T-20261026-01
- Base：abc123
- Head：def456
- Merge：789abc
- 接受人：zhangyw
- 接受时间：2026-10-26T15:10:00+08:00

## 合同
目标：修复登录接口偶发 500。
范围：src/auth/**、tests/auth/**。
不做：不改 schema、不装依赖、不 push。

## 验收
| id | 命令 | 结果 | 耗时 |
|---|---|---|---|
| A1 | pnpm vitest run tests/auth | exit 0 | 12s |
| A2 | node scripts/check-session.mjs | exit 0 | 3s |

## 结果
- verify：passed
- open risks：无
- 证据摘要：evidence/A1.log sha256:...

## 本地完整记录
~/.weefleet/repos/app/tasks/T-20261026-01/
```

这个 commit 的 message 也带 `Task:` trailer：

```text
docs: record T-20261026-01

Task: T-20261026-01
Merge: 789abc
```

它是在 merge 之后对主分支做的文档提交，**不改变已经验证过的任务 revision**，所以不需要重新跑 checks。

如果团队不想在仓库里放生成文档，可以退一步：把同样内容写进 PR 描述。PR 也是持久的，但不可 grep、不在本地 checkout 里。最小版建议先写文件。

### 17.5 路径要稳定，别把绝对路径写进 commit

commit trailer 里的 `Report` 用相对 task 目录的路径：

```text
Report: report.json
```

不要写：

```text
Report: /Users/zhangyw/.weefleet/repos/app/tasks/T-20261026-01/report.json
```

原因：

- 换机器路径就失效；
- 任务目录归档后路径会变；
- `Task:` 已经是稳定定位符，`Report` 只需要在 task 目录内相对定位。

`report.json` 里的证据路径也保持相对：`evidence/A1.log`。绝对路径只出现在本地 ledger 和日志里，不进入 commit。

### 17.6 合并方式对 trailers 的影响

| 合并方式 | trailers 会怎样 | 建议 |
|---|---|---|
| merge / rebase | 分支上的 commit 保留 trailers | merge commit 的 message 也加 `Task:`，方便在主干上搜到 |
| squash | 默认丢掉分支 commit 的 trailers | squash message 必须手写 `Task:` / `Checks:`，或用 `weefleet record` 生成 |

不要只靠分支名。分支合并后通常会删，`task/T-...` 不是持久记录。持久链路是：

```text
merged commit 的 Task trailer  →  docs/weefleet/tasks/<id>.md  →  本地完整记录
```

### 17.7 一个功能包含多个任务

最小版一个任务一个记录文件。以后一个功能拆成多个任务时，在 brief 里加 `feature: auth-rework`，commit 加 `Feature: auth-rework` trailer；`weefleet record` 可以额外维护一个 `docs/weefleet/features/<feature-id>.md` 索引，链到各个任务记录。

现在不做。先把一个任务一个记录跑顺。

### 17.8 清理策略

- 默认不自动清理；
- `weefleet gc --older-than 90d --dry-run` 只列出候选；
- `weefleet gc --older-than 90d --delete` 才删，而且只删 `accepted` / `rejected` / `closed` 的任务；
- 删除顺序：先删 `logs/` 和 `evidence/`，保留 `brief.md` / `checks.json` / `report.json` / `verify.json`；
- 如果仓库里的 `docs/weefleet/tasks/<id>.md` 不存在，拒绝删除，除非显式 `--force`。

一句话：**本地全量记录保留，仓库精简记录跟代码走，合并后跑一次 `weefleet record`。**

### 17.9 tab / pane 什么时候关

**不会自动关。**

worker 停下来只改变 `agent_status`（`working → idle/done`）；Herdr 不会因为 agent 结束就关闭 tab 或 pane。pane 会一直留着，直到有人显式关闭。

只有 `weefleet close` 会关，而且只关 dispatch 时记录的那个 pane/tab：

1. 状态必须是 `accepted` / `rejected` / `closed`；否则拒绝，除非 `--force`；
2. 按记录的 `pane_id` 关，不按 agent 名、不按侧栏顺序；
3. 如果这个 tab 已经没有别的 pane，再按 `tab_id` 关 tab；如果人在里面加过 pane，保留 tab；
4. `git worktree remove`；有未提交改动就拒绝，`--force` 才强删；
5. 分支和任务目录保留。

协调者 tab 不是 `weefleet` 创建的，**永远不关**。工作区也不关；`workspace close --group` 会连带关掉 worktree 工作区，不在最小版里用。

为什么不在 `done` 或 `verify` 通过时自动关：

- `done` 不是 accepted；
- verify 通过后，人可能还要看 diff、证据，或者让同一个 worker 继续修；
- 关闭是破坏性动作；Herdr 自己的规则也是“不要关不是你创建的 pane”。

以后可以加 `weefleet accept --close` 作为可选行为，最小版不做。
