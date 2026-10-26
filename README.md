# weefleet

一个“用 Agent 舰队执行开发任务”的设计与实践仓库。

> 旧版的 `fleet` / `/fleet` / `~/.fleet` 已统一改名为 `weefleet` / `/weefleet` / `~/.weefleet`。

## 内容

- [`references/`](references/)：开源项目的机制笔记。
- [`references/weefleet-design.md`](references/weefleet-design.md)：基于 Herdr + Claude 的最小舰队设计。
- [`cmd/weefleet`](cmd/weefleet)：该设计的实现入口。单个 Go 二进制，skill、slash command 和模板用 `//go:embed` 编进二进制。
- [`internal/app`](internal/app)：命令、账本、git、Herdr、验收和安装逻辑。
- [`internal/assets/files`](internal/assets/files)：运行时要写出的 skill、命令、模板和协调者系统提示。
- [`docs/weefleet-quickstart.md`](docs/weefleet-quickstart.md)：安装、第一个任务、人类门、已知限制。

## 快速开始

新机器：

```bash
chmod +x weefleet      # 下载对应平台（macOS/Linux，arm64/amd64）的 release 二进制
./weefleet install     # 二进制、weefleet-claude、skill、/weefleet 命令、模板
```

在项目目录的 Claude 会话里：

```text
/weefleet init                   # 初始化项目，默认写 CLAUDE.md 块
/weefleet 修复登录接口偶发 500    # 走协调者流程
```

或者在 Herdr 里启动受限的协调者会话：

```bash
weefleet-claude
```

三层入口保证：`weefleet-claude`（会话级限制）、`/weefleet`（显式命令）、项目 `CLAUDE.md`（普通请求兜底）。

从源码构建需要 Go 1.24+：

```bash
make build
./bin/weefleet install
```

完整流程见 [`docs/weefleet-quickstart.md`](docs/weefleet-quickstart.md)。

## 测试

```bash
go test ./...
```

集成测试使用 `internal/app/testdata/fakeherdr`，不需要真实的 Herdr。
