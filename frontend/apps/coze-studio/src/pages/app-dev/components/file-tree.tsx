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

import { useMemo, useState } from 'react';

import { formatFileSize } from '../utils/file-utils';
import type { AppDevFileNode } from '../types';
import {
  type TextInputDialogOptions,
  type ConfirmDialogOptions,
  useConfirmDialog,
  useTextInputDialog,
} from './text-input-dialog';

interface FileTreeProps {
  tree: AppDevFileNode[];
  selectedPath?: string;
  loading?: boolean;
  onRefresh: () => void;
  onOpenFile: (path: string) => void;
  onCreateFile: (path: string) => void;
  onCreateDirectory: (path: string) => void;
  onUploadFiles: (
    files: File[],
    options?: { basePath?: string; preserveRelativePath?: boolean },
  ) => void;
  onRenamePath: (sourcePath: string, targetPath: string) => void;
  onDeletePath: (path: string) => void;
}

const FileNodeItem = ({
  node,
  selectedPath,
  depth,
  collapsedPaths,
  onOpenFile,
  onToggleDirectory,
  onRenamePath,
  onDeletePath,
  requestTextInput,
  requestConfirm,
}: {
  node: AppDevFileNode;
  selectedPath?: string;
  depth: number;
  collapsedPaths: Set<string>;
  onOpenFile: (path: string) => void;
  onToggleDirectory: (path: string) => void;
  onRenamePath: (sourcePath: string, targetPath: string) => void;
  onDeletePath: (path: string) => void;
  requestTextInput: (options: TextInputDialogOptions) => Promise<string | null>;
  requestConfirm: (options: ConfirmDialogOptions) => Promise<boolean>;
}) => {
  const isDirectory = node.type === 'directory';
  const expanded = isDirectory && !collapsedPaths.has(node.path);
  if (node.name === '.gitkeep') {
    return null;
  }

  return (
    <li>
      <div
        className="app-dev-file-tree__node"
        data-selected={selectedPath === node.path}
        style={{ paddingLeft: 12 + depth * 14 }}
      >
        <button
          type="button"
          className="app-dev-file-tree__node-main"
          aria-expanded={isDirectory ? expanded : undefined}
          onClick={() => {
            if (isDirectory) {
              onToggleDirectory(node.path);
            } else {
              onOpenFile(node.path);
            }
          }}
        >
          <span aria-hidden="true">
            {isDirectory ? (expanded ? '▾' : '▸') : '•'}
          </span>
          <span>{node.name}</span>
        </button>
        {!isDirectory ? <small>{formatFileSize(node.size)}</small> : null}
        <span className="app-dev-file-tree__actions">
          <button
            type="button"
            onClick={() => {
              void (async () => {
                const nextPath = await requestTextInput({
                  title: '重命名路径',
                  description: `修改「${node.path}」的文件或目录路径。`,
                  label: '新路径',
                  initialValue: node.path,
                  placeholder: '例如 src/pages/Home.tsx',
                  confirmText: '重命名',
                });
                if (nextPath && nextPath !== node.path) {
                  onRenamePath(node.path, nextPath);
                }
              })();
            }}
          >
            重命名
          </button>
          <button
            type="button"
            onClick={() => {
              void (async () => {
                const confirmed = await requestConfirm({
                  title: '删除文件',
                  description: `确认删除「${node.path}」吗？此操作不可撤销。`,
                  confirmText: '删除',
                });
                if (confirmed) {
                  onDeletePath(node.path);
                }
              })();
            }}
          >
            删除
          </button>
        </span>
      </div>
      {isDirectory && expanded && node.children?.length ? (
        <ul>
          {node.children.map(child => (
            <FileNodeItem
              key={child.path}
              node={child}
              selectedPath={selectedPath}
              depth={depth + 1}
              collapsedPaths={collapsedPaths}
              onOpenFile={onOpenFile}
              onToggleDirectory={onToggleDirectory}
              onRenamePath={onRenamePath}
              onDeletePath={onDeletePath}
              requestTextInput={requestTextInput}
              requestConfirm={requestConfirm}
            />
          ))}
        </ul>
      ) : null}
    </li>
  );
};

const collectDirectoryPaths = (nodes: AppDevFileNode[]): string[] =>
  nodes.flatMap(node => {
    if (node.type !== 'directory') {
      return [];
    }

    return [node.path, ...collectDirectoryPaths(node.children || [])];
  });

const countFiles = (nodes: AppDevFileNode[]): number =>
  nodes.reduce(
    (total, node) =>
      total + (node.type === 'file' ? 1 : countFiles(node.children || [])),
    0,
  );

const filterFileTree = (
  nodes: AppDevFileNode[],
  keyword: string,
): AppDevFileNode[] => {
  const normalizedKeyword = keyword.trim().toLowerCase();
  if (!normalizedKeyword) {
    return nodes;
  }

  return nodes.flatMap(node => {
    const children = filterFileTree(node.children || [], normalizedKeyword);
    const matched =
      node.name.toLowerCase().includes(normalizedKeyword) ||
      node.path.toLowerCase().includes(normalizedKeyword);

    if (!matched && !children.length) {
      return [];
    }

    return [
      {
        ...node,
        children: matched ? node.children : children,
      },
    ];
  });
};

export const FileTree = ({
  tree,
  selectedPath,
  loading,
  onRefresh,
  onOpenFile,
  onCreateFile,
  onCreateDirectory,
  onUploadFiles,
  onRenamePath,
  onDeletePath,
}: FileTreeProps) => {
  const [collapsedPaths, setCollapsedPaths] = useState<Set<string>>(new Set());
  const [keyword, setKeyword] = useState('');
  const { openTextInputDialog, textInputDialog } = useTextInputDialog();
  const { openConfirmDialog, confirmDialog } = useConfirmDialog();
  const visibleTree = useMemo(
    () => filterFileTree(tree, keyword),
    [keyword, tree],
  );
  const fileCount = useMemo(() => countFiles(tree), [tree]);

  const toggleDirectory = (path: string) => {
    setCollapsedPaths(current => {
      const next = new Set(current);
      if (next.has(path)) {
        next.delete(path);
      } else {
        next.add(path);
      }
      return next;
    });
  };

  return (
    <aside className="app-dev-file-tree">
      <header>
        <div className="app-dev-file-tree__title">
          <span>项目文件</span>
          <small>{fileCount} files</small>
        </div>
        <div className="app-dev-file-tree__header-actions">
          <button
            type="button"
            onClick={() => {
              void (async () => {
                const path = await openTextInputDialog({
                  title: '新建文件',
                  description: '输入要创建的项目相对路径。',
                  label: '文件路径',
                  placeholder: '例如 src/pages/Home.tsx',
                  confirmText: '创建',
                });
                if (path) {
                  onCreateFile(path);
                }
              })();
            }}
            disabled={loading}
          >
            新建文件
          </button>
          <button
            type="button"
            onClick={() => {
              void (async () => {
                const path = await openTextInputDialog({
                  title: '新建目录',
                  description: '输入要创建的项目相对目录路径。',
                  label: '目录路径',
                  placeholder: '例如 src/pages',
                  confirmText: '创建',
                });
                if (path) {
                  onCreateDirectory(path);
                }
              })();
            }}
            disabled={loading}
          >
            新建目录
          </button>
          <button
            type="button"
            onClick={() => setCollapsedPaths(new Set())}
            disabled={loading}
          >
            展开
          </button>
          <button
            type="button"
            onClick={() =>
              setCollapsedPaths(new Set(collectDirectoryPaths(tree)))
            }
            disabled={loading}
          >
            收起
          </button>
          <label className="app-dev-file-tree__upload">
            上传文件
            <input
              type="file"
              multiple
              onChange={event => {
                const files = Array.from(event.target.files || []);
                const input = event.currentTarget;
                void (async () => {
                  if (files.length) {
                    const basePath = await openTextInputDialog({
                      title: '上传文件',
                      description: '选择文件上传到的目录，留空表示项目根目录。',
                      label: '目标目录',
                      placeholder: '例如 src/assets',
                      confirmText: '上传',
                      allowEmpty: true,
                    });
                    if (basePath !== null) {
                      onUploadFiles(files, { basePath });
                    }
                  }
                  input.value = '';
                })();
              }}
              disabled={loading}
            />
          </label>
          <label className="app-dev-file-tree__upload">
            上传目录
            <input
              type="file"
              multiple
              {...{
                webkitdirectory: '',
                directory: '',
              }}
              onChange={event => {
                const files = Array.from(event.target.files || []);
                const input = event.currentTarget;
                void (async () => {
                  if (files.length) {
                    const basePath = await openTextInputDialog({
                      title: '上传目录',
                      description:
                        '选择目录上传到的目标位置，留空表示项目根目录。',
                      label: '目标目录',
                      placeholder: '例如 src/assets',
                      confirmText: '上传',
                      allowEmpty: true,
                    });
                    if (basePath !== null) {
                      onUploadFiles(files, {
                        basePath,
                        preserveRelativePath: true,
                      });
                    }
                  }
                  input.value = '';
                })();
              }}
              disabled={loading}
            />
          </label>
          <button type="button" onClick={onRefresh} disabled={loading}>
            刷新
          </button>
        </div>
      </header>

      <label className="app-dev-file-tree__search">
        <span>搜索文件</span>
        <input
          value={keyword}
          placeholder="输入文件名或路径"
          onChange={event => setKeyword(event.target.value)}
        />
      </label>

      {loading ? (
        <div className="app-dev-file-tree__state">加载文件树...</div>
      ) : null}

      {!loading && !tree.length ? (
        <div className="app-dev-file-tree__state">暂无文件</div>
      ) : null}

      {!loading && tree.length && !visibleTree.length ? (
        <div className="app-dev-file-tree__state">未找到匹配文件</div>
      ) : null}

      {!loading && visibleTree.length ? (
        <ul>
          {visibleTree.map(node => (
            <FileNodeItem
              key={node.path}
              node={node}
              selectedPath={selectedPath}
              depth={0}
              collapsedPaths={collapsedPaths}
              onOpenFile={onOpenFile}
              onToggleDirectory={toggleDirectory}
              onRenamePath={onRenamePath}
              onDeletePath={onDeletePath}
              requestTextInput={openTextInputDialog}
              requestConfirm={openConfirmDialog}
            />
          ))}
        </ul>
      ) : null}
      {textInputDialog}
      {confirmDialog}
    </aside>
  );
};
