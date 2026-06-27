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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/cloudwego/eino/adk/filesystem"
	"github.com/cloudwego/eino/adk/middlewares/reduction"
	"github.com/cloudwego/eino/components/tool"
	toolutils "github.com/cloudwego/eino/components/tool/utils"

	"github.com/coze-dev/coze-studio/backend/infra/storage"
)

const (
	adkOffloadVirtualPathPrefix = "/mnt/user-data/workspace/.coze/tool-results/runs"
	defaultADKMaxOffloadBytes   = 16 * 1024 * 1024
	defaultADKReadBytes         = 64 * 1024
	defaultADKMaxReadBytes      = 256 * 1024
)

var adkOffloadPathPattern = regexp.MustCompile(
	`^/mnt/user-data/workspace/\.coze/tool-results/runs/([1-9][0-9]*)/(trunc|clear)/([0-9a-f]{64})\.txt$`,
)

type ADKOffloadObjectStorage interface {
	PutObject(
		ctx context.Context,
		objectKey string,
		content []byte,
		opts ...storage.PutOptFn,
	) error
	GetObject(ctx context.Context, objectKey string) ([]byte, error)
}

type RegisterRuntimeFileRequest struct {
	RunID            int64
	FileName         string
	OriginalFileName string
	VirtualPath      string
	ObjectURI        string
	ContentType      string
	SizeBytes        int64
	Digest           string
	Metadata         string
}

type ResolveRuntimeFileRequest struct {
	SpaceID     int64
	ThreadID    int64
	RunID       int64
	VirtualPath string
}

type RuntimeFileSummary struct {
	FileID    int64
	ObjectURI string
}

type ADKRuntimeFileRegistry interface {
	RegisterRuntimeFile(
		ctx context.Context,
		req *RegisterRuntimeFileRequest,
	) (*RuntimeFileSummary, bool, error)
	ResolveRuntimeFile(
		ctx context.Context,
		req *ResolveRuntimeFileRequest,
	) (*RuntimeFileSummary, error)
}

type ADKOffloadLimits struct {
	MaxOffloadBytes  int
	DefaultReadBytes int
	MaxReadBytes     int
}

type ADKToolResultReductionConfig struct {
	MaxLengthForTrunc         int
	MaxTokensForClear         int64
	ClearRetentionSuffixLimit int
	ClearAtLeastTokens        int64
	OffloadLimits             ADKOffloadLimits
}

type adkToolResultReductionConfigJSON struct {
	MaxLengthForTrunc         *int   `json:"max_length_for_trunc"`
	MaxTokensForClear         *int64 `json:"max_tokens_for_clear"`
	ClearRetentionSuffixLimit *int   `json:"clear_retention_suffix_limit"`
	ClearAtLeastTokens        *int64 `json:"clear_at_least_tokens"`
	MaxOffloadBytes           *int   `json:"max_offload_bytes"`
	DefaultReadBytes          *int   `json:"default_read_bytes"`
	MaxReadBytes              *int   `json:"max_read_bytes"`
}

type ADKOffloadReadChunk struct {
	Content        string
	OffsetByte     int64
	NextOffsetByte int64
	TotalBytes     int64
}

type parsedADKOffloadPath struct {
	RunID int64
	Phase string
	Name  string
}

type ADKOffloadBackend struct {
	run       *RunSummary
	storage   ADKOffloadObjectStorage
	registry  ADKRuntimeFileRegistry
	eventSink RunEventSink
	limits    ADKOffloadLimits
}

type ADKOffloadBackendFactory interface {
	Build(
		ctx context.Context,
		run *RunSummary,
		limits ADKOffloadLimits,
	) (*ADKOffloadBackend, error)
}

type ADKOffloadBackendFactoryFunc func(
	ctx context.Context,
	run *RunSummary,
	limits ADKOffloadLimits,
) (*ADKOffloadBackend, error)

func (f ADKOffloadBackendFactoryFunc) Build(
	ctx context.Context,
	run *RunSummary,
	limits ADKOffloadLimits,
) (*ADKOffloadBackend, error) {
	if f == nil {
		return nil, fmt.Errorf("runtime offload backend factory is required")
	}
	return f(ctx, run, limits)
}

func NewADKOffloadBackend(
	run *RunSummary,
	objectStorage ADKOffloadObjectStorage,
	registry ADKRuntimeFileRegistry,
	eventSink RunEventSink,
	limits ADKOffloadLimits,
) (*ADKOffloadBackend, error) {
	if run == nil || run.RunID <= 0 || run.ThreadID <= 0 || run.SpaceID <= 0 {
		return nil, fmt.Errorf("runtime offload run scope is invalid")
	}
	if objectStorage == nil {
		return nil, fmt.Errorf("runtime offload object storage is required")
	}
	if registry == nil {
		return nil, fmt.Errorf("runtime offload file registry is required")
	}
	if limits.MaxOffloadBytes == 0 {
		limits.MaxOffloadBytes = defaultADKMaxOffloadBytes
	}
	if limits.DefaultReadBytes == 0 {
		limits.DefaultReadBytes = defaultADKReadBytes
	}
	if limits.MaxReadBytes == 0 {
		limits.MaxReadBytes = defaultADKMaxReadBytes
	}
	if limits.MaxOffloadBytes <= 0 ||
		limits.DefaultReadBytes <= 0 ||
		limits.MaxReadBytes <= 0 ||
		limits.DefaultReadBytes > limits.MaxReadBytes ||
		limits.MaxReadBytes > limits.MaxOffloadBytes {
		return nil, fmt.Errorf("runtime offload limits are invalid")
	}
	return &ADKOffloadBackend{
		run:       run,
		storage:   objectStorage,
		registry:  registry,
		eventSink: eventSink,
		limits:    limits,
	}, nil
}

func adkToolResultReductionConfigFromRun(
	run *RunSummary,
	contextBudget ADKContextBudget,
) (ADKToolResultReductionConfig, error) {
	config := ADKToolResultReductionConfig{
		MaxLengthForTrunc:         50000,
		MaxTokensForClear:         100000,
		ClearRetentionSuffixLimit: 1,
		ClearAtLeastTokens:        1000,
		OffloadLimits: ADKOffloadLimits{
			MaxOffloadBytes:  defaultADKMaxOffloadBytes,
			DefaultReadBytes: defaultADKReadBytes,
			MaxReadBytes:     defaultADKMaxReadBytes,
		},
	}
	if config.MaxTokensForClear >= int64(contextBudget.ContextWindowTokens) {
		config.MaxTokensForClear = int64(contextBudget.ContextWindowTokens - 1)
	}
	if run != nil && strings.TrimSpace(run.Config) != "" {
		var payload struct {
			ToolResultReduction adkToolResultReductionConfigJSON `json:"tool_result_reduction"`
		}
		if err := json.Unmarshal([]byte(run.Config), &payload); err != nil {
			return ADKToolResultReductionConfig{}, fmt.Errorf(
				"decode adk tool result reduction config: %w",
				err,
			)
		}
		value := payload.ToolResultReduction
		if value.MaxLengthForTrunc != nil {
			config.MaxLengthForTrunc = *value.MaxLengthForTrunc
		}
		if value.MaxTokensForClear != nil {
			config.MaxTokensForClear = *value.MaxTokensForClear
		}
		if value.ClearRetentionSuffixLimit != nil {
			config.ClearRetentionSuffixLimit =
				*value.ClearRetentionSuffixLimit
		}
		if value.ClearAtLeastTokens != nil {
			config.ClearAtLeastTokens = *value.ClearAtLeastTokens
		}
		if value.MaxOffloadBytes != nil {
			config.OffloadLimits.MaxOffloadBytes = *value.MaxOffloadBytes
		}
		if value.DefaultReadBytes != nil {
			config.OffloadLimits.DefaultReadBytes = *value.DefaultReadBytes
		}
		if value.MaxReadBytes != nil {
			config.OffloadLimits.MaxReadBytes = *value.MaxReadBytes
		}
	}
	if config.MaxLengthForTrunc <= 0 ||
		config.MaxLengthForTrunc > config.OffloadLimits.MaxOffloadBytes {
		return ADKToolResultReductionConfig{}, fmt.Errorf(
			"tool result truncation length is invalid",
		)
	}
	if config.MaxTokensForClear <= 0 ||
		config.MaxTokensForClear >= int64(contextBudget.ContextWindowTokens) {
		return ADKToolResultReductionConfig{}, fmt.Errorf(
			"tool result clear tokens must be positive and below context window",
		)
	}
	if config.ClearRetentionSuffixLimit <= 0 {
		return ADKToolResultReductionConfig{}, fmt.Errorf(
			"tool result clear retention suffix must be positive",
		)
	}
	if config.ClearAtLeastTokens <= 0 ||
		config.ClearAtLeastTokens >= config.MaxTokensForClear {
		return ADKToolResultReductionConfig{}, fmt.Errorf(
			"tool result clear minimum tokens is invalid",
		)
	}
	if config.OffloadLimits.MaxOffloadBytes <= 0 ||
		config.OffloadLimits.DefaultReadBytes <= 0 ||
		config.OffloadLimits.MaxReadBytes <= 0 ||
		config.OffloadLimits.DefaultReadBytes >
			config.OffloadLimits.MaxReadBytes ||
		config.OffloadLimits.MaxReadBytes >
			config.OffloadLimits.MaxOffloadBytes {
		return ADKToolResultReductionConfig{}, fmt.Errorf(
			"tool result offload limits are invalid",
		)
	}
	return config, nil
}

func adkOffloadVirtualPath(runID int64, phase, callID string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(callID)))
	return fmt.Sprintf(
		"%s/%d/%s/%s.txt",
		adkOffloadVirtualPathPrefix,
		runID,
		phase,
		hex.EncodeToString(digest[:]),
	)
}

func adkReductionOffloadPath(
	run *RunSummary,
	phase string,
	detail *reduction.ToolDetail,
) (string, error) {
	if run == nil || run.RunID <= 0 {
		return "", fmt.Errorf("runtime offload run is invalid")
	}
	if detail == nil || detail.ToolContext == nil {
		return "", fmt.Errorf("runtime offload tool context is required")
	}
	callID := strings.TrimSpace(detail.ToolContext.CallID)
	if callID == "" {
		return "", fmt.Errorf("runtime offload tool call id is required")
	}
	if phase != "trunc" && phase != "clear" {
		return "", fmt.Errorf("runtime offload phase is invalid")
	}
	return adkOffloadVirtualPath(run.RunID, phase, callID), nil
}

func parseADKOffloadVirtualPath(value string) (parsedADKOffloadPath, error) {
	if value == "" ||
		strings.Contains(value, `\`) ||
		strings.IndexFunc(value, func(r rune) bool {
			return r < 0x20 || r == 0x7f
		}) >= 0 ||
		path.Clean(value) != value {
		return parsedADKOffloadPath{}, fmt.Errorf(
			"runtime offload virtual path is invalid",
		)
	}
	match := adkOffloadPathPattern.FindStringSubmatch(value)
	if len(match) != 4 {
		return parsedADKOffloadPath{}, fmt.Errorf(
			"runtime offload virtual path is invalid",
		)
	}
	runID, err := strconv.ParseInt(match[1], 10, 64)
	if err != nil || runID <= 0 {
		return parsedADKOffloadPath{}, fmt.Errorf(
			"runtime offload virtual path run id is invalid",
		)
	}
	return parsedADKOffloadPath{
		RunID: runID,
		Phase: match[2],
		Name:  match[3],
	}, nil
}

func lastADKOffloadPathSegment(value string) string {
	return path.Base(value)
}

type adkReadOffloadInput struct {
	FilePath   string `json:"file_path" jsonschema:"description=Virtual path returned by a tool-result offload notice"`
	OffsetByte int64  `json:"offset_byte,omitempty" jsonschema:"description=Zero-based byte offset for bounded pagination"`
	LimitBytes int    `json:"limit_bytes,omitempty" jsonschema:"description=Maximum bytes to return for this page"`
}

func newADKReadOffloadTool(
	backend *ADKOffloadBackend,
) (tool.InvokableTool, error) {
	if backend == nil {
		return nil, fmt.Errorf("runtime offload backend is required")
	}
	return toolutils.InferTool(
		"read_file",
		"Read a bounded byte range from a tool-result virtual path. Use next_offset_byte to continue reading large results.",
		func(
			ctx context.Context,
			input adkReadOffloadInput,
		) (string, error) {
			chunk, err := backend.ReadRange(
				ctx,
				input.FilePath,
				input.OffsetByte,
				input.LimitBytes,
			)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf(
				"path: %s\noffset_byte: %d\nnext_offset_byte: %d\ntotal_bytes: %d\ncontent:\n%s",
				input.FilePath,
				chunk.OffsetByte,
				chunk.NextOffsetByte,
				chunk.TotalBytes,
				chunk.Content,
			), nil
		},
	)
}

func (b *ADKOffloadBackend) Write(
	ctx context.Context,
	req *filesystem.WriteRequest,
) error {
	if b == nil || b.run == nil {
		return fmt.Errorf("runtime offload backend is invalid")
	}
	if req == nil {
		return fmt.Errorf("runtime offload write request is required")
	}
	parsed, err := parseADKOffloadVirtualPath(req.FilePath)
	if err != nil {
		return err
	}
	if parsed.RunID != b.run.RunID {
		return fmt.Errorf("runtime offload write run does not match active run")
	}
	size := len(req.Content)
	if size == 0 || size > b.limits.MaxOffloadBytes {
		return fmt.Errorf("runtime offload content size is invalid")
	}

	objectKey := b.objectKey(parsed)
	content := []byte(req.Content)
	if err := b.storage.PutObject(
		ctx,
		objectKey,
		content,
		storage.WithContentType("text/plain; charset=utf-8"),
		storage.WithObjectSize(int64(size)),
	); err != nil {
		return fmt.Errorf("runtime offload storage write failed")
	}

	digestBytes := sha256.Sum256(content)
	digest := hex.EncodeToString(digestBytes[:])
	metadata := fmt.Sprintf(
		`{"phase":%q,"purpose":"tool_result_offload"}`,
		parsed.Phase,
	)
	file, created, err := b.registry.RegisterRuntimeFile(
		ctx,
		&RegisterRuntimeFileRequest{
			RunID:       parsed.RunID,
			FileName:    lastADKOffloadPathSegment(req.FilePath),
			VirtualPath: req.FilePath,
			ObjectURI:   objectKey,
			ContentType: "text/plain; charset=utf-8",
			SizeBytes:   int64(size),
			Digest:      digest,
			Metadata:    metadata,
		},
	)
	if err != nil {
		return fmt.Errorf("runtime offload file registration failed")
	}

	fileID := int64(0)
	if file != nil {
		fileID = file.FileID
	}
	emitRunEvent(ctx, b.eventSink, RunEvent{
		ThreadID:  b.run.ThreadID,
		RunID:     b.run.RunID,
		EventType: "context.tool_result_offloaded",
		Payload: encodeRunEventPayload(ctx, map[string]any{
			"file_id":       fileID,
			"source_run_id": parsed.RunID,
			"phase":         parsed.Phase,
			"virtual_path":  req.FilePath,
			"size_bytes":    size,
			"digest":        digest,
			"created":       created,
		}),
	})
	return nil
}

func (b *ADKOffloadBackend) ReadRange(
	ctx context.Context,
	virtualPath string,
	offsetByte int64,
	limitBytes int,
) (*ADKOffloadReadChunk, error) {
	if b == nil || b.run == nil {
		return nil, fmt.Errorf("runtime offload backend is invalid")
	}
	parsed, err := parseADKOffloadVirtualPath(virtualPath)
	if err != nil {
		return nil, err
	}
	if offsetByte < 0 {
		return nil, fmt.Errorf("runtime offload read offset is invalid")
	}
	if limitBytes == 0 {
		limitBytes = b.limits.DefaultReadBytes
	}
	if limitBytes < 0 || limitBytes > b.limits.MaxReadBytes {
		return nil, fmt.Errorf("runtime offload read limit is invalid")
	}
	file, err := b.registry.ResolveRuntimeFile(
		ctx,
		&ResolveRuntimeFileRequest{
			SpaceID:     b.run.SpaceID,
			ThreadID:    b.run.ThreadID,
			RunID:       parsed.RunID,
			VirtualPath: virtualPath,
		},
	)
	if err != nil || file == nil || strings.TrimSpace(file.ObjectURI) == "" {
		return nil, fmt.Errorf("runtime offload file is not registered")
	}
	content, err := b.storage.GetObject(ctx, file.ObjectURI)
	if err != nil {
		return nil, fmt.Errorf("runtime offload storage read failed")
	}
	total := int64(len(content))
	if offsetByte >= total {
		return &ADKOffloadReadChunk{
			OffsetByte: offsetByte,
			TotalBytes: total,
		}, nil
	}

	start := int(offsetByte)
	for start < len(content) && !utf8.RuneStart(content[start]) {
		start++
	}
	end := start + limitBytes
	if end > len(content) {
		end = len(content)
	}
	for end > start && end < len(content) && !utf8.RuneStart(content[end]) {
		end--
	}
	if end == start && start < len(content) {
		_, width := utf8.DecodeRune(content[start:])
		end = start + width
	}
	next := int64(0)
	if end < len(content) {
		next = int64(end)
	}
	return &ADKOffloadReadChunk{
		Content:        string(content[start:end]),
		OffsetByte:     int64(start),
		NextOffsetByte: next,
		TotalBytes:     total,
	}, nil
}

func (b *ADKOffloadBackend) objectKey(parsed parsedADKOffloadPath) string {
	return fmt.Sprintf(
		"agent-runtime/%d/%d/runs/%d/tool-results/%s/%s.txt",
		b.run.SpaceID,
		b.run.ThreadID,
		parsed.RunID,
		parsed.Phase,
		parsed.Name,
	)
}

func (b *ADKOffloadBackend) Read(
	ctx context.Context,
	req *filesystem.ReadRequest,
) (*filesystem.FileContent, error) {
	if req == nil {
		return nil, fmt.Errorf("runtime offload read request is required")
	}
	chunk, err := b.ReadRange(
		ctx,
		req.FilePath,
		0,
		b.limits.DefaultReadBytes,
	)
	if err != nil {
		return nil, err
	}
	return &filesystem.FileContent{Content: chunk.Content}, nil
}

func (b *ADKOffloadBackend) LsInfo(
	context.Context,
	*filesystem.LsInfoRequest,
) ([]filesystem.FileInfo, error) {
	return nil, fmt.Errorf("runtime offload list operation is disabled")
}

func (b *ADKOffloadBackend) GrepRaw(
	context.Context,
	*filesystem.GrepRequest,
) ([]filesystem.GrepMatch, error) {
	return nil, fmt.Errorf("runtime offload grep operation is disabled")
}

func (b *ADKOffloadBackend) GlobInfo(
	context.Context,
	*filesystem.GlobInfoRequest,
) ([]filesystem.FileInfo, error) {
	return nil, fmt.Errorf("runtime offload glob operation is disabled")
}

func (b *ADKOffloadBackend) Edit(
	context.Context,
	*filesystem.EditRequest,
) error {
	return fmt.Errorf("runtime offload edit operation is disabled")
}
