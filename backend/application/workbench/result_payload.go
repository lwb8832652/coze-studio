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

import "encoding/json"

const (
	resultTypeAnswer     = "answer"
	resultTypeAgentTrace = "agent_trace"
	resultTypeReport     = "report"
	executionTypeArk     = "Ark"
	executionTypeAgent   = "Agent"
)

type resultPayload struct {
	Message          string   `json:"message"`
	ResultType       string   `json:"result_type"`
	ExecutionType    string   `json:"execution_type,omitempty"`
	RetrievalSources []string `json:"retrieval_sources,omitempty"`
}

func marshalResultPayload(payload resultPayload) (string, error) {
	bytes, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}
