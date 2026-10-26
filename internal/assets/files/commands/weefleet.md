---
description: weefleet 入口：/weefleet init 初始化项目；/weefleet <需求> 走协调者流程
argument-hint: init | <需求描述>
---

你收到的是 `/weefleet` 的参数：

$ARGUMENTS

先判断模式。

## 模式 A：init

如果参数去掉首尾空格后等于 `init` 或 `init --no-claude-md`，只做项目初始化，不要进入任务流程：

1. 确认当前目录是 git 仓库：`git rev-parse --show-toplevel`。不是就停下来，告诉人先 `git init` 并提交一次。
2. 确认 `weefleet` 在 PATH：`command -v weefleet`。没有就让人类先下载 weefleet 二进制并运行 `weefleet install`，然后停止。
3. 运行：

   ```bash
   # 参数是 init 时：默认写 CLAUDE.md
   weefleet init --repo .

   # 参数是 init --no-claude-md 时：不碰 CLAUDE.md
   weefleet init --repo . --no-claude-md
   ```

   它会自动识别 main/master，并在 `~/.weefleet` 建账本；默认还会追加 weefleet 的 `CLAUDE.md` 块。

4. 运行 `weefleet doctor`，把警告告诉人。
5. 汇报 repo key、home、默认分支，以及下一步：
   - 用 `weefleet-claude` 启动协调者；或
   - 在普通会话里用 `/weefleet <需求>`。
6. 停止。不要创建任务、不要改代码、不要 dispatch。

## 模式 B：任务

否则，把 `$ARGUMENTS` 当作开发需求，使用 `weefleet-coordinator` skill，严格按它执行。

硬规则：

- 你不改代码，不编辑仓库文件。
- 先把目标、范围、checks、预算写成合同给我看；我明确说“批准”后，才运行 `weefleet approve` 和 `weefleet dispatch`。
- 只在三个门停下来问我：合同、`blocked`、`accept` / `reject`。
- 你不要替我做 accept/reject；不要自动合并。
- 不要读完整测试日志；只读 `weefleet status` 和 `weefleet verify` 摘要。
- 不要把 `done` 当完成；只认 `weefleet verify`。
