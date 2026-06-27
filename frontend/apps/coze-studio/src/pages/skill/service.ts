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

export const listSkills = workbenchSkill.ListSkills;
export const listSkillToolCandidates = workbenchSkill.ListSkillToolCandidates;
export const importSkill = workbenchSkill.ImportSkill;
export const createSkill = workbenchSkill.CreateSkill;
export const updateSkill = workbenchSkill.UpdateSkill;
export const deleteSkill = workbenchSkill.DeleteSkill;
export const testRunSkill = workbenchSkill.TestRunSkill;
export const exportSkill = workbenchSkill.ExportSkill;
export const listSkillVersions = workbenchSkill.ListSkillVersions;
export const listSkillVersionResources =
  workbenchSkill.ListSkillVersionResources;
export const updateSkillVersionContent =
  workbenchSkill.UpdateSkillVersionContent;
export const updateSkillVersionResource =
  workbenchSkill.UpdateSkillVersionResource;
export const exportSkillVersion = workbenchSkill.ExportSkillVersion;
export const rollbackSkillVersion = workbenchSkill.RollbackSkillVersion;
