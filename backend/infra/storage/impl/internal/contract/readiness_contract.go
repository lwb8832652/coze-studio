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

import "testing"

type ReadinessRecorder struct {
	HeadBucketCalls int
	ListCalls       int
	CreateCalls     int
	PutCalls        int
	DeleteCalls     int
}

func AssertReadinessIsReadOnly(t *testing.T, recorder ReadinessRecorder) {
	t.Helper()
	if recorder.CreateCalls != 0 || recorder.PutCalls != 0 || recorder.DeleteCalls != 0 {
		t.Fatalf("readiness performed writes: %+v", recorder)
	}
	if recorder.HeadBucketCalls+recorder.ListCalls == 0 {
		t.Fatalf("readiness did not perform a read-only bucket check: %+v", recorder)
	}
}
