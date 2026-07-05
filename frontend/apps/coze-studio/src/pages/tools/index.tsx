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

import { WorkspacePageTopBar } from '../../components/workspace-page-top-bar';
import '../../components/workspace-prototype.less';
import { MCPToolSettingsPanel } from './mcp-settings-panel';

interface ToolsPageHeaderProps {
  description: string;
  title: string;
}

const ToolsPageHeader = ({ description, title }: ToolsPageHeaderProps) => (
  <header className="coze-prototype-settings-section-header">
    <h1>{title}</h1>
    <p>{description}</p>
  </header>
);

const ToolsPage = () => {
  const { space_id } = useParams();

  return (
    <main className="coze-prototype-page coze-prototype-tools-settings-page">
      <WorkspacePageTopBar />
      <section className="coze-prototype-page-inner">
        <ToolsPageHeader
          title="工具"
          description="管理 MCP 工具的配置和启用状态。"
        />
        <MCPToolSettingsPanel spaceId={space_id} />
      </section>
    </main>
  );
};

export default ToolsPage;
