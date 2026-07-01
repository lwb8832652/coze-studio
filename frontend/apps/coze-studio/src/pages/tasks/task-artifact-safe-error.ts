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

type TaskArtifactSignedURLMode = 'preview' | 'download';

const artifactScanBlockedMessage = (
  reason: string | undefined,
  mode: TaskArtifactSignedURLMode,
) => {
  const actionLabel = mode === 'download' ? '下载' : '预览';

  const messages: Record<string, string> = {
    scan_blocked: `产物安全扫描未通过，暂不能${actionLabel}`,
    scan_failed: '产物安全扫描暂不可用，请稍后重试',
    scan_infected: `产物存在安全风险，已阻断${actionLabel}`,
    scan_pending: `产物安全扫描中，暂不能${actionLabel}`,
    scan_quarantined: `产物已隔离，暂不能${actionLabel}`,
    scan_unknown: `产物安全扫描状态未知，暂不能${actionLabel}`,
  };

  return messages[reason?.trim() ?? ''] ?? '';
};

export class TaskThreadArtifactSafeError extends Error {
  public readonly safeForDisplay = true;
}

export const isTaskThreadArtifactSafeError = (
  err: unknown,
): err is TaskThreadArtifactSafeError =>
  err instanceof TaskThreadArtifactSafeError ||
  Boolean(
    err &&
      typeof err === 'object' &&
      'safeForDisplay' in err &&
      (err as { safeForDisplay?: unknown }).safeForDisplay === true,
  );

export const taskThreadArtifactPayloadError = (
  reason: string | undefined,
  fallback: string,
  mode: TaskArtifactSignedURLMode,
) => {
  const safeMessage = artifactScanBlockedMessage(reason, mode);

  return safeMessage
    ? new TaskThreadArtifactSafeError(safeMessage)
    : new Error(fallback);
};

export const taskThreadArtifactServiceError = async (
  response: Response,
  fallback: string,
  mode: TaskArtifactSignedURLMode,
) => {
  const contentType = response.headers.get('content-type') ?? '';
  if (!contentType.toLowerCase().includes('application/json')) {
    return new Error(fallback);
  }

  const payload = await response.json().then(
    value => value as { reason?: string },
    (err: unknown) => {
      void err;
      return undefined;
    },
  );

  return taskThreadArtifactPayloadError(payload?.reason, fallback, mode);
};
