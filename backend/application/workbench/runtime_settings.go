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

package workbench

import (
	"encoding/json"
	"strings"

	chatapi "github.com/coze-dev/coze-studio/backend/api/model/workbench/chat"
)

const maxWorkbenchRuntimeSettingsBytes = 64 << 10

func workbenchRuntimeSettingsPayload(req *chatapi.WorkbenchChatRequest) (map[string]any, bool, error) {
	if req == nil || !req.IsSetRuntimeSettings() {
		return nil, false, nil
	}
	raw := strings.TrimSpace(req.GetRuntimeSettings())
	if raw == "" {
		return nil, false, nil
	}
	if len(raw) > maxWorkbenchRuntimeSettingsBytes {
		return nil, false, InvalidArgumentErrorf("runtime_settings is too large")
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil, false, InvalidArgumentErrorf("runtime_settings must be a JSON object")
	}
	if payload == nil {
		return nil, false, InvalidArgumentErrorf("runtime_settings must be a JSON object")
	}

	return payload, true, nil
}

func mustJSONAny(payload map[string]any) string {
	bytes, _ := json.Marshal(payload)
	return string(bytes)
}
