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

import { workbenchSkill } from '@coze-studio/api-schema';

import { importSkill, listSkills, testRunSkill } from './service';

type Skill = workbenchSkill.Skill;

const TEST_INPUT = JSON.stringify({ message: 'ping' });

const getSkillTypeText = (type: workbenchSkill.SkillType) => {
  const typeMap: Record<workbenchSkill.SkillType, string> = {
    [workbenchSkill.SkillType.Script]: '脚本',
    [workbenchSkill.SkillType.Workflow]: '工作流',
  };

  return typeMap[type] ?? '未知';
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
  <article className="rounded-[8px] border border-solid coz-stroke-primary px-[16px] py-[14px]">
    <div className="flex flex-wrap items-start justify-between gap-[12px]">
      <div className="min-w-0">
        <h2 className="m-0 break-words text-[16px] leading-[24px] font-[600] coz-fg-primary">
          {skill.name}
        </h2>
        <div className="mt-[6px] flex flex-wrap gap-[8px] text-[13px] leading-[20px] coz-fg-secondary">
          <span>{getSkillTypeText(skill.type)}</span>
          <span>v{skill.version}</span>
          <span>{skill.enabled ? '已启用' : '已停用'}</span>
        </div>
      </div>
      <button
        type="button"
        className="min-h-[32px] rounded-[6px] border border-solid coz-stroke-primary px-[12px] text-[14px] coz-fg-primary coz-bg-plus disabled:opacity-50"
        disabled={disabled}
        onClick={() => onTestRun(skill)}
      >
        {running ? '运行中' : '试运行'}
      </button>
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
      <div className="py-[32px] text-center text-[14px] coz-fg-secondary">
        加载中...
      </div>
    ) : null}

    {!loading && skills.length === 0 ? (
      <div className="rounded-[8px] border border-dashed coz-stroke-primary px-[16px] py-[32px] text-center text-[14px] coz-fg-secondary">
        暂无技能
      </div>
    ) : null}

    <div className="grid gap-[12px]">
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
    <main className="h-full px-[24px] py-[24px] coz-bg-primary overflow-auto">
      <section className="rounded-[8px] border border-solid coz-stroke-primary coz-bg-plus px-[24px] py-[20px]">
        <div className="flex flex-wrap items-center justify-between gap-[12px]">
          <h1 className="m-0 text-[20px] leading-[28px] font-[600] coz-fg-primary">
            技能配置
          </h1>
          <button
            type="button"
            className="h-[32px] rounded-[6px] border border-solid coz-stroke-primary px-[12px] text-[14px] coz-fg-primary coz-bg-plus"
            disabled={loading || !space_id}
            onClick={loadSkills}
          >
            刷新
          </button>
        </div>

        {error ? (
          <div className="mt-[16px] rounded-[8px] border border-solid border-[#ffd4cc] bg-[#fff1ee] px-[12px] py-[10px] text-[14px] leading-[20px] text-[#c02a1d] break-words">
            {error}
          </div>
        ) : null}

        <ImportPanel
          content={content}
          disabled={!space_id || !content.trim() || importing}
          fileName={fileName}
          importing={importing}
          onContentChange={setContent}
          onFileNameChange={setFileName}
          onImport={handleImport}
        />

        <SkillList
          loading={loading}
          runningSkillId={runningSkillId}
          skills={skills}
          testResults={testResults}
          onTestRun={handleTestRun}
        />
      </section>
    </main>
  );
};

export default SkillPage;
