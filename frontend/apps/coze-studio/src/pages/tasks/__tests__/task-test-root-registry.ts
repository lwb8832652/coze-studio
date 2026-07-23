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

import { afterEach } from 'vitest';
import { act } from 'react-dom/test-utils';
import { createRoot as createReactRoot, type Root } from 'react-dom/client';

const roots = new Map<Root, Parameters<typeof createReactRoot>[0]>();

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

export const createRoot = (
  ...args: Parameters<typeof createReactRoot>
): Root => {
  const root = createReactRoot(...args);
  const originalUnmount = root.unmount.bind(root);

  roots.set(root, args[0]);
  root.unmount = () => {
    roots.delete(root);
    originalUnmount();
  };

  return root;
};

export const cleanupTaskTestRoots = () => {
  const containers = [...roots.values()];

  act(() => {
    for (const root of [...roots.keys()]) {
      root.unmount();
    }
  });
  for (const container of containers) {
    if (container instanceof Element) {
      container.remove();
    }
  }
  roots.clear();
};

afterEach(() => {
  cleanupTaskTestRoots();
});

export type { Root };
