# Workbench Canonical Typed Submission V2 C3i1 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在不切换任何第一方 writer 的前提下，为现有 canonical Thread/Run/Resume 路由交付生成式 Typed Submission V2 合同、严格 raw JSON 接受边界、无损 V1 兼容映射与稳定错误合同。

**Architecture:** IDL 是公开合同唯一来源，固定的 18 个 V2 struct 通过受控 codegen 写入 Go/TypeScript；handler 在 binder 前对 raw JSON 做封闭字段、重复键、presence、`null`、预算与 union 校验，再确定性映射到现有 application command、fingerprint 和 authorization 链。C3i1 只增加服务端接受能力，V1 保持可读，C3i2 继续锁定，直到本计划的 parity corpus 全绿。

**Tech Stack:** Thrift IDL、Hertz `hz v0.9.7`、`thriftgo 0.4.5`、Go 1.24 `encoding/json`/Hertz/Testify、TypeScript 5.8、IDL2TS、Vitest、Workbench execution graph verifier

---

## 0. 交付边界与执行纪律
- 基线设计：`docs/superpowers/specs/2026-08-13-workbench-canonical-typed-submission-v2-design.md`，实施时逐项以该文件的 normative IDL、mapping、validation order、budgets 和 errors 为准。
- 交付优先：C3i1 只包含 18 个 IDL struct、6 个 append field（Stream 复用 Create Run field 27）、Go/TS 生成物、strict raw V2 validator/mapper、Create Thread/Create/Wait/Stream/Resume 接受、parity 与 authority；不扩展 codegen 脚本。
- C3i1 完成后的唯一 authority 表述是：`typed contract accepted, first-party writers still V1`。
- C3i2 仍为 locked；不得修改 Workbench/Task 页面、service、`canonicalThreadClient` 或五个第一方 writer。
- P1M 仍未 PASS；不得声称 Human Resume closure、legacy consumer retirement、real MySQL typed recovery race verified 或 P1M PASS。
- 本计划不新增 URL、migration、UI、Human Journal Attempt rollover、ordinary non-Journal enrollment、legacy decoder、gate-on producer、P1D/P2 loop、MySQL race 实现、generic extension、map、raw JSON public field 或 V2 `api.value_type="any"`。
- 两份用户拥有的未跟踪计划文件始终不读、不改、不暂存、不删除、不提交。
- 所有实现步骤遵循 RED → 最小 GREEN → refactor；每个 commit 前只暂存该任务列出的文件。
## 1. 文件职责锁定
| 文件 | 单一职责 |
| --- | --- |
| `backend/scripts/verify_api_codegen.sh` | 保持只读不改；以 `KEEP_API_CODEGEN_TMP=1` 生成可审计的两个临时 clean tree |
| `idl/workbench/thread.thrift` | 18 个封闭 V2 类型及 6 个 additive request field 的唯一公开来源 |
| `backend/api/model/workbench/thread_contract/thread.go` | Hertz 生成的 Go V2 类型与 request field，不手改 |
| `frontend/packages/arch/api-schema/src/idl/workbench/thread.ts` | IDL2TS 生成的 TS V2 类型与 request mapping，不手改 |
| `frontend/packages/arch/api-schema/src/__tests__/workbench-thread-contract.test.ts` | 锁定类型名、field optionality、i64 string conversion、body/header mapping 和 V2 无 `any`/map/retired control |
| `backend/api/handler/coze/workbench_canonical_typed_submission_v2.go` | raw JSON object walker、封闭 schema、presence/null/duplicate/budget/semantic validator、V2→现有 submission/config mapper |
| `backend/api/handler/coze/workbench_canonical_typed_submission_v2_test.go` | strict parser 表、semantic matrix、config/request fingerprint parity corpus |
| `backend/api/handler/coze/workbench_canonical_thread_service.go` | 在现有 Create Thread 顺序内选择 V1/V2、映射 initial/deferred，不改变 mutation/application 接口 |
| `backend/api/handler/coze/workbench_canonical_thread_service_test.go` | initial/deferred、mixing、priority、idempotency、零副作用 |
| `backend/api/handler/coze/workbench_canonical_run_service.go` | Create/Wait 共用 typed run parser；dedicated Resume 接受 `response_v2` 和 header key |
| `backend/api/handler/coze/workbench_canonical_run_service_test.go` | turn/retry/Resume 映射、authority、fingerprint、replay/conflict、零副作用 |
| `backend/api/handler/coze/workbench_canonical_run_stream.go` | 保持 principal/workspace 与 path ID parsing 在 body 前；把 application `AuthorizeThreadAccess` 放到 strict typed parse 后；解析及授权成功前不启动 SSE |
| `backend/api/handler/coze/workbench_canonical_run_stream_test.go` | Stream priority、turn/retry、SSE-before-parse 禁止、共享 parity |
| `docs/superpowers/context/workbench-chat.md` | 当前产品事实：服务端已接受 V2、第一方仍写 V1 |
| `docs/superpowers/context/project-context.md` | 长期公共合同事实与交付边界 |
| `docs/superpowers/context/workbench-execution-chain.md` | current execution chain 新增 typed acceptance/mapping 边界，不声称 writer cutover |
| `docs/superpowers/context/workbench-execution-graph.json` | 与 chain 同步的 monitored nodes/edges/source anchors |
| `docs/superpowers/plans/2026-08-10-workbench-adaptive-execution-mvp.md` | 主 MVP tracker 更新 C3i1 已交付、C3i2 locked、P1M not PASS |
| `scripts/workbench-execution-graph/contract.mjs` | execution graph 合同代码，与 chain/JSON 同步 |
### Task 1: 添加 18 个 IDL struct 并生成 Go/TypeScript 合同
**Files:**
- Modify: `frontend/packages/arch/api-schema/src/__tests__/workbench-thread-contract.test.ts`
- Modify: `idl/workbench/thread.thrift`
- Modify (generated): `backend/api/model/workbench/thread_contract/thread.go`
- Modify (generated): `frontend/packages/arch/api-schema/src/idl/workbench/thread.ts`
- [ ] **Step 1: 先写 generated contract RED**
新增准确类型表和 request mapping 断言：
```ts
const typedSubmissionV2Types = [
  'CanonicalComposerSelectionV2', 'CanonicalMemoryRetrievalV2',
  'CanonicalSkillsV2', 'CanonicalMCPToolsV2', 'CanonicalWebHTTPV2',
  'CanonicalWebSearchV2', 'CanonicalWebToolsV2', 'CanonicalModelRetryV2',
  'CanonicalModelFailoverV2', 'CanonicalTokenUsageV2',
  'CanonicalRunConfigV2', 'CanonicalUploadedFileReferenceV2',
  'CanonicalRunInputV2', 'CanonicalRunLineageV2',
  'CanonicalRunMetadataV2', 'CanonicalRunSubmissionV2',
  'CanonicalInitialRunSubmissionV2', 'CanonicalHumanInteractionResponseV2',
] as const;
it('generates the closed canonical typed submission V2 contract', () => {
  for (const name of typedSubmissionV2Types) {
    const source = interfaceSourceFrom(generatedSource, name);
    expect(source, name).not.toMatch(/\bany\b|Record<|\[key:\s*string\]/);
  }
  expect(interfaceSourceFrom(generatedSource, 'CanonicalComposerSelectionV2'))
    .toMatch(/model_type\?:\s*string/);
  expect(interfaceSourceFrom(generatedSource, 'CanonicalModelFailoverV2'))
    .toMatch(/candidate_model_ids:\s*string\[\]/);
  expect(interfaceSourceFrom(generatedSource, 'CanonicalUploadedFileReferenceV2'))
    .toMatch(/file_id:\s*string/);
});
```
把现有 API mapping 期望精确扩为：Create Thread body 追加 `initial_submission_v2`、`deferred_initial_submission_v2`；Create/Stream Run body 追加 `submission_v2`；Wait body 追加 `submission_v2`；Resume body 为 `interrupt_id,response,response_v2` 且 header 按冻结 field 5/6 的生成器顺序为 `X-Coze-Space-ID,Idempotency-Key`。
- [ ] **Step 2: 运行 RED**
Run:
```bash
cd frontend/packages/arch/api-schema
rushx test src/__tests__/workbench-thread-contract.test.ts
```
Expected: FAIL，18 个 interface 和 6 个 request field 尚未生成。
- [ ] **Step 3: 写入 exact normative IDL**
从批准设计 Section 3 逐字加入 18 个 struct，保持以下闭包和次序；不得加字段、map、`api.value_type="any"` 或 extension：
```thrift
struct CanonicalComposerSelectionV2 { 1: optional i64 model_type (agw.js_conv="str", api.js_conv="true"); 2: optional string model_name; 3: optional list<string> explicit_enable_skills; 4: required list<string> allowed_skills; 5: required list<string> enable_mcp; 6: required list<string> enable_kbs; 7: required list<string> enable_databases; 8: required list<string> allowed_mcp_tools }
struct CanonicalMemoryRetrievalV2 { 1: required i32 limit; 2: required i32 candidate_limit; 3: required list<string> scopes; 4: required double min_confidence }
struct CanonicalSkillsV2 { 1: required bool enabled; 2: required string visibility }
struct CanonicalMCPToolsV2 { 1: required bool enabled; 2: required string visibility }
struct CanonicalWebHTTPV2 { 1: required bool enabled; 2: required list<string> allowed_hosts; 3: required i64 timeout_ms; 4: required i64 max_response_bytes }
struct CanonicalWebSearchV2 { 1: required bool enabled; 2: required i32 max_results }
struct CanonicalWebToolsV2 { 1: required bool enabled; 2: required string visibility; 3: required CanonicalWebHTTPV2 http; 4: required CanonicalWebSearchV2 search }
struct CanonicalModelRetryV2 { 1: required i32 max_retries; 2: required i64 backoff_ms; 3: required bool retry_empty_output; 4: required list<string> retry_finish_reasons }
struct CanonicalModelFailoverV2 { 1: required list<i64> candidate_model_ids (agw.js_conv="str", api.js_conv="true"); 2: required i32 max_retries; 3: required bool failover_empty_output; 4: required list<string> failover_finish_reasons }
struct CanonicalTokenUsageV2 { 1: required bool enabled }
struct CanonicalRunConfigV2 { 1: required string runtime; 2: required CanonicalMemoryRetrievalV2 memory_retrieval; 3: required CanonicalSkillsV2 skills; 4: required CanonicalMCPToolsV2 mcp_tools; 5: required CanonicalWebToolsV2 web_tools; 6: optional CanonicalModelRetryV2 model_retry; 7: optional CanonicalModelFailoverV2 model_failover; 8: required CanonicalTokenUsageV2 token_usage }
struct CanonicalUploadedFileReferenceV2 { 1: required i64 file_id (agw.js_conv="str", api.js_conv="true") }
struct CanonicalRunInputV2 { 1: required string message; 2: required list<CanonicalUploadedFileReferenceV2> uploaded_files }
struct CanonicalRunLineageV2 { 1: required i64 source_run_id (agw.js_conv="str", api.js_conv="true") }
struct CanonicalRunMetadataV2 { 1: required string source }
struct CanonicalRunSubmissionV2 { 1: required string schema_version; 2: required string kind; 3: required CanonicalRunInputV2 input; 4: required CanonicalComposerSelectionV2 composer; 5: required CanonicalRunConfigV2 config; 6: optional CanonicalRunLineageV2 lineage; 7: optional CanonicalRunMetadataV2 metadata }
struct CanonicalInitialRunSubmissionV2 { 1: required string schema_version; 2: required CanonicalRunInputV2 input; 3: required CanonicalComposerSelectionV2 composer; 4: required CanonicalRunConfigV2 config; 5: optional CanonicalRunMetadataV2 metadata }
struct CanonicalHumanInteractionResponseV2 { 1: required string schema; 2: required string interaction_id; 3: required string kind; 4: required string decision; 5: optional string answer; 6: optional string choice_id; 7: optional string comment }
```
在现有 request 中只 append：Thread 8/9、Run 27、Wait 28、Resume 6 header/7 body，exact annotation 与设计表一致。
- [ ] **Step 4: 从现有 verifier 的 clean tree 精确复制 Go 生成物**
Run:
```bash
cd backend
codegen_log="$(mktemp)"
set +e
KEEP_API_CODEGEN_TMP=1 bash scripts/verify_api_codegen.sh >"${codegen_log}" 2>&1
codegen_status="$?"
set -e
cat "${codegen_log}"
test "${codegen_status}" -ne 0
codegen_root="$(sed -n 's/^API codegen temporary trees retained at //p' "${codegen_log}")"
test -n "${codegen_root}"
test -d "${codegen_root}/run-one/backend"
cmp \
  "${codegen_root}/run-one/backend/api/model/workbench/thread_contract/thread.go" \
  "${codegen_root}/run-two/backend/api/model/workbench/thread_contract/thread.go"
cp \
  "${codegen_root}/run-one/backend/api/model/workbench/thread_contract/thread.go" \
  api/model/workbench/thread_contract/thread.go
bash scripts/verify_api_codegen.sh
cd ../frontend/packages/arch/api-schema
rushx update
cd ../../../..
git diff --name-only -- backend/api/model backend/api/router frontend/packages/arch/api-schema/src/idl | sort
```
Expected: 第一次 verifier 只因 `REAL_VS_CLEAN_RUN1` stale baseline 非零退出且日志提供唯一临时根；run-one/run-two 的目标文件完全相同；复制后 verifier 全绿。最终 generator-owned diff 只有 `backend/api/model/workbench/thread_contract/thread.go` 与 `frontend/packages/arch/api-schema/src/idl/workbench/thread.ts`，router 与 verifier 脚本无 diff；若多出文件则停止、查明并只保留这两个已验证生成物。
- [ ] **Step 5: 运行合同 GREEN**
Run:
```bash
cd backend
bash scripts/verify_api_codegen.sh
cd ../frontend/packages/arch/api-schema
rushx test src/__tests__/workbench-thread-contract.test.ts
```
Expected: codegen 所有 PASS marker 出现；Vitest PASS；TS scalar IDs 为 `string`、candidate IDs 为生成器固定形式 `string[]`；V2 interfaces 不含 `any`/map/retired field。
- [ ] **Step 6: 提交 IDL 与生成物**
```bash
git add idl/workbench/thread.thrift \
  backend/api/model/workbench/thread_contract/thread.go \
  frontend/packages/arch/api-schema/src/idl/workbench/thread.ts \
  frontend/packages/arch/api-schema/src/__tests__/workbench-thread-contract.test.ts
git commit -m "feat: generate canonical typed submission v2 contract"
```
### Task 2: 实现 strict raw JSON 结构验证器
**Files:**
- Create: `backend/api/handler/coze/workbench_canonical_typed_submission_v2.go`
- Create: `backend/api/handler/coze/workbench_canonical_typed_submission_v2_test.go`
- [ ] **Step 1: 写 strict table RED**
建立最小合法 turn fixture 和表驱动 mutation；至少包含 outer malformed/trailing、每层 unknown、duplicate、required missing、required/optional `null`、invalid UTF-8、V1/V2 mix、initial/deferred mix。断言稳定 status/code/class/path 且 detail 不含 submitted value：
```go
func TestDecodeCanonicalTypedRunSubmissionV2RejectsClosedShapeViolations(t *testing.T) {
	tests := []struct{ name, body, code, class, path string; status int }{
		{"unknown nested", typedRunV2With(`"config":{"web_tools":{"foo":true}}`), "unsupported_sdk_field", "unsupported_field", "submission_v2.config.web_tools.foo", 422},
		{"duplicate", typedRunV2WithDuplicate("submission_v2.kind"), "invalid_request", "invalid_typed_submission", "submission_v2.kind", 422},
		{"missing", typedRunV2Without("submission_v2.config.runtime"), "invalid_request", "invalid_typed_submission", "submission_v2.config.runtime", 422},
		{"null optional", typedRunV2With(`"lineage":null`), "invalid_request", "invalid_typed_submission", "submission_v2.lineage", 422},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, got := decodeCanonicalTypedRunSubmissionV2([]byte(tt.body))
			require.NotNil(t, got)
			require.Equal(t, tt.status, got.status)
			require.Equal(t, tt.code, got.Code)
			require.Equal(t, tt.class, got.errorClass)
			require.Contains(t, got.Detail, tt.path)
		})
	}
}
```
- [ ] **Step 2: 运行 RED**
Run:
```bash
cd backend
go test ./api/handler/coze -run 'TestDecodeCanonicalTyped.*V2' -count=1
```
Expected: FAIL，decoder 和 schema 尚不存在。
- [ ] **Step 3: 实现 token walker 与封闭 schema**
使用 `json.Decoder.Token` 递归读取 object/array，不能先 unmarshal 到 map。固定接口：
```go
type canonicalV2Node struct {
	path string
	raw  json.RawMessage
	kind byte
	obj  map[string]*canonicalV2Node
	arr  []*canonicalV2Node
}
type canonicalV2FieldRule struct {
	required bool
	kind     byte
	object   map[string]canonicalV2FieldRule
	element  *canonicalV2FieldRule
}
// Extraction uses generated contract values for scalar/list storage and keeps
// raw presence separately so missing, explicit null and present-empty remain distinct.
type canonicalTypedV2Presence map[string]struct{}
type canonicalTypedRunV2 struct {
	Value    threadcontract.CanonicalRunSubmissionV2
	Presence canonicalTypedV2Presence
}
type canonicalTypedInitialV2 struct {
	Value    threadcontract.CanonicalInitialRunSubmissionV2
	Presence canonicalTypedV2Presence
}
type canonicalTypedHumanV2 struct {
	Value    threadcontract.CanonicalHumanInteractionResponseV2
	Presence canonicalTypedV2Presence
}
func decodeCanonicalV2Object(raw []byte, root string, rules map[string]canonicalV2FieldRule) (*canonicalV2Node, *canonicalError)
func readCanonicalV2Node(dec *json.Decoder, path string) (*canonicalV2Node, *canonicalError)
func validateCanonicalV2Object(node *canonicalV2Node, rules map[string]canonicalV2FieldRule) *canonicalError
func extractCanonicalTypedRunV2(node *canonicalV2Node) (*canonicalTypedRunV2, *canonicalError)
func extractCanonicalTypedInitialV2(node *canonicalV2Node) (*canonicalTypedInitialV2, *canonicalError)
func extractCanonicalTypedHumanV2(node *canonicalV2Node) (*canonicalTypedHumanV2, *canonicalError)
func decodeCanonicalTypedRunSubmissionV2(raw []byte) (*canonicalTypedRunV2, *canonicalError)
func decodeCanonicalTypedInitialSubmissionV2(raw []byte, field string) (*canonicalTypedInitialV2, *canonicalError)
func decodeCanonicalTypedHumanResponseV2(raw []byte) (*canonicalTypedHumanV2, *canonicalError)
```
`readCanonicalV2Node` 在同一 object 第二次见到 key 时立即返回 `invalid_typed_submission`；`validateCanonicalV2Object` unknown 返回 `unsupported_sdk_field`，再按 schema 顺序检查 missing/null/kind；decoder 必须确认 EOF。规则表逐层列全 18 struct 字段，错误 path 从 request field 开始，不记录或回显值。
- [ ] **Step 4: 运行 strict GREEN**
Run:
```bash
cd backend
go test ./api/handler/coze -run 'TestDecodeCanonicalTyped.*V2' -count=1
```
Expected: 表内 malformed 为 400 `invalid_json`；unknown 为 422 `unsupported_sdk_field/unsupported_field`；其他 strict 失败为 422 `invalid_request/invalid_typed_submission`；全部 PASS。
- [ ] **Step 5: 提交 raw decoder**
```bash
git add backend/api/handler/coze/workbench_canonical_typed_submission_v2.go \
  backend/api/handler/coze/workbench_canonical_typed_submission_v2_test.go
git commit -m "feat: strictly decode canonical typed submissions"
```
### Task 3: 完成 V2 semantic validator、Config mapper 与 parity corpus
**Files:**
- Modify: `backend/api/handler/coze/workbench_canonical_typed_submission_v2.go`
- Modify: `backend/api/handler/coze/workbench_canonical_typed_submission_v2_test.go`
- [ ] **Step 1: 写 budgets、union、presence 和 parity RED**
表必须覆盖设计 Sections 4/7 全部边界：message 256 KiB、IDs/grammar/sensitive-value、lists unique/caps/order、model safe integer、file positive int64、memory/web/retry/failover 数值、finite confidence、runtime/visibility、Skill 两集合规则、MCP 两列表规则、turn/retry lineage/metadata flow、Human kind/decision/optional normalization。建立 parity cases：primary model、Skill absent/empty/disabled/explicit、MCP auto/disabled/explicit、KB、database、memory、web、retry、ordered failover、Token Usage、absence rules、ordered attachments。
```go
func TestCanonicalTypedV2ConfigParity(t *testing.T) {
	for _, tc := range canonicalTypedV2ParityCases() {
		t.Run(tc.name, func(t *testing.T) {
			mapped, public := mapCanonicalTypedRunV2(tc.v2)
			require.Nil(t, public)
			require.JSONEq(t, tc.wantConfig, mapped.Config)
			require.Equal(t, tc.wantMessageMetadata, mapped.MessageMetadata)
			require.Equal(t, tc.wantUploadedFileIDs, mapped.UploadedFileIDs)
			require.Equal(t, tc.wantFingerprint, mapped.IdempotencyFingerprint)
		})
	}
}
```
- [ ] **Step 2: 运行 RED**
Run:
```bash
cd backend
go test ./api/handler/coze -run 'TestCanonicalTypedV2(ConfigParity|Semantic|Human|Numeric|Collection)' -count=1
```
Expected: FAIL，semantic validators 和 mappers 尚未实现。
- [ ] **Step 3: 实现确定性 mapper**
固定输出 seam：
```go
func validateCanonicalRunSubmissionV2(v *canonicalTypedRunV2) *canonicalError
func validateCanonicalInitialRunSubmissionV2(v *canonicalTypedInitialV2, deferred bool) *canonicalError
func validateCanonicalHumanResponseV2(v *canonicalTypedHumanV2) (canonicalResumeResponse, *canonicalError)
func mapCanonicalTypedRunV2(v *canonicalTypedRunV2) (*canonicalRunSubmission, *canonicalError)
func mapCanonicalTypedInitialV2(v *canonicalTypedInitialV2, deferred bool) (*canonicalInitialThreadRun, *canonicalError)
func canonicalTypedConfigV2(composer threadcontract.CanonicalComposerSelectionV2, config threadcontract.CanonicalRunConfigV2, presence canonicalTypedV2Presence) (string, *canonicalError)
```
Config 通过 typed structs 和 `json.Encoder`/`json.Marshal` 生成，不经过 `map[string]any` 或 `float64`。model IDs 先 `ParseInt`，再以 `json.Number`/typed integer 写 JSON number；list 保留输入顺序。turn 输出 `MessageMetadata == Config`、`Context == "{}"`；retry 输出 Run input message、空 attachments、空 MessageContent/MessageMetadata/Command/Context、typed source lineage；metadata absent 映射 `{}`。沿用现有 `canonicalRunTurnRequestFingerprint`、`canonicalRunRetryRequestFingerprint`、`canonicalInitialThreadRunRequestFingerprint` 和 `Version="v1"`。
- [ ] **Step 4: 运行 parity GREEN**
Run:
```bash
cd backend
go test ./api/handler/coze -run 'TestCanonicalTypedV2' -count=1
```
Expected: 所有 semantic/budget/parity tests PASS；配置与 compatibility components 相等；retry normalization 的历史差异仅为设计批准的 metadata 差异。
- [ ] **Step 5: 提交 validator/mapper**
```bash
git add backend/api/handler/coze/workbench_canonical_typed_submission_v2.go \
  backend/api/handler/coze/workbench_canonical_typed_submission_v2_test.go
git commit -m "feat: map canonical typed submissions to run commands"
```
### Task 4: 接入 Create Thread initial/deferred V2
**Files:**
- Modify: `backend/api/handler/coze/workbench_canonical_thread_service.go`
- Modify: `backend/api/handler/coze/workbench_canonical_thread_service_test.go`
- [ ] **Step 1: 写 Create Thread RED**
新增 atomic、deferred、mixing、unknown/null、retired-control priority、auth-before-body、zero-mutation、optional-key replay/conflict tests。atomic 断言 assistant=`agent`、固定 message metadata；deferred 断言 title 仍由 message 推导但 Run/Message/fingerprint 均未创建。
- [ ] **Step 2: 运行 RED**
Run:
```bash
cd backend
go test ./api/handler/coze -run 'TestCreateCanonicalThread.*TypedV2' -count=1
```
Expected: FAIL，handler 尚未选择 V2。
- [ ] **Step 3: 在既有优先级内接入 typed branch**
扩展 raw request 只保存 V2 `json.RawMessage` presence；在 dependency/auth/body-cap/retired scan 后、V1 decode/mutation 前调用：
```go
func canonicalCreateThreadTypedSubmissionV2(body []byte) (*canonicalInitialThreadRun, bool, *canonicalError)
```
该函数先检查 `initial_submission_v2`/`deferred_initial_submission_v2` 与 V1 `coze.*` mix，mixed 返回 class `mixed_submission_versions`；随后 strict decode/map。typed atomic/deferred 复用现有 application command 和 fingerprint path，不新增 repository/application 方法。
- [ ] **Step 4: 运行 Thread GREEN**
Run:
```bash
cd backend
go test ./api/handler/coze -run 'TestCreateCanonicalThread' -count=1
```
Expected: 既有 V1 与新增 V2 tests 全 PASS；所有 rejection 的 repository call count 为 0。
- [ ] **Step 5: 提交 Thread 接入**
```bash
git add backend/api/handler/coze/workbench_canonical_thread_service.go \
  backend/api/handler/coze/workbench_canonical_thread_service_test.go
git commit -m "feat: accept typed initial thread submissions"
```
### Task 5: 接入 Create/Wait/Stream Run turn/retry V2
**Files:**
- Modify: `backend/api/handler/coze/workbench_canonical_run_service.go`
- Modify: `backend/api/handler/coze/workbench_canonical_run_service_test.go`
- Modify: `backend/api/handler/coze/workbench_canonical_run_stream.go`
- Modify: `backend/api/handler/coze/workbench_canonical_run_stream_test.go`
- [ ] **Step 1: 写共享 parser 与 route priority RED**
Create/Wait/Stream 各覆盖 turn/retry；再覆盖 legacy mix、assistant 非 `agent`、turn+lineage、retry-lineage missing、retry attachments、source lookup after typed validation、same-Thread authority、ordered attachments、retry no Message/MessageMetadata、idempotency replay/conflict、historical V1 fingerprint+same key 409 no mutation。Stream 断言 principal/workspace 与 path ID parsing 先于 body；application `AuthorizeThreadAccess` 必须在 strict typed parse 之后，typed rejection 前不得读 Thread 或写 SSE header/body。
- [ ] **Step 2: 运行 RED**
Run:
```bash
cd backend
go test ./api/handler/coze -run 'Test(Create|Wait|Stream)CanonicalRun.*TypedV2' -count=1
```
Expected: FAIL，shared parser 尚未识别 `submission_v2`。
- [ ] **Step 3: 扩展 shared parser，不复制 route 实现**
在 `parseCanonicalRunSubmission` body-cap 和 retired scan 后检查 raw root presence：
```go
func parseCanonicalRunSubmission(c *app.RequestContext, allowRaiseError bool) (*canonicalRunSubmission, *canonicalError) {
	// existing body cap and retired-control scan remain first
	if canonicalRawObjectHasField(c.Request.Body(), "submission_v2") {
		return parseCanonicalTypedRunSubmissionV2(c, allowRaiseError)
	}
	return parseCanonicalLegacyRunSubmission(c, allowRaiseError)
}
```
typed parser 保留 safe route options，拒绝 legacy `input/command/metadata/config/context/coze` 的存在（包括 `null`），严格验证/映射后才进入 source/upload/application reads。Create/Wait 继续共用该 parser；Stream 顺序固定为 dependency → principal/workspace → path ID → body cap/retired scan/strict typed parse → application `AuthorizeThreadAccess` → source/upload reads → SSE writer。
- [ ] **Step 4: 运行 Run GREEN**
Run:
```bash
cd backend
go test ./api/handler/coze -run 'Test(Create|Wait|Stream)CanonicalRun' -count=1
```
Expected: V1/V2 tests 全 PASS；retry same-Thread authorization 保持；historical V1/V2 fingerprint collision 返回 409 且不建 Run；Stream rejection 不产生 SSE。
- [ ] **Step 5: 提交 Run 接入**
```bash
git add backend/api/handler/coze/workbench_canonical_run_service.go \
  backend/api/handler/coze/workbench_canonical_run_service_test.go \
  backend/api/handler/coze/workbench_canonical_run_stream.go \
  backend/api/handler/coze/workbench_canonical_run_stream_test.go
git commit -m "feat: accept typed canonical run submissions"
```
### Task 6: 接入 dedicated Resume `response_v2`
**Files:**
- Modify: `backend/api/handler/coze/workbench_canonical_run_service.go`
- Modify: `backend/api/handler/coze/workbench_canonical_run_service_test.go`
- [ ] **Step 1: 写 Resume RED**
覆盖 exactly-one V1/V2、required distinct interrupt/interaction IDs、clarification/confirmation matrix、unknown/null/empty/budget、body lineage rejection、header 128-byte cap、typed response normalization、existing fingerprint equivalence、replay/conflict 和 zero mutation。
- [ ] **Step 2: 运行 RED**
Run:
```bash
cd backend
go test ./api/handler/coze -run 'TestResumeCanonicalRun.*TypedV2' -count=1
```
Expected: FAIL，dedicated handler 尚未读取 `response_v2`/header key。
- [ ] **Step 3: 接入 typed Resume**
在 dependency/auth/path/body-cap/retired scan 后 raw-detect V1/V2 presence；mixed/none 先返回稳定错误；V2 strict decode/normalize 后调用现有：
```go
resume, public := canonicalResumeSubmissionFromRequest(runID, interruptID, normalizedResponse)
```
`runID` 仅来自 path，`interrupt_id` 与 `interaction_id` 均进入既有 fingerprint components；header key 走 `canonicalRunIdempotencyKey` 与 principal scoping；不得接收 body lineage/config/context/composer。
- [ ] **Step 4: 运行 Resume GREEN**
Run:
```bash
cd backend
go test ./api/handler/coze -run 'TestResumeCanonicalRun' -count=1
```
Expected: V1/V2 Resume tests 全 PASS；相同 normalized body/key replay，语义变化 conflict；rejection 无 checkpoint/source mutation。
- [ ] **Step 5: 提交 Resume 接入**
```bash
git add backend/api/handler/coze/workbench_canonical_run_service.go \
  backend/api/handler/coze/workbench_canonical_run_service_test.go
git commit -m "feat: accept typed canonical human responses"
```
### Task 7: 全量验收、execution authority 与 C3i1 阶段提交
**Files:**
- Modify: `docs/superpowers/context/project-context.md`
- Modify: `docs/superpowers/context/workbench-chat.md`
- Modify: `docs/superpowers/context/workbench-execution-chain.md`
- Modify: `docs/superpowers/context/workbench-execution-graph.json`
- Modify: `docs/superpowers/plans/2026-08-10-workbench-adaptive-execution-mvp.md`
- Modify: `scripts/workbench-execution-graph/contract.mjs`
- [ ] **Step 1: 先写 authority 差异**
只记录已由本轮测试证明的事实：IDL/generated closed V2、server strict acceptance/mapping、V1 readable、五个第一方 writer 仍为 V1、C3i2 locked、P1M not PASS。同步 `project-context.md`、`workbench-chat.md`、主 MVP plan、chain、graph JSON 与 `contract.mjs`；execution chain/graph 增加 raw typed validation → deterministic mapping → existing application/fingerprint 的有向边，并把新 handler tests/IDL/generated anchors 纳入对应 current node。
- [ ] **Step 2: 运行 focused + package verification**
Run:
```bash
cd backend
bash scripts/verify_api_codegen.sh
GOCACHE=/private/tmp/coze-go-build go test ./api/handler/coze ./api/router/coze -count=1
cd ../frontend/packages/arch/api-schema
rushx test src/__tests__/workbench-thread-contract.test.ts
rushx lint
cd ../../../..
node scripts/workbench-execution-graph.mjs verify --changed-from origin/dev
node scripts/workbench-execution-graph.mjs build
node scripts/workbench-execution-graph.mjs verify-derived
```
Expected: codegen PASS；Go packages PASS；Vitest/lint PASS；execution graph 三条命令 PASS。`graphify-out/` 派生产物不暂存。
- [ ] **Step 3: 运行结构扫描与范围审计**
Run:
```bash
v2_block="$(awk '
  /^struct CanonicalComposerSelectionV2 / { in_v2 = 1 }
  in_v2 { print }
  /^struct CanonicalHumanInteractionResponseV2 / { in_human = 1 }
  in_human && /^}$/ { exit }
' idl/workbench/thread.thrift)"
if printf '%s\n' "${v2_block}" | rg 'api\.value_type="any"|map<|extension|webhook|on_completion|after_seconds|feedback_keys|interrupt_before|interrupt_after|langsmith_tracer'; then
  exit 1
fi
rg -n 'api\.value_type="any"|webhook|on_completion|after_seconds|feedback_keys|interrupt_before|interrupt_after|langsmith_tracer' idl/workbench/thread.thrift || true
git diff --name-only ea99c4a49..HEAD
git status --short
```
Expected: `rg` 命中只来自既有 V1 compatibility fields，18 个 V2 struct block 内零命中；diff 不含 frontend app writer；status 只含本任务 authority 修改与两份保持原样的用户未跟踪文件。
- [ ] **Step 4: 独立 code review gate**
按 `requesting-code-review` 对当前 exact HEAD 做 specification review 和 code-quality review。验收条件：P0/P1/P2 均为 0；任何 parity loss、auth/path priority 回归、unknown/duplicate/null 漏洞、fingerprint namespace 漂移或第一方 writer 改动都阻止阶段提交并保持 C3i2 locked。
- [ ] **Step 5: 提交 authority**
```bash
git add docs/superpowers/context/workbench-chat.md \
  docs/superpowers/context/project-context.md \
  docs/superpowers/context/workbench-execution-chain.md \
  docs/superpowers/context/workbench-execution-graph.json \
  docs/superpowers/plans/2026-08-10-workbench-adaptive-execution-mvp.md \
  scripts/workbench-execution-graph/contract.mjs
git commit -m "docs: record canonical typed submission acceptance"
```
- [ ] **Step 6: completion evidence**
Run:
```bash
git status --short
git log --oneline --decorate -8
```
Expected: 两份用户未跟踪计划仍原样存在且没有其他 tracked diff；报告 exact HEAD、各验证命令、review P0/P1/P2、未运行项和剩余风险。只有 C3i1 parity corpus 全绿且 review 无发现时，下一执行计划才可解锁 C3i2；本计划本身不得宣称 C3i2 或 P1M 完成。
