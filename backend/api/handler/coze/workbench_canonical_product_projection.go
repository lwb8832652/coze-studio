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
	"encoding/json"
	"fmt"
	"time"

	appagentthread "github.com/coze-dev/coze-studio/backend/application/agentthread"
)

type canonicalProductUpload struct {
	FileID      string `json:"file_id"`
	FileName    string `json:"file_name"`
	VirtualPath string `json:"virtual_path"`
	ContentType string `json:"content_type"`
	SizeBytes   int64  `json:"size_bytes"`
	CreatedAt   string `json:"created_at"`
}

type canonicalProductArtifact struct {
	ArtifactID   string         `json:"artifact_id"`
	ThreadID     string         `json:"thread_id"`
	RunID        string         `json:"run_id"`
	FileID       string         `json:"file_id"`
	Title        string         `json:"title"`
	ArtifactType string         `json:"artifact_type"`
	VirtualPath  string         `json:"virtual_path"`
	ContentType  string         `json:"content_type"`
	SizeBytes    int64          `json:"size_bytes"`
	PreviewMode  string         `json:"preview_mode"`
	Metadata     map[string]any `json:"metadata"`
	CreatedAt    string         `json:"created_at"`
	UpdatedAt    string         `json:"updated_at"`
	DeletedAt    string         `json:"deleted_at,omitempty"`
}

type canonicalProductArtifactScanJob struct {
	JobID        string `json:"job_id"`
	ThreadID     string `json:"thread_id"`
	RunID        string `json:"run_id"`
	ArtifactID   string `json:"artifact_id"`
	FileID       string `json:"file_id"`
	Scanner      string `json:"scanner"`
	Status       string `json:"status"`
	WorkerRef    string `json:"worker_ref"`
	AttemptCount int32  `json:"attempt_count"`
	ErrorCode    string `json:"error_code"`
	AvailableAt  string `json:"available_at,omitempty"`
	StartedAt    string `json:"started_at,omitempty"`
	EndedAt      string `json:"ended_at,omitempty"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

type canonicalProductTokenUsage struct {
	UsageID      string `json:"usage_id"`
	ThreadID     string `json:"thread_id"`
	RunID        string `json:"run_id"`
	Source       string `json:"source"`
	StepID       string `json:"step_id"`
	StepIndex    int32  `json:"step_index"`
	StepName     string `json:"step_name"`
	ModelName    string `json:"model_name"`
	Provider     string `json:"provider"`
	InputTokens  int64  `json:"input_tokens"`
	OutputTokens int64  `json:"output_tokens"`
	TotalTokens  int64  `json:"total_tokens"`
	CostMicros   int64  `json:"cost_micros"`
	Currency     string `json:"currency"`
	Estimated    bool   `json:"estimated"`
	CreatedAt    string `json:"created_at"`
}

type canonicalProductTokenUsageAggregate struct {
	InputTokens      int64 `json:"input_tokens"`
	OutputTokens     int64 `json:"output_tokens"`
	TotalTokens      int64 `json:"total_tokens"`
	CostMicros       int64 `json:"cost_micros"`
	CallCount        int64 `json:"call_count"`
	LeadAgentTokens  int64 `json:"lead_agent_tokens"`
	SubagentTokens   int64 `json:"subagent_tokens"`
	MiddlewareTokens int64 `json:"middleware_tokens"`
	ToolTokens       int64 `json:"tool_tokens"`
}

type canonicalProductRunTokenUsageAggregate struct {
	RunID     string                              `json:"run_id"`
	Aggregate canonicalProductTokenUsageAggregate `json:"aggregate"`
}

type canonicalProductMemory struct {
	MemoryID             string         `json:"memory_id"`
	ThreadID             string         `json:"thread_id"`
	RunID                string         `json:"run_id,omitempty"`
	Scope                string         `json:"scope"`
	Content              string         `json:"content"`
	Metadata             map[string]any `json:"metadata"`
	Score                float64        `json:"score"`
	Confidence           float64        `json:"confidence"`
	SourceType           string         `json:"source_type"`
	SourceID             string         `json:"source_id"`
	CorrectionOfMemoryID string         `json:"correction_of_memory_id,omitempty"`
	CorrectedAt          string         `json:"corrected_at,omitempty"`
	ExpiresAt            string         `json:"expires_at,omitempty"`
	CreatedAt            string         `json:"created_at"`
	UpdatedAt            string         `json:"updated_at"`
	DeletedAt            string         `json:"deleted_at,omitempty"`
}

type canonicalProductMemoryAudit struct {
	EventID       string `json:"event_id"`
	ThreadID      string `json:"thread_id"`
	RunID         string `json:"run_id,omitempty"`
	MemoryID      string `json:"memory_id,omitempty"`
	ActorID       string `json:"actor_id,omitempty"`
	EventType     string `json:"event_type"`
	Scope         string `json:"scope"`
	SourceType    string `json:"source_type"`
	SourceID      string `json:"source_id"`
	AffectedCount int64  `json:"affected_count"`
	CreatedAt     string `json:"created_at"`
}

type canonicalProductGuardrailAudit struct {
	EventID    string   `json:"event_id"`
	ThreadID   string   `json:"thread_id"`
	RunID      string   `json:"run_id,omitempty"`
	ActorID    string   `json:"actor_id,omitempty"`
	EventType  string   `json:"event_type"`
	TargetType string   `json:"target_type"`
	TargetID   string   `json:"target_id"`
	Operation  string   `json:"operation"`
	Source     string   `json:"source"`
	Action     string   `json:"action"`
	FailMode   string   `json:"fail_mode"`
	Provider   string   `json:"provider"`
	ReasonCode string   `json:"reason_code"`
	RuleIDs    []string `json:"rule_ids"`
	CreatedAt  string   `json:"created_at"`
}

type canonicalProductMCPRuntimeAudit struct {
	EventID         string `json:"event_id"`
	ThreadID        string `json:"thread_id"`
	RunID           string `json:"run_id,omitempty"`
	ServerID        string `json:"server_id,omitempty"`
	RuntimeToolName string `json:"runtime_tool_name"`
	EventType       string `json:"event_type"`
	ErrorCode       string `json:"error_code"`
	ElapsedMillis   int64  `json:"elapsed_millis"`
	OutputBytes     int64  `json:"output_bytes"`
	CreatedAt       string `json:"created_at"`
}

func projectCanonicalProductUpload(summary *appagentthread.TaskThreadUploadedFileSummary) (*canonicalProductUpload, error) {
	if summary == nil {
		return nil, nil
	}
	fileID, err := canonicalProductRequiredID(summary.FileID, "upload file")
	if err != nil {
		return nil, err
	}
	createdAt, err := canonicalProductRequiredTime(summary.CreatedAt, "upload created_at")
	if err != nil {
		return nil, err
	}
	return &canonicalProductUpload{FileID: fileID, FileName: canonicalCleanString(summary.FileName, 512), VirtualPath: canonicalCleanString(summary.VirtualPath, 4096), ContentType: canonicalCleanString(summary.ContentType, 128), SizeBytes: summary.SizeBytes, CreatedAt: createdAt}, nil
}

func projectCanonicalProductArtifact(summary *appagentthread.ArtifactSummary) (*canonicalProductArtifact, error) {
	public := appagentthread.ProjectPublicArtifact(summary)
	if public == nil {
		return nil, nil
	}
	artifactID, err := canonicalProductRequiredID(public.ArtifactID, "artifact")
	if err != nil {
		return nil, err
	}
	threadID, err := canonicalProductRequiredID(public.ThreadID, "artifact thread")
	if err != nil {
		return nil, err
	}
	runID, err := canonicalProductRequiredID(public.RunID, "artifact run")
	if err != nil {
		return nil, err
	}
	fileID, err := canonicalProductRequiredID(public.FileID, "artifact file")
	if err != nil {
		return nil, err
	}
	createdAt, err := canonicalProductRequiredTime(public.CreatedAt, "artifact created_at")
	if err != nil {
		return nil, err
	}
	updatedAt, err := canonicalProductRequiredTime(public.UpdatedAt, "artifact updated_at")
	if err != nil {
		return nil, err
	}
	return &canonicalProductArtifact{ArtifactID: artifactID, ThreadID: threadID, RunID: runID, FileID: fileID, Title: canonicalCleanString(public.Title, 512), ArtifactType: canonicalProductIdentifier(public.ArtifactType), VirtualPath: canonicalCleanString(public.VirtualPath, 4096), ContentType: canonicalCleanString(public.ContentType, 128), SizeBytes: public.SizeBytes, PreviewMode: canonicalProductIdentifier(string(public.PreviewMode)), Metadata: canonicalSanitizeMap(canonicalEntityMetadataFromJSON(public.Metadata, "")), CreatedAt: createdAt, UpdatedAt: updatedAt, DeletedAt: canonicalTime(public.DeletedAt)}, nil
}

func projectCanonicalProductArtifactScanJob(summary *appagentthread.ArtifactScanJobSummary) (*canonicalProductArtifactScanJob, error) {
	if summary == nil {
		return nil, nil
	}
	jobID, err := canonicalProductRequiredID(summary.JobID, "artifact scan job")
	if err != nil {
		return nil, err
	}
	threadID, err := canonicalProductRequiredID(summary.ThreadID, "artifact scan job thread")
	if err != nil {
		return nil, err
	}
	runID, err := canonicalProductRequiredID(summary.RunID, "artifact scan job run")
	if err != nil {
		return nil, err
	}
	artifactID, err := canonicalProductRequiredID(summary.ArtifactID, "artifact scan job artifact")
	if err != nil {
		return nil, err
	}
	fileID, err := canonicalProductRequiredID(summary.FileID, "artifact scan job file")
	if err != nil {
		return nil, err
	}
	createdAt, err := canonicalProductRequiredTime(summary.CreatedAt, "artifact scan job created_at")
	if err != nil {
		return nil, err
	}
	updatedAt, err := canonicalProductRequiredTime(summary.UpdatedAt, "artifact scan job updated_at")
	if err != nil {
		return nil, err
	}
	status := canonicalProductScanStatus(string(summary.Status))
	result := &canonicalProductArtifactScanJob{JobID: jobID, ThreadID: threadID, RunID: runID, ArtifactID: artifactID, FileID: fileID, Scanner: canonicalProductIdentifier(summary.Scanner), Status: status, WorkerRef: canonicalLogHash(summary.WorkerID), AttemptCount: summary.AttemptCount, ErrorCode: "none", AvailableAt: canonicalTime(summary.AvailableAt), StartedAt: canonicalTime(summary.StartedAt), EndedAt: canonicalTime(summary.EndedAt), CreatedAt: createdAt, UpdatedAt: updatedAt}
	if status == "failed" {
		result.ErrorCode = "scan_failed"
	}
	return result, nil
}

func projectCanonicalProductTokenUsage(summary *appagentthread.TokenUsageSummary) (*canonicalProductTokenUsage, error) {
	public := appagentthread.ProjectPublicTokenUsage(summary)
	if public == nil {
		return nil, nil
	}
	usageID, err := canonicalProductRequiredID(public.UsageID, "token usage")
	if err != nil {
		return nil, err
	}
	threadID, err := canonicalProductRequiredID(public.ThreadID, "token usage thread")
	if err != nil {
		return nil, err
	}
	runID, err := canonicalProductRequiredID(public.RunID, "token usage run")
	if err != nil {
		return nil, err
	}
	createdAt, err := canonicalProductRequiredTime(public.CreatedAt, "token usage created_at")
	if err != nil {
		return nil, err
	}
	return &canonicalProductTokenUsage{UsageID: usageID, ThreadID: threadID, RunID: runID, Source: canonicalProductIdentifier(string(public.Source)), StepID: canonicalProductIdentifier(public.StepID), StepIndex: public.StepIndex, StepName: canonicalCleanString(public.StepName, 512), ModelName: canonicalCleanString(public.ModelName, 512), Provider: canonicalProductIdentifier(public.Provider), InputTokens: public.InputTokens, OutputTokens: public.OutputTokens, TotalTokens: public.TotalTokens, CostMicros: public.CostMicros, Currency: canonicalProductIdentifier(public.Currency), Estimated: public.Estimated, CreatedAt: createdAt}, nil
}

func projectCanonicalProductTokenUsageAggregate(summary *appagentthread.TokenUsageAggregateSummary) canonicalProductTokenUsageAggregate {
	if summary == nil {
		return canonicalProductTokenUsageAggregate{}
	}
	return canonicalProductTokenUsageAggregate{InputTokens: summary.InputTokens, OutputTokens: summary.OutputTokens, TotalTokens: summary.TotalTokens, CostMicros: summary.CostMicros, CallCount: summary.CallCount, LeadAgentTokens: summary.LeadAgentTokens, SubagentTokens: summary.SubagentTokens, MiddlewareTokens: summary.MiddlewareTokens, ToolTokens: summary.ToolTokens}
}

func projectCanonicalProductRunTokenUsageAggregate(summary *appagentthread.RunTokenUsageAggregateSummary) (*canonicalProductRunTokenUsageAggregate, error) {
	if summary == nil {
		return nil, nil
	}
	runID, err := canonicalProductRequiredID(summary.RunID, "run token usage aggregate")
	if err != nil {
		return nil, err
	}
	return &canonicalProductRunTokenUsageAggregate{RunID: runID, Aggregate: projectCanonicalProductTokenUsageAggregate(summary.Aggregate)}, nil
}

func projectCanonicalProductMemory(summary *appagentthread.MemorySummary) (*canonicalProductMemory, error) {
	if summary == nil {
		return nil, nil
	}
	memoryID, err := canonicalProductRequiredID(summary.MemoryID, "memory")
	if err != nil {
		return nil, err
	}
	threadID, err := canonicalProductRequiredID(summary.ThreadID, "memory thread")
	if err != nil {
		return nil, err
	}
	createdAt, err := canonicalProductRequiredTime(summary.CreatedAt, "memory created_at")
	if err != nil {
		return nil, err
	}
	updatedAt, err := canonicalProductRequiredTime(summary.UpdatedAt, "memory updated_at")
	if err != nil {
		return nil, err
	}
	return &canonicalProductMemory{MemoryID: memoryID, ThreadID: threadID, RunID: canonicalOptionalIDString(summary.RunID), Scope: canonicalProductIdentifier(string(summary.Scope)), Content: canonicalCleanString(summary.Content, canonicalMaxPublicValueRunes), Metadata: canonicalSanitizeMap(canonicalEntityMetadataFromJSON(summary.Metadata, "")), Score: summary.Score, Confidence: summary.Confidence, SourceType: canonicalProductIdentifier(summary.SourceType), SourceID: canonicalProductIdentifier(summary.SourceID), CorrectionOfMemoryID: canonicalOptionalIDString(summary.CorrectionOfMemoryID), CorrectedAt: canonicalTime(summary.CorrectedAt), ExpiresAt: canonicalTime(summary.ExpiresAt), CreatedAt: createdAt, UpdatedAt: updatedAt, DeletedAt: canonicalTime(summary.DeletedAt)}, nil
}

func projectCanonicalProductMemoryAudit(summary *appagentthread.MemoryAuditEventSummary) (*canonicalProductMemoryAudit, error) {
	if summary == nil {
		return nil, nil
	}
	eventID, err := canonicalProductRequiredID(summary.EventID, "memory audit")
	if err != nil {
		return nil, err
	}
	threadID, err := canonicalProductRequiredID(summary.ThreadID, "memory audit thread")
	if err != nil {
		return nil, err
	}
	createdAt, err := canonicalProductRequiredTime(summary.CreatedAt, "memory audit created_at")
	if err != nil {
		return nil, err
	}
	return &canonicalProductMemoryAudit{EventID: eventID, ThreadID: threadID, RunID: canonicalOptionalIDString(summary.RunID), MemoryID: canonicalOptionalIDString(summary.MemoryID), ActorID: canonicalOptionalIDString(summary.ActorID), EventType: canonicalProductIdentifier(summary.EventType), Scope: canonicalProductIdentifier(string(summary.Scope)), SourceType: canonicalProductIdentifier(summary.SourceType), SourceID: canonicalProductIdentifier(summary.SourceID), AffectedCount: summary.AffectedCount, CreatedAt: createdAt}, nil
}

func projectCanonicalProductGuardrailAudit(summary *appagentthread.GuardrailAuditEventSummary) (*canonicalProductGuardrailAudit, error) {
	if summary == nil {
		return nil, nil
	}
	eventID, err := canonicalProductRequiredID(summary.EventID, "guardrail audit")
	if err != nil {
		return nil, err
	}
	threadID, err := canonicalProductRequiredID(summary.ThreadID, "guardrail audit thread")
	if err != nil {
		return nil, err
	}
	createdAt, err := canonicalProductRequiredTime(summary.CreatedAt, "guardrail audit created_at")
	if err != nil {
		return nil, err
	}
	return &canonicalProductGuardrailAudit{EventID: eventID, ThreadID: threadID, RunID: canonicalOptionalIDString(summary.RunID), ActorID: canonicalOptionalIDString(summary.ActorID), EventType: canonicalProductIdentifier(summary.EventType), TargetType: canonicalProductIdentifier(summary.TargetType), TargetID: canonicalProductIdentifier(summary.TargetID), Operation: canonicalProductIdentifier(summary.Operation), Source: canonicalProductIdentifier(summary.Source), Action: canonicalProductIdentifier(summary.Action), FailMode: canonicalProductIdentifier(summary.FailMode), Provider: canonicalProductIdentifier(summary.Provider), ReasonCode: canonicalProductIdentifier(summary.ReasonCode), RuleIDs: canonicalProductRuleIDs(summary.RuleIDs), CreatedAt: createdAt}, nil
}

func projectCanonicalProductMCPRuntimeAudit(summary *appagentthread.MCPRuntimeAuditEventSummary) (*canonicalProductMCPRuntimeAudit, error) {
	if summary == nil {
		return nil, nil
	}
	eventID, err := canonicalProductRequiredID(summary.EventID, "mcp runtime audit")
	if err != nil {
		return nil, err
	}
	threadID, err := canonicalProductRequiredID(summary.ThreadID, "mcp runtime audit thread")
	if err != nil {
		return nil, err
	}
	createdAt, err := canonicalProductRequiredTime(summary.CreatedAt, "mcp runtime audit created_at")
	if err != nil {
		return nil, err
	}
	return &canonicalProductMCPRuntimeAudit{EventID: eventID, ThreadID: threadID, RunID: canonicalOptionalIDString(summary.RunID), ServerID: canonicalOptionalIDString(summary.ServerID), RuntimeToolName: canonicalProductIdentifier(summary.RuntimeToolName), EventType: canonicalProductIdentifier(summary.EventType), ErrorCode: canonicalProductIdentifier(summary.ErrorCode), ElapsedMillis: summary.ElapsedMillis, OutputBytes: summary.OutputBytes, CreatedAt: createdAt}, nil
}

func canonicalProductRequiredID(value int64, resource string) (string, error) {
	projected, ok := canonicalPositiveInt64ID(value)
	if !ok {
		return "", fmt.Errorf("canonical %s projection requires a positive id", resource)
	}
	return projected.(string), nil
}

func canonicalProductRequiredTime(value int64, resource string) (string, error) {
	projected := canonicalTime(value)
	if projected == "" {
		return "", fmt.Errorf("canonical %s projection requires a valid time", resource)
	}
	if _, err := time.Parse(time.RFC3339Nano, projected); err != nil {
		return "", fmt.Errorf("canonical %s projection requires a valid time", resource)
	}
	return projected, nil
}

func canonicalProductIdentifier(value string) string {
	value = canonicalCleanString(value, 128)
	if value == "" || !canonicalIdentifierPattern.MatchString(value) || canonicalSensitiveValuePattern.MatchString(value) {
		return ""
	}
	return value
}

func canonicalProductScanStatus(value string) string {
	switch value {
	case "pending", "processing", "succeeded", "failed":
		return value
	default:
		return "pending"
	}
}

func canonicalProductRuleIDs(raw string) []string {
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return []string{}
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value = canonicalProductIdentifier(value); value != "" {
			result = append(result, value)
		}
	}
	return result
}
