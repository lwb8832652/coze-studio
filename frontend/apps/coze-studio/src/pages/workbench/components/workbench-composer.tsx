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

/* eslint-disable @coze-arch/max-line-per-function -- Composer suggestion state and submission orchestration remain synchronized in one module. */

/* eslint-disable max-lines -- P0 composer container, split in phase 2. */

import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type CSSProperties,
  type ChangeEvent,
  type Dispatch,
  type KeyboardEvent,
  type ReactNode,
  type SetStateAction,
} from 'react';

import type { workbenchSkill } from '@coze-studio/api-schema';

import { listSkills } from '../../skill/service';
import { ChatComposer } from '../../../components/chat-composer';
import { findSelectedWorkbenchModel } from './workbench-model-selector';
import {
  WorkbenchComposerBody,
  WorkbenchComposerToolbar,
  type WorkbenchComposerPresentation,
} from './workbench-composer-controls';
import {
  addDatabaseSelection,
  addKnowledgeSelection,
  addSkillSelection,
  AtMenu,
  type WorkbenchAtDraft,
  type WorkbenchAtReference,
  type WorkbenchAtResourceType,
  type WorkbenchAtSegment,
} from './workbench-composer-at-menu';
import {
  createWorkbenchSubmitPayload,
  createDefaultWorkbenchResourceSelection,
  createDefaultWorkbenchRuntimeSettings,
  getWorkbenchFailoverCandidateModelIds,
  workbenchModelTypeToNumber,
  type WorkbenchLLMModel,
  type WorkbenchComposerSubmitPayload,
  type WorkbenchComposerVariant,
  type WorkbenchResourceSelection,
  type WorkbenchRuntimeSettings,
} from './types';

type Skill = workbenchSkill.Skill;
type WorkbenchComposerActiveOverlay = 'at' | 'extensions' | 'model' | null;

const MAX_SLASH_SKILL_SUGGESTIONS = 6;
const EXTENSION_USAGE_STORAGE_VERSION = 1;
const EXTENSION_USAGE_STORAGE_PREFIX = 'coze-workbench-extension-usage';
const WORKBENCH_AT_MENU_WIDTH = 300;
const WORKBENCH_AT_MENU_GAP = 4;

interface StoredWorkbenchExtensionUsage {
  version: typeof EXTENSION_USAGE_STORAGE_VERSION;
  resourceSelection: WorkbenchResourceSelection;
  skillsEnabled: boolean;
  mcpToolsEnabled: boolean;
}

export interface WorkbenchComposerCapabilities {
  attachments: boolean;
  attachmentDisabledReason?: string;
}

const getExtensionUsageStorageKey = (spaceId?: string) =>
  `${EXTENSION_USAGE_STORAGE_PREFIX}:${spaceId || 'default'}`;

const normalizeStoredStringArray = (value: unknown): string[] =>
  Array.isArray(value)
    ? value.filter((item): item is string => typeof item === 'string')
    : [];

const normalizeStoredResourceSelection = (
  value: unknown,
): WorkbenchResourceSelection => {
  if (!value || typeof value !== 'object') {
    return createDefaultWorkbenchResourceSelection();
  }

  const payload = value as Partial<WorkbenchResourceSelection>;

  return {
    enable_skills: normalizeStoredStringArray(payload.enable_skills),
    explicit_enable_skills: [],
    enable_mcp: normalizeStoredStringArray(payload.enable_mcp),
    enable_kbs: normalizeStoredStringArray(payload.enable_kbs),
    enable_databases: normalizeStoredStringArray(payload.enable_databases),
  };
};

const readStoredExtensionUsage = (
  spaceId?: string,
): StoredWorkbenchExtensionUsage | null => {
  if (typeof window === 'undefined') {
    return null;
  }

  const rawValue = window.localStorage.getItem(
    getExtensionUsageStorageKey(spaceId),
  );
  if (!rawValue) {
    return null;
  }

  try {
    const payload = JSON.parse(
      rawValue,
    ) as Partial<StoredWorkbenchExtensionUsage>;
    if (payload.version !== EXTENSION_USAGE_STORAGE_VERSION) {
      return null;
    }

    return {
      version: EXTENSION_USAGE_STORAGE_VERSION,
      resourceSelection: normalizeStoredResourceSelection(
        payload.resourceSelection,
      ),
      skillsEnabled: payload.skillsEnabled !== false,
      mcpToolsEnabled: payload.mcpToolsEnabled !== false,
    };
  } catch {
    return null;
  }
};

const createRuntimeSettingsFromStoredUsage = (
  usage: StoredWorkbenchExtensionUsage | null,
) => {
  const resourceSelection =
    usage?.resourceSelection ?? createDefaultWorkbenchResourceSelection();
  const runtimeSettings =
    createDefaultWorkbenchRuntimeSettings(resourceSelection);

  if (usage) {
    runtimeSettings.skills.enabled = usage.skillsEnabled;
    runtimeSettings.mcp_tools.enabled = usage.mcpToolsEnabled;
  }

  return runtimeSettings;
};

const writeStoredExtensionUsage = ({
  resourceSelection,
  runtimeSettings,
  spaceId,
}: {
  resourceSelection: WorkbenchResourceSelection;
  runtimeSettings: WorkbenchRuntimeSettings;
  spaceId?: string;
}) => {
  if (typeof window === 'undefined') {
    return;
  }

  const payload: StoredWorkbenchExtensionUsage = {
    version: EXTENSION_USAGE_STORAGE_VERSION,
    resourceSelection: {
      ...resourceSelection,
      explicit_enable_skills: [],
    },
    skillsEnabled: runtimeSettings.skills.enabled,
    mcpToolsEnabled: runtimeSettings.mcp_tools.enabled,
  };

  window.localStorage.setItem(
    getExtensionUsageStorageKey(spaceId),
    JSON.stringify(payload),
  );
};

const getLeadingSlashSkillQuery = (value: string): string | null => {
  if (!value.startsWith('/')) {
    return null;
  }

  const query = value.slice(1);
  if (query.includes('/') || /\s/.test(query)) {
    return null;
  }

  return query;
};

const getMatchingSkillSuggestions = (
  skills: Skill[],
  query: string,
): Skill[] => {
  const normalizedQuery = query.toLowerCase();

  return skills
    .map((skill, index) => ({
      index,
      skill,
      name: skill.name.toLowerCase(),
    }))
    .filter(({ name, skill }) => {
      if (!skill.enabled) {
        return false;
      }

      return !normalizedQuery || name.includes(normalizedQuery);
    })
    .sort((left, right) => {
      const leftStartsWith = left.name.startsWith(normalizedQuery);
      const rightStartsWith = right.name.startsWith(normalizedQuery);
      if (leftStartsWith !== rightStartsWith) {
        return leftStartsWith ? -1 : 1;
      }

      return left.index - right.index;
    })
    .slice(0, MAX_SLASH_SKILL_SUGGESTIONS)
    .map(({ skill }) => skill);
};

export interface WorkbenchComposerProps {
  value: string;
  loading: boolean;
  error?: string;
  variant?: WorkbenchComposerVariant;
  presentation?: WorkbenchComposerPresentation;
  spaceId?: string;
  taskId?: string;
  resetKey?: string | number;
  capabilities?: Partial<WorkbenchComposerCapabilities>;
  footerEnd?: ReactNode;
  stopLoading?: boolean;
  stopMode?: boolean;
  modelLoader?: (spaceId: string) => Promise<WorkbenchLLMModel[]>;
  onValueChange: (value: string) => void;
  onStop?: () => void | Promise<void>;
  onSubmit: (payload: WorkbenchComposerSubmitPayload) => void | Promise<void>;
}

const useWorkbenchModelSelection = ({
  spaceId,
  modelLoader,
}: {
  spaceId?: string;
  modelLoader?: (spaceId: string) => Promise<WorkbenchLLMModel[]>;
}) => {
  const requestScope = spaceId && modelLoader ? spaceId : '';
  const requestScopeRef = useRef(requestScope);
  const requestGenerationRef = useRef(0);
  const [modelState, setModelState] = useState<{
    loading: boolean;
    models: WorkbenchLLMModel[];
    scope: string;
    selectedModelType?: number;
  }>({
    loading: false,
    models: [],
    scope: '',
  });

  if (requestScopeRef.current !== requestScope) {
    requestScopeRef.current = requestScope;
    requestGenerationRef.current += 1;
  }

  const isCurrentScope = modelState.scope === requestScope;
  const models = isCurrentScope ? modelState.models : [];
  const modelsLoading =
    Boolean(requestScope) && (!isCurrentScope || modelState.loading);
  const selectedModelType = isCurrentScope
    ? modelState.selectedModelType
    : undefined;
  const selectedModel = findSelectedWorkbenchModel(models, selectedModelType);
  const setSelectedModelType = useCallback(
    (nextModelType: number) => {
      setModelState(current =>
        current.scope === requestScope
          ? { ...current, selectedModelType: nextModelType }
          : current,
      );
    },
    [requestScope],
  );

  const reloadModels = useCallback(async () => {
    if (!spaceId || !modelLoader) {
      setModelState({
        loading: false,
        models: [],
        scope: '',
      });
      return;
    }

    const requestGeneration = ++requestGenerationRef.current;
    const submittedScope = spaceId;
    setModelState(current => ({
      loading: true,
      models: current.scope === submittedScope ? current.models : [],
      scope: submittedScope,
      selectedModelType:
        current.scope === submittedScope
          ? current.selectedModelType
          : undefined,
    }));

    try {
      const nextModels = await modelLoader(spaceId);
      if (
        requestGenerationRef.current !== requestGeneration ||
        requestScopeRef.current !== submittedScope
      ) {
        return;
      }

      setModelState(current => {
        const prevModelType = current.selectedModelType;
        const selectedType =
          prevModelType &&
          nextModels.some(
            model => workbenchModelTypeToNumber(model) === prevModelType,
          )
            ? prevModelType
            : nextModels[0]
              ? workbenchModelTypeToNumber(nextModels[0])
              : undefined;

        return {
          loading: false,
          models: nextModels,
          scope: submittedScope,
          selectedModelType: selectedType,
        };
      });
    } catch {
      if (
        requestGenerationRef.current === requestGeneration &&
        requestScopeRef.current === submittedScope
      ) {
        setModelState({
          loading: false,
          models: [],
          scope: submittedScope,
        });
      }
    }
  }, [modelLoader, spaceId]);

  useEffect(() => {
    void reloadModels();

    return () => {
      requestGenerationRef.current += 1;
    };
  }, [reloadModels]);

  return {
    models,
    modelsLoading,
    reloadModels,
    selectedModel,
    selectedModelType,
    setSelectedModelType,
  };
};

const useSyncRuntimeSettingsWithResourceSelection = ({
  enabled = true,
  resourceSelection,
  setRuntimeSettings,
}: {
  enabled?: boolean;
  resourceSelection: ReturnType<typeof createDefaultWorkbenchResourceSelection>;
  setRuntimeSettings: Dispatch<
    SetStateAction<ReturnType<typeof createDefaultWorkbenchRuntimeSettings>>
  >;
}) => {
  useEffect(() => {
    if (!enabled) {
      return;
    }

    setRuntimeSettings(prevSettings => ({
      ...prevSettings,
      skills: {
        ...prevSettings.skills,
        enabled:
          resourceSelection.enable_skills.length > 0
            ? true
            : prevSettings.skills.enabled,
        allowed_skills: [...resourceSelection.enable_skills],
      },
      mcp_tools: {
        ...prevSettings.mcp_tools,
        enabled:
          resourceSelection.enable_mcp.length > 0
            ? true
            : prevSettings.mcp_tools.enabled,
        allowed_tools: [...resourceSelection.enable_mcp],
      },
    }));
  }, [
    enabled,
    resourceSelection.enable_mcp,
    resourceSelection.enable_skills,
    setRuntimeSettings,
  ]);
};

const useWorkbenchSkillSuggestions = ({
  disabled,
  onValueChange,
  spaceId,
  value,
}: {
  disabled: boolean;
  onValueChange: (value: string) => void;
  spaceId?: string;
  value: string;
}) => {
  const [textareaFocused, setTextareaFocused] = useState(false);
  const requestScope =
    !disabled && spaceId && getLeadingSlashSkillQuery(value) !== null
      ? spaceId
      : '';
  const requestScopeRef = useRef(requestScope);
  const requestGenerationRef = useRef(0);
  const [skillState, setSkillState] = useState<{
    scope: string;
    skills: Skill[];
  }>({ scope: '', skills: [] });
  const [skillSuggestionIndex, setSkillSuggestionIndex] = useState(0);
  const [dismissedSkillSuggestionValue, setDismissedSkillSuggestionValue] =
    useState('');
  const slashSkillQuery = useMemo(
    () => getLeadingSlashSkillQuery(value),
    [value],
  );
  if (requestScopeRef.current !== requestScope) {
    requestScopeRef.current = requestScope;
    requestGenerationRef.current += 1;
  }
  const skillScopeReady =
    Boolean(requestScope) && skillState.scope === requestScope;
  const skills = skillScopeReady ? skillState.skills : [];
  const skillSuggestions = useMemo(
    () =>
      slashSkillQuery === null
        ? []
        : getMatchingSkillSuggestions(skills, slashSkillQuery),
    [skills, slashSkillQuery],
  );
  const showSkillSuggestions =
    !disabled &&
    skillScopeReady &&
    textareaFocused &&
    slashSkillQuery !== null &&
    skillSuggestions.length > 0 &&
    dismissedSkillSuggestionValue !== value;

  useEffect(() => {
    if (!requestScope) {
      setSkillState({ scope: '', skills: [] });
      return;
    }

    let canceled = false;
    const requestGeneration = ++requestGenerationRef.current;
    const submittedScope = requestScope;
    setSkillState({ scope: submittedScope, skills: [] });

    void listSkills({ space_id: submittedScope, enabled: true })
      .then(response => {
        if (
          !canceled &&
          requestGenerationRef.current === requestGeneration &&
          requestScopeRef.current === submittedScope
        ) {
          setSkillState({
            scope: submittedScope,
            skills: response.data?.skills ?? [],
          });
        }
      })
      .catch(() => {
        if (
          !canceled &&
          requestGenerationRef.current === requestGeneration &&
          requestScopeRef.current === submittedScope
        ) {
          setSkillState({ scope: submittedScope, skills: [] });
        }
      });

    return () => {
      canceled = true;
    };
  }, [requestScope]);

  useEffect(() => {
    setDismissedSkillSuggestionValue('');
    setSkillSuggestionIndex(0);
  }, [requestScope]);

  useEffect(() => {
    setSkillSuggestionIndex(0);
  }, [slashSkillQuery, skillSuggestions.length]);

  const handleSkillSuggestionApply = (skill: { name: string }) => {
    if (
      disabled ||
      !skillScopeReady ||
      requestScopeRef.current !== requestScope
    ) {
      return;
    }

    const nextValue = `/${skill.name} `;
    onValueChange(nextValue);
    setDismissedSkillSuggestionValue(nextValue);
  };

  const handleSkillSuggestionKeyDown = (
    event: KeyboardEvent<HTMLTextAreaElement>,
  ) => {
    if (disabled || !showSkillSuggestions) {
      return;
    }

    if (event.key === 'ArrowDown') {
      event.preventDefault();
      setSkillSuggestionIndex(index => (index + 1) % skillSuggestions.length);
      return;
    }

    if (event.key === 'ArrowUp') {
      event.preventDefault();
      setSkillSuggestionIndex(
        index =>
          (index - 1 + skillSuggestions.length) % skillSuggestions.length,
      );
      return;
    }

    if ((event.key === 'Enter' || event.key === 'Tab') && !event.shiftKey) {
      event.preventDefault();
      const selectedSkill = skillSuggestions[skillSuggestionIndex];
      if (selectedSkill) {
        handleSkillSuggestionApply(selectedSkill);
      }
      return;
    }

    if (event.key === 'Escape') {
      event.preventDefault();
      setDismissedSkillSuggestionValue(value);
    }
  };

  return {
    handleSkillSuggestionApply,
    handleSkillSuggestionKeyDown,
    setSkillSuggestionIndex,
    setTextareaFocused,
    showSkillSuggestions,
    skillSuggestionIndex,
    skillSuggestions,
  };
};

const createSubmitHandler = ({
  loading,
  models,
  onStop,
  onSubmit,
  resourceSelection,
  runtimeSettings,
  selectedModel,
  files,
  stopLoading,
  stopMode,
  taskId,
  value,
}: {
  loading: boolean;
  models: WorkbenchLLMModel[];
  onStop?: () => void | Promise<void>;
  onSubmit: (payload: WorkbenchComposerSubmitPayload) => void | Promise<void>;
  resourceSelection: ReturnType<typeof createDefaultWorkbenchResourceSelection>;
  runtimeSettings: ReturnType<typeof createDefaultWorkbenchRuntimeSettings>;
  selectedModel?: WorkbenchLLMModel;
  files?: File[];
  stopLoading?: boolean;
  stopMode?: boolean;
  taskId?: string;
  value: string;
}) => {
  if (stopMode) {
    if (!stopLoading) {
      void onStop?.();
    }
    return;
  }

  const message = value.trim();

  if (!message || loading) {
    return;
  }

  onSubmit(
    createWorkbenchSubmitPayload({
      message,
      taskId,
      selectedModel,
      models,
      resourceSelection,
      runtimeSettings,
      files,
    }),
  );
};

const createWorkbenchAtSegmentId = () =>
  `at-segment-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;

const createTextSegment = (text: string): WorkbenchAtSegment | undefined =>
  text
    ? {
        id: createWorkbenchAtSegmentId(),
        text,
        type: 'text',
      }
    : undefined;

const createReferenceSegment = (
  reference: WorkbenchAtReference,
): WorkbenchAtSegment => ({
  id: createWorkbenchAtSegmentId(),
  reference,
  type: 'reference',
});

const createWorkbenchComposerMessage = (
  segments: WorkbenchAtSegment[],
  value: string,
) => {
  const segmentMessage = segments.reduce((message, segment, index) => {
    if (segment.type === 'text') {
      const previousSegment = segments[index - 1];
      const separator =
        previousSegment?.type === 'reference' &&
        segment.text &&
        !segment.text.startsWith(' ')
          ? ' '
          : '';

      return `${message}${separator}${segment.text}`;
    }

    const separator = message && !message.endsWith(' ') ? ' ' : '';

    return `${message}${separator}@${segment.reference.name}`;
  }, '');
  const lastSegment = segments[segments.length - 1];
  const valueSeparator =
    lastSegment?.type === 'reference' && value && !value.startsWith(' ')
      ? ' '
      : '';

  return `${segmentMessage}${valueSeparator}${value}`;
};

const addReferenceSelection = (
  selection: WorkbenchResourceSelection,
  reference: WorkbenchAtReference,
) => {
  if (reference.resourceType === '技能') {
    return addSkillSelection(selection, reference.id);
  }

  if (reference.resourceType === '知识库') {
    return addKnowledgeSelection(selection, reference.id);
  }

  if (reference.resourceType === '数据库') {
    return addDatabaseSelection(selection, reference.id);
  }

  return selection;
};

const mergeResourceSelection = (
  persistentSelection: WorkbenchResourceSelection,
  messageSelection: WorkbenchResourceSelection,
): WorkbenchResourceSelection => ({
  enable_skills: Array.from(
    new Set([
      ...persistentSelection.enable_skills,
      ...messageSelection.enable_skills,
    ]),
  ),
  explicit_enable_skills: Array.from(
    new Set([
      ...persistentSelection.explicit_enable_skills,
      ...messageSelection.explicit_enable_skills,
    ]),
  ),
  enable_mcp: Array.from(
    new Set([
      ...persistentSelection.enable_mcp,
      ...messageSelection.enable_mcp,
    ]),
  ),
  enable_kbs: Array.from(
    new Set([
      ...persistentSelection.enable_kbs,
      ...messageSelection.enable_kbs,
    ]),
  ),
  enable_databases: Array.from(
    new Set([
      ...persistentSelection.enable_databases,
      ...messageSelection.enable_databases,
    ]),
  ),
});

const getMessageResourceSelection = (
  segments: WorkbenchAtSegment[],
): WorkbenchResourceSelection =>
  segments.reduce(
    (selection, segment) =>
      segment.type === 'reference'
        ? addReferenceSelection(selection, segment.reference)
        : selection,
    createDefaultWorkbenchResourceSelection(),
  );

export const WorkbenchComposer = ({
  value,
  loading,
  error,
  variant = 'home',
  presentation = 'default',
  spaceId,
  taskId,
  resetKey,
  capabilities,
  footerEnd,
  stopLoading,
  stopMode,
  modelLoader,
  onValueChange,
  onStop,
  onSubmit,
}: WorkbenchComposerProps) => {
  const [activeOverlay, setActiveOverlay] =
    useState<WorkbenchComposerActiveOverlay>(null);
  const [atDraft, setAtDraft] = useState<WorkbenchAtDraft | null>(null);
  const [atMenuAnchorPosition, setAtMenuAnchorPosition] =
    useState<CSSProperties>();
  const [atSegments, setAtSegments] = useState<WorkbenchAtSegment[]>([]);
  const [files, setFiles] = useState<File[]>([]);
  const composerRef = useRef<HTMLElement>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const atMenuReturnFocusRef = useRef<'editor' | 'trigger'>('editor');
  const previousResetKeyRef = useRef(resetKey);
  const previousTaskIdRef = useRef(taskId);
  const [resourceSelection, setResourceSelection] = useState(
    () =>
      readStoredExtensionUsage(spaceId)?.resourceSelection ??
      createDefaultWorkbenchResourceSelection(),
  );
  const resourceScope = spaceId || 'default';
  const [resourceSelectionScope, setResourceSelectionScope] =
    useState(resourceScope);
  const resourceSelectionReady = resourceSelectionScope === resourceScope;
  const emptyResourceSelection = useMemo(
    () => createDefaultWorkbenchResourceSelection(),
    [],
  );
  const scopedResourceSelection = resourceSelectionReady
    ? resourceSelection
    : emptyResourceSelection;
  const [runtimeSettings, setRuntimeSettings] = useState(() =>
    createRuntimeSettingsFromStoredUsage(readStoredExtensionUsage(spaceId)),
  );
  const composerMessage = createWorkbenchComposerMessage(atSegments, value);
  const messageResourceSelection = useMemo(
    () => getMessageResourceSelection(atSegments),
    [atSegments],
  );
  const submitResourceSelection = useMemo(
    () =>
      mergeResourceSelection(scopedResourceSelection, messageResourceSelection),
    [messageResourceSelection, scopedResourceSelection],
  );
  const attachmentsEnabled = capabilities?.attachments !== false;
  const attachmentDisabledReason =
    capabilities?.attachmentDisabledReason ?? '当前场景暂不支持附件';
  const interactionDisabled = loading || Boolean(stopMode);
  const canSend =
    Boolean(composerMessage.trim()) && !loading && resourceSelectionReady;
  const overlayPlacement = variant === 'detail' ? 'top' : 'bottom';
  const atMenuOpen = activeOverlay === 'at';
  const extensionsOpen = activeOverlay === 'extensions';
  const modelMenuOpen = activeOverlay === 'model';
  const {
    models,
    modelsLoading,
    reloadModels,
    selectedModel,
    selectedModelType,
    setSelectedModelType,
  } = useWorkbenchModelSelection({ spaceId, modelLoader });
  const failoverCandidateCount = getWorkbenchFailoverCandidateModelIds(
    models,
    selectedModelType,
  ).length;
  const {
    handleSkillSuggestionApply,
    handleSkillSuggestionKeyDown,
    setSkillSuggestionIndex,
    setTextareaFocused,
    showSkillSuggestions,
    skillSuggestionIndex,
    skillSuggestions,
  } = useWorkbenchSkillSuggestions({
    disabled: interactionDisabled,
    onValueChange,
    spaceId,
    value,
  });

  useSyncRuntimeSettingsWithResourceSelection({
    enabled: resourceSelectionReady,
    resourceSelection: scopedResourceSelection,
    setRuntimeSettings,
  });

  useEffect(() => {
    const storedUsage = readStoredExtensionUsage(spaceId);
    setResourceSelectionScope(resourceScope);
    setResourceSelection(
      storedUsage?.resourceSelection ??
        createDefaultWorkbenchResourceSelection(),
    );
    setRuntimeSettings(createRuntimeSettingsFromStoredUsage(storedUsage));
  }, [resourceScope, spaceId]);

  useEffect(() => {
    if (!resourceSelectionReady) {
      return;
    }

    writeStoredExtensionUsage({
      resourceSelection: scopedResourceSelection,
      runtimeSettings,
      spaceId,
    });
  }, [
    resourceSelectionReady,
    runtimeSettings.mcp_tools.enabled,
    runtimeSettings.skills.enabled,
    scopedResourceSelection,
    spaceId,
  ]);

  useEffect(() => {
    if (Object.is(previousResetKeyRef.current, resetKey)) {
      return;
    }

    previousResetKeyRef.current = resetKey;
    setActiveOverlay(null);
    setAtDraft(null);
    setAtMenuAnchorPosition(undefined);
    setFiles([]);
    setAtSegments([]);
    if (fileInputRef.current) {
      fileInputRef.current.value = '';
    }
    // resetKey is the explicit success boundary; current segments belong to
    // the completed draft and must not become effect dependencies.
  }, [resetKey]);

  useEffect(() => {
    if (Object.is(previousTaskIdRef.current, taskId)) {
      return;
    }

    previousTaskIdRef.current = taskId;
    setActiveOverlay(null);
    setAtDraft(null);
    setAtMenuAnchorPosition(undefined);
    setFiles([]);
    setAtSegments([]);
    if (fileInputRef.current) {
      fileInputRef.current.value = '';
    }
  }, [taskId]);

  useEffect(() => {
    if (attachmentsEnabled) {
      return;
    }

    setFiles([]);
    if (fileInputRef.current) {
      fileInputRef.current.value = '';
    }
  }, [attachmentsEnabled]);

  useEffect(() => {
    if (interactionDisabled) {
      setActiveOverlay(null);
    }
  }, [interactionDisabled]);

  const commitCurrentText = (text: string) => {
    const textSegment = createTextSegment(text);

    if (textSegment) {
      setAtSegments(prevSegments => [...prevSegments, textSegment]);
    }
  };

  const startAtResourceSelection = (
    textBeforeAt = value,
    returnFocus: 'editor' | 'trigger' = 'editor',
  ) => {
    if (interactionDisabled) {
      return;
    }

    commitCurrentText(textBeforeAt);
    atMenuReturnFocusRef.current = returnFocus;
    onValueChange('');
    setAtMenuAnchorPosition(undefined);
    setAtDraft({ stage: 'resource-type', query: '' });
    setActiveOverlay('at');
  };

  const closeAtResourceSelection = (
    reason?: 'escape' | 'outside' | 'selection',
  ) => {
    setAtMenuAnchorPosition(undefined);
    setAtDraft(null);
    setActiveOverlay(null);
    queueMicrotask(() => {
      if (reason === 'selection' || atMenuReturnFocusRef.current === 'editor') {
        composerRef.current
          ?.querySelector<HTMLTextAreaElement>(
            'textarea[aria-label="任务描述"]',
          )
          ?.focus();
      } else {
        composerRef.current
          ?.querySelector<HTMLButtonElement>('button[aria-label="添加上下文"]')
          ?.focus();
      }
    });
  };

  useEffect(() => {
    if (!atMenuOpen) {
      return;
    }

    const handleOutsideMouseDown = (event: MouseEvent) => {
      if (
        event.target instanceof Node &&
        !composerRef.current?.contains(event.target)
      ) {
        closeAtResourceSelection('outside');
      }
    };

    document.addEventListener('mousedown', handleOutsideMouseDown);

    return () =>
      document.removeEventListener('mousedown', handleOutsideMouseDown);
  }, [atMenuOpen]);

  const handleAtAnchorRectChange = useCallback(
    (anchorRect: DOMRect) => {
      const composerRect = composerRef.current?.getBoundingClientRect();

      if (!composerRect) {
        return;
      }

      const rawAnchorLeft = anchorRect.left - composerRect.left;
      const rawAnchorTop = anchorRect.top - composerRect.top;
      const rawAnchorBottom = anchorRect.bottom - composerRect.top;
      const maxAnchorLeft = Math.max(
        0,
        composerRect.width - WORKBENCH_AT_MENU_WIDTH,
      );
      const nextAnchorLeft = Math.min(
        Math.max(0, rawAnchorLeft),
        maxAnchorLeft,
      );

      setAtMenuAnchorPosition({
        left: nextAnchorLeft,
        ...(overlayPlacement === 'top'
          ? {
              bottom:
                composerRect.height - rawAnchorTop + WORKBENCH_AT_MENU_GAP,
            }
          : {
              top: rawAnchorBottom + WORKBENCH_AT_MENU_GAP,
            }),
      });
    },
    [overlayPlacement],
  );

  const handleAtDraftQueryChange = (query: string) => {
    if (interactionDisabled) {
      return;
    }

    setAtDraft(prevDraft => (prevDraft ? { ...prevDraft, query } : prevDraft));
  };

  const handleAtResourceTypeSelect = (
    resourceType: WorkbenchAtResourceType,
  ) => {
    if (interactionDisabled) {
      return;
    }

    setAtDraft({
      stage: 'resource-search',
      resourceType,
      query: '',
    });
    setActiveOverlay('at');
  };

  const handleAtReferenceSelect = (reference: WorkbenchAtReference) => {
    if (interactionDisabled) {
      return;
    }

    setAtSegments(prevSegments => [
      ...prevSegments,
      createReferenceSegment(reference),
    ]);
    setAtDraft(null);
  };

  const handleAtSegmentRemove = (segment: WorkbenchAtSegment) => {
    if (interactionDisabled || segment.type !== 'reference') {
      return;
    }

    setAtSegments(prevSegments =>
      prevSegments.filter(item => item.id !== segment.id),
    );
  };

  const handleLastAtSegmentRemove = () => {
    setAtSegments(prevSegments => {
      const lastSegment = prevSegments[prevSegments.length - 1];

      if (!lastSegment) {
        return prevSegments;
      }

      const nextSegments = prevSegments.slice(0, -1);

      if (lastSegment.type === 'reference') {
        return nextSegments;
      }

      onValueChange(lastSegment.text);

      return nextSegments;
    });
  };

  const handleEditorKeyDown = (event: KeyboardEvent<HTMLTextAreaElement>) => {
    if (interactionDisabled) {
      event.preventDefault();
      return;
    }

    if (
      event.key === 'Backspace' &&
      !value &&
      !atDraft &&
      atSegments.length > 0
    ) {
      event.preventDefault();
      handleLastAtSegmentRemove();

      return;
    }

    handleSkillSuggestionKeyDown(event);
  };

  const handleValueChange = (nextValue: string) => {
    if (interactionDisabled) {
      return;
    }

    if (nextValue.endsWith('@')) {
      startAtResourceSelection(nextValue.slice(0, -1));

      return;
    }

    onValueChange(nextValue);
  };

  const handleSubmit = () => {
    createSubmitHandler({
      files,
      loading: loading || !resourceSelectionReady,
      models,
      onStop,
      onSubmit,
      resourceSelection: submitResourceSelection,
      runtimeSettings,
      selectedModel,
      stopLoading,
      stopMode,
      taskId,
      value: composerMessage,
    });
  };

  const handleAttachClick = () => {
    if (interactionDisabled || !attachmentsEnabled) {
      return;
    }

    fileInputRef.current?.click();
  };

  const handleFileInputChange = (event: ChangeEvent<HTMLInputElement>) => {
    if (interactionDisabled || !attachmentsEnabled) {
      event.target.value = '';
      return;
    }

    const nextFiles = Array.from(event.target.files ?? []);
    if (nextFiles.length > 0) {
      setFiles(prevFiles => [...prevFiles, ...nextFiles].slice(0, 10));
    }
    event.target.value = '';
  };

  const handleFileRemove = (targetFile: File) => {
    if (interactionDisabled) {
      return;
    }

    setFiles(prevFiles => prevFiles.filter(file => file !== targetFile));
  };

  return (
    <ChatComposer
      rootRef={composerRef}
      ariaLabel="任务输入"
      className="chat-workbench-composer"
      composerStyle={presentation}
      error={error}
      footerEnd={footerEnd}
      onChange={handleValueChange}
      onEditorKeyDown={handleEditorKeyDown}
      onStop={onStop}
      onSubmit={handleSubmit}
      showDefaultAction={false}
      stopping={stopLoading}
      streaming={stopMode}
      submitting={loading}
      value={composerMessage}
      variant={variant === 'detail' ? 'docked' : 'hero'}
      overlay={
        atMenuOpen ? (
          <AtMenu
            anchorPosition={atMenuAnchorPosition}
            disabled={interactionDisabled}
            draft={atDraft ?? { stage: 'resource-type', query: '' }}
            eventScopeRef={composerRef}
            placement={overlayPlacement}
            spaceId={spaceId}
            value={submitResourceSelection}
            onClose={closeAtResourceSelection}
            onBackToResourceTypes={() =>
              setAtDraft({ stage: 'resource-type', query: '' })
            }
            onReferenceSelect={handleAtReferenceSelect}
            onResourceTypeSelect={handleAtResourceTypeSelect}
          />
        ) : null
      }
      renderEditor={editorProps => (
        <WorkbenchComposerBody
          atDraft={atDraft}
          atSegments={atSegments}
          variant={variant}
          value={value}
          presentation={presentation}
          showSkillSuggestions={showSkillSuggestions}
          skillSuggestionPlacement={overlayPlacement}
          skillSuggestionIndex={skillSuggestionIndex}
          skillSuggestions={skillSuggestions}
          onChange={editorProps.onChange}
          disabled={editorProps.disabled}
          readOnly={editorProps.readOnly}
          onCompositionEnd={editorProps.onCompositionEnd}
          onCompositionStart={editorProps.onCompositionStart}
          onSkillSuggestionApply={handleSkillSuggestionApply}
          onSkillSuggestionIndexChange={setSkillSuggestionIndex}
          onSkillSuggestionKeyDown={editorProps.onKeyDown}
          onAtAnchorRectChange={handleAtAnchorRectChange}
          onAtDraftCancel={closeAtResourceSelection}
          onAtDraftQueryChange={handleAtDraftQueryChange}
          onAtSegmentRemove={handleAtSegmentRemove}
          files={files}
          onFileRemove={handleFileRemove}
          onTextareaBlur={() => setTextareaFocused(false)}
          onTextareaFocus={() => setTextareaFocused(true)}
        />
      )}
      auxiliary={
        <>
          <input
            ref={fileInputRef}
            type="file"
            multiple
            disabled={interactionDisabled || !attachmentsEnabled}
            className="chat-workbench-file-input"
            aria-label="选择附件"
            title={attachmentsEnabled ? undefined : attachmentDisabledReason}
            onChange={handleFileInputChange}
          />
          {!attachmentsEnabled ? (
            <div className="chat-workbench-capability-note" role="note">
              {attachmentDisabledReason}
            </div>
          ) : null}
        </>
      }
      footerStart={
        <WorkbenchComposerToolbar
          atMenuOpen={atMenuOpen}
          canSend={canSend}
          extensionsOpen={extensionsOpen}
          disabled={interactionDisabled}
          attachmentsEnabled={attachmentsEnabled}
          attachmentDisabledReason={attachmentDisabledReason}
          loading={loading}
          modelLoader={modelLoader}
          modelMenuOpen={modelMenuOpen}
          models={models}
          modelsLoading={modelsLoading}
          overlayPlacement={overlayPlacement}
          presentation={presentation}
          resourceSelection={scopedResourceSelection}
          runtimeSettings={runtimeSettings}
          selectedModelType={selectedModelType}
          stopLoading={stopLoading}
          stopMode={stopMode}
          failoverCandidateCount={failoverCandidateCount}
          spaceId={spaceId}
          onAtMenuOpenChange={open =>
            open
              ? startAtResourceSelection(value, 'trigger')
              : closeAtResourceSelection()
          }
          onAttachClick={handleAttachClick}
          onExtensionsOpenChange={open =>
            setActiveOverlay(open ? 'extensions' : null)
          }
          onModelMenuOpenChange={open => {
            if (open) {
              void reloadModels();
            }
            setActiveOverlay(open ? 'model' : null);
          }}
          onReloadModels={reloadModels}
          onResourceSelectionChange={nextSelection => {
            if (resourceSelectionReady) {
              setResourceSelection(nextSelection);
            }
          }}
          onRuntimeSettingsChange={setRuntimeSettings}
          onSelectedModelTypeChange={setSelectedModelType}
          onSubmit={handleSubmit}
        />
      }
    />
  );
};
