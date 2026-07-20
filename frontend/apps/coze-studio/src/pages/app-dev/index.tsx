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

/* eslint-disable @coze-arch/max-line-per-function -- Cohesive orchestrator. */

import { useNavigate, useParams } from 'react-router-dom';
import { useState } from 'react';

import { WorkspacePageTopBar } from '../../components/workspace-page-top-bar';
import { exportAppDevProject } from './service';
import { useAppDevProjects } from './hooks/use-app-dev-projects';
import {
  useConfirmDialog,
  useTextInputDialog,
} from './components/text-input-dialog';
import { ProjectList } from './components/project-list';
import { ImportProjectModal } from './components/import-project-modal';
import { CreateProjectModal } from './components/create-project-modal';

import './index.less';
import './newx-refresh.less';

export default function AppDevPage() {
  const navigate = useNavigate();
  const { space_id: spaceId = '' } = useParams();
  const [createModalVisible, setCreateModalVisible] = useState(false);
  const [importModalVisible, setImportModalVisible] = useState(false);
  const [notice, setNotice] = useState('');
  const { openTextInputDialog, textInputDialog } = useTextInputDialog();
  const { openConfirmDialog, confirmDialog } = useConfirmDialog();
  const {
    projects,
    total,
    keyword,
    setKeyword,
    loading,
    creating,
    importing,
    error,
    refresh,
    createProject,
    importProject,
    archiveProject,
    updateProject,
    duplicateProject,
  } = useAppDevProjects(spaceId);

  return (
    <main className="app-dev-page newx-menu-page">
      <WorkspacePageTopBar />
      {notice ? <div className="app-dev-page__notice">{notice}</div> : null}

      <ProjectList
        projects={projects}
        total={total}
        loading={loading}
        error={error}
        keyword={keyword}
        onKeywordChange={setKeyword}
        onRefresh={() => void refresh()}
        onCreate={() => setCreateModalVisible(true)}
        onImport={() => setImportModalVisible(true)}
        onOpen={project => navigate(`/space/${spaceId}/app-dev/${project.id}`)}
        onRename={project => {
          void (async () => {
            const name = await openTextInputDialog({
              title: '重命名项目',
              description: `更新「${project.name}」的名称和描述。`,
              label: '项目名称',
              initialValue: project.name,
              placeholder: '请输入新的项目名称',
              confirmText: '下一步',
              maxLength: 50,
            });
            if (!name || name === project.name) {
              return;
            }
            const normalizedName = name.trim();
            if (!normalizedName) {
              setNotice('项目名称不能为空');
              return;
            }
            if (normalizedName.length > 50) {
              setNotice('项目名称不能超过 50 个字符');
              return;
            }

            const description = await openTextInputDialog({
              title: '项目描述',
              description: '可选填写，用于在列表中说明项目用途。',
              label: '项目描述',
              initialValue: project.description || '',
              placeholder: '请输入项目描述（可选）',
              confirmText: '保存',
              allowEmpty: true,
              maxLength: 200,
            });
            if (description === null) {
              return;
            }
            const normalizedDescription = description?.trim() || '';
            if (normalizedDescription.length > 200) {
              setNotice('项目描述不能超过 200 个字符');
              return;
            }

            void updateProject({
              projectId: project.id,
              name: normalizedName,
              description: normalizedDescription,
            })
              .then(updated => setNotice(`已更新「${updated.name}」`))
              .catch(caughtError => {
                setNotice(
                  caughtError instanceof Error
                    ? caughtError.message
                    : '更新项目失败',
                );
              });
          })();
        }}
        onDuplicate={project => {
          void (async () => {
            const name = await openTextInputDialog({
              title: '复制项目',
              description: `基于「${project.name}」创建一个新副本。`,
              label: '副本项目名称',
              initialValue: `${project.name} 副本`,
              placeholder: '请输入副本项目名称',
              confirmText: '复制',
              maxLength: 50,
            });
            if (!name) {
              return;
            }
            const normalizedName = name.trim();
            if (!normalizedName) {
              setNotice('副本项目名称不能为空');
              return;
            }
            if (normalizedName.length > 50) {
              setNotice('副本项目名称不能超过 50 个字符');
              return;
            }

            void duplicateProject({
              projectId: project.id,
              name: normalizedName,
            })
              .then(created => setNotice(`已复制为「${created.name}」`))
              .catch(caughtError => {
                setNotice(
                  caughtError instanceof Error
                    ? caughtError.message
                    : '复制项目失败',
                );
              });
          })();
        }}
        onExport={project => {
          void (async () => {
            setNotice('');
            try {
              const blob = await exportAppDevProject({
                spaceId,
                projectId: project.id,
              });
              const url = URL.createObjectURL(blob);
              const link = document.createElement('a');
              link.href = url;
              link.download = `${project.name || 'appdev-project'}.zip`;
              link.click();
              URL.revokeObjectURL(url);
              setNotice(`「${project.name}」源码已开始下载`);
            } catch (caughtError) {
              setNotice(
                caughtError instanceof Error
                  ? caughtError.message
                  : '导出源码失败',
              );
            }
          })();
        }}
        onArchive={project => {
          void (async () => {
            const confirmed = await openConfirmDialog({
              title: '归档项目',
              description: `确认归档「${project.name}」吗？归档后列表中将不再展示。`,
              confirmText: '归档',
            });
            if (confirmed) {
              void archiveProject(project.id)
                .then(() => setNotice(`已归档「${project.name}」`))
                .catch(caughtError => {
                  setNotice(
                    caughtError instanceof Error
                      ? caughtError.message
                      : '归档项目失败',
                  );
                });
            }
          })();
        }}
      />

      <CreateProjectModal
        visible={createModalVisible}
        loading={creating}
        onCancel={() => setCreateModalVisible(false)}
        onSubmit={async payload => {
          const project = await createProject(payload);
          setCreateModalVisible(false);
          setNotice(`已创建「${project.name}」，正在进入开发工作台`);
          navigate(`/space/${spaceId}/app-dev/${project.id}`);
        }}
      />

      <ImportProjectModal
        visible={importModalVisible}
        loading={importing}
        onCancel={() => setImportModalVisible(false)}
        onSubmit={async payload => {
          const project = await importProject(payload);
          setImportModalVisible(false);
          setNotice(`已导入「${project.name}」，正在进入开发工作台`);
          navigate(`/space/${spaceId}/app-dev/${project.id}`);
        }}
      />
      {textInputDialog}
      {confirmDialog}
    </main>
  );
}
