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
import { useParams } from 'react-router-dom';

import { IconCozSendFill } from '@coze-arch/coze-design/icons';
import { Button, TextArea } from '@coze-arch/coze-design';
import { workbench, workbenchTask } from '@coze-studio/api-schema';

import './index.less';

import { sendWorkbenchChat } from './service';

const TEMPLATE_TAGS = [
  '工作总结',
  '数据分析',
  '代码分析',
  '异常排查',
  '飞书文档撰写',
];

const MODES = ['Auto', 'Ask', 'Agent'] as const;

type WorkbenchMode = (typeof MODES)[number];

type WorkbenchChatData = workbench.WorkbenchChatData;
type ChatTask = workbenchTask.ChatTask;

export const mapModeToChatMode = (mode: WorkbenchMode): workbench.ChatMode => {
  const modeMap: Record<WorkbenchMode, workbench.ChatMode> = {
    Auto: workbench.ChatMode.Auto,
    Ask: workbench.ChatMode.Ask,
    Agent: workbench.ChatMode.Agent,
  };

  return modeMap[mode];
};

export const getTaskStatusText = (status: workbenchTask.TaskStatus) => {
  const statusMap: Record<workbenchTask.TaskStatus, string> = {
    [workbenchTask.TaskStatus.Created]: '已创建',
    [workbenchTask.TaskStatus.Queued]: '排队中',
    [workbenchTask.TaskStatus.Running]: '运行中',
    [workbenchTask.TaskStatus.Succeeded]: '已完成',
    [workbenchTask.TaskStatus.Failed]: '失败',
    [workbenchTask.TaskStatus.Canceling]: '取消中',
    [workbenchTask.TaskStatus.Canceled]: '已取消',
  };

  return statusMap[status] ?? '未知';
};

const TaskCard = ({ task }: { task: ChatTask }) => (
  <article className="chat-workbench-task-card" aria-label="任务结果">
    <div className="chat-workbench-task-main">
      <h2>{task.title}</h2>
      <span>{getTaskStatusText(task.status)}</span>
    </div>
    <div className="chat-workbench-progress-row">
      <progress value={task.progress} max={100} />
      <span>{task.progress}%</span>
    </div>
  </article>
);

const WorkbenchPage = () => {
  const { space_id } = useParams();
  const [value, setValue] = useState('');
  const [mode, setMode] = useState<WorkbenchMode>('Auto');
  const [chatData, setChatData] = useState<WorkbenchChatData | undefined>();
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const canSend = Boolean(value.trim()) && Boolean(space_id) && !loading;

  const handleSend = async () => {
    const message = value.trim();

    if (!message || !space_id || loading) {
      return;
    }

    setLoading(true);
    setError('');

    try {
      const response = await sendWorkbenchChat({
        space_id,
        message,
        mode: mapModeToChatMode(mode),
      });

      setChatData(response.data);
      setValue('');
    } catch (err) {
      setError(err instanceof Error ? err.message : '发送失败，请稍后重试');
    } finally {
      setLoading(false);
    }
  };

  return (
    <main className="chat-workbench-page">
      <section className="chat-workbench-shell" aria-label="Chat 工作台">
        <header className="chat-workbench-header">
          <h1>欢迎来到 刘文波 的工作空间</h1>
          <p>让我们一起高效完成工作吧</p>
        </header>

        <section className="chat-workbench-composer" aria-label="任务输入">
          <TextArea
            aria-label="任务描述"
            autosize={false}
            rows={5}
            value={value}
            onChange={setValue}
            placeholder="Hi，我会根据你的任务特性，自动匹配最佳处理方式。"
            className="chat-workbench-input"
          />

          <div className="chat-workbench-toolbar">
            <div className="chat-workbench-mode" aria-label="模式选择">
              {MODES.map(item => (
                <button
                  key={item}
                  type="button"
                  className="chat-workbench-mode-button"
                  data-active={mode === item}
                  aria-pressed={mode === item}
                  onClick={() => setMode(item)}
                >
                  {item}
                </button>
              ))}
            </div>

            <Button
              color="primary"
              disabled={!canSend}
              icon={<IconCozSendFill />}
              loading={loading}
              onClick={handleSend}
              className="chat-workbench-send"
            >
              {loading ? '发送中' : '发送'}
            </Button>
          </div>
        </section>

        {error ? (
          <div className="chat-workbench-error" role="alert">
            {error}
          </div>
        ) : null}

        {chatData?.answer || chatData?.task ? (
          <section className="chat-workbench-result" aria-label="执行结果">
            {chatData.answer ? (
              <article className="chat-workbench-answer">
                {chatData.answer}
              </article>
            ) : null}
            {chatData.task ? <TaskCard task={chatData.task} /> : null}
          </section>
        ) : null}

        <div className="chat-workbench-templates" aria-label="任务模板">
          {TEMPLATE_TAGS.map(tag => (
            <button
              key={tag}
              type="button"
              className="chat-workbench-tag"
              onClick={() => setValue(tag)}
            >
              {tag}
            </button>
          ))}
        </div>
      </section>
    </main>
  );
};

export default WorkbenchPage;
