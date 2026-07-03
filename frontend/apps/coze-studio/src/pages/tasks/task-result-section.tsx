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

import type { workbenchTask } from '@coze-studio/api-schema';

import { TaskMessageTokenUsage } from './task-message-token-usage';
import { TaskMarkdownContent } from './task-markdown-content';
import { TaskInlineReasoning } from './task-inline-reasoning';
import type {
  TaskDetailTokenUsage,
  TaskTokenUsageViewMode,
} from './task-detail-loader';
import { TaskAssistantMessageActions } from './task-assistant-message-actions';
import {
  getLatestAnswerEventMessage,
  getLatestAnswerEventReasoning,
  getTaskInputText,
  isTaskTerminalStatus,
  parseTaskResultPayload,
  type TaskResultPayload,
} from './helpers';

type ChatTask = workbenchTask.ChatTask;
type TaskEvent = workbenchTask.TaskEvent;

export const TaskStreamingIndicator = () => (
  <div
    className="coze-prototype-answer-loading"
    data-testid="task-answer-loading"
    aria-label="任务执行中"
  >
    <span />
    <span />
    <span />
  </div>
);

const TaskAnswer = ({
  task,
  result,
  reasoning,
  streamingMessage,
  tokenUsage,
  tokenUsageViewMode,
}: {
  task: ChatTask;
  result: TaskResultPayload;
  reasoning?: string;
  streamingMessage?: string;
  tokenUsage?: TaskDetailTokenUsage;
  tokenUsageViewMode: TaskTokenUsageViewMode;
}) => {
  const isRunning = !isTaskTerminalStatus(task.status);
  const answerMessage = result.message || streamingMessage || task.error;

  return (
    <article className="coze-prototype-answer" data-result-type="answer">
      <TaskInlineReasoning content={reasoning} />
      {answerMessage ? <TaskMarkdownContent value={answerMessage} /> : null}
      {isRunning ? <TaskStreamingIndicator /> : null}
      <TaskMessageTokenUsage
        tokenUsage={tokenUsage}
        viewMode={tokenUsageViewMode}
      />
      {!isRunning && answerMessage ? (
        <TaskAssistantMessageActions copyText={answerMessage} />
      ) : null}
      {result.retrievalSources.length ? (
        <div className="coze-prototype-result-sources">
          {result.retrievalSources.map(source => (
            <span key={source}>{source}</span>
          ))}
        </div>
      ) : null}
    </article>
  );
};

const TaskAgentResult = ({
  task,
  result,
  tokenUsage,
  tokenUsageViewMode,
}: {
  task: ChatTask;
  result: TaskResultPayload;
  tokenUsage?: TaskDetailTokenUsage;
  tokenUsageViewMode: TaskTokenUsageViewMode;
}) => (
  <article
    className="coze-prototype-agent-result"
    data-result-type="agent_trace"
  >
    <h2>Agent 最终结果</h2>
    <TaskMarkdownContent value={result.message || task.error || '结果生成中'} />
    <TaskMessageTokenUsage
      tokenUsage={tokenUsage}
      viewMode={tokenUsageViewMode}
    />
  </article>
);

const TaskReport = ({
  task,
  result,
}: {
  task: ChatTask;
  result: TaskResultPayload;
}) => (
  <article className="coze-prototype-report">
    <h2>{task.title}报告</h2>
    <TaskMarkdownContent value={result.message || task.error || '结果生成中'} />

    <h3>一、任务输入</h3>
    <p>{getTaskInputText(task.input) || task.title}</p>
  </article>
);

export const TaskResultSection = ({
  task,
  events,
  tokenUsage,
  tokenUsageViewMode,
}: {
  task: ChatTask;
  events: TaskEvent[];
  tokenUsage?: TaskDetailTokenUsage;
  tokenUsageViewMode: TaskTokenUsageViewMode;
}) => {
  const result = parseTaskResultPayload(task.result);

  if (result.resultType === 'report') {
    return <TaskReport task={task} result={result} />;
  }

  if (result.resultType === 'agent_trace') {
    return (
      <TaskAgentResult
        task={task}
        result={result}
        tokenUsage={tokenUsage}
        tokenUsageViewMode={tokenUsageViewMode}
      />
    );
  }

  return (
    <TaskAnswer
      task={task}
      result={result}
      reasoning={getLatestAnswerEventReasoning(events) || result.reasoning}
      streamingMessage={getLatestAnswerEventMessage(events)}
      tokenUsage={tokenUsage}
      tokenUsageViewMode={tokenUsageViewMode}
    />
  );
};
