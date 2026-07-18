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

import { useNavigate, useParams } from 'react-router-dom';
import { useState } from 'react';

import type { workbenchSkill } from '@coze-studio/api-schema';

import '../../components/workspace-prototype.less';
import { createSkillCreationNavigationState } from '../workbench/skill-creation-intent';
import { SkillVersionPanel } from './skill-version-panel';
import { useSkillListLoader, useSkillRunActions } from './skill-page-hooks';
import {
  SkillList,
  SkillPageHeader,
  SkillToolbar,
  getVisibleSkills,
  type SkillTypeFilter,
} from './skill-page-components';

type Skill = workbenchSkill.Skill;

const SkillPage = () => {
  const { space_id } = useParams();
  const navigate = useNavigate();
  const [activeType, setActiveType] = useState<SkillTypeFilter>('public');
  const [openActionsSkillId, setOpenActionsSkillId] = useState('');
  const [managedSkill, setManagedSkill] = useState<Skill>();
  const { error, loadSkills, loading, setError, skills } =
    useSkillListLoader(space_id);
  const runActions = useSkillRunActions({
    loadSkills,
    setError,
  });

  const visibleSkills = getVisibleSkills(skills, '', activeType);
  const openSkillCreator = () => {
    setOpenActionsSkillId('');

    if (!space_id) {
      return;
    }

    navigate(`/space/${space_id}/chats/new`, {
      state: createSkillCreationNavigationState(),
    });
  };

  return (
    <main className="coze-prototype-page coze-prototype-skill-settings-page">
      <section className="coze-prototype-page-inner">
        <SkillPageHeader />
        <SkillToolbar
          activeType={activeType}
          onActiveTypeChange={value => {
            setActiveType(value);
            setOpenActionsSkillId('');
          }}
          onCreate={openSkillCreator}
        />

        {error ? <div className="coze-prototype-error">{error}</div> : null}

        <SkillList
          deletingSkillId={runActions.deletingSkillId}
          loading={loading}
          openActionsSkillId={openActionsSkillId}
          runningSkillId={runActions.runningSkillId}
          skills={visibleSkills}
          testResults={runActions.testResults}
          updatingSkillId={runActions.updatingSkillId}
          onCreate={openSkillCreator}
          onDelete={runActions.handleDeleteSkill}
          onManage={skill => {
            setManagedSkill(skill);
            setOpenActionsSkillId('');
          }}
          onTestRun={runActions.handleTestRun}
          onToggleActions={skill =>
            setOpenActionsSkillId(current =>
              current === skill.id ? '' : skill.id,
            )
          }
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
