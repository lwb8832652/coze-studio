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

package s3

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/coze-dev/coze-studio/backend/infra/storage"
)

type s3ReadinessRecorder struct {
	HeadBucketCalls   int
	CreateBucketCalls int
	PutCalls          int
	DeleteCalls       int
	err               error
}

func (r *s3ReadinessRecorder) HeadBucket(context.Context, string) error {
	r.HeadBucketCalls++
	return r.err
}

func TestCheckReadinessCanceledContextDoesNotCallSDK(t *testing.T) {
	recorder := &s3ReadinessRecorder{}
	client := &s3Client{bucketName: "bucket", readinessCheck: recorder.HeadBucket}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := client.CheckReadiness(ctx)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("CheckReadiness(canceled) error = %v", err)
	}
	assertS3NoWriteCalls(t, recorder)
	if recorder.HeadBucketCalls != 0 {
		t.Fatalf("HeadBucketCalls = %d, want 0", recorder.HeadBucketCalls)
	}
}

func TestCheckReadinessMapsSDKError(t *testing.T) {
	recorder := &s3ReadinessRecorder{err: errors.New("sdk unavailable")}
	client := &s3Client{bucketName: "bucket", readinessCheck: recorder.HeadBucket}

	err := client.CheckReadiness(context.Background())

	if !errors.Is(err, storage.ErrReadinessUnavailable) {
		t.Fatalf("CheckReadiness(sdk error) error = %v", err)
	}
	assertS3NoWriteCalls(t, recorder)
	if recorder.HeadBucketCalls != 1 {
		t.Fatalf("HeadBucketCalls = %d, want 1", recorder.HeadBucketCalls)
	}
}

func TestCheckReadinessSuccessUsesOnlyHeadBucket(t *testing.T) {
	recorder := &s3ReadinessRecorder{}
	client := &s3Client{bucketName: "bucket", readinessCheck: recorder.HeadBucket}

	if err := client.CheckReadiness(context.Background()); err != nil {
		t.Fatalf("CheckReadiness() error = %v", err)
	}
	assertS3NoWriteCalls(t, recorder)
	if recorder.HeadBucketCalls != 1 {
		t.Fatalf("HeadBucketCalls = %d, want 1", recorder.HeadBucketCalls)
	}
}

func TestProductionFileDoesNotExposeReadinessTestHook(t *testing.T) {
	assertNoReadinessTestHook(t, "s3.go")
}

func assertS3NoWriteCalls(t *testing.T, recorder *s3ReadinessRecorder) {
	t.Helper()
	if recorder.CreateBucketCalls != 0 || recorder.PutCalls != 0 || recorder.DeleteCalls != 0 {
		t.Fatalf("write calls = create:%d put:%d delete:%d, want all 0", recorder.CreateBucketCalls, recorder.PutCalls, recorder.DeleteCalls)
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
