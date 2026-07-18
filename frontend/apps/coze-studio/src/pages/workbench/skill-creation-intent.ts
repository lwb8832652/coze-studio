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

export const SKILL_CREATION_INTENT = 'create_skill';

export const SKILL_CREATION_INITIAL_MESSAGE =
  '我想创建一个技能，请先询问我技能用途、使用场景和期望输出。';

export const SKILL_CREATION_HINT =
  '描述你想创建的技能、使用场景和期望输出，专属助理会引导你完成创建。';

export interface SkillCreationNavigationState {
  workbenchIntent: typeof SKILL_CREATION_INTENT;
  initialMessage: typeof SKILL_CREATION_INITIAL_MESSAGE;
}

export const createSkillCreationNavigationState =
  (): SkillCreationNavigationState => ({
    workbenchIntent: SKILL_CREATION_INTENT,
    initialMessage: SKILL_CREATION_INITIAL_MESSAGE,
  });

export const isSkillCreationNavigationState = (
  state: unknown,
): state is SkillCreationNavigationState => {
  if (!state || typeof state !== 'object') {
    return false;
  }

  const candidate = state as Partial<SkillCreationNavigationState>;

  return (
    candidate.workbenchIntent === SKILL_CREATION_INTENT &&
    candidate.initialMessage === SKILL_CREATION_INITIAL_MESSAGE
  );
};
