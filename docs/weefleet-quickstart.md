# weefleet 快速开始

`weefleet` 是设计文档 [`references/weefleet-design.md`](../references/weefleet-design.md) 的最小实现：一个 Go 二进制，skill、slash command 和模板都编进二进制里。

一句话：**一个任务 = 一个 worktree + 一个 Claude pane + 一份 brief + 一份 report + 一次独立复验。**

## 新机器，3 步

```bash
# 1. 下载对应平台的 weefleet 二进制（macOS/Linux，arm64/amd64），重命名为 weefleet
chmod +x weefleet

# 2. 装入口：二进制、weefleet-claude、skill、/weefleet 命令、模板
./weefleet install

# 3. 到项目里，在 Claude 会话输入
/weefleet init
```

`weefleet install` 只装 weefleet 自己的东西，不装 Herdr 和 Claude Code。前提仍然是：Herdr ≥ 0.9.3、Claude Code、git。

`/weefleet init` 等价于：

```bash
cd ~/code/new-project
weefleet init --repo .
weefleet doctor
```

`weefleet init` 会自动识别 main/master，并在项目的 `CLAUDE.md` 里追加一个带 `<!-- weefleet:begin -->` 标记的块；已有块就跳过，不覆盖你原来的内容。不想要这个块就用 `weefleet init --repo . --no-claude-md`（或 `/weefleet init --no-claude-md`）。

要求项目至少有一个 commit。

## 从源码构建

需要 Go 1.24+：

```bash
git clone <weefleet-repo-url> ~/weefleet
cd ~/weefleet
make build          # -> bin/weefleet
./bin/weefleet install
```

其他目标：

```bash
make test           # go test ./...
make dist           # -> dist/weefleet-<version>-<os>-<arch>
make install        # 构建后执行 bin/weefleet install
```

## `weefleet install` 做了什么

| 动作 | 默认位置 |
|---|---|
| 复制自身 | `~/.local/bin/weefleet` |
| 建启动器软链 | `~/.local/bin/weefleet-claude -> weefleet` |
| 写协调者 skill | `~/.claude/skills/weefleet-coordinator/SKILL.md` |
| 写 slash command | `~/.claude/commands/weefleet.md` |
| 写模板 | `~/.weefleet/templates/{brief.md,checks.json,commit.txt}` |
| 写安装清单 | `~/.weefleet/installed.json` |

- 幂等：重复运行只会更新内容有变化的文件。
- `--dry-run`：只打印将要做什么。
- `--force`：覆盖同名但不是 weefleet 写的文件。
- 目录可用 `WEEFLEET_BIN_DIR`、`WEEFLEET_SKILL_DIR`、`WEEFLEET_COMMAND_DIR` 或 `--bin-dir` / `--skill-dir` / `--command-dir` 改。
- 不会覆盖一个不是 weefleet 的现有二进制；不会覆盖非软链的 `weefleet-claude`。
- 最后运行 `weefleet doctor` 自检；外部前提缺失只报告，不影响安装本身。

如果 `~/.local/bin` 不在 `PATH` 里，安装会提示；加进 shell 配置后重开终端。

### 配置（可选）

`~/.weefleet/config.json` 可以改默认行为；缺字段就用默认值：

```json
{
  "worktree_root": null,
  "agent_kind": "claude",
  "ready_timeout_ms": 60000,
  "wait_timeout_s": 1800,
  "budget": {
    "max_total_seconds": 300,
    "max_runs_per_check": 2,
    "max_output_lines": 80
  }
}
```

- `worktree_root`：worktree 根目录；`null` 表示 `~/.weefleet/worktrees`。
- `agent_kind`：`herdr agent start --kind` 的值，默认 `claude`。
- `ready_timeout_ms`：dispatch 等 agent 变为 idle 的超时。
- `wait_timeout_s`：`weefleet wait` 的默认超时。
- `budget`：`checks.json` 没写 budget 时的默认预算；`checks.json` 里显式写了的字段优先。

`~/.weefleet/installed.json` 记录上一次 `weefleet install` 的版本和路径，不要手改。

### `/weefleet init` 之外的手动初始化

```bash
cd ~/code/app
weefleet init --repo .
weefleet doctor
```

它会创建：

```text
~/.weefleet/
  config.json
  installed.json
  templates/
  repos/<repo-key>/
    repo.json
    ledger.jsonl
    lock
    tasks/
```

`weefleet init` 只做一次。之后所有命令都可以在这个仓库里运行。

## 启动协调者

推荐用 `weefleet-claude`：

```bash
herdr
# 在项目目录开一个 pane
weefleet-claude
```

它会做两件事：

- 禁用 `Edit` / `Write` / `NotebookEdit`，让协调者不能直接改代码；
- 在系统提示里要求它加载 `weefleet-coordinator` skill，并在三个门停下来问人。

worker 是 `weefleet dispatch` 另起的 Claude 进程，不受这些限制。

如果直接用 `claude`，就在会话里用显式入口：

```text
/weefleet 修复登录接口偶发 500，只改 auth
```

`/weefleet` 会把后面的需求交给协调者流程。

为了让**没有加 `/weefleet` 的普通请求**也走 weefleet，项目 `CLAUDE.md` 里要有那段块；`weefleet init` 默认会加。

三层保证：

| 层 | 依赖项目文件吗 | 作用 |
|---|---|---|
| `weefleet-claude` | 不依赖 | 会话级工具限制 + 系统提示，默认就是协调者 |
| `/weefleet init` | 不依赖 | 一个命令完成项目初始化 + doctor |
| `/weefleet <需求>` | 不依赖 | 显式入口，把这一条需求交给 weefleet |
| 项目 `CLAUDE.md` | 依赖 | 兜底：普通开发请求也先走 weefleet；`/weefleet init` 会自动加 |

## 一个任务的流程

### 1. 写合同并建任务

```bash
# 协调者先建 draft，把合同摘要给人看
weefleet new --repo . \
  --goal-file /tmp/goal.md \
  --checks /tmp/checks.json \
  --scope 'src/**'

# 人明确说“批准”后，记录人的原话
weefleet approve T-20261026-01 \
  --by "$(git config user.name)" \
  --note "批准，按这个做"
```

验收命令只写证明这个任务的命令，不要写 `pnpm test` 这种全量套件；全量套件属于合并前的 L3。模板见 `~/.weefleet/templates/checks.json`。

### 2. 派活

```bash
weefleet dispatch T-20261026-01
weefleet status
```

`dispatch` 会：

1. 建 worktree（一个任务一个）；
2. 建一个 tab 和一个 pane；
3. 起一个 Claude Code；
4. 等它 idle；
5. 发 brief 路径和 commit trailer 模板。

命令发完就返回，不会卡住协调者。

### 3. 看状态

```bash
weefleet status
```

一屏看到：

```text
T-20261026-01  running   t-20261026-01-impl  w12:p3  12m
```

`blocked` 时，人会看到 pane id。人进入那个 pane 回答问题，然后记录决定：

```bash
weefleet resolve T-20261026-01 \
  --decision '不改 schema，用兼容写法' \
  --send --by "$(git config user.name)"
```

如果 agent 还卡在审批/提问 UI 上，`--send` 不会生效；请人直接在 pane 里回答，`weefleet resolve` 只负责记录。

### 4. 等它停下

需要阻塞等待时：

```bash
weefleet wait T-20261026-01 --timeout 1800
```

它会调用 `herdr agent wait`，不自己轮询。

### 5. 验收

worker 写完 report 后：

```bash
weefleet verify T-20261026-01
```

它会：

- 解析最终 commit 的 trailers（`Task/Attempt/Checks/Report/Base/Status`）；
- 检查 worktree 干净、`head_sha` 是新 commit；
- 在 worktree 里重跑 `checks.json`；
- 记录证据哈希；
- 按 `(task, head_sha, checks hash)` 缓存。

第二次 `verify` 同一个 commit 会直接读缓存，**不重跑测试**。

### 6. 人接受

```bash
weefleet accept T-20261026-01 --yes --by "$(git config user.name)"
```

没有 `--yes` 会拒绝；这是第三道人类门。接受后不自动合并，合并由人手动做。

### 7. 合并后

```bash
# 手动 merge 之后
weefleet record T-20261026-01 --merge-commit <merge-sha>
weefleet close T-20261026-01 --delete-branch
```

- `record` 写 `docs/weefleet/tasks/T-20261026-01.md` 并提交；
- `close` 关 pane/tab、删 worktree、删分支；
- 任务目录 `~/.weefleet/repos/<key>/tasks/T-.../` **保留**。

## 三个门

| 门 | 命令 | 谁执行命令 | 谁做决定 |
|---|---|---|---|
| 合同 | `weefleet approve ... --by ... --note "<原话>"` | 协调者 | 人 |
| 阻塞 | `weefleet resolve ... --decision "<原话>" --by ...` | 协调者 | 人 |
| 验收 | `weefleet accept ... --yes --note "<原话>"` / `weefleet reject ... --reason ...` | 协调者 | 人 |

**人只做决定，不需要敲这些命令。** 协调者负责执行，并在 `--note` / `--reason` / `--decision` 里记录人的原话。协调者可以提议、总结、转达，但不能替人决定。

## 状态和文件

```text
~/.weefleet/repos/<repo-key>/
  ledger.jsonl                 # 追加式事件
  tasks/T-20261026-01/
    brief.md
    checks.json
    report.json                # worker 写
    verify.json                # weefleet verify 写
    logs/A1.log                # 完整测试输出
    evidence/                  # worker 的证据
```

`weefleet status` 从 ledger 折叠状态，加上 `herdr agent list` 的实时状态。

## 运行测试

```bash
go test ./...
```

不需要真实的 Herdr：集成测试会编译 `internal/app/testdata/fakeherdr` 作为假的 `herdr`，覆盖建任务 → dispatch → report → verify → 缓存 → accept → record → close → gc，以及预算超限、checks 篡改和缺失 trailers 的拒绝。

## 从旧名字迁移

如果之前用的是 `fleet`：

| 旧 | 新 |
|---|---|
| `fleet` | `weefleet` |
| `fleet-claude` | `weefleet-claude` |
| `/fleet` | `/weefleet` |
| `fleet-coordinator` skill | `weefleet-coordinator` |
| `~/.fleet` | `~/.weefleet` |

`weefleet install` 会替换旧安装脚本留下的 `weefleet` 软链，并刷新自己和 skill/命令文件；它不会自动删除旧名字，只会提示。旧数据目录也不会自动搬。

如果旧版的 `~/.fleet` 里真的有 `repos/` 或 `config.json`，并且 `~/.weefleet` 不存在：

```bash
# 需要保留旧任务时，且仓库绝对路径不变
mv ~/.fleet ~/.weefleet
```

如果 `~/.fleet` 是别的工具留下的目录，不要动它；weefleet 不会读它。

项目 `CLAUDE.md` 里的旧 `<!-- fleet:begin -->` 块，会在下次 `weefleet init` 时原地换成新块。

## 换了一台电脑

最简单：**不搬 `~/.weefleet`，只搬仓库和 weefleet 二进制。**

```bash
# 1. 装前提：Herdr、Claude Code、git
herdr integration install claude

# 2. 下载 weefleet 二进制，装入口
./weefleet install

# 3. 到项目里初始化
cd ~/code/app
weefleet init --repo .
# 或在 Claude 里：/weefleet init

# 4. 开始用
weefleet-claude
# 或普通 claude 会话里：/weefleet 修复登录接口偶发 500
```

不要把旧电脑的 `~/.local/bin/weefleet` 拷过来；重新下载对应平台的二进制再 `install`。`~/.claude/skills/weefleet-coordinator` 和 `~/.claude/commands/weefleet.md` 也由 `install` 重新写出，不需要手工搬。

### 旧任务要不要搬

| 想保留什么 | 怎么做 |
|---|---|
| 已合并任务的历史记录 | 不用搬。`docs/weefleet/tasks/<id>.md` 和 commit trailers 跟仓库走 |
| 未完成任务、完整 brief/checks/report | 搬 `~/.weefleet/repos/<repo-key>/`，但**不要搬 `~/.weefleet/worktrees/`** |

搬 `~/.weefleet` 有一个前提：新旧电脑上的仓库**绝对路径相同**。`repo-key` 是仓库绝对路径的 SHA-1 前 8 位；路径变了，key 就变了。路径不同时，最省事的做法是：

1. 新电脑上重新 `weefleet init`；
2. 旧任务的结论从仓库的 `docs/weefleet/tasks/` 看；
3. 未完成的任务重新建一次，不要手工搬 ledger。

## 已知限制

- 一次只跑一个任务；没有并行队列。
- `close` 只关最新一次 dispatch 的 pane/tab；重试留下的旧 pane 需要手动关。
- `checks.json` 在创建时记哈希；worker 改了会被 `verify` 拒绝。要改验收，重新建任务。
- 范围越界只记 warning，不阻断（没有 writes policy / 沙箱）。
- 预算按任务累计，不按 revision 清零；超了用 `--ignore-budget` 显式覆盖。
- `--send` 不能替人回答 `blocked` UI；人必须在 pane 里回答。
- `record` 在主分支上写一个 docs commit；如果主分支受保护，改成写 PR 描述或换分支。
- 只支持 macOS / Linux（用到 `flock(2)` 和 Unix 信号语义）。
- 没有 reviewer agent、没有自动合并、没有多机、没有 Web UI。
