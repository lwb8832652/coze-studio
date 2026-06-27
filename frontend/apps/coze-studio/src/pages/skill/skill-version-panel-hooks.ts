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

import { useCallback, useEffect, useMemo, useState } from 'react';
import type { Dispatch, SetStateAction } from 'react';

import type { workbenchSkill } from '@coze-studio/api-schema';

import {
  decodeBase64Text,
  downloadVersionArchive,
  encodeBase64Text,
  resourceIsText,
} from './skill-version-panel-utils';
import {
  exportSkillVersion,
  listSkillVersionResources,
  listSkillVersions,
  rollbackSkillVersion,
  updateSkill,
  updateSkillVersionContent,
  updateSkillVersionResource,
} from './service';

type Skill = workbenchSkill.Skill;
type SkillResource = workbenchSkill.SkillResource;
type SkillVersion = workbenchSkill.SkillVersion;

const skillToMetadataUpdateRequest = (
  skill: Skill,
  values: {
    description: string;
    name: string;
    version: string;
  },
): workbenchSkill.UpdateSkillRequest => ({
  id: skill.id,
  space_id: skill.space_id,
  name: values.name,
  description: values.description,
  type: skill.type,
  version: values.version,
  enabled: skill.enabled,
  input_schema: skill.input_schema,
  output_schema: skill.output_schema,
  executor: skill.executor,
  permissions: skill.permissions,
});

export const useSkillVersionData = (skill: Skill) => {
  const [versions, setVersions] = useState<SkillVersion[]>([]);
  const [selectedVersionId, setSelectedVersionId] = useState('');
  const [resources, setResources] = useState<SkillResource[]>([]);
  const [selectedResourcePath, setSelectedResourcePath] = useState('');
  const [skillMD, setSkillMD] = useState('');
  const [resourceContent, setResourceContent] = useState('');
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');

  const selectedVersion = useMemo(
    () => versions.find(version => version.id === selectedVersionId),
    [selectedVersionId, versions],
  );
  const selectedResource = useMemo(
    () => resources.find(resource => resource.path === selectedResourcePath),
    [resources, selectedResourcePath],
  );
  const selectedResourceIsText = resourceIsText(selectedResource);

  const loadResources = useCallback(
    async (versionID: string) => {
      const response = await listSkillVersionResources({
        skill_id: skill.id,
        version_id: versionID,
      });
      const nextResources = response.data?.resources ?? [];
      setResources(nextResources);
      setSelectedResourcePath(current =>
        nextResources.some(resource => resource.path === current)
          ? current
          : (nextResources[0]?.path ?? ''),
      );
    },
    [skill.id],
  );

  const loadVersions = useCallback(
    async (preferredVersionID?: string) => {
      setLoading(true);
      setError('');

      try {
        const response = await listSkillVersions({ skill_id: skill.id });
        const nextVersions = response.data?.versions ?? [];
        const nextVersionID =
          preferredVersionID &&
          nextVersions.some(version => version.id === preferredVersionID)
            ? preferredVersionID
            : (nextVersions[0]?.id ?? '');

        setVersions(nextVersions);
        setSelectedVersionId(nextVersionID);
        setSkillMD(
          nextVersions.find(version => version.id === nextVersionID)
            ?.skill_md ?? '',
        );
        if (nextVersionID) {
          await loadResources(nextVersionID);
        } else {
          setResources([]);
          setSelectedResourcePath('');
        }
      } catch (err) {
        setError(err instanceof Error ? err.message : '加载技能版本失败');
      } finally {
        setLoading(false);
      }
    },
    [loadResources, skill.id],
  );

  useEffect(() => {
    void loadVersions();
  }, [loadVersions]);

  useEffect(() => {
    if (!selectedResource) {
      setResourceContent('');
      return;
    }

    setResourceContent(
      selectedResourceIsText
        ? decodeBase64Text(selectedResource.content_base64)
        : selectedResource.content_base64,
    );
  }, [selectedResource, selectedResourceIsText]);

  const handleSelectVersion = async (version: SkillVersion) => {
    setSelectedVersionId(version.id);
    setSkillMD(version.skill_md);
    setNotice('');
    setError('');

    try {
      await loadResources(version.id);
    } catch (err) {
      setError(err instanceof Error ? err.message : '加载版本资源失败');
    }
  };

  return {
    error,
    handleSelectVersion,
    loadVersions,
    loading,
    notice,
    resourceContent,
    resources,
    saving,
    selectedResource,
    selectedResourceIsText,
    selectedResourcePath,
    selectedVersion,
    selectedVersionId,
    setError,
    setNotice,
    setResourceContent,
    setSaving,
    setSelectedResourcePath,
    setSkillMD,
    skillMD,
    versions,
  };
};

interface SkillMetadataActionParams {
  onSkillChanged: () => void | Promise<void>;
  saving: boolean;
  setError: (value: string) => void;
  setNotice: (value: string) => void;
  setSaving: Dispatch<SetStateAction<boolean>>;
  skill: Skill;
}

export const useSkillMetadataActions = ({
  onSkillChanged,
  saving,
  setError,
  setNotice,
  setSaving,
  skill,
}: SkillMetadataActionParams) => {
  const [name, setName] = useState(skill.name);
  const [description, setDescription] = useState(skill.description);
  const [version, setVersion] = useState(skill.version);

  useEffect(() => {
    setName(skill.name);
    setDescription(skill.description);
    setVersion(skill.version);
  }, [skill.description, skill.id, skill.name, skill.version]);

  const handleSaveMetadata = async () => {
    const trimmedName = name.trim();
    const trimmedDescription = description.trim();
    const trimmedVersion = version.trim();
    if (!trimmedName || !trimmedDescription || !trimmedVersion || saving) {
      return;
    }

    setSaving(true);
    setError('');
    setNotice('');
    try {
      await updateSkill(
        skillToMetadataUpdateRequest(skill, {
          description: trimmedDescription,
          name: trimmedName,
          version: trimmedVersion,
        }),
      );
      await onSkillChanged();
      setNotice('基础信息已保存');
    } catch (err) {
      setError(err instanceof Error ? err.message : '保存基础信息失败');
    } finally {
      setSaving(false);
    }
  };

  return {
    description,
    handleSaveMetadata,
    name,
    setDescription,
    setName,
    setVersion,
    version,
  };
};

interface SkillVersionActionParams {
  loadVersions: (preferredVersionID?: string) => Promise<void>;
  onSkillChanged: () => void | Promise<void>;
  resourceContent: string;
  saving: boolean;
  selectedResource?: SkillResource;
  selectedResourceIsText: boolean;
  selectedVersion?: SkillVersion;
  setError: (value: string) => void;
  setNotice: (value: string) => void;
  setSaving: Dispatch<SetStateAction<boolean>>;
  skill: Skill;
  skillMD: string;
}

export const useSkillVersionActions = ({
  loadVersions,
  onSkillChanged,
  resourceContent,
  saving,
  selectedResource,
  selectedResourceIsText,
  selectedVersion,
  setError,
  setNotice,
  setSaving,
  skill,
  skillMD,
}: SkillVersionActionParams) => {
  const handleSaveSkillMD = async () => {
    if (!selectedVersion || !skillMD.trim() || saving) {
      return;
    }

    setSaving(true);
    setError('');
    setNotice('');
    try {
      const response = await updateSkillVersionContent({
        skill_id: skill.id,
        version_id: selectedVersion.id,
        skill_md: skillMD,
      });
      await loadVersions(response.data?.id);
      await onSkillChanged();
      setNotice('SKILL.md 已保存为新版本快照');
    } catch (err) {
      setError(err instanceof Error ? err.message : '保存 SKILL.md 失败');
    } finally {
      setSaving(false);
    }
  };

  const handleSaveResource = async () => {
    if (!selectedVersion || !selectedResource || saving) {
      return;
    }

    const resourcePath = selectedResource.path;
    setSaving(true);
    setError('');
    setNotice('');
    try {
      const response = await updateSkillVersionResource({
        skill_id: skill.id,
        version_id: selectedVersion.id,
        path: resourcePath,
        content_base64: selectedResourceIsText
          ? encodeBase64Text(resourceContent)
          : resourceContent.trim(),
      });
      await loadVersions(response.data?.id);
      await onSkillChanged();
      setNotice(`${resourcePath} 已保存为新版本快照`);
    } catch (err) {
      setError(err instanceof Error ? err.message : '保存资源失败');
    } finally {
      setSaving(false);
    }
  };

  const handleRollback = async () => {
    if (
      !selectedVersion ||
      saving ||
      !window.confirm(`确认回滚到版本 ${selectedVersion.version}？`)
    ) {
      return;
    }

    setSaving(true);
    setError('');
    setNotice('');
    try {
      await rollbackSkillVersion({
        skill_id: skill.id,
        version_id: selectedVersion.id,
      });
      await loadVersions();
      await onSkillChanged();
      setNotice(`已回滚到版本 ${selectedVersion.version}`);
    } catch (err) {
      setError(err instanceof Error ? err.message : '回滚技能版本失败');
    } finally {
      setSaving(false);
    }
  };

  const handleExport = async () => {
    if (!selectedVersion || saving) {
      return;
    }

    setSaving(true);
    setError('');
    setNotice('');
    try {
      const response = await exportSkillVersion({
        skill_id: skill.id,
        version_id: selectedVersion.id,
      });
      if (!response.data) {
        throw new Error('导出响应缺少文件内容');
      }
      downloadVersionArchive(
        response.data.content_base64,
        response.data.content_type,
        response.data.file_name,
      );
      setNotice('技能包已开始下载');
    } catch (err) {
      setError(err instanceof Error ? err.message : '导出技能版本失败');
    } finally {
      setSaving(false);
    }
  };

  return {
    handleExport,
    handleRollback,
    handleSaveResource,
    handleSaveSkillMD,
  };
};
