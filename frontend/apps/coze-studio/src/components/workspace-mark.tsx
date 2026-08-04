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

import { useState } from 'react';

import { useCommonConfigStore } from '@coze-foundation/global-store';

export const WorkspaceMark = ({
  variant = 'workspace',
}: {
  variant?: 'assistant' | 'workspace';
}) => {
  const siteLogoUrl = useCommonConfigStore(
    state => state.siteConfig.siteLogoUrl,
  );
  const [failedLogoUrl, setFailedLogoUrl] = useState('');
  const displaySiteLogo = siteLogoUrl && siteLogoUrl !== failedLogoUrl;

  return (
    <span
      className={
        variant === 'assistant'
          ? 'coze-prototype-assistant-avatar'
          : 'coze-prototype-workspace-mark'
      }
      aria-hidden="true"
    >
      {displaySiteLogo ? (
        <img
          alt=""
          className="h-full w-full rounded-[inherit] object-contain"
          src={siteLogoUrl}
          onError={() => setFailedLogoUrl(siteLogoUrl)}
        />
      ) : (
        <svg
          viewBox="0 0 24 24"
          className="h-[16px] w-[16px]"
          fill="currentColor"
        >
          <path d="M12 2 2 22h20L12 2zm0 6 6 12H6l6-12z" />
        </svg>
      )}
    </span>
  );
};
