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
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/stretchr/testify/require"

	"github.com/coze-dev/coze-studio/backend/pkg/sonic"
)

var canonicalEntrypoints = []struct {
	name    string
	handler app.HandlerFunc
}{
	{"CreateCanonicalThread", CreateCanonicalThread},
	{"SearchCanonicalThreads", SearchCanonicalThreads},
	{"GetCanonicalThread", GetCanonicalThread},
	{"PatchCanonicalThread", PatchCanonicalThread},
	{"DeleteCanonicalThread", DeleteCanonicalThread},
	{"GetCanonicalThreadState", GetCanonicalThreadState},
	{"UpdateCanonicalThreadState", UpdateCanonicalThreadState},
	{"GetCanonicalThreadHistory", GetCanonicalThreadHistory},
	{"PostCanonicalThreadHistory", PostCanonicalThreadHistory},
	{"ListCanonicalThreadMessages", ListCanonicalThreadMessages},
	{"ListCanonicalRuns", ListCanonicalRuns},
	{"CreateCanonicalRun", CreateCanonicalRun},
	{"StreamCanonicalRun", StreamCanonicalRun},
	{"WaitCanonicalRun", WaitCanonicalRun},
	{"GetCanonicalRun", GetCanonicalRun},
	{"ReconnectCanonicalRunStream", ReconnectCanonicalRunStream},
	{"JoinCanonicalRun", JoinCanonicalRun},
	{"CancelCanonicalRun", CancelCanonicalRun},
	{"ResumeCanonicalRun", ResumeCanonicalRun},
	{"ListCanonicalRunEvents", ListCanonicalRunEvents},
	{"ListCanonicalRunMessages", ListCanonicalRunMessages},
	{"AppendCanonicalThreadMessage", AppendCanonicalThreadMessage},
	{"GenerateCanonicalThreadSuggestions", GenerateCanonicalThreadSuggestions},
	{"ListCanonicalThreadUploads", ListCanonicalThreadUploads},
	{"UploadCanonicalThreadFiles", UploadCanonicalThreadFiles},
	{"DeleteCanonicalThreadUpload", DeleteCanonicalThreadUpload},
	{"ListCanonicalThreadArtifacts", ListCanonicalThreadArtifacts},
	{"GetCanonicalThreadArtifactContent", GetCanonicalThreadArtifactContent},
	{"GetCanonicalThreadArtifactSignedURL", GetCanonicalThreadArtifactSignedURL},
	{"DeleteCanonicalThreadArtifact", DeleteCanonicalThreadArtifact},
	{"RestoreCanonicalThreadArtifact", RestoreCanonicalThreadArtifact},
	{"ReviewCanonicalThreadArtifactScan", ReviewCanonicalThreadArtifactScan},
	{"ListCanonicalThreadArtifactScanJobs", ListCanonicalThreadArtifactScanJobs},
	{"RetryCanonicalThreadArtifactScanJob", RetryCanonicalThreadArtifactScanJob},
	{"GetCanonicalThreadTokenUsage", GetCanonicalThreadTokenUsage},
	{"ListCanonicalThreadMemories", ListCanonicalThreadMemories},
	{"UpdateCanonicalThreadMemory", UpdateCanonicalThreadMemory},
	{"DeleteCanonicalThreadMemory", DeleteCanonicalThreadMemory},
	{"RestoreCanonicalThreadMemory", RestoreCanonicalThreadMemory},
	{"ClearCanonicalThreadMemories", ClearCanonicalThreadMemories},
	{"ExportCanonicalThreadMemories", ExportCanonicalThreadMemories},
	{"ImportCanonicalThreadMemories", ImportCanonicalThreadMemories},
	{"ListCanonicalThreadMemoryAuditEvents", ListCanonicalThreadMemoryAuditEvents},
	{"ListCanonicalThreadGuardrailAuditEvents", ListCanonicalThreadGuardrailAuditEvents},
	{"ExportCanonicalThreadGuardrailAuditEvents", ExportCanonicalThreadGuardrailAuditEvents},
	{"ListCanonicalThreadMCPRuntimeAuditEvents", ListCanonicalThreadMCPRuntimeAuditEvents},
	{"RetryCanonicalSubagentRun", RetryCanonicalSubagentRun},
}

func TestCanonicalEntrypointsAreAlwaysActive(t *testing.T) {
	require.Len(t, canonicalEntrypoints, 47)

	for _, entrypoint := range canonicalEntrypoints {
		entrypoint := entrypoint
		t.Run(entrypoint.name, func(t *testing.T) {
			var c app.RequestContext
			entrypoint.handler(context.Background(), &c)
			require.Contains(t, []int{
				consts.StatusBadRequest,
				consts.StatusUnauthorized,
				consts.StatusNotFound,
				consts.StatusServiceUnavailable,
			}, c.Response.StatusCode())
			require.False(t,
				c.Response.StatusCode() == consts.StatusNotFound && len(c.Response.Body()) == 0,
				"handler returned the retired empty-body migration-gate response",
			)
		})
	}
}

func TestCanonicalCoreEntrypointsRequireAuthorizedSpaceHeader(t *testing.T) {
	installAgentThreadTestService(t)
	require.GreaterOrEqual(t, len(canonicalEntrypoints), 21)

	for _, entrypoint := range canonicalEntrypoints[:21] {
		entrypoint := entrypoint
		t.Run(entrypoint.name, func(t *testing.T) {
			var c app.RequestContext
			entrypoint.handler(canonicalViewerContext(2), &c)

			require.Equal(t, consts.StatusBadRequest, c.Response.StatusCode())
			var response canonicalError
			require.NoError(t, sonic.Unmarshal(c.Response.Body(), &response))
			require.Equal(t, "invalid_space_id", response.Code)
		})
	}
}

func TestCanonicalMigrationGateIsAbsentFromProductionGo(t *testing.T) {
	_, sourceFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	backendDir := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "..", "..", ".."))
	forbidden := []string{
		"COZE_WORKBENCH_CANONICAL_API_ENABLED",
		"canonicalAPIEnabled",
		"requireCanonicalAPI",
		"serveCanonicalEntrypoint",
	}

	var matches []string
	err := filepath.WalkDir(backendDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		for _, identifier := range forbidden {
			if strings.Contains(string(content), identifier) {
				relative, relErr := filepath.Rel(backendDir, path)
				if relErr != nil {
					return relErr
				}
				matches = append(matches, relative+": "+identifier)
			}
		}
		return nil
	})
	require.NoError(t, err)
	require.Empty(t, matches)
}
