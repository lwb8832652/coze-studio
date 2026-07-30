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
import { execFile } from 'node:child_process';
import {
  lstat,
  mkdtemp,
  mkdir,
  readFile,
  readlink,
  rm,
  symlink,
  writeFile,
} from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';
import { promisify } from 'node:util';

import { CLI_HELP } from './workbench-execution-graph.mjs';
import {
  evaluateAuthorityChanges,
  gitChangedPaths,
  validateContract,
} from './workbench-execution-graph/contract.mjs';
import {
  assertNonEmptyASTGraph,
  assertASTSourceCoverage,
  assertManagedBuildRoot,
  astEligibleCorpusFiles,
  buildDerivedGraph,
  computeBuilderDigest,
  DERIVED_MANAGED_MARKER,
  graphifyExtractArgs,
  installCompletedBuild,
  mergeExplicitGraph,
  readGitProvenance,
  renderContractLedger,
  validateCorpusSourcePath,
  verifyDerivedGraph,
  verifyOrderedPaths,
  verifyRequiredQueries,
  writeCorpus,
} from './workbench-execution-graph/derived.mjs';

const execFileAsync = promisify(execFile);
const REPO_ROOT = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  '..',
);
const CLI_PATH = path.join(REPO_ROOT, 'scripts/workbench-execution-graph.mjs');
const CONTRACT_PATH = path.join(
  REPO_ROOT,
  'docs/superpowers/context/workbench-execution-graph.json',
);
const CONTRACT_RELATIVE =
  'docs/superpowers/context/workbench-execution-graph.json';
const CONTEXT_RELATIVE =
  'docs/superpowers/context/workbench-execution-chain.md';
const RUNBOOK_RELATIVE =
  'docs/superpowers/runbooks/workbench-execution-graph.md';

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
    await callback({ repoRoot, requireCanonicalProfile: false });
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

test('canonical profile cannot remove required chains, queries, or exclusions', async () => {
  const cases = [
    {
      mutate: contract => {
        contract.chains = contract.chains.filter(
          chain => chain.id !== 'entry.scheduled_task',
        );
      },
      expected: /profile_chain_missing: entry\.scheduled_task/,
    },
    {
      mutate: contract => {
        contract.required_queries = [];
      },
      expected: /profile_query_missing: query\.framework_inventory/,
    },
    {
      mutate: contract => {
        contract.exclusions = contract.exclusions.filter(
          exclusion => exclusion.id !== 'exclude.k2',
        );
      },
      expected: /profile_exclusion_missing: exclude\.k2/,
    },
  ];

  for (const testCase of cases) {
    const contract = await loadCanonicalContract();
    testCase.mutate(contract);
    const result = await validateContract(contract, { repoRoot: REPO_ROOT });
    assert.match(result.errors.join('\n'), testCase.expected);
  }
});

test('every exclusion requires source-anchored evidence', async () => {
  const contract = await loadCanonicalContract();
  contract.exclusions[0].evidence = [];

  const result = await validateContract(contract, { repoRoot: REPO_ROOT });

  assert.match(
    result.errors.join('\n'),
    /exclusion_evidence_missing: exclude\.k2/,
  );
});

test('canonical profile cannot weaken query, chain, or exclusion facts', async () => {
  const cases = [
    contract => {
      const query = contract.required_queries.find(
        item => item.id === 'query.framework_inventory',
      );
      query.required_node_ids = ['framework.react'];
      query.required_edge_ids = ['edge.ui_uses_react'];
    },
    contract => {
      const chain = contract.chains.find(
        item => item.id === 'framework.eino_sdk',
      );
      chain.required_side_edge_ids = [];
    },
    contract => {
      const exclusion = contract.exclusions.find(
        item => item.id === 'exclude.external_queue',
      );
      exclusion.term = 'Redis queue';
    },
    contract => {
      contract.authority.rule = 'Generated inference is authoritative.';
    },
  ];

  for (const mutate of cases) {
    const contract = await loadCanonicalContract();
    mutate(contract);
    const result = await validateContract(contract, { repoRoot: REPO_ROOT });
    assert.match(
      result.errors.join('\n'),
      /canonical_profile_structure_mismatch/,
    );
  }
});

test('canonical profile remains enforced for fault-injection copies', async () => {
  const contract = await loadCanonicalContract();
  contract.authority.contract = 'tmp/contract.json';
  contract.authority.document = 'tmp/authority.md';
  delete contract.profile;

  const result = await validateContract(contract, { repoRoot: REPO_ROOT });

  assert.match(
    result.errors.join('\n'),
    /canonical_profile_(?:mismatch|structure_mismatch)/,
  );
});

test('canonical profile cannot self-close production branch facts', async () => {
  const contract = await loadCanonicalContract();
  contract.nodes = contract.nodes.filter(
    node => node.id !== 'integration.scheduled.start_in_thread',
  );
  contract.edges = contract.edges.filter(
    edge =>
      ![
        'edge.scheduled_execute_routes_existing',
        'edge.scheduled_existing_calls_create_run',
      ].includes(edge.id),
  );
  const scheduled = contract.chains.find(
    chain => chain.id === 'entry.scheduled_task',
  );
  scheduled.required_side_edge_ids = [];

  const result = await validateContract(contract, { repoRoot: REPO_ROOT });
  const errors = result.errors.join('\n');

  assert.match(
    errors,
    /profile_node_missing: integration\.scheduled\.start_in_thread/,
  );
  assert.match(
    errors,
    /profile_edge_missing: edge\.scheduled_existing_calls_create_run/,
  );
});

test('canonical profile fixes legacy exclusion direction to the new-run policy', async () => {
  const contract = await loadCanonicalContract();
  const edge = contract.edges.find(
    candidate => candidate.id === 'edge.legacy_excluded_from_new_runs',
  );
  edge.from = 'historical.legacy_runtime';
  edge.to = 'runtime.adk_executor.execute';

  const result = await validateContract(contract, { repoRoot: REPO_ROOT });

  assert.match(
    result.errors.join('\n'),
    /profile_edge_shape_mismatch: edge\.legacy_excluded_from_new_runs/,
  );
});

test('canonical profile cannot weaken runtime or forbidden-term boundaries', async () => {
  const contract = await loadCanonicalContract();
  contract.profile = 'generic';
  contract.scope.canonical_runtime = 'legacy';
  contract.scope.forbidden_current_terms = [];

  const result = await validateContract(contract, { repoRoot: REPO_ROOT });

  assert.match(result.errors.join('\n'), /canonical_profile_mismatch/);
  assert.match(
    result.errors.join('\n'),
    /profile_scope_mismatch: canonical_runtime/,
  );
  assert.match(result.errors.join('\n'), /profile_forbidden_term_missing: k2/);
  assert.match(
    result.errors.join('\n'),
    /profile_forbidden_term_missing: chattask/,
  );
});

test('rejects duplicate query IDs and invalid chain references', async () => {
  const contract = await loadCanonicalContract();
  contract.required_queries.push(structuredClone(contract.required_queries[0]));
  contract.edges[0].chain_ids.push('chain.missing');
  const sideEdgeID = contract.chains.find(
    chain => chain.id === 'framework.eino_sdk',
  ).required_side_edge_ids[0];
  contract.edges.find(edge => edge.id === sideEdgeID).chain_ids = [];

  const result = await validateContract(contract, { repoRoot: REPO_ROOT });
  const errors = result.errors.join('\n');

  assert.match(errors, /duplicate_required_query_id/);
  assert.match(errors, /edge_chain_missing: .*chain\.missing/);
  assert.match(errors, /chain_side_edge_membership_missing/);
});

test('requires an exact expanded term for every Graphify smoke anchor', async () => {
  const contract = await loadCanonicalContract();
  contract.required_queries[0].expanded_terms = ['framework'];

  const result = await validateContract(contract, { repoRoot: REPO_ROOT });

  assert.match(result.errors.join('\n'), /query_smoke_term_missing/);
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
  chain.ordered_node_ids[0] = 'compat.deerflow_config';

  const result = await validateContract(contract, { repoRoot: REPO_ROOT });
  assert.match(
    result.errors.join('\n'),
    /noncanonical_runtime_in_execution_chain/,
  );
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
  assert.deepEqual(evaluateAuthorityChanges([CONTRACT_RELATIVE], scope), [
    'authority_files_must_change_together',
  ]);
  assert.deepEqual(
    evaluateAuthorityChanges([CONTRACT_RELATIVE, CONTEXT_RELATIVE], scope),
    [],
  );
});

test('canonical monitored paths cover every referenced source and manifest', async () => {
  const contract = await loadCanonicalContract();
  contract.scope.monitored_paths = contract.scope.monitored_paths.filter(
    pattern => pattern !== 'backend/application/application.go',
  );

  const result = await validateContract(contract, { repoRoot: REPO_ROOT });

  assert.match(
    result.errors.join('\n'),
    /contract_source_unmonitored: backend\/application\/application\.go/,
  );
  assert.deepEqual(
    evaluateAuthorityChanges(
      ['backend/application/application.go'],
      contract.scope,
    ),
    [],
  );
});

test('canonical nodes all have an AST-file bridge candidate', async () => {
  const contract = await loadCanonicalContract();
  const missing = contract.nodes
    .filter(
      node =>
        astEligibleCorpusFiles(
          node.source_anchors.map(anchor => `source/${anchor.path}`),
        ).length === 0,
    )
    .map(node => node.id);

  assert.deepEqual(missing, []);
});

test('canonical transport, retired routes, and integration ingress are explicit', async () => {
  const contract = await loadCanonicalContract();
  const anchorPaths = new Set(
    contract.nodes.flatMap(node =>
      node.source_anchors.map(anchor => anchor.path),
    ),
  );
  for (const expected of [
    'frontend/apps/coze-studio/src/pages/workbench/thread-client/canonical-thread-client-singleton.ts',
    'frontend/apps/coze-studio/src/pages/workbench/thread-client/canonical-thread-client.ts',
    'frontend/packages/arch/api-schema/src/idl/workbench/thread.ts',
    'idl/workbench/thread.thrift',
    'idl/workbench/thread_product.thrift',
    'backend/api/router/coze/api.go',
    'backend/api/router/coze/workbench_canonical_thread_route_test.go',
  ]) {
    assert.equal(anchorPaths.has(expected), true, expected);
  }

  const scheduledContract = contract.nodes.find(
    item => item.id === 'contract.workbench_scheduled_task.thrift',
  );
  assert.ok(scheduledContract);
  assert.deepEqual(
    new Set(scheduledContract.source_anchors.map(anchor => anchor.path)),
    new Set([
      'idl/workbench/task.thrift',
      'frontend/packages/arch/api-schema/src/idl/workbench/task.ts',
      'backend/api/model/workbench/task/task.go',
    ]),
  );

  const routeSurface = contract.nodes.find(
    item => item.id === 'contract.workbench.route_surface',
  );
  assert.equal(
    routeSurface?.label,
    'Workbench route surface: 47 canonical, 11 scheduled; 36/23/10 retired',
  );
  assert.equal(
    contract.nodes.filter(item => item.id === 'frontend.client.singleton')
      .length,
    1,
  );

  for (const retiredNodeID of [
    'compat.langgraph.create_run',
    'compat.langgraph.stateless_run',
    'compat.langgraph.stateless_backing_thread',
    'contract.workbench_task.thrift',
    'frontend.events.event_source',
    'framework.browser_eventsource',
    'http.workbench.create_thread',
    'http.workbench.create_run',
    'http.workbench.stream_events',
    'http.workbench.cancel_run',
    'http.workbench.resume_run',
    'http.workbench.retry_subagent',
  ]) {
    assert.equal(
      contract.nodes.some(item => item.id === retiredNodeID),
      false,
      retiredNodeID,
    );
  }
  assert.equal(
    contract.chains.some(item => item.id === 'entry.langgraph_compat'),
    false,
  );
  assert.equal(
    contract.chains.some(item => item.id === 'entry.langgraph_stateless'),
    false,
  );
  assert.equal(
    contract.edges.some(
      item => item.id === 'edge.stateless_route_surface_maps_adapter',
    ),
    false,
  );
  assert.equal(
    JSON.stringify(contract).includes('Feature-gated canonical'),
    false,
  );
  assert.equal(
    contract.exclusions.some(
      item => item.id === 'exclude.taskthread_v1_routes',
    ),
    true,
  );
  assert.equal(
    contract.exclusions.some(
      item => item.id === 'exclude.langgraph_thread_routes',
    ),
    true,
  );

  const query = contract.required_queries.find(
    item => item.id === 'query.integration_ingress',
  );
  assert.ok(query);
  for (const edgeID of [
    'edge.scheduled_execute_routes_new',
    'edge.scheduled_execute_routes_existing',
    'edge.feishu_start_calls_create_thread',
    'edge.feishu_start_calls_create_run',
  ]) {
    assert.equal(query.required_edge_ids.includes(edgeID), true, edgeID);
  }
});

test('authority change detection includes deleted monitored files', async () => {
  const repoRoot = await mkdtemp(path.join(os.tmpdir(), 'workbench-git-diff-'));
  try {
    await mkdir(path.join(repoRoot, 'src'), { recursive: true });
    await writeFile(path.join(repoRoot, 'src/runtime.go'), 'package runtime\n');
    await execFileAsync('git', ['init'], { cwd: repoRoot });
    await execFileAsync('git', ['add', 'src/runtime.go'], { cwd: repoRoot });
    await execFileAsync(
      'git',
      [
        '-c',
        'user.name=Workbench Graph Test',
        '-c',
        'user.email=workbench-graph-test@example.invalid',
        'commit',
        '-m',
        'initial',
      ],
      { cwd: repoRoot },
    );
    await rm(path.join(repoRoot, 'src/runtime.go'));

    const changedPaths = await gitChangedPaths(repoRoot, 'HEAD');

    assert.deepEqual(changedPaths, ['src/runtime.go']);
  } finally {
    await rm(repoRoot, { recursive: true, force: true });
  }
});

test('authority change detection includes untracked monitored files', async () => {
  const repoRoot = await mkdtemp(path.join(os.tmpdir(), 'workbench-git-diff-'));
  try {
    await execFileAsync('git', ['init'], { cwd: repoRoot });
    await mkdir(path.join(repoRoot, 'src'), { recursive: true });
    await writeFile(
      path.join(repoRoot, 'src/new_runtime.go'),
      'package runtime\n',
    );

    assert.deepEqual(await gitChangedPaths(repoRoot), ['src/new_runtime.go']);
  } finally {
    await rm(repoRoot, { recursive: true, force: true });
  }
});

test(
  'authority change detection preserves newline and backslash filenames',
  { skip: process.platform === 'win32' },
  async () => {
    const repoRoot = await mkdtemp(
      path.join(os.tmpdir(), 'workbench-git-diff-'),
    );
    const relativePath = 'src/runtime\nvariant\\handler.go';
    try {
      await execFileAsync('git', ['init'], { cwd: repoRoot });
      await mkdir(path.join(repoRoot, 'src'), { recursive: true });
      await writeFile(path.join(repoRoot, relativePath), 'package runtime\n');

      const changedPaths = await gitChangedPaths(repoRoot);
      assert.deepEqual(changedPaths, [relativePath]);
      assert.deepEqual(
        evaluateAuthorityChanges(changedPaths, {
          authority_files: [CONTRACT_RELATIVE, CONTEXT_RELATIVE],
          monitored_paths: ['src/**'],
        }),
        ['authority_files_not_updated'],
      );
    } finally {
      await rm(repoRoot, { recursive: true, force: true });
    }
  },
);

test(
  'authority change detection includes tracked file type changes',
  { skip: process.platform === 'win32' },
  async () => {
    const repoRoot = await mkdtemp(
      path.join(os.tmpdir(), 'workbench-git-diff-'),
    );
    const relativePath = 'src/runtime.go';
    const runtimePath = path.join(repoRoot, relativePath);
    const scope = {
      authority_files: [CONTRACT_RELATIVE, CONTEXT_RELATIVE],
      monitored_paths: ['src/**'],
    };
    const commit = message =>
      execFileAsync(
        'git',
        [
          '-c',
          'user.name=Workbench Graph Test',
          '-c',
          'user.email=workbench-graph-test@example.invalid',
          'commit',
          '-m',
          message,
        ],
        { cwd: repoRoot },
      );
    const assertAuthorityGate = async baseRef => {
      const changedPaths = await gitChangedPaths(repoRoot, baseRef);
      assert.deepEqual(changedPaths, [relativePath]);
      assert.deepEqual(evaluateAuthorityChanges(changedPaths, scope), [
        'authority_files_not_updated',
      ]);
    };

    try {
      await execFileAsync('git', ['init'], { cwd: repoRoot });
      await mkdir(path.dirname(runtimePath), { recursive: true });
      await writeFile(runtimePath, 'package runtime\n');
      await execFileAsync('git', ['add', relativePath], { cwd: repoRoot });
      await commit('initial regular file');

      await rm(runtimePath);
      await symlink('missing-runtime-target.go', runtimePath);
      await assertAuthorityGate();
      await execFileAsync('git', ['add', relativePath], { cwd: repoRoot });
      await assertAuthorityGate();
      await commit('replace regular file with symlink');
      await assertAuthorityGate('HEAD~1');

      await rm(runtimePath);
      await writeFile(runtimePath, 'package runtime\n');
      await assertAuthorityGate();
      await execFileAsync('git', ['add', relativePath], { cwd: repoRoot });
      await assertAuthorityGate();
      await commit('replace symlink with regular file');
      await assertAuthorityGate('HEAD~1');
    } finally {
      await rm(repoRoot, { recursive: true, force: true });
    }
  },
);

test('authority change detection includes both sides of a monitored rename', async () => {
  const repoRoot = await mkdtemp(path.join(os.tmpdir(), 'workbench-git-diff-'));
  try {
    await execFileAsync('git', ['init'], { cwd: repoRoot });
    await mkdir(path.join(repoRoot, 'src'), { recursive: true });
    await writeFile(path.join(repoRoot, 'src/runtime.go'), 'package runtime\n');
    await execFileAsync('git', ['add', 'src/runtime.go'], { cwd: repoRoot });
    await execFileAsync(
      'git',
      [
        '-c',
        'user.name=Workbench Graph Test',
        '-c',
        'user.email=workbench-graph-test@example.invalid',
        'commit',
        '-m',
        'initial',
      ],
      { cwd: repoRoot },
    );
    await mkdir(path.join(repoRoot, 'archive'));
    await execFileAsync('git', ['mv', 'src/runtime.go', 'archive/runtime.go'], {
      cwd: repoRoot,
    });

    assert.deepEqual(await gitChangedPaths(repoRoot, 'HEAD'), [
      'archive/runtime.go',
      'src/runtime.go',
    ]);
  } finally {
    await rm(repoRoot, { recursive: true, force: true });
  }
});

test('renders a deterministic corpus with repository-relative source paths', async () => {
  const contract = await loadCanonicalContract();
  const validation = await validateContract(contract, { repoRoot: REPO_ROOT });
  const outputRoot = await mkdtemp(path.join(os.tmpdir(), 'workbench-corpus-'));

  try {
    const first = await writeCorpus(contract, {
      repoRoot: REPO_ROOT,
      derivedRoot: path.join(outputRoot, 'first'),
      authorityDocument: CONTEXT_RELATIVE,
      resolvedVersions: validation.resolvedVersions,
    });
    const second = await writeCorpus(contract, {
      repoRoot: REPO_ROOT,
      derivedRoot: path.join(outputRoot, 'second'),
      authorityDocument: CONTEXT_RELATIVE,
      resolvedVersions: validation.resolvedVersions,
    });

    assert.equal(first.digest, second.digest);
    assert.equal(
      await readFile(
        path.join(
          first.corpusDir,
          'source/frontend/apps/coze-studio/src/pages/workbench/index.tsx',
        ),
        'utf8',
      ),
      await readFile(
        path.join(
          REPO_ROOT,
          'frontend/apps/coze-studio/src/pages/workbench/index.tsx',
        ),
        'utf8',
      ),
    );
    assert.equal(
      renderContractLedger(contract, validation.resolvedVersions),
      renderContractLedger(contract, validation.resolvedVersions),
    );
  } finally {
    await rm(outputRoot, { recursive: true, force: true });
  }
});

test('corpus whitelist rejects sensitive files and escaping symlinks', async () => {
  const repoRoot = await mkdtemp(
    path.join(os.tmpdir(), 'workbench-corpus-safe-'),
  );
  const outsideRoot = await mkdtemp(
    path.join(os.tmpdir(), 'workbench-corpus-outside-'),
  );
  try {
    await writeFile(path.join(repoRoot, '.env'), 'TOKEN=secret\n', 'utf8');
    await writeFile(path.join(repoRoot, 'credentials.json'), '{}\n', 'utf8');
    await mkdir(path.join(repoRoot, 'secrets'));
    await writeFile(path.join(repoRoot, 'secrets/actual.json'), '{}\n', 'utf8');
    await symlink(
      path.join(repoRoot, 'secrets/actual.json'),
      path.join(repoRoot, 'public.json'),
    );
    await writeFile(path.join(repoRoot, 'safe.go'), 'package safe\n', 'utf8');
    await symlink(
      path.join(repoRoot, 'safe.go'),
      path.join(repoRoot, 'alias.go'),
    );
    await writeFile(path.join(outsideRoot, 'outside.go'), 'package outside\n');
    await symlink(
      path.join(outsideRoot, 'outside.go'),
      path.join(repoRoot, 'linked.go'),
    );

    await assert.rejects(
      validateCorpusSourcePath(repoRoot, '.env'),
      /sensitive_corpus_path/,
    );
    await assert.rejects(
      validateCorpusSourcePath(repoRoot, 'credentials.json'),
      /sensitive_corpus_path/,
    );
    await assert.rejects(
      validateCorpusSourcePath(repoRoot, 'public.json'),
      /sensitive_corpus_path/,
    );
    await assert.rejects(
      validateCorpusSourcePath(repoRoot, 'alias.go'),
      /corpus_path_symlink/,
    );
    await assert.rejects(
      validateCorpusSourcePath(repoRoot, 'linked.go'),
      /corpus_path_escapes_repo/,
    );
  } finally {
    await rm(repoRoot, { recursive: true, force: true });
    await rm(outsideRoot, { recursive: true, force: true });
  }
});

test('explicit contract nodes and directed edges override Graphify AST facts', async () => {
  const contract = await loadCanonicalContract();
  const astGraph = {
    directed: false,
    nodes: [
      {
        id: 'frontend.workbench.handle_send',
        label: 'stale AST label',
        type: 'ast_symbol',
      },
      { id: 'ast.source', label: 'AST Source', type: 'ast_symbol' },
      { id: 'ast.target', label: 'AST Target', type: 'ast_symbol' },
      {
        id: 'ast.workbench.symbol',
        label: 'handleSend',
        file_type: 'code',
        source_file:
          'source/frontend/apps/coze-studio/src/pages/workbench/index.tsx',
        source_location: 'L32',
        _origin: 'ast',
      },
      {
        id: 'ast.workbench.aaa-lookalike',
        label: 'index.tsx',
        source_file:
          'source/frontend/apps/coze-studio/src/pages/workbench/index.tsx',
        source_location: 'L32',
        _origin: 'ast',
      },
      {
        id: 'ast.workbench.file',
        label: 'index.tsx',
        file_type: 'code',
        source_file:
          'source/frontend/apps/coze-studio/src/pages/workbench/index.tsx',
        source_location: 'L1',
        _origin: 'ast',
      },
    ],
    links: [
      {
        source: 'frontend.workbench.handle_send',
        target: 'frontend.api.create_thread',
        relation: 'INFERRED_CALL',
        confidence: 'INFERRED',
      },
      {
        source: 'frontend.workbench.handle_send',
        target: 'missing.external.reference',
        relation: 'references',
        confidence: 'EXTRACTED',
      },
      {
        source: 'ast.source',
        target: 'ast.target',
        relation: 'calls',
        confidence: 'EXTRACTED',
      },
    ],
  };
  const graph = mergeExplicitGraph(astGraph, contract, {});

  assert.equal(graph.directed, true);
  assert.equal(
    graph.nodes.find(node => node.id === 'frontend.workbench.handle_send')
      .label,
    'Workbench Immediate Submit',
  );
  const explicit = graph.links.find(
    link =>
      link.source === 'frontend.workbench.handle_send' &&
      link.target === 'frontend.api.create_thread',
  );
  assert.equal(explicit.relation, 'calls');
  assert.equal(explicit.confidence, 'EXTRACTED');
  assert.equal(
    explicit.source_file,
    'frontend/apps/coze-studio/src/pages/workbench/index.tsx',
  );
  assert.deepEqual(explicit.chain_ids, [
    'entry.workbench_immediate',
    'entry.workbench_deferred',
    'framework.canonical_stack',
  ]);
  assert.equal(
    graph.links.some(link => link.target === 'missing.external.reference'),
    false,
  );
  const astLink = graph.links.find(
    link => link.source === 'ast.source' && link.target === 'ast.target',
  );
  assert.match(astLink.id, /^ast\.[a-f0-9]{24}$/);
  assert.equal(
    mergeExplicitGraph(
      {
        nodes: [
          { id: 'ast.source', label: 'AST Source', type: 'ast_symbol' },
          { id: 'ast.target', label: 'AST Target', type: 'ast_symbol' },
        ],
        links: [
          {
            source: 'ast.source',
            target: 'ast.target',
            relation: 'calls',
          },
        ],
      },
      contract,
      {},
    ).links.find(
      link => link.source === 'ast.source' && link.target === 'ast.target',
    ).id,
    astLink.id,
  );
  assert.ok(
    graph.links.some(
      link =>
        link.source === 'frontend.workbench.handle_send' &&
        link.target === 'ast.workbench.file' &&
        link.relation === 'anchored_in' &&
        link._origin === 'bridge',
    ),
  );
  assert.equal(
    graph.links.some(
      link =>
        link.source === 'frontend.workbench.handle_send' &&
        link.target === 'ast.workbench.symbol' &&
        link.relation === 'anchored_in',
    ),
    false,
  );
  assert.equal(
    graph.links.some(
      link =>
        link.source === 'frontend.workbench.handle_send' &&
        link.target === 'ast.workbench.aaa-lookalike' &&
        link.relation === 'anchored_in',
    ),
    false,
  );
  assert.equal(
    graph.nodes.some(node => node._origin === 'query_overlay'),
    false,
  );
  assert.equal(
    graph.links.some(link => link._origin === 'query_overlay'),
    false,
  );

  const queryGraph = mergeExplicitGraph(
    astGraph,
    contract,
    {},
    {
      includeQueryOverlay: true,
    },
  );
  const frameworkQuery = contract.required_queries.find(
    query => query.id === 'query.framework_inventory',
  );
  assert.equal(
    queryGraph.nodes.find(node => node.id === frameworkQuery.id).label,
    frameworkQuery.question,
  );
  assert.ok(
    queryGraph.links.some(
      link =>
        link.source === frameworkQuery.id &&
        link.target === 'framework.react' &&
        link.relation === 'retrieves' &&
        link._origin === 'query_overlay' &&
        link.source_file === CONTRACT_RELATIVE,
    ),
  );
});

test('missing Graphify leaves the previous valid derived graph untouched', async () => {
  const contract = await loadCanonicalContract();
  const validation = await validateContract(contract, { repoRoot: REPO_ROOT });
  const outputRoot = await mkdtemp(
    path.join(os.tmpdir(), 'workbench-derived-'),
  );
  const derivedRoot = path.join(outputRoot, 'derived');
  const markerPath = path.join(derivedRoot, 'previous-valid.txt');

  try {
    await mkdir(derivedRoot, { recursive: true });
    await writeFile(markerPath, 'keep-me\n', 'utf8');
    await assert.rejects(
      buildDerivedGraph(contract, {
        repoRoot: REPO_ROOT,
        derivedRoot,
        resolvedVersions: validation.resolvedVersions,
        graphifyBinary: path.join(outputRoot, 'missing-graphify'),
      }),
      error => error?.code === 'graphify_unavailable',
    );
    assert.equal(await readFile(markerPath, 'utf8'), 'keep-me\n');
  } finally {
    await rm(outputRoot, { recursive: true, force: true });
  }
});

test('failed final graph verification leaves the previous build untouched', async () => {
  const contract = await loadCanonicalContract();
  const validation = await validateContract(contract, { repoRoot: REPO_ROOT });
  const outputRoot = await mkdtemp(
    path.join(os.tmpdir(), 'workbench-derived-'),
  );
  const derivedRoot = path.join(outputRoot, 'derived');
  const markerPath = path.join(derivedRoot, 'previous-valid.txt');

  try {
    await mkdir(derivedRoot, { recursive: true });
    await writeFile(markerPath, 'keep-me\n', 'utf8');
    await assert.rejects(
      buildDerivedGraph(contract, {
        repoRoot: REPO_ROOT,
        derivedRoot,
        resolvedVersions: validation.resolvedVersions,
        graphifyBinary: path.join(outputRoot, 'unused-graphify'),
        graphifyExtractor: async ({ outputDir, corpusFiles }) => {
          const astFiles = astEligibleCorpusFiles(corpusFiles);
          const nodes = astFiles.map((sourceFile, index) => ({
            id: `atomic.ast.file.${index}`,
            label: path.posix.basename(sourceFile),
            file_type: 'code',
            source_file: sourceFile,
            source_location: 'L1',
            _origin: 'ast',
          }));
          nodes.push(structuredClone(nodes[0]));
          const links = nodes.slice(1, astFiles.length).map((node, index) => ({
            source: nodes[index].id,
            target: node.id,
            relation: 'references',
            source_file: node.source_file,
            _origin: 'ast',
          }));
          await mkdir(path.join(outputDir, 'graphify-out'), {
            recursive: true,
          });
          await writeFile(
            path.join(outputDir, 'graphify-out/graph.json'),
            `${JSON.stringify({ directed: true, nodes, links })}\n`,
            'utf8',
          );
        },
        queryRunner: async () => '',
      }),
      error => error?.code === 'derived_build_verification_failed',
    );
    assert.equal(await readFile(markerPath, 'utf8'), 'keep-me\n');
  } finally {
    await rm(outputRoot, { recursive: true, force: true });
  }
});

test('successful builds publish through one atomic version pointer', async () => {
  const contract = await loadCanonicalContract();
  const validation = await validateContract(contract, { repoRoot: REPO_ROOT });
  assert.deepEqual(validation.errors, []);
  const outputRoot = await mkdtemp(
    path.join(os.tmpdir(), 'workbench-derived-'),
  );
  const derivedRoot = path.join(outputRoot, 'derived');
  const markerPath = path.join(derivedRoot, 'legacy-valid.txt');
  const graphifyExtractor = async ({ outputDir, corpusFiles }) => {
    const astFiles = astEligibleCorpusFiles(corpusFiles);
    const nodes = astFiles.map((sourceFile, index) => ({
      id: `atomic.ast.file.${index}`,
      label: path.posix.basename(sourceFile),
      source_file: sourceFile,
      source_location: 'L1',
      _origin: 'ast',
    }));
    const links = nodes.slice(1).map((node, index) => ({
      source: nodes[index].id,
      target: node.id,
      relation: 'references',
      source_file: node.source_file,
      _origin: 'ast',
    }));
    await mkdir(path.join(outputDir, 'graphify-out'), { recursive: true });
    await writeFile(
      path.join(outputDir, 'graphify-out/graph.json'),
      `${JSON.stringify({ directed: true, nodes, links })}\n`,
      'utf8',
    );
  };

  try {
    await mkdir(derivedRoot, { recursive: true });
    await writeFile(markerPath, 'legacy\n', 'utf8');
    await writeFile(
      path.join(derivedRoot, DERIVED_MANAGED_MARKER),
      `${JSON.stringify({
        schema_version: 1,
        owner: 'workbench_execution_graph',
      })}\n`,
      'utf8',
    );
    const options = {
      repoRoot: REPO_ROOT,
      derivedRoot,
      resolvedVersions: validation.resolvedVersions,
      graphifyBinary: path.join(outputRoot, 'unused-graphify'),
      graphifyExtractor,
      runGraphifyQueries: false,
    };

    await buildDerivedGraph(contract, options);
    assert.equal((await lstat(derivedRoot)).isSymbolicLink(), true);
    await assert.doesNotReject(
      assertManagedBuildRoot(derivedRoot, { allowRootSymlink: true }),
    );
    const firstTarget = path.resolve(
      path.dirname(derivedRoot),
      await readlink(derivedRoot),
    );
    assert.match(
      await readFile(path.join(derivedRoot, 'build-meta.json'), 'utf8'),
      /"corpus_digest"/,
    );

    await buildDerivedGraph(contract, options);
    const secondTarget = path.resolve(
      path.dirname(derivedRoot),
      await readlink(derivedRoot),
    );
    assert.notEqual(secondTarget, firstTarget);
    assert.equal((await lstat(firstTarget)).isDirectory(), true);
    const previousPointer = `${derivedRoot}-previous`;
    assert.equal((await lstat(previousPointer)).isSymbolicLink(), true);
    assert.equal(
      path.resolve(
        path.dirname(previousPointer),
        await readlink(previousPointer),
      ),
      firstTarget,
    );
    assert.match(
      await readFile(path.join(derivedRoot, 'build-meta.json'), 'utf8'),
      /"query_graph_node_count"/,
    );
    const metadata = JSON.parse(
      await readFile(path.join(derivedRoot, 'build-meta.json'), 'utf8'),
    );
    assert.equal(typeof metadata.git_dirty, 'boolean');
    assert.match(metadata.git_status_digest, /^[a-f0-9]{64}$/);
  } finally {
    await rm(outputRoot, { recursive: true, force: true });
  }
});

test('generated graph pointer and version directories stay ignored', async () => {
  const { stdout } = await execFileAsync(
    'git',
    [
      'check-ignore',
      'docs/superpowers/context/workbench-execution-graphify',
      'docs/superpowers/context/workbench-execution-graphify-previous',
      'docs/superpowers/context/workbench-execution-graphify-versions/build-test',
    ],
    { cwd: REPO_ROOT },
  );

  assert.deepEqual(stdout.trim().split(/\r?\n/), [
    'docs/superpowers/context/workbench-execution-graphify',
    'docs/superpowers/context/workbench-execution-graphify-previous',
    'docs/superpowers/context/workbench-execution-graphify-versions/build-test',
  ]);
});

test('atomic publisher refuses to replace an unmanaged directory', async () => {
  const outputRoot = await mkdtemp(
    path.join(os.tmpdir(), 'workbench-publish-'),
  );
  const temporaryRoot = path.join(outputRoot, 'completed');
  const derivedRoot = path.join(outputRoot, 'important-data');
  try {
    await mkdir(temporaryRoot);
    await writeFile(
      path.join(temporaryRoot, DERIVED_MANAGED_MARKER),
      `${JSON.stringify({
        schema_version: 1,
        owner: 'workbench_execution_graph',
      })}\n`,
      'utf8',
    );
    await mkdir(derivedRoot);
    await writeFile(path.join(derivedRoot, 'keep.txt'), 'keep\n', 'utf8');

    await assert.rejects(
      installCompletedBuild(temporaryRoot, derivedRoot),
      error => error?.code === 'derived_publish_target_unmanaged',
    );
    assert.equal(
      await readFile(path.join(derivedRoot, 'keep.txt'), 'utf8'),
      'keep\n',
    );
  } finally {
    await rm(outputRoot, { recursive: true, force: true });
  }
});

test('atomic publisher rejects invalid previous before switching current', async () => {
  const outputRoot = await mkdtemp(
    path.join(os.tmpdir(), 'workbench-publish-'),
  );
  const temporaryRoot = path.join(outputRoot, 'completed');
  const derivedRoot = path.join(outputRoot, 'workbench-execution-graph-test');
  const versionsRoot = `${derivedRoot}-versions`;
  const currentVersionRoot = path.join(versionsRoot, 'build-current');
  const previousPointerRoot = `${derivedRoot}-previous`;
  const marker = `${JSON.stringify({
    schema_version: 1,
    owner: 'workbench_execution_graph',
  })}\n`;
  try {
    await mkdir(temporaryRoot);
    await writeFile(
      path.join(temporaryRoot, DERIVED_MANAGED_MARKER),
      marker,
      'utf8',
    );
    await mkdir(currentVersionRoot, { recursive: true });
    await writeFile(
      path.join(currentVersionRoot, DERIVED_MANAGED_MARKER),
      marker,
      'utf8',
    );
    await symlink(
      path.relative(outputRoot, currentVersionRoot),
      derivedRoot,
      'dir',
    );
    await mkdir(previousPointerRoot);
    const originalCurrentTarget = await readlink(derivedRoot);

    await assert.rejects(
      installCompletedBuild(temporaryRoot, derivedRoot),
      /derived_previous_pointer_invalid/,
    );
    assert.equal(await readlink(derivedRoot), originalCurrentTarget);
    assert.equal((await lstat(temporaryRoot)).isDirectory(), true);
    assert.equal((await lstat(previousPointerRoot)).isDirectory(), true);
  } finally {
    await rm(outputRoot, { recursive: true, force: true });
  }
});

test('derived verifier rejects a managed pointer outside its version root', async () => {
  const contract = await loadCanonicalContract();
  const validation = await validateContract(contract, { repoRoot: REPO_ROOT });
  const outputRoot = await mkdtemp(
    path.join(os.tmpdir(), 'workbench-pointer-'),
  );
  const derivedRoot = path.join(outputRoot, 'workbench-execution-graph-test');
  const outsideRoot = path.join(outputRoot, 'outside-managed');
  try {
    await mkdir(outsideRoot);
    await writeFile(
      path.join(outsideRoot, DERIVED_MANAGED_MARKER),
      `${JSON.stringify({
        schema_version: 1,
        owner: 'workbench_execution_graph',
      })}\n`,
      'utf8',
    );
    await symlink(outsideRoot, derivedRoot, 'dir');

    const result = await verifyDerivedGraph(contract, {
      repoRoot: REPO_ROOT,
      derivedRoot,
      resolvedVersions: validation.resolvedVersions,
      runGraphifyQueries: false,
    });
    assert.match(result.errors.join('\n'), /derived_pointer_outside_versions/);
  } finally {
    await rm(outputRoot, { recursive: true, force: true });
  }
});

test('derived verifier rejects a symlink nested inside its version root', async () => {
  const contract = await loadCanonicalContract();
  const validation = await validateContract(contract, { repoRoot: REPO_ROOT });
  const outputRoot = await mkdtemp(
    path.join(os.tmpdir(), 'workbench-pointer-'),
  );
  const derivedRoot = path.join(outputRoot, 'workbench-execution-graph-test');
  const versionsRoot = `${derivedRoot}-versions`;
  const outsideRoot = path.join(outputRoot, 'outside-managed');
  const nestedTarget = path.join(versionsRoot, 'build-proxy');
  try {
    await mkdir(versionsRoot);
    await mkdir(outsideRoot);
    await writeFile(
      path.join(outsideRoot, DERIVED_MANAGED_MARKER),
      `${JSON.stringify({
        schema_version: 1,
        owner: 'workbench_execution_graph',
      })}\n`,
      'utf8',
    );
    await symlink(outsideRoot, nestedTarget, 'dir');
    await symlink(nestedTarget, derivedRoot, 'dir');

    const result = await verifyDerivedGraph(contract, {
      repoRoot: REPO_ROOT,
      derivedRoot,
      resolvedVersions: validation.resolvedVersions,
      runGraphifyQueries: false,
    });
    assert.match(result.errors.join('\n'), /derived_pointer_target_invalid/);
  } finally {
    await rm(outputRoot, { recursive: true, force: true });
  }
});

test('Graphify extraction includes ignored whitelist corpus files', () => {
  assert.deepEqual(graphifyExtractArgs('/tmp/corpus', '/tmp/output'), [
    'extract',
    '/tmp/corpus',
    '--out',
    '/tmp/output',
    '--code-only',
    '--no-cluster',
    '--no-gitignore',
    '--max-workers',
    '1',
  ]);
  assert.throws(
    () => assertNonEmptyASTGraph({ nodes: [], links: [] }, ['source/main.go']),
    /graphify_ast_empty/,
  );
  assert.throws(
    () =>
      assertNonEmptyASTGraph(
        {
          nodes: [{ id: 'main', source_file: 'source/main.go' }],
          links: [],
        },
        ['source/main.go'],
      ),
    /graphify_ast_empty/,
  );
  assert.throws(
    () =>
      assertASTSourceCoverage(
        {
          nodes: [{ id: 'main', source_file: 'source/main.go' }],
          links: [{ source: 'main', target: 'main', relation: 'contains' }],
        },
        ['source/main.go', 'source/other.ts'],
      ),
    /graphify_ast_source_missing.*source\/other\.ts/,
  );
  assert.doesNotThrow(() =>
    assertNonEmptyASTGraph({ nodes: [], links: [] }, ['contract-ledger.md']),
  );
});

const withDerivedFixture = async callback => {
  const contract = await loadCanonicalContract();
  const validation = await validateContract(contract, { repoRoot: REPO_ROOT });
  const outputRoot = await mkdtemp(path.join(os.tmpdir(), 'workbench-verify-'));
  const derivedRoot = path.join(outputRoot, 'derived');
  try {
    const builderDigest = await computeBuilderDigest(REPO_ROOT);
    const gitProvenance = await readGitProvenance(REPO_ROOT);
    const corpus = await writeCorpus(contract, {
      repoRoot: REPO_ROOT,
      derivedRoot: path.join(outputRoot, 'digest'),
      authorityDocument: CONTEXT_RELATIVE,
      resolvedVersions: validation.resolvedVersions,
    });
    const astFiles = astEligibleCorpusFiles(corpus.files);
    const astNodes = astFiles.map((sourceFile, index) => ({
      id: `fixture.ast.file.${index}`,
      label: path.posix.basename(sourceFile),
      file_type: 'code',
      source_file: sourceFile,
      source_location: 'L1',
      _origin: 'ast',
    }));
    const astLinks = astNodes.slice(1).map((node, index) => ({
      source: astNodes[index].id,
      target: node.id,
      relation: 'references',
      confidence: 'EXTRACTED',
      source_file: node.source_file,
      _origin: 'ast',
    }));
    const graph = mergeExplicitGraph(
      { directed: true, nodes: astNodes, links: astLinks },
      contract,
      validation.resolvedVersions,
    );
    const queryGraph = mergeExplicitGraph(
      { directed: true, nodes: astNodes, links: astLinks },
      contract,
      validation.resolvedVersions,
      { includeQueryOverlay: true },
    );
    const buildStats = {
      execution_node_count: graph.nodes.length,
      execution_link_count: graph.links.length,
      query_graph_node_count: queryGraph.nodes.length,
      query_graph_link_count: queryGraph.links.length,
      ast_node_count: astNodes.length,
      ast_link_count: astLinks.length,
      ast_source_files: astFiles,
      bridge_link_count: graph.links.filter(link => link._origin === 'bridge')
        .length,
      query_overlay_node_count: queryGraph.nodes.filter(
        node => node._origin === 'query_overlay',
      ).length,
      query_overlay_link_count: queryGraph.links.filter(
        link => link._origin === 'query_overlay',
      ).length,
    };
    await mkdir(path.join(derivedRoot, 'graphify-out'), { recursive: true });
    await writeFile(
      path.join(derivedRoot, DERIVED_MANAGED_MARKER),
      `${JSON.stringify({
        schema_version: 1,
        owner: 'workbench_execution_graph',
      })}\n`,
      'utf8',
    );

    const writeDerived = async (
      nextGraph = graph,
      digest = corpus.digest,
      metadata = {},
      nextQueryGraph = queryGraph,
    ) => {
      await writeFile(
        path.join(derivedRoot, 'graphify-out/graph.json'),
        `${JSON.stringify(nextGraph, null, 2)}\n`,
        'utf8',
      );
      await writeFile(
        path.join(derivedRoot, 'graphify-out/query-graph.json'),
        `${JSON.stringify(nextQueryGraph, null, 2)}\n`,
        'utf8',
      );
      await writeFile(
        path.join(derivedRoot, 'build-meta.json'),
        `${JSON.stringify(
          {
            ...gitProvenance,
            corpus_digest: digest,
            builder_digest: builderDigest,
            corpus_files: corpus.files,
            ...buildStats,
            ...metadata,
          },
          null,
          2,
        )}\n`,
        'utf8',
      );
    };
    await writeDerived();
    await callback({
      contract,
      derivedRoot,
      graph,
      queryGraph,
      resolvedVersions: validation.resolvedVersions,
      writeDerived,
    });
  } finally {
    await rm(outputRoot, { recursive: true, force: true });
  }
};

test('derived graph verifier accepts complete explicit paths and queries', async () => {
  await withDerivedFixture(async fixture => {
    assert.deepEqual(
      verifyRequiredQueries(fixture.graph, fixture.contract.required_queries),
      [],
    );
    assert.deepEqual(
      verifyOrderedPaths(fixture.graph, fixture.contract.chains),
      [],
    );
    const result = await verifyDerivedGraph(fixture.contract, {
      repoRoot: REPO_ROOT,
      derivedRoot: fixture.derivedRoot,
      resolvedVersions: fixture.resolvedVersions,
      runGraphifyQueries: false,
    });
    assert.deepEqual(result.errors, []);
  });
});

test('derived verifier rejects a missing query graph', async () => {
  await withDerivedFixture(async fixture => {
    await rm(path.join(fixture.derivedRoot, 'graphify-out/query-graph.json'));

    const result = await verifyDerivedGraph(fixture.contract, {
      repoRoot: REPO_ROOT,
      derivedRoot: fixture.derivedRoot,
      resolvedVersions: fixture.resolvedVersions,
      runGraphifyQueries: false,
    });

    assert.match(
      result.errors.join('\n'),
      /derived_graph_unreadable:.*query-graph\.json/,
    );
  });
});

test('derived verifier rejects stale, malformed, and incomplete graphs', async () => {
  await withDerivedFixture(async fixture => {
    const verify = async () =>
      verifyDerivedGraph(fixture.contract, {
        repoRoot: REPO_ROOT,
        derivedRoot: fixture.derivedRoot,
        resolvedVersions: fixture.resolvedVersions,
        runGraphifyQueries: false,
      });

    await fixture.writeDerived(fixture.graph, 'stale');
    assert.match((await verify()).errors.join('\n'), /stale_derived_digest/);

    await fixture.writeDerived(fixture.graph, undefined, {
      builder_digest: 'stale-builder-digest',
    });
    assert.match((await verify()).errors.join('\n'), /stale_builder_digest/);

    await fixture.writeDerived(fixture.graph, undefined, {
      git_commit: 'stale-git-commit',
    });
    assert.match((await verify()).errors.join('\n'), /stale_git_commit/);

    await fixture.writeDerived(fixture.graph, undefined, {
      ast_node_count: 0,
    });
    assert.match(
      (await verify()).errors.join('\n'),
      /derived_ast_count_mismatch/,
    );

    const leakedOverlay = structuredClone(fixture.graph);
    leakedOverlay.nodes.push(
      structuredClone(
        fixture.queryGraph.nodes.find(node => node._origin === 'query_overlay'),
      ),
    );
    await fixture.writeDerived(leakedOverlay);
    assert.match(
      (await verify()).errors.join('\n'),
      /execution_graph_query_overlay_present/,
    );

    const disguisedRetrieval = structuredClone(fixture.graph);
    const disguisedQueryGraph = structuredClone(fixture.queryGraph);
    const disguisedEdge = {
      id: 'ast.disguised-retrieval',
      source: 'compat.deerflow_config',
      target: 'application.create_run',
      relation: 'retrieves',
      confidence: 'EXTRACTED',
      _origin: 'ast',
    };
    disguisedRetrieval.links.push(structuredClone(disguisedEdge));
    disguisedQueryGraph.links.push(structuredClone(disguisedEdge));
    await fixture.writeDerived(
      disguisedRetrieval,
      undefined,
      {},
      disguisedQueryGraph,
    );
    assert.match(
      (await verify()).errors.join('\n'),
      /execution_graph_retrieves_edge_present/,
    );

    const unexpectedOverlay = structuredClone(fixture.queryGraph);
    unexpectedOverlay.nodes.push({
      id: 'query.uncontracted',
      label: 'Uncontracted query intent',
      _origin: 'query_overlay',
    });
    await fixture.writeDerived(fixture.graph, undefined, {}, unexpectedOverlay);
    assert.match(
      (await verify()).errors.join('\n'),
      /query_graph_unexpected_node: query\.uncontracted/,
    );

    const missingBridge = structuredClone(fixture.graph);
    missingBridge.links = missingBridge.links.filter(
      link => link._origin !== 'bridge',
    );
    await fixture.writeDerived(missingBridge);
    assert.match(
      (await verify()).errors.join('\n'),
      /source_anchor_bridge_missing/,
    );

    const dangling = structuredClone(fixture.graph);
    dangling.links.push({
      id: 'fault.dangling',
      source: 'frontend.workbench.handle_send',
      target: 'missing.node',
      relation: 'calls',
    });
    await fixture.writeDerived(dangling);
    assert.match(
      (await verify()).errors.join('\n'),
      /derived_edge_target_missing/,
    );

    const duplicate = structuredClone(fixture.graph);
    duplicate.links.push({
      ...duplicate.links[0],
      id: 'fault.duplicate',
    });
    await fixture.writeDerived(duplicate);
    assert.match(
      (await verify()).errors.join('\n'),
      /duplicate_derived_relation/,
    );

    const duplicateID = structuredClone(fixture.graph);
    duplicateID.links.push({
      ...duplicateID.links[0],
      target: 'runtime.adk_executor.execute',
      relation: 'fault_relation',
    });
    await fixture.writeDerived(duplicateID);
    assert.match((await verify()).errors.join('\n'), /duplicate_derived_edge/);

    const selfLoop = structuredClone(fixture.graph);
    selfLoop.links.push({
      id: 'fault.self-loop',
      source: 'frontend.workbench.handle_send',
      target: 'frontend.workbench.handle_send',
      relation: 'calls',
    });
    await fixture.writeDerived(selfLoop);
    assert.match((await verify()).errors.join('\n'), /derived_self_loop/);

    const missingNode = structuredClone(fixture.graph);
    missingNode.nodes = missingNode.nodes.filter(
      node => node.id !== 'runtime.adk_executor.execute',
    );
    await fixture.writeDerived(missingNode);
    assert.match(
      (await verify()).errors.join('\n'),
      /required_query_node_missing/,
    );

    const brokenPath = structuredClone(fixture.graph);
    brokenPath.links = brokenPath.links.filter(
      link => link.id !== 'edge.selector_executes_adk',
    );
    await fixture.writeDerived(brokenPath);
    assert.match(
      (await verify()).errors.join('\n'),
      /ordered_path_edge_missing/,
    );

    const forbidden = structuredClone(fixture.graph);
    forbidden.nodes.push({
      id: 'fault.k2',
      label: 'K2 current runtime',
      production_status: 'current',
    });
    await fixture.writeDerived(forbidden);
    assert.match((await verify()).errors.join('\n'), /forbidden_derived_node/);
  });
});

test('derived verifier rejects tampered canonical node and edge metadata', async () => {
  await withDerivedFixture(async fixture => {
    const verify = async () =>
      verifyDerivedGraph(fixture.contract, {
        repoRoot: REPO_ROOT,
        derivedRoot: fixture.derivedRoot,
        resolvedVersions: fixture.resolvedVersions,
        runGraphifyQueries: false,
      });

    const tamperedNode = structuredClone(fixture.graph);
    const node = tamperedNode.nodes.find(
      candidate => candidate.id === 'runtime.adk_executor.execute',
    );
    node.label = 'Unverified Runtime Executor';
    node.authority = 'ast_inference';
    await fixture.writeDerived(tamperedNode);
    assert.match((await verify()).errors.join('\n'), /contract_node_mismatch/);

    const tamperedEdge = structuredClone(fixture.graph);
    const edge = tamperedEdge.links.find(
      candidate => candidate.id === 'edge.selector_executes_adk',
    );
    edge.authority = 'ast_inference';
    edge.chain_ids = [];
    await fixture.writeDerived(tamperedEdge);
    assert.match((await verify()).errors.join('\n'), /contract_edge_mismatch/);
  });
});

test('derived verifier reports Graphify query smoke failures', async () => {
  await withDerivedFixture(async fixture => {
    const result = await verifyDerivedGraph(fixture.contract, {
      repoRoot: REPO_ROOT,
      derivedRoot: fixture.derivedRoot,
      resolvedVersions: fixture.resolvedVersions,
      queryRunner: async () => '',
    });
    assert.match(
      result.errors.join('\n'),
      /graphify_query_(?:intent|smoke)_failed/,
    );
  });
});

test('Graphify smoke rejects labels echoed outside NODE result lines', async () => {
  await withDerivedFixture(async fixture => {
    const result = await verifyDerivedGraph(fixture.contract, {
      repoRoot: REPO_ROOT,
      derivedRoot: fixture.derivedRoot,
      resolvedVersions: fixture.resolvedVersions,
      queryRunner: async ({ terms }) => `Traversal start: ['${terms}']`,
    });

    assert.match(
      result.errors.join('\n'),
      /graphify_query_(?:intent|smoke)_failed/,
    );
  });
});

test('Graphify business intent rejects node-only results without required edges', async () => {
  await withDerivedFixture(async fixture => {
    const result = await verifyDerivedGraph(fixture.contract, {
      repoRoot: REPO_ROOT,
      derivedRoot: fixture.derivedRoot,
      resolvedVersions: fixture.resolvedVersions,
      queryRunner: async ({ requiredLabels }) =>
        requiredLabels.map(label => `NODE ${label} [src=contract]`).join('\n'),
    });

    assert.match(
      result.errors.join('\n'),
      /graphify_query_intent_failed: .*missing edge/,
    );
  });
});

test('Graphify retrieves each business intent and exact anchor independently', async () => {
  await withDerivedFixture(async fixture => {
    const labelsByID = new Map(
      fixture.graph.nodes.map(node => [node.id, node.label]),
    );
    const expectedAnchorTerms = fixture.contract.required_queries.flatMap(
      query => query.smoke_node_ids.map(nodeID => labelsByID.get(nodeID)),
    );
    const expectedIntentTerms = fixture.contract.required_queries.map(
      query => query.question,
    );
    const actualIntentTerms = [];
    const actualAnchorTerms = [];

    const result = await verifyDerivedGraph(fixture.contract, {
      repoRoot: REPO_ROOT,
      derivedRoot: fixture.derivedRoot,
      resolvedVersions: fixture.resolvedVersions,
      queryRunner: async ({
        mode,
        requiredEdges = [],
        requiredLabels,
        terms,
      }) => {
        if (mode === 'intent') {
          actualIntentTerms.push(terms);
        } else if (mode === 'anchor') {
          actualAnchorTerms.push(terms);
        }
        return [
          ...requiredLabels.map(label => `NODE ${label} [src=contract]`),
          ...requiredEdges.map(edge => {
            const relation =
              mode === 'edge'
                ? `graphify_collapsed/${edge.relation}`
                : edge.relation;
            return `EDGE ${edge.sourceLabel} --${relation} [EXTRACTED]--> ${edge.targetLabel}`;
          }),
        ].join('\n');
      },
    });

    assert.deepEqual(result.errors, []);
    assert.deepEqual(actualIntentTerms, expectedIntentTerms);
    assert.deepEqual(actualAnchorTerms, expectedAnchorTerms);
  });
});

test('Graphify routes intent and edge checks to their assigned graphs', async () => {
  await withDerivedFixture(async fixture => {
    const pathsByMode = new Map();
    const runner = async ({
      graphPath,
      mode,
      requiredEdges = [],
      requiredLabels,
    }) => {
      pathsByMode.set(mode, graphPath);
      return [
        ...requiredLabels.map(label => `NODE ${label} [src=contract]`),
        ...requiredEdges.map(
          edge =>
            `EDGE ${edge.sourceLabel} --${edge.relation} [EXTRACTED]--> ${edge.targetLabel}`,
        ),
      ].join('\n');
    };

    const result = await verifyDerivedGraph(fixture.contract, {
      repoRoot: REPO_ROOT,
      derivedRoot: fixture.derivedRoot,
      resolvedVersions: fixture.resolvedVersions,
      queryRunner: runner,
      pathRunner: runner,
    });

    assert.deepEqual(result.errors, []);
    assert.equal(path.basename(pathsByMode.get('intent')), 'query-graph.json');
    assert.equal(path.basename(pathsByMode.get('edge')), 'graph.json');
    assert.equal(path.basename(pathsByMode.get('anchor')), 'graph.json');
  });
});

test('CLI verify succeeds and unknown commands fail concisely', async () => {
  const verified = await execFileAsync(process.execPath, [CLI_PATH, 'verify'], {
    cwd: os.tmpdir(),
    encoding: 'utf8',
  });
  assert.match(verified.stdout, /verification passed/);

  await assert.rejects(
    execFileAsync(
      process.execPath,
      [CLI_PATH, 'verify-derived', '--derived-root', os.tmpdir()],
      { cwd: os.tmpdir(), encoding: 'utf8' },
    ),
    error =>
      error?.code === 1 &&
      /--derived-root must stay inside --repo-root/.test(error?.stderr),
  );

  await assert.rejects(
    execFileAsync(process.execPath, [CLI_PATH, 'unknown'], {
      cwd: os.tmpdir(),
      encoding: 'utf8',
    }),
    error =>
      error?.code === 1 &&
      /unknown command/.test(error?.stderr) &&
      !/at file:/.test(error?.stderr),
  );
});

test('documentation references authority files and only supported CLI commands', async () => {
  const documents = await Promise.all(
    ['AGENTS.md', RUNBOOK_RELATIVE].map(relativePath =>
      readFile(path.join(REPO_ROOT, relativePath), 'utf8'),
    ),
  );
  for (const content of documents) {
    assert.match(content, new RegExp(CONTRACT_RELATIVE.replaceAll('/', '\\/')));
    assert.match(content, new RegExp(CONTEXT_RELATIVE.replaceAll('/', '\\/')));
    const commandLines = content
      .split(/\r?\n/)
      .map(line => line.trim())
      .filter(line =>
        line.startsWith('node scripts/workbench-execution-graph.mjs '),
      );
    assert.ok(commandLines.length > 0);
    for (const commandLine of commandLines) {
      const command = commandLine.split(/\s+/)[2];
      assert.match(command, /^(verify|build|verify-derived)$/);
      assert.match(
        CLI_HELP,
        new RegExp(`workbench-execution-graph\\.mjs ${command}(?: |\\n)`),
      );
    }
  }
});
