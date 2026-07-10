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

package appdev

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
)

func TestImportProjectArchiveRejectsTooManyEntries(t *testing.T) {
	entries := make(map[string]string, appDevMaxArchiveEntries+1)
	for index := 0; index <= appDevMaxArchiveEntries; index++ {
		entries[fmt.Sprintf("src/file-%04d.ts", index)] = "export {};"
	}

	store := NewLocalStoreForTest(t.TempDir())
	_, err := store.ImportProjectArchive(
		context.Background(),
		&domainappdev.Project{ID: "project", SpaceID: "space"},
		makeProjectArchive(t, entries),
	)

	require.ErrorContains(t, err, "cannot exceed 2000 entries")
}

func TestImportProjectArchiveRejectsExcessivePathDepth(t *testing.T) {
	deepPath := strings.Repeat("level/", appDevMaxArchiveDepth) + "file.ts"
	store := NewLocalStoreForTest(t.TempDir())
	_, err := store.ImportProjectArchive(
		context.Background(),
		&domainappdev.Project{ID: "project", SpaceID: "space"},
		makeProjectArchive(t, map[string]string{deepPath: "export {};"}),
	)

	require.ErrorContains(t, err, "path depth cannot exceed 20")
}

func TestImportProjectArchiveCleansPartialProjectOnFailure(t *testing.T) {
	root := t.TempDir()
	store := NewLocalStoreForTest(root)
	project := &domainappdev.Project{ID: "project", SpaceID: "space"}
	archive := makeProjectArchive(t, map[string]string{
		"src/App.tsx": "export default function App() { return null; }",
		"../invalid":  "invalid",
	})

	_, err := store.ImportProjectArchive(context.Background(), project, archive)

	require.Error(t, err)
	_, statErr := os.Stat(store.projectDir(project.SpaceID, project.ID))
	require.True(t, os.IsNotExist(statErr), "partial imported project must be removed")
}

func makeProjectArchive(t *testing.T, entries map[string]string) []byte {
	t.Helper()

	buffer := bytes.NewBuffer(nil)
	writer := zip.NewWriter(buffer)
	for name, content := range entries {
		entry, err := writer.Create(name)
		require.NoError(t, err)
		_, err = entry.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())
	return buffer.Bytes()
}
