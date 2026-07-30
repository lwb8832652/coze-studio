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
- 派生且 ignored 的主执行图：
  `docs/superpowers/context/workbench-execution-graphify/graphify-out/graph.json`
- 派生且 ignored 的业务检索图：
  `docs/superpowers/context/workbench-execution-graphify/graphify-out/query-graph.json`

`workbench-execution-graphify/` 是稳定符号链接；实际完整版本保存在 ignored 的
`workbench-execution-graphify-versions/`。上一有效版本由 ignored 的
`workbench-execution-graphify-previous` 指向。正常查询只使用稳定路径，previous
只用于本地回滚核验。

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

`verify` 由 CLI 强制启用 canonical 模式，并用完整结构摘要检查
`workbench_execution_v1` profile、authority rule、源码定位、版本、
middleware 顺序、必需分支、排除边界、监控覆盖和权威文件联动；`build` 在临时目录构建 Graphify
AST，为 AST 边生成稳定 ID，并生成同基底的两张图：`graph.json` 只注入显式有向
关系和源码锚点桥，`query-graph.json` 在同一基底上额外注入查询 overlay。完成两图
一致性、健康和查询校验后，写入受管标记，把完整目录移入版本区并原子替换稳定
指针，同时保留上一完整版本；
`verify-derived` 检查语料/构建器摘要、
AST 节点/边/源码覆盖、桥接数量、必经路径与查询结果。构建元数据记录 Git commit、
dirty 状态和 status digest；构建器摘要绑定 CLI、合同校验器和派生构建器，提交、
状态或任一脚本变化都必须重建。

每个业务问题先在 `query-graph.json` 用完整 `question` 查询检索意图节点并返回全部
必需节点；每个 `required_edge_id` 再在无 overlay 的 `graph.json` 用独立
`graphify path` 校验一跳、正向和关系一致，最后在主图逐个使用完整节点标签做锚点
烟测。查询输出只有真实 `NODE`/有向 path 行参与判断，命令回显、宽查询截断、反向
箭头、同名文本或 `retrieves` 捷径不能充当执行证据。Graphify 将同端点多关系显示
为 `relation_a/relation_b` 时，目标关系必须明确包含在该集合中。

## 工具职责

### Graphify

Graphify 查询稳定业务关系和已提交框架事实。合同边的
`authority=workbench_execution_contract`、`confidence=EXTRACTED` 是权威关系；
AST 边只补充源码结构。`query_overlay` 只存在于业务检索图，把 8 类业务问题连接
到必需事实；`workbench_source_anchor` 同时存在于两图，把合同节点连接到真实 AST
文件节点。两张图由当前合同和白名单源码确定性生成，基底必须逐项一致。Graphify
不得从历史文档推断当前执行链。

常用查询：

```bash
graphify query "workbench execution runtime framework" --graph docs/superpowers/context/workbench-execution-graphify/graphify-out/query-graph.json
graphify query "ADKExecutor.Execute" --graph docs/superpowers/context/workbench-execution-graphify/graphify-out/graph.json
graphify path "Workbench Immediate Submit" "TaskDetail Event Projection" --graph docs/superpowers/context/workbench-execution-graphify/graphify-out/graph.json
```

`graphify path` 可能为寻找连通路径而显示反向箭头。验收执行顺序时必须逐跳确认
箭头方向；出现 `<--` 的路径不能作为有向执行链证据。顺序事实以合同中的
`ordered_node_ids`、`ordered_edge_ids` 和 `verify-derived` 结果为准。

### codebase-memory / CodeGraph

CodeGraph 查询当前 checkout 的函数、调用者、被调用者和影响范围。首次使用先
初始化；已有索引但状态不是 current 时再刷新：

```bash
codegraph status "$PWD"
codegraph init "$PWD" # 仅在 status 显示 Not initialized 时运行
codegraph sync "$PWD" # 已初始化且有源码变化时运行
codegraph explore -p "$PWD" --max-files 20 Workbench CanonicalThreadClient RunWorker ADKExecutor RunEventStream
```

Graphify 与 CodeGraph 结论冲突时，以当前源码、IDL、迁移、测试和运行时现象为
准，并立即修正权威上下文或刷新派生索引。

## 更新清单

以下任何变化都必须同时更新两份权威文件：

- Workbench 立即/延迟提交、上传或 TaskDetail follow-up 顺序；
- Thrift contract、生成 client/model、生成 Hertz route、自定义 SSE route、handler
  或 SSE 投影；
- application/domain/repository 所有权或事务边界；
- MySQL admission、pending/queued 状态、`SKIP LOCKED`、lease fence；
- RunWorker、RunProcessor、RuntimeSelector 或 Eino ADK 执行；
- Eino Runner、ChatModelAgent、middleware、tool、MCP、checkpoint、event mapping；
- cancel、human resume、subagent retry、lease recovery、multitask rollback；
- Memory、Artifact、Token、Guardrail Audit、MCP Runtime Audit；
- React、Thriftgo、Hertz、Go、GORM/MySQL、Eino/Eino-ext、mcp-go、Sonic、
  Prometheus、cron、飞书 SDK 的版本或职责；
- LangGraph、DeerFlow、legacy、K2、ChatTask 的边界事实。
- LangGraph stateless backing thread、Scheduled 新建/复用会话、飞书新建/复用
  session 的任一分支。

更新时遵循：

1. 先改源码和测试；
2. 修改 `workbench-execution-graph.json` 的节点、边、链、查询和证据；
3. 同步修改 `workbench-execution-chain.md` 的人类说明；
4. 运行 `verify --changed-from origin/dev`；
5. 重建并运行 `verify-derived`；
6. 刷新 CodeGraph，复查改动符号和调用链。

每项 `exclusion` 必须带可定位的 `evidence`。监控变化集合包含 tracked diff、staged
diff、未跟踪非 ignored 文件和 tracked 文件类型变化；Git rename 按删除旧路径和
新增新路径处理，普通文件与符号链接互换、移出监控目录也必须触发两份权威文件
联动。Git 路径使用 NUL 分隔读取，不能因换行、反斜杠或其它合法文件名字符漏报。

不得只改其中一份权威文件；受监控源码的新增、修改、重命名和删除都属于联动
范围。不得用失效行号代替稳定 symbol/locator。不得为了图连通而把时序边写成
调用边；没有直接调用时使用有证据的 `precedes` 或其它准确关系。

## 派生图过期与恢复

`verify-derived` 报 `stale_derived_digest`、`stale_builder_digest` 或
`stale_git_commit` / `stale_git_dirty` / `stale_git_status_digest` 时，先运行合同
验证，再重建：

```bash
node scripts/workbench-execution-graph.mjs verify
node scripts/workbench-execution-graph.mjs build
node scripts/workbench-execution-graph.mjs verify-derived
```

Graphify 不可用时，`build` 会报 `graphify_unavailable` 并保留上一份有效图；若
提取结果缺少 AST 节点、AST 边、任一可解析源码文件，或任一 current 合同节点缺少
真实 AST 文件桥、最终图缺少业务边、
稳定 ID，构建同样失败且不会替换旧图。不要手工删除旧图。恢复 Graphify 或修正
合同后重新构建，不得用 `--force` 绕过。

若受限沙箱中出现 `AST extraction failed: Operation not permitted`，是 Graphify
`ProcessPool` 被系统权限阻止；应授权同一条本地 `build` 命令使用所需进程权限后
重跑。不得把 `graphify_ast_empty` 改成 warning，也不得安装空图或复用过期图冒充
当前结果。

派生稳定/previous 指针和版本目录全部 ignored，不提交 `corpus/`、`build-meta.json`、
`graph.json`、`query-graph.json` 或 HTML。不要手工改写稳定指针；一次成功构建只
切换当前指针并保留上一有效版本；首次旧目录迁移的可捕获失败会自动回滚。历史
孤立版本只能在确认既不被 current/previous 指针引用后手工清理。

`--derived-root` 只允许指向仓库内 `docs/superpowers/context` 下以
`workbench-execution-graph` 开头的路径。发布器在移动任何既有目录前要求受管标记；
绝不依据文件名猜测目录所有权，也不替换普通目录。若本机残留无标记的早期 ignored
图谱，先核实其内容和 current/previous 引用，再经明确确认人工移走。

## 故障注入

在仓库内创建临时合同副本，修改副本后通过 `--contract` 验证错误码；结束后删除
临时目录：

```bash
tmp_dir="$(mktemp -d docs/superpowers/context/.workbench-graph-fault.XXXXXX)"
cp docs/superpowers/context/workbench-execution-graph.json "$tmp_dir/contract.json"
node scripts/workbench-execution-graph.mjs verify --contract "$tmp_dir/contract.json"
rm -rf "$tmp_dir"
```

可注入的安全故障包括：移除 profile/改写 authority path、框架版本漂移、middleware
换序、悬空 edge、缺失 exclusion evidence、兼容节点进入 `canonical_executor`、
K2/ChatTask current node。不要在故障注入中写入真实
prompt、completion、tool 参数/结果、凭据、对象地址、checkpoint bytes、provider
原始响应或 audit 原始载荷。

## 查询验收

每次主链变化至少确认：

- Workbench immediate submit 能到 pending Run、worker、Eino、canonical SSE、
  `@coze-arch/fetch-stream` 和 TaskDetail projection；
- 带文件的 Workbench 和 TaskDetail 路径保持先上传再创建 Run；
- LangGraph stateless 先创建 backing Thread 再创建 Run；Scheduled 与飞书的
  新建会话、复用会话两条分支都可检索；
- Eino 查询返回 Runner、ChatModelAgent、middleware/tool/MCP 和 EventSink；
- cancel、human resume、subagent retry、lease recovery 可分别遍历；
- Memory、Artifact、Token、Guardrail、MCP audit 有独立持久化边；
- LangGraph/DeerFlow/legacy 查询明确返回兼容边界，而不是并行运行时；
- K2 和 ChatTask 的 current production node 数为 0。

业务问题先在检索图返回合同要求的节点；查询结果中的有向业务边和执行路径必须
再从主执行图取得，并可经 `anchored_in` 到达 Graphify AST 文件节点。不能只经过
`retrieves`、文档 `references` 或反向连通边。

## 安全扫描

```bash
rg -n "prompt|completion|tool_arguments|tool_results|checkpoint_bytes|credential|object_uri|raw_provider|raw_audit" docs/superpowers/context/workbench-execution-chain.md docs/superpowers/context/workbench-execution-graph.json
rg -n "K2|ChatTask" docs/superpowers/context/workbench-execution-graph.json
```

敏感词只能出现在安全禁止声明；K2/ChatTask 只能出现在排除规则和解释性边界，
不能成为 current node、runtime value 或 canonical edge。

Corpus 只接受仓库内普通源码/合同文件。`.env`、私钥/keystore、数据库/日志文件、
`.git`/`.ssh`/secret/credential/log 目录和任何 corpus 符号链接会以
`sensitive_corpus_path`、`unsupported_corpus_path` 或
`corpus_path_symlink` / `corpus_path_escapes_repo` 失败；不得为了构建图谱放宽该
保护。候选路径和解析后的真实路径都必须通过同一敏感路径检查。

语料按证据文件整文件复制，因此 ignored 的本地 `corpus/` 可能包含仓库已跟踪的
合成安全测试值或公开的本地 Docker 默认值；这些值不得进入最终 Graphify JSON，
也不得上传或导出。若未来引入远程语义提取、共享语料或产物导出，必须先增加
内容级脱敏和独立安全复核。

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
