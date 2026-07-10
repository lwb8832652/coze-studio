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

interface CreateProjectModalProps {
  visible: boolean;
  loading?: boolean;
  onCancel: () => void;
  onSubmit: (payload: { name: string; prompt: string }) => Promise<void>;
}

export const CreateProjectModal = ({
  visible,
  loading,
  onCancel,
  onSubmit,
}: CreateProjectModalProps) => {
  const [name, setName] = useState('');
  const [prompt, setPrompt] = useState('');
  const [error, setError] = useState('');

  if (!visible) {
    return null;
  }

  const handleSubmit = async () => {
    if (loading) {
      return;
    }

    const nextName = name.trim();
    const nextPrompt = prompt.trim();

    if (!nextName) {
      setError('请输入应用名称');
      return;
    }

    if (!nextPrompt) {
      setError('请输入初始需求');
      return;
    }

    setError('');

    try {
      await onSubmit({
        name: nextName,
        prompt: nextPrompt,
      });
      setName('');
      setPrompt('');
    } catch (submitError) {
      setError(
        submitError instanceof Error
          ? submitError.message
          : '创建网页应用失败，请稍后重试',
      );
    }
  };

  return (
    <div className="app-dev-modal-mask" role="presentation">
      <section
        className="app-dev-modal app-dev-modal--page-create"
        role="dialog"
        aria-modal="true"
      >
        <header className="app-dev-modal__header">
          <div>
            <h2>创建网页应用</h2>
            <p>描述你想构建的网页，AI 会进入开发工作台继续实现。</p>
          </div>
          <button type="button" onClick={onCancel} disabled={loading}>
            关闭
          </button>
        </header>

        <div className="app-dev-create-type-card">
          <span>创建方式</span>
          <strong>在线开发</strong>
          <em>React 模板</em>
        </div>

        <label className="app-dev-form-field">
          <span>应用名称</span>
          <input
            value={name}
            maxLength={50}
            disabled={loading}
            placeholder="例如：活动落地页"
            onChange={event => {
              setName(event.target.value);
              setError('');
            }}
          />
        </label>

        <label className="app-dev-form-field">
          <span>应用描述</span>
          <textarea
            value={prompt}
            maxLength={2000}
            disabled={loading}
            placeholder="请描述页面目标、内容模块、风格和交互要求"
            onChange={event => {
              setPrompt(event.target.value);
              setError('');
            }}
          />
        </label>

        {error ? <div className="app-dev-form-error">{error}</div> : null}

        <footer className="app-dev-modal__footer">
          <button type="button" onClick={onCancel} disabled={loading}>
            取消
          </button>
          <button type="button" onClick={handleSubmit} disabled={loading}>
            {loading ? '创建中...' : '创建并进入开发'}
          </button>
        </footer>
      </section>
    </div>
  );
};
