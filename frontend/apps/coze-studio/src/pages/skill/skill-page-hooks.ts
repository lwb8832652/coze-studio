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

import { useCallback, useEffect, useState } from 'react';

import { workbenchSkill } from '@coze-studio/api-schema';

import {
  createSkill,
  deleteSkill,
  importSkill,
  listSkills,
  testRunSkill,
  updateSkill,
} from './service';

type Skill = workbenchSkill.Skill;

const TEST_INPUT = JSON.stringify({ message: 'ping' });
const DEFAULT_SCHEMA = '{}';
const DEFAULT_EXECUTOR = '{}';
const DEFAULT_PERMISSIONS = '{"network":false,"allowed_tools":[]}';

const skillToUpdateRequest = (
  skill: Skill,
  enabled: boolean,
): workbenchSkill.UpdateSkillRequest => ({
  id: skill.id,
  space_id: skill.space_id,
  name: skill.name,
  description: skill.description,
  type: skill.type,
  version: skill.version,
  enabled,
  input_schema: skill.input_schema,
  output_schema: skill.output_schema,
  executor: skill.executor,
  permissions: skill.permissions,
});

const readSkillArchiveContent = async (file: File) => {
  const bytes = new Uint8Array(await file.arrayBuffer());
  let binary = '';

  bytes.forEach(byte => {
    binary += String.fromCharCode(byte);
  });

  return `base64:${window.btoa(binary)}`;
};

export const useSkillListLoader = (spaceId?: string) => {
  const [skills, setSkills] = useState<Skill[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  const loadSkills = useCallback(async () => {
    if (!spaceId) {
      return;
    }

    setLoading(true);
    setError('');

    try {
      const response = await listSkills({ space_id: spaceId });
      setSkills(response.data?.skills ?? []);
    } catch (err) {
      setError(err instanceof Error ? err.message : '加载技能失败');
    } finally {
      setLoading(false);
    }
  }, [spaceId]);

  useEffect(() => {
    void loadSkills();
  }, [loadSkills]);

  return {
    error,
    loadSkills,
    loading,
    setError,
    skills,
  };
};

export const useSkillImportActions = ({
  loadSkills,
  setError,
  spaceId,
}: {
  loadSkills: () => Promise<void>;
  setError: (value: string) => void;
  spaceId?: string;
}) => {
  const [fileName, setFileName] = useState('skill.json');
  const [content, setContent] = useState('');
  const [archiveContent, setArchiveContent] = useState('');
  const [archiveLabel, setArchiveLabel] = useState('');
  const [importing, setImporting] = useState(false);

  const handleContentChange = (value: string) => {
    setContent(value);
    setArchiveContent('');
    setArchiveLabel('');
  };

  const handleImport = async () => {
    const importContent = archiveContent || content;
    if (!spaceId || !importContent.trim() || importing) {
      return;
    }

    setImporting(true);
    setError('');

    try {
      await importSkill({
        space_id: spaceId,
        file_name: fileName.trim() || 'skill.json',
        content: importContent,
      });
      handleContentChange('');
      await loadSkills();
    } catch (err) {
      setError(err instanceof Error ? err.message : '导入技能失败');
    } finally {
      setImporting(false);
    }
  };

  const handleImportFile = async (file: File) => {
    setError('');
    setFileName(file.name);

    try {
      if (file.name.toLowerCase().endsWith('.skill')) {
        setContent('');
        setArchiveContent(await readSkillArchiveContent(file));
        setArchiveLabel(`已载入 ${file.name}，${file.size} B`);
        return;
      }

      setContent(await file.text());
      setArchiveContent('');
      setArchiveLabel('');
    } catch (err) {
      setArchiveContent('');
      setArchiveLabel('');
      setError(err instanceof Error ? err.message : '读取技能文件失败');
    }
  };

  return {
    archiveContent,
    archiveLabel,
    content,
    fileName,
    handleContentChange,
    handleImport,
    handleImportFile,
    importing,
    setFileName,
  };
};

export const useSkillCreateActions = ({
  loadSkills,
  setError,
  spaceId,
}: {
  loadSkills: () => Promise<void>;
  setError: (value: string) => void;
  spaceId?: string;
}) => {
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [creating, setCreating] = useState(false);

  const handleCreate = async () => {
    const trimmedName = name.trim();
    const trimmedDescription = description.trim();
    if (!spaceId || !trimmedName || !trimmedDescription || creating) {
      return;
    }

    setCreating(true);
    setError('');

    try {
      const createRequest = {
        space_id: spaceId,
        name: trimmedName,
        description: trimmedDescription,
        type: workbenchSkill.SkillType.CustomSkill,
        version: '1.0.0',
        enabled: true,
        input_schema: DEFAULT_SCHEMA,
        output_schema: DEFAULT_SCHEMA,
        executor: DEFAULT_EXECUTOR,
        permissions: DEFAULT_PERMISSIONS,
      };
      await createSkill(createRequest);
      setName('');
      setDescription('');
      await loadSkills();
    } catch (err) {
      setError(err instanceof Error ? err.message : '创建技能失败');
    } finally {
      setCreating(false);
    }
  };

  return {
    creating,
    description,
    handleCreate,
    name,
    setDescription,
    setName,
  };
};

export const useSkillRunActions = ({
  loadSkills,
  setError,
}: {
  loadSkills: () => Promise<void>;
  setError: (value: string) => void;
}) => {
  const [runningSkillId, setRunningSkillId] = useState('');
  const [updatingSkillId, setUpdatingSkillId] = useState('');
  const [deletingSkillId, setDeletingSkillId] = useState('');
  const [testResults, setTestResults] = useState<Record<string, string>>({});

  const handleTestRun = async (skill: Skill) => {
    if (runningSkillId || updatingSkillId || deletingSkillId) {
      return;
    }

    setRunningSkillId(skill.id);
    setError('');
    setTestResults(current => {
      const rest = { ...current };
      delete rest[skill.id];

      return rest;
    });

    try {
      const response = await testRunSkill({
        skill_id: skill.id,
        input: TEST_INPUT,
      });
      const output =
        response.data?.output ??
        (response.data?.task_id
          ? `已创建任务：${response.data.task_id}`
          : '运行成功');

      setTestResults(current => ({
        ...current,
        [skill.id]: output,
      }));
    } catch (err) {
      setError(err instanceof Error ? err.message : '试运行失败');
    } finally {
      setRunningSkillId('');
    }
  };

  const handleToggleEnabled = async (skill: Skill) => {
    if (runningSkillId || updatingSkillId || deletingSkillId) {
      return;
    }

    setUpdatingSkillId(skill.id);
    setError('');

    try {
      await updateSkill(skillToUpdateRequest(skill, !skill.enabled));
      await loadSkills();
    } catch (err) {
      setError(err instanceof Error ? err.message : '更新技能状态失败');
    } finally {
      setUpdatingSkillId('');
    }
  };

  const handleDeleteSkill = async (skill: Skill) => {
    if (
      runningSkillId ||
      updatingSkillId ||
      deletingSkillId ||
      !window.confirm(`确认删除技能 ${skill.name}？`)
    ) {
      return;
    }

    setDeletingSkillId(skill.id);
    setError('');

    try {
      await deleteSkill({ skill_id: skill.id });
      setTestResults(current => {
        const rest = { ...current };
        delete rest[skill.id];

        return rest;
      });
      await loadSkills();
    } catch (err) {
      setError(err instanceof Error ? err.message : '删除技能失败');
    } finally {
      setDeletingSkillId('');
    }
  };

  return {
    deletingSkillId,
    handleDeleteSkill,
    handleTestRun,
    handleToggleEnabled,
    runningSkillId,
    testResults,
    updatingSkillId,
  };
};
