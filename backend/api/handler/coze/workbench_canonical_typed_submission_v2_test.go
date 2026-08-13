/*
 * Copyright 2025 coze-dev Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package coze

import (
	"bytes"
	"strings"
	"testing"

	hertzconsts "github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/stretchr/testify/require"
)

const canonicalStrictRunV2 = `{
  "schema_version":"coze.workbench.run_submission.v2",
  "kind":"turn",
  "input":{"message":"hello","uploaded_files":[]},
  "composer":{"allowed_skills":[],"enable_mcp":[],"enable_kbs":[],"enable_databases":[],"allowed_mcp_tools":[]},
  "config":{
    "runtime":"eino_adk",
    "memory_retrieval":{"limit":1,"candidate_limit":1,"scopes":["thread"],"min_confidence":0.5},
    "skills":{"enabled":false,"visibility":"deferred"},
    "mcp_tools":{"enabled":false,"visibility":"deferred"},
    "web_tools":{
      "enabled":false,
      "visibility":"deferred",
      "http":{"enabled":false,"allowed_hosts":[],"timeout_ms":1000,"max_response_bytes":1024},
      "search":{"enabled":false,"max_results":1}
    },
    "token_usage":{"enabled":true}
  }
}`

const canonicalStrictInitialV2 = `{
  "schema_version":"coze.workbench.initial_run_submission.v2",
  "input":{"message":"hello","uploaded_files":[]},
  "composer":{"allowed_skills":[],"enable_mcp":[],"enable_kbs":[],"enable_databases":[],"allowed_mcp_tools":[]},
  "config":{
    "runtime":"eino_adk",
    "memory_retrieval":{"limit":1,"candidate_limit":1,"scopes":["thread"],"min_confidence":0.5},
    "skills":{"enabled":false,"visibility":"deferred"},
    "mcp_tools":{"enabled":false,"visibility":"deferred"},
    "web_tools":{
      "enabled":false,
      "visibility":"deferred",
      "http":{"enabled":false,"allowed_hosts":[],"timeout_ms":1000,"max_response_bytes":1024},
      "search":{"enabled":false,"max_results":1}
    },
    "token_usage":{"enabled":true}
  }
}`

const canonicalStrictHumanV2 = `{
  "schema":"coze.human_interaction_response.v1",
  "interaction_id":"interaction-7",
  "kind":"clarification",
  "decision":"answered",
  "answer":"continue"
}`

func TestDecodeCanonicalTypedRunSubmissionV2PreservesGeneratedValueAndPresence(t *testing.T) {
	body := canonicalTypedV2Replace(
		canonicalStrictRunV2,
		`"uploaded_files":[]`,
		`"uploaded_files":[{"file_id":"7"}]`,
	)
	body = canonicalTypedV2Replace(
		body,
		`"composer":{"allowed_skills":`,
		`"composer":{"model_type":"17","allowed_skills":`,
	)
	got, public := decodeCanonicalTypedRunSubmissionV2([]byte(body))
	require.Nil(t, public)
	require.NotNil(t, got)
	require.Equal(t, "coze.workbench.run_submission.v2", got.Value.SchemaVersion)
	require.Equal(t, "turn", got.Value.Kind)
	require.NotNil(t, got.Value.Composer.ModelType)
	require.EqualValues(t, 17, *got.Value.Composer.ModelType)
	require.Len(t, got.Value.Input.UploadedFiles, 1)
	require.EqualValues(t, 7, got.Value.Input.UploadedFiles[0].FileID)
	requireCanonicalTypedV2Presence(t, got.Presence, "submission_v2.schema_version", true)
	requireCanonicalTypedV2Presence(t, got.Presence, "submission_v2.input.uploaded_files", true)
	requireCanonicalTypedV2Presence(t, got.Presence, "submission_v2.composer.explicit_enable_skills", false)
	requireCanonicalTypedV2Presence(t, got.Presence, "submission_v2.lineage", false)
	requireCanonicalTypedV2Presence(t, got.Presence, "submission_v2.metadata", false)
}

func TestDecodeCanonicalTypedRunSubmissionV2RejectsClosedShapeViolations(t *testing.T) {
	withOptionals := canonicalStrictRunV2WithOptionals()
	tests := []struct {
		name   string
		body   []byte
		path   string
		code   string
		class  string
		status int
	}{
		{
			name: "malformed object",
			body: []byte(`{"schema_version":`),
			code: "invalid_json", class: "invalid_json", status: hertzconsts.StatusBadRequest,
		},
		{
			name: "trailing object",
			body: []byte(canonicalStrictRunV2 + `{}`),
			code: "invalid_json", class: "trailing_json", status: hertzconsts.StatusBadRequest,
		},
		{
			name: "root must be object",
			body: []byte(`[]`), path: "submission_v2",
			code: "invalid_request", class: "invalid_typed_submission", status: hertzconsts.StatusUnprocessableEntity,
		},
		{
			name: "unknown root",
			body: canonicalTypedV2ReplaceBytes(canonicalStrictRunV2, `"schema_version":`, `"foo":"secret-root-value","schema_version":`),
			path: "submission_v2.foo", code: "unsupported_sdk_field", class: "unsupported_field", status: hertzconsts.StatusUnprocessableEntity,
		},
		{
			name: "unknown input",
			body: canonicalTypedV2ReplaceBytes(canonicalStrictRunV2, `"input":{"message":`, `"input":{"foo":"secret-input-value","message":`),
			path: "submission_v2.input.foo", code: "unsupported_sdk_field", class: "unsupported_field", status: hertzconsts.StatusUnprocessableEntity,
		},
		{
			name: "unknown uploaded file",
			body: canonicalTypedV2ReplaceBytes(canonicalStrictRunV2, `"uploaded_files":[]`, `"uploaded_files":[{"file_id":"7","foo":"secret-file-value"}]`),
			path: "submission_v2.input.uploaded_files[0].foo", code: "unsupported_sdk_field", class: "unsupported_field", status: hertzconsts.StatusUnprocessableEntity,
		},
		{
			name: "unknown composer",
			body: canonicalTypedV2ReplaceBytes(canonicalStrictRunV2, `"composer":{"allowed_skills":`, `"composer":{"foo":"secret-composer-value","allowed_skills":`),
			path: "submission_v2.composer.foo", code: "unsupported_sdk_field", class: "unsupported_field", status: hertzconsts.StatusUnprocessableEntity,
		},
		{
			name: "unknown config",
			body: canonicalTypedV2ReplaceBytes(canonicalStrictRunV2, `"runtime":"eino_adk",`, `"foo":"secret-config-value","runtime":"eino_adk",`),
			path: "submission_v2.config.foo", code: "unsupported_sdk_field", class: "unsupported_field", status: hertzconsts.StatusUnprocessableEntity,
		},
		{
			name: "unknown memory retrieval",
			body: canonicalTypedV2ReplaceBytes(canonicalStrictRunV2, `"memory_retrieval":{"limit":`, `"memory_retrieval":{"foo":"secret-memory-value","limit":`),
			path: "submission_v2.config.memory_retrieval.foo", code: "unsupported_sdk_field", class: "unsupported_field", status: hertzconsts.StatusUnprocessableEntity,
		},
		{
			name: "unknown skills",
			body: canonicalTypedV2ReplaceBytes(canonicalStrictRunV2, `"skills":{"enabled":`, `"skills":{"foo":"secret-skills-value","enabled":`),
			path: "submission_v2.config.skills.foo", code: "unsupported_sdk_field", class: "unsupported_field", status: hertzconsts.StatusUnprocessableEntity,
		},
		{
			name: "unknown mcp tools",
			body: canonicalTypedV2ReplaceBytes(canonicalStrictRunV2, `"mcp_tools":{"enabled":`, `"mcp_tools":{"foo":"secret-mcp-value","enabled":`),
			path: "submission_v2.config.mcp_tools.foo", code: "unsupported_sdk_field", class: "unsupported_field", status: hertzconsts.StatusUnprocessableEntity,
		},
		{
			name: "unknown web tools",
			body: canonicalTypedV2ReplaceBytes(canonicalStrictRunV2, `"web_tools":{`, `"web_tools":{"foo":"secret-web-value",`),
			path: "submission_v2.config.web_tools.foo", code: "unsupported_sdk_field", class: "unsupported_field", status: hertzconsts.StatusUnprocessableEntity,
		},
		{
			name: "unknown web http",
			body: canonicalTypedV2ReplaceBytes(canonicalStrictRunV2, `"http":{"enabled":`, `"http":{"foo":"secret-http-value","enabled":`),
			path: "submission_v2.config.web_tools.http.foo", code: "unsupported_sdk_field", class: "unsupported_field", status: hertzconsts.StatusUnprocessableEntity,
		},
		{
			name: "unknown web search",
			body: canonicalTypedV2ReplaceBytes(canonicalStrictRunV2, `"search":{"enabled":`, `"search":{"foo":"secret-search-value","enabled":`),
			path: "submission_v2.config.web_tools.search.foo", code: "unsupported_sdk_field", class: "unsupported_field", status: hertzconsts.StatusUnprocessableEntity,
		},
		{
			name: "unknown model retry",
			body: canonicalTypedV2ReplaceBytes(withOptionals, `"model_retry":{"max_retries":`, `"model_retry":{"foo":"secret-retry-value","max_retries":`),
			path: "submission_v2.config.model_retry.foo", code: "unsupported_sdk_field", class: "unsupported_field", status: hertzconsts.StatusUnprocessableEntity,
		},
		{
			name: "unknown model failover",
			body: canonicalTypedV2ReplaceBytes(withOptionals, `"model_failover":{"candidate_model_ids":`, `"model_failover":{"foo":"secret-failover-value","candidate_model_ids":`),
			path: "submission_v2.config.model_failover.foo", code: "unsupported_sdk_field", class: "unsupported_field", status: hertzconsts.StatusUnprocessableEntity,
		},
		{
			name: "unknown token usage",
			body: canonicalTypedV2ReplaceBytes(canonicalStrictRunV2, `"token_usage":{"enabled":`, `"token_usage":{"foo":"secret-token-value","enabled":`),
			path: "submission_v2.config.token_usage.foo", code: "unsupported_sdk_field", class: "unsupported_field", status: hertzconsts.StatusUnprocessableEntity,
		},
		{
			name: "unknown lineage",
			body: canonicalTypedV2ReplaceBytes(withOptionals, `"lineage":{"source_run_id":`, `"lineage":{"foo":"secret-lineage-value","source_run_id":`),
			path: "submission_v2.lineage.foo", code: "unsupported_sdk_field", class: "unsupported_field", status: hertzconsts.StatusUnprocessableEntity,
		},
		{
			name: "unknown metadata",
			body: canonicalTypedV2ReplaceBytes(withOptionals, `"metadata":{"source":`, `"metadata":{"foo":"secret-metadata-value","source":`),
			path: "submission_v2.metadata.foo", code: "unsupported_sdk_field", class: "unsupported_field", status: hertzconsts.StatusUnprocessableEntity,
		},
		{
			name: "duplicate root field",
			body: canonicalTypedV2ReplaceBytes(canonicalStrictRunV2, `"kind":"turn",`, `"kind":"turn","kind":"turn",`),
			path: "submission_v2.kind", code: "invalid_request", class: "invalid_typed_submission", status: hertzconsts.StatusUnprocessableEntity,
		},
		{
			name: "duplicate nested field",
			body: canonicalTypedV2ReplaceBytes(canonicalStrictRunV2, `"runtime":"eino_adk",`, `"runtime":"eino_adk","runtime":"eino_adk",`),
			path: "submission_v2.config.runtime", code: "invalid_request", class: "invalid_typed_submission", status: hertzconsts.StatusUnprocessableEntity,
		},
		{
			name: "duplicate array element field",
			body: canonicalTypedV2ReplaceBytes(canonicalStrictRunV2, `"uploaded_files":[]`, `"uploaded_files":[{"file_id":"7","file_id":"8"}]`),
			path: "submission_v2.input.uploaded_files[0].file_id", code: "invalid_request", class: "invalid_typed_submission", status: hertzconsts.StatusUnprocessableEntity,
		},
		{
			name: "missing root required field",
			body: canonicalTypedV2ReplaceBytes(canonicalStrictRunV2, `"kind":"turn",`, ``),
			path: "submission_v2.kind", code: "invalid_request", class: "invalid_typed_submission", status: hertzconsts.StatusUnprocessableEntity,
		},
		{
			name: "missing nested required field",
			body: canonicalTypedV2ReplaceBytes(canonicalStrictRunV2, `,"uploaded_files":[]`, ``),
			path: "submission_v2.input.uploaded_files", code: "invalid_request", class: "invalid_typed_submission", status: hertzconsts.StatusUnprocessableEntity,
		},
		{
			name: "missing config required field",
			body: canonicalTypedV2ReplaceBytes(canonicalStrictRunV2, `"runtime":"eino_adk",`, ``),
			path: "submission_v2.config.runtime", code: "invalid_request", class: "invalid_typed_submission", status: hertzconsts.StatusUnprocessableEntity,
		},
		{
			name: "null required field",
			body: canonicalTypedV2ReplaceBytes(canonicalStrictRunV2, `"uploaded_files":[]`, `"uploaded_files":null`),
			path: "submission_v2.input.uploaded_files", code: "invalid_request", class: "invalid_typed_submission", status: hertzconsts.StatusUnprocessableEntity,
		},
		{
			name: "null optional scalar",
			body: canonicalTypedV2ReplaceBytes(canonicalStrictRunV2, `"allowed_skills":[]`, `"model_name":null,"allowed_skills":[]`),
			path: "submission_v2.composer.model_name", code: "invalid_request", class: "invalid_typed_submission", status: hertzconsts.StatusUnprocessableEntity,
		},
		{
			name: "null optional list",
			body: canonicalTypedV2ReplaceBytes(canonicalStrictRunV2, `"allowed_skills":[]`, `"explicit_enable_skills":null,"allowed_skills":[]`),
			path: "submission_v2.composer.explicit_enable_skills", code: "invalid_request", class: "invalid_typed_submission", status: hertzconsts.StatusUnprocessableEntity,
		},
		{
			name: "null optional object",
			body: canonicalTypedV2ReplaceBytes(canonicalStrictRunV2, `"token_usage":`, `"model_retry":null,"token_usage":`),
			path: "submission_v2.config.model_retry", code: "invalid_request", class: "invalid_typed_submission", status: hertzconsts.StatusUnprocessableEntity,
		},
		{
			name: "null optional lineage",
			body: canonicalTypedV2ReplaceBytes(canonicalStrictRunV2, `"kind":"turn",`, `"kind":"turn","lineage":null,`),
			path: "submission_v2.lineage", code: "invalid_request", class: "invalid_typed_submission", status: hertzconsts.StatusUnprocessableEntity,
		},
		{
			name: "wrong collection kind",
			body: canonicalTypedV2ReplaceBytes(canonicalStrictRunV2, `"allowed_skills":[]`, `"allowed_skills":{}`),
			path: "submission_v2.composer.allowed_skills", code: "invalid_request", class: "invalid_typed_submission", status: hertzconsts.StatusUnprocessableEntity,
		},
		{
			name: "invalid utf8 string",
			body: canonicalTypedV2InvalidUTF8(canonicalStrictRunV2, "hello"),
			path: "submission_v2.input.message", code: "invalid_request", class: "invalid_typed_submission", status: hertzconsts.StatusUnprocessableEntity,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, public := decodeCanonicalTypedRunSubmissionV2(tt.body)
			requireCanonicalTypedV2Error(t, public, tt.status, tt.code, tt.class, tt.path)
		})
	}
}

func TestDecodeCanonicalTypedInitialSubmissionV2PreservesGeneratedValueAndPresence(t *testing.T) {
	for _, field := range []string{"initial_submission_v2", "deferred_initial_submission_v2"} {
		t.Run(field, func(t *testing.T) {
			got, public := decodeCanonicalTypedInitialSubmissionV2([]byte(canonicalStrictInitialV2), field)
			require.Nil(t, public)
			require.NotNil(t, got)
			require.Equal(t, "coze.workbench.initial_run_submission.v2", got.Value.SchemaVersion)
			requireCanonicalTypedV2Presence(t, got.Presence, field+".schema_version", true)
			requireCanonicalTypedV2Presence(t, got.Presence, field+".input.uploaded_files", true)
			requireCanonicalTypedV2Presence(t, got.Presence, field+".metadata", false)
		})
	}
}

func TestDecodeCanonicalTypedInitialSubmissionV2RejectsClosedShapeViolations(t *testing.T) {
	tests := []struct {
		name, field, path, code, class string
		body                           []byte
		status                         int
	}{
		{
			name: "unknown nested", field: "initial_submission_v2",
			body: canonicalTypedV2ReplaceBytes(canonicalStrictInitialV2, `"config":{`, `"config":{"foo":"secret-initial-value",`),
			path: "initial_submission_v2.config.foo", code: "unsupported_sdk_field", class: "unsupported_field", status: hertzconsts.StatusUnprocessableEntity,
		},
		{
			name: "duplicate required", field: "initial_submission_v2",
			body: canonicalTypedV2ReplaceBytes(canonicalStrictInitialV2, `"schema_version":"coze.workbench.initial_run_submission.v2",`, `"schema_version":"coze.workbench.initial_run_submission.v2","schema_version":"coze.workbench.initial_run_submission.v2",`),
			path: "initial_submission_v2.schema_version", code: "invalid_request", class: "invalid_typed_submission", status: hertzconsts.StatusUnprocessableEntity,
		},
		{
			name: "missing required", field: "deferred_initial_submission_v2",
			body: canonicalTypedV2ReplaceBytes(canonicalStrictInitialV2, `"input":{"message":"hello","uploaded_files":[]},`, ``),
			path: "deferred_initial_submission_v2.input", code: "invalid_request", class: "invalid_typed_submission", status: hertzconsts.StatusUnprocessableEntity,
		},
		{
			name: "null optional", field: "deferred_initial_submission_v2",
			body: canonicalTypedV2ReplaceBytes(canonicalStrictInitialV2, `"config":{`, `"metadata":null,"config":{`),
			path: "deferred_initial_submission_v2.metadata", code: "invalid_request", class: "invalid_typed_submission", status: hertzconsts.StatusUnprocessableEntity,
		},
		{
			name: "invalid utf8", field: "initial_submission_v2",
			body: canonicalTypedV2InvalidUTF8(canonicalStrictInitialV2, "hello"),
			path: "initial_submission_v2.input.message", code: "invalid_request", class: "invalid_typed_submission", status: hertzconsts.StatusUnprocessableEntity,
		},
		{
			name: "trailing object", field: "initial_submission_v2",
			body: []byte(canonicalStrictInitialV2 + `{}`),
			code: "invalid_json", class: "trailing_json", status: hertzconsts.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, public := decodeCanonicalTypedInitialSubmissionV2(tt.body, tt.field)
			requireCanonicalTypedV2Error(t, public, tt.status, tt.code, tt.class, tt.path)
		})
	}
}

func TestDecodeCanonicalTypedHumanResponseV2PreservesGeneratedValueAndPresence(t *testing.T) {
	got, public := decodeCanonicalTypedHumanResponseV2([]byte(canonicalStrictHumanV2))
	require.Nil(t, public)
	require.NotNil(t, got)
	require.Equal(t, "coze.human_interaction_response.v1", got.Value.Schema)
	require.Equal(t, "interaction-7", got.Value.InteractionID)
	requireCanonicalTypedV2Presence(t, got.Presence, "response_v2.answer", true)
	requireCanonicalTypedV2Presence(t, got.Presence, "response_v2.choice_id", false)
	requireCanonicalTypedV2Presence(t, got.Presence, "response_v2.comment", false)
}

func TestDecodeCanonicalTypedHumanResponseV2RejectsClosedShapeViolations(t *testing.T) {
	tests := []struct {
		name, path, code, class string
		body                    []byte
		status                  int
	}{
		{
			name: "unknown root",
			body: canonicalTypedV2ReplaceBytes(canonicalStrictHumanV2, `"schema":`, `"foo":"secret-human-value","schema":`),
			path: "response_v2.foo", code: "unsupported_sdk_field", class: "unsupported_field", status: hertzconsts.StatusUnprocessableEntity,
		},
		{
			name: "duplicate required",
			body: canonicalTypedV2ReplaceBytes(canonicalStrictHumanV2, `"decision":"answered",`, `"decision":"answered","decision":"answered",`),
			path: "response_v2.decision", code: "invalid_request", class: "invalid_typed_submission", status: hertzconsts.StatusUnprocessableEntity,
		},
		{
			name: "missing required",
			body: canonicalTypedV2ReplaceBytes(canonicalStrictHumanV2, `"interaction_id":"interaction-7",`, ``),
			path: "response_v2.interaction_id", code: "invalid_request", class: "invalid_typed_submission", status: hertzconsts.StatusUnprocessableEntity,
		},
		{
			name: "null optional",
			body: canonicalTypedV2ReplaceBytes(canonicalStrictHumanV2, `"answer":"continue"`, `"answer":null`),
			path: "response_v2.answer", code: "invalid_request", class: "invalid_typed_submission", status: hertzconsts.StatusUnprocessableEntity,
		},
		{
			name: "invalid utf8",
			body: canonicalTypedV2InvalidUTF8(canonicalStrictHumanV2, "continue"),
			path: "response_v2.answer", code: "invalid_request", class: "invalid_typed_submission", status: hertzconsts.StatusUnprocessableEntity,
		},
		{
			name: "trailing object",
			body: []byte(canonicalStrictHumanV2 + `{}`),
			code: "invalid_json", class: "trailing_json", status: hertzconsts.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, public := decodeCanonicalTypedHumanResponseV2(tt.body)
			requireCanonicalTypedV2Error(t, public, tt.status, tt.code, tt.class, tt.path)
		})
	}
}

func TestDecodeCanonicalTypedVersionMixingV2(t *testing.T) {
	runV2 := `{"submission_v2":` + canonicalStrictRunV2 + `}`
	for _, legacy := range []string{"input", "command", "metadata", "config", "context", "coze"} {
		legacy := legacy
		t.Run("run rejects present legacy "+legacy, func(t *testing.T) {
			body := canonicalTypedV2Replace(runV2, `{"submission_v2":`, `{"`+legacy+`":null,"submission_v2":`)
			public := validateCanonicalTypedRunVersionMixing([]byte(body))
			requireCanonicalTypedV2Error(
				t, public, hertzconsts.StatusUnprocessableEntity,
				"invalid_request", "mixed_submission_versions", "submission_v2",
			)
		})
	}

	threadCases := []struct {
		name, body, path string
	}{
		{
			name: "atomic and deferred v2",
			body: `{"initial_submission_v2":` + canonicalStrictInitialV2 + `,"deferred_initial_submission_v2":` + canonicalStrictInitialV2 + `}`,
			path: "initial_submission_v2",
		},
		{
			name: "atomic v2 and atomic v1",
			body: `{"initial_submission_v2":` + canonicalStrictInitialV2 + `,"coze":{"initial_run":{}}}`,
			path: "initial_submission_v2",
		},
		{
			name: "atomic v2 and deferred v1",
			body: `{"initial_submission_v2":` + canonicalStrictInitialV2 + `,"coze":{"deferred_initial_run":{}}}`,
			path: "initial_submission_v2",
		},
		{
			name: "deferred v2 and deferred v1",
			body: `{"deferred_initial_submission_v2":` + canonicalStrictInitialV2 + `,"coze":{"deferred_initial_run":{}}}`,
			path: "deferred_initial_submission_v2",
		},
		{
			name: "deferred v2 and atomic v1",
			body: `{"deferred_initial_submission_v2":` + canonicalStrictInitialV2 + `,"coze":{"initial_run":{}}}`,
			path: "deferred_initial_submission_v2",
		},
	}
	for _, tt := range threadCases {
		t.Run("thread rejects "+tt.name, func(t *testing.T) {
			public := validateCanonicalTypedThreadVersionMixing([]byte(tt.body))
			requireCanonicalTypedV2Error(
				t, public, hertzconsts.StatusUnprocessableEntity,
				"invalid_request", "mixed_submission_versions", tt.path,
			)
		})
	}
}

func TestDecodeCanonicalTypedV2EnforcesParseBudget(t *testing.T) {
	t.Run("depth greater than 128", func(t *testing.T) {
		body := strings.Repeat("[", 129) + "0" + strings.Repeat("]", 129)
		_, public := decodeCanonicalV2Node([]byte(body), "submission_v2")
		requireCanonicalTypedV2Error(
			t, public, hertzconsts.StatusUnprocessableEntity,
			"invalid_request", "invalid_typed_submission", "submission_v2",
		)
	})

	t.Run("more than 65536 value units", func(t *testing.T) {
		body := "[" + strings.Repeat("0,", 65536) + "0]"
		_, public := decodeCanonicalV2Node([]byte(body), "submission_v2")
		requireCanonicalTypedV2Error(
			t, public, hertzconsts.StatusUnprocessableEntity,
			"invalid_request", "invalid_typed_submission", "submission_v2",
		)
	})
}

func TestDecodeCanonicalTypedV2SanitizesHostileKeysAndRejectsInvalidUTF8Keys(t *testing.T) {
	const hostileKey = `secret_hostile_key\u000a_with_control_and_a_name_that_is_far_too_long_to_echo`

	t.Run("unknown hostile key", func(t *testing.T) {
		body := canonicalTypedV2Replace(
			canonicalStrictRunV2,
			`"schema_version":`,
			`"`+hostileKey+`":true,"schema_version":`,
		)
		_, public := decodeCanonicalTypedRunSubmissionV2([]byte(body))
		requireCanonicalTypedV2Error(
			t, public, hertzconsts.StatusUnprocessableEntity,
			"unsupported_sdk_field", "unsupported_field", "submission_v2.<unsupported>",
		)
		require.NotContains(t, public.Detail, "secret_hostile_key")
		require.NotContains(t, public.Detail, "\n")
	})

	t.Run("duplicate hostile key", func(t *testing.T) {
		body := []byte(`{"` + hostileKey + `":1,"` + hostileKey + `":2}`)
		_, public := decodeCanonicalTypedRunSubmissionV2(body)
		requireCanonicalTypedV2Error(
			t, public, hertzconsts.StatusUnprocessableEntity,
			"invalid_request", "invalid_typed_submission", "submission_v2.<unsupported>",
		)
		require.NotContains(t, public.Detail, "secret_hostile_key")
		require.NotContains(t, public.Detail, "\n")
	})

	t.Run("invalid utf8 object key", func(t *testing.T) {
		body := []byte(canonicalStrictRunV2)
		index := bytes.Index(body, []byte("schema_version"))
		require.GreaterOrEqual(t, index, 0)
		body[index] = 0xff
		_, public := decodeCanonicalTypedRunSubmissionV2(body)
		requireCanonicalTypedV2Error(
			t, public, hertzconsts.StatusUnprocessableEntity,
			"invalid_request", "invalid_typed_submission", "submission_v2",
		)
		require.NotContains(t, public.Detail, "schema_version")
	})
}

func TestDecodeCanonicalTypedVersionMixingV2PrioritizesRootPresence(t *testing.T) {
	duplicateRun := canonicalTypedV2Replace(
		canonicalStrictRunV2,
		`"kind":"turn",`,
		`"kind":"turn","kind":"turn",`,
	)

	t.Run("run mix wins over duplicate inside v2", func(t *testing.T) {
		body := `{"input":null,"submission_v2":` + duplicateRun + `}`
		public := validateCanonicalTypedRunVersionMixing([]byte(body))
		requireCanonicalTypedV2Error(
			t, public, hertzconsts.StatusUnprocessableEntity,
			"invalid_request", "mixed_submission_versions", "submission_v2",
		)
	})

	t.Run("run non-mix leaves nested validation to field decoder", func(t *testing.T) {
		body := `{"submission_v2":` + duplicateRun + `}`
		require.Nil(t, validateCanonicalTypedRunVersionMixing([]byte(body)))
		_, public := decodeCanonicalTypedRunSubmissionV2([]byte(duplicateRun))
		requireCanonicalTypedV2Error(
			t, public, hertzconsts.StatusUnprocessableEntity,
			"invalid_request", "invalid_typed_submission", "submission_v2.kind",
		)
		require.NotContains(t, public.Detail, "request.")
	})

	t.Run("thread mix wins over duplicate inside v2", func(t *testing.T) {
		duplicateInitial := canonicalTypedV2Replace(
			canonicalStrictInitialV2,
			`"schema_version":"coze.workbench.initial_run_submission.v2",`,
			`"schema_version":"coze.workbench.initial_run_submission.v2","schema_version":"coze.workbench.initial_run_submission.v2",`,
		)
		body := `{"initial_submission_v2":` + duplicateInitial + `,"coze":{"initial_run":{}}}`
		public := validateCanonicalTypedThreadVersionMixing([]byte(body))
		requireCanonicalTypedV2Error(
			t, public, hertzconsts.StatusUnprocessableEntity,
			"invalid_request", "mixed_submission_versions", "initial_submission_v2",
		)
	})
}

func TestDecodeCanonicalTypedRunSubmissionV2ExtractsAndBoundsStringIDs(t *testing.T) {
	body := canonicalStrictRunV2WithOptionals()
	got, public := decodeCanonicalTypedRunSubmissionV2([]byte(body))
	require.Nil(t, public)
	require.NotNil(t, got)
	require.NotNil(t, got.Value.Config.ModelFailover)
	require.Equal(t, []int64{17}, got.Value.Config.ModelFailover.CandidateModelIds)
	require.NotNil(t, got.Value.Lineage)
	require.EqualValues(t, 9, got.Value.Lineage.SourceRunID)

	tests := []struct {
		name, old, replacement, path string
	}{
		{
			name: "candidate model id overflow",
			old:  `"candidate_model_ids":["17"]`, replacement: `"candidate_model_ids":["9223372036854775808"]`,
			path: "submission_v2.config.model_failover.candidate_model_ids[0]",
		},
		{
			name: "source run id overflow",
			old:  `"source_run_id":"9"`, replacement: `"source_run_id":"9223372036854775808"`,
			path: "submission_v2.lineage.source_run_id",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			overflow := canonicalTypedV2Replace(body, tt.old, tt.replacement)
			_, public := decodeCanonicalTypedRunSubmissionV2([]byte(overflow))
			requireCanonicalTypedV2Error(
				t, public, hertzconsts.StatusUnprocessableEntity,
				"invalid_request", "invalid_typed_submission", tt.path,
			)
			require.NotContains(t, public.Detail, "9223372036854775808")
		})
	}
}

func canonicalStrictRunV2WithOptionals() string {
	body := canonicalTypedV2Replace(
		canonicalStrictRunV2,
		`"kind":"turn",`,
		`"kind":"retry",`,
	)
	body = canonicalTypedV2Replace(
		body,
		`"token_usage":{"enabled":true}`,
		`"model_retry":{"max_retries":1,"backoff_ms":0,"retry_empty_output":false,"retry_finish_reasons":[]},`+
			`"model_failover":{"candidate_model_ids":["17"],"max_retries":1,"failover_empty_output":false,"failover_finish_reasons":[]},`+
			`"token_usage":{"enabled":true}`,
	)
	return canonicalTypedV2Replace(
		body,
		"\n}",
		`,"lineage":{"source_run_id":"9"},"metadata":{"source":"task_retry"}
}`,
	)
}

func canonicalTypedV2ReplaceBytes(body, old, replacement string) []byte {
	return []byte(canonicalTypedV2Replace(body, old, replacement))
}

func canonicalTypedV2Replace(body, old, replacement string) string {
	if strings.Count(body, old) != 1 {
		panic("typed V2 test fixture replacement is not unique: " + old)
	}
	return strings.Replace(body, old, replacement, 1)
}

func canonicalTypedV2InvalidUTF8(body, value string) []byte {
	raw := []byte(body)
	index := bytes.Index(raw, []byte(value))
	if index < 0 {
		panic("typed V2 test fixture value not found: " + value)
	}
	raw[index] = 0xff
	return raw
}

func requireCanonicalTypedV2Presence(
	t *testing.T,
	presence canonicalTypedV2Presence,
	path string,
	want bool,
) {
	t.Helper()
	_, got := presence[path]
	require.Equal(t, want, got, path)
}

func requireCanonicalTypedV2Error(
	t *testing.T,
	public *canonicalError,
	status int,
	code string,
	class string,
	path string,
) {
	t.Helper()
	require.NotNil(t, public)
	require.Equal(t, status, public.status)
	require.Equal(t, code, public.Code)
	require.Equal(t, class, public.errorClass)
	require.False(t, public.Retryable)
	if path != "" {
		require.Contains(t, public.Detail, path)
	}
	for _, secret := range []string{
		"secret-root-value",
		"secret-input-value",
		"secret-file-value",
		"secret-composer-value",
		"secret-config-value",
		"secret-memory-value",
		"secret-skills-value",
		"secret-mcp-value",
		"secret-web-value",
		"secret-http-value",
		"secret-search-value",
		"secret-retry-value",
		"secret-failover-value",
		"secret-token-value",
		"secret-lineage-value",
		"secret-metadata-value",
		"secret-initial-value",
		"secret-human-value",
	} {
		require.NotContains(t, public.Detail, secret)
	}
}
