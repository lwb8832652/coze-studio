# Workbench Canonical Typed Submission V2 Design

**Status:** Approved design, implementation pending
**Date:** 2026-08-13
**Delivery rule:** Ship the smallest typed write path on existing routes. Do not add a route, migration, page, Human rollover, legacy decoder, gate-on producer, or generic extension mechanism.

## 1. Goal and delivery slices

Replace the five first-party Workbench write shapes that still use untyped canonical request fields with an additive, generated, closed typed contract. The new contract must:

- preserve the current model, Skill, MCP, knowledge-base, database, memory, web-tool, retry, failover, Token Usage, message, and ordered attachment semantics;
- preserve existing authorization, upload-before-run, same-Thread retry, SSE, and idempotency behavior;
- never accept or emit the seven retired execution-control fields;
- avoid `api.value_type="any"`, JSON-in-string, maps, metadata overflow, or a generic extension field inside every new V2 type.

Delivery is two sequentially reviewed slices:

- **C3i1 — contract and server acceptance:** add the exact IDL below, regenerate Go and TypeScript, strictly decode V2 from raw JSON, map it to the existing application command and fingerprint, and cover Create Thread, Create/Wait/Stream Run, and Resume. C3i1 does not change a first-party writer.
- **C3i2 — first-party cutover:** switch root without attachments, root with attachments, follow-up, top-level retry, and Resume to one shared typed serializer behind the existing `canonicalThreadClient`.

C3i2 remains locked until C3i1 proves the frozen parity corpus is lossless.

## 2. Chosen approach

Add optional V2 fields to the current canonical routes. V1 remains readable during a bounded compatibility window, but one request may use only one version. A second `/v2` route and a frontend-only type wrapper are rejected because both would leave duplicate authorization, streaming, idempotency, or untyped server behavior.

## 3. Normative IDL

This section is normative. Field names are also the exact JSON property names. `required` means the property must be present in raw V2 JSON; an explicit JSON `null` is invalid for both required and optional V2 fields. Strict raw validation enforces presence before generated binders can collapse missing values to zero values.

Model and file IDs use Thrift `i64` with JS string conversion. Generated TypeScript therefore carries canonical positive decimal strings. Model IDs are additionally capped at JavaScript's maximum safe integer because the current first-party model catalogue is number-based. The server parses every model decimal into `int64` and writes exact JSON integer digits when producing the existing internal Run Config; it never routes the value through `float64` or persists a JSON string.

```thrift
struct CanonicalComposerSelectionV2 {
    1: optional i64 model_type (agw.js_conv="str", api.js_conv="true")
    2: optional string model_name
    // The current intermediate explicit_enable_skills selection. It maps to the
    // existing persisted Config key enable_skills.
    // absent = no explicit Skill selection; non-empty = explicit ordered selection.
    // Present-empty is legal only while skills.enabled is false.
    3: optional list<string> explicit_enable_skills
    // Full ordered Skill allowlist from runtimeSettings.skills.allowed_skills,
    // including persistent and explicit selections.
    4: required list<string> allowed_skills
    // Active MCP selection: empty means auto-discovery while enabled and no
    // execution while disabled. Configured tools are retained separately in field 8.
    5: required list<string> enable_mcp
    6: required list<string> enable_kbs
    7: required list<string> enable_databases
    // Full ordered configured MCP allowlist from
    // runtimeSettings.mcp_tools.allowed_tools. It remains independent from the
    // currently enabled flat enable_mcp list so disabling MCP does not erase UI state.
    8: required list<string> allowed_mcp_tools
}

struct CanonicalMemoryRetrievalV2 {
    1: required i32 limit
    2: required i32 candidate_limit
    3: required list<string> scopes
    4: required double min_confidence
}

struct CanonicalSkillsV2 {
    1: required bool enabled
    2: required string visibility
}

struct CanonicalMCPToolsV2 {
    1: required bool enabled
    2: required string visibility
}

struct CanonicalWebHTTPV2 {
    1: required bool enabled
    2: required list<string> allowed_hosts
    3: required i64 timeout_ms
    4: required i64 max_response_bytes
}

struct CanonicalWebSearchV2 {
    1: required bool enabled
    2: required i32 max_results
}

struct CanonicalWebToolsV2 {
    1: required bool enabled
    2: required string visibility
    3: required CanonicalWebHTTPV2 http
    4: required CanonicalWebSearchV2 search
}

struct CanonicalModelRetryV2 {
    1: required i32 max_retries
    2: required i64 backoff_ms
    3: required bool retry_empty_output
    4: required list<string> retry_finish_reasons
}

struct CanonicalModelFailoverV2 {
    1: required list<i64> candidate_model_ids (agw.js_conv="str", api.js_conv="true")
    2: required i32 max_retries
    3: required bool failover_empty_output
    4: required list<string> failover_finish_reasons
}

struct CanonicalTokenUsageV2 {
    1: required bool enabled
}

struct CanonicalRunConfigV2 {
    1: required string runtime
    2: required CanonicalMemoryRetrievalV2 memory_retrieval
    3: required CanonicalSkillsV2 skills
    4: required CanonicalMCPToolsV2 mcp_tools
    5: required CanonicalWebToolsV2 web_tools
    // Object absence means disabled. No separate enabled flag exists on the wire.
    6: optional CanonicalModelRetryV2 model_retry
    // Object absence means disabled. No separate enabled flag exists on the wire.
    7: optional CanonicalModelFailoverV2 model_failover
    8: required CanonicalTokenUsageV2 token_usage
}

struct CanonicalUploadedFileReferenceV2 {
    1: required i64 file_id (agw.js_conv="str", api.js_conv="true")
}

struct CanonicalRunInputV2 {
    1: required string message
    // Always present. Empty means no attachments; order is authoritative.
    2: required list<CanonicalUploadedFileReferenceV2> uploaded_files
}

struct CanonicalRunLineageV2 {
    1: required i64 source_run_id (agw.js_conv="str", api.js_conv="true")
}

struct CanonicalRunMetadataV2 {
    // Exact allowlist is workbench_new_task, workbench_detail_followup, task_retry.
    1: required string source
}

struct CanonicalRunSubmissionV2 {
    1: required string schema_version
    // Exact allowlist is turn, retry.
    2: required string kind
    3: required CanonicalRunInputV2 input
    4: required CanonicalComposerSelectionV2 composer
    5: required CanonicalRunConfigV2 config
    6: optional CanonicalRunLineageV2 lineage
    7: optional CanonicalRunMetadataV2 metadata
}

struct CanonicalInitialRunSubmissionV2 {
    1: required string schema_version
    2: required CanonicalRunInputV2 input
    3: required CanonicalComposerSelectionV2 composer
    4: required CanonicalRunConfigV2 config
    5: optional CanonicalRunMetadataV2 metadata
}

struct CanonicalHumanInteractionResponseV2 {
    // Exact value remains coze.human_interaction_response.v1.
    1: required string schema
    2: required string interaction_id
    // Exact allowlist is clarification, confirmation.
    3: required string kind
    // clarification => answered; confirmation => approved or rejected.
    4: required string decision
    5: optional string answer
    6: optional string choice_id
    7: optional string comment
}
```

The existing request fields and IDs remain unchanged. C3i1 appends exactly these fields:

| Existing request | New field ID | Exact Thrift field |
| --- | ---: | --- |
| `CreateCanonicalThreadRequest` | 8 | `optional CanonicalInitialRunSubmissionV2 initial_submission_v2 (api.body="initial_submission_v2")` |
| `CreateCanonicalThreadRequest` | 9 | `optional CanonicalInitialRunSubmissionV2 deferred_initial_submission_v2 (api.body="deferred_initial_submission_v2")` |
| `CreateCanonicalRunRequest` | 27 | `optional CanonicalRunSubmissionV2 submission_v2 (api.body="submission_v2")` |
| `WaitCanonicalRunRequest` | 28 | `optional CanonicalRunSubmissionV2 submission_v2 (api.body="submission_v2")` |
| `ResumeCanonicalRunRequest` | 6 | `optional string idempotency_key (api.header="Idempotency-Key")` |
| `ResumeCanonicalRunRequest` | 7 | `optional CanonicalHumanInteractionResponseV2 response_v2 (api.body="response_v2")` |

The Stream create endpoint already uses `CreateCanonicalRunRequest`, so it receives field 27 without a duplicate struct. Existing Resume field 4 `response:any` stays readable for V1; field 7 is the only response field written by C3i2.

## 4. Closed shapes and presence semantics

Schema values are exact:

- initial/deferred: `coze.workbench.initial_run_submission.v2`;
- turn/retry: `coze.workbench.run_submission.v2`;
- Human response: existing `coze.human_interaction_response.v1`.

The following matrix is normative:

| Field/behavior | Atomic initial | Deferred initial | Turn Run | Retry Run |
| --- | --- | --- | --- | --- |
| Request field | `initial_submission_v2` | `deferred_initial_submission_v2` | `submission_v2` | `submission_v2` |
| `kind` | not present in type | not present in type | required `turn` | required `retry` |
| `input.message` | required non-empty | required non-empty | required non-empty | required non-empty; becomes Run input only |
| `input.uploaded_files` | required `[]` | required `[]` | required ordered list, possibly empty | required `[]` |
| `composer` / `config` | required | required | required | required |
| `lineage` | not present in type | not present in type | prohibited | required; `source_run_id` positive |
| `metadata` | wire-optional; C3i2 omits it | wire-optional; C3i2 omits it | wire-optional; C3i2 sends source `workbench_new_task` or `workbench_detail_followup` according to the flow | wire-optional; C3i2 sends source `task_retry` |
| User Message persisted | yes | no Run or Message yet | yes | no |
| Message metadata | server-fixed `{"source":"workbench_new_task"}`; not client supplied | no Message | server-derived typed Config copy during compatibility | empty |

Deferred initial is not an empty Thread create. It preserves the current behavior: the server validates the initial message/config and uses the message for title derivation, but `DeferStart=true` persists only the Thread and no Run fingerprint. After upload, the writer sends a separate typed turn containing the original message/config and the ordered uploaded file IDs.

Optional means absent, never explicit `null`. Required lists remain present when empty. Duplicate list elements are invalid. Input order is preserved; the server must not sort or convert a list to a set.

`metadata` is genuinely optional for third-party V2 callers. When absent, the server maps it to `{}` and does not try to infer whether the caller is C3i2. The conditional values in the matrix are first-party writer requirements enforced by C3i2 payload snapshots, not hidden server-side caller detection.

When `metadata` is present, its `source` is also closed by flow: atomic/deferred initial accepts only `workbench_new_task`; a turn accepts only `workbench_new_task` or `workbench_detail_followup`; a retry accepts only `task_retry`. A mismatched source is an invalid typed submission even though the string belongs to the global enum. This rule applies equally to third-party and first-party V2 callers.

Skill presence is canonical:

- `skills.enabled=true`, `composer.explicit_enable_skills` absent: no explicit Skill selection;
- `skills.enabled=true`, non-empty `composer.explicit_enable_skills`: explicit ordered selection and each item must appear in `allowed_skills`;
- `skills.enabled=true`, present-empty `composer.explicit_enable_skills`: invalid; use absence;
- `skills.enabled=false`: `explicit_enable_skills` must be present-empty; `allowed_skills` preserves the ordered configured set and may be empty or non-empty.

This pair is the exact current two-set model: `explicit_enable_skills` is the ordered explicit subset currently emitted as Config `enable_skills`, while `allowed_skills` is the ordered complete set currently emitted as `skills.allowed_skills`. No third Skill list exists in V2 and neither list is derived from the other.

MCP presence is canonical:

- `mcp_tools.enabled=true`, `allowed_mcp_tools=[]`: `enable_mcp` must also be empty and means auto-discovery; persisted `mcp_tools.allowed_tools` is absent;
- `mcp_tools.enabled=true`, non-empty `allowed_mcp_tools`: `enable_mcp` must be the exact same ordered list; both the flat selection and nested allowlist are persisted;
- `mcp_tools.enabled=false`: `enable_mcp` must be empty; `allowed_mcp_tools` preserves the ordered configured set and is persisted as `mcp_tools.allowed_tools`, including an explicit empty array.

This is the exact current two-list MCP model. `enable_mcp` is the active flat selection, while `allowed_mcp_tools` is the configured nested allowlist retained while the feature is disabled. Neither list is inferred from arbitrary Config.

`model_retry` and `model_failover` are absent when disabled. All other lists in V2 are required and retain an empty array. No V2 `context`, client-submitted `message_metadata`, client lineage metadata, reasoning setting, or generic metadata object exists. The five first-party writers currently emit no non-empty Run Context. During compatibility the server may derive the existing turn Message metadata copy from typed Config; the client cannot supply or diverge it.

## 5. Typed-to-application mapping

Composer is the sole V2 authority for model/resource selection. Run Config is the sole authority for runtime behavior. The server reconstructs the existing internal Config object deterministically:

| V2 path | Internal persisted Config path |
| --- | --- |
| `composer.model_type` | `model_type` JSON number |
| `composer.model_name` | `model_name` |
| `composer.explicit_enable_skills` | optional `enable_skills` with presence preserved |
| `composer.allowed_skills` | `skills.allowed_skills` |
| `composer.enable_mcp` | flat `enable_mcp` |
| `composer.allowed_mcp_tools` | `mcp_tools.allowed_tools` with the presence rule above |
| `composer.enable_kbs` | `enable_kbs` |
| `composer.enable_databases` | `enable_databases` |
| `config.runtime` | `runtime` |
| other `config.*` | same snake_case nested path |

No composer field is duplicated in `CanonicalRunConfigV2`, so conflict precedence does not exist. The mapper produces canonical JSON using the existing server-side Config admission and normalization; it does not expose an `any` field publicly.

The assistant identity is not a model selection. For typed Create Thread the server injects the existing public alias `agent`. Typed Create/Wait/Stream Run continues to use the existing required route field `assistant_id`; C3i2 always sends exactly `agent`, and the server rejects every other value before source lookup. `assistant_id` never overrides `composer.model_type` or `composer.model_name`.

For a turn, the mapper builds exactly one user input message, sets `MessageContent`, and preserves ordered uploaded IDs. It derives one canonical Config JSON string and supplies that exact string as both the pre-application `Config` and `MessageMetadata` values for Create/Wait/Stream turns. This preserves the existing first-party fingerprint and the existing semantic persisted copy without accepting client Message metadata. The application may perform its existing Config normalization after this boundary; Message metadata retains the same pre-application copy as V1. Atomic initial Message metadata remains the existing server-fixed `{"source":"workbench_new_task"}` and does not participate in the initial fingerprint. For a retry, the mapper builds Run input from `input.message`, sets `TopLevelRetrySourceRunID`, leaves `MessageContent` and `MessageMetadata` empty, and therefore cannot append a User Message. Retry source Thread/status authorization stays server-owned. Retiring the server-derived turn metadata copy is a later compatibility cleanup, not part of C3i2.

Run metadata maps only the safe `source`. A first-party top-level retry sends exactly `{"source":"task_retry"}`; the typed lineage carries the source Run ID and the caller must not duplicate it in metadata. The application injects exactly the protected `attempt_kind=retry` and numeric `source_run_id`; it does not synthesize `source_thread_id` or `requested_at`. C3i2 intentionally removes those two legacy first-party diagnostic fields: the former duplicates the authorized path Thread and the latter is replaced by Run `created_at` as the authoritative submission time. This is an approved V2 persisted/public metadata normalization difference, not a claim of full metadata parity with historical V1 retries.

V2 uses the existing semantic idempotency operations and fingerprint builders, not a parallel namespace. The transport version does not create a new persistence namespace: the existing fingerprint payload keeps `Version="v1"`. Empty Command, Metadata, Config, and Context components continue to normalize to the literal object string `{}` through `canonicalRunFingerprintJSONObject`.

The exact compatibility mapping is:

| Flow | Existing fingerprint builder and exact V2 components |
| --- | --- |
| Atomic initial | When an optional header key is present, call `canonicalInitialThreadRunRequestFingerprint` with `Version="v1"`, operation `workbench.thread.initial_run.v1`, canonical Thread metadata, validated title/source, `assistant_id="agent"`, trimmed message, mapped Config, empty Context `{}`, and canonical optional Run metadata. The server-fixed Message metadata is not a component. C3i2 preserves the current first-party behavior of sending no key. |
| Deferred initial | Validate the same typed initial values but preserve the current behavior: no Run exists, no idempotency operation/fingerprint is persisted, and C3i2 sends no key. The later post-upload Run is a separate turn operation. |
| Turn | Call `canonicalRunTurnRequestFingerprint` with `Version="v1"`, operation `workbench.run.turn.v1`, assistant `agent`, trimmed message, Message metadata equal to the mapped Config string, ordered uploaded file IDs, canonical source metadata, mapped Config, empty Context `{}`, and the existing validated/default route options. `Command` is prohibited by the V2 route contract and is not a turn fingerprint component. |
| Retry | Call `canonicalRunRetryRequestFingerprint` with `Version="v1"`, operation `workbench.run.retry.v1`, the typed source Run ID, assistant `agent`, trimmed Run input message, empty attachment list, empty Command/Context objects, first-party metadata `{"source":"task_retry"}`, mapped Config, and the existing validated/default route options. The mapper separately guarantees empty `MessageMetadata`, but it is not a retry fingerprint component. |
| Resume | Call `canonicalResumeRequestFingerprint` with `Version="v1"`, operation `workbench.run.resume.v1`, the path source Run ID, existing body `interrupt_id`, and the normalized typed response. |

The parity corpus compares the compatibility components in this table, not arbitrary historical V1 metadata. A V2 retry keeps the existing deterministic key `${space_id}:${thread_id}:${source_run_id}:task_retry`; it must not add a `:v2` namespace. Repeating the same V2 retry payload under that key replays, while changing a typed semantic field conflicts. A historical V1 retry carrying volatile or redundant metadata is not byte-equivalent to V2: if such a V1 record already owns the same key, the V2 request returns 409 `idempotency_conflict` with no new Run. The client then refreshes authoritative state and must not rotate the key or automatically resubmit. More generally, an ambiguous request is retried with its captured payload version and key; the client never silently converts an in-flight V1 mutation to V2 under the same key.

The attachment flow has two distinct operations. Deferred Thread creation validates the no-attachment submission but persists no Run idempotency operation/fingerprint; C3i2 preserves today's absence of a client `Idempotency-Key` on that call. The post-upload Create Run uses the turn operation/fingerprint and a new `${thread_id}:${request_uuid}:new-task` key. The two steps never share a key or fingerprint. The Run key is reused only according to the existing in-flight/ambiguous-result rules; a failed upload never consumes or sends it.

Resume C3i2 creates one key in the form `human-resume:<request_uuid>` at the action/orchestration layer, once per user-intended normalized response—not once per HTTP call. Its page-scoped attempt identity captures `(space, thread, source_run, interrupt_id, response_v2)` so the exact key and normalized payload are reused after any outcome that may have reached the server, including timeout, network/5xx, response-read/decode failure, or an abort not proven pre-dispatch. This client identity is not the server fingerprint: `canonicalResumeRequestFingerprint` remains exactly the table contract above and hashes only its existing source Run, interrupt, and normalized response components; workspace/principal scoping remains outside that hash. A changed response or pending-interaction identity creates a new key. A confirmed 2xx clears the attempt only after the returned Run is committed locally, before refresh; a proven pre-dispatch cancellation or definitive non-409 4xx rejection may clear it. A 409 triggers authoritative refresh and retains the captured attempt, key, and normalized payload until the task scope, pending-interaction identity, or response changes, or that refresh proves an existing Run already resolved the mutation; it must not rotate the key or automatically resubmit. Task-scope change clears the page attempt. This supplements the existing successful-mutation guard and stays below the 128-byte header cap; persistence across a full page reload is deferred.

## 6. Route mixing and validation order

### 6.1 Version mixing

Create Thread rules:

- `initial_submission_v2` and `deferred_initial_submission_v2` are mutually exclusive;
- either V2 field is mutually exclusive with V1 `coze.initial_run` and `coze.deferred_initial_run`;
- existing Thread `metadata` may coexist because it remains the closed title/source Thread metadata contract, not an execution overflow;
- `ttl`, `supersteps`, and unsupported `if_exists` retain their existing errors.

Create/Wait/Stream Run rules:

- when `submission_v2` is present, legacy payload fields `input`, `command`, `metadata`, `config`, `context`, and `coze` must be absent, not `null`;
- existing safe route options (`assistant_id`, supported stream modes, `multitask_strategy`, `on_disconnect`, `durability`, `if_not_exists`, `raise_error`, headers) retain current validation;
- turn plus lineage, retry without lineage, retry plus attachments, and any V1/V2 mix return 422 before source lookup or mutation.

Resume rules:

- exactly one of V1 `response` and V2 `response_v2` is present;
- V2 still requires the existing field 3 `interrupt_id` as a non-empty body identifier. It identifies the runtime interrupt, while `response_v2.interaction_id` identifies the selected Human Interaction prompt, so neither substitutes for nor must equal the other;
- C3i2 writes only `response_v2` and the generated `Idempotency-Key` header;
- body lineage, composer, config, context, and source Run fields are unsupported; source Run remains the path `run_id`;
- clarification requires `decision=answered`; `comment` is prohibited; `answer` and `choice_id` are independently optional, may both be present, and at least one must be present and non-empty after trimming;
- confirmation requires `decision=approved` or `decision=rejected`; `answer` and `choice_id` are prohibited; `comment` is optional and, when present, must be non-empty after trimming;
- all absent Human response optionals must be omitted rather than sent as `null` or an empty string. The normalized response used for persistence and fingerprinting contains only fields allowed by its `kind`.

### 6.2 Validation order

The established priority is preserved:

1. required service/dependency availability;
2. authenticated principal and workspace authorization;
3. route path ID parsing where applicable;
4. the 1 MiB transport body cap;
5. existing raw retired-control scan;
6. strict raw V2 field/presence/union validation, before a binder can erase unknown fields;
7. typed-to-application mapping and semantic fingerprint creation;
8. source/upload/checkpoint reads and application authorization;
9. mutation or SSE writer startup.

Create Thread has no path step. Create, Wait, and Stream share one strict Run parser; Stream cannot start its writer before parsing succeeds. C3i1 includes all server acceptance, mapping, fingerprint, and error behavior above. C3i2 changes only first-party callers.

## 7. Numeric, text, and collection budgets

Budgets are byte limits unless stated otherwise:

- whole request body: existing 1 MiB;
- Run message: valid UTF-8, trimmed non-empty, at most 256 KiB;
- Human response: existing 16 KiB canonical JSON; `answer` and `comment` at most 8 KiB each;
- `interrupt_id` and `interaction_id`: 1–191 bytes and the identifier grammar below; optional `choice_id`, when present, has the same bound and grammar;
- schema version, response schema, kind, decision, source, scope, and visibility strings: at most 64 bytes before exact allowlist validation;
- idempotency header: existing 128 bytes;
- identifiers and model name: 1–191 bytes; resource identifiers use `^[A-Za-z0-9_.:-]+$` and current sensitive-value rejection;
- allowed host: 1–253 bytes, host only—no URL, credentials, absolute path, query, or fragment;
- file references: at most 10, unique positive `i64` decimal IDs;
- resource/Skill/MCP/knowledge/database/failover lists: at most 256 unique values each;
- scopes: 1–3 unique values from `thread`, `run`, `long_term`;
- allowed hosts: at most 64 unique normalized hosts;
- finish reasons: at most 16 unique identifiers of at most 64 bytes each;
- memory `limit` and `candidate_limit`: 1–100 and candidate must be at least limit; `min_confidence`: finite 0–1;
- web HTTP timeout: 1,000–60,000 ms; response bytes: 1 KiB–1 MiB; enabled HTTP requires at least one host;
- search results: 1–10; `web_tools.enabled` must equal `http.enabled || search.enabled`;
- retry/failover `max_retries`: 1–5; retry backoff: 0–60,000 ms; failover retries cannot exceed candidate count;
- model IDs: positive integers no greater than `9007199254740991`; file IDs remain positive signed 64-bit decimal strings. Non-safe/non-finite frontend model values and JSON `null` are invalid.

Runtime is exactly `eino_adk`; all visibility fields are exactly `deferred`. Sensitive keys, credentials, bearer values, provider payloads, URLs outside the dedicated host field, and absolute paths are rejected. Values are rejected, not trimmed into a different semantic value or redacted into persistence.

## 8. Stable public errors

- malformed outer JSON remains 400 `invalid_json`;
- unknown V2 root/nested fields return 422 `unsupported_sdk_field`, class `unsupported_field`, retryable false, and only a safe canonical path such as `submission_v2.config.web_tools.foo`;
- invalid schema, null/presence, bounds, or turn/retry shape returns 422 `invalid_request`, class `invalid_typed_submission`, retryable false, with a safe field path and no submitted value;
- V1/V2 or initial/deferred mixing returns 422 `invalid_request`, class `mixed_submission_versions`, retryable false;
- retired controls keep 422 `unsupported_execution_control`, retryable false, and their current canonical path priority;
- source authorization/not-found/conflict and idempotency replay/conflict retain existing public mappings after typed validation.

Unknown, invalid, or mixed V2 input never silently falls back to V1. Ordinary message text and unrelated business strings are not recursively scanned for coincidental words such as `mode`.

The public error schema is unchanged. A safe canonical field path appears only inside the existing `detail` string; C3i1 does not add a separate `path` property. The detail never includes the submitted value.

## 9. First-party flows

- **Root without attachments:** one atomic create-Thread call with `initial_submission_v2`.
- **Root with attachments:** create deferred Thread with `deferred_initial_submission_v2`, upload, then Create Run with a typed turn and uploaded IDs in response order. Upload failure starts no Run.
- **Follow-up:** upload first, then typed turn; an ambiguous failure reuses the same key according to current Task Detail rules.
- **Top-level retry:** typed retry with the deterministic key and source lineage; input prompt is retained for the Run, but no User Message is appended.
- **Resume:** existing command-shaped route with `response_v2` plus the generated `Idempotency-Key`; no composer or lineage is synthesized.

All flows remain behind the singleton `canonicalThreadClient`. Pages do not call generated transports directly and do not add a second serializer.

## 10. Verification gates

### 10.1 C3i1

- Pinned IDL generation produces the exact Go/TypeScript V2 types and request body/header mappings; `backend/scripts/verify_api_codegen.sh` is green.
- Generated TypeScript snapshots prove scalar `i64` model/file IDs are strings and `candidate_model_ids` is `string[]`; the mapper validates every decimal before converting each model ID to exact internal JSON integer digits.
- New V2 structs contain no `any`, map, raw JSON, generic extension, or retired field; existing V1 compatibility fields are not misreported as V2 escape hatches.
- Strict parser tables cover every root and nested field: unknown, duplicate, missing, explicit null, empty, overflow, invalid UTF-8, trailing JSON, version mix, and stable error path.
- Handler tests cover initial/deferred Thread, Create/Wait/Stream turn/retry, typed Resume, auth/path priority, source lookup priority, and zero side effects on rejection.
- The semantic parity corpus maps old serializer output to V2 and back to internal Config for: primary model, optional/empty Skill selection, MCP auto-discovery/disabled/explicit, knowledge bases, databases, memory, web tools, retry, failover order, Token Usage, and all absence rules.
- Request-level parity separately proves ordered attachment references and equality for the exact compatibility components in Section 5; attachments are not misdescribed as Config fields. The historical-V1-to-V2 retry conflict exception remains intentional and is not mislabeled as replay parity.
- Retry tests prove same-Thread authority, source ID in fingerprint, Run input retained, and no Message/MessageMetadata persistence.
- Retry tests also prove a historical V1 fingerprint plus the same deterministic V2 key returns 409 without creating a Run and that the writer refreshes without rotating or automatically resubmitting.
- Resume tests prove typed response equivalence, required distinct interrupt/interaction identifiers in the existing fingerprint, timeout/5xx reuse the same body and key, a changed answer allocates a new key, success followed by refresh failure does not write twice, 409 does not rotate or automatically resubmit, and no body lineage is accepted.

Any parity loss keeps C3i2 locked.

### 10.2 C3i2

- Payload snapshots for all five flows contain only their V2 field plus existing safe route fields/headers.
- Root atomicity, deferred upload-before-run, follow-up upload ordering, retry no-message behavior, and Resume header remain green.
- Task Detail generation, stale-response, duplicate-mutation, and ambiguous idempotency protections remain green.
- A structural scan finds no first-party V2 writer using legacy `input/config/context/metadata/coze/response:any` as an execution overflow or emitting any retired control.
- Focused Vitest, generated-contract test, lint, typecheck, and production frontend build pass.

## 11. Explicitly deferred and authority status

This package does not add a URL, migration, UI control, Human Journal Attempt rollover, ordinary non-Journal enrollment, legacy decoder, gate-on producer, P1D/P2 loop, MySQL race implementation, or complete V1 retirement.

C3i1 authority must say “typed contract accepted, first-party writers still V1.” C3i2 may say “five first-party writers use V2.” Neither may claim Human Resume closure, all legacy consumers removed, real MySQL typed recovery race verified, or P1M PASS.

The two user-owned untracked P1L/C2 plan files remain outside this design and must not be read, edited, staged, deleted, or committed.
