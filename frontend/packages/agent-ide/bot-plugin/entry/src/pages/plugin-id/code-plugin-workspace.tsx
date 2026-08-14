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

/* eslint-disable @coze-arch/max-line-per-function -- The workspace coordinates editor, persistence, and sandbox debug state as one cohesive view model. */
/* eslint-disable max-lines-per-function -- The workspace keeps the NuWAX single-screen editing flow together. */
/* eslint-disable max-lines -- The single-source workspace intentionally keeps its three tightly coupled panels colocated. */

import {
  type ComponentProps,
  type KeyboardEvent,
  useCallback,
  useEffect,
  useId,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from 'react';

import {
  DebugCodePlugin,
  GetCodePluginDraft,
  SaveCodePluginDraft,
  plugin_develop_common as pluginDevelopCommon,
  type CodePluginDebugData,
  type CodePluginDraftData,
  type DebugCodePluginRequest,
  type SaveCodePluginDraftRequest,
} from '@coze-studio/api-schema/plugin-develop';
import {
  Button,
  Select,
  Tag,
  TextArea,
  Toast,
  Typography,
} from '@coze-arch/coze-design';
import { Editor } from '@coze-arch/bot-monaco-editor';
import { CreationMethod, PluginType } from '@coze-arch/bot-api/plugin_develop';

import {
  DEFAULT_CODE_PLUGIN_SCHEMA,
  normalizeCodePluginSchema,
} from './code-plugin-schema';

import s from './code-plugin-workspace.module.less';

interface CodePluginWorkspaceProps {
  pluginID: string;
  spaceID: string;
  canEdit: boolean;
  onPublishReadyChange?: (ready: boolean) => void;
}

type MonacoEditorMount = NonNullable<ComponentProps<typeof Editor>['onMount']>;
type WorkspacePanel = 'debug' | 'input' | 'output';
type SchemaErrors = Partial<Record<'input' | 'output', string>>;
interface WorkspaceSnapshot {
  runtime: pluginDevelopCommon.CodePluginRuntime;
  entryFile: string;
  source: string;
  inputSchemaJSON: string;
  outputSchemaJSON: string;
}
interface IdentityToken {
  identity: string;
  identityEpoch: number;
}
interface MutationToken extends IdentityToken {
  sequence: number;
}

const DEFAULT_ARGUMENTS = '{\n  "message": "hello"\n}';
const PANEL_ORDER: WorkspacePanel[] = ['debug', 'input', 'output'];
const SAFE_LOAD_ERROR = '代码草稿加载失败，请稍后重试';
const SAFE_SAVE_ERROR = '代码草稿保存失败，请稍后重试';
const SAFE_DEBUG_ERROR = '试运行失败，请稍后重试';

const DEBUG_STATUS_LABEL: Record<
  pluginDevelopCommon.CodePluginDebugStatus,
  string
> = {
  [pluginDevelopCommon.CodePluginDebugStatus.Success]: '成功',
  [pluginDevelopCommon.CodePluginDebugStatus.RuntimeError]: '运行错误',
  [pluginDevelopCommon.CodePluginDebugStatus.Timeout]: '运行超时',
  [pluginDevelopCommon.CodePluginDebugStatus.Capacity]: '容量不足',
  [pluginDevelopCommon.CodePluginDebugStatus.OutputLimit]: '输出超限',
  [pluginDevelopCommon.CodePluginDebugStatus.Canceled]: '已取消',
  [pluginDevelopCommon.CodePluginDebugStatus.Unavailable]: '服务不可用',
};

const runtimeOptions = [
  { label: 'Python', value: pluginDevelopCommon.CodePluginRuntime.Python },
  {
    label: 'JavaScript',
    value: pluginDevelopCommon.CodePluginRuntime.JavaScript,
  },
];

const entryFileForRuntime = (runtime: pluginDevelopCommon.CodePluginRuntime) =>
  runtime === pluginDevelopCommon.CodePluginRuntime.JavaScript
    ? 'index.js'
    : 'main.py';

const languageForRuntime = (runtime: pluginDevelopCommon.CodePluginRuntime) =>
  runtime === pluginDevelopCommon.CodePluginRuntime.JavaScript
    ? 'javascript'
    : 'python';

const snapshotOf = (snapshot: WorkspaceSnapshot) => JSON.stringify(snapshot);

const normalizeLoadedSchema = (value?: string) =>
  normalizeCodePluginSchema(value || DEFAULT_CODE_PLUGIN_SCHEMA);

const parseArguments = (value: string) => {
  let parsed: unknown;
  try {
    parsed = JSON.parse(value);
  } catch (error) {
    void error;
    throw new Error('试运行输入必须是有效 JSON 对象');
  }
  if (!parsed || Array.isArray(parsed) || typeof parsed !== 'object') {
    throw new Error('试运行输入必须是 JSON 对象');
  }
};

const safeDebugFailure = (
  revision: number,
  reason = SAFE_DEBUG_ERROR,
): CodePluginDebugData => ({
  success: false,
  status: pluginDevelopCommon.CodePluginDebugStatus.Unavailable,
  result: '',
  reason,
  duration_ms: 0,
  output_bytes: 0,
  revision,
});

export const isCodePlugin = (
  pluginType?: PluginType,
  creationMethod?: CreationMethod,
) => pluginType === PluginType.FUNC && creationMethod === CreationMethod.IDE;

export const CodePluginWorkspace = ({
  pluginID,
  spaceID,
  canEdit,
  onPublishReadyChange,
}: CodePluginWorkspaceProps) => {
  const [runtime, setRuntime] = useState(
    pluginDevelopCommon.CodePluginRuntime.Python,
  );
  const [entryFile, setEntryFile] = useState('main.py');
  const [source, setSource] = useState('');
  const [inputSchemaJSON, setInputSchemaJSON] = useState(
    DEFAULT_CODE_PLUGIN_SCHEMA,
  );
  const [outputSchemaJSON, setOutputSchemaJSON] = useState(
    DEFAULT_CODE_PLUGIN_SCHEMA,
  );
  const [savedSnapshot, setSavedSnapshot] = useState('');
  const [loadedIdentity, setLoadedIdentity] = useState('');
  const [revision, setRevision] = useState(0);
  const [lastDebuggedRevision, setLastDebuggedRevision] = useState(0);
  const [serverDebugReady, setServerDebugReady] = useState(false);
  const [argumentsJSON, setArgumentsJSON] = useState(DEFAULT_ARGUMENTS);
  const [debugResult, setDebugResult] = useState<CodePluginDebugData>();
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [running, setRunning] = useState(false);
  const [loadError, setLoadError] = useState('');
  const [activePanel, setActivePanel] = useState<WorkspacePanel>('debug');
  const [schemaErrors, setSchemaErrors] = useState<SchemaErrors>({});
  const saveCommandRef = useRef<() => void>(() => undefined);
  const saveInFlightRef = useRef(false);
  const debugInFlightRef = useRef(false);
  const loadRequestRef = useRef(0);
  const mutationSequenceRef = useRef(0);
  const identityEpochRef = useRef(0);
  const identityRef = useRef(`${spaceID}:${pluginID}`);
  const publishReadyCallbackRef = useRef(onPublishReadyChange);
  const tabRefs = useRef<
    Partial<Record<WorkspacePanel, HTMLButtonElement | null>>
  >({});
  const tabsID = useId();
  const identity = `${spaceID}:${pluginID}`;
  publishReadyCallbackRef.current = onPublishReadyChange;

  const dirty =
    savedSnapshot !==
      snapshotOf({
        runtime,
        entryFile,
        source,
        inputSchemaJSON,
        outputSchemaJSON,
      }) && !loading;
  const publishReady =
    loadedIdentity === identity &&
    !loading &&
    serverDebugReady &&
    revision > 0 &&
    lastDebuggedRevision === revision &&
    !dirty;

  const isCurrentIdentity = useCallback(
    (token: IdentityToken) =>
      identityRef.current === token.identity &&
      identityEpochRef.current === token.identityEpoch,
    [],
  );

  const isCurrentMutation = useCallback(
    (token: MutationToken) =>
      isCurrentIdentity(token) &&
      mutationSequenceRef.current === token.sequence,
    [isCurrentIdentity],
  );

  const applyDraft = useCallback(
    (draft: CodePluginDraftData, owner: string) => {
      const nextRuntime =
        draft.runtime ?? pluginDevelopCommon.CodePluginRuntime.Python;
      const nextEntryFile =
        draft.entry_file || entryFileForRuntime(nextRuntime);
      const nextSource = draft.files?.[0]?.content ?? '';
      const nextInputSchema = normalizeLoadedSchema(draft.input_schema_json);
      const nextOutputSchema = normalizeLoadedSchema(draft.output_schema_json);
      setRuntime(nextRuntime);
      setEntryFile(nextEntryFile);
      setSource(nextSource);
      setInputSchemaJSON(nextInputSchema);
      setOutputSchemaJSON(nextOutputSchema);
      setLoadedIdentity(owner);
      setSchemaErrors({});
      setRevision(draft.revision ?? 0);
      setLastDebuggedRevision(draft.last_debugged_revision ?? 0);
      setServerDebugReady(Boolean(draft.debug_ready));
      setSavedSnapshot(
        snapshotOf({
          runtime: nextRuntime,
          entryFile: nextEntryFile,
          source: nextSource,
          inputSchemaJSON: nextInputSchema,
          outputSchemaJSON: nextOutputSchema,
        }),
      );
    },
    [],
  );

  useLayoutEffect(() => {
    identityRef.current = identity;
    identityEpochRef.current += 1;
    loadRequestRef.current += 1;
    mutationSequenceRef.current += 1;
    saveInFlightRef.current = false;
    debugInFlightRef.current = false;
    setLoadedIdentity('');
    setLoading(true);
    setSaving(false);
    setRunning(false);
    setLoadError('');
    setDebugResult(undefined);
    setServerDebugReady(false);
    setSavedSnapshot('');
    setRevision(0);
    setLastDebuggedRevision(0);
    setSchemaErrors({});
    publishReadyCallbackRef.current?.(false);
  }, [identity]);

  const loadDraft = useCallback(
    async (silent = false, ownerMutation?: MutationToken) => {
      const requestID = ++loadRequestRef.current;
      const requestToken: IdentityToken = {
        identity,
        identityEpoch: identityEpochRef.current,
      };
      const isCurrentRequest = () =>
        requestID === loadRequestRef.current &&
        isCurrentIdentity(requestToken) &&
        (!ownerMutation || isCurrentMutation(ownerMutation));
      if (!silent) {
        setLoading(true);
        setLoadError('');
      }
      try {
        const response = await GetCodePluginDraft({
          plugin_id: pluginID,
          space_id: spaceID,
        });
        if (!isCurrentRequest()) {
          return undefined;
        }
        if (
          !response.data ||
          response.data.plugin_id !== pluginID ||
          response.data.space_id !== spaceID
        ) {
          if (!silent) {
            setLoadError(SAFE_LOAD_ERROR);
          }
          return undefined;
        }
        applyDraft(response.data, requestToken.identity);
        return response.data;
      } catch (error) {
        void error;
        if (!silent && isCurrentRequest()) {
          setLoadError(SAFE_LOAD_ERROR);
        }
        return undefined;
      } finally {
        if (!silent && isCurrentRequest()) {
          setLoading(false);
        }
      }
    },
    [
      applyDraft,
      identity,
      isCurrentIdentity,
      isCurrentMutation,
      pluginID,
      spaceID,
    ],
  );

  useEffect(() => {
    void loadDraft();
  }, [loadDraft]);

  useEffect(() => {
    onPublishReadyChange?.(publishReady);
  }, [onPublishReadyChange, publishReady]);

  const validateSchemas = useCallback(() => {
    const errors: SchemaErrors = {};
    let normalizedInput = '';
    let normalizedOutput = '';

    try {
      normalizedInput = normalizeCodePluginSchema(inputSchemaJSON);
    } catch (error) {
      errors.input =
        error instanceof Error ? error.message : '输入结构不是有效 JSON Schema';
    }
    try {
      normalizedOutput = normalizeCodePluginSchema(outputSchemaJSON);
    } catch (error) {
      errors.output =
        error instanceof Error ? error.message : '输出结构不是有效 JSON Schema';
    }

    setSchemaErrors(errors);
    if (Object.keys(errors).length > 0) {
      throw new Error('请修正输入/输出结构后再保存');
    }
    return { normalizedInput, normalizedOutput };
  }, [inputSchemaJSON, outputSchemaJSON]);

  const saveDraft = useCallback(
    async (allowDuringDebug = false) => {
      if (!canEdit || loading || loadedIdentity !== identity) {
        throw new Error(SAFE_SAVE_ERROR);
      }
      if (
        saveInFlightRef.current ||
        (debugInFlightRef.current && !allowDuringDebug)
      ) {
        throw new Error(SAFE_SAVE_ERROR);
      }
      const { normalizedInput, normalizedOutput } = validateSchemas();
      const requestToken: MutationToken = {
        identity,
        identityEpoch: identityEpochRef.current,
        sequence: ++mutationSequenceRef.current,
      };
      saveInFlightRef.current = true;
      setSaving(true);
      try {
        const request: SaveCodePluginDraftRequest = {
          plugin_id: pluginID,
          space_id: spaceID,
          revision,
          runtime,
          entry_file: entryFile,
          files: [{ path: entryFile, content: source }],
          input_schema_json: normalizedInput,
          output_schema_json: normalizedOutput,
        };
        const response = await SaveCodePluginDraft(request);
        if (!isCurrentMutation(requestToken)) {
          return undefined;
        }
        if (!response.data) {
          throw new Error(SAFE_SAVE_ERROR);
        }
        if (
          response.data.plugin_id !== pluginID ||
          response.data.space_id !== spaceID
        ) {
          throw new Error(SAFE_SAVE_ERROR);
        }
        applyDraft(response.data, requestToken.identity);
        Toast.success('代码已保存');
        return response.data;
      } catch (error) {
        if (!isCurrentMutation(requestToken)) {
          return undefined;
        }
        throw error;
      } finally {
        if (isCurrentMutation(requestToken)) {
          saveInFlightRef.current = false;
          setSaving(false);
        }
      }
    },
    [
      applyDraft,
      canEdit,
      entryFile,
      identity,
      isCurrentMutation,
      loadedIdentity,
      loading,
      pluginID,
      revision,
      runtime,
      source,
      spaceID,
      validateSchemas,
    ],
  );

  saveCommandRef.current = () => {
    void saveDraft().catch(() => {
      Toast.error(SAFE_SAVE_ERROR);
    });
  };

  const handleEditorMount = useCallback<MonacoEditorMount>((editor, monaco) => {
    editor.addAction({
      id: 'plugin-code.save',
      label: '保存代码',
      keybindings: [monaco.KeyMod.CtrlCmd | monaco.KeyCode.KeyS],
      run: () => saveCommandRef.current(),
    });
  }, []);

  const handleRuntimeChange = (
    nextRuntime: pluginDevelopCommon.CodePluginRuntime,
  ) => {
    setRuntime(nextRuntime);
    setEntryFile(entryFileForRuntime(nextRuntime));
    setServerDebugReady(false);
    setDebugResult(undefined);
  };

  const handleSchemaChange = (kind: 'input' | 'output', value: string) => {
    if (kind === 'input') {
      setInputSchemaJSON(value);
    } else {
      setOutputSchemaJSON(value);
    }
    setSchemaErrors(current => ({ ...current, [kind]: undefined }));
    setServerDebugReady(false);
    setDebugResult(undefined);
  };

  const handleDebug = async () => {
    try {
      parseArguments(argumentsJSON);
    } catch (error) {
      void error;
      Toast.error('试运行输入必须是有效 JSON 对象');
      return;
    }
    if (debugInFlightRef.current || saveInFlightRef.current) {
      Toast.error('已有保存或试运行请求正在处理');
      return;
    }

    const debugOwner: IdentityToken = {
      identity,
      identityEpoch: identityEpochRef.current,
    };
    debugInFlightRef.current = true;
    setRunning(true);
    setDebugResult(undefined);
    try {
      const shouldSave = dirty || revision === 0;
      const draft = shouldSave ? await saveDraft(true) : undefined;
      if (!isCurrentIdentity(debugOwner) || (shouldSave && !draft)) {
        return;
      }
      const debugRevision = draft?.revision ?? revision;
      const requestToken: MutationToken = {
        ...debugOwner,
        sequence: ++mutationSequenceRef.current,
      };
      const request: DebugCodePluginRequest = {
        plugin_id: pluginID,
        space_id: spaceID,
        revision: debugRevision,
        arguments_in_json: argumentsJSON,
      };
      const response = await DebugCodePlugin(request);
      if (!isCurrentMutation(requestToken)) {
        return;
      }
      if (!response.data) {
        throw new Error(SAFE_DEBUG_ERROR);
      }
      setDebugResult(response.data);
      if (
        response.data.success &&
        response.data.status ===
          pluginDevelopCommon.CodePluginDebugStatus.Success
      ) {
        if (response.data.revision !== debugRevision) {
          setDebugResult(
            safeDebugFailure(
              debugRevision,
              '试运行结果版本不一致，请重新试运行',
            ),
          );
          return;
        }
        const confirmedDraft = await loadDraft(true, requestToken);
        if (!isCurrentMutation(requestToken)) {
          return;
        }
        if (
          confirmedDraft?.revision === debugRevision &&
          confirmedDraft.debug_ready &&
          confirmedDraft.last_debugged_revision === debugRevision
        ) {
          Toast.success('试运行通过，可以发布');
        } else {
          setServerDebugReady(false);
          setDebugResult(
            safeDebugFailure(
              debugRevision,
              '服务端尚未确认发布资格，请重新试运行',
            ),
          );
        }
      }
    } catch (error) {
      void error;
      if (!isCurrentIdentity(debugOwner)) {
        return;
      }
      setServerDebugReady(false);
      setDebugResult(safeDebugFailure(revision));
    } finally {
      if (isCurrentIdentity(debugOwner)) {
        debugInFlightRef.current = false;
        setRunning(false);
      }
    }
  };

  const handleTabKeyDown = (
    event: KeyboardEvent<HTMLButtonElement>,
    currentPanel: WorkspacePanel,
  ) => {
    const currentIndex = PANEL_ORDER.indexOf(currentPanel);
    let nextIndex: number | undefined;
    if (event.key === 'ArrowRight') {
      nextIndex = (currentIndex + 1) % PANEL_ORDER.length;
    } else if (event.key === 'ArrowLeft') {
      nextIndex = (currentIndex - 1 + PANEL_ORDER.length) % PANEL_ORDER.length;
    } else if (event.key === 'Home') {
      nextIndex = 0;
    } else if (event.key === 'End') {
      nextIndex = PANEL_ORDER.length - 1;
    }
    if (nextIndex === undefined) {
      return;
    }
    event.preventDefault();
    const nextPanel = PANEL_ORDER[nextIndex];
    setActivePanel(nextPanel);
    tabRefs.current[nextPanel]?.focus();
  };

  const editorOptions = useMemo(
    () => ({
      ariaLabel: '代码插件编辑器',
      automaticLayout: true,
      bracketPairColorization: { enabled: true },
      fixedOverflowWidgets: true,
      fontFamily:
        "'SFMono-Regular', Consolas, 'Liberation Mono', Menlo, monospace",
      fontLigatures: true,
      fontSize: 13,
      formatOnPaste: true,
      formatOnType: true,
      lineHeight: 21,
      minimap: { enabled: false },
      padding: { bottom: 16, top: 16 },
      readOnly: !canEdit || saving || running,
      renderLineHighlight: 'line' as const,
      scrollBeyondLastLine: false,
      smoothScrolling: true,
      tabSize: 2,
      wordWrap: 'off' as const,
    }),
    [canEdit, running, saving],
  );

  if (loadError) {
    return (
      <div className={s['load-state']} role="alert">
        <strong>代码草稿加载失败</strong>
        <span>{loadError}</span>
        <Button onClick={() => void loadDraft()}>重新加载</Button>
      </div>
    );
  }

  return (
    <section className={s.workspace} aria-label="代码插件开发">
      <header className={s.header}>
        <div>
          <Typography.Title heading={5}>代码配置</Typography.Title>
          <Typography.Text type="secondary">
            编辑入口函数，保存后通过试运行校验，再发布为不可变版本。
          </Typography.Text>
        </div>
        <div className={s.actions}>
          <Tag color={publishReady ? 'green' : dirty ? 'orange' : 'grey'}>
            {publishReady ? '可发布' : dirty ? '有未保存修改' : '等待试运行'}
          </Tag>
          <Button
            disabled={!canEdit || loading || running || saving || !dirty}
            loading={saving}
            onClick={() => saveCommandRef.current()}
          >
            保存
          </Button>
          <Button
            theme="solid"
            disabled={!canEdit || loading || running || !source.trim()}
            loading={running}
            onClick={handleDebug}
          >
            试运行
          </Button>
        </div>
      </header>

      <div className={s.body}>
        <div className={s.editorPanel}>
          <div className={s.editorToolbar}>
            <Select
              aria-label="代码运行时"
              size="small"
              value={runtime}
              optionList={runtimeOptions}
              disabled={!canEdit || saving || running}
              onChange={value =>
                handleRuntimeChange(
                  value as pluginDevelopCommon.CodePluginRuntime,
                )
              }
            />
            <code>{entryFile}</code>
            <span>Revision {revision}</span>
          </div>
          <div className={s.editorSurface}>
            {loading ? (
              <div className={s['load-state']}>正在加载代码草稿...</div>
            ) : (
              <Editor
                height="100%"
                language={languageForRuntime(runtime)}
                onChange={value => {
                  setSource(value ?? '');
                  setServerDebugReady(false);
                  setDebugResult(undefined);
                }}
                onMount={handleEditorMount}
                options={editorOptions}
                path={entryFile}
                theme="vs"
                value={source}
              />
            )}
          </div>
          <footer className={s.editorFooter}>
            <span>UTF-8</span>
            <span>入口文件：{entryFile}</span>
            <span>⌘/Ctrl + S 保存</span>
          </footer>
        </div>

        <aside className={s.sidePanel}>
          <div className={s.panelTabs} role="tablist" aria-label="代码插件配置">
            {(
              [
                ['debug', '试运行'],
                ['input', '输入结构'],
                ['output', '输出结构'],
              ] as const
            ).map(([value, label]) => (
              <button
                key={value}
                type="button"
                role="tab"
                id={`${tabsID}-${value}-tab`}
                aria-controls={`${tabsID}-${value}-panel`}
                aria-selected={activePanel === value}
                aria-label={`${label}${
                  value !== 'debug' && schemaErrors[value] ? '，存在错误' : ''
                }`}
                tabIndex={activePanel === value ? 0 : -1}
                ref={node => {
                  tabRefs.current[value] = node;
                }}
                className={activePanel === value ? s.activeTab : undefined}
                onClick={() => setActivePanel(value)}
                onKeyDown={event => handleTabKeyDown(event, value)}
              >
                {label}
                {value !== 'debug' && schemaErrors[value] ? (
                  <>
                    <span className={s.errorDot} aria-hidden="true" />
                    <span className={s.srOnly}>{label}存在错误</span>
                  </>
                ) : null}
              </button>
            ))}
          </div>

          {activePanel === 'debug' ? (
            <div
              className={s.panelContent}
              role="tabpanel"
              id={`${tabsID}-debug-panel`}
              aria-labelledby={`${tabsID}-debug-tab`}
              tabIndex={0}
            >
              <div className={s.panelHeading}>
                <div>
                  <strong>试运行</strong>
                  <span>传入 JSON 对象验证入口函数</span>
                </div>
                {debugResult && debugResult.duration_ms > 0 ? (
                  <small>{debugResult.duration_ms} ms</small>
                ) : null}
              </div>
              <label className={s.fieldLabel} htmlFor="plugin-debug-arguments">
                输入参数
              </label>
              <TextArea
                id="plugin-debug-arguments"
                className={s.arguments}
                value={argumentsJSON}
                disabled={!canEdit || running}
                autosize={{ minRows: 8, maxRows: 14 }}
                onChange={setArgumentsJSON}
              />
              <div className={s.result}>
                <div className={s.resultHeading}>
                  <strong>运行结果</strong>
                  {debugResult ? (
                    <Tag color={debugResult.success ? 'green' : 'red'}>
                      {DEBUG_STATUS_LABEL[debugResult.status]}
                    </Tag>
                  ) : null}
                </div>
                {debugResult ? (
                  <>
                    {debugResult.reason ? <p>{debugResult.reason}</p> : null}
                    {debugResult.result ? (
                      <pre>{debugResult.result}</pre>
                    ) : null}
                    {debugResult.output_bytes > 0 ? (
                      <small>输出大小：{debugResult.output_bytes} bytes</small>
                    ) : null}
                  </>
                ) : (
                  <div className={s.resultEmpty}>
                    保存并点击“试运行”，这里会展示返回值与受限日志。
                  </div>
                )}
              </div>
            </div>
          ) : (
            <div
              className={s.panelContent}
              role="tabpanel"
              id={`${tabsID}-${activePanel}-panel`}
              aria-labelledby={`${tabsID}-${activePanel}-tab`}
              tabIndex={0}
            >
              <div className={s.schemaHeading}>
                <strong>
                  {activePanel === 'input' ? '输入结构' : '输出结构'}
                </strong>
                <span>
                  使用 JSON Schema object 描述
                  {activePanel === 'input' ? '调用参数' : '返回结果'}。
                </span>
              </div>
              <label
                className={s.fieldLabel}
                htmlFor={`plugin-${activePanel}-schema`}
              >
                {activePanel === 'input' ? '输入结构 JSON' : '输出结构 JSON'}
              </label>
              <TextArea
                id={`plugin-${activePanel}-schema`}
                className={s.schemaEditor}
                value={
                  activePanel === 'input' ? inputSchemaJSON : outputSchemaJSON
                }
                disabled={!canEdit || saving || running}
                autosize={{ minRows: 18, maxRows: 28 }}
                onChange={value => handleSchemaChange(activePanel, value)}
              />
              {schemaErrors[activePanel] ? (
                <p className={s.schemaError} role="alert">
                  {schemaErrors[activePanel]}
                </p>
              ) : (
                <p className={s.schemaHint}>
                  顶层必须为 <code>type: object</code>，单份结构最大 64KB。
                  此处为前端预检，保存和执行仍以后端权威校验为准。
                </p>
              )}
            </div>
          )}
        </aside>
      </div>
    </section>
  );
};
