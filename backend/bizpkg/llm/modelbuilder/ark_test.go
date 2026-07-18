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

package modelbuilder

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coze-dev/coze-studio/backend/api/model/admin/config"
	"github.com/coze-dev/coze-studio/backend/pkg/logs"
)

func TestArkModelBuilderDoesNotLogCredentials(t *testing.T) {
	var output bytes.Buffer
	logs.SetOutput(&output)
	logs.SetLevel(logs.LevelDebug)
	t.Cleanup(func() {
		logs.SetLevel(logs.LevelInfo)
		logs.SetOutput(os.Stderr)
	})

	builder := newArkModelBuilder(&config.Model{
		Connection: &config.Connection{
			BaseConnInfo: &config.BaseConnectionInfo{
				APIKey: "ark-sensitive-marker",
				Model:  "test-model",
			},
		},
	})

	_, err := builder.Build(context.Background(), nil)
	require.NoError(t, err)
	require.NotContains(t, output.String(), "ark-sensitive-marker")
}
