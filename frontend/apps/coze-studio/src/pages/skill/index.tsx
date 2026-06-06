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
import { useParams } from 'react-router-dom';

import {
  IconCozMore,
  IconCozPlus,
  IconCozSetting,
} from '@coze-arch/coze-design/icons';
import { workbenchSkill } from '@coze-studio/api-schema';

import { WorkspacePageTopBar } from '../../components/workspace-page-top-bar';
import '../../components/workspace-prototype.less';
import { importSkill, listSkills, testRunSkill } from './service';

type Skill = workbenchSkill.Skill;

const TEST_INPUT = JSON.stringify({ message: 'ping' });
type SkillTypeFilter = 'all' | 'script' | 'workflow';

const SKILL_TYPE_FILTERS: Array<{ label: string; value: SkillTypeFilter }> = [
  { label: '个人配置', value: 'all' },
  { label: '我的收藏', value: 'script' },
  { label: '市场发现', value: 'workflow' },
];

const getVisibleSkills = (
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
      activeType === 'all' ||
      (activeType === 'script' &&
        skill.type === workbenchSkill.SkillType.Script) ||
      (activeType === 'workflow' &&
        skill.type === workbenchSkill.SkillType.Workflow);

    return matchesKeyword && matchesType;
  });
};

const getSkillTypeText = (type: workbenchSkill.SkillType) => {
  const typeMap: Record<workbenchSkill.SkillType, string> = {
    [workbenchSkill.SkillType.Script]: '脚本',
    [workbenchSkill.SkillType.Workflow]: '工作流',
  };

  return typeMap[type] ?? '未知';
};

const getSkillUpdatedText = (timestamp: number) => {
  if (!timestamp) {
    return '-';
  }

  return new Date(timestamp).toLocaleDateString();
};

interface ImportPanelProps {
  content: string;
  disabled: boolean;
  fileName: string;
  importing: boolean;
  onContentChange: (value: string) => void;
  onFileNameChange: (value: string) => void;
  onImport: () => void;
}

const ImportPanel = ({
  content,
  disabled,
  fileName,
  importing,
  onContentChange,
  onFileNameChange,
  onImport,
}: ImportPanelProps) => (
  <section className="coze-prototype-import-panel">
    <h2>
      导入技能
    </h2>
    <input
      aria-label="技能文件名"
      className="coze-prototype-form-field"
      value={fileName}
      onChange={event => onFileNameChange(event.target.value)}
      placeholder="skill.json"
    />
    <textarea
      aria-label="技能内容"
      className="coze-prototype-form-textarea"
      value={content}
      onChange={event => onContentChange(event.target.value)}
      placeholder="粘贴技能 JSON 内容"
    />
    <div className="coze-prototype-form-actions">
      <button
        type="button"
        className="coze-prototype-primary-button disabled:opacity-50"
        disabled={disabled}
        onClick={onImport}
      >
        {importing ? '导入中' : '导入'}
      </button>
    </div>
  </section>
);

interface SkillCardProps {
  disabled: boolean;
  result?: string;
  running: boolean;
  skill: Skill;
  onTestRun: (skill: Skill) => void;
}

const SkillCard = ({
  disabled,
  result,
  running,
  skill,
  onTestRun,
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
        <span className="coze-prototype-tag">{getSkillTypeText(skill.type)}</span>
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
      <button
        type="button"
        className="coze-prototype-secondary-button h-[28px] px-[10px] text-[12px] disabled:opacity-50"
        disabled={disabled}
        onClick={() => onTestRun(skill)}
      >
        {running ? '运行中' : '试运行'}
      </button>
      <IconCozMore className="text-[16px] text-[#747b8a]" />
    </div>
    {result ? (
      <pre className="mt-[10px] max-w-full overflow-auto rounded-[6px] bg-[#f7f7fa] px-[10px] py-[8px] text-[13px] leading-[20px] coz-fg-primary">
        {result}
      </pre>
    ) : null}
  </article>
);

interface SkillListProps {
  loading: boolean;
  runningSkillId: string;
  skills: Skill[];
  testResults: Record<string, string>;
  onTestRun: (skill: Skill) => void;
}

const SkillList = ({
  loading,
  runningSkillId,
  skills,
  testResults,
  onTestRun,
}: SkillListProps) => (
  <section className="mt-[20px]" aria-label="技能列表">
    {loading ? (
      <div className="coze-prototype-empty">
        加载中...
      </div>
    ) : null}

    {!loading && skills.length === 0 ? (
      <div className="coze-prototype-empty">
        暂无技能
      </div>
    ) : null}

    <div className="coze-prototype-list">
      {skills.map(skill => (
        <SkillCard
          key={skill.id}
          disabled={Boolean(runningSkillId)}
          result={testResults[skill.id]}
          running={runningSkillId === skill.id}
          skill={skill}
          onTestRun={onTestRun}
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

const SkillPageHeader = ({
  loading,
  spaceId,
  onRefresh,
}: SkillPageHeaderProps) => (
  <div className="text-center">
    <h1 className="coze-prototype-page-title">技能配置</h1>
    <p className="coze-prototype-page-subtitle">
      集中管理工作空间内的全部技能,支持发布、订阅、调用与版本管理。
      <button
        type="button"
        className="coze-prototype-link-button"
      >
        查看文档
      </button>
    </p>
    <button
      type="button"
      className="sr-only"
      disabled={loading || !spaceId}
      onClick={onRefresh}
    >
      刷新
    </button>
  </div>
);

interface SkillToolbarProps {
  activeType: SkillTypeFilter;
  keyword: string;
  onActiveTypeChange: (value: SkillTypeFilter) => void;
  onCreate: () => void;
  onKeywordChange: (value: string) => void;
}

const SkillToolbar = ({
  activeType,
  keyword,
  onActiveTypeChange,
  onCreate,
  onKeywordChange,
}: SkillToolbarProps) => (
  <section className="coze-prototype-toolbar">
    <div className="coze-prototype-segment" data-size="small">
      {SKILL_TYPE_FILTERS.map(item => (
        <button
          key={item.value}
          type="button"
          data-active={activeType === item.value}
          onClick={() => onActiveTypeChange(item.value)}
        >
          {item.label}
        </button>
      ))}
    </div>

    <div className="flex-1" />

    <label className="coze-prototype-search" data-width="compact">
      <span aria-hidden="true">
        ⌕
      </span>
      <input
        aria-label="搜索技能"
        value={keyword}
        onChange={event => onKeywordChange(event.target.value)}
        placeholder="搜索技能"
      />
    </label>

    <button
      type="button"
      className="coze-prototype-primary-button"
      onClick={onCreate}
    >
      <IconCozPlus className="text-[14px]" />
      创建技能
    </button>
  </section>
);

const SkillPage = () => {
  const { space_id } = useParams();
  const [skills, setSkills] = useState<Skill[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [fileName, setFileName] = useState('skill.json');
  const [content, setContent] = useState('');
  const [importing, setImporting] = useState(false);
  const [runningSkillId, setRunningSkillId] = useState('');
  const [testResults, setTestResults] = useState<Record<string, string>>({});
  const [keyword, setKeyword] = useState('');
  const [activeType, setActiveType] = useState<SkillTypeFilter>('all');
  const [showImportPanel, setShowImportPanel] = useState(false);

  const visibleSkills = getVisibleSkills(skills, keyword, activeType);

  const loadSkills = async () => {
    if (!space_id) {
      return;
    }

    setLoading(true);
    setError('');

    try {
      const response = await listSkills({ space_id });
      setSkills(response.data?.skills ?? []);
    } catch (err) {
      setError(err instanceof Error ? err.message : '加载技能失败');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void loadSkills();
  }, [space_id]);

  const handleImport = async () => {
    if (!space_id || !content.trim() || importing) {
      return;
    }

    setImporting(true);
    setError('');

    try {
      await importSkill({
        space_id,
        file_name: fileName.trim() || 'skill.json',
        content,
      });
      setContent('');
      await loadSkills();
    } catch (err) {
      setError(err instanceof Error ? err.message : '导入技能失败');
    } finally {
      setImporting(false);
    }
  };

  const handleTestRun = async (skill: Skill) => {
    if (runningSkillId) {
      return;
    }

    setRunningSkillId(skill.id);
    setError('');
    setTestResults(current => {
      const { [skill.id]: _staleResult, ...rest } = current;

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

  return (
    <main className="coze-prototype-page">
      <WorkspacePageTopBar />
      <section className="coze-prototype-page-inner">
        <SkillPageHeader
          loading={loading}
          spaceId={space_id}
          onRefresh={loadSkills}
        />
        <SkillToolbar
          activeType={activeType}
          keyword={keyword}
          onActiveTypeChange={setActiveType}
          onCreate={() => setShowImportPanel(current => !current)}
          onKeywordChange={setKeyword}
        />

        {error ? (
          <div className="coze-prototype-error">{error}</div>
        ) : null}

        {showImportPanel ? (
          <ImportPanel
            content={content}
            disabled={!space_id || !content.trim() || importing}
            fileName={fileName}
            importing={importing}
            onContentChange={setContent}
            onFileNameChange={setFileName}
            onImport={handleImport}
          />
        ) : null}

        <SkillList
          loading={loading}
          runningSkillId={runningSkillId}
          skills={visibleSkills}
          testResults={testResults}
          onTestRun={handleTestRun}
        />
      </section>
    </main>
  );
};

export default SkillPage;
