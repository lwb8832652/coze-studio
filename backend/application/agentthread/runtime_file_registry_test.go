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
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	domainentity "github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	domainservice "github.com/coze-dev/coze-studio/backend/domain/agentthread/service"
)

func TestApplicationADKRuntimeFileRegistryMapsWorkspaceFile(t *testing.T) {
	domainSVC := &recordingRuntimeFileService{}
	registry := NewApplicationADKRuntimeFileRegistry(
		&ApplicationService{RuntimeFileSVC: domainSVC},
	)
	fileName := strings.Repeat("a", 64) + ".txt"

	result, created, err := registry.RegisterRuntimeFile(
		context.Background(),
		&RegisterRuntimeFileRequest{
			RunID:       20,
			FileName:    fileName,
			VirtualPath: "/mnt/user-data/workspace/.coze/tool-results/runs/20/trunc/" + fileName,
			ObjectURI:   "agent-runtime/30/10/runs/20/tool-results/trunc/" + fileName,
			ContentType: "text/plain; charset=utf-8",
			SizeBytes:   100,
			Digest:      "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			Metadata:    `{"phase":"trunc"}`,
		},
	)

	require.NoError(t, err)
	require.True(t, created)
	require.Equal(t, int64(99), result.FileID)
	require.NotNil(t, domainSVC.req)
	require.Equal(t, domainentity.AgentFileKindWorkspace, domainSVC.req.FileKind)
	require.Equal(t, int64(20), domainSVC.req.RunID)
	require.Equal(t, fileName, domainSVC.req.FileName)
	require.Equal(t, "agent-runtime/30/10/runs/20/tool-results/trunc/"+fileName, result.ObjectURI)
}

func TestApplicationADKRuntimeFileRegistryResolvesWorkspaceFile(t *testing.T) {
	domainSVC := &recordingRuntimeFileService{
		resolved: &domainentity.AgentFile{
			ID:          99,
			ObjectURI:   "agent-runtime/30/10/runs/20/tool-results/trunc/" + strings.Repeat("a", 64) + ".txt",
			VirtualPath: "/mnt/user-data/workspace/.coze/tool-results/runs/20/trunc/" + strings.Repeat("a", 64) + ".txt",
		},
	}
	registry := NewApplicationADKRuntimeFileRegistry(
		&ApplicationService{RuntimeFileSVC: domainSVC},
	)

	result, err := registry.ResolveRuntimeFile(
		context.Background(),
		&ResolveRuntimeFileRequest{
			SpaceID:     30,
			ThreadID:    10,
			RunID:       20,
			VirtualPath: domainSVC.resolved.VirtualPath,
		},
	)

	require.NoError(t, err)
	require.Equal(t, int64(99), result.FileID)
	require.Equal(t, domainSVC.resolved.ObjectURI, result.ObjectURI)
	require.NotNil(t, domainSVC.resolveReq)
	require.Equal(t, int64(30), domainSVC.resolveReq.SpaceID)
	require.Equal(t, int64(10), domainSVC.resolveReq.ThreadID)
}

type recordingRuntimeFileService struct {
	req        *domainservice.RegisterRuntimeFileRequest
	resolveReq *domainservice.ResolveRuntimeFileRequest
	resolved   *domainentity.AgentFile
}

func (s *recordingRuntimeFileService) RegisterRuntimeFile(
	_ context.Context,
	req *domainservice.RegisterRuntimeFileRequest,
) (*domainentity.AgentFile, bool, error) {
	cloned := *req
	s.req = &cloned
	return &domainentity.AgentFile{
		ID:               99,
		RunID:            req.RunID,
		FileName:         req.FileName,
		OriginalFileName: req.OriginalFileName,
		FileKind:         req.FileKind,
		VirtualPath:      req.VirtualPath,
		ObjectURI:        req.ObjectURI,
		ContentType:      req.ContentType,
		SizeBytes:        req.SizeBytes,
		Digest:           req.Digest,
		Status:           domainentity.AgentFileStatusActive,
		Metadata:         req.Metadata,
	}, true, nil
}

func (s *recordingRuntimeFileService) ResolveRuntimeFile(
	_ context.Context,
	req *domainservice.ResolveRuntimeFileRequest,
) (*domainentity.AgentFile, error) {
	cloned := *req
	s.resolveReq = &cloned
	return s.resolved, nil
}
