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
  IconCozEdit,
  IconCozHistory,
  IconCozRefresh,
  IconCozTrashCan,
} from '@coze-arch/coze-design/icons';
import { Button, Popconfirm, Tag } from '@coze-arch/coze-design';

import {
  buildMemoryMeta,
  isDeletedMemory,
  scopeLabel,
} from './task-memory-section-utils';
import type { TaskThreadMemory } from './service';

export const TaskMemoryRow = ({
  activeAction,
  memory,
  readOnly,
  onAudit,
  onDelete,
  onEdit,
  onRestore,
}: {
  activeAction: string;
  memory: TaskThreadMemory;
  readOnly: boolean;
  onAudit: (memory: TaskThreadMemory) => void | Promise<void>;
  onDelete: (memory: TaskThreadMemory) => void | Promise<void>;
  onEdit: (memory: TaskThreadMemory) => void;
  onRestore: (memory: TaskThreadMemory) => void | Promise<void>;
}) => (
  <li
    className={
      isDeletedMemory(memory)
        ? 'coze-prototype-memory-row coze-prototype-memory-deleted'
        : 'coze-prototype-memory-row'
    }
    data-memory-id={memory.memory_id}
    data-testid="task-memory-row"
  >
    <span className="coze-prototype-step-check">✓</span>
    <span className="coze-prototype-memory-main">
      <span className="coze-prototype-memory-title-row">
        <span className="coze-prototype-memory-content">{memory.content}</span>
        <Tag>{scopeLabel(memory.scope)}</Tag>
        {isDeletedMemory(memory) ? <Tag>已删除</Tag> : null}
      </span>
      <span className="coze-prototype-memory-meta">
        {buildMemoryMeta(memory)}
      </span>
    </span>
    <span className="coze-prototype-memory-actions">
      <Button
        icon={<IconCozHistory />}
        size="small"
        theme="borderless"
        type="tertiary"
        onClick={() => void onAudit(memory)}
      >
        审计
      </Button>
      {isDeletedMemory(memory) ? (
        readOnly ? (
          <Button
            disabled
            icon={<IconCozRefresh />}
            size="small"
            theme="borderless"
            type="primary"
          >
            恢复
          </Button>
        ) : (
          <Popconfirm
            title="恢复记忆"
            content="恢复后该条记忆会重新进入默认任务记忆列表。"
            okText="恢复"
            cancelText="取消"
            cancelButtonProps={{ autoFocus: true }}
            onConfirm={() => onRestore(memory)}
          >
            <Button
              icon={<IconCozRefresh />}
              loading={activeAction === `restore:${memory.memory_id}`}
              size="small"
              theme="borderless"
              type="primary"
            >
              恢复
            </Button>
          </Popconfirm>
        )
      ) : (
        <>
          <Button
            disabled={readOnly}
            icon={<IconCozEdit />}
            size="small"
            theme="borderless"
            type="primary"
            onClick={() => onEdit(memory)}
          >
            编辑
          </Button>
          {readOnly ? (
            <Button
              disabled
              icon={<IconCozTrashCan />}
              size="small"
              theme="borderless"
              type="danger"
            >
              删除
            </Button>
          ) : (
            <Popconfirm
              title="删除记忆"
              content="只会软删除该条记忆，不会暴露底层运行输入或工具结果。"
              okText="删除"
              cancelText="取消"
              okType="danger"
              cancelButtonProps={{ autoFocus: true }}
              onConfirm={() => onDelete(memory)}
            >
              <Button
                icon={<IconCozTrashCan />}
                loading={activeAction === `delete:${memory.memory_id}`}
                size="small"
                theme="borderless"
                type="danger"
              >
                删除
              </Button>
            </Popconfirm>
          )}
        </>
      )}
    </span>
  </li>
);
