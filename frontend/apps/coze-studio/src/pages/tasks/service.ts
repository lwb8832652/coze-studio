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

import { workbench, workbenchTask } from '@coze-studio/api-schema';

export const listTasks = workbenchTask.ListTasks;
export const getTask = workbenchTask.GetTask;
export const listTaskThreads = workbenchTask.ListTaskThreads;
export const getTaskThread = workbenchTask.GetTaskThread;
export const listTaskThreadMessages = workbenchTask.ListTaskThreadMessages;
export const appendTaskThreadMessage = workbenchTask.AppendTaskThreadMessage;
export const cancelTask = workbenchTask.CancelTask;
export const retryTask = workbenchTask.RetryTask;
export const listTaskEvents = workbenchTask.ListTaskEvents;
export const sendWorkbenchChat = workbench.WorkbenchChat;
