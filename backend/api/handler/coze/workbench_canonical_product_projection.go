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
	"strconv"
	"strings"

	appagentthread "github.com/coze-dev/coze-studio/backend/application/agentthread"
)

type canonicalProductUpload struct {
	FileID      string `json:"file_id"`
	FileName    string `json:"file_name"`
	VirtualPath string `json:"virtual_path,omitempty"`
	ContentType string `json:"content_type,omitempty"`
	SizeBytes   int64  `json:"size_bytes"`
	CreatedAt   string `json:"created_at,omitempty"`
}

type canonicalProductArtifact struct {
	ArtifactID   string         `json:"artifact_id"`
	ThreadID     string         `json:"thread_id,omitempty"`
	RunID        string         `json:"run_id,omitempty"`
	FileID       string         `json:"file_id,omitempty"`
	Title        string         `json:"title"`
	ArtifactType string         `json:"artifact_type,omitempty"`
	VirtualPath  string         `json:"virtual_path,omitempty"`
	ContentType  string         `json:"content_type,omitempty"`
	SizeBytes    int64          `json:"size_bytes"`
	PreviewMode  string         `json:"preview_mode,omitempty"`
	Metadata     map[string]any `json:"metadata"`
	CreatedAt    string         `json:"created_at,omitempty"`
	UpdatedAt    string         `json:"updated_at,omitempty"`
	DeletedAt    string         `json:"deleted_at,omitempty"`
}

type canonicalProductArtifactScanJob struct {
	JobID          string `json:"job_id"`
	ThreadID       string `json:"thread_id,omitempty"`
	RunID          string `json:"run_id,omitempty"`
	ArtifactID     string `json:"artifact_id,omitempty"`
	FileID         string `json:"file_id,omitempty"`
	Scanner        string `json:"scanner,omitempty"`
	Status         string `json:"status"`
	WorkerRef      string `json:"worker_ref,omitempty"`
	AttemptCount   int32  `json:"attempt_count"`
	ErrorCode      string `json:"error_code,omitempty"`
	AvailableAt    string `json:"available_at,omitempty"`
	LeaseExpiresAt string `json:"lease_expires_at,omitempty"`
	StartedAt      string `json:"started_at,omitempty"`
	EndedAt        string `json:"ended_at,omitempty"`
	CreatedAt      string `json:"created_at,omitempty"`
	UpdatedAt      string `json:"updated_at,omitempty"`
}

type canonicalProductTokenUsage struct {
	UsageID      string `json:"usage_id"`
	ThreadID     string `json:"thread_id,omitempty"`
	RunID        string `json:"run_id,omitempty"`
	Source       string `json:"source"`
	StepID       string `json:"step_id,omitempty"`
	StepIndex    int32  `json:"step_index,omitempty"`
	StepName     string `json:"step_name,omitempty"`
	ModelName    string `json:"model_name,omitempty"`
	Provider     string `json:"provider,omitempty"`
	InputTokens  int64  `json:"input_tokens"`
	OutputTokens int64  `json:"output_tokens"`
	TotalTokens  int64  `json:"total_tokens"`
	CostMicros   int64  `json:"cost_micros,omitempty"`
	Currency     string `json:"currency,omitempty"`
	Estimated    bool   `json:"estimated"`
	CreatedAt    string `json:"created_at,omitempty"`
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
	ThreadID             string         `json:"thread_id,omitempty"`
	RunID                string         `json:"run_id,omitempty"`
	Scope                string         `json:"scope,omitempty"`
	Content              string         `json:"content"`
	Metadata             map[string]any `json:"metadata"`
	Score                float64        `json:"score,omitempty"`
	Confidence           float64        `json:"confidence,omitempty"`
	SourceType           string         `json:"source_type,omitempty"`
	SourceID             string         `json:"source_id,omitempty"`
	CorrectionOfMemoryID string         `json:"correction_of_memory_id,omitempty"`
	CorrectedAt          string         `json:"corrected_at,omitempty"`
	ExpiresAt            string         `json:"expires_at,omitempty"`
	CreatedAt            string         `json:"created_at,omitempty"`
	UpdatedAt            string         `json:"updated_at,omitempty"`
	DeletedAt            string         `json:"deleted_at,omitempty"`
}

type canonicalProductMemoryAudit struct {
	EventID       string `json:"event_id"`
	ThreadID      string `json:"thread_id,omitempty"`
	RunID         string `json:"run_id,omitempty"`
	MemoryID      string `json:"memory_id,omitempty"`
	EventType     string `json:"event_type,omitempty"`
	Scope         string `json:"scope,omitempty"`
	SourceType    string `json:"source_type,omitempty"`
	SourceID      string `json:"source_id,omitempty"`
	AffectedCount int64  `json:"affected_count"`
	CreatedAt     string `json:"created_at,omitempty"`
}

type canonicalProductGuardrailAudit struct {
	EventID    string   `json:"event_id"`
	ThreadID   string   `json:"thread_id,omitempty"`
	RunID      string   `json:"run_id,omitempty"`
	EventType  string   `json:"event_type,omitempty"`
	TargetType string   `json:"target_type,omitempty"`
	TargetID   string   `json:"target_id,omitempty"`
	Operation  string   `json:"operation,omitempty"`
	Source     string   `json:"source,omitempty"`
	Action     string   `json:"action,omitempty"`
	FailMode   string   `json:"fail_mode,omitempty"`
	Provider   string   `json:"provider,omitempty"`
	ReasonCode string   `json:"reason_code,omitempty"`
	RuleIDs    []string `json:"rule_ids"`
	CreatedAt  string   `json:"created_at,omitempty"`
}

type canonicalProductMCPRuntimeAudit struct {
	EventID         string `json:"event_id"`
	ThreadID        string `json:"thread_id,omitempty"`
	RunID           string `json:"run_id,omitempty"`
	ServerID        string `json:"server_id,omitempty"`
	RuntimeToolName string `json:"runtime_tool_name,omitempty"`
	EventType       string `json:"event_type,omitempty"`
	ErrorCode       string `json:"error_code,omitempty"`
	ElapsedMillis   int64  `json:"elapsed_millis"`
	OutputBytes     int64  `json:"output_bytes"`
	CreatedAt       string `json:"created_at,omitempty"`
}

func projectCanonicalProductUpload(summary *appagentthread.TaskThreadUploadedFileSummary) (*canonicalProductUpload, error) {
	if summary == nil {
		return nil, nil
	}
	fileID, err := canonicalProductRequiredID(summary.FileID, "upload file")
	if err != nil {
		return nil, err
	}
	return &canonicalProductUpload{FileID: fileID, FileName: canonicalCleanString(summary.FileName, 512), VirtualPath: canonicalCleanString(summary.VirtualPath, 4096), ContentType: canonicalCleanString(summary.ContentType, 128), SizeBytes: summary.SizeBytes, CreatedAt: canonicalTime(summary.CreatedAt)}, nil
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
	return &canonicalProductArtifact{ArtifactID: artifactID, ThreadID: canonicalOptionalIDString(public.ThreadID), RunID: canonicalOptionalIDString(public.RunID), FileID: canonicalOptionalIDString(public.FileID), Title: canonicalCleanString(public.Title, 512), ArtifactType: canonicalProductIdentifier(public.ArtifactType), VirtualPath: canonicalCleanString(public.VirtualPath, 4096), ContentType: canonicalCleanString(public.ContentType, 128), SizeBytes: public.SizeBytes, PreviewMode: canonicalProductIdentifier(string(public.PreviewMode)), Metadata: canonicalSanitizeMap(canonicalEntityMetadataFromJSON(public.Metadata, "")), CreatedAt: canonicalTime(public.CreatedAt), UpdatedAt: canonicalTime(public.UpdatedAt), DeletedAt: canonicalTime(public.DeletedAt)}, nil
}

func projectCanonicalProductArtifactScanJob(summary *appagentthread.ArtifactScanJobSummary) (*canonicalProductArtifactScanJob, error) {
	if summary == nil {
		return nil, nil
	}
	jobID, err := canonicalProductRequiredID(summary.JobID, "artifact scan job")
	if err != nil {
		return nil, err
	}
	status := canonicalProductScanStatus(string(summary.Status))
	result := &canonicalProductArtifactScanJob{JobID: jobID, ThreadID: canonicalOptionalIDString(summary.ThreadID), RunID: canonicalOptionalIDString(summary.RunID), ArtifactID: canonicalOptionalIDString(summary.ArtifactID), FileID: canonicalOptionalIDString(summary.FileID), Scanner: canonicalProductIdentifier(summary.Scanner), Status: status, AttemptCount: summary.AttemptCount, AvailableAt: canonicalTime(summary.AvailableAt), LeaseExpiresAt: canonicalTime(summary.LeaseExpiresAt), StartedAt: canonicalTime(summary.StartedAt), EndedAt: canonicalTime(summary.EndedAt), CreatedAt: canonicalTime(summary.CreatedAt), UpdatedAt: canonicalTime(summary.UpdatedAt)}
	if strings.TrimSpace(summary.WorkerID) != "" {
		result.WorkerRef = canonicalLogHash(summary.WorkerID)
	}
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
	return &canonicalProductTokenUsage{UsageID: usageID, ThreadID: canonicalOptionalIDString(public.ThreadID), RunID: canonicalOptionalIDString(public.RunID), Source: canonicalProductIdentifier(string(public.Source)), StepID: canonicalProductIdentifier(public.StepID), StepIndex: public.StepIndex, StepName: canonicalCleanString(public.StepName, 512), ModelName: canonicalCleanString(public.ModelName, 512), Provider: canonicalProductIdentifier(public.Provider), InputTokens: public.InputTokens, OutputTokens: public.OutputTokens, TotalTokens: public.TotalTokens, CostMicros: public.CostMicros, Currency: canonicalProductIdentifier(public.Currency), Estimated: public.Estimated, CreatedAt: canonicalTime(public.CreatedAt)}, nil
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
	return &canonicalProductMemory{MemoryID: memoryID, ThreadID: canonicalOptionalIDString(summary.ThreadID), RunID: canonicalOptionalIDString(summary.RunID), Scope: canonicalProductIdentifier(string(summary.Scope)), Content: canonicalCleanString(summary.Content, canonicalMaxPublicValueRunes), Metadata: canonicalSanitizeMap(canonicalEntityMetadataFromJSON(summary.Metadata, "")), Score: summary.Score, Confidence: summary.Confidence, SourceType: canonicalProductIdentifier(summary.SourceType), SourceID: canonicalProductOptionalDecimalID(summary.SourceID), CorrectionOfMemoryID: canonicalOptionalIDString(summary.CorrectionOfMemoryID), CorrectedAt: canonicalTime(summary.CorrectedAt), ExpiresAt: canonicalTime(summary.ExpiresAt), CreatedAt: canonicalTime(summary.CreatedAt), UpdatedAt: canonicalTime(summary.UpdatedAt), DeletedAt: canonicalTime(summary.DeletedAt)}, nil
}

func projectCanonicalProductMemoryAudit(summary *appagentthread.MemoryAuditEventSummary) (*canonicalProductMemoryAudit, error) {
	if summary == nil {
		return nil, nil
	}
	eventID, err := canonicalProductRequiredID(summary.EventID, "memory audit")
	if err != nil {
		return nil, err
	}
	return &canonicalProductMemoryAudit{EventID: eventID, ThreadID: canonicalOptionalIDString(summary.ThreadID), RunID: canonicalOptionalIDString(summary.RunID), MemoryID: canonicalOptionalIDString(summary.MemoryID), EventType: canonicalProductIdentifier(summary.EventType), Scope: canonicalProductIdentifier(string(summary.Scope)), SourceType: canonicalProductIdentifier(summary.SourceType), SourceID: canonicalProductOptionalDecimalID(summary.SourceID), AffectedCount: summary.AffectedCount, CreatedAt: canonicalTime(summary.CreatedAt)}, nil
}

func projectCanonicalProductGuardrailAudit(summary *appagentthread.GuardrailAuditEventSummary) (*canonicalProductGuardrailAudit, error) {
	if summary == nil {
		return nil, nil
	}
	eventID, err := canonicalProductRequiredID(summary.EventID, "guardrail audit")
	if err != nil {
		return nil, err
	}
	return &canonicalProductGuardrailAudit{EventID: eventID, ThreadID: canonicalOptionalIDString(summary.ThreadID), RunID: canonicalOptionalIDString(summary.RunID), EventType: canonicalProductIdentifier(summary.EventType), TargetType: canonicalProductIdentifier(summary.TargetType), TargetID: canonicalProductOptionalDecimalID(summary.TargetID), Operation: canonicalProductIdentifier(summary.Operation), Source: canonicalProductIdentifier(summary.Source), Action: canonicalProductIdentifier(summary.Action), FailMode: canonicalProductIdentifier(summary.FailMode), Provider: canonicalProductIdentifier(summary.Provider), ReasonCode: canonicalProductIdentifier(summary.ReasonCode), RuleIDs: canonicalProductRuleIDs(summary.RuleIDs), CreatedAt: canonicalTime(summary.CreatedAt)}, nil
}

func projectCanonicalProductMCPRuntimeAudit(summary *appagentthread.MCPRuntimeAuditEventSummary) (*canonicalProductMCPRuntimeAudit, error) {
	if summary == nil {
		return nil, nil
	}
	eventID, err := canonicalProductRequiredID(summary.EventID, "mcp runtime audit")
	if err != nil {
		return nil, err
	}
	return &canonicalProductMCPRuntimeAudit{EventID: eventID, ThreadID: canonicalOptionalIDString(summary.ThreadID), RunID: canonicalOptionalIDString(summary.RunID), ServerID: canonicalOptionalIDString(summary.ServerID), RuntimeToolName: canonicalProductIdentifier(summary.RuntimeToolName), EventType: canonicalProductIdentifier(summary.EventType), ErrorCode: canonicalProductIdentifier(summary.ErrorCode), ElapsedMillis: summary.ElapsedMillis, OutputBytes: summary.OutputBytes, CreatedAt: canonicalTime(summary.CreatedAt)}, nil
}

func canonicalProductRequiredID(value int64, resource string) (string, error) {
	if value <= 0 {
		return "", fmt.Errorf("canonical %s projection requires a positive id", resource)
	}
	return strconv.FormatInt(value, 10), nil
}

func canonicalProductOptionalDecimalID(value string) string {
	value = canonicalCleanString(value, 128)
	if value == "" {
		return ""
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return ""
		}
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return ""
	}
	return canonicalOptionalIDString(parsed)
}

func canonicalProductIdentifier(value string) string {
	value = canonicalCleanString(value, 128)
	if value == "" || canonicalUnsafePublicField(value) || !canonicalIdentifierPattern.MatchString(value) {
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
