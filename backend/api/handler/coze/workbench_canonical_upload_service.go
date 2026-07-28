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
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"strconv"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	threadproduct "github.com/coze-dev/coze-studio/backend/api/model/workbench/thread_product_contract"
	appagentthread "github.com/coze-dev/coze-studio/backend/application/agentthread"
)

const (
	canonicalThreadUploadMaxFiles      = 10
	canonicalThreadUploadMaxFileBytes  = 50 << 20
	canonicalThreadUploadMaxTotalBytes = 100 << 20
)

// ListCanonicalThreadUploads returns canonical upload metadata for one
// authorized Thread without the legacy code/msg/data envelope.
func ListCanonicalThreadUploads(ctx context.Context, c *app.RequestContext) {
	requestLog := beginCanonicalRequestLog("upload.list", "/api/workbench/threads/:thread_id/uploads")
	requestLog.ResponseBodyKind = "values"
	requestLog.ResourceType = "upload"
	defer completeCanonicalRequestLog(ctx, c, requestLog)
	if !requireCanonicalAPI(ctx, c) {
		return
	}
	if !requireCanonicalAgentThreadService(ctx, c) {
		return
	}
	threadID, public := canonicalPathID(c, "thread_id")
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	requestLog.ThreadID = threadID
	spaceID, public := canonicalSpaceID(ctx, c)
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}

	ctx = workbenchThreadAccessContext(ctx, threadID, 0)
	if public := authorizeCanonicalUploadThreadAccess(ctx, threadID, spaceID); public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	resp, err := appagentthread.SVC.ListTaskThreadUploadFiles(
		ctx,
		&appagentthread.ListTaskThreadUploadFilesRequest{
			SpaceID:  spaceID,
			UserID:   workbenchViewerIDFromCtx(ctx),
			ThreadID: threadID,
		},
	)
	if err != nil {
		writeCanonicalApplicationError(ctx, c, err)
		return
	}
	uploads, err := canonicalThreadUploadsToAPI(resp.Files)
	if err != nil {
		writeCanonicalApplicationError(ctx, c, fmt.Errorf("project canonical uploads: %w", err))
		return
	}

	c.JSON(consts.StatusOK, &threadproduct.CanonicalUploadListResponse{
		Uploads: uploads,
		Total:   int64(len(uploads)),
		HasMore: false,
	})
}

// UploadCanonicalThreadFiles accepts canonical multipart file uploads under
// either "files" or the SDK-compatible singular "file" field.
func UploadCanonicalThreadFiles(ctx context.Context, c *app.RequestContext) {
	requestLog := beginCanonicalRequestLog("upload.create", "/api/workbench/threads/:thread_id/uploads")
	requestLog.ResponseBodyKind = "values"
	requestLog.ResourceType = "upload"
	requestLog.LifecycleStage = "upload"
	defer completeCanonicalRequestLog(ctx, c, requestLog)
	if !requireCanonicalAPI(ctx, c) {
		return
	}
	if !requireCanonicalAgentThreadService(ctx, c) {
		return
	}
	threadID, public := canonicalPathID(c, "thread_id")
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	requestLog.ThreadID = threadID
	spaceID, public := canonicalSpaceID(ctx, c)
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}

	ctx = workbenchThreadAccessContext(ctx, threadID, 0)
	if public := authorizeCanonicalUploadThreadAccess(ctx, threadID, spaceID); public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	form, err := c.MultipartForm()
	if err != nil {
		writeCanonicalError(ctx, c, consts.StatusBadRequest, *canonicalUploadInvalidRequest("Multipart form is required"))
		return
	}
	fileHeaders := canonicalUploadFileHeaders(form)
	if len(fileHeaders) == 0 {
		writeCanonicalError(ctx, c, consts.StatusBadRequest, *canonicalUploadInvalidRequest("Upload files are required"))
		return
	}
	if len(fileHeaders) > canonicalThreadUploadMaxFiles {
		writeCanonicalError(ctx, c, consts.StatusBadRequest, *canonicalUploadInvalidRequest("Upload file count exceeds limit"))
		return
	}
	if public := validateCanonicalUploadFileHeaderSizes(fileHeaders); public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}

	files, public := readCanonicalUploadFiles(fileHeaders)
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	resp, err := appagentthread.SVC.UploadTaskThreadFiles(
		ctx,
		&appagentthread.UploadTaskThreadFilesRequest{
			SpaceID:  spaceID,
			UserID:   workbenchViewerIDFromCtx(ctx),
			ThreadID: threadID,
			Files:    files,
		},
	)
	if err != nil {
		writeCanonicalApplicationError(ctx, c, err)
		return
	}
	uploads, err := canonicalThreadUploadsToAPI(resp.Files)
	if err != nil {
		writeCanonicalApplicationError(ctx, c, fmt.Errorf("project canonical uploads: %w", err))
		return
	}
	if len(uploads) > 0 {
		requestLog.ResourceID = uploads[0].FileID
	}
	requestLog.LifecycleStage = "uploaded"

	c.JSON(consts.StatusOK, &threadproduct.CanonicalUploadResponse{
		Uploads:      uploads,
		SkippedFiles: resp.SkippedFiles,
	})
}

// DeleteCanonicalThreadUpload deletes one upload by stable file_id. Filename
// deletion remains owned by the legacy task_threads route.
func DeleteCanonicalThreadUpload(ctx context.Context, c *app.RequestContext) {
	requestLog := beginCanonicalRequestLog("upload.delete", "/api/workbench/threads/:thread_id/uploads/:file_id")
	requestLog.ResponseBodyKind = "empty"
	requestLog.ResourceType = "upload"
	defer completeCanonicalRequestLog(ctx, c, requestLog)
	if !requireCanonicalAPI(ctx, c) {
		return
	}
	if !requireCanonicalAgentThreadService(ctx, c) {
		return
	}
	threadID, public := canonicalPathID(c, "thread_id")
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	requestLog.ThreadID = threadID
	fileID, public := canonicalProductPathID(c, "file_id")
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	requestLog.ResourceID = strconv.FormatInt(fileID, 10)
	spaceID, public := canonicalSpaceID(ctx, c)
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}

	ctx = workbenchThreadAccessContext(ctx, threadID, 0)
	if public := authorizeCanonicalUploadThreadAccess(ctx, threadID, spaceID); public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	resp, err := appagentthread.SVC.DeleteTaskThreadUploadFileByID(
		ctx,
		&appagentthread.DeleteTaskThreadUploadFileByIDRequest{
			SpaceID:  spaceID,
			UserID:   workbenchViewerIDFromCtx(ctx),
			ThreadID: threadID,
			FileID:   fileID,
		},
	)
	if err != nil {
		writeCanonicalApplicationError(ctx, c, err)
		return
	}
	if resp == nil || !resp.Deleted {
		public := canonicalUploadNotFound()
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	requestLog.LifecycleStage = "deleted"
	c.Status(consts.StatusNoContent)
}

func authorizeCanonicalUploadThreadAccess(ctx context.Context, threadID, spaceID int64) *canonicalError {
	err := appagentthread.SVC.AuthorizeThreadAccess(ctx, appagentthread.ThreadAccessRequest{
		ViewerID: workbenchViewerIDFromCtx(ctx),
		SpaceID:  spaceID,
		ThreadID: threadID,
	})
	if err != nil {
		public := mapCanonicalApplicationError(err)
		return &public
	}
	return nil
}

func canonicalUploadFileHeaders(form *multipart.Form) []*multipart.FileHeader {
	if form == nil {
		return nil
	}
	fileHeaders := form.File["files"]
	if len(fileHeaders) == 0 {
		fileHeaders = form.File["file"]
	}
	return fileHeaders
}

func validateCanonicalUploadFileHeaderSizes(fileHeaders []*multipart.FileHeader) *canonicalError {
	var totalBytes int64
	for _, header := range fileHeaders {
		if header == nil || header.Size <= 0 || header.Size > canonicalThreadUploadMaxFileBytes {
			return canonicalUploadInvalidRequest("Upload file size is invalid")
		}
		totalBytes += header.Size
		if totalBytes > canonicalThreadUploadMaxTotalBytes {
			return canonicalUploadInvalidRequest("Upload total size exceeds limit")
		}
	}
	return nil
}

func readCanonicalUploadFiles(
	fileHeaders []*multipart.FileHeader,
) ([]appagentthread.TaskThreadUploadFileInput, *canonicalError) {
	files := make([]appagentthread.TaskThreadUploadFileInput, 0, len(fileHeaders))
	var totalBytes int64
	for _, header := range fileHeaders {
		if header == nil {
			continue
		}
		opened, err := header.Open()
		if err != nil {
			return nil, canonicalUploadInvalidRequest("Upload file cannot be opened")
		}
		content, readErr := io.ReadAll(opened)
		closeErr := opened.Close()
		if readErr != nil {
			return nil, canonicalUploadInvalidRequest("Upload file cannot be read")
		}
		if closeErr != nil {
			return nil, canonicalUploadInvalidRequest("Upload file cannot be closed")
		}
		size := int64(len(content))
		if size <= 0 || size > canonicalThreadUploadMaxFileBytes {
			return nil, canonicalUploadInvalidRequest("Upload file size is invalid")
		}
		totalBytes += size
		if totalBytes > canonicalThreadUploadMaxTotalBytes {
			return nil, canonicalUploadInvalidRequest("Upload total size exceeds limit")
		}
		files = append(files, appagentthread.TaskThreadUploadFileInput{
			FileName:    header.Filename,
			Content:     content,
			ContentType: header.Header.Get("Content-Type"),
		})
	}
	if len(files) == 0 {
		return nil, canonicalUploadInvalidRequest("Upload files are required")
	}
	return files, nil
}

func canonicalThreadUploadsToAPI(
	summaries []*appagentthread.TaskThreadUploadedFileSummary,
) ([]*threadproduct.CanonicalUploadFile, error) {
	uploads := make([]*threadproduct.CanonicalUploadFile, 0, len(summaries))
	for _, summary := range summaries {
		upload, err := canonicalThreadUploadToAPI(summary)
		if err != nil {
			return nil, err
		}
		if upload != nil {
			uploads = append(uploads, upload)
		}
	}
	return uploads, nil
}

func canonicalThreadUploadToAPI(
	summary *appagentthread.TaskThreadUploadedFileSummary,
) (*threadproduct.CanonicalUploadFile, error) {
	projected, err := projectCanonicalProductUpload(summary)
	if err != nil {
		return nil, err
	}
	if projected == nil {
		return nil, nil
	}
	return &threadproduct.CanonicalUploadFile{
		FileID:      projected.FileID,
		FileName:    projected.FileName,
		VirtualPath: projected.VirtualPath,
		ContentType: projected.ContentType,
		SizeBytes:   projected.SizeBytes,
		CreatedAt:   projected.CreatedAt,
	}, nil
}

func canonicalUploadInvalidRequest(detail string) *canonicalError {
	return newCanonicalError(
		consts.StatusBadRequest,
		"invalid_request",
		detail,
		"invalid_upload",
		false,
	)
}

func canonicalUploadNotFound() *canonicalError {
	return newCanonicalError(
		consts.StatusNotFound,
		"resource_not_found",
		"Resource not found",
		"resource_not_found",
		false,
	)
}
