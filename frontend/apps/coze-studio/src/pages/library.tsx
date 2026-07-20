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

import { LibraryPage } from '@coze-studio/workspace-adapter/library';

import { WorkspacePageTopBar } from '../components/workspace-page-top-bar';

const Page = () => {
  const { space_id } = useParams();
  return space_id ? (
    <main className="newx-menu-page newx-menu-page--library">
      <WorkspacePageTopBar />
      <div className="newx-menu-page__adapter-content">
        <LibraryPage spaceId={space_id} />
      </div>
    </main>
  ) : null;
};

export default Page;
