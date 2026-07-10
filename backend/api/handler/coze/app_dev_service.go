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
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"github.com/coze-dev/coze-studio/backend/api/internal/httputil"
	appdev "github.com/coze-dev/coze-studio/backend/application/appdev"
	"github.com/coze-dev/coze-studio/backend/application/base/ctxutil"
	appworkspace "github.com/coze-dev/coze-studio/backend/application/workspace"
	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
)

var appDevApplicationSVC = appdev.SVC

type createAppDevProjectRequest struct {
	Name   string `json:"name"`
	Prompt string `json:"prompt"`
}

type updateAppDevProjectRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type duplicateAppDevProjectRequest struct {
	Name string `json:"name"`
}

type buildAppDevProjectRequest struct {
	PublishType string `json:"publishType"`
}

type createAppDevSnapshotRequest struct {
	Label string `json:"label"`
}

type saveAppDevFileContentRequest struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	Version string `json:"version"`
}

type deleteAppDevFileRequest struct {
	Path string `json:"path"`
}

type renameAppDevFileRequest struct {
	SourcePath string `json:"sourcePath"`
	TargetPath string `json:"targetPath"`
}

type sendAppDevChatRequest struct {
	Message     string                  `json:"message"`
	ModelID     string                  `json:"modelId"`
	DataSources []appdev.ChatDataSource `json:"dataSources"`
	Attachments []appdev.ChatAttachment `json:"attachments"`
}

func ListAppDevProjects(ctx context.Context, c *app.RequestContext) {
	identity, ok := getAppDevIdentity(ctx, c, false)
	if !ok {
		return
	}

	resp, err := appDevApplicationSVC.ListProjects(ctx, &appdev.ListProjectsRequest{
		SpaceID:       identity.spaceID,
		CurrentUserID: identity.userID,
		Keyword:       c.Query("keyword"),
	})
	if err != nil {
		appDevErrorResponse(c, err)
		return
	}

	c.JSON(consts.StatusOK, resp)
}

func CreateAppDevProject(ctx context.Context, c *app.RequestContext) {
	identity, ok := getAppDevIdentity(ctx, c, false)
	if !ok {
		return
	}

	var req createAppDevProjectRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	resp, err := appDevApplicationSVC.CreateProject(ctx, &appdev.CreateProjectRequest{
		SpaceID:       identity.spaceID,
		CurrentUserID: identity.userID,
		Name:          req.Name,
		Prompt:        req.Prompt,
	})
	if err != nil {
		appDevErrorResponse(c, err)
		return
	}

	c.JSON(consts.StatusOK, resp)
}

func ImportAppDevProject(ctx context.Context, c *app.RequestContext) {
	identity, ok := getAppDevIdentity(ctx, c, false)
	if !ok {
		return
	}

	fileHeader, err := c.FormFile("file")
	if err != nil {
		invalidParamRequestResponse(c, "project archive file is required")
		return
	}

	file, err := fileHeader.Open()
	if err != nil {
		appDevErrorResponse(c, err)
		return
	}
	defer file.Close()

	archive, err := io.ReadAll(io.LimitReader(file, 100*1024*1024+1))
	if err != nil {
		appDevErrorResponse(c, err)
		return
	}

	resp, err := appDevApplicationSVC.ImportProject(ctx, &appdev.ImportProjectRequest{
		SpaceID:       identity.spaceID,
		CurrentUserID: identity.userID,
		Name:          c.PostForm("name"),
		FileName:      fileHeader.Filename,
		Archive:       archive,
	})
	if err != nil {
		appDevErrorResponse(c, err)
		return
	}

	c.JSON(consts.StatusOK, resp)
}

func GetAppDevProject(ctx context.Context, c *app.RequestContext) {
	identity, ok := getAppDevIdentity(ctx, c, true)
	if !ok {
		return
	}

	resp, err := appDevApplicationSVC.GetProject(ctx, &appdev.GetProjectRequest{
		SpaceID:       identity.spaceID,
		CurrentUserID: identity.userID,
		ProjectID:     identity.projectID,
	})
	if err != nil {
		appDevErrorResponse(c, err)
		return
	}

	c.JSON(consts.StatusOK, resp)
}

func UpdateAppDevProject(ctx context.Context, c *app.RequestContext) {
	identity, ok := getAppDevIdentity(ctx, c, true)
	if !ok {
		return
	}

	var req updateAppDevProjectRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	resp, err := appDevApplicationSVC.UpdateProject(ctx, &appdev.UpdateProjectRequest{
		SpaceID:       identity.spaceID,
		CurrentUserID: identity.userID,
		ProjectID:     identity.projectID,
		Name:          req.Name,
		Description:   req.Description,
	})
	if err != nil {
		appDevErrorResponse(c, err)
		return
	}

	c.JSON(consts.StatusOK, resp)
}

func DuplicateAppDevProject(ctx context.Context, c *app.RequestContext) {
	identity, ok := getAppDevIdentity(ctx, c, true)
	if !ok {
		return
	}

	var req duplicateAppDevProjectRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	resp, err := appDevApplicationSVC.DuplicateProject(ctx, &appdev.DuplicateProjectRequest{
		SpaceID:       identity.spaceID,
		CurrentUserID: identity.userID,
		ProjectID:     identity.projectID,
		Name:          req.Name,
	})
	if err != nil {
		appDevErrorResponse(c, err)
		return
	}

	c.JSON(consts.StatusOK, resp)
}

func ArchiveAppDevProject(ctx context.Context, c *app.RequestContext) {
	identity, ok := getAppDevIdentity(ctx, c, true, true)
	if !ok {
		return
	}

	resp, err := appDevApplicationSVC.ArchiveProject(ctx, &appdev.ArchiveProjectRequest{
		SpaceID:       identity.spaceID,
		CurrentUserID: identity.userID,
		ProjectID:     identity.projectID,
	})
	if err != nil {
		appDevErrorResponse(c, err)
		return
	}

	c.JSON(consts.StatusOK, resp)
}

func ExportAppDevProject(ctx context.Context, c *app.RequestContext) {
	identity, ok := getAppDevIdentity(ctx, c, true)
	if !ok {
		return
	}

	resp, err := appDevApplicationSVC.ExportProject(ctx, &appdev.ExportProjectRequest{
		SpaceID:       identity.spaceID,
		CurrentUserID: identity.userID,
		ProjectID:     identity.projectID,
	})
	if err != nil {
		appDevErrorResponse(c, err)
		return
	}

	c.Response.Header.Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", resp.FileName))
	c.Data(consts.StatusOK, resp.ContentType, resp.Content)
}

func BuildAppDevProject(ctx context.Context, c *app.RequestContext) {
	identity, ok := getAppDevIdentity(ctx, c, true, true)
	if !ok {
		return
	}

	var req buildAppDevProjectRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	resp, err := appDevApplicationSVC.BuildProject(ctx, &appdev.BuildProjectRequest{
		SpaceID:       identity.spaceID,
		CurrentUserID: identity.userID,
		ProjectID:     identity.projectID,
		PublishType:   req.PublishType,
	})
	if err != nil {
		appDevErrorResponse(c, err)
		return
	}

	c.JSON(consts.StatusOK, resp)
}

func DownloadAppDevRelease(ctx context.Context, c *app.RequestContext) {
	identity, ok := getAppDevIdentity(ctx, c, true)
	if !ok {
		return
	}

	resp, err := appDevApplicationSVC.ExportBuildArtifact(ctx, &appdev.ExportProjectRequest{
		SpaceID:       identity.spaceID,
		CurrentUserID: identity.userID,
		ProjectID:     identity.projectID,
	})
	if err != nil {
		appDevErrorResponse(c, err)
		return
	}

	c.Response.Header.Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", resp.FileName))
	c.Data(consts.StatusOK, resp.ContentType, resp.Content)
}

func ListAppDevFiles(ctx context.Context, c *app.RequestContext) {
	identity, ok := getAppDevIdentity(ctx, c, true)
	if !ok {
		return
	}

	resp, err := appDevApplicationSVC.ListFiles(ctx, &appdev.ProjectFileRequest{
		SpaceID:       identity.spaceID,
		CurrentUserID: identity.userID,
		ProjectID:     identity.projectID,
	})
	if err != nil {
		appDevErrorResponse(c, err)
		return
	}

	c.JSON(consts.StatusOK, resp)
}

func GetAppDevFileContent(ctx context.Context, c *app.RequestContext) {
	identity, ok := getAppDevIdentity(ctx, c, true)
	if !ok {
		return
	}

	resp, err := appDevApplicationSVC.GetFileContent(ctx, &appdev.FileContentRequest{
		SpaceID:       identity.spaceID,
		CurrentUserID: identity.userID,
		ProjectID:     identity.projectID,
		Path:          c.Query("path"),
	})
	if err != nil {
		appDevErrorResponse(c, err)
		return
	}

	c.JSON(consts.StatusOK, resp)
}

func SaveAppDevFileContent(ctx context.Context, c *app.RequestContext) {
	identity, ok := getAppDevIdentity(ctx, c, true)
	if !ok {
		return
	}

	var req saveAppDevFileContentRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	resp, err := appDevApplicationSVC.SaveFileContent(ctx, &appdev.SaveFileContentRequest{
		SpaceID:       identity.spaceID,
		CurrentUserID: identity.userID,
		ProjectID:     identity.projectID,
		Path:          req.Path,
		Content:       req.Content,
		Version:       req.Version,
	})
	if err != nil {
		appDevErrorResponse(c, err)
		return
	}

	c.JSON(consts.StatusOK, resp)
}

func UploadAppDevFiles(ctx context.Context, c *app.RequestContext) {
	identity, ok := getAppDevIdentity(ctx, c, true)
	if !ok {
		return
	}

	form, err := c.MultipartForm()
	if err != nil {
		invalidParamRequestResponse(c, "multipart form is required")
		return
	}

	fileHeaders := form.File["files"]
	if len(fileHeaders) == 0 {
		fileHeaders = form.File["file"]
	}
	filePaths := form.Value["filePaths"]
	if len(filePaths) == 0 {
		filePaths = form.Value["filePath"]
	}
	if len(fileHeaders) == 0 {
		invalidParamRequestResponse(c, "upload files are required")
		return
	}
	if len(fileHeaders) > 100 {
		invalidParamRequestResponse(c, "upload files cannot exceed 100")
		return
	}
	if len(filePaths) != len(fileHeaders) {
		invalidParamRequestResponse(c, "filePaths count must match files count")
		return
	}

	files := make([]appdev.UploadFileItem, 0, len(fileHeaders))
	var totalBytes int64
	for index, header := range fileHeaders {
		if header == nil {
			continue
		}
		if header.Size > 10*1024*1024 {
			invalidParamRequestResponse(c, fmt.Sprintf("upload file %s cannot exceed 10MB", header.Filename))
			return
		}
		opened, err := header.Open()
		if err != nil {
			appDevErrorResponse(c, err)
			return
		}
		content, readErr := io.ReadAll(io.LimitReader(opened, 10*1024*1024+1))
		closeErr := opened.Close()
		if readErr != nil {
			appDevErrorResponse(c, readErr)
			return
		}
		if closeErr != nil {
			appDevErrorResponse(c, closeErr)
			return
		}
		if len(content) > 10*1024*1024 {
			invalidParamRequestResponse(c, fmt.Sprintf("upload file %s cannot exceed 10MB", header.Filename))
			return
		}
		totalBytes += int64(len(content))
		if totalBytes > 100*1024*1024 {
			invalidParamRequestResponse(c, "upload files cannot exceed 100MB")
			return
		}

		files = append(files, appdev.UploadFileItem{
			Path:    filePaths[index],
			Content: content,
		})
	}

	resp, err := appDevApplicationSVC.UploadFiles(ctx, &appdev.UploadFilesRequest{
		SpaceID:       identity.spaceID,
		CurrentUserID: identity.userID,
		ProjectID:     identity.projectID,
		Files:         files,
	})
	if err != nil {
		appDevErrorResponse(c, err)
		return
	}

	c.JSON(consts.StatusOK, resp)
}

func ListAppDevSnapshots(ctx context.Context, c *app.RequestContext) {
	identity, ok := getAppDevIdentity(ctx, c, true)
	if !ok {
		return
	}

	resp, err := appDevApplicationSVC.ListSnapshots(ctx, &appdev.ProjectFileRequest{
		SpaceID:       identity.spaceID,
		CurrentUserID: identity.userID,
		ProjectID:     identity.projectID,
	})
	if err != nil {
		appDevErrorResponse(c, err)
		return
	}

	c.JSON(consts.StatusOK, resp)
}

func CreateAppDevSnapshot(ctx context.Context, c *app.RequestContext) {
	identity, ok := getAppDevIdentity(ctx, c, true)
	if !ok {
		return
	}

	var req createAppDevSnapshotRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	resp, err := appDevApplicationSVC.CreateSnapshot(ctx, &appdev.CreateSnapshotRequest{
		SpaceID:       identity.spaceID,
		CurrentUserID: identity.userID,
		ProjectID:     identity.projectID,
		Label:         req.Label,
	})
	if err != nil {
		appDevErrorResponse(c, err)
		return
	}

	c.JSON(consts.StatusOK, resp)
}

func RestoreAppDevSnapshot(ctx context.Context, c *app.RequestContext) {
	identity, ok := getAppDevIdentity(ctx, c, true)
	if !ok {
		return
	}

	resp, err := appDevApplicationSVC.RestoreSnapshot(ctx, &appdev.RestoreSnapshotRequest{
		SpaceID:       identity.spaceID,
		CurrentUserID: identity.userID,
		ProjectID:     identity.projectID,
		SnapshotID:    c.Param("snapshot_id"),
	})
	if err != nil {
		appDevErrorResponse(c, err)
		return
	}

	c.JSON(consts.StatusOK, resp)
}

func DeleteAppDevFile(ctx context.Context, c *app.RequestContext) {
	identity, ok := getAppDevIdentity(ctx, c, true)
	if !ok {
		return
	}

	var req deleteAppDevFileRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	resp, err := appDevApplicationSVC.DeletePath(ctx, &appdev.DeletePathRequest{
		SpaceID:       identity.spaceID,
		CurrentUserID: identity.userID,
		ProjectID:     identity.projectID,
		Path:          req.Path,
	})
	if err != nil {
		appDevErrorResponse(c, err)
		return
	}

	c.JSON(consts.StatusOK, resp)
}

func RenameAppDevFile(ctx context.Context, c *app.RequestContext) {
	identity, ok := getAppDevIdentity(ctx, c, true)
	if !ok {
		return
	}

	var req renameAppDevFileRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	resp, err := appDevApplicationSVC.RenamePath(ctx, &appdev.RenamePathRequest{
		SpaceID:       identity.spaceID,
		CurrentUserID: identity.userID,
		ProjectID:     identity.projectID,
		SourcePath:    req.SourcePath,
		TargetPath:    req.TargetPath,
	})
	if err != nil {
		appDevErrorResponse(c, err)
		return
	}

	c.JSON(consts.StatusOK, resp)
}

func StartAppDevRuntime(ctx context.Context, c *app.RequestContext) {
	handleAppDevRuntimeMutation(ctx, c, appDevApplicationSVC.StartRuntime, true)
}

func GetAppDevRuntimeStatus(ctx context.Context, c *app.RequestContext) {
	handleAppDevRuntimeMutation(ctx, c, appDevApplicationSVC.GetRuntimeStatus)
}

func KeepAliveAppDevRuntime(ctx context.Context, c *app.RequestContext) {
	handleAppDevRuntimeMutation(ctx, c, appDevApplicationSVC.KeepAliveRuntime)
}

func RestartAppDevRuntime(ctx context.Context, c *app.RequestContext) {
	handleAppDevRuntimeMutation(ctx, c, appDevApplicationSVC.RestartRuntime, true)
}

func StopAppDevRuntime(ctx context.Context, c *app.RequestContext) {
	handleAppDevRuntimeMutation(ctx, c, appDevApplicationSVC.StopRuntime, true)
}

func ListAppDevRuntimeLogs(ctx context.Context, c *app.RequestContext) {
	identity, ok := getAppDevIdentity(ctx, c, true)
	if !ok {
		return
	}

	resp, err := appDevApplicationSVC.ListRuntimeLogs(ctx, &appdev.RuntimeRequest{
		SpaceID:       identity.spaceID,
		CurrentUserID: identity.userID,
		ProjectID:     identity.projectID,
	})
	if err != nil {
		appDevErrorResponse(c, err)
		return
	}

	c.JSON(consts.StatusOK, resp)
}

func ListAppDevModels(ctx context.Context, c *app.RequestContext) {
	identity, ok := getAppDevIdentity(ctx, c, false)
	if !ok {
		return
	}

	resp, err := appDevApplicationSVC.ListModels(ctx, &appdev.ListModelsRequest{
		SpaceID:       identity.spaceID,
		CurrentUserID: identity.userID,
		Scenario:      c.Query("scenario"),
	})
	if err != nil {
		appDevErrorResponse(c, err)
		return
	}

	c.JSON(consts.StatusOK, resp)
}

func SendAppDevChatMessage(ctx context.Context, c *app.RequestContext) {
	identity, ok := getAppDevIdentity(ctx, c, true)
	if !ok {
		return
	}

	var req sendAppDevChatRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	resp, err := appDevApplicationSVC.SendChatMessage(ctx, &appdev.SendChatRequest{
		SpaceID:       identity.spaceID,
		CurrentUserID: identity.userID,
		ProjectID:     identity.projectID,
		Message:       req.Message,
		ModelID:       req.ModelID,
		DataSources:   req.DataSources,
		Attachments:   req.Attachments,
	})
	if err != nil {
		appDevErrorResponse(c, err)
		return
	}

	c.JSON(consts.StatusOK, resp)
}

func SubscribeAppDevChatEvents(ctx context.Context, c *app.RequestContext) {
	identity, ok := getAppDevIdentity(ctx, c, true)
	if !ok {
		return
	}

	events, cleanup, err := appDevApplicationSVC.SubscribeChatEvents(ctx, &appdev.ChatStreamRequest{
		SpaceID:       identity.spaceID,
		CurrentUserID: identity.userID,
		ProjectID:     identity.projectID,
	})
	if err != nil {
		appDevErrorResponse(c, err)
		return
	}

	c.SetStatusCode(consts.StatusOK)
	c.Response.Header.SetContentType("text/event-stream")
	c.Response.Header.Set("Cache-Control", "no-cache")
	c.Response.Header.Set("Connection", "keep-alive")
	pipeReader, pipeWriter := io.Pipe()
	c.Response.SetBodyStream(pipeReader, -1)

	go func() {
		defer cleanup()
		defer func() {
			_ = pipeWriter.Close()
		}()

		w := bufio.NewWriter(pipeWriter)
		ticker := time.NewTicker(25 * time.Second)
		defer ticker.Stop()

		writeEvent := func(eventName string, data map[string]any) bool {
			payload, err := json.Marshal(data)
			if err != nil {
				return true
			}
			if _, err := w.WriteString("event: " + eventName + "\n"); err != nil {
				return false
			}
			if _, err := w.WriteString("data: " + string(payload) + "\n\n"); err != nil {
				return false
			}
			return w.Flush() == nil
		}

		for {
			select {
			case event, ok := <-events:
				if !ok {
					return
				}
				if !writeEvent(event.Event, event.Data) {
					return
				}
			case <-ticker.C:
				if !writeEvent("heartbeat", map[string]any{
					"ts": time.Now().UTC().Format(time.RFC3339),
				}) {
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()
}

func CancelAppDevChat(ctx context.Context, c *app.RequestContext) {
	handleAppDevChatStatus(ctx, c, appDevApplicationSVC.CancelChat)
}

func ListAppDevChatHistory(ctx context.Context, c *app.RequestContext) {
	identity, ok := getAppDevIdentity(ctx, c, true)
	if !ok {
		return
	}

	resp, err := appDevApplicationSVC.ListChatHistory(ctx, &appdev.ChatStreamRequest{
		SpaceID:       identity.spaceID,
		CurrentUserID: identity.userID,
		ProjectID:     identity.projectID,
	})
	if err != nil {
		appDevErrorResponse(c, err)
		return
	}

	c.JSON(consts.StatusOK, resp)
}

func GetAppDevChatStatus(ctx context.Context, c *app.RequestContext) {
	handleAppDevChatStatus(ctx, c, appDevApplicationSVC.GetChatStatus)
}

func handleAppDevChatStatus(
	ctx context.Context,
	c *app.RequestContext,
	handler func(context.Context, *appdev.ChatStreamRequest) (*appdev.ChatStatusResponse, error),
) {
	identity, ok := getAppDevIdentity(ctx, c, true)
	if !ok {
		return
	}

	resp, err := handler(ctx, &appdev.ChatStreamRequest{
		SpaceID:       identity.spaceID,
		CurrentUserID: identity.userID,
		ProjectID:     identity.projectID,
	})
	if err != nil {
		appDevErrorResponse(c, err)
		return
	}

	c.JSON(consts.StatusOK, resp)
}

func handleAppDevRuntimeMutation(
	ctx context.Context,
	c *app.RequestContext,
	handler func(context.Context, *appdev.RuntimeRequest) (*appdev.RuntimeInfoResponse, error),
	requireManager ...bool,
) {
	identity, ok := getAppDevIdentity(ctx, c, true, requireManager...)
	if !ok {
		return
	}

	resp, err := handler(ctx, &appdev.RuntimeRequest{
		SpaceID:       identity.spaceID,
		CurrentUserID: identity.userID,
		ProjectID:     identity.projectID,
	})
	if err != nil {
		appDevErrorResponse(c, err)
		return
	}

	c.JSON(consts.StatusOK, resp)
}

type appDevErrorPayload struct {
	Code    int64  `json:"code"`
	Message string `json:"message"`
	Msg     string `json:"msg"`
}

func appDevErrorResponse(c *app.RequestContext, err error) {
	statusCode := consts.StatusInternalServerError
	if errors.Is(err, domainappdev.ErrInvalidPath) {
		statusCode = consts.StatusBadRequest
	} else if errors.Is(err, domainappdev.ErrNotFound) {
		statusCode = consts.StatusNotFound
	} else if strings.Contains(err.Error(), "required") ||
		strings.Contains(err.Error(), "empty") ||
		strings.Contains(err.Error(), "already") ||
		strings.Contains(err.Error(), "archived") ||
		strings.Contains(err.Error(), "cannot") ||
		strings.Contains(err.Error(), "invalid") ||
		strings.Contains(err.Error(), "only") ||
		strings.Contains(err.Error(), "not available") ||
		strings.Contains(err.Error(), "unsupported") ||
		strings.Contains(err.Error(), "exceed") {
		statusCode = consts.StatusBadRequest
	}

	message := err.Error()
	if statusCode == consts.StatusInternalServerError {
		message = "AppDev 服务异常，请稍后重试"
	}

	c.JSON(statusCode, appDevErrorPayload{
		Code:    -1,
		Message: message,
		Msg:     message,
	})
}

type appDevIdentity struct {
	spaceID   string
	spaceInt  int64
	projectID string
	userID    int64
}

func getAppDevIdentity(
	ctx context.Context,
	c *app.RequestContext,
	requireProject bool,
	requireManager ...bool,
) (*appDevIdentity, bool) {
	currentUserID := ctxutil.GetUIDFromCtx(ctx)
	if currentUserID == nil {
		httputil.Unauthorized(c, "missing user session")
		return nil, false
	}

	spaceID := strings.TrimSpace(c.Param("space_id"))
	if spaceID == "" {
		invalidParamRequestResponse(c, "space_id is required")
		return nil, false
	}

	spaceInt, err := strconv.ParseInt(spaceID, 10, 64)
	if err != nil || spaceInt <= 0 {
		invalidParamRequestResponse(c, "invalid space_id")
		return nil, false
	}

	projectID := strings.TrimSpace(c.Param("project_id"))
	if requireProject && projectID == "" {
		invalidParamRequestResponse(c, "project_id is required")
		return nil, false
	}

	managerAccess := len(requireManager) > 0 && requireManager[0]
	if err := appworkspace.SVC.CheckWorkspaceAppDevAccess(ctx, &appworkspace.CheckWorkspaceAppDevAccessRequest{
		SpaceID:        spaceInt,
		CurrentUserID:  *currentUserID,
		RequireManager: managerAccess,
	}); err != nil {
		internalServerErrorResponse(ctx, c, err)
		return nil, false
	}

	return &appDevIdentity{
		spaceID:   spaceID,
		spaceInt:  spaceInt,
		projectID: projectID,
		userID:    *currentUserID,
	}, true
}
