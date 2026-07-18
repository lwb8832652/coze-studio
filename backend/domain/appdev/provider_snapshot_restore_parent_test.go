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
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestProviderSnapshotRestoreParentOperationHashSeparatesIntentFromSnapshot(t *testing.T) {
	parentA, err := HashProviderSnapshotRestoreParentOperation("1001", "project-a", "snapshot-parent-operation-a")
	if err != nil {
		t.Fatal(err)
	}
	parentARetry, err := HashProviderSnapshotRestoreParentOperation("1001", "project-a", "snapshot-parent-operation-a")
	if err != nil {
		t.Fatal(err)
	}
	parentB, err := HashProviderSnapshotRestoreParentOperation("1001", "project-a", "snapshot-parent-operation-b")
	if err != nil {
		t.Fatal(err)
	}
	fullA, err := HashProviderSnapshotRestoreOperation("1001", "project-a", "snapshot-a", "snapshot-parent-operation-a")
	if err != nil {
		t.Fatal(err)
	}
	fullB, err := HashProviderSnapshotRestoreOperation("1001", "project-a", "snapshot-b", "snapshot-parent-operation-a")
	if err != nil {
		t.Fatal(err)
	}
	if !parentA.Equal(parentARetry) || parentA.Equal(parentB) || fullA.Equal(fullB) {
		t.Fatalf("parent/full hash identities are not separated")
	}
	formatted := fmt.Sprintf("%v|%+v|%#v", parentA, parentA, parentA)
	if strings.Contains(formatted, fmt.Sprintf("%x", parentA.Bytes())) {
		t.Fatalf("parent hash leaked through formatting: %s", formatted)
	}
	if encoded, marshalErr := json.Marshal(parentA); marshalErr == nil || len(encoded) != 0 {
		t.Fatalf("parent hash JSON = %q, %v", encoded, marshalErr)
	}
}
