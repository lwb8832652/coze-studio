# Workbench 执行图谱运维

## 目的

本 runbook 维护 Workbench 当前生产执行链的长期记忆，避免后续修改把历史
`ChatTask`、K2 设计、LangGraph API 兼容层、DeerFlow 语义或 legacy 历史恢复
误认为并行生产运行时。

权威文件：

- 人类可读事实：
  `docs/superpowers/context/workbench-execution-chain.md`
- 机器合同：
  `docs/superpowers/context/workbench-execution-graph.json`
- 派生且 ignored 的图：
  `docs/superpowers/context/workbench-execution-graphify/graphify-out/graph.json`

## 前置条件

- Node.js 22；脚本只使用 Node 内置模块。
- Graphify CLI 可用，当前构建基线为 `0.9.x`。
- codebase-memory 可用时优先使用；本机 CLI 入口为 `codegraph`。
- 在仓库根目录执行 Git 审计命令。图谱脚本自身从脚本位置解析默认仓库，调用
  时不依赖当前工作目录。

## 日常流程

开始修改 Workbench、TaskDetail、Run、worker、Eino、MCP、Memory、Artifact、
Token、Guardrail 或相关入口前：

1. 阅读 `docs/superpowers/context/workbench-execution-chain.md`。
2. 查询 `docs/superpowers/context/workbench-execution-graph.json` 中对应 chain、
   node、edge、source anchor 和 test evidence。
3. 用 codebase-memory/CodeGraph 查实时调用方和影响范围，再打开真实源码核对。
4. 明确改动是否会改变权威节点、关系、顺序、框架版本或边界。

提交前运行：

```bash
node scripts/workbench-execution-graph.mjs verify --changed-from origin/dev
node scripts/workbench-execution-graph.mjs build
node scripts/workbench-execution-graph.mjs verify-derived
```

`verify` 检查合同、源码定位、版本、middleware 顺序、排除边界和权威文件联动；
`build` 在临时目录构建 Graphify AST 后注入显式有向关系，成功后才替换旧图；
`verify-derived` 检查摘要、图健康、必经路径与查询烟测。

## 工具职责

### Graphify

Graphify 查询稳定业务关系和已提交框架事实。合同边的
`authority=workbench_execution_contract`、`confidence=EXTRACTED` 是权威关系；
AST 边只补充源码结构。Graphify 不得从历史文档推断当前执行链。

常用查询：

```bash
graphify query "workbench execution runtime framework" --graph docs/superpowers/context/workbench-execution-graphify/graphify-out/graph.json
graphify query "eino adk runner chat model middleware tool event" --graph docs/superpowers/context/workbench-execution-graphify/graphify-out/graph.json
graphify path "Workbench Immediate Submit" "TaskDetail Event Projection" --graph docs/superpowers/context/workbench-execution-graphify/graphify-out/graph.json
```

### codebase-memory / CodeGraph

CodeGraph 查询当前 checkout 的函数、调用者、被调用者和影响范围。修改前后分别
运行结构查询；如果索引状态不是 current，先刷新：

```bash
codegraph sync "$PWD"
codegraph status "$PWD"
codegraph explore -p "$PWD" --max-files 20 Workbench TaskThread RunWorker ADKExecutor EventSource
```

Graphify 与 CodeGraph 结论冲突时，以当前源码、IDL、迁移、测试和运行时现象为
准，并立即修正权威上下文或刷新派生索引。

## 更新清单

以下任何变化都必须同时更新两份权威文件：

- Workbench 立即/延迟提交、上传或 TaskDetail follow-up 顺序；
- Thrift contract、生成 client、Hertz handler 或 SSE 投影；
- application/domain/repository 所有权或事务边界；
- MySQL admission、pending/queued 状态、`SKIP LOCKED`、lease fence；
- RunWorker、RunProcessor、RuntimeSelector 或 Eino ADK 执行；
- Eino Runner、ChatModelAgent、middleware、tool、MCP、checkpoint、event mapping；
- cancel、human resume、subagent retry、lease recovery、multitask rollback；
- Memory、Artifact、Token、Guardrail Audit、MCP Runtime Audit；
- React、Thriftgo、Hertz、Go、GORM/MySQL、Eino/Eino-ext、mcp-go、Sonic、
  Prometheus、cron、飞书 SDK 的版本或职责；
- LangGraph、DeerFlow、legacy、K2、ChatTask 的边界事实。

更新时遵循：

1. 先改源码和测试；
2. 修改 `workbench-execution-graph.json` 的节点、边、链、查询和证据；
3. 同步修改 `workbench-execution-chain.md` 的人类说明；
4. 运行 `verify --changed-from origin/dev`；
5. 重建并运行 `verify-derived`；
6. 刷新 CodeGraph，复查改动符号和调用链。

不得只改其中一份权威文件。不得用失效行号代替稳定 symbol/locator。不得为了图
连通而把时序边写成调用边；没有直接调用时使用有证据的 `precedes` 或其它准确
关系。

## 派生图过期与恢复

`verify-derived` 报 `stale_derived_digest` 时，先运行合同验证，再重建：

```bash
node scripts/workbench-execution-graph.mjs verify
node scripts/workbench-execution-graph.mjs build
node scripts/workbench-execution-graph.mjs verify-derived
```

Graphify 不可用时，`build` 会报 `graphify_unavailable` 并保留上一份有效图；不要
手工删除旧图。恢复 Graphify 后重新构建。若合同本身失败，先修合同或源码事实，
不得用 `--force` 绕过。

派生目录全部 ignored，不提交 `corpus/`、`build-meta.json`、`graph.json` 或 HTML。

## 故障注入

在仓库内创建临时合同副本，修改副本后通过 `--contract` 验证错误码；结束后删除
临时目录：

```bash
tmp_dir="$(mktemp -d docs/superpowers/context/.workbench-graph-fault.XXXXXX)"
cp docs/superpowers/context/workbench-execution-graph.json "$tmp_dir/contract.json"
node scripts/workbench-execution-graph.mjs verify --contract "$tmp_dir/contract.json"
rm -rf "$tmp_dir"
```

可注入的安全故障包括：框架版本漂移、middleware 换序、悬空 edge、兼容节点
进入 `canonical_executor`、K2/ChatTask current node。不要在故障注入中写入真实
prompt、completion、tool 参数/结果、凭据、对象地址、checkpoint bytes、provider
原始响应或 audit 原始载荷。

## 查询验收

每次主链变化至少确认：

- Workbench immediate submit 能到 pending Run、worker、Eino、EventSource；
- 带文件的 Workbench 和 TaskDetail 路径保持先上传再创建 Run；
- Eino 查询返回 Runner、ChatModelAgent、middleware/tool/MCP 和 EventSink；
- cancel、human resume、subagent retry、lease recovery 可分别遍历；
- Memory、Artifact、Token、Guardrail、MCP audit 有独立持久化边；
- LangGraph/DeerFlow/legacy 查询明确返回兼容边界，而不是并行运行时；
- K2 和 ChatTask 的 current production node 数为 0。

查询结果必须引用源码锚点，不能只经过文档 `references` 边。

## 安全扫描

```bash
rg -n "prompt|completion|tool_arguments|tool_results|checkpoint_bytes|credential|object_uri|raw_provider|raw_audit" docs/superpowers/context/workbench-execution-chain.md docs/superpowers/context/workbench-execution-graph.json
rg -n "K2|ChatTask" docs/superpowers/context/workbench-execution-graph.json
```

敏感词只能出现在安全禁止声明；K2/ChatTask 只能出现在排除规则和解释性边界，
不能成为 current node、runtime value 或 canonical edge。

## dev 两阶段审计

本图谱任务与其它需求一样遵循
`docs/superpowers/runbooks/dev-integration-audit.md`，并增加图谱专项检查。

### 第一次审计：需求分支

1. `git fetch origin dev`，记录 `origin/dev` SHA、需求分支 SHA 和 merge-base；
2. 确认需求分支基于最新 `origin/dev`，工作区无未解释改动；
3. 审查 `origin/dev...HEAD` 文件范围，确认无业务代码误改和敏感内容；
4. 运行全部 Node 测试、三条图谱命令、Graphify 关键查询和 CodeGraph 刷新；
5. 检查潜在 merge conflict、同文件并行修改和 dev 新功能是否可能被覆盖；
6. 向用户报告 SHA、范围、测试、查询、安全扫描和风险，等待“合入本地 dev”的
   明确确认。

未获确认不得 checkout/merge 本地 `dev`，不得推送。

### 第二次审计：本地 dev 合并后

用户确认后，先再次确认 `origin/dev` 未变化，再把已审计 SHA 合入本地 `dev`。
合并后执行更严格审计：

1. 记录合并前后本地 `dev` SHA 和实际合入提交；
2. 验证本地 `dev` 包含远端最新功能和需求分支全部预期提交；
3. 复查合并 diff、冲突解决、重复/丢失提交、意外覆盖和工作区状态；
4. 从合并后的 `dev` 重跑全部测试、`verify --changed-from origin/dev`、build、
   `verify-derived`、Graphify 查询、CodeGraph 和安全扫描；
5. 向用户提交第二次审计报告，等待单独的“推送远端”确认。

远端 `dev` 在任一审计阶段变化、需求 SHA 改变、冲突解决引入新内容或验证结果
变化时，第一次审计作废并重新开始。第一次确认只授权本地合并，第二次确认才
授权推送；禁止 force push。
