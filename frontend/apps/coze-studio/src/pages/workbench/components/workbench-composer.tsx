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

import { useEffect, useState } from 'react';

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
} from './types';

type WorkbenchComposerActiveOverlay =
  | 'at'
  | 'extensions'
  | 'mode'
  | 'model'
  | null;

export interface WorkbenchComposerProps {
  value: string;
  mode: WorkbenchMode;
  loading: boolean;
  error?: string;
  variant?: WorkbenchComposerVariant;
  presentation?: WorkbenchComposerPresentation;
  spaceId?: string;
  taskId?: string;
  modelLoader?: (spaceId: string) => Promise<WorkbenchLLMModel[]>;
  onValueChange: (value: string) => void;
  onModeChange: (mode: WorkbenchMode) => void;
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

export const WorkbenchComposer = ({
  value,
  mode,
  loading,
  error,
  variant = 'home',
  presentation = 'default',
  spaceId,
  taskId,
  modelLoader,
  onValueChange,
  onModeChange,
  onSubmit,
}: WorkbenchComposerProps) => {
  const [activeOverlay, setActiveOverlay] =
    useState<WorkbenchComposerActiveOverlay>(null);
  const [resourceSelection, setResourceSelection] = useState(
    createDefaultWorkbenchResourceSelection,
  );
  const [runtimeSettings, setRuntimeSettings] = useState(
    createDefaultWorkbenchRuntimeSettings,
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

  useEffect(() => {
    setRuntimeSettings(prevSettings => ({
      ...prevSettings,
      skills: {
        ...prevSettings.skills,
        enabled: resourceSelection.enable_skills.length > 0,
        allowed_skills: [...resourceSelection.enable_skills],
      },
      mcp_tools: {
        ...prevSettings.mcp_tools,
        enabled: resourceSelection.enable_mcp.length > 0,
        allowed_tools: [...resourceSelection.enable_mcp],
      },
    }));
  }, [resourceSelection.enable_mcp, resourceSelection.enable_skills]);

  const handleValueChange = (nextValue: string) => {
    onValueChange(nextValue);
    if (nextValue.endsWith('@')) {
      setActiveOverlay('at');
    } else if (activeOverlay === 'at') {
      setActiveOverlay(null);
    }
  };

  const handleSubmit = () => {
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
            onClose={() => setActiveOverlay(null)}
          />
        ) : null}
        <WorkbenchComposerBody
          value={value}
          mode={mode}
          presentation={presentation}
          onChange={handleValueChange}
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
