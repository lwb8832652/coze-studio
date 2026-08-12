// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package plugin

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/getkin/kin-openapi/openapi3"

	pluginAPI "github.com/coze-dev/coze-studio/backend/api/model/plugin_develop"
	common "github.com/coze-dev/coze-studio/backend/api/model/plugin_develop/common"
	"github.com/coze-dev/coze-studio/backend/application/base/ctxutil"
	"github.com/coze-dev/coze-studio/backend/domain/plugin/entity"
	"github.com/coze-dev/coze-studio/backend/domain/plugin/repository"
	"github.com/coze-dev/coze-studio/backend/infra/coderunner"
	"github.com/coze-dev/coze-studio/backend/pkg/sandboxidentity"
)

const (
	defaultPythonEntry = "main.py"
	defaultJSEntry     = "main.js"

	maxCodePluginSchemaDepth  = 16
	maxCodePluginSchemaTokens = 2048
	maxCodePluginValueDepth   = 32
	maxCodePluginValueTokens  = 8192

	defaultPythonCode = `async def main(args):
    params = args.params
    return {"result": params}
`
	defaultJSCode = `async function main({ params }) {
  return { result: params };
}
`
)

func (p *PluginApplicationService) GetCodePluginDraft(ctx context.Context, req *pluginAPI.GetCodePluginDraftRequest) (*pluginAPI.GetCodePluginDraftResponse, error) {
	plugin, err := p.validateCodePluginReadAccess(ctx, req.PluginID, req.SpaceID)
	if err != nil {
		return nil, err
	}
	if p.codeRepo == nil {
		return nil, codePluginUnavailable(fmt.Errorf("code plugin repository is unavailable"))
	}

	draft, exists, err := p.codeRepo.GetDraft(ctx, plugin.ID)
	if err != nil {
		return nil, classifyCodePluginRepositoryError(err)
	}
	if !exists {
		draft = defaultCodeDraft(plugin.ID, plugin.SpaceID, entity.CodeRuntimePython)
	}
	return &pluginAPI.GetCodePluginDraftResponse{
		Data: codeDraftData(draft),
	}, nil
}

func (p *PluginApplicationService) SaveCodePluginDraft(ctx context.Context, req *pluginAPI.SaveCodePluginDraftRequest) (*pluginAPI.SaveCodePluginDraftResponse, error) {
	plugin, err := p.validateCodePluginAccess(ctx, req.PluginID, req.SpaceID)
	if err != nil {
		return nil, err
	}
	if p.codeRepo == nil {
		return nil, codePluginUnavailable(fmt.Errorf("code plugin repository is unavailable"))
	}
	if len(req.Files) != 1 {
		return nil, codePluginInvalid(fmt.Errorf("code plugin requires exactly one source file"))
	}

	inputSchemaJSON := entity.DefaultCodeSchemaJSON
	outputSchemaJSON := entity.DefaultCodeSchemaJSON
	if req.InputSchemaJSON == nil || req.OutputSchemaJSON == nil {
		current, exists, getErr := p.codeRepo.GetDraft(ctx, plugin.ID)
		if getErr != nil {
			return nil, classifyCodePluginRepositoryError(getErr)
		}
		if exists {
			inputSchemaJSON = current.InputSchemaJSON
			outputSchemaJSON = current.OutputSchemaJSON
		}
	}
	if req.InputSchemaJSON != nil {
		inputSchemaJSON = *req.InputSchemaJSON
	}
	if req.OutputSchemaJSON != nil {
		outputSchemaJSON = *req.OutputSchemaJSON
	}
	if _, err = parseCodePluginSchema(ctx, inputSchemaJSON, "input"); err != nil {
		return nil, codePluginValidation(err)
	}
	if _, err = parseCodePluginSchema(ctx, outputSchemaJSON, "output"); err != nil {
		return nil, codePluginValidation(err)
	}

	runtime, err := apiRuntimeToEntity(req.Runtime)
	if err != nil {
		return nil, codePluginInvalid(err)
	}
	files := make([]*entity.CodeFile, 0, len(req.Files))
	for _, file := range req.Files {
		if file == nil {
			return nil, codePluginInvalid(fmt.Errorf("code file is required"))
		}
		files = append(files, &entity.CodeFile{
			Path:    file.Path,
			Content: []byte(file.Content),
		})
	}
	draft := &entity.CodeDraft{
		PluginID:         plugin.ID,
		SpaceID:          plugin.SpaceID,
		Runtime:          runtime,
		EntryFile:        req.EntryFile,
		InputSchemaJSON:  inputSchemaJSON,
		OutputSchemaJSON: outputSchemaJSON,
		Files:            files,
	}
	if _, err = entity.PrepareCodeDraft(draft); err != nil {
		return nil, codePluginInvalid(err)
	}
	saved, err := p.codeRepo.SaveDraftCAS(ctx, draft, req.Revision)
	if err != nil {
		return nil, classifyCodePluginRepositoryError(err)
	}
	return &pluginAPI.SaveCodePluginDraftResponse{
		Data: codeDraftData(saved),
	}, nil
}

func (p *PluginApplicationService) DebugCodePlugin(ctx context.Context, req *pluginAPI.DebugCodePluginRequest) (*pluginAPI.DebugCodePluginResponse, error) {
	plugin, err := p.validateCodePluginAccess(ctx, req.PluginID, req.SpaceID)
	if err != nil {
		return nil, err
	}
	if p.codeRepo == nil {
		return nil, codePluginUnavailable(fmt.Errorf("code plugin repository is unavailable"))
	}

	draft, exists, err := p.codeRepo.GetDraft(ctx, plugin.ID)
	if err != nil {
		return nil, classifyCodePluginRepositoryError(err)
	}
	if !exists {
		return nil, codePluginInvalid(repository.ErrCodeDraftNotFound)
	}
	if draft.Revision != req.Revision {
		return nil, codePluginConflict(repository.ErrCodeDraftConflict)
	}
	if len(draft.Files) != 1 || draft.Files[0].Path != draft.EntryFile {
		return nil, codePluginInvalid(fmt.Errorf("code plugin source is invalid"))
	}

	arguments, err := decodeCodePluginInput(req.ArgumentsInJSON)
	if err != nil {
		return nil, codePluginValidation(err)
	}
	if err := validateCodePluginValue(ctx, draft.InputSchemaJSON, arguments, "input", openapi3.VisitAsRequest()); err != nil {
		return nil, codePluginValidation(err)
	}

	language, err := entityRuntimeToRunner(draft.Runtime)
	if err != nil {
		return nil, err
	}
	runner := p.codeRunner
	if runner == nil {
		return nil, codePluginUnavailable(coderunner.ErrCodeRunnerUnavailable)
	}
	executionCtx, err := codePluginSandboxContext(ctx, plugin)
	if err != nil {
		return nil, err
	}

	startedAt := time.Now()
	runResponse, runErr := runner.Run(executionCtx, &coderunner.RunRequest{
		Purpose:  coderunner.PurposePlugin,
		Code:     string(draft.Files[0].Content),
		Params:   arguments,
		Language: language,
	})
	duration := time.Since(startedAt)
	if runErr != nil {
		if errors.Is(runErr, coderunner.ErrCodeRunnerUnavailable) {
			return nil, codePluginUnavailable(runErr)
		}
		return codeDebugFailure(req.Revision, runErr, duration), nil
	}
	if runResponse == nil {
		return codeDebugFailure(req.Revision, coderunner.ErrCodeRunnerExecutionFailed, duration), nil
	}

	result, err := sonic.MarshalString(runResponse.Result)
	if err != nil {
		return nil, fmt.Errorf("serialize code plugin result: %w", err)
	}
	if len(result) > entity.MaxCodeBundleSize {
		return codeDebugFailure(req.Revision, coderunner.ErrCodeRunnerOutputLimit, duration), nil
	}
	if runResponse.Result != nil {
		output, err := decodeCodePluginOutput(result)
		if err != nil {
			return nil, codePluginValidation(err)
		}
		if err := validateCodePluginValue(ctx, draft.OutputSchemaJSON, output, "output", openapi3.VisitAsResponse()); err != nil {
			return nil, codePluginValidation(err)
		}
	}
	if err := p.codeRepo.MarkDebuggedCAS(ctx, plugin.ID, req.Revision); err != nil {
		return nil, classifyCodePluginRepositoryError(err)
	}

	return &pluginAPI.DebugCodePluginResponse{
		Data: &common.CodePluginDebugData{
			Success:     true,
			Status:      common.CodePluginDebugStatus_Success,
			Result:      result,
			Reason:      "",
			DurationMs:  duration.Milliseconds(),
			OutputBytes: int64(len(result)),
			Revision:    req.Revision,
		},
	}, nil
}

func (p *PluginApplicationService) GetCodePluginVersion(ctx context.Context, req *pluginAPI.GetCodePluginVersionRequest) (*pluginAPI.GetCodePluginVersionResponse, error) {
	plugin, err := p.validateCodePluginReadAccess(ctx, req.PluginID, req.SpaceID)
	if err != nil {
		return nil, err
	}
	if p.codeRepo == nil {
		return nil, codePluginUnavailable(fmt.Errorf("code plugin repository is unavailable"))
	}
	version, exists, err := p.codeRepo.GetVersion(ctx, plugin.ID, req.Version)
	if err != nil {
		return nil, classifyCodePluginRepositoryError(err)
	}
	if !exists {
		return nil, codePluginInvalid(repository.ErrCodeDraftNotFound)
	}
	return &pluginAPI.GetCodePluginVersionResponse{
		Data: codeVersionData(version),
	}, nil
}

func (p *PluginApplicationService) validateCodePluginAccess(ctx context.Context, pluginID, spaceID int64) (*entity.PluginInfo, error) {
	plugin, err := p.validateDraftPluginAccess(ctx, pluginID)
	if err != nil {
		return nil, err
	}
	if plugin.SpaceID != spaceID {
		return nil, codePluginInvalid(fmt.Errorf("plugin does not belong to the requested space"))
	}
	if plugin.PluginType != common.PluginType_FUNC {
		return nil, codePluginInvalid(fmt.Errorf("plugin is not an executable code plugin"))
	}
	return plugin, nil
}

func (p *PluginApplicationService) validateCodePluginReadAccess(ctx context.Context, pluginID, spaceID int64) (*entity.PluginInfo, error) {
	plugin, err := p.validateDraftPluginReadAccess(ctx, pluginID)
	if err != nil {
		return nil, err
	}
	if plugin.SpaceID != spaceID {
		return nil, codePluginPermission(fmt.Errorf("plugin does not belong to the requested space"))
	}
	if plugin.PluginType != common.PluginType_FUNC {
		return nil, codePluginInvalid(fmt.Errorf("plugin is not an executable code plugin"))
	}
	return plugin, nil
}

func defaultCodeDraft(pluginID, spaceID int64, runtime entity.CodeRuntime) *entity.CodeDraft {
	entryFile := defaultPythonEntry
	code := defaultPythonCode
	if runtime == entity.CodeRuntimeJavaScript {
		entryFile = defaultJSEntry
		code = defaultJSCode
	}
	return &entity.CodeDraft{
		PluginID:         pluginID,
		SpaceID:          spaceID,
		Runtime:          runtime,
		EntryFile:        entryFile,
		InputSchemaJSON:  entity.DefaultCodeSchemaJSON,
		OutputSchemaJSON: entity.DefaultCodeSchemaJSON,
		Files: []*entity.CodeFile{
			{Path: entryFile, Content: []byte(code), Size: int64(len(code))},
		},
	}
}

func registrationCodeRuntime(value *string) (entity.CodeRuntime, error) {
	if value == nil || *value == "" || *value == "1" || *value == string(entity.CodeRuntimePython) {
		return entity.CodeRuntimePython, nil
	}
	if *value == "2" || *value == string(entity.CodeRuntimeJavaScript) {
		return entity.CodeRuntimeJavaScript, nil
	}
	return "", fmt.Errorf("unsupported code plugin runtime %q", *value)
}

func apiRuntimeToEntity(runtime common.CodePluginRuntime) (entity.CodeRuntime, error) {
	switch runtime {
	case common.CodePluginRuntime_Python:
		return entity.CodeRuntimePython, nil
	case common.CodePluginRuntime_JavaScript:
		return entity.CodeRuntimeJavaScript, nil
	default:
		return "", fmt.Errorf("unsupported code plugin runtime %q", runtime.String())
	}
}

func entityRuntimeToAPI(runtime entity.CodeRuntime) common.CodePluginRuntime {
	if runtime == entity.CodeRuntimeJavaScript {
		return common.CodePluginRuntime_JavaScript
	}
	return common.CodePluginRuntime_Python
}

func entityRuntimeToRunner(runtime entity.CodeRuntime) (coderunner.Language, error) {
	switch runtime {
	case entity.CodeRuntimePython:
		return coderunner.Python, nil
	case entity.CodeRuntimeJavaScript:
		return coderunner.JavaScript, nil
	default:
		return "", fmt.Errorf("unsupported code plugin runtime %q", runtime)
	}
}

func codeDraftData(draft *entity.CodeDraft) *common.CodePluginDraftData {
	files := make([]*common.CodePluginFile, 0, len(draft.Files))
	for _, file := range draft.Files {
		if file == nil {
			continue
		}
		checksum := file.SHA256
		files = append(files, &common.CodePluginFile{
			Path:    file.Path,
			Content: string(file.Content),
			Sha256:  &checksum,
		})
	}
	return &common.CodePluginDraftData{
		PluginID:             draft.PluginID,
		SpaceID:              draft.SpaceID,
		Runtime:              entityRuntimeToAPI(draft.Runtime),
		EntryFile:            draft.EntryFile,
		Files:                files,
		Revision:             draft.Revision,
		LastDebuggedRevision: draft.LastDebuggedRevision,
		DebugReady:           draft.Revision > 0 && draft.LastDebuggedRevision == draft.Revision,
		InputSchemaJSON:      draft.InputSchemaJSON,
		OutputSchemaJSON:     draft.OutputSchemaJSON,
	}
}

func codeVersionData(version *entity.CodeVersion) *common.CodePluginVersionData {
	files := make([]*common.CodePluginFile, 0, len(version.Files))
	for _, file := range version.Files {
		if file == nil {
			continue
		}
		checksum := file.SHA256
		files = append(files, &common.CodePluginFile{
			Path:    file.Path,
			Content: string(file.Content),
			Sha256:  &checksum,
		})
	}
	return &common.CodePluginVersionData{
		PluginID:         version.PluginID,
		SpaceID:          version.SpaceID,
		Version:          version.Version,
		Runtime:          entityRuntimeToAPI(version.Runtime),
		EntryFile:        version.EntryFile,
		Files:            files,
		SourceRevision:   version.SourceRevision,
		CreatedBy:        version.CreatedBy,
		InputSchemaJSON:  version.InputSchemaJSON,
		OutputSchemaJSON: version.OutputSchemaJSON,
	}
}

type codePluginJSONContainer struct {
	delimiter json.Delim
	expectKey bool
}

type codePluginValidationError struct {
	stage  string
	path   string
	schema bool
}

func (e *codePluginValidationError) Error() string {
	if e.schema {
		return fmt.Sprintf("code plugin %s schema is invalid", e.stage)
	}
	if e.path != "" {
		return fmt.Sprintf("code plugin %s validation failed at %s", e.stage, e.path)
	}
	return fmt.Sprintf("code plugin %s validation failed", e.stage)
}

func decodeCodePluginInput(raw string) (map[string]any, error) {
	data := []byte(raw)
	if err := inspectCodePluginJSON(data, entity.MaxCodeBundleSize, maxCodePluginValueDepth, maxCodePluginValueTokens, false); err != nil {
		return nil, &codePluginValidationError{stage: "input"}
	}
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return nil, &codePluginValidationError{stage: "input"}
	}
	object, ok := value.(map[string]any)
	if !ok || object == nil {
		return nil, &codePluginValidationError{stage: "input"}
	}
	return object, nil
}

func decodeCodePluginOutput(raw string) (any, error) {
	data := []byte(raw)
	if err := inspectCodePluginJSON(data, entity.MaxCodeBundleSize, maxCodePluginValueDepth, maxCodePluginValueTokens, false); err != nil {
		return nil, &codePluginValidationError{stage: "output"}
	}
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return nil, &codePluginValidationError{stage: "output"}
	}
	return value, nil
}

func validateCodePluginValue(
	ctx context.Context,
	schemaJSON string,
	value any,
	stage string,
	visitMode openapi3.SchemaValidationOption,
) error {
	schema, err := parseCodePluginSchema(ctx, schemaJSON, stage)
	if err != nil {
		return err
	}
	if err := schema.VisitJSON(value, visitMode); err != nil {
		return &codePluginValidationError{
			stage: stage,
			path:  safeCodePluginJSONPointer(err),
		}
	}
	return nil
}

func parseCodePluginSchema(ctx context.Context, schemaJSON, stage string) (*openapi3.Schema, error) {
	if schemaJSON == "" {
		schemaJSON = entity.DefaultCodeSchemaJSON
	}
	data := []byte(schemaJSON)
	if err := inspectCodePluginJSON(data, entity.MaxCodeSchemaSize, maxCodePluginSchemaDepth, maxCodePluginSchemaTokens, true); err != nil {
		return nil, &codePluginValidationError{stage: stage, schema: true}
	}
	var schema openapi3.Schema
	if err := json.Unmarshal(data, &schema); err != nil {
		return nil, &codePluginValidationError{stage: stage, schema: true}
	}
	if err := schema.Validate(ctx); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, &codePluginValidationError{stage: stage, schema: true}
	}
	return &schema, nil
}

func inspectCodePluginJSON(data []byte, maxBytes, maxDepth, maxTokens int, rejectRefs bool) error {
	if len(data) == 0 || len(data) > maxBytes {
		return errors.New("JSON size is outside the allowed range")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	stack := make([]codePluginJSONContainer, 0, maxDepth)
	rootSeen := false
	tokenCount := 0

	consumeValue := func() error {
		if len(stack) == 0 {
			if rootSeen {
				return errors.New("multiple JSON values are not allowed")
			}
			rootSeen = true
			return nil
		}
		parent := &stack[len(stack)-1]
		if parent.delimiter == '{' {
			if parent.expectKey {
				return errors.New("JSON object key is required")
			}
			parent.expectKey = true
		}
		return nil
	}

	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		tokenCount++
		if tokenCount > maxTokens {
			return errors.New("JSON token limit exceeded")
		}

		if delimiter, ok := token.(json.Delim); ok {
			switch delimiter {
			case '{', '[':
				if err := consumeValue(); err != nil {
					return err
				}
				if len(stack)+1 > maxDepth {
					return errors.New("JSON depth limit exceeded")
				}
				stack = append(stack, codePluginJSONContainer{
					delimiter: delimiter,
					expectKey: delimiter == '{',
				})
			case '}', ']':
				if len(stack) == 0 {
					return errors.New("unexpected JSON container close")
				}
				current := stack[len(stack)-1]
				expected := json.Delim('}')
				if current.delimiter == '[' {
					expected = ']'
				}
				if delimiter != expected || (current.delimiter == '{' && !current.expectKey) {
					return errors.New("invalid JSON container")
				}
				stack = stack[:len(stack)-1]
			default:
				return errors.New("invalid JSON delimiter")
			}
			continue
		}

		if len(stack) > 0 {
			current := &stack[len(stack)-1]
			if current.delimiter == '{' && current.expectKey {
				key, ok := token.(string)
				if !ok {
					return errors.New("JSON object keys must be strings")
				}
				if rejectRefs && key == "$ref" {
					return errors.New("JSON references are not allowed")
				}
				current.expectKey = false
				continue
			}
		}
		if err := consumeValue(); err != nil {
			return err
		}
	}
	if !rootSeen || len(stack) != 0 {
		return errors.New("incomplete JSON value")
	}
	return nil
}

func safeCodePluginJSONPointer(err error) string {
	var schemaError *openapi3.SchemaError
	if !errors.As(err, &schemaError) {
		return ""
	}
	parts := schemaError.JSONPointer()
	if len(parts) == 0 && schemaError.SchemaField == "required" && schemaError.Schema != nil {
		if object, ok := schemaError.Value.(map[string]any); ok {
			for _, required := range schemaError.Schema.Required {
				if _, exists := object[required]; !exists {
					parts = []string{required}
					break
				}
			}
		}
	}
	if len(parts) > 16 {
		parts = parts[:16]
	}
	var pointer strings.Builder
	for _, part := range parts {
		runes := []rune(part)
		if len(runes) > 64 {
			part = string(runes[:64])
		}
		part = strings.ReplaceAll(strings.ReplaceAll(part, "~", "~0"), "/", "~1")
		if pointer.Len()+len(part)+1 > 256 {
			break
		}
		pointer.WriteByte('/')
		pointer.WriteString(part)
	}
	return pointer.String()
}

func codeDebugFailure(revision int64, err error, duration time.Duration) *pluginAPI.DebugCodePluginResponse {
	status := common.CodePluginDebugStatus_RuntimeError
	reason := "code execution failed"
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, coderunner.ErrCodeRunnerCanceled):
		status = common.CodePluginDebugStatus_Canceled
		reason = "code execution canceled"
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, coderunner.ErrCodeRunnerTimeout):
		status = common.CodePluginDebugStatus_Timeout
		reason = "code execution timed out"
	case errors.Is(err, coderunner.ErrCodeRunnerCapacityExhausted):
		status = common.CodePluginDebugStatus_Capacity
		reason = "sandbox capacity is exhausted"
	case errors.Is(err, coderunner.ErrCodeRunnerOutputLimit):
		status = common.CodePluginDebugStatus_OutputLimit
		reason = "code output exceeds the allowed limit"
	case errors.Is(err, coderunner.ErrCodeRunnerUnavailable):
		status = common.CodePluginDebugStatus_Unavailable
		reason = "code sandbox is unavailable"
	}
	return &pluginAPI.DebugCodePluginResponse{
		Data: &common.CodePluginDebugData{
			Success:     false,
			Status:      status,
			Result:      "{}",
			Reason:      reason,
			DurationMs:  duration.Milliseconds(),
			OutputBytes: 0,
			Revision:    revision,
		},
	}
}

func codePluginSandboxContext(ctx context.Context, plugin *entity.PluginInfo) (context.Context, error) {
	if ctx == nil || plugin == nil || plugin.SpaceID <= 0 {
		return nil, codePluginUnavailable(fmt.Errorf("code plugin execution identity is unavailable"))
	}
	userID := ctxutil.GetUIDFromCtx(ctx)
	if userID == nil || *userID <= 0 {
		return nil, codePluginPermission(fmt.Errorf("session is required"))
	}
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return nil, codePluginUnavailable(fmt.Errorf("create code plugin execution identity: %w", err))
	}
	return sandboxidentity.WithRequest(ctx, sandboxidentity.Request{
		Scope:       sandboxidentity.ScopePlugin,
		SpaceID:     plugin.SpaceID,
		UserID:      *userID,
		ExecutionID: "plugin-" + hex.EncodeToString(nonce),
	}), nil
}

func codePluginSource(draft *entity.CodeDraft) (string, error) {
	if draft == nil || len(draft.Files) != 1 || draft.Files[0] == nil {
		return "", fmt.Errorf("code plugin requires exactly one source file")
	}
	if strings.TrimSpace(draft.EntryFile) != draft.Files[0].Path {
		return "", fmt.Errorf("code plugin entry file does not match source")
	}
	return string(draft.Files[0].Content), nil
}
