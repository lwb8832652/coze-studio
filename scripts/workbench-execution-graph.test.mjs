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

import assert from 'node:assert/strict';
import { mkdtemp, mkdir, rm, writeFile } from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import test from 'node:test';

import { validateContract } from './workbench-execution-graph/contract.mjs';

const makeContract = () => ({
  schema_version: 1,
  authority: {
    contract: 'contract.json',
    document: 'authority.md',
  },
  scope: {
    forbidden_current_terms: ['K2', 'ChatTask'],
    monitored_paths: ['src/**'],
  },
  nodes: [
    {
      id: 'entry.start',
      label: 'Start',
      layer: 'entry',
      kind: 'function',
      production_status: 'current',
      source_anchors: [
        {
          path: 'src/start.mjs',
          symbol: 'start',
          locator: 'export const start',
        },
      ],
      evidence: [
        {
          path: 'test/start.test.mjs',
          locator: "test('start reaches finish'",
        },
      ],
    },
    {
      id: 'result.finish',
      label: 'Finish',
      layer: 'result',
      kind: 'function',
      production_status: 'current',
      source_anchors: [
        {
          path: 'src/finish.mjs',
          symbol: 'finish',
          locator: 'export const finish',
        },
      ],
      evidence: [
        {
          path: 'test/start.test.mjs',
          locator: "test('start reaches finish'",
        },
      ],
    },
  ],
  edges: [
    {
      id: 'edge.start_finish',
      from: 'entry.start',
      to: 'result.finish',
      relation: 'calls',
      chain_ids: ['chain.valid'],
      confidence: 'extracted',
      evidence: [
        {
          path: 'src/start.mjs',
          locator: 'finish()',
        },
      ],
    },
  ],
  chains: [
    {
      id: 'chain.valid',
      name: 'Valid chain',
      entry: 'entry.start',
      terminal: 'result.finish',
      ordered_node_ids: ['entry.start', 'result.finish'],
      ordered_edge_ids: ['edge.start_finish'],
      production_status: 'current',
      canonical_executor: true,
      test_evidence: [
        {
          path: 'test/start.test.mjs',
          locator: "test('start reaches finish'",
        },
      ],
    },
  ],
  exclusions: [],
  required_queries: [],
});

const withFixture = async callback => {
  const repoRoot = await mkdtemp(path.join(os.tmpdir(), 'workbench-graph-'));
  try {
    await mkdir(path.join(repoRoot, 'src'), { recursive: true });
    await mkdir(path.join(repoRoot, 'test'), { recursive: true });
    await writeFile(
      path.join(repoRoot, 'src/start.mjs'),
      'export const start = () => finish();\n',
      'utf8',
    );
    await writeFile(
      path.join(repoRoot, 'src/finish.mjs'),
      "export const finish = () => 'done';\n",
      'utf8',
    );
    await writeFile(
      path.join(repoRoot, 'test/start.test.mjs'),
      "test('start reaches finish', () => {});\n",
      'utf8',
    );
    await callback({ repoRoot });
  } finally {
    await rm(repoRoot, { recursive: true, force: true });
  }
};

test('accepts a valid source-anchored ordered contract', async () => {
  await withFixture(async fixture => {
    const result = await validateContract(makeContract(), fixture);
    assert.deepEqual(result.errors, []);
  });
});

test('rejects duplicate node IDs', async () => {
  await withFixture(async fixture => {
    const contract = makeContract();
    contract.nodes.push(structuredClone(contract.nodes[0]));

    const result = await validateContract(contract, fixture);
    assert.match(result.errors.join('\n'), /duplicate_node_id/);
  });
});

test('rejects a dangling edge target', async () => {
  await withFixture(async fixture => {
    const contract = makeContract();
    contract.edges[0].to = 'result.missing';

    const result = await validateContract(contract, fixture);
    assert.match(result.errors.join('\n'), /edge_target_missing/);
  });
});

test('rejects an edge that contradicts declared chain order', async () => {
  await withFixture(async fixture => {
    const contract = makeContract();
    contract.edges[0].from = 'result.finish';
    contract.edges[0].to = 'entry.start';

    const result = await validateContract(contract, fixture);
    assert.match(result.errors.join('\n'), /chain_edge_order_invalid/);
  });
});

test('requires test evidence for every chain', async () => {
  await withFixture(async fixture => {
    const contract = makeContract();
    contract.chains[0].test_evidence = [];

    const result = await validateContract(contract, fixture);
    assert.match(result.errors.join('\n'), /chain_test_evidence_missing/);
  });
});
