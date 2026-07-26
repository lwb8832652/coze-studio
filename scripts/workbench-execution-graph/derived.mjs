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

import { execFile } from 'node:child_process';
import { createHash } from 'node:crypto';
import {
  copyFile,
  mkdir,
  mkdtemp,
  readFile,
  rename,
  rm,
  writeFile,
} from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import { promisify } from 'node:util';

import { normalizeRepoPath } from './contract.mjs';

const execFileAsync = promisify(execFile);

export const DERIVED_RELATIVE_ROOT =
  'docs/superpowers/context/workbench-execution-graphify';

const asArray = value => (Array.isArray(value) ? value : []);
const stringValue = value =>
  typeof value === 'string' ? value.trim() : '';
const byID = (left, right) =>
  stringValue(left?.id).localeCompare(stringValue(right?.id));

const endpointID = endpoint =>
  typeof endpoint === 'object' && endpoint !== null
    ? stringValue(endpoint.id)
    : stringValue(endpoint);

const edgeKey = edge =>
  `${endpointID(edge?.source)}\u0000${endpointID(edge?.target)}`;

const linkSortKey = link =>
  [
    endpointID(link?.source),
    endpointID(link?.target),
    stringValue(link?.relation),
    stringValue(link?.id),
  ].join('\u0000');

const stableUnique = values => [...new Set(values.filter(Boolean))].sort();

export const renderContractLedger = (contract, resolvedVersions = {}) => {
  const lines = [
    '# Workbench Execution Graph Contract Ledger',
    '',
    'This file is generated deterministically from the committed machine contract.',
    'Explicit relationships are authoritative; AST relationships are supplemental.',
    '',
    '## Nodes',
    '',
  ];

  for (const node of [...asArray(contract?.nodes)].sort(byID)) {
    lines.push(`### ${node.id}`);
    lines.push(`- label: ${node.label}`);
    lines.push(`- layer: ${node.layer}`);
    lines.push(`- kind: ${node.kind}`);
    lines.push(`- production_status: ${node.production_status}`);
    if (node.runtime_scope) {
      lines.push(`- runtime_scope: ${node.runtime_scope}`);
    }
    if (node.package) {
      lines.push(`- package: ${node.package}`);
      lines.push(
        `- version: ${resolvedVersions[node.id] ?? node.version ?? 'unknown'}`,
      );
    }
    for (const anchor of asArray(node.source_anchors)) {
      lines.push(
        `- source: ${anchor.path} :: ${anchor.symbol ?? anchor.locator}`,
      );
    }
    lines.push('');
  }

  lines.push('## Explicit Edges', '');
  for (const edge of [...asArray(contract?.edges)].sort(byID)) {
    lines.push(
      `- ${edge.id}: ${edge.from} -[${edge.relation}]-> ${edge.to} ` +
        `(chains: ${asArray(edge.chain_ids).join(', ')})`,
    );
  }

  lines.push('', '## Ordered Chains', '');
  for (const chain of [...asArray(contract?.chains)].sort(byID)) {
    lines.push(`### ${chain.id}`);
    lines.push(`- name: ${chain.name}`);
    lines.push(`- canonical_executor: ${chain.canonical_executor === true}`);
    lines.push(`- path: ${asArray(chain.ordered_node_ids).join(' -> ')}`);
    if (asArray(chain.required_side_edge_ids).length > 0) {
      lines.push(
        `- required_side_edges: ${chain.required_side_edge_ids.join(', ')}`,
      );
    }
    lines.push('');
  }

  lines.push('## Exclusions', '');
  for (const exclusion of [...asArray(contract?.exclusions)].sort(byID)) {
    lines.push(
      `- ${exclusion.id}: ${exclusion.term} = ${exclusion.status}; ${exclusion.reason}`,
    );
  }

  return `${lines.join('\n')}\n`;
};

export const collectCorpusFiles = async (contract, options) => {
  const files = new Set();
  const addPath = candidate => {
    const relativePath = stringValue(candidate);
    if (!relativePath) {
      return;
    }
    normalizeRepoPath(options.repoRoot, relativePath);
    files.add(relativePath.replaceAll('\\', '/'));
  };
  const addAnchors = anchors => {
    for (const anchor of asArray(anchors)) {
      addPath(anchor?.path);
    }
  };

  addPath(contract?.authority?.contract);
  addPath(options?.authorityDocument ?? contract?.authority?.document);
  for (const node of asArray(contract?.nodes)) {
    addAnchors(node?.source_anchors);
    addAnchors(node?.evidence);
    addPath(node?.version_source?.path);
  }
  for (const edge of asArray(contract?.edges)) {
    addAnchors(edge?.evidence);
  }
  for (const chain of asArray(contract?.chains)) {
    addAnchors(chain?.test_evidence);
    addAnchors(chain?.ordered_source_steps);
  }

  const sorted = [...files].sort();
  await Promise.all(
    sorted.map(relativePath =>
      readFile(normalizeRepoPath(options.repoRoot, relativePath)),
    ),
  );
  return sorted;
};

export const sha256Files = async (files, root) => {
  const digest = createHash('sha256');
  for (const relativePath of stableUnique(files)) {
    const absolutePath = path.resolve(root, relativePath);
    const relativeToRoot = path.relative(path.resolve(root), absolutePath);
    if (
      relativeToRoot === '..' ||
      relativeToRoot.startsWith(`..${path.sep}`) ||
      path.isAbsolute(relativeToRoot)
    ) {
      throw new Error(`digest path escapes root: ${relativePath}`);
    }
    digest.update(relativePath.replaceAll('\\', '/'));
    digest.update('\u0000');
    digest.update(await readFile(absolutePath));
    digest.update('\u0000');
  }
  return digest.digest('hex');
};

export const writeCorpus = async (contract, options) => {
  const derivedRoot = path.resolve(options.derivedRoot);
  const corpusDir = path.join(derivedRoot, 'corpus');
  await rm(corpusDir, { recursive: true, force: true });
  await mkdir(path.join(corpusDir, 'source'), { recursive: true });

  const sourceFiles = await collectCorpusFiles(contract, options);
  for (const relativePath of sourceFiles) {
    const destination = path.join(corpusDir, 'source', relativePath);
    await mkdir(path.dirname(destination), { recursive: true });
    await copyFile(
      normalizeRepoPath(options.repoRoot, relativePath),
      destination,
    );
  }

  const ledgerPath = path.join(corpusDir, 'contract-ledger.md');
  await writeFile(
    ledgerPath,
    renderContractLedger(contract, options.resolvedVersions),
    'utf8',
  );
  const corpusFiles = [
    'contract-ledger.md',
    ...sourceFiles.map(relativePath => `source/${relativePath}`),
  ];
  const digest = await sha256Files(corpusFiles, corpusDir);

  return {
    corpusDir,
    digest,
    files: corpusFiles.sort(),
    sourceFiles,
  };
};

const explicitNode = (node, resolvedVersions) => {
  const primaryAnchor = asArray(node?.source_anchors)[0] ?? {};
  return {
    id: node.id,
    label: node.label,
    type: node.kind,
    layer: node.layer,
    production_status: node.production_status,
    runtime_scope: node.runtime_scope,
    package: node.package,
    version: resolvedVersions[node.id] ?? node.version,
    source_file: primaryAnchor.path,
    source_location: primaryAnchor.symbol ?? primaryAnchor.locator,
    authority: 'workbench_execution_contract',
  };
};

const explicitLink = edge => ({
  id: edge.id,
  source: edge.from,
  target: edge.to,
  relation: edge.relation,
  confidence: 'EXTRACTED',
  chain_ids: [...asArray(edge.chain_ids)],
  authority: 'workbench_execution_contract',
});

export const mergeExplicitGraph = (
  astGraph,
  contract,
  resolvedVersions = {},
) => {
  const astNodes = asArray(astGraph?.nodes);
  const astLinks = asArray(astGraph?.links ?? astGraph?.edges);
  const contractNodes = [...asArray(contract?.nodes)].sort(byID);
  const contractEdges = [...asArray(contract?.edges)].sort(byID);
  const contractNodeIDs = new Set(contractNodes.map(node => node.id));
  const contractEndpointKeys = new Set(
    contractEdges.map(edge => `${edge.from}\u0000${edge.to}`),
  );
  const contractEdgeIDs = new Set(contractEdges.map(edge => edge.id));

  const nodes = [
    ...astNodes.filter(node => !contractNodeIDs.has(stringValue(node?.id))),
    ...contractNodes.map(node => explicitNode(node, resolvedVersions)),
  ].sort(byID);
  const links = [
    ...astLinks.filter(
      link =>
        !contractEndpointKeys.has(edgeKey(link)) &&
        !contractEdgeIDs.has(stringValue(link?.id)),
    ),
    ...contractEdges.map(explicitLink),
  ].sort((left, right) => linkSortKey(left).localeCompare(linkSortKey(right)));

  const { nodes: _nodes, links: _links, edges: _edges, ...graphMetadata } =
    astGraph ?? {};
  return {
    ...graphMetadata,
    directed: true,
    nodes,
    links,
  };
};

const graphLinks = graph => asArray(graph?.links ?? graph?.edges);

export const verifyRequiredQueries = (graph, requiredQueries) => {
  const errors = [];
  const nodeIDs = new Set(asArray(graph?.nodes).map(node => stringValue(node?.id)));
  const linksByID = new Map(
    graphLinks(graph).map(link => [stringValue(link?.id), link]),
  );

  for (const query of asArray(requiredQueries)) {
    const queryID = stringValue(query?.id) || '<missing-query-id>';
    for (const nodeID of asArray(query?.required_node_ids)) {
      if (!nodeIDs.has(nodeID)) {
        errors.push(`required_query_node_missing: ${queryID}: ${nodeID}`);
      }
    }
    for (const edgeID of asArray(query?.required_edge_ids)) {
      if (!linksByID.has(edgeID)) {
        errors.push(`required_query_edge_missing: ${queryID}: ${edgeID}`);
      }
    }
  }

  return errors;
};

export const verifyOrderedPaths = (graph, chains) => {
  const errors = [];
  const linksByID = new Map(
    graphLinks(graph).map(link => [stringValue(link?.id), link]),
  );

  for (const chain of asArray(chains)) {
    const chainID = stringValue(chain?.id) || '<missing-chain-id>';
    const nodeIDs = asArray(chain?.ordered_node_ids);
    const edgeIDs = asArray(chain?.ordered_edge_ids);
    for (const [index, edgeID] of edgeIDs.entries()) {
      const link = linksByID.get(edgeID);
      if (!link) {
        errors.push(`ordered_path_edge_missing: ${chainID}: ${edgeID}`);
        continue;
      }
      const source = endpointID(link.source);
      const target = endpointID(link.target);
      if (source !== nodeIDs[index] || target !== nodeIDs[index + 1]) {
        errors.push(
          `ordered_path_edge_mismatch: ${chainID}: ${edgeID}: expected ${nodeIDs[index]} -> ${nodeIDs[index + 1]}`,
        );
      }
    }
    for (const edgeID of asArray(chain?.required_side_edge_ids)) {
      if (!linksByID.has(edgeID)) {
        errors.push(`ordered_path_side_edge_missing: ${chainID}: ${edgeID}`);
      }
    }
  }

  return errors;
};

const verifyGraphHealth = (graph, contract) => {
  const errors = [];
  if (graph?.directed !== true) {
    errors.push('derived_graph_not_directed: directed must be true');
  }

  const nodes = asArray(graph?.nodes);
  const links = graphLinks(graph);
  const nodeIDs = new Set();
  for (const node of nodes) {
    const nodeID = stringValue(node?.id);
    if (!nodeID) {
      errors.push('derived_node_id_missing: node requires an id');
    } else if (nodeIDs.has(nodeID)) {
      errors.push(`duplicate_derived_node: ${nodeID}`);
    }
    nodeIDs.add(nodeID);
  }

  const relationTuples = new Set();
  for (const link of links) {
    const linkID = stringValue(link?.id) || '<missing-edge-id>';
    const source = endpointID(link?.source);
    const target = endpointID(link?.target);
    const relation = stringValue(link?.relation);
    if (!nodeIDs.has(source)) {
      errors.push(`derived_edge_source_missing: ${linkID}: ${source}`);
    }
    if (!nodeIDs.has(target)) {
      errors.push(`derived_edge_target_missing: ${linkID}: ${target}`);
    }
    if (source && source === target) {
      errors.push(`derived_self_loop: ${linkID}: ${source}`);
    }
    const tuple = `${source}\u0000${target}\u0000${relation}`;
    if (relationTuples.has(tuple)) {
      errors.push(
        `duplicate_derived_relation: ${source} -> ${target}: ${relation}`,
      );
    }
    relationTuples.add(tuple);
  }

  const linksByID = new Map(links.map(link => [stringValue(link?.id), link]));
  for (const node of asArray(contract?.nodes)) {
    if (!nodeIDs.has(node.id)) {
      errors.push(`contract_node_missing: ${node.id}`);
    }
  }
  for (const edge of asArray(contract?.edges)) {
    const link = linksByID.get(edge.id);
    if (!link) {
      errors.push(`contract_edge_missing: ${edge.id}`);
      continue;
    }
    if (
      endpointID(link.source) !== edge.from ||
      endpointID(link.target) !== edge.to ||
      stringValue(link.relation) !== edge.relation ||
      link.confidence !== 'EXTRACTED'
    ) {
      errors.push(`contract_edge_mismatch: ${edge.id}`);
    }
  }

  const forbiddenTerms = asArray(contract?.scope?.forbidden_current_terms)
    .map(term => stringValue(term).toLowerCase())
    .filter(Boolean);
  for (const node of nodes) {
    if (node?.production_status !== 'current') {
      continue;
    }
    const searchable = `${stringValue(node?.id)} ${stringValue(node?.label)}`.toLowerCase();
    for (const term of forbiddenTerms) {
      if (searchable.includes(term)) {
        errors.push(`forbidden_derived_node: ${node.id}: ${term}`);
      }
    }
  }

  return errors;
};

const defaultQueryRunner = async ({ graphPath, terms, graphifyBinary }) => {
  const { stdout } = await execFileAsync(
    graphifyBinary,
    ['query', terms, '--graph', graphPath, '--budget', '2000'],
    {
      encoding: 'utf8',
      maxBuffer: 16 * 1024 * 1024,
    },
  );
  return stdout;
};

const verifyGraphifyQueries = async (graph, contract, options) => {
  const errors = [];
  const nodesByID = new Map(
    asArray(graph?.nodes).map(node => [stringValue(node?.id), node]),
  );
  const queryRunner = options.queryRunner ?? defaultQueryRunner;
  for (const query of asArray(contract?.required_queries)) {
    const queryID = stringValue(query?.id) || '<missing-query-id>';
    const terms = asArray(query?.expanded_terms).map(stringValue).filter(Boolean);
    if (terms.length === 0) {
      errors.push(`graphify_query_terms_missing: ${queryID}`);
      continue;
    }
    const smokeNodeIDs =
      asArray(query?.smoke_node_ids).length > 0
        ? query.smoke_node_ids
        : asArray(query?.required_node_ids).slice(0, 1);
    const requiredLabels = smokeNodeIDs
      .map(nodeID => stringValue(nodesByID.get(nodeID)?.label))
      .filter(Boolean);
    try {
      const output = String(
        await queryRunner({
          graphPath: options.graphPath,
          graphifyBinary: options.graphifyBinary ?? 'graphify',
          query,
          requiredLabels,
          terms: terms.join(' '),
        }),
      ).toLowerCase();
      const missingLabels = requiredLabels.filter(
        label => !output.includes(label.toLowerCase()),
      );
      if (missingLabels.length > 0) {
        errors.push(
          `graphify_query_smoke_failed: ${queryID}: missing ${missingLabels.join(', ')}`,
        );
      }
    } catch (error) {
      errors.push(
        `graphify_query_smoke_failed: ${queryID}: ${stringValue(error?.message) || 'query failed'}`,
      );
    }
  }
  return errors;
};

export const verifyDerivedGraph = async (contract, options) => {
  const errors = [];
  const warnings = [];
  const derivedRoot = path.resolve(
    options?.derivedRoot ?? path.join(options.repoRoot, DERIVED_RELATIVE_ROOT),
  );
  const graphPath = path.join(derivedRoot, 'graphify-out', 'graph.json');
  const metadataPath = path.join(derivedRoot, 'build-meta.json');

  let graph;
  let metadata;
  try {
    [graph, metadata] = await Promise.all([
      readFile(graphPath, 'utf8').then(JSON.parse),
      readFile(metadataPath, 'utf8').then(JSON.parse),
    ]);
  } catch (error) {
    return {
      errors: [`derived_graph_unreadable: ${error.message}`],
      warnings,
    };
  }

  const digestRoot = await mkdtemp(
    path.join(os.tmpdir(), 'workbench-derived-digest-'),
  );
  try {
    const current = await writeCorpus(contract, {
      ...options,
      derivedRoot: digestRoot,
    });
    if (metadata?.corpus_digest !== current.digest) {
      errors.push(
        `stale_derived_digest: built ${stringValue(metadata?.corpus_digest) || '<missing>'} current ${current.digest}`,
      );
    }
  } catch (error) {
    errors.push(`derived_digest_failed: ${error.message}`);
  } finally {
    await rm(digestRoot, { recursive: true, force: true });
  }

  errors.push(...verifyGraphHealth(graph, contract));
  errors.push(...verifyRequiredQueries(graph, contract?.required_queries));
  errors.push(...verifyOrderedPaths(graph, contract?.chains));
  if (options?.runGraphifyQueries !== false) {
    errors.push(
      ...(await verifyGraphifyQueries(graph, contract, {
        ...options,
        graphPath,
      })),
    );
  }

  return {
    errors: [...new Set(errors)].sort(),
    warnings: [...new Set(warnings)].sort(),
  };
};

const runGraphifyExtraction = async (corpusDir, outputDir, graphifyBinary) => {
  try {
    await execFileAsync(
      graphifyBinary,
      [
        'extract',
        corpusDir,
        '--out',
        outputDir,
        '--code-only',
        '--no-cluster',
      ],
      {
        encoding: 'utf8',
        maxBuffer: 32 * 1024 * 1024,
      },
    );
  } catch (error) {
    if (error?.code === 'ENOENT') {
      const unavailable = new Error('graphify_unavailable');
      unavailable.code = 'graphify_unavailable';
      throw unavailable;
    }
    throw error;
  }
};

const commandOutput = async (command, args, cwd, fallback) => {
  try {
    const { stdout } = await execFileAsync(command, args, {
      cwd,
      encoding: 'utf8',
    });
    return stringValue(stdout) || fallback;
  } catch {
    return fallback;
  }
};

const installCompletedBuild = async (temporaryRoot, derivedRoot) => {
  const parent = path.dirname(derivedRoot);
  const backupRoot = path.join(
    parent,
    `.${path.basename(derivedRoot)}-backup-${process.pid}-${Date.now()}`,
  );
  let previousMoved = false;
  try {
    try {
      await rename(derivedRoot, backupRoot);
      previousMoved = true;
    } catch (error) {
      if (error?.code !== 'ENOENT') {
        throw error;
      }
    }
    await rename(temporaryRoot, derivedRoot);
    if (previousMoved) {
      await rm(backupRoot, { recursive: true, force: true });
    }
  } catch (error) {
    if (previousMoved) {
      await rename(backupRoot, derivedRoot).catch(() => undefined);
    }
    throw error;
  }
};

export const buildDerivedGraph = async (contract, options) => {
  const derivedRoot = path.resolve(
    options?.derivedRoot ?? path.join(options.repoRoot, DERIVED_RELATIVE_ROOT),
  );
  const parent = path.dirname(derivedRoot);
  await mkdir(parent, { recursive: true });
  const temporaryRoot = await mkdtemp(
    path.join(parent, `.${path.basename(derivedRoot)}-build-`),
  );

  try {
    const corpus = await writeCorpus(contract, {
      ...options,
      derivedRoot: temporaryRoot,
    });
    const graphifyBinary = options?.graphifyBinary ?? 'graphify';
    await runGraphifyExtraction(
      corpus.corpusDir,
      temporaryRoot,
      graphifyBinary,
    );
    const graphPath = path.join(temporaryRoot, 'graphify-out', 'graph.json');
    const astGraph = JSON.parse(await readFile(graphPath, 'utf8'));
    const mergedGraph = mergeExplicitGraph(
      astGraph,
      contract,
      options?.resolvedVersions,
    );
    await writeFile(
      graphPath,
      `${JSON.stringify(mergedGraph, null, 2)}\n`,
      'utf8',
    );

    const [gitCommit, graphifyVersion] = await Promise.all([
      commandOutput('git', ['rev-parse', 'HEAD'], options.repoRoot, 'unknown'),
      commandOutput(graphifyBinary, ['--version'], options.repoRoot, 'unknown'),
    ]);
    const metadata = {
      schema_version: contract.schema_version,
      git_commit: gitCommit,
      corpus_digest: corpus.digest,
      corpus_files: corpus.files,
      graphify_version: graphifyVersion,
      generated_at: new Date().toISOString(),
    };
    await writeFile(
      path.join(temporaryRoot, 'build-meta.json'),
      `${JSON.stringify(metadata, null, 2)}\n`,
      'utf8',
    );
    await installCompletedBuild(temporaryRoot, derivedRoot);
    return {
      derivedRoot,
      graphPath: path.join(derivedRoot, 'graphify-out', 'graph.json'),
      metadata,
    };
  } catch (error) {
    await rm(temporaryRoot, { recursive: true, force: true });
    throw error;
  }
};
