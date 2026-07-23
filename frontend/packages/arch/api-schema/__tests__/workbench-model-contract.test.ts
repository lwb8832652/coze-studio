/*
 * Copyright 2025 coze-dev Authors
 * SPDX-License-Identifier: Apache-2.0
 */

import { describe, expect, it } from 'vitest';

import * as model from '../src/idl/workbench/model';

describe('workbench model generated contract', () => {
  it('exports the complete workspace model management API surface', () => {
    [
      model.ListWorkspaceModels,
      model.GetWorkspaceModel,
      model.UpsertWorkspaceModel,
      model.TestWorkspaceModel,
      model.SetWorkspaceModelStatus,
      model.DeleteWorkspaceModel,
    ].forEach(client => expect(typeof client).toBe('function'));
  });
});
