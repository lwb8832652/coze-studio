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

/* eslint-disable @coze-arch/max-line-per-function -- Authorization modal. */

import { useEffect, useMemo, useState } from 'react';

import type {
  AdminManagedModel,
  AdminModelGrantSubject,
  AdminUser,
  AdminWorkspace,
} from './service';

const WORKSPACE_SUBJECT = 1;
const USER_SUBJECT = 2;

interface ModelGrantsDialogProps {
  grants: AdminModelGrantSubject[];
  loading: boolean;
  model: AdminManagedModel | null;
  open: boolean;
  users: AdminUser[];
  workspaces: AdminWorkspace[];
  onCancel: () => void;
  onSave: (grants: AdminModelGrantSubject[]) => void | Promise<void>;
}

export const ModelGrantsDialog = ({
  grants,
  loading,
  model,
  open,
  users,
  workspaces,
  onCancel,
  onSave,
}: ModelGrantsDialogProps) => {
  const [activeTab, setActiveTab] = useState<'workspace' | 'user'>('workspace');
  const [selectedWorkspaces, setSelectedWorkspaces] = useState<string[]>([]);
  const [selectedUsers, setSelectedUsers] = useState<string[]>([]);

  useEffect(() => {
    if (!open) {
      return;
    }
    setActiveTab('workspace');
    setSelectedWorkspaces(
      grants
        .filter(grant => grant.subject_type === WORKSPACE_SUBJECT)
        .map(grant => String(grant.subject_id)),
    );
    setSelectedUsers(
      grants
        .filter(grant => grant.subject_type === USER_SUBJECT)
        .map(grant => String(grant.subject_id)),
    );
  }, [grants, open]);

  const allIDs = useMemo(
    () =>
      activeTab === 'workspace'
        ? workspaces.map(workspace => String(workspace.id))
        : users.map(user => String(user.user_id)),
    [activeTab, users, workspaces],
  );
  const selected =
    activeTab === 'workspace' ? selectedWorkspaces : selectedUsers;
  const allSelected =
    allIDs.length > 0 && allIDs.every(id => selected.includes(id));

  if (!open || !model) {
    return null;
  }

  const toggle = (id: string) => {
    const setter =
      activeTab === 'workspace' ? setSelectedWorkspaces : setSelectedUsers;
    setter(current =>
      current.includes(id)
        ? current.filter(currentID => currentID !== id)
        : [...current, id],
    );
  };

  const toggleAll = () => {
    const setter =
      activeTab === 'workspace' ? setSelectedWorkspaces : setSelectedUsers;
    setter(allSelected ? [] : allIDs);
  };

  const submit = () =>
    onSave([
      ...selectedWorkspaces.map(subjectID => ({
        subject_id: subjectID,
        subject_type: WORKSPACE_SUBJECT,
      })),
      ...selectedUsers.map(subjectID => ({
        subject_id: subjectID,
        subject_type: USER_SUBJECT,
      })),
    ]);

  return (
    <div className="coze-prototype-model-dialog-backdrop">
      <section
        aria-labelledby="system-model-grants-title"
        aria-modal="true"
        className="coze-prototype-model-dialog coze-prototype-model-grants-dialog"
        role="dialog"
      >
        <header className="coze-prototype-model-dialog-header">
          <div>
            <h2 id="system-model-grants-title">授权 - {model.name}</h2>
            <p>仅选中的工作空间和用户可以使用该模型。</p>
          </div>
          <button
            aria-label="关闭模型授权弹窗"
            type="button"
            onClick={onCancel}
          >
            关闭
          </button>
        </header>
        <div className="coze-prototype-model-grants-tabs">
          <div role="tablist">
            <button
              aria-selected={activeTab === 'workspace'}
              role="tab"
              type="button"
              onClick={() => setActiveTab('workspace')}
            >
              工作空间
            </button>
            <button
              aria-selected={activeTab === 'user'}
              role="tab"
              type="button"
              onClick={() => setActiveTab('user')}
            >
              用户
            </button>
          </div>
          {allIDs.length > 0 ? (
            <button type="button" onClick={toggleAll}>
              {allSelected ? '取消全选' : '全选'}
            </button>
          ) : null}
        </div>
        <div className="coze-prototype-model-grants-list" role="tabpanel">
          {activeTab === 'workspace'
            ? workspaces.map(workspace => {
                const id = String(workspace.id);
                return (
                  <label key={id}>
                    <input
                      checked={selectedWorkspaces.includes(id)}
                      type="checkbox"
                      onChange={() => toggle(id)}
                    />
                    <span>
                      <strong>{workspace.name || workspace.id}</strong>
                      <small>{workspace.owner_name || '工作空间'}</small>
                    </span>
                  </label>
                );
              })
            : users.map(user => {
                const id = String(user.user_id);
                return (
                  <label key={id}>
                    <input
                      checked={selectedUsers.includes(id)}
                      type="checkbox"
                      onChange={() => toggle(id)}
                    />
                    <span>
                      <strong>{user.name || user.email || user.user_id}</strong>
                      <small>
                        {user.email || user.user_unique_name || '用户'}
                      </small>
                    </span>
                  </label>
                );
              })}
          {allIDs.length === 0 ? <p>暂无可授权对象</p> : null}
        </div>
        <footer className="coze-prototype-model-dialog-footer">
          <p>已选择 {selectedWorkspaces.length + selectedUsers.length} 项</p>
          <div>
            <button disabled={loading} type="button" onClick={onCancel}>
              取消
            </button>
            <button
              className="is-primary"
              disabled={loading}
              type="button"
              onClick={() => void submit()}
            >
              {loading ? '保存中...' : '确认'}
            </button>
          </div>
        </footer>
      </section>
    </div>
  );
};
