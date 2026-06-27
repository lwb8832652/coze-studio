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

import type { ReactNode } from 'react';

import type { workbenchSkill } from '@coze-studio/api-schema';
import { IconCozHistory } from '@coze-arch/coze-design/icons';
import {
  Banner,
  Button,
  Checkbox,
  Input,
  Spin,
  TabPane,
  Tabs,
  TextArea,
} from '@coze-arch/coze-design';

import { formatVersionTime } from './skill-version-panel-utils';

type SkillResource = workbenchSkill.SkillResource;
type SkillToolCandidate = workbenchSkill.SkillToolCandidate;
type SkillVersion = workbenchSkill.SkillVersion;

const skillToolCandidateSourceLabel = (candidate: SkillToolCandidate) =>
  candidate.source === 'mcp' && candidate.source_name
    ? candidate.source_name
    : '';

interface SkillMetadataEditorProps {
  description: string;
  disabled: boolean;
  name: string;
  saving: boolean;
  version: string;
  onDescriptionChange: (value: string) => void;
  onNameChange: (value: string) => void;
  onSave: () => void;
  onVersionChange: (value: string) => void;
}

export const SkillMetadataEditor = ({
  description,
  disabled,
  name,
  saving,
  version,
  onDescriptionChange,
  onNameChange,
  onSave,
  onVersionChange,
}: SkillMetadataEditorProps) => (
  <section className="coze-prototype-skill-metadata">
    <div className="coze-prototype-skill-metadata-header">
      <h3>基础信息</h3>
      <Button
        size="small"
        theme="solid"
        loading={saving}
        disabled={disabled}
        onClick={onSave}
      >
        保存基础信息
      </Button>
    </div>
    <div className="coze-prototype-skill-metadata-grid">
      <Input
        aria-label="技能名称"
        value={name}
        onChange={onNameChange}
        placeholder="技能名称"
      />
      <Input
        aria-label="技能版本"
        value={version}
        onChange={onVersionChange}
        placeholder="1.0.0"
      />
      <TextArea
        aria-label="技能描述"
        className="coze-prototype-skill-metadata-description"
        value={description}
        autosize={false}
        rows={3}
        onChange={onDescriptionChange}
        placeholder="技能描述"
      />
    </div>
  </section>
);

interface SkillPermissionEditorProps {
  allowedToolsText: string;
  disabled: boolean;
  network: boolean;
  saving: boolean;
  toolCandidates: SkillToolCandidate[];
  onAllowedToolsChange: (value: string) => void;
  onNetworkChange: (value: boolean) => void;
  onSave: () => void;
  onToggleToolCandidate: (toolName: string) => void;
}

export const SkillPermissionEditor = ({
  allowedToolsText,
  disabled,
  network,
  saving,
  toolCandidates,
  onAllowedToolsChange,
  onNetworkChange,
  onSave,
  onToggleToolCandidate,
}: SkillPermissionEditorProps) => (
  <section className="coze-prototype-skill-permissions">
    <div className="coze-prototype-skill-metadata-header">
      <h3>权限配置</h3>
      <Button
        size="small"
        theme="solid"
        loading={saving}
        disabled={disabled}
        onClick={onSave}
      >
        保存权限配置
      </Button>
    </div>
    <div className="coze-prototype-skill-permissions-body">
      <Checkbox
        aria-label="允许网络访问"
        checked={network}
        disabled={saving}
        onChange={event => onNetworkChange(Boolean(event.target.checked))}
      >
        允许网络访问
      </Checkbox>
      <TextArea
        aria-label="允许工具名称"
        value={allowedToolsText}
        autosize={false}
        rows={4}
        onChange={onAllowedToolsChange}
        placeholder={'search\nweb_fetch'}
      />
    </div>
    {toolCandidates.length ? (
      <div className="coze-prototype-skill-tool-candidates">
        {toolCandidates.map(candidate => (
          <Button
            key={candidate.name}
            size="small"
            theme="outline"
            type="tertiary"
            disabled={saving}
            onClick={() => onToggleToolCandidate(candidate.name)}
          >
            <span className="coze-prototype-skill-tool-candidate-main">
              {candidate.display_name || candidate.name}
            </span>
            {skillToolCandidateSourceLabel(candidate) ? (
              <span className="coze-prototype-skill-tool-candidate-source">
                {skillToolCandidateSourceLabel(candidate)}
              </span>
            ) : null}
          </Button>
        ))}
      </div>
    ) : null}
  </section>
);

export const SkillVersionPanelBanners = ({
  error,
  notice,
}: {
  error: string;
  notice: string;
}) => (
  <>
    {error ? (
      <Banner
        className="coze-prototype-skill-panel-banner"
        type="danger"
        description={error}
      />
    ) : null}
    {notice ? (
      <Banner
        className="coze-prototype-skill-panel-banner"
        type="success"
        description={notice}
      />
    ) : null}
  </>
);

export const SkillVersionList = ({
  loading,
  selectedVersionId,
  versions,
  onSelectVersion,
}: {
  loading: boolean;
  selectedVersionId: string;
  versions: SkillVersion[];
  onSelectVersion: (version: SkillVersion) => void;
}) => (
  <nav className="coze-prototype-skill-version-list" aria-label="版本历史">
    <div className="coze-prototype-skill-version-list-title">
      <IconCozHistory className="text-[14px]" />
      版本历史
    </div>
    {loading ? <Spin size="small" /> : null}
    {!loading && versions.length === 0 ? (
      <div className="coze-prototype-muted">暂无版本快照</div>
    ) : null}
    {versions.map((version, index) => (
      <Button
        key={version.id}
        block
        theme="borderless"
        type="tertiary"
        className="coze-prototype-skill-version-button"
        data-active={version.id === selectedVersionId}
        onClick={() => onSelectVersion(version)}
      >
        <span>v{version.version}</span>
        {index === 0 ? <small>当前</small> : null}
        <time>{formatVersionTime(version.created_at)}</time>
      </Button>
    ))}
  </nav>
);

interface SkillResourceEditorProps {
  resourceContent: string;
  resources: SkillResource[];
  saving: boolean;
  selectedResource?: SkillResource;
  selectedResourcePath: string;
  onResourceContentChange: (value: string) => void;
  onSaveResource: () => void;
  onSelectedResourcePathChange: (value: string) => void;
}

const SkillResourceEditor = ({
  resourceContent,
  resources,
  saving,
  selectedResource,
  selectedResourcePath,
  onResourceContentChange,
  onSaveResource,
  onSelectedResourcePathChange,
}: SkillResourceEditorProps) => (
  <TabPane tab={`资源文件 ${resources.length}`} itemKey="resources">
    <div className="coze-prototype-skill-editor-heading">
      <p>文本资源直接编辑，二进制资源使用 Base64 内容编辑。</p>
      <Button
        size="small"
        theme="solid"
        loading={saving}
        disabled={!selectedResource}
        onClick={onSaveResource}
      >
        保存资源
      </Button>
    </div>

    {resources.length ? (
      <div className="coze-prototype-skill-resource-editor">
        <div className="coze-prototype-skill-resource-list">
          {resources.map(resource => (
            <Button
              key={resource.id}
              block
              theme="borderless"
              type="tertiary"
              data-active={resource.path === selectedResourcePath}
              onClick={() => onSelectedResourcePathChange(resource.path)}
            >
              <span>{resource.path}</span>
              <small>{resource.size} B</small>
            </Button>
          ))}
        </div>
        <TextArea
          aria-label="资源内容"
          className="coze-prototype-skill-code-editor"
          value={resourceContent}
          autosize={false}
          rows={18}
          onChange={onResourceContentChange}
        />
      </div>
    ) : (
      <div className="coze-prototype-skill-resource-empty">
        当前版本没有附加资源
      </div>
    )}
  </TabPane>
);

interface SkillVersionWorkspaceProps extends SkillResourceEditorProps {
  metadataEditor?: ReactNode;
  permissionEditor?: ReactNode;
  selectedVersion?: SkillVersion;
  skillMD: string;
  onExport: () => void;
  onRollback: () => void;
  onSaveSkillMD: () => void;
  onSkillMDChange: (value: string) => void;
}

export const SkillVersionWorkspace = ({
  metadataEditor,
  permissionEditor,
  resourceContent,
  resources,
  saving,
  selectedResource,
  selectedResourcePath,
  selectedVersion,
  skillMD,
  onExport,
  onResourceContentChange,
  onRollback,
  onSaveResource,
  onSaveSkillMD,
  onSelectedResourcePathChange,
  onSkillMDChange,
}: SkillVersionWorkspaceProps) => (
  <section className="coze-prototype-skill-version-workspace">
    {metadataEditor}
    {permissionEditor}
    {selectedVersion ? (
      <>
        <div className="coze-prototype-skill-version-actions">
          <span className="coze-prototype-muted">
            快照 {selectedVersion.id}
          </span>
          <div className="flex-1" />
          <Button
            size="small"
            theme="outline"
            disabled={saving}
            onClick={onExport}
          >
            导出 .skill
          </Button>
          <Button
            size="small"
            theme="light"
            type="warning"
            disabled={saving}
            onClick={onRollback}
          >
            回滚到此版本
          </Button>
        </div>

        <Tabs className="coze-prototype-skill-version-tabs" type="line">
          <TabPane tab="SKILL.md" itemKey="skill-md">
            <div className="coze-prototype-skill-editor-heading">
              <p>编辑入口说明会创建新的不可变版本快照。</p>
              <Button
                size="small"
                theme="solid"
                loading={saving}
                disabled={!skillMD.trim()}
                onClick={onSaveSkillMD}
              >
                保存入口文件
              </Button>
            </div>
            <TextArea
              aria-label="SKILL.md 内容"
              className="coze-prototype-skill-code-editor"
              value={skillMD}
              autosize={false}
              rows={18}
              onChange={onSkillMDChange}
            />
          </TabPane>

          <SkillResourceEditor
            resourceContent={resourceContent}
            resources={resources}
            saving={saving}
            selectedResource={selectedResource}
            selectedResourcePath={selectedResourcePath}
            onResourceContentChange={onResourceContentChange}
            onSaveResource={onSaveResource}
            onSelectedResourcePathChange={onSelectedResourcePathChange}
          />
        </Tabs>
      </>
    ) : null}
  </section>
);
