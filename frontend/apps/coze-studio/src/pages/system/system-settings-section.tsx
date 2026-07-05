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

import type {
  AdminBasicConfig,
  AdminKnowledgeConfig,
} from './service';

interface SystemSettingsSectionProps {
  basicConfig: AdminBasicConfig;
  knowledgeConfig: AdminKnowledgeConfig;
}

export const SystemSettingsSection = ({
  basicConfig,
  knowledgeConfig,
}: SystemSettingsSectionProps) => (
  <section className="coze-prototype-workspace-settings-list">
      <article className="coze-prototype-workspace-settings-row">
        <div>
          <h2>服务地址</h2>
          <p>{basicConfig.server_host || '未配置'}</p>
        </div>
        <span>基础服务</span>
      </article>
      <article className="coze-prototype-workspace-settings-row">
        <div>
          <h2>注册开关</h2>
          <p>
            {basicConfig.disable_user_registration ? '已关闭' : '允许用户注册'}
          </p>
        </div>
        <span>账号</span>
      </article>
      <article className="coze-prototype-workspace-settings-row">
        <div>
          <h2>管理员邮箱</h2>
          <p>{basicConfig.admin_emails || '未配置'}</p>
        </div>
        <span>权限</span>
      </article>
      <article className="coze-prototype-workspace-settings-row">
        <div>
          <h2>知识库配置</h2>
          <p>
            内置模型 ID：
            {knowledgeConfig.builtin_model_id ?? '未配置'}
          </p>
          <p>
            Embedding：
            {knowledgeConfig.embedding_config ? '已配置' : '未配置'}
          </p>
        </div>
        <span>只读</span>
      </article>
  </section>
);
