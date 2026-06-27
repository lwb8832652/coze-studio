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

import { workbenchSkill } from '@coze-studio/api-schema';
import {
  IconCozMore,
  IconCozPlus,
  IconCozSetting,
  IconCozUpload,
} from '@coze-arch/coze-design/icons';
import {
  Button,
  ButtonGroup,
  IconButton,
  Input,
  TextArea,
  Upload,
} from '@coze-arch/coze-design';

type Skill = workbenchSkill.Skill;

export type SkillTypeFilter =
  | 'all'
  | 'bootstrap'
  | 'custom'
  | 'public'
  | 'script'
  | 'workflow';

const SKILL_TYPE_FILTERS: Array<{ label: string; value: SkillTypeFilter }> = [
  { label: '全部技能', value: 'all' },
  { label: '自定义', value: 'custom' },
  { label: '公共', value: 'public' },
  { label: '内置', value: 'bootstrap' },
  { label: '脚本', value: 'script' },
  { label: '工作流', value: 'workflow' },
];

const SKILL_TYPE_FILTER_MAP: Record<
  Exclude<SkillTypeFilter, 'all'>,
  workbenchSkill.SkillType
> = {
  bootstrap: workbenchSkill.SkillType.DeerSkill,
  custom: workbenchSkill.SkillType.CustomSkill,
  public: workbenchSkill.SkillType.PublicSkill,
  script: workbenchSkill.SkillType.Script,
  workflow: workbenchSkill.SkillType.Workflow,
};

export const getVisibleSkills = (
  skills: Skill[],
  keyword: string,
  activeType: SkillTypeFilter,
) => {
  const normalizedKeyword = keyword.trim().toLowerCase();

  return skills.filter(skill => {
    const matchesKeyword = normalizedKeyword
      ? skill.name.toLowerCase().includes(normalizedKeyword) ||
        skill.description?.toLowerCase().includes(normalizedKeyword)
      : true;
    const matchesType =
      activeType === 'all' || skill.type === SKILL_TYPE_FILTER_MAP[activeType];

    return matchesKeyword && matchesType;
  });
};

const getSkillTypeText = (type: workbenchSkill.SkillType) => {
  const typeMap: Record<workbenchSkill.SkillType, string> = {
    [workbenchSkill.SkillType.Script]: '脚本',
    [workbenchSkill.SkillType.Workflow]: '工作流',
    [workbenchSkill.SkillType.DeerSkill]: 'Deer Skill',
    [workbenchSkill.SkillType.PublicSkill]: '公共技能',
    [workbenchSkill.SkillType.CustomSkill]: '自定义技能',
  };

  return typeMap[type] ?? '未知';
};

const getSkillUpdatedText = (timestamp: number) =>
  timestamp ? new Date(timestamp).toLocaleDateString() : '-';

interface ImportPanelProps {
  archiveLabel: string;
  content: string;
  disabled: boolean;
  fileName: string;
  importing: boolean;
  onContentChange: (value: string) => void;
  onFileSelect: (file: File) => void;
  onFileNameChange: (value: string) => void;
  onImport: () => void;
}

export const ImportPanel = ({
  archiveLabel,
  content,
  disabled,
  fileName,
  importing,
  onContentChange,
  onFileSelect,
  onFileNameChange,
  onImport,
}: ImportPanelProps) => (
  <section className="coze-prototype-import-panel">
    <h2>导入技能</h2>
    <Input
      aria-label="技能文件名"
      className="coze-prototype-form-field"
      value={fileName}
      onChange={onFileNameChange}
      placeholder="skill.json"
    />
    <Upload
      action=""
      uploadTrigger="custom"
      showUploadList={false}
      limit={1}
      accept=".skill,.md,.json,application/zip,application/json,text/markdown"
      onFileChange={files => {
        const file = files[0];
        if (file) {
          onFileSelect(file);
        }
      }}
    >
      <Button
        className="coze-prototype-import-upload"
        icon={<IconCozUpload />}
        theme="outline"
      >
        选择技能文件
      </Button>
    </Upload>
    {archiveLabel ? (
      <div className="coze-prototype-import-file-status">{archiveLabel}</div>
    ) : null}
    <TextArea
      aria-label="技能内容"
      className="coze-prototype-form-textarea"
      value={content}
      autosize={false}
      rows={7}
      onChange={onContentChange}
      placeholder="粘贴 SKILL.md 或技能 JSON 内容"
    />
    <div className="coze-prototype-form-actions">
      <Button
        theme="solid"
        loading={importing}
        disabled={disabled}
        onClick={onImport}
      >
        导入技能
      </Button>
    </div>
  </section>
);

interface CreateSkillPanelProps {
  creating: boolean;
  description: string;
  disabled: boolean;
  name: string;
  onCreate: () => void;
  onDescriptionChange: (value: string) => void;
  onNameChange: (value: string) => void;
}

export const CreateSkillPanel = ({
  creating,
  description,
  disabled,
  name,
  onCreate,
  onDescriptionChange,
  onNameChange,
}: CreateSkillPanelProps) => (
  <section className="coze-prototype-import-panel">
    <h2>新建技能</h2>
    <Input
      aria-label="新技能名称"
      className="coze-prototype-form-field"
      value={name}
      onChange={onNameChange}
      placeholder="例如 Market Scan"
    />
    <TextArea
      aria-label="新技能描述"
      className="coze-prototype-form-textarea"
      value={description}
      autosize={false}
      rows={5}
      onChange={onDescriptionChange}
      placeholder="描述技能用途、适用场景和调用边界"
    />
    <div className="coze-prototype-form-actions">
      <Button
        theme="solid"
        loading={creating}
        disabled={disabled}
        onClick={onCreate}
      >
        保存技能
      </Button>
    </div>
  </section>
);

interface SkillCardProps {
  deleting: boolean;
  disabled: boolean;
  result?: string;
  running: boolean;
  skill: Skill;
  updating: boolean;
  onManage: (skill: Skill) => void;
  onDelete: (skill: Skill) => void;
  onTestRun: (skill: Skill) => void;
  onToggleEnabled: (skill: Skill) => void;
}

const SkillCard = ({
  deleting,
  disabled,
  result,
  running,
  skill,
  updating,
  onDelete,
  onManage,
  onTestRun,
  onToggleEnabled,
}: SkillCardProps) => (
  <article className="coze-prototype-row">
    <div className="coze-prototype-skill-icon">
      <IconCozSetting className="text-[14px]" />
    </div>
    <div className="coze-prototype-row-main">
      <div className="flex min-w-0 items-center gap-[8px]">
        <h2 className="m-0 truncate text-[14px] leading-[20px] font-[400] text-[#232938]">
          {skill.name}
        </h2>
        <span className="h-[6px] w-[6px] shrink-0 rounded-full bg-[#2a9e06]" />
        <span className="coze-prototype-tag">
          {getSkillTypeText(skill.type)}
        </span>
        <span className="coze-prototype-tag">v{skill.version}</span>
      </div>
      <div className="coze-prototype-row-desc mt-[4px]">
        {skill.description || '暂无技能描述'}
      </div>
    </div>
    <div className="coze-prototype-row-actions mt-[4px]">
      <span className="coze-prototype-status-pill" data-tone="success">
        <span
          className="coze-prototype-status-dot"
          style={{ backgroundColor: '#2a9e06' }}
        />
        {skill.enabled ? '已发布' : '已停用'}
      </span>
      <span className="coze-prototype-muted">
        上架于 {getSkillUpdatedText(skill.updated_at)}
      </span>
      <Button
        size="small"
        theme="outline"
        loading={running}
        disabled={disabled || updating || deleting}
        onClick={() => onTestRun(skill)}
      >
        试运行
      </Button>
      <Button
        size="small"
        theme="outline"
        loading={updating}
        disabled={disabled || running || deleting}
        onClick={() => onToggleEnabled(skill)}
      >
        {skill.enabled ? '停用' : '启用'}
      </Button>
      <Button
        size="small"
        theme="outline"
        type="danger"
        loading={deleting}
        disabled={disabled || running || updating}
        onClick={() => onDelete(skill)}
      >
        删除
      </Button>
      <IconButton
        size="small"
        theme="borderless"
        icon={<IconCozMore className="text-[16px] text-[#747b8a]" />}
        aria-label={`管理 ${skill.name}`}
        onClick={() => onManage(skill)}
      />
    </div>
    {result ? (
      <pre className="mt-[10px] max-w-full overflow-auto rounded-[6px] bg-[#f7f7fa] px-[10px] py-[8px] text-[13px] leading-[20px] coz-fg-primary">
        {result}
      </pre>
    ) : null}
  </article>
);

interface SkillListProps {
  deletingSkillId: string;
  loading: boolean;
  runningSkillId: string;
  skills: Skill[];
  testResults: Record<string, string>;
  updatingSkillId: string;
  onDelete: (skill: Skill) => void;
  onManage: (skill: Skill) => void;
  onTestRun: (skill: Skill) => void;
  onToggleEnabled: (skill: Skill) => void;
}

export const SkillList = ({
  deletingSkillId,
  loading,
  runningSkillId,
  skills,
  testResults,
  updatingSkillId,
  onDelete,
  onManage,
  onTestRun,
  onToggleEnabled,
}: SkillListProps) => (
  <section className="mt-[20px]" aria-label="技能列表">
    {loading ? <div className="coze-prototype-empty">加载中...</div> : null}
    {!loading && skills.length === 0 ? (
      <div className="coze-prototype-empty">暂无技能</div>
    ) : null}

    <div className="coze-prototype-list">
      {skills.map(skill => (
        <SkillCard
          key={skill.id}
          deleting={deletingSkillId === skill.id}
          disabled={Boolean(
            runningSkillId || updatingSkillId || deletingSkillId,
          )}
          result={testResults[skill.id]}
          running={runningSkillId === skill.id}
          skill={skill}
          updating={updatingSkillId === skill.id}
          onDelete={onDelete}
          onManage={onManage}
          onTestRun={onTestRun}
          onToggleEnabled={onToggleEnabled}
        />
      ))}
    </div>
  </section>
);

interface SkillPageHeaderProps {
  loading: boolean;
  spaceId?: string;
  onRefresh: () => void;
}

export const SkillPageHeader = ({
  loading,
  spaceId,
  onRefresh,
}: SkillPageHeaderProps) => (
  <div className="text-center">
    <h1 className="coze-prototype-page-title">技能配置</h1>
    <p className="coze-prototype-page-subtitle">
      集中管理工作空间内的全部技能,支持发布、订阅、调用与版本管理。
      <Button
        size="small"
        theme="borderless"
        className="coze-prototype-link-button"
      >
        查看文档
      </Button>
    </p>
    <Button
      theme="borderless"
      className="sr-only"
      disabled={loading || !spaceId}
      onClick={onRefresh}
    >
      刷新
    </Button>
  </div>
);

interface SkillToolbarProps {
  activeType: SkillTypeFilter;
  keyword: string;
  onActiveTypeChange: (value: SkillTypeFilter) => void;
  onCreate: () => void;
  onImport: () => void;
  onKeywordChange: (value: string) => void;
}

export const SkillToolbar = ({
  activeType,
  keyword,
  onActiveTypeChange,
  onCreate,
  onImport,
  onKeywordChange,
}: SkillToolbarProps) => (
  <section className="coze-prototype-toolbar">
    <ButtonGroup size="small" className="coze-prototype-skill-filter">
      {SKILL_TYPE_FILTERS.map(item => (
        <Button
          key={item.value}
          theme={activeType === item.value ? 'light' : 'borderless'}
          data-active={activeType === item.value}
          onClick={() => onActiveTypeChange(item.value)}
        >
          {item.label}
        </Button>
      ))}
    </ButtonGroup>
    <div className="flex-1" />
    <Input
      size="small"
      className="coze-prototype-skill-search"
      aria-label="搜索技能"
      value={keyword}
      prefix={<span aria-hidden="true">⌕</span>}
      onChange={onKeywordChange}
      placeholder="搜索技能"
    />
    <Button
      size="small"
      theme="outline"
      icon={<IconCozUpload className="text-[14px]" />}
      onClick={onImport}
    >
      导入技能
    </Button>
    <Button
      size="small"
      theme="solid"
      icon={<IconCozPlus className="text-[14px]" />}
      onClick={onCreate}
    >
      创建技能
    </Button>
  </section>
);
