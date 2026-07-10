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

/* eslint-disable @coze-arch/max-line-per-function, max-lines-per-function -- Cohesive orchestrator. */

import { useCallback, useEffect, useState } from 'react';

import type { AppDevFileContent, AppDevFileNode } from '../types';
import {
  deleteAppDevFile,
  getAppDevFileContent,
  listAppDevFiles,
  normalizeAppDevError,
  renameAppDevFile,
  saveAppDevFileContent,
  uploadAppDevFiles,
} from '../service';
import { useLatestRequest } from './use-latest-request';

const hasFileNodePath = (nodes: AppDevFileNode[], path: string): boolean =>
  nodes.some(
    node => node.path === path || hasFileNodePath(node.children || [], path),
  );

const maxUploadFileBytes = 10 * 1024 * 1024;
const maxUploadFiles = 100;
const maxUploadTotalBytes = 100 * 1024 * 1024;

const normalizeUploadPath = (path: string) =>
  path.trim().replace(/^\/+/u, '').replace(/\\/gu, '/');

export const useAppDevFiles = (spaceId?: string, projectId?: string) => {
  const [tree, setTree] = useState<AppDevFileNode[]>([]);
  const [selectedPath, setSelectedPath] = useState('');
  const [fileContent, setFileContent] = useState<
    AppDevFileContent | undefined
  >();
  const [draft, setDraft] = useState('');
  const [loadingTree, setLoadingTree] = useState(false);
  const [loadingContent, setLoadingContent] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const {
    beginRequest: beginTreeRequest,
    invalidateRequests: invalidateTreeRequests,
  } = useLatestRequest();
  const {
    beginRequest: beginContentRequest,
    invalidateRequests: invalidateContentRequests,
  } = useLatestRequest();
  const {
    beginRequest: beginMutationRequest,
    invalidateRequests: invalidateMutationRequests,
  } = useLatestRequest();

  const refreshTree = useCallback(async () => {
    const isLatest = beginTreeRequest();
    if (!spaceId || !projectId) {
      if (isLatest()) {
        setTree([]);
        setLoadingTree(false);
      }
      return;
    }

    setLoadingTree(true);
    setError('');

    try {
      const result = await listAppDevFiles({ spaceId, projectId });
      if (isLatest()) {
        setTree(result.items);
      }
    } catch (requestError) {
      if (isLatest()) {
        setError(normalizeAppDevError(requestError));
      }
    } finally {
      if (isLatest()) {
        setLoadingTree(false);
      }
    }
  }, [beginTreeRequest, projectId, spaceId]);

  const openFile = useCallback(
    async (path: string) => {
      const isLatest = beginContentRequest();
      if (!spaceId || !projectId) {
        return;
      }

      setSelectedPath(path);
      setLoadingContent(true);
      setError('');

      try {
        const content = await getAppDevFileContent({
          spaceId,
          projectId,
          path,
        });
        if (isLatest()) {
          setFileContent(content);
          setDraft(content.content);
        }
      } catch (requestError) {
        if (isLatest()) {
          setError(normalizeAppDevError(requestError));
        }
      } finally {
        if (isLatest()) {
          setLoadingContent(false);
        }
      }
    },
    [beginContentRequest, projectId, spaceId],
  );

  const saveCurrentFile = useCallback(async () => {
    if (!spaceId || !projectId || !selectedPath) {
      setError('请先选择要保存的文件');
      return undefined;
    }
    if (loadingContent || !fileContent || fileContent.path !== selectedPath) {
      setError('文件正在加载，请等待当前文件加载完成后再保存');
      return undefined;
    }

    const isLatest = beginMutationRequest();

    setSaving(true);
    setError('');

    try {
      const saved = await saveAppDevFileContent({
        spaceId,
        projectId,
        path: selectedPath,
        content: draft,
        version: fileContent.version,
      });
      if (!isLatest()) {
        return undefined;
      }
      setFileContent(saved);
      setDraft(saved.content);
      await refreshTree();
      return isLatest() ? saved : undefined;
    } catch (requestError) {
      if (isLatest()) {
        setError(normalizeAppDevError(requestError));
      }
      return undefined;
    } finally {
      if (isLatest()) {
        setSaving(false);
      }
    }
  }, [
    beginMutationRequest,
    draft,
    fileContent,
    loadingContent,
    projectId,
    refreshTree,
    selectedPath,
    spaceId,
  ]);

  const createFile = useCallback(
    async (path: string) => {
      if (!spaceId || !projectId) {
        setError('项目地址不完整，请返回项目列表重新进入');
        return;
      }

      const normalizedPath = path.trim();
      if (!normalizedPath) {
        setError('请输入文件路径');
        return;
      }
      if (hasFileNodePath(tree, normalizedPath)) {
        setError('同名文件已存在，请换一个路径');
        return;
      }

      const isLatest = beginMutationRequest();

      setSaving(true);
      setError('');
      try {
        await saveAppDevFileContent({
          spaceId,
          projectId,
          path: normalizedPath,
          content: '',
        });
        if (!isLatest()) {
          return;
        }
        await refreshTree();
        if (!isLatest()) {
          return;
        }
        await openFile(normalizedPath);
      } catch (requestError) {
        if (isLatest()) {
          setError(normalizeAppDevError(requestError));
        }
      } finally {
        if (isLatest()) {
          setSaving(false);
        }
      }
    },
    [beginMutationRequest, openFile, projectId, refreshTree, spaceId, tree],
  );

  const createDirectory = useCallback(
    async (path: string) => {
      const normalizedPath = path.trim().replace(/\/+$/u, '');
      if (!normalizedPath) {
        setError('请输入目录路径');
        return;
      }
      if (hasFileNodePath(tree, normalizedPath)) {
        setError('同名目录已存在，请换一个路径');
        return;
      }

      await createFile(`${normalizedPath}/.gitkeep`);
    },
    [createFile, tree],
  );

  const uploadFiles = useCallback(
    async (
      files: File[],
      options?: {
        basePath?: string;
        preserveRelativePath?: boolean;
      },
    ) => {
      if (!spaceId || !projectId) {
        setError('项目地址不完整，请返回项目列表重新进入');
        return;
      }

      const uploadItems = files.filter(file => file.size > 0);
      if (!uploadItems.length) {
        setError('请选择要上传的文件');
        return;
      }
      if (uploadItems.length > maxUploadFiles) {
        setError('单次上传不能超过 100 个文件');
        return;
      }
      if (
        uploadItems.reduce((total, file) => total + file.size, 0) >
        maxUploadTotalBytes
      ) {
        setError('单次上传文件总大小不能超过 100MB');
        return;
      }

      const basePath = normalizeUploadPath(options?.basePath || '');
      const filePaths = uploadItems.map(file => {
        const relativePath =
          options?.preserveRelativePath && file.webkitRelativePath
            ? file.webkitRelativePath
            : file.name;
        return normalizeUploadPath(
          [basePath, relativePath].filter(Boolean).join('/'),
        );
      });

      if (filePaths.some(path => !path)) {
        setError('上传路径不能为空');
        return;
      }
      const duplicatedPath = filePaths.find(path =>
        hasFileNodePath(tree, path),
      );
      if (duplicatedPath) {
        setError(`「${duplicatedPath}」已存在，请重命名后再上传`);
        return;
      }
      const oversizedFile = uploadItems.find(
        file => file.size > maxUploadFileBytes,
      );
      if (oversizedFile) {
        setError(`「${oversizedFile.name}」不能超过 10MB`);
        return;
      }

      const isLatest = beginMutationRequest();

      setSaving(true);
      setError('');
      try {
        await uploadAppDevFiles({
          spaceId,
          projectId,
          files: uploadItems,
          filePaths,
        });
        if (isLatest()) {
          await refreshTree();
        }
      } catch (requestError) {
        if (isLatest()) {
          setError(normalizeAppDevError(requestError));
        }
      } finally {
        if (isLatest()) {
          setSaving(false);
        }
      }
    },
    [beginMutationRequest, projectId, refreshTree, spaceId, tree],
  );

  const deletePath = useCallback(
    async (path: string) => {
      if (!spaceId || !projectId) {
        setError('项目地址不完整，请返回项目列表重新进入');
        return;
      }

      const isLatest = beginMutationRequest();

      setSaving(true);
      setError('');
      try {
        await deleteAppDevFile({ spaceId, projectId, path });
        if (!isLatest()) {
          return;
        }
        if (selectedPath === path || selectedPath.startsWith(`${path}/`)) {
          setSelectedPath('');
          setFileContent(undefined);
          setDraft('');
        }
        await refreshTree();
      } catch (requestError) {
        if (isLatest()) {
          setError(normalizeAppDevError(requestError));
        }
      } finally {
        if (isLatest()) {
          setSaving(false);
        }
      }
    },
    [beginMutationRequest, projectId, refreshTree, selectedPath, spaceId],
  );

  const renamePath = useCallback(
    async (sourcePath: string, targetPath: string) => {
      if (!spaceId || !projectId) {
        setError('项目地址不完整，请返回项目列表重新进入');
        return;
      }

      const normalizedTarget = targetPath.trim();
      if (!normalizedTarget) {
        setError('请输入新的文件路径');
        return;
      }

      const isLatest = beginMutationRequest();

      setSaving(true);
      setError('');
      try {
        await renameAppDevFile({
          spaceId,
          projectId,
          sourcePath,
          targetPath: normalizedTarget,
        });
        if (!isLatest()) {
          return;
        }
        if (selectedPath === sourcePath) {
          setSelectedPath(normalizedTarget);
          if (fileContent) {
            setFileContent({ ...fileContent, path: normalizedTarget });
          }
        } else if (selectedPath.startsWith(`${sourcePath}/`)) {
          const nextSelectedPath = `${normalizedTarget}${selectedPath.slice(
            sourcePath.length,
          )}`;
          setSelectedPath(nextSelectedPath);
          if (fileContent) {
            setFileContent({ ...fileContent, path: nextSelectedPath });
          }
        }
        await refreshTree();
      } catch (requestError) {
        if (isLatest()) {
          setError(normalizeAppDevError(requestError));
        }
      } finally {
        if (isLatest()) {
          setSaving(false);
        }
      }
    },
    [
      beginMutationRequest,
      fileContent,
      projectId,
      refreshTree,
      selectedPath,
      spaceId,
    ],
  );

  useEffect(() => {
    invalidateTreeRequests();
    invalidateContentRequests();
    invalidateMutationRequests();
    setTree([]);
    setSelectedPath('');
    setFileContent(undefined);
    setDraft('');
    setLoadingTree(false);
    setLoadingContent(false);
    setSaving(false);
    setError('');
  }, [
    invalidateContentRequests,
    invalidateMutationRequests,
    invalidateTreeRequests,
    projectId,
    spaceId,
  ]);

  useEffect(() => {
    void refreshTree();
  }, [refreshTree]);

  return {
    tree,
    selectedPath,
    fileContent,
    draft,
    setDraft,
    dirty: Boolean(
      fileContent &&
        fileContent.path === selectedPath &&
        draft !== fileContent.content,
    ),
    loadingTree,
    loadingContent,
    saving,
    error,
    refreshTree,
    openFile,
    saveCurrentFile,
    createFile,
    createDirectory,
    uploadFiles,
    deletePath,
    renamePath,
  };
};
