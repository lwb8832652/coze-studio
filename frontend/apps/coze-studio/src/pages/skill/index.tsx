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

import { IconCozSetting } from '@coze-arch/coze-design/icons';
import { workbenchSkill } from '@coze-studio/api-schema';

import { WorkspacePageTopBar } from '../../components/workspace-page-top-bar';
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
  <section className="mt-[20px] rounded-[8px] border border-solid coz-stroke-primary px-[16px] py-[16px]">
    <h2 className="m-0 text-[16px] leading-[24px] font-[600] coz-fg-primary">
      导入技能
    </h2>
    <input
      aria-label="技能文件名"
      className="mt-[12px] h-[36px] w-full rounded-[6px] border border-solid coz-stroke-primary px-[10px] text-[14px] coz-fg-primary"
      value={fileName}
      onChange={event => onFileNameChange(event.target.value)}
      placeholder="skill.json"
    />
    <textarea
      aria-label="技能内容"
      className="mt-[10px] min-h-[112px] w-full resize-y rounded-[6px] border border-solid coz-stroke-primary px-[10px] py-[8px] text-[14px] leading-[22px] coz-fg-primary"
      value={content}
      onChange={event => onContentChange(event.target.value)}
      placeholder="粘贴技能 JSON 内容"
    />
    <div className="mt-[10px] flex justify-end">
      <button
        type="button"
        className="min-h-[36px] rounded-[6px] border-0 bg-[#4d53e8] px-[14px] text-[14px] text-white disabled:opacity-50"
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
  <article className="border-0 border-b border-solid border-[rgba(77,101,148,0.08)] px-[16px] py-[12px] last:border-b-0 hover:bg-[rgba(91,100,117,0.04)]">
    <div className="flex items-start gap-[12px]">
      <div className="mt-[2px] flex h-[28px] w-[28px] shrink-0 items-center justify-center rounded-[6px] border border-solid border-[rgba(77,101,148,0.15)] bg-[rgba(91,100,117,0.06)] text-[#444c5c]">
        <IconCozSetting className="text-[14px]" />
      </div>
      <div className="min-w-0 flex-1">
        <div className="flex min-w-0 items-center gap-[8px]">
          <h2 className="m-0 truncate text-[14px] leading-[20px] font-[500] text-[#232938]">
            {skill.name}
          </h2>
          <span className="h-[6px] w-[6px] shrink-0 rounded-full bg-[#2a9e06]" />
          <span className="inline-flex h-[16px] items-center rounded-[4px] bg-[rgba(91,100,117,0.08)] px-[6px] text-[11px] leading-[14px] text-[#444c5c]">
            {getSkillTypeText(skill.type)}
          </span>
          <span className="inline-flex h-[16px] items-center rounded-[4px] bg-[rgba(91,100,117,0.08)] px-[6px] text-[11px] leading-[14px] text-[#444c5c]">
            v{skill.version}
          </span>
        </div>
        <div className="mt-[4px] truncate text-[12px] leading-[18px] text-[#747b8a]">
          {skill.description || '暂无技能描述'}
        </div>
      </div>
      <div className="mt-[2px] flex shrink-0 items-center gap-[12px]">
        <span className="inline-flex h-[20px] items-center gap-[4px] rounded-full bg-[rgba(42,158,6,0.1)] px-[8px] text-[11px] leading-[16px] text-[#2a9e06]">
          <span className="h-[6px] w-[6px] rounded-full bg-[#2a9e06]" />
          {skill.enabled ? '已发布' : '已停用'}
        </span>
        <span className="text-[12px] leading-[18px] text-[#747b8a]">
          上架于 {getSkillUpdatedText(skill.updated_at)}
        </span>
        <button
          type="button"
          className="h-[28px] rounded-[6px] border border-solid border-[rgba(77,101,148,0.2)] bg-white px-[10px] text-[12px] leading-[18px] text-[#444c5c] disabled:opacity-50"
          disabled={disabled}
          onClick={() => onTestRun(skill)}
        >
          {running ? '运行中' : '试运行'}
        </button>
        <span className="text-[16px] leading-[20px] text-[#747b8a]">...</span>
      </div>
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
      <div className="rounded-[12px] border border-solid border-[rgba(77,101,148,0.15)] bg-white py-[32px] text-center text-[14px] text-[#747b8a]">
        加载中...
      </div>
    ) : null}

    {!loading && skills.length === 0 ? (
      <div className="rounded-[12px] border border-dashed border-[rgba(77,101,148,0.2)] bg-white px-[16px] py-[32px] text-center text-[14px] text-[#747b8a]">
        暂无技能
      </div>
    ) : null}

    <div className="overflow-hidden rounded-[12px] border border-solid border-[rgba(77,101,148,0.15)] bg-white">
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
    <h1 className="m-0 text-[26px] leading-[36px] font-[600] text-[#1d2129]">
      技能配置
    </h1>
    <p className="mt-[4px] mb-0 text-[13px] leading-[20px] text-[#747b8a]">
      集中管理工作空间内的全部技能,支持发布、订阅、调用与版本管理。
      <button
        type="button"
        className="ml-[8px] border-0 bg-transparent p-0 text-[13px] leading-[20px] text-[#2a6df4] cursor-pointer"
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
  <section className="mt-[24px] flex flex-wrap items-center gap-[12px]">
    <div className="grid h-[32px] grid-cols-3 rounded-[6px] border border-solid border-[rgba(77,101,148,0.2)] bg-white p-[2px]">
      {SKILL_TYPE_FILTERS.map(item => (
        <button
          key={item.value}
          type="button"
          className="rounded-[4px] border-0 bg-transparent px-[12px] text-[13px] leading-[18px] text-[#444c5c] data-[active=true]:bg-[rgba(91,100,117,0.1)] data-[active=true]:text-[#1d2129]"
          data-active={activeType === item.value}
          onClick={() => onActiveTypeChange(item.value)}
        >
          {item.label}
        </button>
      ))}
    </div>

    <div className="flex-1" />

    <label className="flex h-[32px] w-[220px] items-center gap-[6px] rounded-[6px] border border-solid border-[rgba(77,101,148,0.2)] bg-white px-[8px] text-[#747b8a]">
      <span className="shrink-0 text-[13px]" aria-hidden="true">
        ⌕
      </span>
      <input
        aria-label="搜索技能"
        className="min-w-0 flex-1 border-0 bg-transparent text-[13px] leading-[20px] text-[#232938] outline-none"
        value={keyword}
        onChange={event => onKeywordChange(event.target.value)}
        placeholder="搜索技能"
      />
    </label>

    <button
      type="button"
      className="flex h-[32px] items-center gap-[6px] rounded-[6px] border-0 bg-[#060e1f] px-[12px] text-[13px] leading-[20px] text-white"
      onClick={onCreate}
    >
      + 创建技能
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
    <main className="flex h-full flex-col overflow-auto bg-white">
      <WorkspacePageTopBar />
      <section className="mx-auto w-[calc(100%_-_64px)] max-w-[1016px] pt-[24px] pb-[28px]">
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
          <div className="mt-[16px] rounded-[8px] border border-solid border-[#ffd4cc] bg-[#fff1ee] px-[12px] py-[10px] text-[14px] leading-[20px] text-[#c02a1d] break-words">
            {error}
          </div>
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
