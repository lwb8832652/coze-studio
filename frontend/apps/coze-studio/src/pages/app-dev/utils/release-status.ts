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

interface AppDevReleaseState {
  lastBuildStatus?: string;
  sourceUpdatedAt?: string;
  lastBuildAt?: string;
}

const parseAppDevTime = (value?: string) => {
  if (!value) {
    return 0;
  }

  const timestamp = Date.parse(value);
  return Number.isFinite(timestamp) ? timestamp : 0;
};

export const isAppDevReleaseStale = (project: AppDevReleaseState) =>
  project.lastBuildStatus === 'success' &&
  parseAppDevTime(project.sourceUpdatedAt) > 0 &&
  parseAppDevTime(project.lastBuildAt) > 0 &&
  parseAppDevTime(project.sourceUpdatedAt) >
    parseAppDevTime(project.lastBuildAt);
