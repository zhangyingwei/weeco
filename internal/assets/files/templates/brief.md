# {{TASK_ID}}

## 目标

写清楚这次要改变的用户可见行为。不要写“优化一下”“完善一下”。

## 只改这些路径

- src/...
- tests/...

## 不要做

- 不 push，不碰 main
- 不安装新依赖，除非这里明确要求
- 不改数据库 schema，除非这里明确要求

## 验收

`weefleet verify` 会重跑同目录的 `checks.json`，不要自己改它。

本次 checks：{{CHECK_IDS}}

## 交付

- 代码改动（必须 commit，不要留着未提交的改动）
- `report.json`
- 证据文件放在 `evidence/`

## 结束时

1. `git add -A && git commit -m "<summary>" -m "<trailer 段落>"`
   commit message 最后一段必须带：
   Task: {{TASK_ID}}
   Attempt: <dispatch 给你的 attempt>
   Checks: {{CHECK_IDS}}
   Report: report.json
   Base: <dispatch 给你的 base_sha>
   Status: done
   （做不完就改成 Status: blocked 或 Status: failed）
2. 把 commit sha 写进 report 的 `head_sha`
3. 最后一行输出：REPORT: {{REPORT_PATH}}
