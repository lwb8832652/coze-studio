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

package agentthread

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	toolapi "github.com/coze-dev/coze-studio/backend/api/model/workbench/tool"
)

const (
	defaultADKMCPRuntimeExecutorTimeout        = 30 * time.Second
	defaultADKMCPRuntimeExecutorMaxOutputBytes = 64 << 10
)

type ADKMCPRuntimeServerResolver interface {
	ResolveADKMCPRuntimeServer(
		ctx context.Context,
		serverID int64,
	) (*toolapi.MCPToolServer, error)
}

type ADKMCPRuntimeTransportCall struct {
	Run       *RunSummary
	Name      string
	Server    *toolapi.MCPToolServer
	ToolName  string
	Arguments string
}

type ADKMCPRuntimeTransportInvoker interface {
	InvokeADKMCPRuntimeTransport(
		ctx context.Context,
		call ADKMCPRuntimeTransportCall,
	) (string, error)
}

type ADKMCPRuntimeHealthReport struct {
	ServerID  int64
	Transport string
	Success   bool
	ErrorCode string
	LatencyMs int64
}

type ADKMCPRuntimeHealthReporter interface {
	ReportADKMCPRuntimeHealth(
		ctx context.Context,
		report ADKMCPRuntimeHealthReport,
	) error
}

type ADKMCPRuntimeHealthReporterFunc func(
	ctx context.Context,
	report ADKMCPRuntimeHealthReport,
) error

func (f ADKMCPRuntimeHealthReporterFunc) ReportADKMCPRuntimeHealth(
	ctx context.Context,
	report ADKMCPRuntimeHealthReport,
) error {
	if f == nil {
		return nil
	}

	return f(ctx, report)
}

type ADKMCPRuntimeOutputOffloadRequest struct {
	Run       *RunSummary
	Name      string
	ServerID  int64
	ToolName  string
	Content   string
	StartedAt time.Time
}

type ADKMCPRuntimeOutputOffloadResult struct {
	Notice      string
	VirtualPath string
	OutputBytes int
}

type ADKMCPRuntimeOutputOffloader interface {
	OffloadADKMCPRuntimeOutput(
		ctx context.Context,
		request ADKMCPRuntimeOutputOffloadRequest,
	) (ADKMCPRuntimeOutputOffloadResult, error)
}

type ADKMCPRuntimeExecutor struct {
	resolver        ADKMCPRuntimeServerResolver
	transport       ADKMCPRuntimeTransportInvoker
	timeout         time.Duration
	maxOutputBytes  int
	eventSink       RunEventSink
	auditRecorder   ADKMCPRuntimeAuditRecorder
	healthReporter  ADKMCPRuntimeHealthReporter
	outputOffloader ADKMCPRuntimeOutputOffloader
}

type ADKMCPRuntimeExecutorOption func(*ADKMCPRuntimeExecutor)

func WithADKMCPRuntimeExecutorTimeout(
	timeout time.Duration,
) ADKMCPRuntimeExecutorOption {
	return func(executor *ADKMCPRuntimeExecutor) {
		executor.timeout = timeout
	}
}

func WithADKMCPRuntimeExecutorMaxOutputBytes(
	maxOutputBytes int,
) ADKMCPRuntimeExecutorOption {
	return func(executor *ADKMCPRuntimeExecutor) {
		executor.maxOutputBytes = maxOutputBytes
	}
}

func WithADKMCPRuntimeExecutorEventSink(
	eventSink RunEventSink,
) ADKMCPRuntimeExecutorOption {
	return func(executor *ADKMCPRuntimeExecutor) {
		executor.eventSink = eventSink
	}
}

func WithADKMCPRuntimeExecutorAuditRecorder(
	auditRecorder ADKMCPRuntimeAuditRecorder,
) ADKMCPRuntimeExecutorOption {
	return func(executor *ADKMCPRuntimeExecutor) {
		executor.auditRecorder = auditRecorder
	}
}

func WithADKMCPRuntimeExecutorHealthReporter(
	healthReporter ADKMCPRuntimeHealthReporter,
) ADKMCPRuntimeExecutorOption {
	return func(executor *ADKMCPRuntimeExecutor) {
		executor.healthReporter = healthReporter
	}
}

func WithADKMCPRuntimeExecutorOutputOffloader(
	outputOffloader ADKMCPRuntimeOutputOffloader,
) ADKMCPRuntimeExecutorOption {
	return func(executor *ADKMCPRuntimeExecutor) {
		executor.outputOffloader = outputOffloader
	}
}

func NewADKMCPRuntimeExecutor(
	resolver ADKMCPRuntimeServerResolver,
	transport ADKMCPRuntimeTransportInvoker,
	options ...ADKMCPRuntimeExecutorOption,
) *ADKMCPRuntimeExecutor {
	executor := &ADKMCPRuntimeExecutor{
		resolver:       resolver,
		transport:      transport,
		timeout:        defaultADKMCPRuntimeExecutorTimeout,
		maxOutputBytes: defaultADKMCPRuntimeExecutorMaxOutputBytes,
	}
	for _, option := range options {
		if option != nil {
			option(executor)
		}
	}
	if executor.maxOutputBytes <= 0 {
		executor.maxOutputBytes = defaultADKMCPRuntimeExecutorMaxOutputBytes
	}

	return executor
}

func (e *ADKMCPRuntimeExecutor) InvokeADKMCPRuntimeTool(
	ctx context.Context,
	call ADKMCPRuntimeToolCall,
) (string, error) {
	startedAt := time.Now()
	name := adkMCPRuntimeSafeName(call.Name)
	run := call.Run
	transportType := ""
	e.emitLifecycle(ctx, run, "mcp.tool.started", name, call.ServerID, transportType, "", startedAt, 0)
	fail := func(code string, message string) (string, error) {
		e.emitLifecycle(ctx, run, "mcp.tool.failed", name, call.ServerID, transportType, code, startedAt, 0)
		return "", fmt.Errorf("%s: %s", message, name)
	}

	if e == nil || e.resolver == nil {
		return fail("resolver_missing", "mcp runtime server resolver is required")
	}
	if e.transport == nil {
		return fail("transport_missing", "mcp runtime transport is required")
	}
	if run == nil || run.SpaceID <= 0 {
		return fail("run_invalid", "mcp runtime run is required")
	}
	if call.ServerID <= 0 {
		return fail("server_invalid", "mcp runtime server is required")
	}
	toolName := strings.TrimSpace(call.ToolName)
	if toolName == "" {
		return fail("tool_invalid", "mcp runtime tool is required")
	}
	arguments := strings.TrimSpace(call.Arguments)
	if arguments == "" {
		arguments = "{}"
	}
	if !validADKMCPRuntimeArguments(arguments) {
		return fail("arguments_invalid", "mcp runtime arguments must be valid JSON")
	}

	server, err := e.resolver.ResolveADKMCPRuntimeServer(ctx, call.ServerID)
	if err != nil || server == nil {
		return fail("server_unavailable", "mcp runtime server unavailable")
	}
	if server.SpaceID != run.SpaceID {
		return fail("space_mismatch", "mcp runtime space mismatch")
	}
	if !server.Enabled {
		return fail("server_disabled", "mcp runtime server disabled")
	}
	transportType = normalizeADKMCPRuntimeTransportType(
		ADKMCPRuntimeTransportCall{Server: server},
	)
	if !adkMCPRuntimeServerHasTool(server, toolName) {
		return fail("tool_not_configured", "mcp runtime tool is not configured")
	}

	transportCtx := ctx
	cancel := func() {}
	if e.timeout > 0 {
		transportCtx, cancel = context.WithTimeout(ctx, e.timeout)
	}
	defer cancel()

	result, err := e.transport.InvokeADKMCPRuntimeTransport(
		transportCtx,
		ADKMCPRuntimeTransportCall{
			Run:       run,
			Name:      name,
			Server:    server,
			ToolName:  toolName,
			Arguments: arguments,
		},
	)
	if err != nil {
		e.reportHealth(ctx, call.ServerID, transportType, false, "transport_failed", startedAt)
		return fail("transport_failed", "mcp runtime transport failed")
	}
	outputBytes := len([]byte(result))
	if outputBytes > e.maxOutputBytes {
		offloaded, err := e.offloadOutput(
			ctx,
			ADKMCPRuntimeOutputOffloadRequest{
				Run:       run,
				Name:      name,
				ServerID:  call.ServerID,
				ToolName:  toolName,
				Content:   result,
				StartedAt: startedAt,
			},
		)
		if err != nil {
			errorCode := "output_budget_exceeded"
			message := "mcp runtime output exceeds budget"
			if e.outputOffloader != nil {
				errorCode = "output_offload_failed"
				message = "mcp runtime output offload failed"
			}
			e.reportHealth(ctx, call.ServerID, transportType, false, errorCode, startedAt)
			return fail(errorCode, message)
		}
		result = offloaded.Notice
	}
	e.reportHealth(ctx, call.ServerID, transportType, true, "", startedAt)

	e.emitLifecycle(
		ctx,
		run,
		"mcp.tool.completed",
		name,
		call.ServerID,
		transportType,
		"",
		startedAt,
		outputBytes,
	)

	return result, nil
}

func (e *ADKMCPRuntimeExecutor) offloadOutput(
	ctx context.Context,
	request ADKMCPRuntimeOutputOffloadRequest,
) (ADKMCPRuntimeOutputOffloadResult, error) {
	if e == nil || e.outputOffloader == nil {
		return ADKMCPRuntimeOutputOffloadResult{}, fmt.Errorf(
			"mcp runtime output offloader is not configured",
		)
	}
	result, err := e.outputOffloader.OffloadADKMCPRuntimeOutput(ctx, request)
	if err != nil {
		return ADKMCPRuntimeOutputOffloadResult{}, err
	}
	if strings.TrimSpace(result.Notice) == "" {
		return ADKMCPRuntimeOutputOffloadResult{}, fmt.Errorf(
			"mcp runtime output offload notice is empty",
		)
	}
	if len([]byte(result.Notice)) > e.maxOutputBytes {
		return ADKMCPRuntimeOutputOffloadResult{}, fmt.Errorf(
			"mcp runtime output offload notice exceeds budget",
		)
	}

	return result, nil
}

func (e *ADKMCPRuntimeExecutor) emitLifecycle(
	ctx context.Context,
	run *RunSummary,
	eventType string,
	name string,
	serverID int64,
	transport string,
	errorCode string,
	startedAt time.Time,
	outputBytes int,
) {
	if e == nil || run == nil {
		return
	}
	e.recordAudit(
		ctx,
		run,
		eventType,
		name,
		serverID,
		transport,
		errorCode,
		startedAt,
		outputBytes,
	)
	if e.eventSink == nil {
		return
	}
	payload := map[string]any{
		"schema":     "coze.mcp_runtime_tool.v1",
		"tool_name":  name,
		"server_id":  serverID,
		"elapsed_ms": time.Since(startedAt).Milliseconds(),
	}
	if errorCode != "" {
		payload["error_code"] = errorCode
	}
	if outputBytes > 0 {
		payload["output_bytes"] = outputBytes
	}

	emitRunEvent(ctx, e.eventSink, RunEvent{
		ThreadID:  run.ThreadID,
		RunID:     run.RunID,
		EventType: eventType,
		Payload:   encodeRunEventPayload(ctx, payload),
	})
}

func (e *ADKMCPRuntimeExecutor) reportHealth(
	ctx context.Context,
	serverID int64,
	transport string,
	success bool,
	errorCode string,
	startedAt time.Time,
) {
	if e == nil || e.healthReporter == nil || serverID <= 0 {
		return
	}
	_ = e.healthReporter.ReportADKMCPRuntimeHealth(
		ctx,
		ADKMCPRuntimeHealthReport{
			ServerID:  serverID,
			Transport: transport,
			Success:   success,
			ErrorCode: errorCode,
			LatencyMs: time.Since(startedAt).Milliseconds(),
		},
	)
}

func (e *ADKMCPRuntimeExecutor) recordAudit(
	ctx context.Context,
	run *RunSummary,
	eventType string,
	name string,
	serverID int64,
	transport string,
	errorCode string,
	startedAt time.Time,
	outputBytes int,
) {
	if e == nil || e.auditRecorder == nil || run == nil {
		return
	}
	_ = e.auditRecorder.RecordADKMCPRuntimeAudit(
		ctx,
		ADKMCPRuntimeAuditRecord{
			SpaceID:         run.SpaceID,
			ThreadID:        run.ThreadID,
			RunID:           run.RunID,
			ServerID:        serverID,
			RuntimeToolName: name,
			Transport:       transport,
			EventType:       eventType,
			ErrorCode:       errorCode,
			ElapsedMillis:   time.Since(startedAt).Milliseconds(),
			OutputBytes:     int64(outputBytes),
		},
	)
}

func validADKMCPRuntimeArguments(arguments string) bool {
	var payload map[string]any

	return json.Unmarshal([]byte(arguments), &payload) == nil
}

func adkMCPRuntimeServerHasTool(server *toolapi.MCPToolServer, toolName string) bool {
	if server == nil {
		return false
	}
	toolName = strings.TrimSpace(toolName)
	for _, item := range server.Tools {
		if item != nil && strings.TrimSpace(item.Name) == toolName {
			return true
		}
	}

	return false
}

func adkMCPRuntimeSafeName(name string) string {
	name = strings.TrimSpace(name)
	if isADKSubagentToolName(name) {
		return name
	}

	return "mcp_tool"
}
