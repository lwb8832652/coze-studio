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
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/tool"
)

type ADKToolPolicyProvider struct {
	base ADKToolProvider
}

func NewADKToolPolicyProvider(base ADKToolProvider) *ADKToolPolicyProvider {
	return &ADKToolPolicyProvider{base: base}
}

func (p *ADKToolPolicyProvider) ResolveToolSet(
	ctx context.Context,
	run *RunSummary,
) (ADKToolSet, error) {
	set, err := p.resolveBaseToolSet(ctx, run)
	if err != nil {
		return ADKToolSet{}, err
	}

	policy, err := adkToolPolicyFromRun(run)
	if err != nil {
		return ADKToolSet{}, err
	}
	if !policy.Enabled {
		return set, nil
	}

	set.StaticTools, err = filterADKToolsByAllowedNames(
		ctx,
		set.StaticTools,
		policy.StaticAllowed,
	)
	if err != nil {
		return ADKToolSet{}, err
	}
	staticNames, err := adkToolNameSet(ctx, set.StaticTools)
	if err != nil {
		return ADKToolSet{}, err
	}
	set.SubagentToolNames = filterADKSubagentToolNames(
		set.SubagentToolNames,
		staticNames,
	)
	set.DynamicTools, err = filterADKToolsByAllowedNames(
		ctx,
		set.DynamicTools,
		policy.DynamicAllowed,
	)
	if err != nil {
		return ADKToolSet{}, err
	}

	return set, nil
}

func filterADKSubagentToolNames(
	names []string,
	activeStaticNames map[string]struct{},
) []string {
	if len(names) == 0 {
		return nil
	}
	filtered := make([]string, 0, len(names))
	seen := make(map[string]struct{}, len(names))
	for _, rawName := range names {
		name := strings.TrimSpace(rawName)
		if _, ok := activeStaticNames[name]; !ok {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		filtered = append(filtered, name)
	}
	return filtered
}

func (p *ADKToolPolicyProvider) ResolveTools(
	ctx context.Context,
	run *RunSummary,
) ([]tool.BaseTool, error) {
	set, err := p.ResolveToolSet(ctx, run)
	if err != nil {
		return nil, err
	}
	return set.StaticTools, nil
}

func (p *ADKToolPolicyProvider) ResolveDynamicTools(
	ctx context.Context,
	run *RunSummary,
) ([]tool.BaseTool, error) {
	set, err := p.ResolveToolSet(ctx, run)
	if err != nil {
		return nil, err
	}
	return set.DynamicTools, nil
}

func (p *ADKToolPolicyProvider) resolveBaseToolSet(
	ctx context.Context,
	run *RunSummary,
) (ADKToolSet, error) {
	if p == nil || p.base == nil {
		return ADKToolSet{}, nil
	}
	if toolSetProvider, ok := p.base.(ADKToolSetProvider); ok {
		return toolSetProvider.ResolveToolSet(ctx, run)
	}
	staticTools, err := p.base.ResolveTools(ctx, run)
	if err != nil {
		return ADKToolSet{}, err
	}
	set := ADKToolSet{StaticTools: staticTools}
	if dynamicProvider, ok := p.base.(ADKDynamicToolProvider); ok {
		set.DynamicTools, err = dynamicProvider.ResolveDynamicTools(ctx, run)
		if err != nil {
			return ADKToolSet{}, err
		}
	}
	return set, nil
}

type adkToolPolicy struct {
	Enabled        bool
	StaticAllowed  map[string]struct{}
	DynamicAllowed map[string]struct{}
}

func adkToolPolicyFromRun(run *RunSummary) (adkToolPolicy, error) {
	if run == nil || strings.TrimSpace(run.Config) == "" {
		return adkToolPolicy{}, nil
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(run.Config), &payload); err != nil {
		return adkToolPolicy{}, fmt.Errorf("parse tool policy config: %w", err)
	}
	raw, ok := firstADKConfigObject(payload, "tool_policy", "toolPolicy")
	if !ok {
		return adkToolPolicy{}, nil
	}

	policy := adkToolPolicy{Enabled: true}
	shared, sharedSet := configStringSetWithPresence(
		raw,
		"allowed_tools",
		"allowedTools",
	)
	if sharedSet {
		policy.StaticAllowed = shared
		policy.DynamicAllowed = shared
	}
	static, staticSet := configStringSetWithPresence(
		raw,
		"allowed_static_tools",
		"allowedStaticTools",
	)
	if staticSet {
		policy.StaticAllowed = static
	}
	dynamic, dynamicSet := configStringSetWithPresence(
		raw,
		"allowed_dynamic_tools",
		"allowedDynamicTools",
	)
	if dynamicSet {
		policy.DynamicAllowed = dynamic
	}
	if policy.StaticAllowed == nil {
		policy.StaticAllowed = map[string]struct{}{}
	}
	if policy.DynamicAllowed == nil {
		policy.DynamicAllowed = policy.StaticAllowed
	}

	return policy, nil
}

func configStringSetWithPresence(
	payload map[string]any,
	keys ...string,
) (map[string]struct{}, bool) {
	for _, key := range keys {
		value, ok := payload[key]
		if !ok {
			continue
		}
		items := normalizeConfigStringSlice(configStringSlice(value))
		set := make(map[string]struct{}, len(items))
		for _, item := range items {
			set[item] = struct{}{}
		}
		return set, true
	}
	return nil, false
}

func filterADKToolsByAllowedNames(
	ctx context.Context,
	tools []tool.BaseTool,
	allowed map[string]struct{},
) ([]tool.BaseTool, error) {
	if allowed == nil {
		return tools, nil
	}
	filtered := make([]tool.BaseTool, 0, len(tools))
	for _, candidate := range tools {
		name, err := adkToolName(ctx, candidate)
		if err != nil {
			return nil, err
		}
		if _, ok := allowed[name]; ok {
			filtered = append(filtered, candidate)
		}
	}
	return filtered, nil
}
