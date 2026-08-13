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

import { describe, expect, it } from 'vitest';

import {
  createDefaultWorkbenchRuntimeSettings,
  createInitialSubmissionV2,
  createRetrySubmissionV2,
  createTurnSubmissionV2,
  normalizeHumanResponseV2,
  type WorkbenchComposerSubmitPayload,
} from '../types';

const payload = (
  overrides: Partial<WorkbenchComposerSubmitPayload> = {},
): WorkbenchComposerSubmitPayload => ({
  message: 'ship it',
  modelType: 123,
  modelName: 'model-a',
  enable_skills: ['skill.b', 'skill.a'],
  enable_mcp: ['mcp.b'],
  enable_kbs: ['kb.b', 'kb.a'],
  enable_databases: ['db.a'],
  runtimeSettings: {
    ...createDefaultWorkbenchRuntimeSettings(),
    skills: {
      enabled: true,
      visibility: 'deferred',
      allowed_skills: ['skill.b', 'skill.a'],
    },
    mcp_tools: {
      enabled: true,
      visibility: 'deferred',
      allowed_tools: ['mcp.b'],
    },
  },
  ...overrides,
});

describe('canonical typed submission v2', () => {
  it.each([
    [
      'enabled-explicit',
      {
        skillsEnabled: true,
        mcpEnabled: true,
        enabledSkills: ['skill.b', 'skill.a'],
        enabledMCP: ['mcp.b'],
      },
    ],
    [
      'enabled-auto',
      {
        skillsEnabled: true,
        mcpEnabled: true,
        enabledSkills: undefined,
        enabledMCP: [],
      },
    ],
    [
      'disabled-configured',
      {
        skillsEnabled: false,
        mcpEnabled: false,
        enabledSkills: [],
        enabledMCP: [],
      },
    ],
  ] as const)(
    'preserves the exact Skill/MCP presence state: %s',
    (_name, state) => {
      const valuePayload = payload({
        enable_skills: state.enabledSkills,
        enable_mcp: [...state.enabledMCP],
      });
      valuePayload.runtimeSettings.skills.enabled = state.skillsEnabled;
      valuePayload.runtimeSettings.mcp_tools.enabled = state.mcpEnabled;
      const value = createInitialSubmissionV2(valuePayload);

      expect(value.composer.explicit_enable_skills).toEqual(
        state.enabledSkills,
      );
      expect(value.composer.enable_mcp).toEqual(state.enabledMCP);
      expect(value.composer.allowed_skills).toEqual(['skill.b', 'skill.a']);
      expect(value.composer.allowed_mcp_tools).toEqual(['mcp.b']);
    },
  );

  it('preserves ordering, decimal IDs and exact optional-object absence', () => {
    const value = createInitialSubmissionV2(payload());

    expect(value).toMatchObject({
      schema_version: 'coze.workbench.initial_run_submission.v2',
      input: { message: 'ship it', uploaded_files: [] },
      composer: {
        model_type: '123',
        model_name: 'model-a',
        explicit_enable_skills: ['skill.b', 'skill.a'],
        allowed_skills: ['skill.b', 'skill.a'],
        enable_mcp: ['mcp.b'],
        enable_kbs: ['kb.b', 'kb.a'],
        enable_databases: ['db.a'],
        allowed_mcp_tools: ['mcp.b'],
      },
      config: {
        runtime: 'eino_adk',
        memory_retrieval: {
          limit: 5,
          candidate_limit: 20,
          scopes: ['thread', 'long_term'],
          min_confidence: 0.2,
        },
        token_usage: { enabled: true },
      },
    });
    expect(value.config).not.toHaveProperty('model_retry');
    expect(value.config).not.toHaveProperty('model_failover');
    expect(value.config).not.toHaveProperty('reasoning');
  });

  it('omits empty failover, stringifies candidates and rejects unsafe IDs', () => {
    const enabledEmpty = payload();
    enabledEmpty.runtimeSettings.model_failover.enabled = true;
    expect(createInitialSubmissionV2(enabledEmpty).config).not.toHaveProperty(
      'model_failover',
    );

    const enabled = payload();
    enabled.runtimeSettings.model_failover = {
      enabled: true,
      candidate_model_ids: [22, 11],
      max_retries: 3,
      failover_empty_output: true,
      failover_finish_reasons: ['length'],
    };
    expect(createInitialSubmissionV2(enabled).config.model_failover).toEqual({
      candidate_model_ids: ['22', '11'],
      max_retries: 2,
      failover_empty_output: true,
      failover_finish_reasons: ['length'],
    });

    expect(() =>
      createInitialSubmissionV2(
        payload({ modelType: Number.MAX_SAFE_INTEGER + 1 }),
      ),
    ).toThrow('模型 ID 无效');
  });

  it('keeps file order for turns and typed lineage only for retries', () => {
    expect(
      createTurnSubmissionV2(payload(), ['9', '3'], 'workbench_new_task'),
    ).toMatchObject({
      kind: 'turn',
      input: {
        message: 'ship it',
        uploaded_files: [{ file_id: '9' }, { file_id: '3' }],
      },
      metadata: { source: 'workbench_new_task' },
    });
    expect(createRetrySubmissionV2(payload(), '77')).toMatchObject({
      kind: 'retry',
      input: { message: 'ship it', uploaded_files: [] },
      lineage: { source_run_id: '77' },
      metadata: { source: 'task_retry' },
    });
  });

  it('normalizes Human response without absent or prohibited fields', () => {
    expect(
      normalizeHumanResponseV2({
        schema: 'coze.human_interaction_response.v1',
        interaction_id: 'interaction-1',
        kind: 'clarification',
        decision: 'answered',
        answer: 'yes',
      }),
    ).toEqual({
      schema: 'coze.human_interaction_response.v1',
      interaction_id: 'interaction-1',
      kind: 'clarification',
      decision: 'answered',
      answer: 'yes',
    });
  });
});
