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

import type { workbenchSkill } from '@coze-studio/api-schema';
import { SideSheet } from '@coze-arch/coze-design';

import {
  SkillMetadataEditor,
  SkillPermissionEditor,
  SkillVersionList,
  SkillVersionPanelBanners,
  SkillVersionWorkspace,
} from './skill-version-panel-sections';
import {
  useSkillMetadataActions,
  useSkillVersionActions,
  useSkillVersionData,
} from './skill-version-panel-hooks';
import { useSkillPermissionActions } from './skill-permission-hooks';

type Skill = workbenchSkill.Skill;

interface SkillVersionPanelProps {
  skill: Skill;
  onClose: () => void;
  onSkillChanged: () => void | Promise<void>;
}

export const SkillVersionPanel = ({
  skill,
  onClose,
  onSkillChanged,
}: SkillVersionPanelProps) => {
  const versionState = useSkillVersionData(skill);
  const metadataActions = useSkillMetadataActions({
    onSkillChanged,
    saving: versionState.saving,
    setError: versionState.setError,
    setNotice: versionState.setNotice,
    setSaving: versionState.setSaving,
    skill,
  });
  const permissionActions = useSkillPermissionActions({
    onSkillChanged,
    saving: versionState.saving,
    setError: versionState.setError,
    setNotice: versionState.setNotice,
    setSaving: versionState.setSaving,
    skill,
  });
  const versionActions = useSkillVersionActions({
    loadVersions: versionState.loadVersions,
    onSkillChanged,
    resourceContent: versionState.resourceContent,
    saving: versionState.saving,
    selectedResource: versionState.selectedResource,
    selectedResourceIsText: versionState.selectedResourceIsText,
    selectedVersion: versionState.selectedVersion,
    setError: versionState.setError,
    setNotice: versionState.setNotice,
    setSaving: versionState.setSaving,
    skill,
    skillMD: versionState.skillMD,
  });

  return (
    <SideSheet
      className="coze-prototype-skill-panel"
      title={`${skill.name} · 版本管理`}
      visible
      size="large"
      closeOnEsc
      bodyStyle={{ padding: 0 }}
      onCancel={onClose}
    >
      <SkillVersionPanelBanners
        error={versionState.error}
        notice={versionState.notice}
      />

      <div className="coze-prototype-skill-panel-body">
        <SkillVersionList
          loading={versionState.loading}
          selectedVersionId={versionState.selectedVersionId}
          versions={versionState.versions}
          onSelectVersion={version =>
            void versionState.handleSelectVersion(version)
          }
        />
        <SkillVersionWorkspace
          metadataEditor={
            <SkillMetadataEditor
              description={metadataActions.description}
              disabled={
                versionState.saving ||
                !metadataActions.name.trim() ||
                !metadataActions.description.trim() ||
                !metadataActions.version.trim()
              }
              name={metadataActions.name}
              saving={versionState.saving}
              version={metadataActions.version}
              onDescriptionChange={metadataActions.setDescription}
              onNameChange={metadataActions.setName}
              onSave={() => void metadataActions.handleSaveMetadata()}
              onVersionChange={metadataActions.setVersion}
            />
          }
          permissionEditor={
            <SkillPermissionEditor
              allowedToolsText={permissionActions.allowedToolsText}
              disabled={versionState.saving}
              network={permissionActions.network}
              saving={versionState.saving}
              toolCandidates={permissionActions.toolCandidates}
              onAllowedToolsChange={permissionActions.setAllowedToolsText}
              onNetworkChange={permissionActions.setNetwork}
              onSave={() => void permissionActions.handleSavePermissions()}
              onToggleToolCandidate={
                permissionActions.handleToggleToolCandidate
              }
            />
          }
          resourceContent={versionState.resourceContent}
          resources={versionState.resources}
          saving={versionState.saving}
          selectedResource={versionState.selectedResource}
          selectedResourcePath={versionState.selectedResourcePath}
          selectedVersion={versionState.selectedVersion}
          skillMD={versionState.skillMD}
          onExport={() => void versionActions.handleExport()}
          onResourceContentChange={versionState.setResourceContent}
          onRollback={() => void versionActions.handleRollback()}
          onSaveResource={() => void versionActions.handleSaveResource()}
          onSaveSkillMD={() => void versionActions.handleSaveSkillMD()}
          onSelectedResourcePathChange={versionState.setSelectedResourcePath}
          onSkillMDChange={versionState.setSkillMD}
        />
      </div>
    </SideSheet>
  );
};
