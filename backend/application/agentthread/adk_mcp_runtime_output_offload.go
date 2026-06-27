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
	"strings"

	"github.com/cloudwego/eino/adk/filesystem"
)

const adkMCPRuntimeOutputOffloadSchema = "coze.mcp_runtime_output_offload.v1"

type ADKMCPRuntimeOutputOffloadBackendAdapterOptions struct {
	BackendFactory ADKOffloadBackendFactory
	Limits         ADKOffloadLimits
}

type ADKMCPRuntimeOutputOffloadBackendAdapter struct {
	backendFactory ADKOffloadBackendFactory
	limits         ADKOffloadLimits
}

type adkMCPRuntimeOutputOffloadNotice struct {
	Schema      string `json:"schema"`
	Offloaded   bool   `json:"offloaded"`
	ToolName    string `json:"tool_name"`
	ServerID    int64  `json:"server_id"`
	OutputBytes int    `json:"output_bytes"`
	VirtualPath string `json:"virtual_path"`
	ReadTool    string `json:"read_tool"`
}

func NewADKMCPRuntimeOutputOffloadBackendAdapter(
	options ADKMCPRuntimeOutputOffloadBackendAdapterOptions,
) *ADKMCPRuntimeOutputOffloadBackendAdapter {
	return &ADKMCPRuntimeOutputOffloadBackendAdapter{
		backendFactory: options.BackendFactory,
		limits:         options.Limits,
	}
}

func (a *ADKMCPRuntimeOutputOffloadBackendAdapter) OffloadADKMCPRuntimeOutput(
	ctx context.Context,
	request ADKMCPRuntimeOutputOffloadRequest,
) (ADKMCPRuntimeOutputOffloadResult, error) {
	if !validADKMCPRuntimeOutputOffloadRequest(request) {
		return ADKMCPRuntimeOutputOffloadResult{}, fmt.Errorf(
			"mcp runtime output offload request is invalid",
		)
	}
	if a == nil || a.backendFactory == nil {
		return ADKMCPRuntimeOutputOffloadResult{}, fmt.Errorf(
			"mcp runtime output offload backend is not configured",
		)
	}
	backend, err := a.backendFactory.Build(ctx, request.Run, a.limits)
	if err != nil || backend == nil {
		return ADKMCPRuntimeOutputOffloadResult{}, fmt.Errorf(
			"mcp runtime output offload backend failed",
		)
	}

	outputBytes := len([]byte(request.Content))
	virtualPath := adkOffloadVirtualPath(
		request.Run.RunID,
		"trunc",
		adkMCPRuntimeOutputOffloadCallID(request),
	)
	if err := backend.Write(
		ctx,
		&filesystem.WriteRequest{
			FilePath: virtualPath,
			Content:  request.Content,
		},
	); err != nil {
		return ADKMCPRuntimeOutputOffloadResult{}, fmt.Errorf(
			"mcp runtime output offload write failed",
		)
	}

	notice, err := encodeADKMCPRuntimeOutputOffloadNotice(
		adkMCPRuntimeOutputOffloadNotice{
			Schema:      adkMCPRuntimeOutputOffloadSchema,
			Offloaded:   true,
			ToolName:    adkMCPRuntimeSafeName(request.Name),
			ServerID:    request.ServerID,
			OutputBytes: outputBytes,
			VirtualPath: virtualPath,
			ReadTool:    "read_file",
		},
	)
	if err != nil {
		return ADKMCPRuntimeOutputOffloadResult{}, fmt.Errorf(
			"mcp runtime output offload notice failed",
		)
	}

	return ADKMCPRuntimeOutputOffloadResult{
		Notice:      notice,
		VirtualPath: virtualPath,
		OutputBytes: outputBytes,
	}, nil
}

func validADKMCPRuntimeOutputOffloadRequest(
	request ADKMCPRuntimeOutputOffloadRequest,
) bool {
	return request.Run != nil &&
		request.Run.RunID > 0 &&
		request.Run.ThreadID > 0 &&
		request.Run.SpaceID > 0 &&
		isADKSubagentToolName(strings.TrimSpace(request.Name)) &&
		request.ServerID > 0 &&
		strings.TrimSpace(request.ToolName) != "" &&
		len([]byte(request.Content)) > 0 &&
		!request.StartedAt.IsZero()
}

func adkMCPRuntimeOutputOffloadCallID(
	request ADKMCPRuntimeOutputOffloadRequest,
) string {
	contentDigest := sha256.Sum256([]byte(request.Content))

	return fmt.Sprintf(
		"mcp:%d:%d:%s:%s:%d:%s",
		request.Run.RunID,
		request.ServerID,
		adkMCPRuntimeSafeName(request.Name),
		strings.TrimSpace(request.ToolName),
		request.StartedAt.UnixNano(),
		hex.EncodeToString(contentDigest[:8]),
	)
}

func encodeADKMCPRuntimeOutputOffloadNotice(
	notice adkMCPRuntimeOutputOffloadNotice,
) (string, error) {
	if notice.Schema != adkMCPRuntimeOutputOffloadSchema ||
		!notice.Offloaded ||
		!isADKSubagentToolName(strings.TrimSpace(notice.ToolName)) ||
		notice.ServerID <= 0 ||
		notice.OutputBytes <= 0 {
		return "", fmt.Errorf("mcp runtime output offload notice is invalid")
	}
	if _, err := parseADKOffloadVirtualPath(notice.VirtualPath); err != nil {
		return "", err
	}
	payload, err := json.Marshal(notice)
	if err != nil {
		return "", err
	}

	return string(payload), nil
}
