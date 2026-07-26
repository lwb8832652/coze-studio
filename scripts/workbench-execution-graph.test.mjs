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
import { mkdtemp, mkdir, readFile, rm, writeFile } from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

import {
  evaluateAuthorityChanges,
  validateContract,
} from './workbench-execution-graph/contract.mjs';

const REPO_ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const CONTRACT_PATH = path.join(
  REPO_ROOT,
  'docs/superpowers/context/workbench-execution-graph.json',
);
const CONTRACT_RELATIVE =
  'docs/superpowers/context/workbench-execution-graph.json';
const CONTEXT_RELATIVE =
  'docs/superpowers/context/workbench-execution-chain.md';

const loadCanonicalContract = async () =>
  JSON.parse(await readFile(CONTRACT_PATH, 'utf8'));

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

test('canonical Workbench execution contract is valid', async () => {
  const contract = await loadCanonicalContract();
  const result = await validateContract(contract, { repoRoot: REPO_ROOT });
  assert.deepEqual(result.errors, []);
});

test('rejects framework version drift from repository manifests', async () => {
  const contract = await loadCanonicalContract();
  contract.nodes.find(node => node.id === 'framework.eino').version = 'v0.0.0';

  const result = await validateContract(contract, { repoRoot: REPO_ROOT });
  assert.match(result.errors.join('\n'), /framework_version_mismatch/);
});

test('requires canonical frameworks to anchor production code', async () => {
  const contract = await loadCanonicalContract();
  contract.nodes.find(node => node.id === 'framework.eino').source_anchors = [
    {
      path: 'backend/go.mod',
      locator: 'github.com/cloudwego/eino v0.9.9',
    },
  ];

  const result = await validateContract(contract, { repoRoot: REPO_ROOT });
  assert.match(result.errors.join('\n'), /canonical_framework_anchor_missing/);
});

test('rejects middleware order drift from adkMiddlewareOrder', async () => {
  const contract = await loadCanonicalContract();
  const chain = contract.chains.find(
    item => item.id === 'framework.eino_middleware_order',
  );
  [chain.ordered_node_ids[0], chain.ordered_node_ids[1]] = [
    chain.ordered_node_ids[1],
    chain.ordered_node_ids[0],
  ];

  const result = await validateContract(contract, { repoRoot: REPO_ROOT });
  assert.match(result.errors.join('\n'), /middleware_order_mismatch/);
});

test('rejects compatibility nodes from canonical execution chains', async () => {
  const contract = await loadCanonicalContract();
  const chain = contract.chains.find(
    item => item.id === 'framework.canonical_stack',
  );
  chain.ordered_node_ids[0] = 'compat.langgraph.create_run';

  const result = await validateContract(contract, { repoRoot: REPO_ROOT });
  assert.match(result.errors.join('\n'), /noncanonical_runtime_in_execution_chain/);
});

for (const forbiddenTerm of ['K2', 'ChatTask']) {
  test(`rejects forbidden current ${forbiddenTerm} nodes`, async () => {
    const contract = await loadCanonicalContract();
    const node = structuredClone(contract.nodes[0]);
    node.id = `forbidden.${forbiddenTerm.toLowerCase()}`;
    node.label = `${forbiddenTerm} current runtime`;
    contract.nodes.push(node);

    const result = await validateContract(contract, { repoRoot: REPO_ROOT });
    assert.match(result.errors.join('\n'), /forbidden_current_node/);
  });
}

test('requires authority files to move together with monitored source', () => {
  const scope = {
    monitored_paths: ['backend/application/agentthread/**'],
    authority_files: [CONTRACT_RELATIVE, CONTEXT_RELATIVE],
  };

  assert.deepEqual(
    evaluateAuthorityChanges(
      ['backend/application/agentthread/adk_executor.go'],
      scope,
    ),
    ['authority_files_not_updated'],
  );
  assert.deepEqual(
    evaluateAuthorityChanges([CONTRACT_RELATIVE], scope),
    ['authority_files_must_change_together'],
  );
  assert.deepEqual(
    evaluateAuthorityChanges([CONTRACT_RELATIVE, CONTEXT_RELATIVE], scope),
    [],
  );
});
