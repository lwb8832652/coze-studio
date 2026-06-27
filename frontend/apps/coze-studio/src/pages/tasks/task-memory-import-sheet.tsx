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

import { Button, SideSheet, TextArea } from '@coze-arch/coze-design';

export const MemoryImportSheet = ({
  error,
  importing,
  value,
  visible,
  onCancel,
  onChange,
  onImport,
}: {
  error?: string;
  importing: boolean;
  value: string;
  visible: boolean;
  onCancel: () => void;
  onChange: (value: string) => void;
  onImport: () => void | Promise<void>;
}) => (
  <SideSheet
    title="导入任务记忆"
    visible={visible}
    onCancel={onCancel}
    width={560}
  >
    <div
      className="coze-prototype-memory-editor"
      data-testid="task-memory-import-sheet"
    >
      {error ? (
        <div className="coze-prototype-memory-error" role="alert">
          {error}
        </div>
      ) : null}
      <TextArea
        aria-label="导入任务记忆 JSON"
        data-testid="task-memory-import-input"
        value={value}
        rows={12}
        autosize={false}
        placeholder="粘贴任务记忆导出 JSON"
        onChange={onChange}
      />
      <div className="coze-prototype-memory-editor-actions">
        <Button theme="borderless" type="tertiary" onClick={onCancel}>
          取消
        </Button>
        <Button
          theme="solid"
          type="primary"
          data-testid="task-memory-import-submit"
          loading={importing}
          onClick={onImport}
        >
          开始导入
        </Button>
      </div>
    </div>
  </SideSheet>
);
