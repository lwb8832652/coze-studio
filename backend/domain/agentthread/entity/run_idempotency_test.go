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

package entity

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRunIdempotencyContractPreservesMetadataAndValidatesReplay(t *testing.T) {
	const operation = "workbench.run.turn.v1"
	fingerprint := strings.Repeat("a", 64)
	metadata, err := MergeRunIdempotencyContract(
		`{"large_id":9007199254740993}`,
		operation,
		fingerprint,
	)
	require.NoError(t, err)
	require.Contains(t, metadata, `"large_id":9007199254740993`)
	require.NoError(t, ValidateRunIdempotencyValues(metadata, operation, fingerprint))
	require.ErrorIs(
		t,
		ValidateRunIdempotencyValues(metadata, operation, strings.Repeat("b", 64)),
		ErrRunIdempotencyConflict,
	)
}

func TestRunIdempotencyContractKeepsLegacyMetadataUntouched(t *testing.T) {
	const metadata = ` {"source":"legacy"} `
	got, err := MergeRunIdempotencyContract(metadata, "", "")
	require.NoError(t, err)
	require.Equal(t, metadata, got)
	require.NoError(t, ValidateRunIdempotencyValues(metadata, "", ""))
}
