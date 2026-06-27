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

import { useParams } from 'react-router-dom';
import { useState } from 'react';

import type { workbenchSkill } from '@coze-studio/api-schema';

import { WorkspacePageTopBar } from '../../components/workspace-page-top-bar';
import '../../components/workspace-prototype.less';
import { SkillVersionPanel } from './skill-version-panel';
import {
  useSkillCreateActions,
  useSkillImportActions,
  useSkillListLoader,
  useSkillRunActions,
} from './skill-page-hooks';
import {
  CreateSkillPanel,
  ImportPanel,
  SkillList,
  SkillPageHeader,
  SkillToolbar,
  getVisibleSkills,
  type SkillTypeFilter,
} from './skill-page-components';

type Skill = workbenchSkill.Skill;

const SkillPage = () => {
  const { space_id } = useParams();
  const [keyword, setKeyword] = useState('');
  const [activeType, setActiveType] = useState<SkillTypeFilter>('all');
  const [showCreatePanel, setShowCreatePanel] = useState(false);
  const [showImportPanel, setShowImportPanel] = useState(false);
  const [managedSkill, setManagedSkill] = useState<Skill>();
  const { error, loadSkills, loading, setError, skills } =
    useSkillListLoader(space_id);
  const importActions = useSkillImportActions({
    loadSkills,
    setError,
    spaceId: space_id,
  });
  const createActions = useSkillCreateActions({
    loadSkills,
    setError,
    spaceId: space_id,
  });
  const runActions = useSkillRunActions({
    loadSkills,
    setError,
  });

  const visibleSkills = getVisibleSkills(skills, keyword, activeType);

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
          onCreate={() => setShowCreatePanel(current => !current)}
          onImport={() => setShowImportPanel(current => !current)}
          onKeywordChange={setKeyword}
        />

        {error ? <div className="coze-prototype-error">{error}</div> : null}

        {showCreatePanel ? (
          <CreateSkillPanel
            creating={createActions.creating}
            description={createActions.description}
            disabled={
              !space_id ||
              !createActions.name.trim() ||
              !createActions.description.trim() ||
              createActions.creating
            }
            name={createActions.name}
            onCreate={createActions.handleCreate}
            onDescriptionChange={createActions.setDescription}
            onNameChange={createActions.setName}
          />
        ) : null}

        {showImportPanel ? (
          <ImportPanel
            archiveLabel={importActions.archiveLabel}
            content={importActions.content}
            disabled={
              !space_id ||
              (!importActions.archiveContent &&
                !importActions.content.trim()) ||
              importActions.importing
            }
            fileName={importActions.fileName}
            importing={importActions.importing}
            onContentChange={importActions.handleContentChange}
            onFileSelect={file => void importActions.handleImportFile(file)}
            onFileNameChange={importActions.setFileName}
            onImport={importActions.handleImport}
          />
        ) : null}

        <SkillList
          deletingSkillId={runActions.deletingSkillId}
          loading={loading}
          runningSkillId={runActions.runningSkillId}
          skills={visibleSkills}
          testResults={runActions.testResults}
          updatingSkillId={runActions.updatingSkillId}
          onDelete={runActions.handleDeleteSkill}
          onManage={setManagedSkill}
          onTestRun={runActions.handleTestRun}
          onToggleEnabled={runActions.handleToggleEnabled}
        />
      </section>
      {managedSkill ? (
        <SkillVersionPanel
          skill={managedSkill}
          onClose={() => setManagedSkill(undefined)}
          onSkillChanged={loadSkills}
        />
      ) : null}
    </main>
  );
};

export default SkillPage;
