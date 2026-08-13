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

package aio

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	"github.com/stretchr/testify/require"
)

func TestWorkspaceMapperDerivesOnlyCanonicalThreadIdentity(t *testing.T) {
	ref := workspaceTestSessionRef()
	mapper, err := NewWorkspaceMapper(ref)
	require.NoError(t, err)

	require.Equal(t, "/mnt/user-data/42/7/thread.alpha/workspace", mapper.physicalWorkspaceRoot())
	require.Equal(t, "/mnt/user-data/42/7/thread.alpha", mapper.physicalThreadRoot())

	changed := ref
	changed.SessionID = "another-session"
	changed.Key.DeploymentID = "another-deployment"
	changed.Key.ProviderID = 999
	changed.Key.Profile = domainsandbox.SessionProfileInteractive
	changed.RuntimeGeneration = 999
	changedMapper, err := NewWorkspaceMapper(changed)
	require.NoError(t, err)
	require.Equal(t, mapper.physicalThreadRoot(), changedMapper.physicalThreadRoot())
}

func TestWorkspaceMapperRejectsInvalidSessionRef(t *testing.T) {
	mutations := []func(*domainsandbox.SessionRef){
		func(ref *domainsandbox.SessionRef) { ref.Key.SpaceID = 0 },
		func(ref *domainsandbox.SessionRef) { ref.Key.UserID = -1 },
		func(ref *domainsandbox.SessionRef) { ref.Key.ThreadID = "../sibling" },
		func(ref *domainsandbox.SessionRef) { ref.RuntimeGeneration = 0 },
		func(ref *domainsandbox.SessionRef) { ref.SessionID = "" },
	}
	for index, mutate := range mutations {
		t.Run(fmt.Sprintf("invalid-%d", index), func(t *testing.T) {
			ref := workspaceTestSessionRef()
			mutate(&ref)
			mapper, err := NewWorkspaceMapper(ref)
			require.Nil(t, mapper)
			require.ErrorIs(t, err, domainsandbox.ErrInvalidInput)
		})
	}
}

func TestWorkspaceMapperMapsCanonicalLogicalRoots(t *testing.T) {
	mapper := newWorkspaceTestMapper(t)
	tests := []struct {
		operation WorkspaceOperation
		logical   string
		physical  string
	}{
		{WorkspaceOperationRead, "/mnt/user-data/workspace", "/mnt/user-data/42/7/thread.alpha/workspace"},
		{WorkspaceOperationWrite, "/mnt/user-data/workspace/src/main.go", "/mnt/user-data/42/7/thread.alpha/workspace/src/main.go"},
		{WorkspaceOperationReplace, "/mnt/user-data/uploads/input.txt", "/mnt/user-data/42/7/thread.alpha/uploads/input.txt"},
		{WorkspaceOperationList, "/mnt/user-data/outputs", "/mnt/user-data/42/7/thread.alpha/outputs"},
		{WorkspaceOperationGlob, "/mnt/skills", "/mnt/skills"},
		{WorkspaceOperationGrep, "/mnt/skills/library/README.md", "/mnt/skills/library/README.md"},
		{WorkspaceOperationDownload, "/mnt/skills/library/manual.pdf", "/mnt/skills/library/manual.pdf"},
	}
	for _, test := range tests {
		t.Run(fmt.Sprintf("%d-%s", test.operation, test.logical), func(t *testing.T) {
			physical, err := mapper.ResolveFilePath(test.operation, test.logical)
			require.NoError(t, err)
			require.Equal(t, test.physical, physical)

			logical, err := mapper.ReverseFilePath(physical)
			require.NoError(t, err)
			require.Equal(t, test.logical, logical)
		})
	}
}

func TestWorkspaceMapperEnforcesReadOnlySkills(t *testing.T) {
	mapper := newWorkspaceTestMapper(t)
	for _, operation := range []WorkspaceOperation{
		WorkspaceOperationRead,
		WorkspaceOperationList,
		WorkspaceOperationGlob,
		WorkspaceOperationGrep,
		WorkspaceOperationDownload,
	} {
		_, err := mapper.ResolveFilePath(operation, "/mnt/skills/catalog/skill.md")
		require.NoError(t, err, "operation %d", operation)
	}
	for _, operation := range []WorkspaceOperation{WorkspaceOperationWrite, WorkspaceOperationReplace} {
		physical, err := mapper.ResolveFilePath(operation, "/mnt/skills/catalog/skill.md")
		require.Empty(t, physical)
		require.ErrorIs(t, err, domainsandbox.ErrInvalidInput)
	}
	_, err := mapper.ResolveFilePath(WorkspaceOperation(255), "/mnt/user-data/workspace/file")
	require.ErrorIs(t, err, domainsandbox.ErrInvalidInput)
}

func TestWorkspaceMapperExecCWDIsWorkspaceOnly(t *testing.T) {
	mapper := newWorkspaceTestMapper(t)
	for logical, want := range map[string]string{
		"/mnt/user-data/workspace":         "/mnt/user-data/42/7/thread.alpha/workspace",
		"/mnt/user-data/workspace/project": "/mnt/user-data/42/7/thread.alpha/workspace/project",
	} {
		got, err := mapper.ResolveExecCWD(logical)
		require.NoError(t, err)
		require.Equal(t, want, got)
	}
	for _, logical := range []string{
		"/mnt/user-data/uploads",
		"/mnt/user-data/outputs",
		"/mnt/skills",
		"workspace",
	} {
		got, err := mapper.ResolveExecCWD(logical)
		require.Empty(t, got)
		require.ErrorIs(t, err, domainsandbox.ErrInvalidInput)
	}
}

func TestWorkspaceMapperRejectsNonCanonicalOrPhysicalClientPaths(t *testing.T) {
	mapper := newWorkspaceTestMapper(t)
	invalidUTF8 := string([]byte{'/', 'm', 'n', 't', '/', 'u', 's', 'e', 'r', '-', 'd', 'a', 't', 'a', '/', 'w', 'o', 'r', 'k', 's', 'p', 'a', 'c', 'e', '/', 0xff})
	require.False(t, utf8.ValidString(invalidUTF8))
	longPath := "/mnt/user-data/workspace/" + strings.Repeat("a", maxWorkspaceLogicalPathBytes)
	tests := []string{
		"/mnt/user-data/42/7/thread.alpha/workspace/secret.txt",
		"/mnt/user-data/42/7/other-thread/workspace/secret.txt",
		"/mnt/user-data/workspace/../outputs/secret.txt",
		"/mnt/user-data/workspace//file.txt",
		"/mnt/user-data/workspace/./file.txt",
		"/mnt/user-data/workspace/file.txt/",
		"/mnt/user-data/workspace\\file.txt",
		"/mnt/user-data/workspace/file\x00.txt",
		"/mnt/user-data/workspace/file\n.txt",
		"/mnt/user-data/workspace/file\u007f.txt",
		"/mnt/user-data",
		"/mnt/user-data/workspace-other/file.txt",
		"//mnt/user-data/workspace/file.txt",
		invalidUTF8,
		longPath,
	}
	for index, logical := range tests {
		t.Run(fmt.Sprintf("invalid-%d", index), func(t *testing.T) {
			physical, err := mapper.ResolveFilePath(WorkspaceOperationRead, logical)
			require.Empty(t, physical)
			require.ErrorIs(t, err, domainsandbox.ErrInvalidInput)
		})
	}
}

func TestWorkspaceMapperReverseMapFailsClosedOutsideCurrentRoots(t *testing.T) {
	mapper := newWorkspaceTestMapper(t)
	for index, physical := range []string{
		"/mnt/user-data/42/7/thread.alpha",
		"/mnt/user-data/42/7/thread.alpha/cache/file.txt",
		"/mnt/user-data/42/7/thread.alpha/workspace/../uploads/file.txt",
		"/mnt/user-data/42/7/other-thread/workspace/file.txt",
		"/mnt/user-data/42/7/thread.alpha-other/workspace/file.txt",
		"/mnt/user-data/42/7/thread.alpha/workspace//file.txt",
		"/mnt/user-data/42/7/thread.alpha/workspace\\file.txt",
		"/mnt/user-data/42/7/thread.alpha/workspace/file\x00.txt",
		"/mnt/user-data/42/7",
		"/etc/passwd",
	} {
		t.Run(fmt.Sprintf("invalid-%d", index), func(t *testing.T) {
			logical, err := mapper.ReverseFilePath(physical)
			require.Empty(t, logical)
			require.ErrorIs(t, err, domainsandbox.ErrInvalidInput)
			require.NotContains(t, err.Error(), physical)
		})
	}
}

func TestWorkspaceMapperReverseMapRejectsOverlongLogicalResult(t *testing.T) {
	mapper := newWorkspaceTestMapper(t)
	logicalOverflowSuffix := "/" + strings.Repeat("a", maxWorkspaceLogicalPathBytes-len(logicalWorkspaceRoot))
	physical := mapper.physicalWorkspaceRoot() + logicalOverflowSuffix

	logical, err := mapper.ReverseFilePath(physical)
	require.Empty(t, logical)
	require.ErrorIs(t, err, domainsandbox.ErrInvalidInput)
}

func TestWorkspaceMapperFormattingRedactsPhysicalIdentity(t *testing.T) {
	mapper := newWorkspaceTestMapper(t)
	for _, formatted := range []string{
		fmt.Sprint(mapper),
		fmt.Sprintf("%v", mapper),
		fmt.Sprintf("%+v", mapper),
		fmt.Sprintf("%#v", mapper),
		fmt.Sprint(*mapper),
		fmt.Sprintf("%+v", *mapper),
	} {
		require.Equal(t, "aio.WorkspaceMapper{workspace:<redacted>}", formatted)
		require.NotContains(t, formatted, "thread.alpha")
		require.NotContains(t, formatted, "/mnt/user-data")
	}
}

func FuzzWorkspaceMapperAcceptedWritesStayInThreadSubroot(f *testing.F) {
	for _, seed := range []string{
		"/mnt/user-data/workspace",
		"/mnt/user-data/workspace/src/main.go",
		"/mnt/user-data/uploads/input.txt",
		"/mnt/user-data/outputs/result.txt",
		"/mnt/skills/forbidden",
		"/mnt/user-data/workspace/../outputs/escape",
		"/etc/passwd",
	} {
		f.Add(seed)
	}
	mapper, err := NewWorkspaceMapper(workspaceTestSessionRef())
	if err != nil {
		f.Fatalf("NewWorkspaceMapper() error = %v", err)
	}
	f.Fuzz(func(t *testing.T, logical string) {
		physical, err := mapper.ResolveFilePath(WorkspaceOperationWrite, logical)
		if err != nil {
			if !errors.Is(err, domainsandbox.ErrInvalidInput) {
				t.Fatalf("ResolveFilePath() error = %v, want ErrInvalidInput", err)
			}
			return
		}
		logicalSubroot := workspaceLogicalSubroot(logical)
		physicalSubroot := mapper.physicalThreadRoot() + "/" + logicalSubroot
		if physical != physicalSubroot && !strings.HasPrefix(physical, physicalSubroot+"/") {
			t.Fatalf("accepted write escaped its thread subroot")
		}
	})
}

func workspaceLogicalSubroot(logical string) string {
	const prefix = "/mnt/user-data/"
	remainder := strings.TrimPrefix(logical, prefix)
	if separator := strings.IndexByte(remainder, '/'); separator >= 0 {
		return remainder[:separator]
	}
	return remainder
}

func newWorkspaceTestMapper(t *testing.T) *WorkspaceMapper {
	t.Helper()
	mapper, err := NewWorkspaceMapper(workspaceTestSessionRef())
	require.NoError(t, err)
	return mapper
}

func workspaceTestSessionRef() domainsandbox.SessionRef {
	return domainsandbox.SessionRef{
		SessionID: "4ac19f2d-64e3-437e-b5a4-0d69dc65c6b2",
		Key: domainsandbox.SessionKey{
			DeploymentID: "aio-primary",
			ProviderID:   11,
			SpaceID:      42,
			UserID:       7,
			ThreadID:     "thread.alpha",
			Profile:      domainsandbox.SessionProfileCore,
		},
		RuntimeGeneration: 3,
	}
}
