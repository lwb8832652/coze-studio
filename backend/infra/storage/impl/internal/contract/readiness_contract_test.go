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

package contract

import (
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
)

func TestAssertReadinessIsReadOnlyRejectsSignedURLCalls(t *testing.T) {
	if os.Getenv("COZE_STORAGE_READINESS_SIGNED_URL_PROBE") == "1" {
		recorder := ReadinessRecorder{HeadBucketCalls: 1}
		setSignedURLCalls(t, &recorder, 1)
		AssertReadinessIsReadOnly(t, recorder)
		return
	}

	recorder := ReadinessRecorder{}
	setSignedURLCalls(t, &recorder, 0)

	cmd := exec.Command(os.Args[0], "-test.run=^TestAssertReadinessIsReadOnlyRejectsSignedURLCalls$")
	cmd.Env = append(os.Environ(), "COZE_STORAGE_READINESS_SIGNED_URL_PROBE=1")

	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("signed URL readiness probe passed, want read-only assertion failure")
	}
	if strings.Contains(string(output), "secret=") {
		t.Fatal("readiness assertion output included signed URL details")
	}
}

func setSignedURLCalls(t *testing.T, recorder *ReadinessRecorder, calls int) {
	t.Helper()
	field := reflect.ValueOf(recorder).Elem().FieldByName("SignedURLCalls")
	if !field.IsValid() {
		t.Fatal("ReadinessRecorder does not expose SignedURLCalls")
	}
	field.SetInt(int64(calls))
}
