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
	"errors"
	"io"
	"math"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	hertzconsts "github.com/cloudwego/hertz/pkg/protocol/consts"

	threadcontract "github.com/coze-dev/coze-studio/backend/api/model/workbench/thread_contract"
)

const (
	canonicalV2ObjectKind byte = '{'
	canonicalV2ArrayKind  byte = '['
	canonicalV2StringKind byte = 's'
	canonicalV2NumberKind byte = 'n'
	canonicalV2BoolKind   byte = 'b'
	canonicalV2NullKind   byte = '0'

	canonicalV2MaxDepth     = 128
	canonicalV2MaxParseUnit = 65536
	canonicalV2MaxSafeKey   = 64
)

type canonicalV2Node struct {
	path string
	raw  json.RawMessage
	kind byte
	obj  map[string]*canonicalV2Node
	arr  []*canonicalV2Node

	order []string
	value any
	start int64
	end   int64
}

type canonicalV2FieldRule struct {
	required bool
	kind     byte
	object   map[string]canonicalV2FieldRule
	element  *canonicalV2FieldRule
}

type canonicalV2ParseBudget struct {
	root  string
	units int
}

// Presence is deliberately separate from the generated value. Generated Go
// values cannot distinguish an omitted optional list from every zero value.
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

const canonicalV2MaxSafeInteger int64 = 9007199254740991

type canonicalTypedConfigV2Output struct {
	ModelType       *int64                          `json:"model_type,omitempty"`
	ModelName       *string                         `json:"model_name,omitempty"`
	EnableSkills    *[]string                       `json:"enable_skills,omitempty"`
	EnableMCP       []string                        `json:"enable_mcp"`
	EnableKBs       []string                        `json:"enable_kbs"`
	EnableDatabases []string                        `json:"enable_databases"`
	Runtime         string                          `json:"runtime"`
	MemoryRetrieval canonicalTypedMemoryV2Output    `json:"memory_retrieval"`
	Skills          canonicalTypedSkillsV2Output    `json:"skills"`
	MCPTools        canonicalTypedMCPToolsV2Output  `json:"mcp_tools"`
	WebTools        canonicalTypedWebToolsV2Output  `json:"web_tools"`
	ModelRetry      *canonicalTypedRetryV2Output    `json:"model_retry,omitempty"`
	ModelFailover   *canonicalTypedFailoverV2Output `json:"model_failover,omitempty"`
	TokenUsage      canonicalTypedTokenV2Output     `json:"token_usage"`
}

type canonicalTypedMemoryV2Output struct {
	Limit          int32    `json:"limit"`
	CandidateLimit int32    `json:"candidate_limit"`
	Scopes         []string `json:"scopes"`
	MinConfidence  float64  `json:"min_confidence"`
}

type canonicalTypedSkillsV2Output struct {
	Enabled       bool     `json:"enabled"`
	Visibility    string   `json:"visibility"`
	AllowedSkills []string `json:"allowed_skills"`
}

type canonicalTypedMCPToolsV2Output struct {
	Enabled      bool      `json:"enabled"`
	Visibility   string    `json:"visibility"`
	AllowedTools *[]string `json:"allowed_tools,omitempty"`
}

type canonicalTypedWebToolsV2Output struct {
	Enabled    bool                            `json:"enabled"`
	Visibility string                          `json:"visibility"`
	HTTP       canonicalTypedWebHTTPV2Output   `json:"http"`
	Search     canonicalTypedWebSearchV2Output `json:"search"`
}

type canonicalTypedWebHTTPV2Output struct {
	Enabled          bool     `json:"enabled"`
	AllowedHosts     []string `json:"allowed_hosts"`
	TimeoutMS        int64    `json:"timeout_ms"`
	MaxResponseBytes int64    `json:"max_response_bytes"`
}

type canonicalTypedWebSearchV2Output struct {
	Enabled    bool  `json:"enabled"`
	MaxResults int32 `json:"max_results"`
}

type canonicalTypedRetryV2Output struct {
	MaxRetries         int32    `json:"max_retries"`
	BackoffMS          int64    `json:"backoff_ms"`
	RetryEmptyOutput   bool     `json:"retry_empty_output"`
	RetryFinishReasons []string `json:"retry_finish_reasons"`
}

type canonicalTypedFailoverV2Output struct {
	CandidateModelIDs     []int64  `json:"candidate_model_ids"`
	MaxRetries            int32    `json:"max_retries"`
	FailoverEmptyOutput   bool     `json:"failover_empty_output"`
	FailoverFinishReasons []string `json:"failover_finish_reasons"`
}

type canonicalTypedTokenV2Output struct {
	Enabled bool `json:"enabled"`
}

func validateCanonicalRunSubmissionV2(v *canonicalTypedRunV2) *canonicalError {
	if v == nil {
		return canonicalTypedV2Invalid("submission_v2")
	}
	value := &v.Value
	if value.SchemaVersion != "coze.workbench.run_submission.v2" {
		return canonicalTypedV2Invalid("submission_v2.schema_version")
	}
	if value.Kind != "turn" && value.Kind != "retry" {
		return canonicalTypedV2Invalid("submission_v2.kind")
	}
	if public := validateCanonicalTypedInputV2(value.Input, "submission_v2.input"); public != nil {
		return public
	}
	if public := validateCanonicalTypedConfigSemanticsV2(value.Composer, value.Config, v.Presence, "submission_v2"); public != nil {
		return public
	}
	lineagePresent := canonicalV2Has(v.Presence, "submission_v2.lineage")
	if value.Kind == "turn" {
		if lineagePresent {
			return canonicalTypedV2Invalid("submission_v2.lineage")
		}
		if value.Metadata != nil && value.Metadata.Source != "workbench_new_task" && value.Metadata.Source != "workbench_detail_followup" {
			return canonicalTypedV2Invalid("submission_v2.metadata.source")
		}
	} else {
		if !lineagePresent || value.Lineage == nil || value.Lineage.SourceRunID <= 0 {
			return canonicalTypedV2Invalid("submission_v2.lineage")
		}
		if len(value.Input.UploadedFiles) != 0 {
			return canonicalTypedV2Invalid("submission_v2.input.uploaded_files")
		}
		if value.Metadata != nil && value.Metadata.Source != "task_retry" {
			return canonicalTypedV2Invalid("submission_v2.metadata.source")
		}
	}
	return nil
}

func validateCanonicalInitialRunSubmissionV2(v *canonicalTypedInitialV2, deferred bool) *canonicalError {
	root := "initial_submission_v2"
	if deferred {
		root = "deferred_initial_submission_v2"
	}
	if v == nil || v.Value.SchemaVersion != "coze.workbench.initial_run_submission.v2" {
		return canonicalTypedV2Invalid(root + ".schema_version")
	}
	if public := validateCanonicalTypedInputV2(v.Value.Input, root+".input"); public != nil {
		return public
	}
	if len(v.Value.Input.UploadedFiles) != 0 {
		return canonicalTypedV2Invalid(root + ".input.uploaded_files")
	}
	if v.Value.Metadata != nil && v.Value.Metadata.Source != "workbench_new_task" {
		return canonicalTypedV2Invalid(root + ".metadata.source")
	}
	return validateCanonicalTypedConfigSemanticsV2(v.Value.Composer, v.Value.Config, v.Presence, root)
}

func validateCanonicalHumanResponseV2(v *canonicalTypedHumanV2) (canonicalResumeResponse, *canonicalError) {
	if v == nil {
		return canonicalResumeResponse{}, canonicalTypedV2Invalid("response_v2")
	}
	x := v.Value
	if x.Schema != "coze.human_interaction_response.v1" {
		return canonicalResumeResponse{}, canonicalTypedV2Invalid("response_v2.schema")
	}
	if !canonicalV2ValidIdentifier(x.InteractionID, 191) {
		return canonicalResumeResponse{}, canonicalTypedV2Invalid("response_v2.interaction_id")
	}
	result := canonicalResumeResponse{Schema: x.Schema, InteractionID: x.InteractionID, Kind: x.Kind, Decision: x.Decision}
	switch x.Kind {
	case "clarification":
		if x.Decision != "answered" {
			return canonicalResumeResponse{}, canonicalTypedV2Invalid("response_v2.decision")
		}
		if x.Comment != nil {
			return canonicalResumeResponse{}, canonicalTypedV2Invalid("response_v2.comment")
		}
		if x.Answer == nil && x.ChoiceID == nil {
			return canonicalResumeResponse{}, canonicalTypedV2Invalid("response_v2.answer")
		}
		if x.Answer != nil {
			result.Answer = strings.TrimSpace(*x.Answer)
			if result.Answer == "" || len(*x.Answer) > 8192 {
				return canonicalResumeResponse{}, canonicalTypedV2Invalid("response_v2.answer")
			}
		}
		if x.ChoiceID != nil {
			result.ChoiceID = strings.TrimSpace(*x.ChoiceID)
			if result.ChoiceID != *x.ChoiceID || !canonicalV2ValidIdentifier(result.ChoiceID, 191) {
				return canonicalResumeResponse{}, canonicalTypedV2Invalid("response_v2.choice_id")
			}
		}
	case "confirmation":
		if x.Decision != "approved" && x.Decision != "rejected" {
			return canonicalResumeResponse{}, canonicalTypedV2Invalid("response_v2.decision")
		}
		if x.Answer != nil {
			return canonicalResumeResponse{}, canonicalTypedV2Invalid("response_v2.answer")
		}
		if x.ChoiceID != nil {
			return canonicalResumeResponse{}, canonicalTypedV2Invalid("response_v2.choice_id")
		}
		if x.Comment != nil {
			result.Comment = strings.TrimSpace(*x.Comment)
			if result.Comment == "" || len(*x.Comment) > 8192 {
				return canonicalResumeResponse{}, canonicalTypedV2Invalid("response_v2.comment")
			}
		}
	default:
		return canonicalResumeResponse{}, canonicalTypedV2Invalid("response_v2.kind")
	}
	return result, nil
}

func mapCanonicalTypedRunV2(v *canonicalTypedRunV2) (*canonicalRunSubmission, *canonicalError) {
	if public := validateCanonicalRunSubmissionV2(v); public != nil {
		return nil, public
	}
	config, public := canonicalTypedConfigV2(*v.Value.Composer, *v.Value.Config, v.Presence)
	if public != nil {
		return nil, public
	}
	metadata := `{}`
	if v.Value.Metadata != nil {
		raw, _ := json.Marshal(struct {
			Source string `json:"source"`
		}{v.Value.Metadata.Source})
		metadata = string(raw)
	}
	message := strings.TrimSpace(v.Value.Input.Message)
	files := make([]int64, 0, len(v.Value.Input.UploadedFiles))
	refs := make([]canonicalRunUploadedFileReference, 0, len(v.Value.Input.UploadedFiles))
	for _, file := range v.Value.Input.UploadedFiles {
		files = append(files, file.FileID)
		refs = append(refs, canonicalRunUploadedFileReference{FileID: json.RawMessage(strconv.FormatInt(file.FileID, 10))})
	}
	inputRaw, _ := json.Marshal(struct {
		Messages      []canonicalRunInputMessage          `json:"messages"`
		UploadedFiles []canonicalRunUploadedFileReference `json:"uploaded_files"`
	}{[]canonicalRunInputMessage{{Role: "user", Content: message}}, refs})
	mapped := &canonicalRunSubmission{
		AssistantID: canonicalPublicAssistantID, Input: string(inputRaw), Metadata: metadata,
		Config: config, Context: `{}`, UploadedFileIDs: files,
		Options: canonicalRunOptions{
			StreamModes:       append([]string(nil), canonicalRunDefaults.StreamModes...),
			MultitaskStrategy: canonicalRunDefaults.MultitaskStrategy,
			OnDisconnect:      canonicalRunDefaults.OnDisconnect,
			Durability:        canonicalRunDefaults.Durability,
		},
	}
	if v.Value.Kind == "turn" {
		mapped.MessageContent, mapped.MessageMetadata = message, config
		mapped.IdempotencyOperation = canonicalRunIdempotencyOperationTurn
		mapped.IdempotencyFingerprint = canonicalRunTurnRequestFingerprint(mapped)
	} else {
		mapped.TopLevelRetry = &canonicalTopLevelRetrySubmission{SourceRunID: v.Value.Lineage.SourceRunID}
		mapped.IdempotencyOperation = canonicalRunIdempotencyOperationRetry
		fingerprintInput := *mapped
		fingerprintInput.MessageContent = message
		mapped.IdempotencyFingerprint = canonicalRunRetryRequestFingerprint(&fingerprintInput)
	}
	return mapped, nil
}

func mapCanonicalTypedInitialV2(v *canonicalTypedInitialV2, deferred bool) (*canonicalValidatedInitialThreadRun, *canonicalError) {
	if public := validateCanonicalInitialRunSubmissionV2(v, deferred); public != nil {
		return nil, public
	}
	config, public := canonicalTypedConfigV2(*v.Value.Composer, *v.Value.Config, v.Presence)
	if public != nil {
		return nil, public
	}
	metadata := `{}`
	if v.Value.Metadata != nil {
		raw, _ := json.Marshal(struct {
			Source string `json:"source"`
		}{v.Value.Metadata.Source})
		metadata = string(raw)
	}
	return &canonicalValidatedInitialThreadRun{
		AssistantID: canonicalPublicAssistantID, MessageContent: strings.TrimSpace(v.Value.Input.Message),
		Config: config, Context: `{}`, Metadata: metadata,
	}, nil
}

func canonicalTypedConfigV2(composer threadcontract.CanonicalComposerSelectionV2, config threadcontract.CanonicalRunConfigV2, presence canonicalTypedV2Presence) (string, *canonicalError) {
	root := canonicalV2PresenceRoot(presence)
	if public := validateCanonicalTypedConfigSemanticsV2(&composer, &config, presence, root); public != nil {
		return "", public
	}
	out := canonicalTypedConfigV2Output{ModelType: composer.ModelType, ModelName: composer.ModelName, EnableMCP: append([]string{}, composer.EnableMcp...), EnableKBs: append([]string{}, composer.EnableKbs...), EnableDatabases: append([]string{}, composer.EnableDatabases...), Runtime: config.Runtime,
		MemoryRetrieval: canonicalTypedMemoryV2Output{config.MemoryRetrieval.Limit, config.MemoryRetrieval.CandidateLimit, append([]string{}, config.MemoryRetrieval.Scopes...), config.MemoryRetrieval.MinConfidence},
		Skills:          canonicalTypedSkillsV2Output{config.Skills.Enabled, config.Skills.Visibility, append([]string{}, composer.AllowedSkills...)},
		MCPTools:        canonicalTypedMCPToolsV2Output{Enabled: config.McpTools.Enabled, Visibility: config.McpTools.Visibility},
		WebTools:        canonicalTypedWebToolsV2Output{Enabled: config.WebTools.Enabled, Visibility: config.WebTools.Visibility, HTTP: canonicalTypedWebHTTPV2Output{config.WebTools.HTTP.Enabled, append([]string{}, config.WebTools.HTTP.AllowedHosts...), config.WebTools.HTTP.TimeoutMs, config.WebTools.HTTP.MaxResponseBytes}, Search: canonicalTypedWebSearchV2Output{config.WebTools.Search.Enabled, config.WebTools.Search.MaxResults}}, TokenUsage: canonicalTypedTokenV2Output{config.TokenUsage.Enabled}}
	if canonicalV2Has(presence, root+".composer.explicit_enable_skills") {
		value := append([]string{}, composer.ExplicitEnableSkills...)
		out.EnableSkills = &value
	}
	if !config.McpTools.Enabled || len(composer.AllowedMcpTools) > 0 {
		value := append([]string{}, composer.AllowedMcpTools...)
		out.MCPTools.AllowedTools = &value
	}
	if config.ModelRetry != nil {
		x := config.ModelRetry
		out.ModelRetry = &canonicalTypedRetryV2Output{x.MaxRetries, x.BackoffMs, x.RetryEmptyOutput, append([]string{}, x.RetryFinishReasons...)}
	}
	if config.ModelFailover != nil {
		x := config.ModelFailover
		out.ModelFailover = &canonicalTypedFailoverV2Output{append([]int64{}, x.CandidateModelIds...), x.MaxRetries, x.FailoverEmptyOutput, append([]string{}, x.FailoverFinishReasons...)}
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return "", canonicalTypedV2Invalid(root + ".config")
	}
	return string(raw), nil
}

func validateCanonicalTypedInputV2(input *threadcontract.CanonicalRunInputV2, path string) *canonicalError {
	if input == nil {
		return canonicalTypedV2Invalid(path)
	}
	message := strings.TrimSpace(input.Message)
	if message == "" || len(input.Message) > 256*1024 || !utf8.ValidString(input.Message) {
		return canonicalTypedV2Invalid(path + ".message")
	}
	if len(input.UploadedFiles) > 10 {
		return canonicalTypedV2Invalid(path + ".uploaded_files")
	}
	seen := make(map[int64]struct{}, len(input.UploadedFiles))
	for index, file := range input.UploadedFiles {
		filePath := canonicalV2IndexPath(path+".uploaded_files", index) + ".file_id"
		if file == nil || file.FileID <= 0 {
			return canonicalTypedV2Invalid(filePath)
		}
		if _, duplicate := seen[file.FileID]; duplicate {
			return canonicalTypedV2Invalid(filePath)
		}
		seen[file.FileID] = struct{}{}
	}
	return nil
}

func validateCanonicalTypedConfigSemanticsV2(composer *threadcontract.CanonicalComposerSelectionV2, config *threadcontract.CanonicalRunConfigV2, presence canonicalTypedV2Presence, root string) *canonicalError {
	if composer == nil {
		return canonicalTypedV2Invalid(root + ".composer")
	}
	if config == nil || config.MemoryRetrieval == nil || config.Skills == nil || config.McpTools == nil || config.WebTools == nil || config.WebTools.HTTP == nil || config.WebTools.Search == nil || config.TokenUsage == nil {
		return canonicalTypedV2Invalid(root + ".config")
	}
	if composer.ModelType != nil && (*composer.ModelType <= 0 || *composer.ModelType > canonicalV2MaxSafeInteger) {
		return canonicalTypedV2Invalid(root + ".composer.model_type")
	}
	if composer.ModelName != nil {
		name := *composer.ModelName
		if name == "" || len(name) > 191 || strings.TrimSpace(name) != name || canonicalSensitiveValuePattern.MatchString(name) {
			return canonicalTypedV2Invalid(root + ".composer.model_name")
		}
	}
	listSpecs := []struct {
		values []string
		path   string
		max    int
	}{{composer.AllowedSkills, ".composer.allowed_skills", 256}, {composer.ExplicitEnableSkills, ".composer.explicit_enable_skills", 256}, {composer.EnableMcp, ".composer.enable_mcp", 256}, {composer.EnableKbs, ".composer.enable_kbs", 256}, {composer.EnableDatabases, ".composer.enable_databases", 256}, {composer.AllowedMcpTools, ".composer.allowed_mcp_tools", 256}, {config.MemoryRetrieval.Scopes, ".config.memory_retrieval.scopes", 3}}
	for _, spec := range listSpecs {
		if public := canonicalV2ValidateStringList(spec.values, root+spec.path, spec.max); public != nil {
			return public
		}
	}
	explicitPresent := canonicalV2Has(presence, root+".composer.explicit_enable_skills")
	if config.Skills.Enabled {
		if explicitPresent && len(composer.ExplicitEnableSkills) == 0 {
			return canonicalTypedV2Invalid(root + ".composer.explicit_enable_skills")
		}
		allowed := canonicalV2StringSet(composer.AllowedSkills)
		for i, item := range composer.ExplicitEnableSkills {
			if _, ok := allowed[item]; !ok {
				return canonicalTypedV2Invalid(canonicalV2IndexPath(root+".composer.explicit_enable_skills", i))
			}
		}
	} else if !explicitPresent || len(composer.ExplicitEnableSkills) != 0 {
		return canonicalTypedV2Invalid(root + ".composer.explicit_enable_skills")
	}
	if config.McpTools.Enabled {
		if len(composer.AllowedMcpTools) == 0 && len(composer.EnableMcp) != 0 {
			return canonicalTypedV2Invalid(root + ".composer.enable_mcp")
		}
		if len(composer.AllowedMcpTools) > 0 && !canonicalV2EqualStrings(composer.EnableMcp, composer.AllowedMcpTools) {
			return canonicalTypedV2Invalid(root + ".composer.enable_mcp")
		}
	} else if len(composer.EnableMcp) != 0 {
		return canonicalTypedV2Invalid(root + ".composer.enable_mcp")
	}
	if config.Runtime != "eino_adk" {
		return canonicalTypedV2Invalid(root + ".config.runtime")
	}
	for _, visibility := range []struct{ value, path string }{{config.Skills.Visibility, "skills"}, {config.McpTools.Visibility, "mcp_tools"}, {config.WebTools.Visibility, "web_tools"}} {
		if visibility.value != "deferred" {
			return canonicalTypedV2Invalid(root + ".config." + visibility.path + ".visibility")
		}
	}
	memory := config.MemoryRetrieval
	if memory.Limit < 1 || memory.Limit > 100 {
		return canonicalTypedV2Invalid(root + ".config.memory_retrieval.limit")
	}
	if memory.CandidateLimit < 1 || memory.CandidateLimit > 100 || memory.CandidateLimit < memory.Limit {
		return canonicalTypedV2Invalid(root + ".config.memory_retrieval.candidate_limit")
	}
	if len(memory.Scopes) < 1 {
		return canonicalTypedV2Invalid(root + ".config.memory_retrieval.scopes")
	}
	for index, scope := range memory.Scopes {
		if scope != "thread" && scope != "run" && scope != "long_term" {
			return canonicalTypedV2Invalid(canonicalV2IndexPath(root+".config.memory_retrieval.scopes", index))
		}
	}
	if math.IsNaN(memory.MinConfidence) || math.IsInf(memory.MinConfidence, 0) || memory.MinConfidence < 0 || memory.MinConfidence > 1 {
		return canonicalTypedV2Invalid(root + ".config.memory_retrieval.min_confidence")
	}
	http := config.WebTools.HTTP
	if len(http.AllowedHosts) > 64 {
		return canonicalTypedV2Invalid(root + ".config.web_tools.http.allowed_hosts")
	}
	hosts := make(map[string]struct{}, len(http.AllowedHosts))
	for index, host := range http.AllowedHosts {
		if !canonicalV2ValidHost(host) {
			return canonicalTypedV2Invalid(canonicalV2IndexPath(root+".config.web_tools.http.allowed_hosts", index))
		}
		normalized := strings.ToLower(host)
		if _, duplicate := hosts[normalized]; duplicate {
			return canonicalTypedV2Invalid(canonicalV2IndexPath(root+".config.web_tools.http.allowed_hosts", index))
		}
		hosts[normalized] = struct{}{}
	}
	if http.TimeoutMs < 1000 || http.TimeoutMs > 60000 {
		return canonicalTypedV2Invalid(root + ".config.web_tools.http.timeout_ms")
	}
	if http.MaxResponseBytes < 1024 || http.MaxResponseBytes > 1024*1024 {
		return canonicalTypedV2Invalid(root + ".config.web_tools.http.max_response_bytes")
	}
	if http.Enabled && len(http.AllowedHosts) == 0 {
		return canonicalTypedV2Invalid(root + ".config.web_tools.http.allowed_hosts")
	}
	search := config.WebTools.Search
	if search.MaxResults < 1 || search.MaxResults > 10 {
		return canonicalTypedV2Invalid(root + ".config.web_tools.search.max_results")
	}
	if config.WebTools.Enabled != (http.Enabled || search.Enabled) {
		return canonicalTypedV2Invalid(root + ".config.web_tools.enabled")
	}
	if retry := config.ModelRetry; retry != nil {
		if retry.MaxRetries < 1 || retry.MaxRetries > 5 {
			return canonicalTypedV2Invalid(root + ".config.model_retry.max_retries")
		}
		if retry.BackoffMs < 0 || retry.BackoffMs > 60000 {
			return canonicalTypedV2Invalid(root + ".config.model_retry.backoff_ms")
		}
		if public := canonicalV2ValidateIdentifiers(retry.RetryFinishReasons, root+".config.model_retry.retry_finish_reasons", 16, 64); public != nil {
			return public
		}
	}
	if failover := config.ModelFailover; failover != nil {
		if len(failover.CandidateModelIds) < 1 || len(failover.CandidateModelIds) > 256 {
			return canonicalTypedV2Invalid(root + ".config.model_failover.candidate_model_ids")
		}
		seen := map[int64]struct{}{}
		for i, id := range failover.CandidateModelIds {
			if id <= 0 || id > canonicalV2MaxSafeInteger {
				return canonicalTypedV2Invalid(canonicalV2IndexPath(root+".config.model_failover.candidate_model_ids", i))
			}
			if _, ok := seen[id]; ok {
				return canonicalTypedV2Invalid(canonicalV2IndexPath(root+".config.model_failover.candidate_model_ids", i))
			}
			seen[id] = struct{}{}
		}
		if failover.MaxRetries < 1 || int(failover.MaxRetries) > len(failover.CandidateModelIds) || failover.MaxRetries > 5 {
			return canonicalTypedV2Invalid(root + ".config.model_failover.max_retries")
		}
		if public := canonicalV2ValidateIdentifiers(failover.FailoverFinishReasons, root+".config.model_failover.failover_finish_reasons", 16, 64); public != nil {
			return public
		}
	}
	return nil
}

func canonicalV2PresenceRoot(p canonicalTypedV2Presence) string {
	for path := range p {
		if index := strings.IndexByte(path, '.'); index > 0 {
			return path[:index]
		}
	}
	return "submission_v2"
}
func canonicalV2Has(p canonicalTypedV2Presence, path string) bool { _, ok := p[path]; return ok }
func canonicalV2StringSet(values []string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, v := range values {
		result[v] = struct{}{}
	}
	return result
}
func canonicalV2EqualStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
func canonicalV2ValidIdentifier(value string, max int) bool {
	return value != "" && len(value) <= max && strings.TrimSpace(value) == value && canonicalIdentifierPattern.MatchString(value) && !canonicalSensitiveValuePattern.MatchString(value)
}
func canonicalV2ValidateStringList(values []string, path string, max int) *canonicalError {
	if len(values) > max {
		return canonicalTypedV2Invalid(path)
	}
	seen := map[string]struct{}{}
	for i, v := range values {
		if !canonicalV2ValidIdentifier(v, 191) {
			return canonicalTypedV2Invalid(canonicalV2IndexPath(path, i))
		}
		if _, ok := seen[v]; ok {
			return canonicalTypedV2Invalid(canonicalV2IndexPath(path, i))
		}
		seen[v] = struct{}{}
	}
	return nil
}
func canonicalV2ValidateIdentifiers(values []string, path string, maxItems, maxLen int) *canonicalError {
	if len(values) > maxItems {
		return canonicalTypedV2Invalid(path)
	}
	seen := map[string]struct{}{}
	for i, v := range values {
		if !canonicalV2ValidIdentifier(v, maxLen) {
			return canonicalTypedV2Invalid(canonicalV2IndexPath(path, i))
		}
		if _, ok := seen[v]; ok {
			return canonicalTypedV2Invalid(canonicalV2IndexPath(path, i))
		}
		seen[v] = struct{}{}
	}
	return nil
}
func canonicalV2ValidHost(host string) bool {
	if host == "" || len(host) > 253 || strings.TrimSpace(host) != host || host != strings.ToLower(host) || canonicalSensitiveValuePattern.MatchString(host) {
		return false
	}
	if strings.ContainsAny(host, "/@?#\\") {
		return false
	}
	parsed, err := url.Parse("//" + host)
	if err != nil || parsed.Host != host || parsed.Hostname() == "" || parsed.Port() != "" {
		return false
	}
	hostname := parsed.Hostname()
	if ip := net.ParseIP(hostname); ip != nil {
		return hostname == ip.String()
	}
	allNumeric := true
	for _, label := range strings.Split(hostname, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for index := 0; index < len(label); index++ {
			char := label[index]
			if char < '0' || char > '9' {
				allNumeric = false
			}
			if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '-' {
				return false
			}
		}
	}
	return !allNumeric
}

func decodeCanonicalTypedRunSubmissionV2(raw []byte) (*canonicalTypedRunV2, *canonicalError) {
	node, public := decodeCanonicalV2Object(raw, "submission_v2", canonicalTypedRunV2Rules())
	if public != nil {
		return nil, public
	}
	return extractCanonicalTypedRunV2(node)
}

func decodeCanonicalTypedInitialSubmissionV2(raw []byte, field string) (*canonicalTypedInitialV2, *canonicalError) {
	node, public := decodeCanonicalV2Object(raw, field, canonicalTypedInitialV2Rules())
	if public != nil {
		return nil, public
	}
	return extractCanonicalTypedInitialV2(node)
}

func decodeCanonicalTypedHumanResponseV2(raw []byte) (*canonicalTypedHumanV2, *canonicalError) {
	node, public := decodeCanonicalV2Object(raw, "response_v2", canonicalTypedHumanV2Rules())
	if public != nil {
		return nil, public
	}
	return extractCanonicalTypedHumanV2(node)
}

func decodeCanonicalV2Object(
	raw []byte,
	root string,
	rules map[string]canonicalV2FieldRule,
) (*canonicalV2Node, *canonicalError) {
	body := bytes.TrimSpace(raw)
	node, public := decodeCanonicalV2Node(body, root)
	if public != nil {
		return nil, public
	}
	if node.kind != canonicalV2ObjectKind {
		return nil, canonicalTypedV2Invalid(root)
	}
	// encoding/json replaces invalid UTF-8 in decoded strings. Inspect the raw
	// slices before closed-shape validation so a hostile key cannot be
	// misclassified as an unsupported field or echoed through its decoded form.
	if path := canonicalV2InvalidUTF8Path(body, node); path != "" {
		return nil, canonicalTypedV2Invalid(path)
	}
	if public = validateCanonicalV2Object(node, rules); public != nil {
		return nil, public
	}
	canonicalV2AttachRaw(body, node)
	return node, nil
}

func decodeCanonicalV2Node(raw []byte, root string) (*canonicalV2Node, *canonicalError) {
	decoder := json.NewDecoder(bytes.NewReader(bytes.TrimSpace(raw)))
	decoder.UseNumber()
	budget := &canonicalV2ParseBudget{root: root}
	node, public := readCanonicalV2NodeWithBudget(decoder, root, 1, budget)
	if public != nil {
		return nil, public
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return nil, canonicalTypedV2JSONError(true)
	}
	return node, nil
}

func readCanonicalV2Node(decoder *json.Decoder, path string) (*canonicalV2Node, *canonicalError) {
	return readCanonicalV2NodeWithBudget(
		decoder,
		path,
		1,
		&canonicalV2ParseBudget{root: path},
	)
}

func readCanonicalV2NodeWithBudget(
	decoder *json.Decoder,
	path string,
	depth int,
	budget *canonicalV2ParseBudget,
) (*canonicalV2Node, *canonicalError) {
	if public := budget.consume(depth, 1); public != nil {
		return nil, public
	}
	start := decoder.InputOffset()
	token, err := decoder.Token()
	if err != nil {
		return nil, canonicalTypedV2JSONError(false)
	}

	node := &canonicalV2Node{path: path, start: start, value: token}
	switch value := token.(type) {
	case json.Delim:
		switch value {
		case '{':
			node.kind = canonicalV2ObjectKind
			node.obj = make(map[string]*canonicalV2Node)
			for decoder.More() {
				if public := budget.consume(depth, 1); public != nil {
					return nil, public
				}
				keyToken, keyErr := decoder.Token()
				if keyErr != nil {
					return nil, canonicalTypedV2JSONError(false)
				}
				key, ok := keyToken.(string)
				if !ok {
					return nil, canonicalTypedV2JSONError(false)
				}
				childPath := canonicalV2ChildPath(path, key)
				if _, duplicate := node.obj[key]; duplicate {
					return nil, canonicalTypedV2Invalid(childPath)
				}
				child, childErr := readCanonicalV2NodeWithBudget(decoder, childPath, depth+1, budget)
				if childErr != nil {
					return nil, childErr
				}
				node.obj[key] = child
				node.order = append(node.order, key)
			}
			closing, closeErr := decoder.Token()
			if closeErr != nil || closing != json.Delim('}') {
				return nil, canonicalTypedV2JSONError(false)
			}
		case '[':
			node.kind = canonicalV2ArrayKind
			for index := 0; decoder.More(); index++ {
				child, childErr := readCanonicalV2NodeWithBudget(
					decoder,
					canonicalV2IndexPath(path, index),
					depth+1,
					budget,
				)
				if childErr != nil {
					return nil, childErr
				}
				node.arr = append(node.arr, child)
			}
			closing, closeErr := decoder.Token()
			if closeErr != nil || closing != json.Delim(']') {
				return nil, canonicalTypedV2JSONError(false)
			}
		default:
			return nil, canonicalTypedV2JSONError(false)
		}
	case string:
		node.kind = canonicalV2StringKind
	case json.Number:
		node.kind = canonicalV2NumberKind
	case bool:
		node.kind = canonicalV2BoolKind
	case nil:
		node.kind = canonicalV2NullKind
	default:
		return nil, canonicalTypedV2JSONError(false)
	}
	node.end = decoder.InputOffset()
	return node, nil
}

func (budget *canonicalV2ParseBudget) consume(depth, units int) *canonicalError {
	if budget == nil || depth > canonicalV2MaxDepth || units > canonicalV2MaxParseUnit-budget.units {
		root := "typed_submission_v2"
		if budget != nil && budget.root != "" {
			root = budget.root
		}
		return canonicalTypedV2Invalid(root)
	}
	budget.units += units
	return nil
}

func validateCanonicalV2Object(node *canonicalV2Node, rules map[string]canonicalV2FieldRule) *canonicalError {
	if node == nil {
		return canonicalTypedV2Invalid("typed_submission_v2")
	}
	if node.kind != canonicalV2ObjectKind {
		return canonicalTypedV2Invalid(node.path)
	}
	for _, name := range node.order {
		if _, ok := rules[name]; !ok {
			return canonicalUnsupportedField(canonicalV2ChildPath(node.path, name))
		}
	}

	names := canonicalV2RuleNamesInSchemaOrder(node.path, rules)
	for _, name := range names {
		rule := rules[name]
		child, present := node.obj[name]
		path := canonicalV2ChildPath(node.path, name)
		if !present {
			if rule.required {
				return canonicalTypedV2Invalid(path)
			}
			continue
		}
		if child.kind == canonicalV2NullKind || child.kind != rule.kind {
			return canonicalTypedV2Invalid(path)
		}
		if public := validateCanonicalV2Rule(child, rule); public != nil {
			return public
		}
	}
	return nil
}

func validateCanonicalV2Rule(node *canonicalV2Node, rule canonicalV2FieldRule) *canonicalError {
	switch rule.kind {
	case canonicalV2ObjectKind:
		return validateCanonicalV2Object(node, rule.object)
	case canonicalV2ArrayKind:
		for _, child := range node.arr {
			if child.kind == canonicalV2NullKind || rule.element == nil || child.kind != rule.element.kind {
				return canonicalTypedV2Invalid(child.path)
			}
			if public := validateCanonicalV2Rule(child, *rule.element); public != nil {
				return public
			}
		}
	}
	return nil
}

func extractCanonicalTypedRunV2(node *canonicalV2Node) (*canonicalTypedRunV2, *canonicalError) {
	var value threadcontract.CanonicalRunSubmissionV2
	if public := canonicalV2UnmarshalGenerated(node, &value); public != nil {
		return nil, public
	}
	return &canonicalTypedRunV2{Value: value, Presence: canonicalV2Presence(node)}, nil
}

func extractCanonicalTypedInitialV2(node *canonicalV2Node) (*canonicalTypedInitialV2, *canonicalError) {
	var value threadcontract.CanonicalInitialRunSubmissionV2
	if public := canonicalV2UnmarshalGenerated(node, &value); public != nil {
		return nil, public
	}
	return &canonicalTypedInitialV2{Value: value, Presence: canonicalV2Presence(node)}, nil
}

func extractCanonicalTypedHumanV2(node *canonicalV2Node) (*canonicalTypedHumanV2, *canonicalError) {
	var value threadcontract.CanonicalHumanInteractionResponseV2
	if public := canonicalV2UnmarshalGenerated(node, &value); public != nil {
		return nil, public
	}
	return &canonicalTypedHumanV2{Value: value, Presence: canonicalV2Presence(node)}, nil
}

func canonicalV2UnmarshalGenerated(node *canonicalV2Node, value any) *canonicalError {
	normalized, public := canonicalV2GeneratedJSONValue(node)
	if public != nil {
		return public
	}
	raw, err := json.Marshal(normalized)
	if err != nil {
		return canonicalTypedV2Invalid(node.path)
	}
	if err = json.Unmarshal(raw, value); err != nil {
		return canonicalTypedV2Invalid(node.path)
	}
	return nil
}

func canonicalV2GeneratedJSONValue(node *canonicalV2Node) (any, *canonicalError) {
	switch node.kind {
	case canonicalV2ObjectKind:
		value := make(map[string]any, len(node.obj))
		for _, name := range node.order {
			childValue, public := canonicalV2GeneratedJSONValue(node.obj[name])
			if public != nil {
				return nil, public
			}
			value[name] = childValue
		}
		return value, nil
	case canonicalV2ArrayKind:
		value := make([]any, 0, len(node.arr))
		for _, child := range node.arr {
			childValue, public := canonicalV2GeneratedJSONValue(child)
			if public != nil {
				return nil, public
			}
			value = append(value, childValue)
		}
		return value, nil
	case canonicalV2StringKind:
		text := node.value.(string)
		if strings.HasSuffix(node.path, ".model_type") ||
			strings.HasSuffix(node.path, ".file_id") ||
			strings.HasSuffix(node.path, ".source_run_id") {
			if !canonicalV2PositiveDecimal(text) {
				return nil, canonicalTypedV2Invalid(node.path)
			}
			parsed, err := strconv.ParseInt(text, 10, 64)
			if err != nil || parsed <= 0 || (strings.HasSuffix(node.path, ".model_type") && parsed > canonicalV2MaxSafeInteger) {
				return nil, canonicalTypedV2Invalid(node.path)
			}
		}
		// The generated Hertz tag for list<i64> does not decode JSON strings
		// element-by-element. Convert only this IDL-declared string-converted list
		// to exact JSON integer digits before populating the generated Go value.
		if strings.Contains(node.path, ".candidate_model_ids[") {
			if !canonicalV2PositiveDecimal(text) {
				return nil, canonicalTypedV2Invalid(node.path)
			}
			parsed, err := strconv.ParseInt(text, 10, 64)
			if err != nil || parsed <= 0 || parsed > canonicalV2MaxSafeInteger {
				return nil, canonicalTypedV2Invalid(node.path)
			}
			return json.Number(strconv.FormatInt(parsed, 10)), nil
		}
		return text, nil
	case canonicalV2NumberKind:
		if public := canonicalV2ValidateGeneratedNumber(node); public != nil {
			return nil, public
		}
		return node.value, nil
	case canonicalV2BoolKind:
		return node.value, nil
	default:
		return nil, canonicalTypedV2Invalid(node.path)
	}
}

func canonicalV2PositiveDecimal(value string) bool {
	if value == "" || value[0] < '1' || value[0] > '9' {
		return false
	}
	for index := 1; index < len(value); index++ {
		if value[index] < '0' || value[index] > '9' {
			return false
		}
	}
	return true
}

func canonicalV2ValidateGeneratedNumber(node *canonicalV2Node) *canonicalError {
	number, ok := node.value.(json.Number)
	if !ok {
		return canonicalTypedV2Invalid(node.path)
	}
	path := node.path
	if strings.HasSuffix(path, ".min_confidence") {
		value, err := strconv.ParseFloat(number.String(), 64)
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
			return canonicalTypedV2Invalid(path)
		}
		return nil
	}
	if strings.HasSuffix(path, ".limit") || strings.HasSuffix(path, ".candidate_limit") ||
		strings.HasSuffix(path, ".max_results") || strings.HasSuffix(path, ".max_retries") {
		value, err := strconv.ParseInt(number.String(), 10, 32)
		if err != nil || strconv.FormatInt(value, 10) != number.String() {
			return canonicalTypedV2Invalid(path)
		}
		return nil
	}
	value, err := strconv.ParseInt(number.String(), 10, 64)
	if err != nil || strconv.FormatInt(value, 10) != number.String() {
		return canonicalTypedV2Invalid(path)
	}
	return nil
}

func canonicalV2Presence(node *canonicalV2Node) canonicalTypedV2Presence {
	presence := make(canonicalTypedV2Presence)
	var visit func(*canonicalV2Node)
	visit = func(current *canonicalV2Node) {
		if current == nil {
			return
		}
		if current != node {
			presence[current.path] = struct{}{}
		}
		for _, name := range current.order {
			visit(current.obj[name])
		}
		for _, child := range current.arr {
			visit(child)
		}
	}
	visit(node)
	return presence
}

func canonicalV2InvalidUTF8Path(raw []byte, node *canonicalV2Node) string {
	for _, name := range node.order {
		if path := canonicalV2InvalidUTF8Path(raw, node.obj[name]); path != "" {
			return path
		}
	}
	for _, child := range node.arr {
		if path := canonicalV2InvalidUTF8Path(raw, child); path != "" {
			return path
		}
	}
	if node.start >= 0 && node.end >= node.start && node.end <= int64(len(raw)) &&
		!utf8.Valid(raw[node.start:node.end]) {
		return node.path
	}
	return ""
}

func canonicalV2AttachRaw(raw []byte, node *canonicalV2Node) {
	if node.start >= 0 && node.end >= node.start && node.end <= int64(len(raw)) {
		node.raw = append(node.raw[:0], raw[node.start:node.end]...)
	}
	for _, name := range node.order {
		canonicalV2AttachRaw(raw, node.obj[name])
	}
	for _, child := range node.arr {
		canonicalV2AttachRaw(raw, child)
	}
}

func canonicalV2RuleNamesInSchemaOrder(
	path string,
	rules map[string]canonicalV2FieldRule,
) []string {
	var preferred []string
	switch {
	case path == "submission_v2":
		preferred = []string{"schema_version", "kind", "input", "composer", "config", "lineage", "metadata"}
	case path == "initial_submission_v2" || path == "deferred_initial_submission_v2":
		preferred = []string{"schema_version", "input", "composer", "config", "metadata"}
	case path == "response_v2":
		preferred = []string{"schema", "interaction_id", "kind", "decision", "answer", "choice_id", "comment"}
	case strings.HasSuffix(path, ".input"):
		preferred = []string{"message", "uploaded_files"}
	case strings.Contains(path, ".uploaded_files["):
		preferred = []string{"file_id"}
	case strings.HasSuffix(path, ".composer"):
		preferred = []string{"model_type", "model_name", "explicit_enable_skills", "allowed_skills", "enable_mcp", "enable_kbs", "enable_databases", "allowed_mcp_tools"}
	case strings.HasSuffix(path, ".config"):
		preferred = []string{"runtime", "memory_retrieval", "skills", "mcp_tools", "web_tools", "model_retry", "model_failover", "token_usage"}
	case strings.HasSuffix(path, ".memory_retrieval"):
		preferred = []string{"limit", "candidate_limit", "scopes", "min_confidence"}
	case strings.HasSuffix(path, ".skills") || strings.HasSuffix(path, ".mcp_tools"):
		preferred = []string{"enabled", "visibility"}
	case strings.HasSuffix(path, ".web_tools"):
		preferred = []string{"enabled", "visibility", "http", "search"}
	case strings.HasSuffix(path, ".http"):
		preferred = []string{"enabled", "allowed_hosts", "timeout_ms", "max_response_bytes"}
	case strings.HasSuffix(path, ".search"):
		preferred = []string{"enabled", "max_results"}
	case strings.HasSuffix(path, ".model_retry"):
		preferred = []string{"max_retries", "backoff_ms", "retry_empty_output", "retry_finish_reasons"}
	case strings.HasSuffix(path, ".model_failover"):
		preferred = []string{"candidate_model_ids", "max_retries", "failover_empty_output", "failover_finish_reasons"}
	case strings.HasSuffix(path, ".token_usage"):
		preferred = []string{"enabled"}
	case strings.HasSuffix(path, ".lineage"):
		preferred = []string{"source_run_id"}
	case strings.HasSuffix(path, ".metadata"):
		preferred = []string{"source"}
	}

	ordered := make([]string, 0, len(rules))
	seen := make(map[string]struct{}, len(rules))
	for _, name := range preferred {
		if _, ok := rules[name]; ok {
			ordered = append(ordered, name)
			seen[name] = struct{}{}
		}
	}
	remainder := make([]string, 0, len(rules)-len(ordered))
	for name := range rules {
		if _, ok := seen[name]; !ok {
			remainder = append(remainder, name)
		}
	}
	sort.Strings(remainder)
	return append(ordered, remainder...)
}

func validateCanonicalTypedRunVersionMixing(raw []byte) *canonicalError {
	root, ok := canonicalV2RootRawMessages(raw)
	if !ok || root["submission_v2"] == nil {
		return nil
	}
	for _, field := range []string{"input", "command", "metadata", "config", "context", "coze"} {
		if root[field] != nil {
			return canonicalTypedV2Mixed("submission_v2")
		}
	}
	return nil
}

func validateCanonicalTypedThreadVersionMixing(raw []byte) *canonicalError {
	root, ok := canonicalV2RootRawMessages(raw)
	if !ok {
		return nil
	}
	initial := root["initial_submission_v2"] != nil
	deferred := root["deferred_initial_submission_v2"] != nil
	if initial && deferred {
		return canonicalTypedV2Mixed("initial_submission_v2")
	}
	coze, cozeOK := canonicalV2RawObject(root["coze"])
	legacy := cozeOK && (coze["initial_run"] != nil || coze["deferred_initial_run"] != nil)
	if initial && legacy {
		return canonicalTypedV2Mixed("initial_submission_v2")
	}
	if deferred && legacy {
		return canonicalTypedV2Mixed("deferred_initial_submission_v2")
	}
	return nil
}

func canonicalV2RootRawMessages(raw []byte) (map[string]json.RawMessage, bool) {
	return canonicalV2RawObject(bytes.TrimSpace(raw))
}

func canonicalV2RawObject(raw []byte) (map[string]json.RawMessage, bool) {
	if len(raw) == 0 || raw[0] != '{' {
		return nil, false
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	var value map[string]json.RawMessage
	if err := decoder.Decode(&value); err != nil {
		return nil, false
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return nil, false
	}
	return value, true
}

func canonicalTypedV2Rules() map[string]canonicalV2FieldRule {
	return map[string]canonicalV2FieldRule{
		"runtime": canonicalV2Required(canonicalV2StringKind),
		"memory_retrieval": canonicalV2RequiredObject(map[string]canonicalV2FieldRule{
			"limit":           canonicalV2Required(canonicalV2NumberKind),
			"candidate_limit": canonicalV2Required(canonicalV2NumberKind),
			"scopes":          canonicalV2RequiredStringArray(),
			"min_confidence":  canonicalV2Required(canonicalV2NumberKind),
		}),
		"skills":    canonicalV2RequiredObject(canonicalV2VisibilityRules()),
		"mcp_tools": canonicalV2RequiredObject(canonicalV2VisibilityRules()),
		"web_tools": canonicalV2RequiredObject(map[string]canonicalV2FieldRule{
			"enabled":    canonicalV2Required(canonicalV2BoolKind),
			"visibility": canonicalV2Required(canonicalV2StringKind),
			"http": canonicalV2RequiredObject(map[string]canonicalV2FieldRule{
				"enabled":            canonicalV2Required(canonicalV2BoolKind),
				"allowed_hosts":      canonicalV2RequiredStringArray(),
				"timeout_ms":         canonicalV2Required(canonicalV2NumberKind),
				"max_response_bytes": canonicalV2Required(canonicalV2NumberKind),
			}),
			"search": canonicalV2RequiredObject(map[string]canonicalV2FieldRule{
				"enabled":     canonicalV2Required(canonicalV2BoolKind),
				"max_results": canonicalV2Required(canonicalV2NumberKind),
			}),
		}),
		"model_retry": canonicalV2OptionalObject(map[string]canonicalV2FieldRule{
			"max_retries":          canonicalV2Required(canonicalV2NumberKind),
			"backoff_ms":           canonicalV2Required(canonicalV2NumberKind),
			"retry_empty_output":   canonicalV2Required(canonicalV2BoolKind),
			"retry_finish_reasons": canonicalV2RequiredStringArray(),
		}),
		"model_failover": canonicalV2OptionalObject(map[string]canonicalV2FieldRule{
			"candidate_model_ids":     canonicalV2RequiredStringArray(),
			"max_retries":             canonicalV2Required(canonicalV2NumberKind),
			"failover_empty_output":   canonicalV2Required(canonicalV2BoolKind),
			"failover_finish_reasons": canonicalV2RequiredStringArray(),
		}),
		"token_usage": canonicalV2RequiredObject(map[string]canonicalV2FieldRule{
			"enabled": canonicalV2Required(canonicalV2BoolKind),
		}),
	}
}

func canonicalTypedRunV2Rules() map[string]canonicalV2FieldRule {
	return map[string]canonicalV2FieldRule{
		"schema_version": canonicalV2Required(canonicalV2StringKind),
		"kind":           canonicalV2Required(canonicalV2StringKind),
		"input":          canonicalV2RequiredObject(canonicalTypedInputV2Rules()),
		"composer":       canonicalV2RequiredObject(canonicalTypedComposerV2Rules()),
		"config":         canonicalV2RequiredObject(canonicalTypedV2Rules()),
		"lineage": canonicalV2OptionalObject(map[string]canonicalV2FieldRule{
			"source_run_id": canonicalV2Required(canonicalV2StringKind),
		}),
		"metadata": canonicalV2OptionalObject(canonicalTypedMetadataV2Rules()),
	}
}

func canonicalTypedInitialV2Rules() map[string]canonicalV2FieldRule {
	return map[string]canonicalV2FieldRule{
		"schema_version": canonicalV2Required(canonicalV2StringKind),
		"input":          canonicalV2RequiredObject(canonicalTypedInputV2Rules()),
		"composer":       canonicalV2RequiredObject(canonicalTypedComposerV2Rules()),
		"config":         canonicalV2RequiredObject(canonicalTypedV2Rules()),
		"metadata":       canonicalV2OptionalObject(canonicalTypedMetadataV2Rules()),
	}
}

func canonicalTypedHumanV2Rules() map[string]canonicalV2FieldRule {
	return map[string]canonicalV2FieldRule{
		"schema":         canonicalV2Required(canonicalV2StringKind),
		"interaction_id": canonicalV2Required(canonicalV2StringKind),
		"kind":           canonicalV2Required(canonicalV2StringKind),
		"decision":       canonicalV2Required(canonicalV2StringKind),
		"answer":         canonicalV2Optional(canonicalV2StringKind),
		"choice_id":      canonicalV2Optional(canonicalV2StringKind),
		"comment":        canonicalV2Optional(canonicalV2StringKind),
	}
}

func canonicalTypedInputV2Rules() map[string]canonicalV2FieldRule {
	file := canonicalV2FieldRule{
		kind: canonicalV2ObjectKind,
		object: map[string]canonicalV2FieldRule{
			"file_id": canonicalV2Required(canonicalV2StringKind),
		},
	}
	return map[string]canonicalV2FieldRule{
		"message":        canonicalV2Required(canonicalV2StringKind),
		"uploaded_files": {required: true, kind: canonicalV2ArrayKind, element: &file},
	}
}

func canonicalTypedComposerV2Rules() map[string]canonicalV2FieldRule {
	return map[string]canonicalV2FieldRule{
		"model_type":             canonicalV2Optional(canonicalV2StringKind),
		"model_name":             canonicalV2Optional(canonicalV2StringKind),
		"explicit_enable_skills": canonicalV2OptionalStringArray(),
		"allowed_skills":         canonicalV2RequiredStringArray(),
		"enable_mcp":             canonicalV2RequiredStringArray(),
		"enable_kbs":             canonicalV2RequiredStringArray(),
		"enable_databases":       canonicalV2RequiredStringArray(),
		"allowed_mcp_tools":      canonicalV2RequiredStringArray(),
	}
}

func canonicalTypedMetadataV2Rules() map[string]canonicalV2FieldRule {
	return map[string]canonicalV2FieldRule{
		"source": canonicalV2Required(canonicalV2StringKind),
	}
}

func canonicalV2VisibilityRules() map[string]canonicalV2FieldRule {
	return map[string]canonicalV2FieldRule{
		"enabled":    canonicalV2Required(canonicalV2BoolKind),
		"visibility": canonicalV2Required(canonicalV2StringKind),
	}
}

func canonicalV2Required(kind byte) canonicalV2FieldRule {
	return canonicalV2FieldRule{required: true, kind: kind}
}

func canonicalV2Optional(kind byte) canonicalV2FieldRule {
	return canonicalV2FieldRule{kind: kind}
}

func canonicalV2RequiredObject(fields map[string]canonicalV2FieldRule) canonicalV2FieldRule {
	return canonicalV2FieldRule{required: true, kind: canonicalV2ObjectKind, object: fields}
}

func canonicalV2OptionalObject(fields map[string]canonicalV2FieldRule) canonicalV2FieldRule {
	return canonicalV2FieldRule{kind: canonicalV2ObjectKind, object: fields}
}

func canonicalV2RequiredStringArray() canonicalV2FieldRule {
	element := canonicalV2Required(canonicalV2StringKind)
	return canonicalV2FieldRule{required: true, kind: canonicalV2ArrayKind, element: &element}
}

func canonicalV2OptionalStringArray() canonicalV2FieldRule {
	element := canonicalV2Required(canonicalV2StringKind)
	return canonicalV2FieldRule{kind: canonicalV2ArrayKind, element: &element}
}

func canonicalV2ChildPath(parent, child string) string {
	child = canonicalV2SafeKey(child)
	if parent == "" {
		return child
	}
	return parent + "." + child
}

func canonicalV2SafeKey(key string) string {
	if len(key) == 0 || len(key) > canonicalV2MaxSafeKey {
		return "<unsupported>"
	}
	for index := 0; index < len(key); index++ {
		char := key[index]
		if index == 0 {
			if (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') && char != '_' {
				return "<unsupported>"
			}
			continue
		}
		if (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') &&
			(char < '0' || char > '9') && char != '_' {
			return "<unsupported>"
		}
	}
	return key
}

func canonicalV2IndexPath(parent string, index int) string {
	return parent + "[" + strconv.Itoa(index) + "]"
}

func canonicalTypedV2JSONError(trailing bool) *canonicalError {
	if trailing {
		return newCanonicalError(
			hertzconsts.StatusBadRequest,
			"invalid_json",
			"Request body must contain one JSON object",
			"trailing_json",
			false,
		)
	}
	return newCanonicalError(
		hertzconsts.StatusBadRequest,
		"invalid_json",
		"Request body is not valid JSON",
		"invalid_json",
		false,
	)
}

func canonicalTypedV2Invalid(path string) *canonicalError {
	return canonicalInvalidRequest("Invalid typed submission field: "+path, "invalid_typed_submission")
}

func canonicalTypedV2Mixed(path string) *canonicalError {
	return canonicalInvalidRequest("Mixed submission versions: "+path, "mixed_submission_versions")
}
