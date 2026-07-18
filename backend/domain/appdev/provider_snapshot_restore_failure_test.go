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
	"testing"
)

func TestValidateFailProviderSnapshotRestoreInput(t *testing.T) {
	full, err := HashProviderSnapshotRestoreOperation("1001", "project-a", "snapshot-a", "snapshot-fail-operation")
	if err != nil {
		t.Fatal(err)
	}
	parent, err := HashProviderSnapshotRestoreParentOperation("1001", "project-a", "snapshot-fail-operation")
	if err != nil {
		t.Fatal(err)
	}
	valid := FailProviderSnapshotRestoreInput{
		SpaceID: "1001", ProjectID: "project-a", SnapshotID: "snapshot-a", OperationHash: full,
		ParentOperationHash: parent, ExpectedSourceVersion: 7, SafeErrorCode: "snapshot_invalid", SafeErrorMessage: "snapshot restore cannot continue",
	}
	if err := ValidateFailProviderSnapshotRestoreInput(valid); err != nil {
		t.Fatalf("valid failure input rejected: %v", err)
	}
	invalid := valid
	invalid.SafeErrorMessage = "raw\nsecret"
	if err := ValidateFailProviderSnapshotRestoreInput(invalid); err == nil {
		t.Fatal("unsafe failure message accepted")
	}
}
