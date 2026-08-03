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

import { IconCozLightbulb } from '@coze-arch/coze-design/icons';

import { JournalViewState } from './view-state';
import type { JournalSnapshotViewProps } from './types';

export const JournalSkillView = ({
  snapshot,
  status,
}: JournalSnapshotViewProps) => {
  const direct = snapshot?.content?.skill?.skills ?? [];
  const fragmented =
    snapshot?.fragments.flatMap(fragment => fragment.skills ?? []) ?? [];
  const skills = direct.length ? direct : fragmented;

  if (status !== 'ready' || !snapshot || !skills.length) {
    return <JournalViewState status={status} />;
  }

  return (
    <div
      className="journal-skill-view"
      data-journal-scroll-key="skill-list"
      role="list"
    >
      {skills.map(skill => (
        <div
          className="journal-skill-row"
          data-testid="journal-skill-row"
          key={skill.skill_id}
          role="listitem"
        >
          <IconCozLightbulb aria-hidden="true" />
          <div>
            <strong>{skill.name}</strong>
            {skill.description ? <span>{skill.description}</span> : null}
          </div>
        </div>
      ))}
    </div>
  );
};
