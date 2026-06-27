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

import "context"

type ADKCompositeRuntimeToolCatalog struct {
	catalogs []ADKRuntimeToolCatalog
}

func NewADKCompositeRuntimeToolCatalog(
	catalogs ...ADKRuntimeToolCatalog,
) *ADKCompositeRuntimeToolCatalog {
	filtered := make([]ADKRuntimeToolCatalog, 0, len(catalogs))
	for _, catalog := range catalogs {
		if catalog != nil {
			filtered = append(filtered, catalog)
		}
	}

	return &ADKCompositeRuntimeToolCatalog{catalogs: filtered}
}

func (c *ADKCompositeRuntimeToolCatalog) LoadADKRuntimeTools(
	ctx context.Context,
	run *RunSummary,
) ([]ADKRuntimeToolDefinition, error) {
	if c == nil || len(c.catalogs) == 0 {
		return nil, nil
	}

	definitions := make([]ADKRuntimeToolDefinition, 0)
	for _, catalog := range c.catalogs {
		loaded, err := catalog.LoadADKRuntimeTools(ctx, run)
		if err != nil {
			return nil, err
		}
		definitions = append(definitions, loaded...)
	}

	return definitions, nil
}
