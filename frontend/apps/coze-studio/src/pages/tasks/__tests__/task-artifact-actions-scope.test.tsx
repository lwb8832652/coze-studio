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

/* eslint-disable @typescript-eslint/no-invalid-void-type -- Deferred test controls model promise completion with explicit void values. */

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act } from 'react-dom/test-utils';
import { createRoot, type Root } from 'react-dom/client';

import {
  useTaskArtifactActions,
  type TaskArtifactActions,
} from '../task-artifact-actions';
import {
  deleteTaskThreadArtifact,
  getTaskThreadArtifactSignedURL,
  restoreTaskThreadArtifact,
  reviewTaskThreadArtifactScan,
} from '../service';

vi.mock('../service', () => ({
  deleteTaskThreadArtifact: vi.fn(),
  fetchTaskThreadArtifactContent: vi.fn(),
  getTaskThreadArtifactSignedURL: vi.fn(),
  installSkillFromArtifact: vi.fn(),
  isTaskThreadArtifactSafeError: vi.fn(() => false),
  restoreTaskThreadArtifact: vi.fn(),
  reviewTaskThreadArtifactScan: vi.fn(),
}));

const deferred = <T,>() => {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>(nextResolve => {
    resolve = nextResolve;
  });
  return { promise, resolve };
};

const artifact = {
  artifact_id: 'artifact-a',
  file_name: 'artifact-a.png',
  name: 'artifact-a.png',
  title: 'artifact-a.png',
  content_type: 'image/png',
  created_at: 1,
  updated_at: 1,
  run_id: 'run-a',
  thread_id: 'thread-a',
} as never;

let currentActions: TaskArtifactActions;

const ArtifactActionsHarness = ({
  onArtifactsChanged,
  threadId,
}: {
  onArtifactsChanged: () => void | Promise<void>;
  threadId: string;
}) => {
  currentActions = useTaskArtifactActions({
    onArtifactsChanged,
    spaceId: 'space-1',
    threadId,
  });

  return null;
};

describe('task artifact action task scope', () => {
  let container: HTMLDivElement;
  let root: Root;
  let onArtifactsChanged: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    globalThis.IS_REACT_ACT_ENVIRONMENT = true;
    vi.clearAllMocks();
    container = document.createElement('div');
    document.body.appendChild(container);
    root = createRoot(container);
    onArtifactsChanged = vi.fn().mockResolvedValue(undefined);
    act(() =>
      root.render(
        <ArtifactActionsHarness
          onArtifactsChanged={onArtifactsChanged}
          threadId="thread-a"
        />,
      ),
    );
  });

  afterEach(() => {
    act(() => root.unmount());
    container.remove();
  });

  it('clears preview state and ignores a late preview after switching tasks', async () => {
    const request = deferred<{
      data: { url: string; content_type: string };
    }>();
    vi.mocked(getTaskThreadArtifactSignedURL).mockReturnValue(request.promise);

    act(() => {
      void currentActions.handleArtifactAction(artifact, 'preview');
    });
    expect(currentActions.activeAction).toContain('preview');

    act(() =>
      root.render(
        <ArtifactActionsHarness
          onArtifactsChanged={onArtifactsChanged}
          threadId="thread-b"
        />,
      ),
    );
    expect(currentActions.activeAction).toBe('');
    expect(currentActions.inlinePreview).toBeNull();
    expect(currentActions.error).toBe('');

    await act(async () => {
      request.resolve({
        data: {
          url: 'https://example.test/old.png',
          content_type: 'image/png',
        },
      });
      await request.promise;
    });
    expect(currentActions.inlinePreview).toBeNull();
    expect(currentActions.activeAction).toBe('');
  });

  it('does not publish a late delete result into the next task', async () => {
    const request = deferred<void>();
    vi.mocked(deleteTaskThreadArtifact).mockReturnValue(request.promise);

    act(() => {
      void currentActions.handleDeleteArtifact(artifact);
    });
    act(() =>
      root.render(
        <ArtifactActionsHarness
          onArtifactsChanged={onArtifactsChanged}
          threadId="thread-b"
        />,
      ),
    );

    await act(async () => {
      request.resolve();
      await request.promise;
    });
    expect(currentActions.removedArtifact).toBeNull();
    expect(onArtifactsChanged).not.toHaveBeenCalled();
    expect(currentActions.activeAction).toBe('');
  });

  it('does not publish a late restore result into the next task', async () => {
    vi.mocked(deleteTaskThreadArtifact).mockResolvedValue(undefined);
    const restoreRequest = deferred<void>();
    vi.mocked(restoreTaskThreadArtifact).mockReturnValue(
      restoreRequest.promise,
    );

    await act(async () => {
      await currentActions.handleDeleteArtifact(artifact);
    });
    expect(currentActions.removedArtifact?.artifactId).toBe('artifact-a');

    act(() => {
      void currentActions.handleRestoreArtifact();
    });
    act(() =>
      root.render(
        <ArtifactActionsHarness
          onArtifactsChanged={onArtifactsChanged}
          threadId="thread-b"
        />,
      ),
    );

    await act(async () => {
      restoreRequest.resolve();
      await restoreRequest.promise;
    });
    expect(currentActions.removedArtifact).toBeNull();
    expect(onArtifactsChanged).toHaveBeenCalledTimes(1);
    expect(currentActions.activeAction).toBe('');
  });

  it('does not publish a late review result into the next task', async () => {
    const request = deferred<void>();
    vi.mocked(reviewTaskThreadArtifactScan).mockReturnValue(request.promise);

    act(() => {
      void currentActions.handleReviewArtifact(artifact, 'approve');
    });
    act(() =>
      root.render(
        <ArtifactActionsHarness
          onArtifactsChanged={onArtifactsChanged}
          threadId="thread-b"
        />,
      ),
    );

    await act(async () => {
      request.resolve();
      await request.promise;
    });
    expect(onArtifactsChanged).not.toHaveBeenCalled();
    expect(currentActions.activeAction).toBe('');
    expect(currentActions.error).toBe('');
  });
});
