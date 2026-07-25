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

import { afterAll, afterEach, describe, expect, it } from 'vitest';
import { act } from 'react-dom/test-utils';
import { createRoot, type Root } from 'react-dom/client';
import { workbenchTask } from '@coze-studio/api-schema';

import { useTaskThreadTitleSync } from '../task-title-sync';
import {
  WORKSPACE_TASK_THREAD_UPSERT_EVENT,
  type WorkspaceTaskThreadUpsertDetail,
} from '../task-thread-events';

type ChatTask = workbenchTask.ChatTask;
type TitleSyncControls = ReturnType<typeof useTaskThreadTitleSync>;

const originalActEnvironment = globalThis.IS_REACT_ACT_ENVIRONMENT;
globalThis.IS_REACT_ACT_ENVIRONMENT = true;

describe('task title synchronization', () => {
  let container: HTMLDivElement | undefined;
  let root: Root | undefined;

  afterEach(() => {
    act(() => {
      root?.unmount();
    });
    container?.remove();
    root = undefined;
    container = undefined;
  });

  it('patches the workspace task list when canonical polling changes a title', () => {
    const emittedDetails: WorkspaceTaskThreadUpsertDetail[] = [];
    let controls: TitleSyncControls | undefined;
    const handleUpsert = (event: Event) => {
      emittedDetails.push(
        (event as CustomEvent<WorkspaceTaskThreadUpsertDetail>).detail,
      );
    };
    const Harness = () => {
      const [, setTask] = useState<ChatTask>();
      controls = useTaskThreadTitleSync({
        setTask,
        spaceID: 'space-1',
      });
      return null;
    };

    window.addEventListener(WORKSPACE_TASK_THREAD_UPSERT_EVENT, handleUpsert);
    container = document.createElement('div');
    document.body.appendChild(container);
    root = createRoot(container);

    act(() => {
      root?.render(<Harness />);
    });
    act(() => {
      controls?.setCurrentTask({
        id: 'thread-1',
        space_id: 'space-1',
        title: '请为一款面向中小企业的智能协作平台设计完整发布计划',
        updated_at: 1717000000000,
      } as ChatTask);
    });
    expect(emittedDetails).toHaveLength(0);

    act(() => {
      controls?.setCurrentTask({
        id: 'thread-1',
        space_id: 'space-1',
        title: '中小企业智能协作平台发布计划',
        updated_at: 1717000100000,
      } as ChatTask);
    });

    expect(emittedDetails).toEqual([
      {
        mode: 'patch',
        space_id: 'space-1',
        thread: {
          thread_id: 'thread-1',
          title: '中小企业智能协作平台发布计划',
          updated_at: 1717000100000,
        },
      },
    ]);

    window.removeEventListener(
      WORKSPACE_TASK_THREAD_UPSERT_EVENT,
      handleUpsert,
    );
  });

  it('patches the workspace task list when canonical polling changes status', () => {
    const emittedDetails: WorkspaceTaskThreadUpsertDetail[] = [];
    let controls: TitleSyncControls | undefined;
    const handleUpsert = (event: Event) => {
      emittedDetails.push(
        (event as CustomEvent<WorkspaceTaskThreadUpsertDetail>).detail,
      );
    };
    const Harness = () => {
      const [, setTask] = useState<ChatTask>();
      controls = useTaskThreadTitleSync({
        setTask,
        spaceID: 'space-1',
      });
      return null;
    };

    window.addEventListener(WORKSPACE_TASK_THREAD_UPSERT_EVENT, handleUpsert);
    container = document.createElement('div');
    document.body.appendChild(container);
    root = createRoot(container);

    act(() => {
      root?.render(<Harness />);
    });
    act(() => {
      controls?.setCurrentTask({
        id: 'thread-1',
        space_id: 'space-1',
        title: '通知终态验收',
        status: workbenchTask.TaskStatus.Created,
        updated_at: 1717000000000,
      } as ChatTask);
    });
    act(() => {
      controls?.setCurrentTask({
        id: 'thread-1',
        space_id: 'space-1',
        title: '通知终态验收',
        status: workbenchTask.TaskStatus.Succeeded,
        updated_at: 1717000100000,
      } as ChatTask);
    });

    expect(emittedDetails).toEqual([
      {
        mode: 'patch',
        space_id: 'space-1',
        thread: {
          thread_id: 'thread-1',
          status: workbenchTask.TaskStatus.Succeeded,
          updated_at: 1717000100000,
        },
      },
    ]);

    window.removeEventListener(
      WORKSPACE_TASK_THREAD_UPSERT_EVENT,
      handleUpsert,
    );
  });
});

afterAll(() => {
  globalThis.IS_REACT_ACT_ENVIRONMENT = originalActEnvironment;
});
