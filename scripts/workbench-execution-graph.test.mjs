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
import { mkdtemp, mkdir, readFile, rm, writeFile } from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';
import { promisify } from 'node:util';

import { CLI_HELP } from './workbench-execution-graph.mjs';
import {
  evaluateAuthorityChanges,
  validateContract,
} from './workbench-execution-graph/contract.mjs';
import {
  buildDerivedGraph,
  mergeExplicitGraph,
  renderContractLedger,
  verifyDerivedGraph,
  verifyOrderedPaths,
  verifyRequiredQueries,
  writeCorpus,
} from './workbench-execution-graph/derived.mjs';

const execFileAsync = promisify(execFile);
const REPO_ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
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

test('explicit contract nodes and directed edges override Graphify AST facts', async () => {
  const contract = await loadCanonicalContract();
  const graph = mergeExplicitGraph(
    {
      directed: false,
      nodes: [
        {
          id: 'frontend.workbench.handle_send',
          label: 'stale AST label',
          type: 'ast_symbol',
        },
      ],
      links: [
        {
          source: 'frontend.workbench.handle_send',
          target: 'frontend.api.create_thread',
          relation: 'INFERRED_CALL',
          confidence: 'INFERRED',
        },
      ],
    },
    contract,
    {},
  );

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
  assert.deepEqual(explicit.chain_ids, [
    'entry.workbench_immediate',
    'entry.workbench_deferred',
    'framework.canonical_stack',
  ]);
});

test('missing Graphify leaves the previous valid derived graph untouched', async () => {
  const contract = await loadCanonicalContract();
  const validation = await validateContract(contract, { repoRoot: REPO_ROOT });
  const outputRoot = await mkdtemp(path.join(os.tmpdir(), 'workbench-derived-'));
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

const withDerivedFixture = async callback => {
  const contract = await loadCanonicalContract();
  const validation = await validateContract(contract, { repoRoot: REPO_ROOT });
  const outputRoot = await mkdtemp(path.join(os.tmpdir(), 'workbench-verify-'));
  const derivedRoot = path.join(outputRoot, 'derived');
  try {
    const corpus = await writeCorpus(contract, {
      repoRoot: REPO_ROOT,
      derivedRoot: path.join(outputRoot, 'digest'),
      authorityDocument: CONTEXT_RELATIVE,
      resolvedVersions: validation.resolvedVersions,
    });
    const graph = mergeExplicitGraph(
      { directed: true, nodes: [], links: [] },
      contract,
      validation.resolvedVersions,
    );
    await mkdir(path.join(derivedRoot, 'graphify-out'), { recursive: true });

    const writeDerived = async (nextGraph = graph, digest = corpus.digest) => {
      await writeFile(
        path.join(derivedRoot, 'graphify-out/graph.json'),
        `${JSON.stringify(nextGraph, null, 2)}\n`,
        'utf8',
      );
      await writeFile(
        path.join(derivedRoot, 'build-meta.json'),
        `${JSON.stringify({ corpus_digest: digest }, null, 2)}\n`,
        'utf8',
      );
    };
    await writeDerived();
    await callback({
      contract,
      derivedRoot,
      graph,
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

    const dangling = structuredClone(fixture.graph);
    dangling.links.push({
      id: 'fault.dangling',
      source: 'frontend.workbench.handle_send',
      target: 'missing.node',
      relation: 'calls',
    });
    await fixture.writeDerived(dangling);
    assert.match((await verify()).errors.join('\n'), /derived_edge_target_missing/);

    const duplicate = structuredClone(fixture.graph);
    duplicate.links.push({
      ...duplicate.links[0],
      id: 'fault.duplicate',
    });
    await fixture.writeDerived(duplicate);
    assert.match((await verify()).errors.join('\n'), /duplicate_derived_relation/);

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
    assert.match((await verify()).errors.join('\n'), /required_query_node_missing/);

    const brokenPath = structuredClone(fixture.graph);
    brokenPath.links = brokenPath.links.filter(
      link => link.id !== 'edge.selector_executes_adk',
    );
    await fixture.writeDerived(brokenPath);
    assert.match((await verify()).errors.join('\n'), /ordered_path_edge_missing/);

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

test('derived verifier reports Graphify query smoke failures', async () => {
  await withDerivedFixture(async fixture => {
    const result = await verifyDerivedGraph(fixture.contract, {
      repoRoot: REPO_ROOT,
      derivedRoot: fixture.derivedRoot,
      resolvedVersions: fixture.resolvedVersions,
      queryRunner: async () => '',
    });
    assert.match(result.errors.join('\n'), /graphify_query_smoke_failed/);
  });
});

test('CLI verify succeeds and unknown commands fail concisely', async () => {
  const verified = await execFileAsync(process.execPath, [CLI_PATH, 'verify'], {
    cwd: os.tmpdir(),
    encoding: 'utf8',
  });
  assert.match(verified.stdout, /verification passed/);

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
