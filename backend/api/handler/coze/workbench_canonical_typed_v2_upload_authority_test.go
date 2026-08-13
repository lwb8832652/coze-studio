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
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/bytedance/mockey"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/stretchr/testify/require"

	appagentthread "github.com/coze-dev/coze-studio/backend/application/agentthread"
	domainentity "github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	domainservice "github.com/coze-dev/coze-studio/backend/domain/agentthread/service"
)

func TestCreateCanonicalRunTypedV2ResolvesUploadedFilesAuthoritativelyInRequestOrder(t *testing.T) {
	installAgentThreadTestService(t)
	thread := createCanonicalTestThread(t, 1001, "typed upload authority", `{}`)

	register := func(name, digest string) *domainentity.AgentFile {
		t.Helper()
		file, err := appagentthread.SVC.UploadFileSVC.RegisterUploadFile(
			context.Background(),
			&domainservice.RegisterUploadFileRequest{
				SpaceID: thread.SpaceID, UserID: 2, ThreadID: thread.ThreadID,
				FileName: name, ContentType: "text/plain", SizeBytes: 12,
				Digest: digest, Metadata: `{}`,
			},
		)
		require.NoError(t, err)
		require.NotNil(t, file)
		return file
	}
	first := register("first.txt", strings.Repeat("1", 64))
	second := register("second.txt", strings.Repeat("2", 64))

	submission := canonicalTypedV2Replace(
		canonicalTypedRunTurnV2("inspect ordered attachments"),
		`"uploaded_files":[]`,
		fmt.Sprintf(`"uploaded_files":[{"file_id":"%d"},{"file_id":"%d"}]`, second.ID, first.ID),
	)
	response := performCanonicalRunJSONRequest(
		t,
		canonicalRunTestServer(),
		http.MethodPost,
		fmt.Sprintf("/api/workbench/threads/%d/runs", thread.ThreadID),
		canonicalTypedRunRequestV2(submission, ""),
	)
	require.Equal(t, http.StatusOK, response.Code, response.Result().Body())

	runs := canonicalRunsForThread(t, thread.ThreadID)
	require.Len(t, runs, 1)
	var input struct {
		UploadedFiles []*appagentthread.TaskThreadUploadedFileSummary `json:"uploaded_files"`
	}
	require.NoError(t, json.Unmarshal([]byte(runs[0].Input), &input))
	require.Len(t, input.UploadedFiles, 2)
	require.Equal(t, []int64{second.ID, first.ID}, []int64{
		input.UploadedFiles[0].FileID,
		input.UploadedFiles[1].FileID,
	})
	require.Equal(t, []string{"second.txt", "first.txt"}, []string{
		input.UploadedFiles[0].FileName,
		input.UploadedFiles[1].FileName,
	})
}

func TestCreateCanonicalRunTypedV2RetryRejectsForeignThreadSourceWithoutMutation(t *testing.T) {
	installAgentThreadTestService(t)
	target := createCanonicalTestThread(t, 1001, "typed retry target", `{}`)
	foreign := createCanonicalTestThread(t, 1001, "typed retry foreign", `{}`)
	source := createCanonicalRunFixture(t, foreign.ThreadID, "foreign failed source")
	failCanonicalRunFixture(t, source, "runtime_failed", "failed")

	response := performCanonicalRunJSONRequest(
		t,
		canonicalRunTestServer(),
		http.MethodPost,
		fmt.Sprintf("/api/workbench/threads/%d/runs", target.ThreadID),
		canonicalTypedRunRequestV2(canonicalTypedRunRetryV2(source.RunID, "retry foreign"), ""),
		ut.Header{Key: "Idempotency-Key", Value: "typed-retry-foreign"},
	)
	require.Equal(t, http.StatusNotFound, response.Code, response.Result().Body())
	require.Empty(t, canonicalRunsForThread(t, target.ThreadID))
	require.Len(t, canonicalRunsForThread(t, foreign.ThreadID), 1)
}

func TestCreateCanonicalRunTypedV2StrictFailurePrecedesSourceLookup(t *testing.T) {
	installAgentThreadTestService(t)
	thread := createCanonicalTestThread(t, 1001, "typed strict before source", `{}`)
	getRunCalls := 0
	patch := mockey.Mock((*appagentthread.ApplicationService).GetRun).To(
		func(
			*appagentthread.ApplicationService,
			context.Context,
			*appagentthread.GetRunRequest,
		) (*appagentthread.GetRunResponse, error) {
			getRunCalls++
			return nil, fmt.Errorf("unexpected source lookup")
		},
	).Build()
	t.Cleanup(func() { patch.UnPatch() })

	invalid := canonicalTypedV2Replace(
		canonicalTypedRunRetryV2(999999, "invalid before lookup"),
		`"message":"invalid before lookup"`,
		`"message":"invalid before lookup","future":"must-not-echo"`,
	)
	response := performCanonicalRunJSONRequest(
		t,
		canonicalRunTestServer(),
		http.MethodPost,
		fmt.Sprintf("/api/workbench/threads/%d/runs", thread.ThreadID),
		canonicalTypedRunRequestV2(invalid, ""),
	)
	require.Equal(t, http.StatusUnprocessableEntity, response.Code, response.Result().Body())
	var public canonicalError
	require.NoError(t, json.Unmarshal(response.Result().Body(), &public))
	require.Equal(t, "unsupported_sdk_field", public.Code)
	require.Zero(t, getRunCalls)
	require.Empty(t, canonicalRunsForThread(t, thread.ThreadID))
}
