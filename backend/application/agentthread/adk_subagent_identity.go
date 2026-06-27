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

package agentthread

import (
	"encoding/json"
	"strings"
)

func adkSubagentIdentityFromRunMetadata(run *RunSummary) *adkSubagentPayload {
	if run == nil || strings.TrimSpace(run.Metadata) == "" {
		return nil
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(run.Metadata), &payload); err != nil {
		return nil
	}
	raw, ok := configObject(payload["subagent"])
	if !ok {
		return nil
	}

	runPath := firstConfigStringSlice(raw, "run_path", "runPath")
	name := firstConfigString(raw, "name")
	if name == "" && len(runPath) > 0 {
		name = runPath[len(runPath)-1]
	}
	stepID := firstConfigString(raw, "step_id", "stepId")
	if stepID == "" && len(runPath) > 0 {
		stepID = strings.Join(runPath, "/")
	}
	if name == "" || stepID == "" {
		return nil
	}

	rootName := firstConfigString(raw, "root_name", "rootName")
	if rootName == "" && len(runPath) > 0 {
		rootName = runPath[0]
	}
	parentName := firstConfigString(raw, "parent_name", "parentName")
	if parentName == "" && len(runPath) > 1 {
		parentName = runPath[len(runPath)-2]
	}
	depth := int(firstConfigInt64(raw, "depth"))
	if depth == 0 && len(runPath) > 1 {
		depth = len(runPath) - 1
	}

	return &adkSubagentPayload{
		Name:       name,
		RootName:   rootName,
		ParentName: parentName,
		StepID:     stepID,
		RunPath:    append([]string(nil), runPath...),
		Depth:      depth,
	}
}

func adkChildSubagentIdentity(
	parent *RunSummary,
	childName string,
) *adkSubagentPayload {
	childName = strings.TrimSpace(childName)
	if childName == "" {
		return nil
	}

	runPath := []string{}
	if parentIdentity := adkSubagentIdentityFromRunMetadata(parent); parentIdentity != nil &&
		len(parentIdentity.RunPath) > 0 {
		runPath = append(runPath, parentIdentity.RunPath...)
	} else {
		runPath = append(runPath, adkRootAgentNameFromRun(parent))
	}
	runPath = append(runPath, childName)

	return &adkSubagentPayload{
		Name:       childName,
		RootName:   runPath[0],
		ParentName: runPath[len(runPath)-2],
		StepID:     strings.Join(runPath, "/"),
		RunPath:    runPath,
		Depth:      len(runPath) - 1,
	}
}

func adkRootAgentNameFromRun(run *RunSummary) string {
	if run != nil {
		cfg, err := parseModelExecutorConfig(run.Config)
		if err == nil && strings.TrimSpace(cfg.AgentName) != "" {
			return strings.TrimSpace(cfg.AgentName)
		}
	}
	return defaultADKAgentName
}
