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
	"fmt"

	domainentity "github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	domainservice "github.com/coze-dev/coze-studio/backend/domain/agentthread/service"
)

type ApplicationADKRuntimeFileRegistry struct {
	app *ApplicationService
}

func NewApplicationADKRuntimeFileRegistry(
	app *ApplicationService,
) *ApplicationADKRuntimeFileRegistry {
	return &ApplicationADKRuntimeFileRegistry{app: app}
}

func (r *ApplicationADKRuntimeFileRegistry) RegisterRuntimeFile(
	ctx context.Context,
	req *RegisterRuntimeFileRequest,
) (*RuntimeFileSummary, bool, error) {
	if r == nil || r.app == nil || r.app.RuntimeFileSVC == nil {
		return nil, false, fmt.Errorf(
			"agent runtime file service is not configured",
		)
	}
	if req == nil {
		return nil, false, fmt.Errorf(
			"register runtime file request is required",
		)
	}
	file, created, err := r.app.RuntimeFileSVC.RegisterRuntimeFile(
		ctx,
		&domainservice.RegisterRuntimeFileRequest{
			RunID:            req.RunID,
			FileName:         req.FileName,
			OriginalFileName: req.OriginalFileName,
			FileKind:         domainentity.AgentFileKindWorkspace,
			VirtualPath:      req.VirtualPath,
			ObjectURI:        req.ObjectURI,
			ContentType:      req.ContentType,
			SizeBytes:        req.SizeBytes,
			Digest:           req.Digest,
			Metadata:         req.Metadata,
		},
	)
	if err != nil {
		return nil, false, err
	}
	if file == nil {
		return nil, false, fmt.Errorf(
			"agent runtime file service returned empty file",
		)
	}
	return &RuntimeFileSummary{
		FileID:    file.ID,
		ObjectURI: file.ObjectURI,
	}, created, nil
}

func (r *ApplicationADKRuntimeFileRegistry) ResolveRuntimeFile(
	ctx context.Context,
	req *ResolveRuntimeFileRequest,
) (*RuntimeFileSummary, error) {
	if r == nil || r.app == nil || r.app.RuntimeFileSVC == nil {
		return nil, fmt.Errorf(
			"agent runtime file service is not configured",
		)
	}
	if req == nil {
		return nil, fmt.Errorf(
			"resolve runtime file request is required",
		)
	}
	file, err := r.app.RuntimeFileSVC.ResolveRuntimeFile(
		ctx,
		&domainservice.ResolveRuntimeFileRequest{
			SpaceID:     req.SpaceID,
			ThreadID:    req.ThreadID,
			RunID:       req.RunID,
			VirtualPath: req.VirtualPath,
		},
	)
	if err != nil {
		return nil, err
	}
	if file == nil {
		return nil, fmt.Errorf(
			"agent runtime file service returned empty file",
		)
	}
	return &RuntimeFileSummary{
		FileID:    file.ID,
		ObjectURI: file.ObjectURI,
	}, nil
}
