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

package tos

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"

	"github.com/coze-dev/coze-studio/backend/infra/storage"
	"github.com/coze-dev/coze-studio/backend/infra/storage/impl/internal/contract"
)

type tosReadinessRecorder struct {
	contract.ReadinessRecorder
	err error
}

func (r *tosReadinessRecorder) HeadBucket(context.Context, string) error {
	r.HeadBucketCalls++
	return r.err
}

func TestCheckReadinessCanceledContextDoesNotCallSDK(t *testing.T) {
	recorder := &tosReadinessRecorder{}
	client := &tosClient{bucketName: "bucket", readinessCheck: recorder.HeadBucket}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := client.CheckReadiness(ctx)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("CheckReadiness(canceled) error = %v", err)
	}
	if recorder.CreateCalls != 0 || recorder.PutCalls != 0 || recorder.DeleteCalls != 0 {
		t.Fatalf("write calls = %+v, want all 0", recorder.ReadinessRecorder)
	}
	if recorder.HeadBucketCalls != 0 {
		t.Fatalf("HeadBucketCalls = %d, want 0", recorder.HeadBucketCalls)
	}
}

func TestCheckReadinessMapsSDKError(t *testing.T) {
	recorder := &tosReadinessRecorder{err: errors.New("sdk unavailable")}
	client := &tosClient{bucketName: "bucket", readinessCheck: recorder.HeadBucket}

	err := client.CheckReadiness(context.Background())

	if !errors.Is(err, storage.ErrReadinessUnavailable) {
		t.Fatalf("CheckReadiness(sdk error) error = %v", err)
	}
	contract.AssertReadinessIsReadOnly(t, recorder.ReadinessRecorder)
	if recorder.HeadBucketCalls != 1 {
		t.Fatalf("HeadBucketCalls = %d, want 1", recorder.HeadBucketCalls)
	}
}

func TestTOSReadinessIsReadOnly(t *testing.T) {
	recorder := &tosReadinessRecorder{}
	client := &tosClient{bucketName: "bucket", readinessCheck: recorder.HeadBucket}

	if err := client.CheckReadiness(context.Background()); err != nil {
		t.Fatalf("CheckReadiness() error = %v", err)
	}
	contract.AssertReadinessIsReadOnly(t, recorder.ReadinessRecorder)
	if recorder.HeadBucketCalls != 1 {
		t.Fatalf("HeadBucketCalls = %d, want 1", recorder.HeadBucketCalls)
	}
}

func TestProductionFileDoesNotExposeReadinessTestHook(t *testing.T) {
	assertNoReadinessTestHook(t, "tos.go")
}

func TestLoggingDoesNotIncludeSignedURL(t *testing.T) {
	source, err := os.ReadFile("tos.go")
	if err != nil {
		t.Fatalf("ReadFile(tos.go) error = %v", err)
	}
	content := string(source)
	if strings.Contains(content, "url = %s") {
		t.Fatal("tos.go debug logs include signed URL format")
	}
	if strings.Contains(content, "object.URL") {
		t.Fatal("tos.go debug logs include object.URL")
	}
}

func assertNoReadinessTestHook(t *testing.T, filename string) {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), filename, nil, 0)
	if err != nil {
		t.Fatalf("ParseFile(%s) error = %v", filename, err)
	}
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "SetReadinessCheckForTest" {
			t.Fatalf("%s exports SetReadinessCheckForTest", filename)
		}
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.VAR {
			continue
		}
		for _, spec := range gen.Specs {
			valueSpec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for _, name := range valueSpec.Names {
				if name.Name == "readinessCheckForTest" {
					t.Fatalf("%s defines readinessCheckForTest", filename)
				}
			}
		}
	}
}
