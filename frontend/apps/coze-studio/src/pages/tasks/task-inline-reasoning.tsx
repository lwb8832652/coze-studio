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

import {
  IconCozArrowDown,
  IconCozLightbulb,
} from '@coze-arch/coze-design/icons';

export const TaskInlineReasoning = ({ content }: { content?: string }) => {
  const value = content?.trim();

  if (!value) {
    return null;
  }

  return (
    <details className="coze-prototype-inline-reasoning">
      <summary>
        <span
          className="coze-prototype-inline-reasoning-icon"
          aria-hidden="true"
        >
          <IconCozLightbulb />
        </span>
        <span className="coze-prototype-inline-reasoning-label">思考</span>
        <span
          className="coze-prototype-inline-reasoning-chevron"
          aria-hidden="true"
        >
          <IconCozArrowDown />
        </span>
      </summary>
      <div className="coze-prototype-inline-reasoning-content">{value}</div>
    </details>
  );
};
