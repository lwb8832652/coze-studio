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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"strings"
	"testing"

	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/stretchr/testify/require"

	appagentthread "github.com/coze-dev/coze-studio/backend/application/agentthread"
	"github.com/coze-dev/coze-studio/backend/infra/storage"
	"github.com/coze-dev/coze-studio/backend/pkg/logs"
)

func TestCanonicalThreadUploadGateOffReturns404(t *testing.T) {
	t.Setenv(canonicalAPIEnabledEnv, "")
	installAgentThreadTestService(t)
	h := canonicalUploadTestServer()

	response := performCanonicalUploadRequest(t, h, http.MethodGet, "/api/workbench/threads/1/uploads", nil)

	require.Equal(t, http.StatusNotFound, response.Code)
}

func TestCanonicalThreadUploadRequiresAuthenticationAndWorkspaceAccess(t *testing.T) {
	t.Run("unauthenticated", func(t *testing.T) {
		t.Setenv(canonicalAPIEnabledEnv, "true")
		installAgentThreadTestService(t)
		h := server.Default()
		registerCanonicalUploadRoutes(h)

		response := performCanonicalUploadRequest(t, h, http.MethodGet, "/api/workbench/threads/1/uploads", nil)

		require.Equal(t, http.StatusUnauthorized, response.Code)
		var public canonicalError
		require.NoError(t, json.Unmarshal(response.Result().Body(), &public))
		require.Equal(t, "unauthenticated", public.Code)
	})

	t.Run("wrong workspace", func(t *testing.T) {
		t.Setenv(canonicalAPIEnabledEnv, "true")
		installAgentThreadTestService(t)
		thread := createCanonicalTestThread(t, 1001, "uploads", `{}`)
		h := canonicalUploadTestServer()

		response := performCanonicalUploadRequest(
			t,
			h,
			http.MethodGet,
			fmt.Sprintf("/api/workbench/threads/%d/uploads", thread.ThreadID),
			nil,
			ut.Header{Key: canonicalSpaceIDHeader, Value: "2002"},
		)

		require.Equal(t, http.StatusNotFound, response.Code)
		var public canonicalError
		require.NoError(t, json.Unmarshal(response.Result().Body(), &public))
		require.Equal(t, "resource_not_found", public.Code)
	})

	t.Run("same workspace non owner", func(t *testing.T) {
		t.Setenv(canonicalAPIEnabledEnv, "true")
		installAgentThreadTestService(t)
		thread := createCanonicalTestThreadForUser(t, 1001, 3, "uploads", `{}`)
		h := canonicalUploadTestServer()

		response := performCanonicalUploadRequest(
			t,
			h,
			http.MethodGet,
			fmt.Sprintf("/api/workbench/threads/%d/uploads", thread.ThreadID),
			nil,
			ut.Header{Key: canonicalSpaceIDHeader, Value: "1001"},
		)

		require.Equal(t, http.StatusNotFound, response.Code)
		var public canonicalError
		require.NoError(t, json.Unmarshal(response.Result().Body(), &public))
		require.Equal(t, "resource_not_found", public.Code)
	})
}

func TestCanonicalThreadUploadRejectsMalformedIDsAndFilenamePath(t *testing.T) {
	t.Setenv(canonicalAPIEnabledEnv, "true")
	installAgentThreadTestService(t)
	h := canonicalUploadTestServer()

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{name: "list thread id", method: http.MethodGet, path: "/api/workbench/threads/not-a-number/uploads"},
		{name: "delete file id", method: http.MethodDelete, path: "/api/workbench/threads/1/uploads/not-a-number"},
		{name: "delete zero file id", method: http.MethodDelete, path: "/api/workbench/threads/1/uploads/0"},
		{name: "delete filename path", method: http.MethodDelete, path: "/api/workbench/threads/1/uploads/report.md"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			response := performCanonicalUploadRequest(t, h, test.method, test.path, nil)

			require.Equal(t, http.StatusBadRequest, response.Code, response.Result().Body())
			var public canonicalError
			require.NoError(t, json.Unmarshal(response.Result().Body(), &public))
			require.Equal(t, "invalid_path_parameter", public.Code)
		})
	}
}

func TestCanonicalThreadUploadRejectsEmptyMultipartAndBounds(t *testing.T) {
	t.Setenv(canonicalAPIEnabledEnv, "true")
	installAgentThreadTestService(t)
	thread := createCanonicalTestThread(t, 1001, "uploads", `{}`)
	storage := &canonicalRecordingUploadStorage{}
	appagentthread.SVC.ArtifactObjectStorage = storage
	h := canonicalUploadTestServer()
	path := fmt.Sprintf("/api/workbench/threads/%d/uploads", thread.ThreadID)

	t.Run("empty multipart", func(t *testing.T) {
		body, contentType := canonicalMultipartBody(t, nil)

		response := performCanonicalUploadRequest(
			t,
			h,
			http.MethodPost,
			path,
			body,
			ut.Header{Key: "Content-Type", Value: contentType},
		)

		require.Equal(t, http.StatusBadRequest, response.Code, response.Result().Body())
	})

	t.Run("too many files", func(t *testing.T) {
		files := make([]canonicalMultipartFile, 0, 11)
		for i := 0; i < 11; i++ {
			files = append(files, canonicalMultipartFile{
				Field:   "files",
				Name:    fmt.Sprintf("file-%02d.txt", i),
				Content: "x",
			})
		}
		body, contentType := canonicalMultipartBody(t, files)

		response := performCanonicalUploadRequest(
			t,
			h,
			http.MethodPost,
			path,
			body,
			ut.Header{Key: "Content-Type", Value: contentType},
		)

		require.Equal(t, http.StatusBadRequest, response.Code, response.Result().Body())
		require.Equal(t, 0, storage.putCount)
	})

	t.Run("single file too large", func(t *testing.T) {
		body, contentType := canonicalMultipartBody(t, []canonicalMultipartFile{
			{
				Field:   "file",
				Name:    "large.bin",
				Content: strings.Repeat("x", 50<<20+1),
			},
		})

		response := performCanonicalUploadRequest(
			t,
			h,
			http.MethodPost,
			path,
			body,
			ut.Header{Key: "Content-Type", Value: contentType},
		)

		require.Equal(t, http.StatusBadRequest, response.Code, response.Result().Body())
		require.Equal(t, 0, storage.putCount)
	})
}

func TestCanonicalThreadUploadListUploadAndDeleteByStableID(t *testing.T) {
	t.Setenv(canonicalAPIEnabledEnv, "true")
	installAgentThreadTestService(t)
	thread := createCanonicalTestThread(t, 1001, "uploads", `{}`)
	appagentthread.SVC.ArtifactObjectStorage = &canonicalRecordingUploadStorage{}
	h := canonicalUploadTestServer()
	path := fmt.Sprintf("/api/workbench/threads/%d/uploads", thread.ThreadID)

	emptyList := performCanonicalUploadRequest(t, h, http.MethodGet, path, nil)
	require.Equal(t, http.StatusOK, emptyList.Code, emptyList.Result().Body())
	require.JSONEq(t, `{"uploads":[],"total":0,"has_more":false}`, string(emptyList.Result().Body()))

	body, contentType := canonicalMultipartBody(t, []canonicalMultipartFile{
		{Field: "files", Name: "report.md", ContentType: "text/markdown; charset=utf-8", Content: "# report"},
		{Field: "files", Name: ".", Content: "skip me"},
	})
	upload := performCanonicalUploadRequest(
		t,
		h,
		http.MethodPost,
		path,
		body,
		ut.Header{Key: "Content-Type", Value: contentType},
	)
	require.Equal(t, http.StatusOK, upload.Code, upload.Result().Body())
	require.NotContains(t, string(upload.Result().Body()), `"code"`)

	var uploadPayload struct {
		Uploads []struct {
			FileID      string `json:"file_id"`
			FileName    string `json:"file_name"`
			VirtualPath string `json:"virtual_path"`
			ContentType string `json:"content_type"`
			SizeBytes   int64  `json:"size_bytes"`
			CreatedAt   string `json:"created_at"`
		} `json:"uploads"`
		SkippedFiles []string `json:"skipped_files"`
	}
	require.NoError(t, json.Unmarshal(upload.Result().Body(), &uploadPayload))
	require.Len(t, uploadPayload.Uploads, 1)
	require.Equal(t, "report.md", uploadPayload.Uploads[0].FileName)
	require.Equal(t, "text/markdown; charset=utf-8", uploadPayload.Uploads[0].ContentType)
	require.Equal(t, int64(8), uploadPayload.Uploads[0].SizeBytes)
	require.NotEmpty(t, uploadPayload.Uploads[0].FileID)
	require.NotEmpty(t, uploadPayload.Uploads[0].CreatedAt)
	require.Equal(t, []string{"."}, uploadPayload.SkippedFiles)

	list := performCanonicalUploadRequest(t, h, http.MethodGet, path, nil)
	require.Equal(t, http.StatusOK, list.Code, list.Result().Body())
	var listPayload struct {
		Uploads    []json.RawMessage `json:"uploads"`
		Total      int64             `json:"total"`
		HasMore    bool              `json:"has_more"`
		NextCursor *string           `json:"next_cursor,omitempty"`
	}
	require.NoError(t, json.Unmarshal(list.Result().Body(), &listPayload))
	require.Len(t, listPayload.Uploads, 1)
	require.Equal(t, int64(1), listPayload.Total)
	require.False(t, listPayload.HasMore)
	require.Nil(t, listPayload.NextCursor)

	deletePath := fmt.Sprintf("%s/%s", path, uploadPayload.Uploads[0].FileID)
	deleted := performCanonicalUploadRequest(t, h, http.MethodDelete, deletePath, nil)
	require.Equal(t, http.StatusNoContent, deleted.Code, deleted.Result().Body())
	require.Empty(t, deleted.Result().Body())

	deletedAgain := performCanonicalUploadRequest(t, h, http.MethodDelete, deletePath, nil)
	require.Equal(t, http.StatusNotFound, deletedAgain.Code, deletedAgain.Result().Body())
}

func TestCanonicalThreadUploadAcceptsSingularFileField(t *testing.T) {
	t.Setenv(canonicalAPIEnabledEnv, "true")
	installAgentThreadTestService(t)
	thread := createCanonicalTestThread(t, 1001, "uploads", `{}`)
	appagentthread.SVC.ArtifactObjectStorage = &canonicalRecordingUploadStorage{}
	h := canonicalUploadTestServer()
	path := fmt.Sprintf("/api/workbench/threads/%d/uploads", thread.ThreadID)
	body, contentType := canonicalMultipartBody(t, []canonicalMultipartFile{
		{Field: "file", Name: "single.txt", Content: "single"},
	})

	response := performCanonicalUploadRequest(
		t,
		h,
		http.MethodPost,
		path,
		body,
		ut.Header{Key: "Content-Type", Value: contentType},
	)

	require.Equal(t, http.StatusOK, response.Code, response.Result().Body())
	require.Contains(t, string(response.Result().Body()), `"file_name":"single.txt"`)
}

func TestCanonicalThreadUploadCompletionLogsStaySafe(t *testing.T) {
	t.Setenv(canonicalAPIEnabledEnv, "true")
	installAgentThreadTestService(t)
	thread := createCanonicalTestThread(t, 1001, "uploads", `{}`)
	appagentthread.SVC.ArtifactObjectStorage = &canonicalRecordingUploadStorage{}
	h := canonicalUploadTestServer()

	var output bytes.Buffer
	logs.SetOutput(&output)
	t.Cleanup(func() { logs.SetOutput(os.Stderr) })

	body, contentType := canonicalMultipartBody(t, []canonicalMultipartFile{
		{Field: "files", Name: "safe-log.txt", Content: "signed-url-secret credential-secret provider-body-secret"},
	})
	response := performCanonicalUploadRequest(
		t,
		h,
		http.MethodPost,
		fmt.Sprintf("/api/workbench/threads/%d/uploads", thread.ThreadID),
		body,
		ut.Header{Key: "Content-Type", Value: contentType},
	)

	require.Equal(t, http.StatusOK, response.Code, response.Result().Body())
	actual := output.String()
	require.Contains(t, actual, "event_name=workbench.api.request.completed")
	require.Contains(t, actual, "operation=upload.create")
	require.Contains(t, actual, "resource_type=upload")
	require.Contains(t, actual, "lifecycle_stage=uploaded")
	for _, forbidden := range []string{
		"signed-url-secret", "credential-secret", "provider-body-secret",
		"agent-runtime/", "/mnt/user-data", "safe-log.txt",
	} {
		require.NotContains(t, actual, forbidden)
	}
}

type canonicalMultipartFile struct {
	Field       string
	Name        string
	ContentType string
	Content     string
}

func canonicalUploadTestServer() *server.Hertz {
	h := authenticatedAgentThreadTestServer()
	registerCanonicalUploadRoutes(h)
	return h
}

func registerCanonicalUploadRoutes(h *server.Hertz) {
	h.GET("/api/workbench/threads/:thread_id/uploads", ListCanonicalThreadUploads)
	h.POST("/api/workbench/threads/:thread_id/uploads", UploadCanonicalThreadFiles)
	h.DELETE("/api/workbench/threads/:thread_id/uploads/:file_id", DeleteCanonicalThreadUpload)
}

func performCanonicalUploadRequest(
	t *testing.T,
	h *server.Hertz,
	method string,
	path string,
	body *ut.Body,
	headers ...ut.Header,
) *ut.ResponseRecorder {
	t.Helper()
	requestHeaders := make([]ut.Header, 0, len(headers)+1)
	hasSpaceHeader := false
	for _, header := range headers {
		if strings.EqualFold(header.Key, canonicalSpaceIDHeader) {
			hasSpaceHeader = true
			break
		}
	}
	if !hasSpaceHeader {
		requestHeaders = append(requestHeaders, ut.Header{Key: canonicalSpaceIDHeader, Value: "1001"})
	}
	requestHeaders = append(requestHeaders, headers...)
	return ut.PerformRequest(h.Engine, method, path, body, requestHeaders...)
}

func canonicalMultipartBody(t *testing.T, files []canonicalMultipartFile) (*ut.Body, string) {
	t.Helper()
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	for _, file := range files {
		header := make(textproto.MIMEHeader)
		header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, file.Field, file.Name))
		if file.ContentType != "" {
			header.Set("Content-Type", file.ContentType)
		}
		part, err := writer.CreatePart(header)
		require.NoError(t, err)
		_, err = io.WriteString(part, file.Content)
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())
	return &ut.Body{Body: bytes.NewReader(buf.Bytes()), Len: buf.Len()}, writer.FormDataContentType()
}

type canonicalRecordingUploadStorage struct {
	putCount int
	key      string
	objects  map[string][]byte
}

func (s *canonicalRecordingUploadStorage) PutObject(
	_ context.Context,
	objectKey string,
	content []byte,
	_ ...storage.PutOptFn,
) error {
	s.putCount++
	s.key = objectKey
	if s.objects == nil {
		s.objects = map[string][]byte{}
	}
	s.objects[objectKey] = append([]byte(nil), content...)
	return nil
}

func (s *canonicalRecordingUploadStorage) GetObject(
	_ context.Context,
	objectKey string,
) ([]byte, error) {
	if s.objects == nil {
		return nil, nil
	}
	return append([]byte(nil), s.objects[objectKey]...), nil
}
