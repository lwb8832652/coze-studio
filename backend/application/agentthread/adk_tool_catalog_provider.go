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
	"github.com/cloudwego/eino/schema"
	"github.com/eino-contrib/jsonschema"
)

type ADKRuntimeToolVisibility string

const (
	ADKRuntimeToolVisibilityStatic   ADKRuntimeToolVisibility = "static"
	ADKRuntimeToolVisibilityDeferred ADKRuntimeToolVisibility = "deferred"
)

type ADKRuntimeToolCall struct {
	Run       *RunSummary
	Name      string
	Arguments string
}

type ADKRuntimeToolInvoker interface {
	InvokeADKRuntimeTool(
		ctx context.Context,
		call ADKRuntimeToolCall,
	) (string, error)
}

type ADKRuntimeToolInvokerFunc func(
	ctx context.Context,
	call ADKRuntimeToolCall,
) (string, error)

func (f ADKRuntimeToolInvokerFunc) InvokeADKRuntimeTool(
	ctx context.Context,
	call ADKRuntimeToolCall,
) (string, error) {
	if f == nil {
		return "", fmt.Errorf("runtime tool invoker is required")
	}

	return f(ctx, call)
}

type ADKRuntimeToolDefinition struct {
	Name        string
	Description string
	InputSchema string
	Visibility  ADKRuntimeToolVisibility
	Invoker     ADKRuntimeToolInvoker
}

type ADKRuntimeToolCatalog interface {
	LoadADKRuntimeTools(
		ctx context.Context,
		run *RunSummary,
	) ([]ADKRuntimeToolDefinition, error)
}

type ADKRuntimeToolCatalogProvider struct {
	catalog ADKRuntimeToolCatalog
}

func NewADKRuntimeToolCatalogProvider(
	catalog ADKRuntimeToolCatalog,
) *ADKRuntimeToolCatalogProvider {
	return &ADKRuntimeToolCatalogProvider{catalog: catalog}
}

func (p *ADKRuntimeToolCatalogProvider) ResolveToolSet(
	ctx context.Context,
	run *RunSummary,
) (ADKToolSet, error) {
	if p == nil || p.catalog == nil {
		return ADKToolSet{}, fmt.Errorf("eino adk runtime tool catalog is required")
	}
	if run == nil {
		return ADKToolSet{}, fmt.Errorf("run is required")
	}

	definitions, err := p.catalog.LoadADKRuntimeTools(ctx, run)
	if err != nil {
		return ADKToolSet{}, err
	}

	seen := make(map[string]struct{}, len(definitions))
	toolSet := ADKToolSet{}
	runSnapshot := *run
	for _, definition := range definitions {
		definition.Name = strings.TrimSpace(definition.Name)
		definition.Description = strings.TrimSpace(definition.Description)
		if definition.Name == "" {
			return ADKToolSet{}, fmt.Errorf("runtime tool name is required")
		}
		if _, ok := seen[definition.Name]; ok {
			return ADKToolSet{}, fmt.Errorf(
				"duplicate eino adk runtime tool name: %s",
				definition.Name,
			)
		}
		seen[definition.Name] = struct{}{}
		if definition.Invoker == nil {
			return ADKToolSet{}, fmt.Errorf(
				"runtime tool invoker is required: %s",
				definition.Name,
			)
		}

		runtimeTool := &adkRuntimeCatalogTool{
			run:        &runSnapshot,
			definition: definition,
		}
		switch definition.Visibility {
		case "", ADKRuntimeToolVisibilityStatic:
			toolSet.StaticTools = append(toolSet.StaticTools, runtimeTool)
		case ADKRuntimeToolVisibilityDeferred:
			toolSet.DynamicTools = append(toolSet.DynamicTools, runtimeTool)
		default:
			return ADKToolSet{}, fmt.Errorf(
				"unsupported runtime tool visibility %s: %s",
				definition.Visibility,
				definition.Name,
			)
		}
	}

	return toolSet, nil
}

func (p *ADKRuntimeToolCatalogProvider) ResolveTools(
	ctx context.Context,
	run *RunSummary,
) ([]tool.BaseTool, error) {
	set, err := p.ResolveToolSet(ctx, run)
	if err != nil {
		return nil, err
	}

	return set.StaticTools, nil
}

func (p *ADKRuntimeToolCatalogProvider) ResolveDynamicTools(
	ctx context.Context,
	run *RunSummary,
) ([]tool.BaseTool, error) {
	set, err := p.ResolveToolSet(ctx, run)
	if err != nil {
		return nil, err
	}

	return set.DynamicTools, nil
}

type adkRuntimeCatalogTool struct {
	run        *RunSummary
	definition ADKRuntimeToolDefinition
}

func (t *adkRuntimeCatalogTool) Info(
	context.Context,
) (*schema.ToolInfo, error) {
	if t == nil {
		return nil, fmt.Errorf("runtime tool is required")
	}
	params, err := adkRuntimeToolParamsOneOf(t.definition.InputSchema)
	if err != nil {
		return nil, fmt.Errorf(
			"parse runtime tool input schema %s: %w",
			t.definition.Name,
			err,
		)
	}

	return &schema.ToolInfo{
		Name:        t.definition.Name,
		Desc:        t.definition.Description,
		ParamsOneOf: params,
	}, nil
}

func (t *adkRuntimeCatalogTool) InvokableRun(
	ctx context.Context,
	argumentsInJSON string,
	_ ...tool.Option,
) (string, error) {
	if t == nil || t.definition.Invoker == nil {
		return "", fmt.Errorf("runtime tool invoker is required")
	}

	return t.definition.Invoker.InvokeADKRuntimeTool(ctx, ADKRuntimeToolCall{
		Run:       t.run,
		Name:      t.definition.Name,
		Arguments: strings.TrimSpace(argumentsInJSON),
	})
}

func adkRuntimeToolParamsOneOf(
	rawSchema string,
) (*schema.ParamsOneOf, error) {
	rawSchema = strings.TrimSpace(rawSchema)
	if rawSchema == "" {
		return nil, nil
	}

	parsed := &jsonschema.Schema{}
	if err := json.Unmarshal([]byte(rawSchema), parsed); err != nil {
		return nil, err
	}

	return schema.NewParamsOneOfByJSONSchema(parsed), nil
}
