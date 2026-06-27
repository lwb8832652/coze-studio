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

import { useEffect, useState } from 'react';
import type { Dispatch, SetStateAction } from 'react';

import type { workbenchSkill } from '@coze-studio/api-schema';

import { listSkillToolCandidates, updateSkill } from './service';

type Skill = workbenchSkill.Skill;
type SkillToolCandidate = workbenchSkill.SkillToolCandidate;

const allowedToolNamePattern = /^[A-Za-z_][A-Za-z0-9_]{0,63}$/;

const skillToPermissionUpdateRequest = (
  skill: Skill,
  permissions: string,
): workbenchSkill.UpdateSkillRequest => ({
  id: skill.id,
  space_id: skill.space_id,
  name: skill.name,
  description: skill.description,
  type: skill.type,
  version: skill.version,
  enabled: skill.enabled,
  input_schema: skill.input_schema,
  output_schema: skill.output_schema,
  executor: skill.executor,
  permissions,
});

const parsePermissionRecord = (value: string): Record<string, unknown> => {
  try {
    const parsed: unknown = JSON.parse(value || '{}');
    if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
      return parsed as Record<string, unknown>;
    }
  } catch {
    return {};
  }

  return {};
};

const permissionNetworkEnabled = (value: string) =>
  parsePermissionRecord(value).network === true;

const allowedToolsFromPermissions = (value: string): string[] => {
  const allowedTools = parsePermissionRecord(value).allowed_tools;
  if (!Array.isArray(allowedTools)) {
    return [];
  }

  return allowedTools.filter(
    (tool): tool is string => typeof tool === 'string' && tool.trim() !== '',
  );
};

const normalizeAllowedTools = (value: string): string[] => {
  const seen = new Set<string>();
  const result: string[] = [];
  const names = value
    .split(/[\n,]/)
    .map(name => name.trim())
    .filter(Boolean);

  for (const name of names) {
    if (!allowedToolNamePattern.test(name)) {
      throw new Error(
        '工具名称只能包含字母、数字和下划线，且必须以字母或下划线开头',
      );
    }
    if (!seen.has(name)) {
      seen.add(name);
      result.push(name);
    }
  }

  return result;
};

const serializePermissions = (
  current: string,
  values: {
    allowedToolsText: string;
    network: boolean;
  },
) =>
  JSON.stringify({
    ...parsePermissionRecord(current),
    network: values.network,
    allowed_tools: normalizeAllowedTools(values.allowedToolsText),
  });

const toggleAllowedToolText = (value: string, toolName: string): string => {
  if (!allowedToolNamePattern.test(toolName)) {
    throw new Error(
      '工具名称只能包含字母、数字和下划线，且必须以字母或下划线开头',
    );
  }
  const tools = normalizeAllowedTools(value);
  const nextTools = tools.includes(toolName)
    ? tools.filter(name => name !== toolName)
    : [...tools, toolName];

  return nextTools.join('\n');
};

interface SkillPermissionActionParams {
  onSkillChanged: () => void | Promise<void>;
  saving: boolean;
  setError: (value: string) => void;
  setNotice: (value: string) => void;
  setSaving: Dispatch<SetStateAction<boolean>>;
  skill: Skill;
}

export const useSkillPermissionActions = ({
  onSkillChanged,
  saving,
  setError,
  setNotice,
  setSaving,
  skill,
}: SkillPermissionActionParams) => {
  const [network, setNetwork] = useState(
    permissionNetworkEnabled(skill.permissions),
  );
  const [allowedToolsText, setAllowedToolsText] = useState(
    allowedToolsFromPermissions(skill.permissions).join('\n'),
  );
  const [toolCandidates, setToolCandidates] = useState<SkillToolCandidate[]>(
    [],
  );

  useEffect(() => {
    setNetwork(permissionNetworkEnabled(skill.permissions));
    setAllowedToolsText(
      allowedToolsFromPermissions(skill.permissions).join('\n'),
    );
  }, [skill.id, skill.permissions]);

  useEffect(() => {
    let canceled = false;

    const loadToolCandidates = async () => {
      try {
        const response = await listSkillToolCandidates({
          space_id: skill.space_id,
        });
        if (!canceled) {
          setToolCandidates(response.data?.tools ?? []);
        }
      } catch (err) {
        if (!canceled) {
          setToolCandidates([]);
          setError(err instanceof Error ? err.message : '加载工具候选失败');
        }
      }
    };

    void loadToolCandidates();

    return () => {
      canceled = true;
    };
  }, [setError, skill.space_id]);

  const handleToggleToolCandidate = (toolName: string) => {
    try {
      setAllowedToolsText(current => toggleAllowedToolText(current, toolName));
      setError('');
    } catch (err) {
      setNotice('');
      setError(err instanceof Error ? err.message : '权限配置无效');
    }
  };

  const handleSavePermissions = async () => {
    if (saving) {
      return;
    }

    let permissions: string;
    try {
      permissions = serializePermissions(skill.permissions, {
        allowedToolsText,
        network,
      });
    } catch (err) {
      setNotice('');
      setError(err instanceof Error ? err.message : '权限配置无效');
      return;
    }

    setSaving(true);
    setError('');
    setNotice('');
    try {
      await updateSkill(skillToPermissionUpdateRequest(skill, permissions));
      await onSkillChanged();
      setNotice('权限配置已保存');
    } catch (err) {
      setError(err instanceof Error ? err.message : '保存权限配置失败');
    } finally {
      setSaving(false);
    }
  };

  return {
    allowedToolsText,
    handleSavePermissions,
    handleToggleToolCandidate,
    network,
    setAllowedToolsText,
    setNetwork,
    toolCandidates,
  };
};
