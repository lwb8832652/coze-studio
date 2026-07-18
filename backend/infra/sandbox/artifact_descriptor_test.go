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

package sandbox

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestArtifactDescriptorValidationAndResultContract(t *testing.T) {
	valid := ArtifactDescriptor{Kind: ArtifactKindAppDevBuildArchive, Digest: "sha256:" + strings.Repeat("a", 64), Size: 1024}
	_, err := NormalizeArtifactDescriptor(valid)
	require.NoError(t, err)

	invalid := []ArtifactDescriptor{
		{Kind: "other", Digest: valid.Digest, Size: valid.Size},
		{Kind: valid.Kind, Digest: "sha256:" + strings.Repeat("0", 64), Size: valid.Size},
		{Kind: valid.Kind, Digest: "sha256:" + strings.Repeat("A", 64), Size: valid.Size},
		{Kind: valid.Kind, Digest: valid.Digest, Size: 0},
		{Kind: valid.Kind, Digest: valid.Digest, Size: MaxArtifactDescriptorBytes + 1},
	}
	for _, descriptor := range invalid {
		_, err := NormalizeArtifactDescriptor(descriptor)
		require.Error(t, err)
	}

	exitCode := 0
	result, err := NormalizeExecuteResult(ExecuteResult{
		ExecutionID: "execution-1", Status: ExecutionStatusSucceeded, ExitCode: &exitCode,
		ArtifactDescriptors: []ArtifactDescriptor{valid},
	}, 1024)
	require.NoError(t, err)
	require.Equal(t, valid, result.ArtifactDescriptors[0])
	_, err = NormalizeExecuteResult(ExecuteResult{
		ExecutionID: "execution-1", Status: ExecutionStatusRunning,
		ArtifactDescriptors: []ArtifactDescriptor{valid},
	}, 1024)
	require.Error(t, err)
}

func TestArtifactPublishRequestFormattingAndJSONRedactCapability(t *testing.T) {
	secret := "grant-token-secret"
	grantURL := "https://gateway.example/internal/grants/secret-id"
	request := ArtifactPublishRequest{
		Descriptor: ArtifactDescriptor{Kind: ArtifactKindAppDevBuildArchive, Digest: "sha256:" + strings.Repeat("b", 64), Size: 10},
		UploadURL:  grantURL, Token: secret, ExpiresAt: time.Now().Add(time.Minute),
	}
	for _, formatted := range []string{fmt.Sprintf("%v", request), fmt.Sprintf("%+v", request), fmt.Sprintf("%#v", request)} {
		require.NotContains(t, formatted, secret)
		require.NotContains(t, formatted, grantURL)
	}
	encoded, err := json.Marshal(request)
	require.Error(t, err)
	require.NotContains(t, string(encoded), secret)
}
