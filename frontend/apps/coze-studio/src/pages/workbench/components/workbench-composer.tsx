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

/* eslint-disable max-lines -- P0 composer container, split in phase 2. */

import {
  useEffect,
  useMemo,
  useState,
  type Dispatch,
  type KeyboardEvent,
  type SetStateAction,
} from 'react';

import type { workbenchSkill } from '@coze-studio/api-schema';

import { listSkills } from '../../skill/service';
import { findSelectedWorkbenchModel } from './workbench-model-selector';
import {
  WorkbenchComposerBody,
  WorkbenchComposerToolbar,
  type WorkbenchComposerPresentation,
} from './workbench-composer-controls';
import { AtMenu } from './workbench-composer-at-menu';
import {
  createWorkbenchSubmitPayload,
  createDefaultWorkbenchResourceSelection,
  createDefaultWorkbenchRuntimeSettings,
  getWorkbenchFailoverCandidateModelIds,
  workbenchModelTypeToNumber,
  type WorkbenchLLMModel,
  type WorkbenchComposerSubmitPayload,
  type WorkbenchComposerVariant,
  type WorkbenchMode,
  type WorkbenchResourceSelection,
  type WorkbenchRuntimeSettings,
} from './types';

type Skill = workbenchSkill.Skill;
type WorkbenchComposerActiveOverlay =
  | 'at'
  | 'extensions'
  | 'mode'
  | 'model'
  | null;

const MAX_SLASH_SKILL_SUGGESTIONS = 6;
const EXTENSION_USAGE_STORAGE_VERSION = 1;
const EXTENSION_USAGE_STORAGE_PREFIX = 'coze-workbench-extension-usage';

interface StoredWorkbenchExtensionUsage {
  version: typeof EXTENSION_USAGE_STORAGE_VERSION;
  resourceSelection: WorkbenchResourceSelection;
  skillsEnabled: boolean;
  mcpToolsEnabled: boolean;
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
  mode: WorkbenchMode;
  loading: boolean;
  error?: string;
  variant?: WorkbenchComposerVariant;
  presentation?: WorkbenchComposerPresentation;
  spaceId?: string;
  taskId?: string;
  stopLoading?: boolean;
  stopMode?: boolean;
  modelLoader?: (spaceId: string) => Promise<WorkbenchLLMModel[]>;
  onValueChange: (value: string) => void;
  onModeChange: (mode: WorkbenchMode) => void;
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
  const [models, setModels] = useState<WorkbenchLLMModel[]>([]);
  const [modelsLoading, setModelsLoading] = useState(false);
  const [selectedModelType, setSelectedModelType] = useState<
    number | undefined
  >();
  const selectedModel = findSelectedWorkbenchModel(models, selectedModelType);

  useEffect(() => {
    if (!spaceId || !modelLoader) {
      setModels([]);
      setSelectedModelType(undefined);
      return;
    }

    let canceled = false;
    setModelsLoading(true);

    void modelLoader(spaceId)
      .then(nextModels => {
        if (canceled) {
          return;
        }

        setModels(nextModels);
        setSelectedModelType(prevModelType => {
          if (
            prevModelType &&
            nextModels.some(
              model => workbenchModelTypeToNumber(model) === prevModelType,
            )
          ) {
            return prevModelType;
          }

          return nextModels[0]
            ? workbenchModelTypeToNumber(nextModels[0])
            : undefined;
        });
      })
      .catch(() => {
        if (!canceled) {
          setModels([]);
          setSelectedModelType(undefined);
        }
      })
      .finally(() => {
        if (!canceled) {
          setModelsLoading(false);
        }
      });

    return () => {
      canceled = true;
    };
  }, [modelLoader, spaceId]);

  return {
    models,
    modelsLoading,
    selectedModel,
    selectedModelType,
    setSelectedModelType,
  };
};

const useSyncRuntimeSettingsWithResourceSelection = ({
  resourceSelection,
  setRuntimeSettings,
}: {
  resourceSelection: ReturnType<typeof createDefaultWorkbenchResourceSelection>;
  setRuntimeSettings: Dispatch<
    SetStateAction<ReturnType<typeof createDefaultWorkbenchRuntimeSettings>>
  >;
}) => {
  useEffect(() => {
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
    resourceSelection.enable_mcp,
    resourceSelection.enable_skills,
    setRuntimeSettings,
  ]);
};

const useWorkbenchSkillSuggestions = ({
  loading,
  onValueChange,
  spaceId,
  value,
}: {
  loading: boolean;
  onValueChange: (value: string) => void;
  spaceId?: string;
  value: string;
}) => {
  const [textareaFocused, setTextareaFocused] = useState(false);
  const [skills, setSkills] = useState<Skill[]>([]);
  const [skillSuggestionIndex, setSkillSuggestionIndex] = useState(0);
  const [dismissedSkillSuggestionValue, setDismissedSkillSuggestionValue] =
    useState('');
  const slashSkillQuery = useMemo(
    () => getLeadingSlashSkillQuery(value),
    [value],
  );
  const slashSkillActive = slashSkillQuery !== null;
  const skillSuggestions = useMemo(
    () =>
      slashSkillQuery === null
        ? []
        : getMatchingSkillSuggestions(skills, slashSkillQuery),
    [skills, slashSkillQuery],
  );
  const showSkillSuggestions =
    !loading &&
    textareaFocused &&
    slashSkillQuery !== null &&
    skillSuggestions.length > 0 &&
    dismissedSkillSuggestionValue !== value;

  useEffect(() => {
    if (!spaceId || !slashSkillActive) {
      return;
    }

    let canceled = false;

    void listSkills({ space_id: spaceId, enabled: true })
      .then(response => {
        if (!canceled) {
          setSkills(response.data?.skills ?? []);
        }
      })
      .catch(() => {
        if (!canceled) {
          setSkills([]);
        }
      });

    return () => {
      canceled = true;
    };
  }, [slashSkillActive, spaceId]);

  useEffect(() => {
    setSkillSuggestionIndex(0);
  }, [slashSkillQuery, skillSuggestions.length]);

  const handleSkillSuggestionApply = (skill: { name: string }) => {
    const nextValue = `/${skill.name} `;
    onValueChange(nextValue);
    setDismissedSkillSuggestionValue(nextValue);
  };

  const handleSkillSuggestionKeyDown = (
    event: KeyboardEvent<HTMLTextAreaElement>,
  ) => {
    if (!showSkillSuggestions) {
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
  mode,
  models,
  onStop,
  onSubmit,
  resourceSelection,
  runtimeSettings,
  selectedModel,
  stopLoading,
  stopMode,
  taskId,
  value,
}: {
  loading: boolean;
  mode: WorkbenchMode;
  models: WorkbenchLLMModel[];
  onStop?: () => void | Promise<void>;
  onSubmit: (payload: WorkbenchComposerSubmitPayload) => void | Promise<void>;
  resourceSelection: ReturnType<typeof createDefaultWorkbenchResourceSelection>;
  runtimeSettings: ReturnType<typeof createDefaultWorkbenchRuntimeSettings>;
  selectedModel?: WorkbenchLLMModel;
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
      mode,
      taskId,
      selectedModel,
      models,
      resourceSelection,
      runtimeSettings,
    }),
  );
};

// eslint-disable-next-line @coze-arch/max-line-per-function -- P0 keeps overlay and submit orchestration together.
export const WorkbenchComposer = ({
  value,
  mode,
  loading,
  error,
  variant = 'home',
  presentation = 'default',
  spaceId,
  taskId,
  stopLoading,
  stopMode,
  modelLoader,
  onValueChange,
  onModeChange,
  onStop,
  onSubmit,
}: WorkbenchComposerProps) => {
  const [activeOverlay, setActiveOverlay] =
    useState<WorkbenchComposerActiveOverlay>(null);
  const [resourceSelection, setResourceSelection] = useState(
    () =>
      readStoredExtensionUsage(spaceId)?.resourceSelection ??
      createDefaultWorkbenchResourceSelection(),
  );
  const [runtimeSettings, setRuntimeSettings] = useState(() =>
    createRuntimeSettingsFromStoredUsage(readStoredExtensionUsage(spaceId)),
  );
  const canSend = Boolean(value.trim()) && !loading;
  const overlayPlacement = variant === 'detail' ? 'top' : 'bottom';
  const atMenuOpen = activeOverlay === 'at';
  const extensionsOpen = activeOverlay === 'extensions';
  const modeMenuOpen = activeOverlay === 'mode';
  const modelMenuOpen = activeOverlay === 'model';
  const {
    models,
    modelsLoading,
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
    loading,
    onValueChange,
    spaceId,
    value,
  });

  useSyncRuntimeSettingsWithResourceSelection({
    resourceSelection,
    setRuntimeSettings,
  });

  useEffect(() => {
    const storedUsage = readStoredExtensionUsage(spaceId);
    setResourceSelection(
      storedUsage?.resourceSelection ??
        createDefaultWorkbenchResourceSelection(),
    );
    setRuntimeSettings(createRuntimeSettingsFromStoredUsage(storedUsage));
  }, [spaceId]);

  useEffect(() => {
    writeStoredExtensionUsage({
      resourceSelection,
      runtimeSettings,
      spaceId,
    });
  }, [
    resourceSelection,
    runtimeSettings.mcp_tools.enabled,
    runtimeSettings.skills.enabled,
    spaceId,
  ]);

  const handleValueChange = (nextValue: string) => {
    onValueChange(nextValue);
    if (nextValue.endsWith('@')) {
      setActiveOverlay('at');
    } else if (activeOverlay === 'at') {
      setActiveOverlay(null);
    }
  };

  const handleSubmit = () => {
    createSubmitHandler({
      loading,
      mode,
      models,
      onStop,
      onSubmit,
      resourceSelection,
      runtimeSettings,
      selectedModel,
      stopLoading,
      stopMode,
      taskId,
      value,
    });
  };

  return (
    <>
      <section
        className="chat-workbench-composer"
        data-variant={variant}
        data-composer-style={presentation}
        aria-label="任务输入"
      >
        {atMenuOpen ? (
          <AtMenu
            placement={overlayPlacement}
            spaceId={spaceId}
            value={resourceSelection}
            onClose={() => setActiveOverlay(null)}
            onChange={setResourceSelection}
          />
        ) : null}
        <WorkbenchComposerBody
          value={value}
          mode={mode}
          presentation={presentation}
          showSkillSuggestions={showSkillSuggestions}
          skillSuggestionPlacement={overlayPlacement}
          skillSuggestionIndex={skillSuggestionIndex}
          skillSuggestions={skillSuggestions}
          onChange={handleValueChange}
          onSkillSuggestionApply={handleSkillSuggestionApply}
          onSkillSuggestionIndexChange={setSkillSuggestionIndex}
          onSkillSuggestionKeyDown={handleSkillSuggestionKeyDown}
          onTextareaBlur={() => setTextareaFocused(false)}
          onTextareaFocus={() => setTextareaFocused(true)}
        />

        <WorkbenchComposerToolbar
          atMenuOpen={atMenuOpen}
          canSend={canSend}
          extensionsOpen={extensionsOpen}
          loading={loading}
          modelLoader={modelLoader}
          modelMenuOpen={modelMenuOpen}
          models={models}
          modelsLoading={modelsLoading}
          mode={mode}
          modeMenuOpen={modeMenuOpen}
          overlayPlacement={overlayPlacement}
          presentation={presentation}
          resourceSelection={resourceSelection}
          runtimeSettings={runtimeSettings}
          selectedModelType={selectedModelType}
          stopLoading={stopLoading}
          stopMode={stopMode}
          failoverCandidateCount={failoverCandidateCount}
          spaceId={spaceId}
          onAtMenuOpenChange={open => setActiveOverlay(open ? 'at' : null)}
          onExtensionsOpenChange={open =>
            setActiveOverlay(open ? 'extensions' : null)
          }
          onModelMenuOpenChange={open =>
            setActiveOverlay(open ? 'model' : null)
          }
          onModeMenuOpenChange={open => setActiveOverlay(open ? 'mode' : null)}
          onModeChange={onModeChange}
          onResourceSelectionChange={setResourceSelection}
          onRuntimeSettingsChange={setRuntimeSettings}
          onSelectedModelTypeChange={setSelectedModelType}
          onSubmit={handleSubmit}
        />
      </section>

      {error ? (
        <div className="chat-workbench-error" role="alert">
          {error}
        </div>
      ) : null}
    </>
  );
};
