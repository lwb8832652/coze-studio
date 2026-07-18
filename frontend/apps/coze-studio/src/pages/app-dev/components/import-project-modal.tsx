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

import { AccessibleDialog } from './accessible-dialog';

import { formatFileSize } from '../utils/file-utils';

interface ImportProjectModalProps {
  visible: boolean;
  loading?: boolean;
  onCancel: () => void;
  onSubmit: (payload: { file: File; name?: string }) => Promise<void>;
}

export const ImportProjectModal = ({
  visible,
  loading,
  onCancel,
  onSubmit,
}: ImportProjectModalProps) => {
  const [file, setFile] = useState<File | undefined>();
  const [name, setName] = useState('');
  const [error, setError] = useState('');

  if (!visible) {
    return null;
  }

  const handleSubmit = async () => {
    if (loading) {
      return;
    }

    if (!file) {
      setError('请选择项目压缩包');
      return;
    }

    if (!file.name.toLowerCase().endsWith('.zip')) {
      setError('仅支持导入 .zip 项目压缩包');
      return;
    }
    if (file.size > 100 * 1024 * 1024) {
      setError('项目压缩包不能超过 100MB');
      return;
    }

    setError('');

    try {
      await onSubmit({
        file,
        name: name.trim() || undefined,
      });
      setFile(undefined);
      setName('');
    } catch (submitError) {
      setError(
        submitError instanceof Error
          ? submitError.message
          : '导入网页应用失败，请稍后重试',
      );
    }
  };

  return (
    <AccessibleDialog
      title="导入网页应用"
      className="app-dev-modal app-dev-modal--page-create"
      onClose={() => {
        if (!loading) {
          onCancel();
        }
      }}
    >
      <header className="app-dev-modal__header">
        <div>
          <h2>导入网页应用</h2>
          <p>上传 zip 项目包后进入网页应用开发工作台。</p>
        </div>
        <button type="button" onClick={onCancel} disabled={loading}>
          关闭
        </button>
      </header>

      <label className="app-dev-form-field">
        <span>项目名称</span>
        <input
          data-dialog-autofocus
          value={name}
          maxLength={50}
          disabled={loading}
          placeholder="可选，默认使用压缩包名称"
          onChange={event => setName(event.target.value)}
        />
      </label>

      <label className="app-dev-upload-field app-dev-upload-field--dragger">
        <strong aria-hidden="true">⇧</strong>
        <span>
          {file
            ? `${file.name} · ${formatFileSize(file.size)}`
            : '选择 .zip 项目包'}
        </span>
        <p>点击或拖拽 zip 文件到这里上传</p>
        <input
          type="file"
          accept=".zip,application/zip"
          disabled={loading}
          onChange={event => {
            setFile(event.target.files?.[0]);
            setError('');
          }}
        />
      </label>

      {error ? <div className="app-dev-form-error">{error}</div> : null}

      <footer className="app-dev-modal__footer">
        <button type="button" onClick={onCancel} disabled={loading}>
          取消
        </button>
        <button
          type="button"
          onClick={handleSubmit}
          disabled={loading || !file}
        >
          {loading ? '导入中...' : '导入并进入开发'}
        </button>
      </footer>
    </AccessibleDialog>
  );
};
