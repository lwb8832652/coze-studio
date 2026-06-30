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
  IconCozUpload,
} from '@coze-arch/coze-design/icons';
import {
  Button,
  IconButton,
  Input,
  Switch,
  TextArea,
  Upload,
} from '@coze-arch/coze-design';

type Skill = workbenchSkill.Skill;

export type SkillTypeFilter = 'custom' | 'public';

const SKILL_TYPE_FILTERS: Array<{ label: string; value: SkillTypeFilter }> = [
  { label: '公共', value: 'public' },
  { label: '自定义', value: 'custom' },
];

const SKILL_TYPE_FILTER_MAP: Record<
  SkillTypeFilter,
  workbenchSkill.SkillType[]
> = {
  custom: [workbenchSkill.SkillType.CustomSkill],
  public: [
    workbenchSkill.SkillType.DeerSkill,
    workbenchSkill.SkillType.PublicSkill,
  ],
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
    const matchesType = SKILL_TYPE_FILTER_MAP[activeType].includes(skill.type);

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
  actionsOpen: boolean;
  deleting: boolean;
  disabled: boolean;
  result?: string;
  running: boolean;
  skill: Skill;
  updating: boolean;
  onManage: (skill: Skill) => void;
  onDelete: (skill: Skill) => void;
  onTestRun: (skill: Skill) => void;
  onToggleActions: (skill: Skill) => void;
  onToggleEnabled: (skill: Skill) => void;
}

const SkillCard = ({
  actionsOpen,
  deleting,
  disabled,
  result,
  running,
  skill,
  updating,
  onDelete,
  onManage,
  onTestRun,
  onToggleActions,
  onToggleEnabled,
}: SkillCardProps) => (
  <article className="coze-prototype-skill-settings-item">
    <div className="coze-prototype-row-main">
      <div className="flex min-w-0 items-center gap-[8px]">
        <h2 className="m-0 truncate text-[14px] leading-[20px] font-[500] text-[#18181b]">
          {skill.name}
        </h2>
      </div>
      <div className="coze-prototype-row-desc mt-[4px]">
        {skill.description || '暂无技能描述'}
      </div>
    </div>
    <div className="coze-prototype-skill-settings-actions">
      <Switch
        size="small"
        checked={skill.enabled}
        loading={updating}
        disabled={disabled || running || deleting}
        aria-label={`${skill.enabled ? '关闭' : '开启'} ${skill.name}`}
        onChange={() => onToggleEnabled(skill)}
      />
      <IconButton
        size="small"
        theme="borderless"
        icon={<IconCozMore className="text-[16px] text-[#747b8a]" />}
        aria-label={`更多 ${skill.name}`}
        onClick={() => onToggleActions(skill)}
      />
    </div>
    {actionsOpen ? (
      <div className="coze-prototype-skill-action-menu">
        <Button size="small" theme="borderless" onClick={() => onManage(skill)}>
          版本管理
        </Button>
        <Button
          size="small"
          theme="borderless"
          loading={running}
          disabled={disabled || updating || deleting}
          onClick={() => onTestRun(skill)}
        >
          试运行
        </Button>
        <Button
          size="small"
          theme="borderless"
          type="danger"
          loading={deleting}
          disabled={disabled || running || updating}
          onClick={() => onDelete(skill)}
        >
          删除
        </Button>
        <span className="coze-prototype-skill-menu-meta">
          {getSkillTypeText(skill.type)} · v{skill.version} ·{' '}
          {getSkillUpdatedText(skill.updated_at)}
        </span>
      </div>
    ) : null}
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
  openActionsSkillId: string;
  skills: Skill[];
  testResults: Record<string, string>;
  updatingSkillId: string;
  onCreate: () => void;
  onDelete: (skill: Skill) => void;
  onManage: (skill: Skill) => void;
  onTestRun: (skill: Skill) => void;
  onToggleActions: (skill: Skill) => void;
  onToggleEnabled: (skill: Skill) => void;
}

export const SkillList = ({
  deletingSkillId,
  loading,
  openActionsSkillId,
  runningSkillId,
  skills,
  testResults,
  updatingSkillId,
  onCreate,
  onDelete,
  onManage,
  onTestRun,
  onToggleActions,
  onToggleEnabled,
}: SkillListProps) => {
  if (loading) {
    return (
      <section
        className="coze-prototype-skill-settings-list"
        aria-label="技能列表"
      >
        <div className="coze-prototype-skill-settings-loading">加载中...</div>
      </section>
    );
  }

  if (skills.length === 0) {
    return (
      <section
        className="coze-prototype-skill-settings-list"
        aria-label="技能列表"
      >
        <div className="coze-prototype-skill-settings-empty">
          <div className="coze-prototype-skill-settings-empty-icon">
            <IconCozPlus className="text-[18px]" />
          </div>
          <div className="coze-prototype-skill-settings-empty-title">
            还没有技能
          </div>
          <p>
            将你的 Agent Skill 文件夹放在 DeerFlow 根目录下的 `/skills/custom`
            文件夹中。
          </p>
          <Button size="small" theme="solid" onClick={onCreate}>
            创建你的第一个技能
          </Button>
        </div>
      </section>
    );
  }

  return (
    <section
      className="coze-prototype-skill-settings-list"
      aria-label="技能列表"
    >
      <div className="coze-prototype-list">
        {skills.map(skill => (
          <SkillCard
            key={skill.id}
            actionsOpen={openActionsSkillId === skill.id}
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
            onToggleActions={onToggleActions}
            onToggleEnabled={onToggleEnabled}
          />
        ))}
      </div>
    </section>
  );
};

export const SkillPageHeader = () => (
  <div className="coze-prototype-skill-settings-header">
    <h1>技能</h1>
    <p>管理 Agent Skill 配置和启用状态。</p>
  </div>
);

interface SkillToolbarProps {
  activeType: SkillTypeFilter;
  onActiveTypeChange: (value: SkillTypeFilter) => void;
  onCreate: () => void;
}

export const SkillToolbar = ({
  activeType,
  onActiveTypeChange,
  onCreate,
}: SkillToolbarProps) => (
  <section className="coze-prototype-skill-settings-toolbar">
    <div className="coze-prototype-skill-settings-tabs" role="tablist">
      {SKILL_TYPE_FILTERS.map(item => (
        <button
          key={item.value}
          type="button"
          role="tab"
          aria-selected={activeType === item.value}
          data-active={activeType === item.value}
          onClick={() => onActiveTypeChange(item.value)}
        >
          {item.label}
        </button>
      ))}
    </div>
    <div className="flex-1" />
    <Button
      size="small"
      theme="solid"
      icon={<IconCozPlus className="text-[14px]" />}
      onClick={onCreate}
    >
      新建技能
    </Button>
  </section>
);
