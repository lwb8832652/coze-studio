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
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	hertzconsts "github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/stretchr/testify/require"
)

func TestCanonicalTypedV2ConfigParity(t *testing.T) {
	body := canonicalSemanticRunV2(canonicalStrictRunV2)
	body = canonicalTypedV2Replace(body, `"uploaded_files":[]`, `"uploaded_files":[{"file_id":"8"},{"file_id":"7"}]`)
	body = canonicalTypedV2Replace(body, `"composer":{"explicit_enable_skills":[],"allowed_skills":[]`, `"composer":{"model_type":"17","model_name":"primary","explicit_enable_skills":[],"allowed_skills":["skill.a"]`)
	typed, public := decodeCanonicalTypedRunSubmissionV2([]byte(body))
	require.Nil(t, public)
	mapped, public := mapCanonicalTypedRunV2(typed)
	require.Nil(t, public)
	require.JSONEq(t, `{"model_type":17,"model_name":"primary","enable_skills":[],"enable_mcp":[],"enable_kbs":[],"enable_databases":[],"runtime":"eino_adk","memory_retrieval":{"limit":1,"candidate_limit":1,"scopes":["thread"],"min_confidence":0.5},"skills":{"enabled":false,"visibility":"deferred","allowed_skills":["skill.a"]},"mcp_tools":{"enabled":false,"visibility":"deferred","allowed_tools":[]},"web_tools":{"enabled":false,"visibility":"deferred","http":{"enabled":false,"allowed_hosts":[],"timeout_ms":1000,"max_response_bytes":1024},"search":{"enabled":false,"max_results":1}},"token_usage":{"enabled":true}}`, mapped.Config)
	require.Equal(t, mapped.Config, mapped.MessageMetadata)
	require.Equal(t, []int64{8, 7}, mapped.UploadedFileIDs)
	require.Equal(t, "{}", mapped.Context)
	require.NotEmpty(t, mapped.IdempotencyFingerprint)
}

func TestCanonicalTypedV2SemanticAndCollectionBudgets(t *testing.T) {
	valid := canonicalSemanticRunV2(canonicalStrictRunV2)
	tests := []struct{ name, body, path string }{
		{"empty message", canonicalTypedV2Replace(valid, `"message":"hello"`, `"message":"  "`), "submission_v2.input.message"},
		{"duplicate attachments", canonicalTypedV2Replace(valid, `"uploaded_files":[]`, `"uploaded_files":[{"file_id":"7"},{"file_id":"7"}]`), "submission_v2.input.uploaded_files[1].file_id"},
		{"turn lineage", canonicalTypedV2Replace(valid, `"kind":"turn",`, `"kind":"turn","lineage":{"source_run_id":"9"},`), "submission_v2.lineage"},
		{"invalid runtime", canonicalTypedV2Replace(valid, `"runtime":"eino_adk"`, `"runtime":"legacy"`), "submission_v2.config.runtime"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			typed, public := decodeCanonicalTypedRunSubmissionV2([]byte(tt.body))
			require.Nil(t, public)
			public = validateCanonicalRunSubmissionV2(typed)
			requireCanonicalTypedV2Error(t, public, hertzconsts.StatusUnprocessableEntity, "invalid_request", "invalid_typed_submission", tt.path)
		})
	}
}

func TestCanonicalTypedV2RetryMapping(t *testing.T) {
	typed, public := decodeCanonicalTypedRunSubmissionV2([]byte(canonicalSemanticRunV2(canonicalStrictRunV2WithOptionals())))
	require.Nil(t, public)
	mapped, public := mapCanonicalTypedRunV2(typed)
	require.Nil(t, public)
	require.Equal(t, "", mapped.MessageContent)
	require.Equal(t, "", mapped.MessageMetadata)
	require.Empty(t, mapped.UploadedFileIDs)
	require.JSONEq(t, `{"messages":[{"role":"user","content":"hello"}],"uploaded_files":[]}`, mapped.Input)
	require.NotNil(t, mapped.TopLevelRetry)
	require.EqualValues(t, 9, mapped.TopLevelRetry.SourceRunID)
	require.JSONEq(t, `{"source":"task_retry"}`, mapped.Metadata)
	require.Equal(t, "{}", mapped.Context)
	require.NotEmpty(t, mapped.IdempotencyFingerprint)
}

func canonicalSemanticRunV2(body string) string {
	return canonicalTypedV2Replace(body, `"composer":{"allowed_skills":`, `"composer":{"explicit_enable_skills":[],"allowed_skills":`)
}

func TestCanonicalTypedV2HumanUnion(t *testing.T) {
	valid, public := decodeCanonicalTypedHumanResponseV2([]byte(canonicalStrictHumanV2))
	require.Nil(t, public)
	normalized, public := validateCanonicalHumanResponseV2(valid)
	require.Nil(t, public)
	require.Equal(t, "continue", normalized.Answer)

	invalidBody := canonicalTypedV2Replace(canonicalStrictHumanV2, `"answer":"continue"`, `"comment":"not allowed"`)
	invalid, public := decodeCanonicalTypedHumanResponseV2([]byte(invalidBody))
	require.Nil(t, public)
	_, public = validateCanonicalHumanResponseV2(invalid)
	requireCanonicalTypedV2Error(t, public, hertzconsts.StatusUnprocessableEntity, "invalid_request", "invalid_typed_submission", "response_v2.comment")
}

func TestCanonicalTypedV2NumericBudgets(t *testing.T) {
	base := canonicalSemanticRunV2(canonicalStrictRunV2)
	tests := []struct{ name, old, replacement, path string }{
		{"model above safe integer", `"allowed_skills":[]`, `"model_type":"9007199254740992","allowed_skills":[]`, "submission_v2.composer.model_type"},
		{"memory limit zero", `"limit":1`, `"limit":0`, "submission_v2.config.memory_retrieval.limit"},
		{"candidate below limit", `"limit":1,"candidate_limit":1`, `"limit":2,"candidate_limit":1`, "submission_v2.config.memory_retrieval.candidate_limit"},
		{"confidence above one", `"min_confidence":0.5`, `"min_confidence":1.1`, "submission_v2.config.memory_retrieval.min_confidence"},
		{"http timeout low", `"timeout_ms":1000`, `"timeout_ms":999`, "submission_v2.config.web_tools.http.timeout_ms"},
		{"response bytes high", `"max_response_bytes":1024`, `"max_response_bytes":1048577`, "submission_v2.config.web_tools.http.max_response_bytes"},
		{"search results high", `"max_results":1`, `"max_results":11`, "submission_v2.config.web_tools.search.max_results"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			typed, public := decodeCanonicalTypedRunSubmissionV2([]byte(canonicalTypedV2Replace(base, tt.old, tt.replacement)))
			if public == nil {
				public = validateCanonicalRunSubmissionV2(typed)
			}
			requireCanonicalTypedV2Error(t, public, hertzconsts.StatusUnprocessableEntity, "invalid_request", "invalid_typed_submission", tt.path)
		})
	}
}

func TestCanonicalTypedV2CollectionAndPresenceRules(t *testing.T) {
	base := canonicalSemanticRunV2(canonicalStrictRunV2)
	tests := []struct{ name, body, path string }{
		{"duplicate skill", canonicalTypedV2Replace(base, `"allowed_skills":[]`, `"allowed_skills":["skill.a","skill.a"]`), "submission_v2.composer.allowed_skills[1]"},
		{"sensitive resource", canonicalTypedV2Replace(base, `"enable_kbs":[]`, `"enable_kbs":["api_key=secret-value"]`), "submission_v2.composer.enable_kbs[0]"},
		{"enabled skill present empty", canonicalTypedV2Replace(base, `"skills":{"enabled":false`, `"skills":{"enabled":true`), "submission_v2.composer.explicit_enable_skills"},
		{"disabled mcp active list", canonicalTypedV2Replace(base, `"enable_mcp":[]`, `"enable_mcp":["server.tool"]`), "submission_v2.composer.enable_mcp"},
		{"uppercase host not normalized", canonicalTypedV2Replace(base, `"allowed_hosts":[]`, `"allowed_hosts":["Example.COM"]`), "submission_v2.config.web_tools.http.allowed_hosts[0]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			typed, public := decodeCanonicalTypedRunSubmissionV2([]byte(tt.body))
			require.Nil(t, public)
			public = validateCanonicalRunSubmissionV2(typed)
			requireCanonicalTypedV2Error(t, public, hertzconsts.StatusUnprocessableEntity, "invalid_request", "invalid_typed_submission", tt.path)
		})
	}
}

func TestCanonicalTypedV2InitialAndMCPParity(t *testing.T) {
	initialBody := canonicalSemanticRunV2(canonicalStrictInitialV2)
	typedInitial, public := decodeCanonicalTypedInitialSubmissionV2([]byte(initialBody), "initial_submission_v2")
	require.Nil(t, public)
	mappedInitial, public := mapCanonicalTypedInitialV2(typedInitial, false)
	require.Nil(t, public)
	require.Equal(t, canonicalPublicAssistantID, mappedInitial.AssistantID)
	require.Equal(t, "hello", mappedInitial.MessageContent)
	require.JSONEq(t, `{}`, mappedInitial.Metadata)
	require.JSONEq(t, `{}`, mappedInitial.Context)
	require.NotEmpty(t, mappedInitial.Config)

	mcpAuto := canonicalTypedV2Replace(canonicalSemanticRunV2(canonicalStrictRunV2), `"mcp_tools":{"enabled":false`, `"mcp_tools":{"enabled":true`)
	typedAuto, public := decodeCanonicalTypedRunSubmissionV2([]byte(mcpAuto))
	require.Nil(t, public)
	mappedAuto, public := mapCanonicalTypedRunV2(typedAuto)
	require.Nil(t, public)
	require.NotContains(t, mappedAuto.Config, `"allowed_tools"`)

	mcpExplicit := canonicalTypedV2Replace(mcpAuto, `"enable_mcp":[]`, `"enable_mcp":["server.tool"]`)
	mcpExplicit = canonicalTypedV2Replace(mcpExplicit, `"allowed_mcp_tools":[]`, `"allowed_mcp_tools":["server.tool"]`)
	typedExplicit, public := decodeCanonicalTypedRunSubmissionV2([]byte(mcpExplicit))
	require.Nil(t, public)
	mappedExplicit, public := mapCanonicalTypedRunV2(typedExplicit)
	require.Nil(t, public)
	require.Contains(t, mappedExplicit.Config, `"allowed_tools":["server.tool"]`)
}

func TestCanonicalTypedV2ModelRetryFailoverBudgets(t *testing.T) {
	base := canonicalSemanticRunV2(canonicalStrictRunV2WithOptionals())
	tests := []struct{ name, old, replacement, path string }{
		{"retry count high", `"model_retry":{"max_retries":1`, `"model_retry":{"max_retries":6`, "submission_v2.config.model_retry.max_retries"},
		{"retry backoff high", `"backoff_ms":0`, `"backoff_ms":60001`, "submission_v2.config.model_retry.backoff_ms"},
		{"duplicate finish reason", `"retry_finish_reasons":[]`, `"retry_finish_reasons":["length","length"]`, "submission_v2.config.model_retry.retry_finish_reasons[1]"},
		{"failover unsafe model", `"candidate_model_ids":["17"]`, `"candidate_model_ids":["9007199254740992"]`, "submission_v2.config.model_failover.candidate_model_ids[0]"},
		{"failover retry above candidates", `"model_failover":{"candidate_model_ids":["17"],"max_retries":1`, `"model_failover":{"candidate_model_ids":["17"],"max_retries":2`, "submission_v2.config.model_failover.max_retries"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			typed, public := decodeCanonicalTypedRunSubmissionV2([]byte(canonicalTypedV2Replace(base, tt.old, tt.replacement)))
			if public == nil {
				public = validateCanonicalRunSubmissionV2(typed)
			}
			requireCanonicalTypedV2Error(t, public, hertzconsts.StatusUnprocessableEntity, "invalid_request", "invalid_typed_submission", tt.path)
		})
	}
}

func TestCanonicalTypedV2DefaultsAndFingerprintParity(t *testing.T) {
	base := canonicalSemanticRunV2(canonicalStrictRunV2)
	typed, public := decodeCanonicalTypedRunSubmissionV2([]byte(base))
	require.Nil(t, public)
	mapped, public := mapCanonicalTypedRunV2(typed)
	require.Nil(t, public)
	require.Equal(t, canonicalRunDefaults, mapped.Options)
	want := *mapped
	want.IdempotencyFingerprint = ""
	require.Equal(t, canonicalRunTurnRequestFingerprint(&want), mapped.IdempotencyFingerprint)

	changed, public := decodeCanonicalTypedRunSubmissionV2([]byte(canonicalTypedV2Replace(base, `"message":"hello"`, `"message":"changed"`)))
	require.Nil(t, public)
	changedMapped, public := mapCanonicalTypedRunV2(changed)
	require.Nil(t, public)
	require.NotEqual(t, mapped.IdempotencyFingerprint, changedMapped.IdempotencyFingerprint)
}

func TestCanonicalTypedV2CanonicalDecimalIDs(t *testing.T) {
	base := canonicalSemanticRunV2(canonicalStrictRunV2)
	tests := []struct{ name, body, path string }{
		{"file plus", canonicalTypedV2Replace(base, `"uploaded_files":[]`, `"uploaded_files":[{"file_id":"+7"}]`), "submission_v2.input.uploaded_files[0].file_id"},
		{"file leading zero", canonicalTypedV2Replace(base, `"uploaded_files":[]`, `"uploaded_files":[{"file_id":"07"}]`), "submission_v2.input.uploaded_files[0].file_id"},
		{"model leading zero", canonicalTypedV2Replace(base, `"allowed_skills":[]`, `"model_type":"017","allowed_skills":[]`), "submission_v2.composer.model_type"},
		{"source leading zero", canonicalTypedV2Replace(canonicalSemanticRunV2(canonicalStrictRunV2WithOptionals()), `"source_run_id":"9"`, `"source_run_id":"09"`), "submission_v2.lineage.source_run_id"},
		{"candidate plus", canonicalTypedV2Replace(canonicalSemanticRunV2(canonicalStrictRunV2WithOptionals()), `"candidate_model_ids":["17"]`, `"candidate_model_ids":["+17"]`), "submission_v2.config.model_failover.candidate_model_ids[0]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, public := decodeCanonicalTypedRunSubmissionV2([]byte(tt.body))
			requireCanonicalTypedV2Error(t, public, hertzconsts.StatusUnprocessableEntity, "invalid_request", "invalid_typed_submission", tt.path)
		})
	}
}

func TestCanonicalTypedV2RawTextBudgetsAndModelName(t *testing.T) {
	base := canonicalSemanticRunV2(canonicalStrictRunV2)
	overMessage := strings.Repeat(" ", 256*1024) + "x"
	typed, public := decodeCanonicalTypedRunSubmissionV2([]byte(canonicalTypedV2Replace(base, `"message":"hello"`, `"message":"`+overMessage+`"`)))
	require.Nil(t, public)
	public = validateCanonicalRunSubmissionV2(typed)
	requireCanonicalTypedV2Error(t, public, hertzconsts.StatusUnprocessableEntity, "invalid_request", "invalid_typed_submission", "submission_v2.input.message")

	validName := canonicalTypedV2Replace(base, `"allowed_skills":[]`, `"model_name":"GPT 4o mini","allowed_skills":[]`)
	typed, public = decodeCanonicalTypedRunSubmissionV2([]byte(validName))
	require.Nil(t, public)
	require.Nil(t, validateCanonicalRunSubmissionV2(typed))

	badName := canonicalTypedV2Replace(base, `"allowed_skills":[]`, `"model_name":" api_key=secret-value ","allowed_skills":[]`)
	typed, public = decodeCanonicalTypedRunSubmissionV2([]byte(badName))
	require.Nil(t, public)
	public = validateCanonicalRunSubmissionV2(typed)
	requireCanonicalTypedV2Error(t, public, hertzconsts.StatusUnprocessableEntity, "invalid_request", "invalid_typed_submission", "submission_v2.composer.model_name")
}

func TestCanonicalTypedV2HumanRawTextBudgetBeforeTrim(t *testing.T) {
	body := canonicalTypedV2Replace(
		canonicalStrictHumanV2,
		`"answer":"continue"`,
		`"answer":"`+strings.Repeat(" ", 8192)+`x"`,
	)
	typed, public := decodeCanonicalTypedHumanResponseV2([]byte(body))
	require.Nil(t, public)
	_, public = validateCanonicalHumanResponseV2(typed)
	requireCanonicalTypedV2Error(
		t, public, hertzconsts.StatusUnprocessableEntity,
		"invalid_request", "invalid_typed_submission", "response_v2.answer",
	)
}

func TestCanonicalTypedV2GeneratedNumericLeafPath(t *testing.T) {
	base := canonicalSemanticRunV2(canonicalStrictRunV2)
	_, public := decodeCanonicalTypedRunSubmissionV2([]byte(canonicalTypedV2Replace(base, `"limit":1`, `"limit":2147483648`)))
	requireCanonicalTypedV2Error(t, public, hertzconsts.StatusUnprocessableEntity, "invalid_request", "invalid_typed_submission", "submission_v2.config.memory_retrieval.limit")
}

func TestCanonicalTypedV2StrictHosts(t *testing.T) {
	base := canonicalSemanticRunV2(canonicalStrictRunV2)
	tests := []struct {
		name         string
		hosts        []string
		invalidIndex int
	}{
		{"dns empty label", []string{"api..example.com"}, 0},
		{"dns leading hyphen", []string{"-api.example.com"}, 0},
		{"dns trailing hyphen", []string{"api-.example.com"}, 0},
		{"dns underscore", []string{"api_service.example.com"}, 0},
		{"ipv4 leading zero", []string{"127.0.0.01"}, 0},
		{"ipv6 noncanonical", []string{"2001:0db8::1"}, 0},
		{"normalized duplicate", []string{"127.0.0.1", "127.0.0.1"}, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := json.Marshal(tt.hosts)
			require.NoError(t, err)
			body := canonicalTypedV2Replace(base, `"allowed_hosts":[]`, `"allowed_hosts":`+string(raw))
			typed, public := decodeCanonicalTypedRunSubmissionV2([]byte(body))
			require.Nil(t, public)
			public = validateCanonicalRunSubmissionV2(typed)
			path := canonicalV2IndexPath("submission_v2.config.web_tools.http.allowed_hosts", tt.invalidIndex)
			requireCanonicalTypedV2Error(t, public, hertzconsts.StatusUnprocessableEntity, "invalid_request", "invalid_typed_submission", path)
		})
	}
}

func TestCanonicalTypedV2AbsenceAndDeepCopyParity(t *testing.T) {
	base := canonicalSemanticRunV2(canonicalStrictRunV2)
	typed, public := decodeCanonicalTypedRunSubmissionV2([]byte(base))
	require.Nil(t, public)
	mapped, public := mapCanonicalTypedRunV2(typed)
	require.Nil(t, public)
	require.NotContains(t, mapped.Config, `"model_retry"`)
	require.NotContains(t, mapped.Config, `"model_failover"`)
	require.JSONEq(t, `{}`, mapped.Metadata)
	require.Equal(t, mapped.Config, mapped.MessageMetadata)

	before := mapped.IdempotencyFingerprint
	mapped.Options.StreamModes[0] = "mutated"
	require.Equal(t, []string{"values"}, canonicalRunDefaults.StreamModes)
	require.NotEqual(t, before, canonicalRunTurnRequestFingerprint(mapped))
}

func TestCanonicalTypedV2SkillAndMCPStateParity(t *testing.T) {
	base := canonicalStrictRunV2
	tests := []struct{ name, body, wantFragment string }{
		{"skill enabled absent explicit", canonicalTypedV2Replace(base, `"skills":{"enabled":false`, `"skills":{"enabled":true`), `"skills":{"enabled":true,"visibility":"deferred","allowed_skills":[]}`},
		{"skill disabled present empty", canonicalSemanticRunV2(base), `"enable_skills":[]`},
		{"skill explicit ordered", canonicalTypedV2Replace(canonicalTypedV2Replace(canonicalTypedV2Replace(canonicalSemanticRunV2(base), `"explicit_enable_skills":[]`, `"explicit_enable_skills":["skill.b","skill.a"]`), `"allowed_skills":[]`, `"allowed_skills":["skill.b","skill.a"]`), `"skills":{"enabled":false`, `"skills":{"enabled":true`), `"enable_skills":["skill.b","skill.a"]`},
		{"mcp disabled preserves empty", canonicalSemanticRunV2(base), `"allowed_tools":[]`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			typed, public := decodeCanonicalTypedRunSubmissionV2([]byte(tt.body))
			require.Nil(t, public)
			mapped, public := mapCanonicalTypedRunV2(typed)
			require.Nil(t, public)
			require.Contains(t, mapped.Config, tt.wantFragment)
		})
	}
}

func TestCanonicalTypedV2HumanClosedUnionAndBudgets(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		wantAnswer  string
		wantChoice  string
		wantComment string
	}{
		{
			name: "clarification answer and choice are independently allowed",
			body: canonicalTypedV2Replace(
				canonicalStrictHumanV2,
				`"answer":"continue"`,
				`"answer":"continue","choice_id":"choice-7"`,
			),
			wantAnswer: "continue", wantChoice: "choice-7",
		},
		{
			name: "clarification choice only",
			body: canonicalTypedV2Replace(
				canonicalStrictHumanV2,
				`"answer":"continue"`,
				`"choice_id":"choice-7"`,
			),
			wantChoice: "choice-7",
		},
		{
			name:        "confirmation approved with comment",
			body:        `{"schema":"coze.human_interaction_response.v1","interaction_id":"interaction-7","kind":"confirmation","decision":"approved","comment":"looks good"}`,
			wantComment: "looks good",
		},
		{
			name: "confirmation rejected without optional fields",
			body: `{"schema":"coze.human_interaction_response.v1","interaction_id":"interaction-7","kind":"confirmation","decision":"rejected"}`,
		},
		{
			name: "answer exactly eight kibibytes",
			body: canonicalTypedV2Replace(
				canonicalStrictHumanV2,
				`"answer":"continue"`,
				`"answer":"`+strings.Repeat("a", 8192)+`"`,
			),
			wantAnswer: strings.Repeat("a", 8192),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			typed, public := decodeCanonicalTypedHumanResponseV2([]byte(tt.body))
			require.Nil(t, public)
			got, public := validateCanonicalHumanResponseV2(typed)
			require.Nil(t, public)
			require.Equal(t, tt.wantAnswer, got.Answer)
			require.Equal(t, tt.wantChoice, got.ChoiceID)
			require.Equal(t, tt.wantComment, got.Comment)
		})
	}

	invalid := []struct{ name, body, path string }{
		{"answer above eight kibibytes", canonicalTypedV2Replace(canonicalStrictHumanV2, `"answer":"continue"`, `"answer":"`+strings.Repeat("a", 8193)+`"`), "response_v2.answer"},
		{"confirmation answer prohibited", `{"schema":"coze.human_interaction_response.v1","interaction_id":"interaction-7","kind":"confirmation","decision":"approved","answer":"yes"}`, "response_v2.answer"},
		{"clarification requires answer or choice", `{"schema":"coze.human_interaction_response.v1","interaction_id":"interaction-7","kind":"clarification","decision":"answered"}`, "response_v2.answer"},
	}
	for _, tt := range invalid {
		t.Run(tt.name, func(t *testing.T) {
			typed, public := decodeCanonicalTypedHumanResponseV2([]byte(tt.body))
			require.Nil(t, public)
			_, public = validateCanonicalHumanResponseV2(typed)
			requireCanonicalTypedV2Error(t, public, hertzconsts.StatusUnprocessableEntity, "invalid_request", "invalid_typed_submission", tt.path)
		})
	}
}

func TestCanonicalTypedV2FlowAndCollectionMatrix(t *testing.T) {
	base := canonicalSemanticRunV2(canonicalStrictRunV2)
	validMessage := canonicalTypedV2Replace(base, `"message":"hello"`, `"message":"`+strings.Repeat("m", 256*1024)+`"`)
	typed, public := decodeCanonicalTypedRunSubmissionV2([]byte(validMessage))
	require.Nil(t, public)
	require.Nil(t, validateCanonicalRunSubmissionV2(typed))

	files := make([]string, 10)
	for i := range files {
		files[i] = `{"file_id":"` + strconv.Itoa(i+1) + `"}`
	}
	tenFiles := canonicalTypedV2Replace(base, `"uploaded_files":[]`, `"uploaded_files":[`+strings.Join(files, ",")+`]`)
	typed, public = decodeCanonicalTypedRunSubmissionV2([]byte(tenFiles))
	require.Nil(t, public)
	require.Nil(t, validateCanonicalRunSubmissionV2(typed))

	uniqueIDs := func(prefix string, count int) string {
		values := make([]string, count)
		for i := range values {
			values[i] = `"` + prefix + strconv.Itoa(i) + `"`
		}
		return strings.Join(values, ",")
	}
	resources256 := canonicalTypedV2Replace(base, `"enable_kbs":[]`, `"enable_kbs":[`+uniqueIDs("kb-", 256)+`]`)
	typed, public = decodeCanonicalTypedRunSubmissionV2([]byte(resources256))
	require.Nil(t, public)
	require.Nil(t, validateCanonicalRunSubmissionV2(typed))

	invalid := []struct{ name, body, path string }{
		{"wrong run schema", canonicalTypedV2Replace(base, `"schema_version":"coze.workbench.run_submission.v2"`, `"schema_version":"wrong"`), "submission_v2.schema_version"},
		{"unknown kind", canonicalTypedV2Replace(base, `"kind":"turn"`, `"kind":"unknown"`), "submission_v2.kind"},
		{"turn source mismatch", canonicalTypedV2Replace(base, `"kind":"turn",`, `"kind":"turn","metadata":{"source":"task_retry"},`), "submission_v2.metadata.source"},
		{"retry source mismatch", canonicalTypedV2Replace(canonicalSemanticRunV2(canonicalStrictRunV2WithOptionals()), `"source":"task_retry"`, `"source":"workbench_new_task"`), "submission_v2.metadata.source"},
		{"visibility mismatch", canonicalTypedV2Replace(base, `"skills":{"enabled":false,"visibility":"deferred"}`, `"skills":{"enabled":false,"visibility":"immediate"}`), "submission_v2.config.skills.visibility"},
		{"eleven files", canonicalTypedV2Replace(tenFiles, `{"file_id":"10"}]`, `{"file_id":"10"},{"file_id":"11"}]`), "submission_v2.input.uploaded_files"},
		{"zero file id", canonicalTypedV2Replace(base, `"uploaded_files":[]`, `"uploaded_files":[{"file_id":"0"}]`), "submission_v2.input.uploaded_files[0].file_id"},
		{"resource list above cap", canonicalTypedV2Replace(base, `"enable_databases":[]`, `"enable_databases":[`+uniqueIDs("db-", 257)+`]`), "submission_v2.composer.enable_databases"},
		{"duplicate scope", canonicalTypedV2Replace(base, `"scopes":["thread"]`, `"scopes":["thread","thread"]`), "submission_v2.config.memory_retrieval.scopes[1]"},
		{"unknown scope", canonicalTypedV2Replace(base, `"scopes":["thread"]`, `"scopes":["workspace"]`), "submission_v2.config.memory_retrieval.scopes[0]"},
		{"four scopes", canonicalTypedV2Replace(base, `"scopes":["thread"]`, `"scopes":["thread","run","long_term","thread"]`), "submission_v2.config.memory_retrieval.scopes"},
	}
	for _, tt := range invalid {
		t.Run(tt.name, func(t *testing.T) {
			typed, public := decodeCanonicalTypedRunSubmissionV2([]byte(tt.body))
			if public == nil {
				public = validateCanonicalRunSubmissionV2(typed)
			}
			requireCanonicalTypedV2Error(t, public, hertzconsts.StatusUnprocessableEntity, "invalid_request", "invalid_typed_submission", tt.path)
		})
	}
}

func TestCanonicalTypedV2HostAndNumericBoundaryMatrix(t *testing.T) {
	base := canonicalSemanticRunV2(canonicalStrictRunV2)
	maxHost := strings.Repeat("a", 63) + "." + strings.Repeat("b", 63) + "." + strings.Repeat("c", 63) + "." + strings.Repeat("d", 61)
	hosts64 := make([]string, 64)
	for i := range hosts64 {
		hosts64[i] = "h" + strconv.Itoa(i) + ".example.com"
	}
	for _, body := range []string{
		canonicalTypedV2Replace(base, `"allowed_hosts":[]`, `"allowed_hosts":["`+maxHost+`"]`),
		canonicalTypedV2Replace(base, `"allowed_hosts":[]`, `"allowed_hosts":["`+strings.Join(hosts64, `","`)+`"]`),
		canonicalTypedV2Replace(canonicalTypedV2Replace(canonicalTypedV2Replace(canonicalTypedV2Replace(canonicalTypedV2Replace(base, `"limit":1,"candidate_limit":1`, `"limit":100,"candidate_limit":100`), `"min_confidence":0.5`, `"min_confidence":1`), `"timeout_ms":1000`, `"timeout_ms":60000`), `"max_response_bytes":1024`, `"max_response_bytes":1048576`), `"max_results":1`, `"max_results":10`),
	} {
		typed, public := decodeCanonicalTypedRunSubmissionV2([]byte(body))
		require.Nil(t, public)
		require.Nil(t, validateCanonicalRunSubmissionV2(typed))
	}

	invalidHosts := []string{
		"https://example.com", "user@example.com", "example.com/path", "example.com?q=1",
		maxHost + "x",
	}
	for _, host := range invalidHosts {
		body := canonicalTypedV2Replace(base, `"allowed_hosts":[]`, `"allowed_hosts":["`+host+`"]`)
		typed, public := decodeCanonicalTypedRunSubmissionV2([]byte(body))
		require.Nil(t, public)
		public = validateCanonicalRunSubmissionV2(typed)
		requireCanonicalTypedV2Error(t, public, hertzconsts.StatusUnprocessableEntity, "invalid_request", "invalid_typed_submission", "submission_v2.config.web_tools.http.allowed_hosts[0]")
	}
	hosts65 := append(append([]string{}, hosts64...), "h64.example.com")
	rawHosts65, err := json.Marshal(hosts65)
	require.NoError(t, err)
	typed, public := decodeCanonicalTypedRunSubmissionV2([]byte(canonicalTypedV2Replace(base, `"allowed_hosts":[]`, `"allowed_hosts":`+string(rawHosts65))))
	require.Nil(t, public)
	public = validateCanonicalRunSubmissionV2(typed)
	requireCanonicalTypedV2Error(t, public, hertzconsts.StatusUnprocessableEntity, "invalid_request", "invalid_typed_submission", "submission_v2.config.web_tools.http.allowed_hosts")

	invalidNumbers := []struct{ old, replacement, path string }{
		{`"limit":1`, `"limit":101`, "submission_v2.config.memory_retrieval.limit"},
		{`"timeout_ms":1000`, `"timeout_ms":60001`, "submission_v2.config.web_tools.http.timeout_ms"},
		{`"max_response_bytes":1024`, `"max_response_bytes":1048577`, "submission_v2.config.web_tools.http.max_response_bytes"},
		{`"max_results":1`, `"max_results":11`, "submission_v2.config.web_tools.search.max_results"},
		{`"min_confidence":0.5`, `"min_confidence":1e400`, "submission_v2.config.memory_retrieval.min_confidence"},
	}
	for _, tt := range invalidNumbers {
		typed, public := decodeCanonicalTypedRunSubmissionV2([]byte(canonicalTypedV2Replace(base, tt.old, tt.replacement)))
		if public == nil {
			public = validateCanonicalRunSubmissionV2(typed)
		}
		requireCanonicalTypedV2Error(t, public, hertzconsts.StatusUnprocessableEntity, "invalid_request", "invalid_typed_submission", tt.path)
	}

	httpEnabledWithoutHosts := canonicalTypedV2Replace(canonicalTypedV2Replace(base, `"web_tools":{
      "enabled":false`, `"web_tools":{
      "enabled":true`), `"http":{"enabled":false`, `"http":{"enabled":true`)
	typed, public = decodeCanonicalTypedRunSubmissionV2([]byte(httpEnabledWithoutHosts))
	require.Nil(t, public)
	public = validateCanonicalRunSubmissionV2(typed)
	requireCanonicalTypedV2Error(t, public, hertzconsts.StatusUnprocessableEntity, "invalid_request", "invalid_typed_submission", "submission_v2.config.web_tools.http.allowed_hosts")
}

func TestCanonicalTypedV2PositiveParityFingerprintAndDeepCopy(t *testing.T) {
	body := canonicalSemanticRunV2(canonicalStrictRunV2WithOptionals())
	body = canonicalTypedV2Replace(body, `"allowed_skills":[]`, `"model_type":"17","model_name":"Primary Model","allowed_skills":["skill.b","skill.a"]`)
	body = canonicalTypedV2Replace(body, `"explicit_enable_skills":[]`, `"explicit_enable_skills":["skill.b","skill.a"]`)
	body = canonicalTypedV2Replace(body, `"enable_mcp":[]`, `"enable_mcp":["mcp.b","mcp.a"]`)
	body = canonicalTypedV2Replace(body, `"enable_kbs":[]`, `"enable_kbs":["kb.b","kb.a"]`)
	body = canonicalTypedV2Replace(body, `"enable_databases":[]`, `"enable_databases":["db.b","db.a"]`)
	body = canonicalTypedV2Replace(body, `"allowed_mcp_tools":[]`, `"allowed_mcp_tools":["mcp.b","mcp.a"]`)
	body = canonicalTypedV2Replace(body, `"skills":{"enabled":false`, `"skills":{"enabled":true`)
	body = canonicalTypedV2Replace(body, `"mcp_tools":{"enabled":false`, `"mcp_tools":{"enabled":true`)
	body = canonicalTypedV2Replace(body, `"limit":1,"candidate_limit":1,"scopes":["thread"],"min_confidence":0.5`, `"limit":2,"candidate_limit":3,"scopes":["thread","run"],"min_confidence":0.75`)
	body = canonicalTypedV2Replace(body, `"web_tools":{
      "enabled":false`, `"web_tools":{
      "enabled":true`)
	body = canonicalTypedV2Replace(body, `"search":{"enabled":false,"max_results":1`, `"search":{"enabled":true,"max_results":5`)
	body = canonicalTypedV2Replace(body, `"retry_finish_reasons":[]`, `"retry_finish_reasons":["length","empty"]`)
	body = canonicalTypedV2Replace(body, `"candidate_model_ids":["17"]`, `"candidate_model_ids":["17","19"]`)
	body = canonicalTypedV2Replace(body, `"max_retries":1,"failover_empty_output"`, `"max_retries":2,"failover_empty_output"`)
	body = canonicalTypedV2Replace(body, `"failover_finish_reasons":[]`, `"failover_finish_reasons":["length","empty"]`)
	body = canonicalTypedV2Replace(body, `"token_usage":{"enabled":true}`, `"token_usage":{"enabled":false}`)
	typed, public := decodeCanonicalTypedRunSubmissionV2([]byte(body))
	require.Nil(t, public)
	mapped, public := mapCanonicalTypedRunV2(typed)
	require.Nil(t, public)
	require.Contains(t, mapped.Config, `"enable_kbs":["kb.b","kb.a"]`)
	require.Contains(t, mapped.Config, `"enable_databases":["db.b","db.a"]`)
	require.Contains(t, mapped.Config, `"candidate_model_ids":[17,19]`)
	require.Contains(t, mapped.Config, `"token_usage":{"enabled":false}`)
	require.Contains(t, mapped.Config, `"scopes":["thread","run"]`)
	require.Equal(t, canonicalRunDefaults, mapped.Options)

	want := *mapped
	want.MessageContent = strings.TrimSpace(typed.Value.Input.Message)
	want.IdempotencyFingerprint = ""
	require.Equal(t, canonicalRunRetryRequestFingerprint(&want), mapped.IdempotencyFingerprint)
	for _, mutate := range []func(*canonicalRunSubmission){
		func(x *canonicalRunSubmission) { x.TopLevelRetry = &canonicalTopLevelRetrySubmission{SourceRunID: 10} },
		func(x *canonicalRunSubmission) { x.Metadata = `{"source":"other"}` },
		func(x *canonicalRunSubmission) { x.Config = `{}` },
		func(x *canonicalRunSubmission) { x.Options.Durability = "sync" },
	} {
		changed := *mapped
		changed.Options.StreamModes = append([]string{}, mapped.Options.StreamModes...)
		changed.MessageContent = strings.TrimSpace(typed.Value.Input.Message)
		mutate(&changed)
		require.NotEqual(t, mapped.IdempotencyFingerprint, canonicalRunRetryRequestFingerprint(&changed))
	}

	configBefore, inputBefore, fingerprintBefore := mapped.Config, mapped.Input, mapped.IdempotencyFingerprint
	typed.Value.Composer.AllowedSkills[0] = "mutated"
	typed.Value.Composer.EnableKbs[0] = "mutated"
	typed.Value.Config.MemoryRetrieval.Scopes[0] = "mutated"
	typed.Value.Config.ModelFailover.CandidateModelIds[0] = 23
	typed.Value.Config.ModelRetry.RetryFinishReasons[0] = "mutated"
	require.Equal(t, configBefore, mapped.Config)
	require.Equal(t, inputBefore, mapped.Input)
	require.Equal(t, fingerprintBefore, mapped.IdempotencyFingerprint)

	initial, public := decodeCanonicalTypedInitialSubmissionV2([]byte(canonicalSemanticRunV2(canonicalStrictInitialV2)), "initial_submission_v2")
	require.Nil(t, public)
	mappedInitial, public := mapCanonicalTypedInitialV2(initial, false)
	require.Nil(t, public)
	initialFingerprint := canonicalInitialThreadRunRequestFingerprint(`{}`, "title", "api", mappedInitial)
	require.NotEmpty(t, initialFingerprint)
	changedInitial := *mappedInitial
	changedInitial.Config = `{}`
	require.NotEqual(t, initialFingerprint, canonicalInitialThreadRunRequestFingerprint(`{}`, "title", "api", &changedInitial))
}

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
