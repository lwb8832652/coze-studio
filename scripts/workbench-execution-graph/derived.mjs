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
  lstat,
  mkdir,
  mkdtemp,
  readFile,
  readlink,
  realpath,
  rename,
  rm,
  stat,
  symlink,
  writeFile,
} from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import { promisify } from 'node:util';

import { normalizeRepoPath } from './contract.mjs';

const execFileAsync = promisify(execFile);

export const DERIVED_RELATIVE_ROOT =
  'docs/superpowers/context/workbench-execution-graphify';
export const DERIVED_MANAGED_MARKER = '.workbench-execution-graph-managed.json';

const DERIVED_MANAGED_OWNER = 'workbench_execution_graph';

const BUILDER_INPUT_FILES = [
  'scripts/workbench-execution-graph.mjs',
  'scripts/workbench-execution-graph/contract.mjs',
  'scripts/workbench-execution-graph/derived.mjs',
];

const asArray = value => (Array.isArray(value) ? value : []);
const stringValue = value => (typeof value === 'string' ? value.trim() : '');
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

const astLinkID = link =>
  `ast.${createHash('sha256')
    .update(
      [
        endpointID(link?.source),
        endpointID(link?.target),
        stringValue(link?.relation),
      ].join('\u0000'),
    )
    .digest('hex')
    .slice(0, 24)}`;

const stableDerivedID = (prefix, ...parts) =>
  `${prefix}.${createHash('sha256')
    .update(parts.join('\u0000'))
    .digest('hex')
    .slice(0, 24)}`;

const stableUnique = values => [...new Set(values.filter(Boolean))].sort();

const managedMarkerPayload = () => ({
  schema_version: 1,
  owner: DERIVED_MANAGED_OWNER,
});

const writeManagedBuildMarker = root =>
  writeFile(
    path.join(root, DERIVED_MANAGED_MARKER),
    `${JSON.stringify(managedMarkerPayload(), null, 2)}\n`,
    'utf8',
  );

const unmanagedBuildRootError = root => {
  const error = new Error(`derived_publish_target_unmanaged: ${root}`);
  error.code = 'derived_publish_target_unmanaged';
  return error;
};

const isManagedVersionPath = (versionsRoot, candidate) => {
  const relative = path.relative(versionsRoot, candidate);
  return (
    relative !== '' &&
    relative !== '..' &&
    !relative.startsWith(`..${path.sep}`) &&
    !path.isAbsolute(relative)
  );
};

export const assertManagedBuildRoot = async (
  root,
  { allowRootSymlink = false } = {},
) => {
  let rootStat;
  try {
    rootStat = await lstat(root);
  } catch {
    throw unmanagedBuildRootError(root);
  }
  if (rootStat.isSymbolicLink()) {
    if (!allowRootSymlink || !(await stat(root)).isDirectory()) {
      throw unmanagedBuildRootError(root);
    }
  } else if (!rootStat.isDirectory()) {
    throw unmanagedBuildRootError(root);
  }

  try {
    if (!(await lstat(path.join(root, DERIVED_MANAGED_MARKER))).isFile()) {
      throw unmanagedBuildRootError(root);
    }
    const marker = JSON.parse(
      await readFile(path.join(root, DERIVED_MANAGED_MARKER), 'utf8'),
    );
    if (
      marker?.schema_version !== 1 ||
      marker?.owner !== DERIVED_MANAGED_OWNER
    ) {
      throw unmanagedBuildRootError(root);
    }
    return;
  } catch {
    throw unmanagedBuildRootError(root);
  }
};

const resolveManagedVersionTarget = async (
  derivedRoot,
  target,
  pointerName,
) => {
  const versionsRoot = `${derivedRoot}-versions`;
  const resolvedVersionsRoot = path.resolve(versionsRoot);
  const resolvedTarget = path.resolve(target);
  if (!isManagedVersionPath(resolvedVersionsRoot, resolvedTarget)) {
    throw new Error(`${pointerName}_outside_versions: ${target}`);
  }
  const versionsStat = await lstat(versionsRoot);
  const targetStat = await lstat(target);
  if (!versionsStat.isDirectory() || !targetStat.isDirectory()) {
    throw new Error(`${pointerName}_target_invalid: ${target}`);
  }
  const [versionsRealPath, targetRealPath] = await Promise.all([
    realpath(versionsRoot),
    realpath(target),
  ]);
  if (!isManagedVersionPath(versionsRealPath, targetRealPath)) {
    throw new Error(`${pointerName}_outside_versions: ${target}`);
  }
  await assertManagedBuildRoot(targetRealPath);
  return resolvedTarget;
};

const assertDerivedFilesContained = async (managedRoot, files) => {
  for (const candidate of files) {
    if (!(await lstat(candidate)).isFile()) {
      throw new Error(`derived_file_invalid: ${candidate}`);
    }
    const candidateRealPath = await realpath(candidate);
    if (!isManagedVersionPath(managedRoot, candidateRealPath)) {
      throw new Error(`derived_file_outside_version: ${candidate}`);
    }
  }
};

const verifyPreviousPointer = async derivedRoot => {
  const previousPointerRoot = `${derivedRoot}-previous`;
  let previousStat;
  try {
    previousStat = await lstat(previousPointerRoot);
  } catch (error) {
    if (error?.code === 'ENOENT') {
      return;
    }
    throw error;
  }
  if (!previousStat.isSymbolicLink()) {
    throw new Error(`derived_previous_pointer_invalid: ${previousPointerRoot}`);
  }
  const target = path.resolve(
    path.dirname(previousPointerRoot),
    await readlink(previousPointerRoot),
  );
  await resolveManagedVersionTarget(
    derivedRoot,
    target,
    'derived_previous_pointer',
  );
};

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

const SENSITIVE_CORPUS_SEGMENTS = new Set([
  '.git',
  '.ssh',
  'credential',
  'credentials',
  'log',
  'logs',
  'secret',
  'secrets',
]);
const SENSITIVE_CORPUS_FILE_PATTERN =
  /(?:^\.env(?:\..*)?$|^(?:credentials?|secrets?)(?:[._-].*)?$|(?:^|[._-])(?:access[_-]?token|api[_-]?key|private[_-]?key)(?:[._-]|$)|\.(?:key|keystore|log|p12|pem|pfx|sqlite|sqlite3)$)/i;
const ALLOWED_CORPUS_FILE_PATTERN =
  /\.(?:c|cc|cpp|cs|go|java|js|jsx|mjs|cjs|json|md|mod|php|py|rb|rs|sum|swift|thrift|ts|tsx|ya?ml)$/i;

const assertCorpusPathAllowed = (inspectedPath, displayPath) => {
  const normalized = stringValue(inspectedPath).replaceAll('\\', '/');
  const segments = normalized.toLowerCase().split('/').filter(Boolean);
  const basename = segments.at(-1) ?? '';
  if (
    segments.some(segment => SENSITIVE_CORPUS_SEGMENTS.has(segment)) ||
    SENSITIVE_CORPUS_FILE_PATTERN.test(basename)
  ) {
    throw new Error(`sensitive_corpus_path: ${displayPath}`);
  }
  if (!ALLOWED_CORPUS_FILE_PATTERN.test(normalized)) {
    throw new Error(`unsupported_corpus_path: ${displayPath}`);
  }
};

const assertNoCorpusSymlink = async (repoRoot, relativePath) => {
  let current = path.resolve(repoRoot);
  for (const segment of relativePath.split('/').filter(Boolean)) {
    current = path.join(current, segment);
    if ((await lstat(current)).isSymbolicLink()) {
      throw new Error(`corpus_path_symlink: ${relativePath}`);
    }
  }
};

export const validateCorpusSourcePath = async (repoRoot, candidate) => {
  const relativePath = stringValue(candidate).replaceAll('\\', '/');
  assertCorpusPathAllowed(relativePath, relativePath);

  const lexicalPath = normalizeRepoPath(repoRoot, relativePath);
  const [rootRealPath, sourceRealPath] = await Promise.all([
    realpath(repoRoot),
    realpath(lexicalPath),
  ]);
  const relativeToRoot = path.relative(rootRealPath, sourceRealPath);
  if (
    relativeToRoot === '..' ||
    relativeToRoot.startsWith(`..${path.sep}`) ||
    path.isAbsolute(relativeToRoot)
  ) {
    throw new Error(`corpus_path_escapes_repo: ${relativePath}`);
  }
  const resolvedRelativePath = path
    .relative(rootRealPath, sourceRealPath)
    .replaceAll('\\', '/');
  assertCorpusPathAllowed(resolvedRelativePath, relativePath);
  await assertNoCorpusSymlink(repoRoot, relativePath);
  if (!(await stat(sourceRealPath)).isFile()) {
    throw new Error(`corpus_path_not_file: ${relativePath}`);
  }
  return sourceRealPath;
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
  for (const exclusion of asArray(contract?.exclusions)) {
    addAnchors(exclusion?.evidence);
  }

  const sorted = [...files].sort();
  await Promise.all(
    sorted.map(relativePath =>
      validateCorpusSourcePath(options.repoRoot, relativePath),
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

export const computeBuilderDigest = repoRoot =>
  sha256Files(BUILDER_INPUT_FILES, repoRoot);

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
      await validateCorpusSourcePath(options.repoRoot, relativePath),
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

const explicitLink = edge => {
  const primaryEvidence = asArray(edge?.evidence)[0] ?? {};
  return {
    id: edge.id,
    source: edge.from,
    target: edge.to,
    relation: edge.relation,
    confidence: 'EXTRACTED',
    chain_ids: [...asArray(edge.chain_ids)],
    source_file: primaryEvidence.path,
    source_location: primaryEvidence.symbol ?? primaryEvidence.locator,
    authority: 'workbench_execution_contract',
  };
};

const queryOverlayNode = (query, contract) => ({
  id: query.id,
  label: query.question,
  type: 'retrieval_intent',
  layer: 'knowledge_query',
  production_status: 'current',
  runtime_scope: 'documentation',
  source_file: contract?.authority?.contract,
  source_location: query.id,
  authority: 'workbench_required_query_contract',
  _origin: 'query_overlay',
});

const queryTargetNodeIDs = (contract, query) => {
  const edgesByID = new Map(
    asArray(contract?.edges).map(edge => [stringValue(edge?.id), edge]),
  );
  return stableUnique([
    ...asArray(query?.required_node_ids),
    ...asArray(query?.required_edge_ids).flatMap(edgeID => {
      const edge = edgesByID.get(edgeID);
      return edge ? [edge.from, edge.to] : [];
    }),
  ]);
};

const queryOverlayLinks = contract =>
  asArray(contract?.required_queries).flatMap(query =>
    queryTargetNodeIDs(contract, query).map(nodeID => ({
      id: stableDerivedID('query', query.id, nodeID, 'retrieves'),
      source: query.id,
      target: nodeID,
      relation: 'retrieves',
      confidence: 'EXTRACTED',
      authority: 'workbench_required_query_contract',
      source_file: contract?.authority?.contract,
      source_location: query.id,
      _origin: 'query_overlay',
    })),
  );

const isASTFileNode = node => {
  const sourceFile = stringValue(node?.source_file).replaceAll('\\', '/');
  const label = stringValue(node?.label).replaceAll('\\', '/');
  const sourceLocation = stringValue(node?.source_location);
  if (!sourceFile || !label || sourceLocation !== 'L1') {
    return false;
  }
  const basename = path.posix.basename(sourceFile);
  return (
    label === basename ||
    (label.includes('/') &&
      (sourceFile === label || sourceFile.endsWith(`/${label}`)))
  );
};

const sourceAnchorLinks = (astNodes, contractNodes) => {
  const fileNodesByPath = new Map();
  for (const node of [...astNodes].sort(byID)) {
    const sourceFile = stringValue(node?.source_file).replaceAll('\\', '/');
    if (!sourceFile) {
      continue;
    }
    if (isASTFileNode(node) && !fileNodesByPath.has(sourceFile)) {
      fileNodesByPath.set(sourceFile, node);
    }
  }

  const links = [];
  for (const node of contractNodes) {
    const anchorPaths = stableUnique(
      asArray(node?.source_anchors).map(anchor =>
        stringValue(anchor?.path).replaceAll('\\', '/'),
      ),
    );
    for (const anchorPath of anchorPaths) {
      const fileNode = fileNodesByPath.get(`source/${anchorPath}`);
      if (!fileNode) {
        continue;
      }
      links.push({
        id: stableDerivedID('anchor', node.id, fileNode.id, 'anchored_in'),
        source: node.id,
        target: fileNode.id,
        relation: 'anchored_in',
        confidence: 'EXTRACTED',
        authority: 'workbench_source_anchor',
        source_file: anchorPath,
        source_location: stringValue(fileNode?.source_location) || 'L1',
        _origin: 'bridge',
      });
    }
  }
  return links.sort((left, right) =>
    linkSortKey(left).localeCompare(linkSortKey(right)),
  );
};

export const mergeExplicitGraph = (
  astGraph,
  contract,
  resolvedVersions = {},
  options = {},
) => {
  const astNodes = asArray(astGraph?.nodes);
  const astLinks = asArray(astGraph?.links ?? astGraph?.edges);
  const contractNodes = [...asArray(contract?.nodes)].sort(byID);
  const contractEdges = [...asArray(contract?.edges)].sort(byID);
  const includeQueryOverlay = options?.includeQueryOverlay === true;
  const queryNodes = includeQueryOverlay
    ? asArray(contract?.required_queries)
        .map(query => queryOverlayNode(query, contract))
        .sort(byID)
    : [];
  const explicitNodeIDs = new Set(
    [...contractNodes, ...queryNodes].map(node => node.id),
  );
  const contractEndpointKeys = new Set(
    contractEdges.map(edge => `${edge.from}\u0000${edge.to}`),
  );
  const contractEdgeIDs = new Set(contractEdges.map(edge => edge.id));

  const nodes = [
    ...astNodes.filter(node => !explicitNodeIDs.has(stringValue(node?.id))),
    ...contractNodes.map(node => explicitNode(node, resolvedVersions)),
    ...queryNodes,
  ].sort(byID);
  const mergedNodeIDs = new Set(nodes.map(node => stringValue(node?.id)));
  const astRelationTuples = new Set();
  const validASTLinks = [...astLinks]
    .sort((left, right) => linkSortKey(left).localeCompare(linkSortKey(right)))
    .filter(link => {
      const source = endpointID(link?.source);
      const target = endpointID(link?.target);
      const relation = stringValue(link?.relation);
      const tuple = `${source}\u0000${target}\u0000${relation}`;
      if (
        !mergedNodeIDs.has(source) ||
        !mergedNodeIDs.has(target) ||
        source === target ||
        contractEndpointKeys.has(edgeKey(link)) ||
        contractEdgeIDs.has(stringValue(link?.id)) ||
        astRelationTuples.has(tuple)
      ) {
        return false;
      }
      astRelationTuples.add(tuple);
      return true;
    })
    .map(link => ({
      ...link,
      id: astLinkID(link),
      authority: 'graphify_ast',
    }));
  const links = [
    ...validASTLinks,
    ...sourceAnchorLinks(astNodes, contractNodes),
    ...(includeQueryOverlay ? queryOverlayLinks(contract) : []),
    ...contractEdges.map(explicitLink),
  ].sort((left, right) => linkSortKey(left).localeCompare(linkSortKey(right)));

  const {
    nodes: _nodes,
    links: _links,
    edges: _edges,
    ...graphMetadata
  } = astGraph ?? {};
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
  const nodeIDs = new Set(
    asArray(graph?.nodes).map(node => stringValue(node?.id)),
  );
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

const sameStringArray = (actual, expected) =>
  JSON.stringify(asArray(actual)) === JSON.stringify(asArray(expected));

const verifyGraphHealth = (graph, contract, resolvedVersions = {}) => {
  const errors = [];
  if (graph?.directed !== true) {
    errors.push('derived_graph_not_directed: directed must be true');
  }

  const nodes = asArray(graph?.nodes);
  const links = graphLinks(graph);
  const nodeIDs = new Set();
  const nodesByID = new Map();
  for (const node of nodes) {
    const nodeID = stringValue(node?.id);
    if (!nodeID) {
      errors.push('derived_node_id_missing: node requires an id');
    } else if (nodeIDs.has(nodeID)) {
      errors.push(`duplicate_derived_node: ${nodeID}`);
    }
    nodeIDs.add(nodeID);
    nodesByID.set(nodeID, node);
  }

  const relationTuples = new Set();
  const linkIDs = new Set();
  for (const link of links) {
    const rawLinkID = stringValue(link?.id);
    const linkID = rawLinkID || '<missing-edge-id>';
    if (!rawLinkID) {
      errors.push('derived_edge_id_missing: edge requires an id');
    } else if (linkIDs.has(rawLinkID)) {
      errors.push(`duplicate_derived_edge: ${rawLinkID}`);
    }
    linkIDs.add(rawLinkID);
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
    const derivedNode = nodesByID.get(node.id);
    if (!derivedNode) {
      errors.push(`contract_node_missing: ${node.id}`);
      continue;
    }
    const expectedNode = explicitNode(node, resolvedVersions);
    const mismatchedFields = [
      'label',
      'type',
      'layer',
      'production_status',
      'runtime_scope',
      'package',
      'version',
      'source_file',
      'source_location',
      'authority',
    ].filter(field => derivedNode[field] !== expectedNode[field]);
    if (mismatchedFields.length > 0) {
      errors.push(
        `contract_node_mismatch: ${node.id}: ${mismatchedFields.join(', ')}`,
      );
    }
  }
  for (const edge of asArray(contract?.edges)) {
    const link = linksByID.get(edge.id);
    if (!link) {
      errors.push(`contract_edge_missing: ${edge.id}`);
      continue;
    }
    const expectedLink = explicitLink(edge);
    if (
      endpointID(link.source) !== edge.from ||
      endpointID(link.target) !== edge.to ||
      stringValue(link.relation) !== edge.relation ||
      link.confidence !== 'EXTRACTED' ||
      link.authority !== 'workbench_execution_contract' ||
      link.source_file !== expectedLink.source_file ||
      link.source_location !== expectedLink.source_location ||
      !sameStringArray(link.chain_ids, edge.chain_ids)
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
    const searchable =
      `${stringValue(node?.id)} ${stringValue(node?.label)}`.toLowerCase();
    for (const term of forbiddenTerms) {
      if (searchable.includes(term)) {
        errors.push(`forbidden_derived_node: ${node.id}: ${term}`);
      }
    }
  }

  return errors;
};

const derivedBuildStats = (graph, queryGraph) => {
  const astNodes = asArray(graph?.nodes).filter(
    node => node?._origin === 'ast',
  );
  const astLinks = graphLinks(graph).filter(link => link?._origin === 'ast');
  return {
    execution_node_count: asArray(graph?.nodes).length,
    execution_link_count: graphLinks(graph).length,
    query_graph_node_count: asArray(queryGraph?.nodes).length,
    query_graph_link_count: graphLinks(queryGraph).length,
    ast_node_count: astNodes.length,
    ast_link_count: astLinks.length,
    ast_source_files: [
      ...astSourceFiles({ nodes: astNodes, links: astLinks }),
    ].sort(),
    bridge_link_count: graphLinks(graph).filter(
      link => link?._origin === 'bridge',
    ).length,
    query_overlay_node_count: asArray(queryGraph?.nodes).filter(
      node => node?._origin === 'query_overlay',
    ).length,
    query_overlay_link_count: graphLinks(queryGraph).filter(
      link => link?._origin === 'query_overlay',
    ).length,
  };
};

const sameGraphItem = (actual, expected) =>
  JSON.stringify(actual) === JSON.stringify(expected);

const verifyQueryGraphBase = (graph, queryGraph, contract) => {
  const errors = [];
  const expectedOverlayNodeIDs = new Set(
    asArray(contract?.required_queries).map(
      query => queryOverlayNode(query, contract).id,
    ),
  );
  const expectedOverlayLinkIDs = new Set(
    queryOverlayLinks(contract).map(link => link.id),
  );
  const executionNodesByID = new Map(
    asArray(graph?.nodes).map(node => [stringValue(node?.id), node]),
  );
  const queryNodesByID = new Map(
    asArray(queryGraph?.nodes).map(node => [stringValue(node?.id), node]),
  );
  const executionLinksByID = new Map(
    graphLinks(graph).map(link => [stringValue(link?.id), link]),
  );
  const queryLinksByID = new Map(
    graphLinks(queryGraph).map(link => [stringValue(link?.id), link]),
  );

  for (const [nodeID, expected] of executionNodesByID) {
    if (!sameGraphItem(queryNodesByID.get(nodeID), expected)) {
      errors.push(`query_graph_base_node_mismatch: ${nodeID}`);
    }
  }
  for (const [linkID, expected] of executionLinksByID) {
    if (!sameGraphItem(queryLinksByID.get(linkID), expected)) {
      errors.push(`query_graph_base_edge_mismatch: ${linkID}`);
    }
  }
  for (const node of asArray(queryGraph?.nodes)) {
    const nodeID = stringValue(node?.id);
    if (!executionNodesByID.has(nodeID)) {
      if (
        node?._origin !== 'query_overlay' ||
        !expectedOverlayNodeIDs.has(nodeID)
      ) {
        errors.push(`query_graph_unexpected_node: ${nodeID}`);
      }
    }
  }
  for (const link of graphLinks(queryGraph)) {
    const linkID = stringValue(link?.id);
    if (!executionLinksByID.has(linkID)) {
      if (
        link?._origin !== 'query_overlay' ||
        !expectedOverlayLinkIDs.has(linkID)
      ) {
        errors.push(`query_graph_unexpected_edge: ${linkID}`);
      }
    }
  }
  return errors;
};

const verifyDerivedStructure = (graph, queryGraph, contract, metadata) => {
  const errors = [];
  const stats = derivedBuildStats(graph, queryGraph);
  for (const field of [
    'execution_node_count',
    'execution_link_count',
    'query_graph_node_count',
    'query_graph_link_count',
    'ast_node_count',
    'ast_link_count',
    'bridge_link_count',
    'query_overlay_node_count',
    'query_overlay_link_count',
  ]) {
    if (metadata?.[field] !== stats[field]) {
      errors.push(
        `derived_ast_count_mismatch: ${field}: built ${String(metadata?.[field])} actual ${stats[field]}`,
      );
    }
  }
  if (!sameStringArray(metadata?.ast_source_files, stats.ast_source_files)) {
    errors.push('derived_ast_source_files_mismatch');
  }
  if (
    asArray(graph?.nodes).some(node => node?._origin === 'query_overlay') ||
    graphLinks(graph).some(link => link?._origin === 'query_overlay')
  ) {
    errors.push('execution_graph_query_overlay_present');
  }
  for (const link of graphLinks(graph)) {
    if (stringValue(link?.relation) === 'retrieves') {
      errors.push(
        `execution_graph_retrieves_edge_present: ${stringValue(link?.id) || '<missing-edge-id>'}`,
      );
    }
  }
  errors.push(...verifyQueryGraphBase(graph, queryGraph, contract));

  const astGraph = {
    nodes: asArray(graph?.nodes).filter(node => node?._origin === 'ast'),
    links: graphLinks(graph).filter(link => link?._origin === 'ast'),
  };
  try {
    assertNonEmptyASTGraph(astGraph, metadata?.corpus_files);
    assertASTSourceCoverage(astGraph, metadata?.corpus_files);
  } catch (error) {
    errors.push(error.message);
  }

  const linksByID = new Map(
    graphLinks(graph).map(link => [stringValue(link?.id), link]),
  );
  const expectedBridges = sourceAnchorLinks(astGraph.nodes, contract?.nodes);
  const expectedBridgeIDs = new Set(expectedBridges.map(link => link.id));
  const bridgedContractNodeIDs = new Set(
    expectedBridges.map(link => endpointID(link.source)),
  );
  for (const node of asArray(contract?.nodes)) {
    if (
      node?.production_status === 'current' &&
      !bridgedContractNodeIDs.has(stringValue(node?.id))
    ) {
      errors.push(`source_anchor_ast_file_missing: ${stringValue(node?.id)}`);
    }
  }
  for (const link of graphLinks(graph)) {
    if (
      (link?._origin === 'bridge' || link?.relation === 'anchored_in') &&
      !expectedBridgeIDs.has(stringValue(link?.id))
    ) {
      errors.push(
        `source_anchor_bridge_unexpected: ${stringValue(link?.id) || '<missing-edge-id>'}`,
      );
    }
  }
  for (const expected of expectedBridges) {
    const actual = linksByID.get(expected.id);
    if (
      !actual ||
      endpointID(actual.source) !== expected.source ||
      endpointID(actual.target) !== expected.target ||
      actual.relation !== expected.relation ||
      actual.authority !== expected.authority ||
      actual.source_file !== expected.source_file ||
      actual.source_location !== expected.source_location ||
      actual._origin !== expected._origin
    ) {
      errors.push(
        `source_anchor_bridge_missing: ${expected.source} -> ${expected.target}`,
      );
    }
  }

  const queryNodesByID = new Map(
    asArray(queryGraph?.nodes).map(node => [stringValue(node?.id), node]),
  );
  for (const query of asArray(contract?.required_queries)) {
    const expectedNode = queryOverlayNode(query, contract);
    const actualNode = queryNodesByID.get(expectedNode.id);
    if (
      !actualNode ||
      actualNode.label !== expectedNode.label ||
      actualNode.authority !== expectedNode.authority ||
      actualNode.source_file !== expectedNode.source_file ||
      actualNode.source_location !== expectedNode.source_location ||
      actualNode._origin !== expectedNode._origin
    ) {
      errors.push(`query_overlay_node_missing: ${expectedNode.id}`);
    }
  }
  const queryLinksByID = new Map(
    graphLinks(queryGraph).map(link => [stringValue(link?.id), link]),
  );
  for (const expected of queryOverlayLinks(contract)) {
    const actual = queryLinksByID.get(expected.id);
    if (
      !actual ||
      endpointID(actual.source) !== expected.source ||
      endpointID(actual.target) !== expected.target ||
      actual.relation !== expected.relation ||
      actual.authority !== expected.authority ||
      actual.source_file !== expected.source_file ||
      actual.source_location !== expected.source_location ||
      actual._origin !== expected._origin
    ) {
      errors.push(
        `query_overlay_edge_missing: ${expected.source} -> ${expected.target}`,
      );
    }
  }

  return errors;
};

const defaultQueryRunner = async ({ graphPath, terms, graphifyBinary }) => {
  const { stdout } = await execFileAsync(
    graphifyBinary,
    ['query', terms, '--graph', graphPath, '--budget', '12000'],
    {
      encoding: 'utf8',
      maxBuffer: 16 * 1024 * 1024,
    },
  );
  return stdout;
};

const defaultPathRunner = async ({
  graphPath,
  graphifyBinary,
  sourceLabel,
  targetLabel,
}) => {
  const { stdout } = await execFileAsync(
    graphifyBinary,
    ['path', sourceLabel, targetLabel, '--graph', graphPath],
    {
      encoding: 'utf8',
      maxBuffer: 4 * 1024 * 1024,
    },
  );
  return stdout;
};

const graphifyResultLabels = output =>
  new Set(
    String(output)
      .split(/\r?\n/)
      .filter(line => line.startsWith('NODE '))
      .map(line =>
        line.slice('NODE '.length).split(' [src=', 1)[0].trim().toLowerCase(),
      )
      .filter(Boolean),
  );

const graphifyEdgeResultKey = (sourceLabel, relation, targetLabel) =>
  [sourceLabel, relation, targetLabel]
    .map(value => stringValue(value).toLowerCase())
    .join('\u0000');

const graphifyResultEdges = output => {
  const result = new Set();
  for (const line of String(output).split(/\r?\n/)) {
    const normalized = line.trim().replace(/^EDGE /, '');
    const match = normalized.match(
      /^(.+?) --([^\s]+) \[[^\]]+\]--> (.+?)(?: at=.*)?$/,
    );
    if (match) {
      for (const relation of match[2].split('/').filter(Boolean)) {
        result.add(graphifyEdgeResultKey(match[1], relation, match[3]));
      }
    }
  }
  return result;
};

const verifyGraphifyQueries = async (graph, contract, options) => {
  const errors = [];
  const nodesByID = new Map(
    asArray(graph?.nodes).map(node => [stringValue(node?.id), node]),
  );
  const linksByID = new Map(
    graphLinks(graph).map(link => [stringValue(link?.id), link]),
  );
  const queryRunner = options.queryRunner ?? defaultQueryRunner;
  const pathRunner =
    options.pathRunner ??
    (options.queryRunner ? options.queryRunner : defaultPathRunner);
  for (const query of asArray(contract?.required_queries)) {
    const queryID = stringValue(query?.id) || '<missing-query-id>';
    const terms = asArray(query?.expanded_terms)
      .map(stringValue)
      .filter(Boolean);
    if (terms.length === 0) {
      errors.push(`graphify_query_terms_missing: ${queryID}`);
      continue;
    }
    const smokeNodeIDs =
      asArray(query?.smoke_node_ids).length > 0
        ? query.smoke_node_ids
        : asArray(query?.required_node_ids).slice(0, 1);
    const intentLabels = [
      stringValue(query?.question),
      ...queryTargetNodeIDs(contract, query).map(nodeID =>
        stringValue(nodesByID.get(nodeID)?.label),
      ),
    ].filter(Boolean);
    const intentEdges = asArray(query?.required_edge_ids)
      .map(edgeID => linksByID.get(edgeID))
      .filter(Boolean)
      .map(edge => ({
        relation: stringValue(edge?.relation),
        sourceLabel: stringValue(
          nodesByID.get(endpointID(edge?.source))?.label,
        ),
        targetLabel: stringValue(
          nodesByID.get(endpointID(edge?.target))?.label,
        ),
      }));
    try {
      const output = await queryRunner({
        graphPath: options.queryGraphPath,
        graphifyBinary: options.graphifyBinary ?? 'graphify',
        mode: 'intent',
        query,
        requiredEdges: intentEdges,
        requiredLabels: intentLabels,
        terms: stringValue(query?.question),
      });
      const resultLabels = graphifyResultLabels(output);
      const missingLabels = intentLabels.filter(
        label => !resultLabels.has(label.toLowerCase()),
      );
      if (missingLabels.length > 0) {
        errors.push(
          `graphify_query_intent_failed: ${queryID}: missing ${missingLabels.join(', ')}`,
        );
      }
    } catch (error) {
      errors.push(
        `graphify_query_intent_failed: ${queryID}: ${stringValue(error?.message) || 'query failed'}`,
      );
    }

    for (const edge of intentEdges) {
      try {
        const output = await pathRunner({
          graphPath: options.graphPath,
          graphifyBinary: options.graphifyBinary ?? 'graphify',
          mode: 'edge',
          query,
          requiredEdges: [edge],
          requiredLabels: [],
          sourceLabel: edge.sourceLabel,
          targetLabel: edge.targetLabel,
          terms: `${edge.sourceLabel} -> ${edge.targetLabel}`,
        });
        const expectedKey = graphifyEdgeResultKey(
          edge.sourceLabel,
          edge.relation,
          edge.targetLabel,
        );
        if (!graphifyResultEdges(output).has(expectedKey)) {
          errors.push(
            `graphify_query_intent_failed: ${queryID}: missing edge ${edge.sourceLabel} -[${edge.relation}]-> ${edge.targetLabel}`,
          );
        }
      } catch (error) {
        errors.push(
          `graphify_query_intent_failed: ${queryID}: edge path failed: ${stringValue(error?.message) || 'query failed'}`,
        );
      }
    }

    const smokeLabels = smokeNodeIDs
      .map(nodeID => stringValue(nodesByID.get(nodeID)?.label))
      .filter(Boolean);
    for (const label of smokeLabels) {
      const exactTerm = terms.find(term => term === label);
      if (!exactTerm) {
        errors.push(
          `graphify_query_smoke_failed: ${queryID}: exact expanded term missing for ${label}`,
        );
        continue;
      }
      try {
        const output = await queryRunner({
          graphPath: options.graphPath,
          graphifyBinary: options.graphifyBinary ?? 'graphify',
          mode: 'anchor',
          query,
          requiredLabels: [label],
          terms: exactTerm,
        });
        const resultLabels = graphifyResultLabels(output);
        if (!resultLabels.has(label.toLowerCase())) {
          errors.push(
            `graphify_query_smoke_failed: ${queryID}: missing ${label}`,
          );
        }
      } catch (error) {
        errors.push(
          `graphify_query_smoke_failed: ${queryID}: ${stringValue(error?.message) || 'query failed'}`,
        );
      }
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
  const queryGraphPath = path.join(
    derivedRoot,
    'graphify-out',
    'query-graph.json',
  );
  const metadataPath = path.join(derivedRoot, 'build-meta.json');

  let graph;
  let queryGraph;
  let metadata;
  try {
    const derivedRootStat = await lstat(derivedRoot);
    let managedRoot;
    if (derivedRootStat.isSymbolicLink()) {
      const target = path.resolve(
        path.dirname(derivedRoot),
        await readlink(derivedRoot),
      );
      managedRoot = await realpath(
        await resolveManagedVersionTarget(
          derivedRoot,
          target,
          'derived_pointer',
        ),
      );
    } else {
      await assertManagedBuildRoot(derivedRoot);
      managedRoot = await realpath(derivedRoot);
    }
    await assertDerivedFilesContained(managedRoot, [
      graphPath,
      queryGraphPath,
      metadataPath,
    ]);
    await verifyPreviousPointer(derivedRoot);
    [graph, queryGraph, metadata] = await Promise.all([
      readFile(graphPath, 'utf8').then(JSON.parse),
      readFile(queryGraphPath, 'utf8').then(JSON.parse),
      readFile(metadataPath, 'utf8').then(JSON.parse),
    ]);
  } catch (error) {
    return {
      errors: [`derived_graph_unreadable: ${error.message}`],
      warnings,
    };
  }

  try {
    const currentBuilderDigest = await computeBuilderDigest(options.repoRoot);
    if (metadata?.builder_digest !== currentBuilderDigest) {
      errors.push(
        `stale_builder_digest: built ${stringValue(metadata?.builder_digest) || '<missing>'} current ${currentBuilderDigest}`,
      );
    }
  } catch (error) {
    errors.push(`builder_digest_failed: ${error.message}`);
  }

  try {
    const currentGitProvenance =
      options?.gitProvenance ?? (await readGitProvenance(options.repoRoot));
    for (const field of ['git_commit', 'git_dirty', 'git_status_digest']) {
      if (metadata?.[field] !== currentGitProvenance[field]) {
        errors.push(
          `stale_${field}: built ${String(metadata?.[field] ?? '<missing>')} current ${String(currentGitProvenance[field])}`,
        );
      }
    }
  } catch (error) {
    errors.push(`git_provenance_failed: ${error.message}`);
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
    if (!sameStringArray(metadata?.corpus_files, current.files)) {
      errors.push('stale_derived_corpus_files');
    }
  } catch (error) {
    errors.push(`derived_digest_failed: ${error.message}`);
  } finally {
    await rm(digestRoot, { recursive: true, force: true });
  }

  errors.push(
    ...verifyGraphHealth(graph, contract, options?.resolvedVersions ?? {}),
  );
  errors.push(
    ...verifyGraphHealth(queryGraph, contract, options?.resolvedVersions ?? {}),
  );
  errors.push(...verifyDerivedStructure(graph, queryGraph, contract, metadata));
  errors.push(...verifyRequiredQueries(graph, contract?.required_queries));
  errors.push(...verifyOrderedPaths(graph, contract?.chains));
  if (options?.runGraphifyQueries !== false) {
    errors.push(
      ...(await verifyGraphifyQueries(graph, contract, {
        ...options,
        graphPath,
        queryGraphPath,
      })),
    );
  }

  return {
    errors: [...new Set(errors)].sort(),
    warnings: [...new Set(warnings)].sort(),
  };
};

export const graphifyExtractArgs = (corpusDir, outputDir) => [
  'extract',
  corpusDir,
  '--out',
  outputDir,
  '--code-only',
  '--no-cluster',
  '--no-gitignore',
  '--max-workers',
  '1',
];

const AST_CODE_PATH_PATTERN =
  /\.(?:c|cc|cpp|cs|go|java|js|jsx|mjs|cjs|php|py|rb|rs|swift|ts|tsx)$/i;

export const astEligibleCorpusFiles = corpusFiles =>
  stableUnique(
    asArray(corpusFiles).filter(relativePath => {
      const normalized = stringValue(relativePath).replaceAll('\\', '/');
      const basename = path.posix.basename(normalized);
      return (
        AST_CODE_PATH_PATTERN.test(normalized) ||
        basename === 'go.mod' ||
        basename === 'package.json'
      );
    }),
  );

const astSourceFiles = graph =>
  new Set(
    [...asArray(graph?.nodes), ...graphLinks(graph)]
      .map(item => stringValue(item?.source_file).replaceAll('\\', '/'))
      .filter(Boolean),
  );

export const assertNonEmptyASTGraph = (graph, corpusFiles) => {
  const hasCode = astEligibleCorpusFiles(corpusFiles).length > 0;
  if (
    hasCode &&
    (asArray(graph?.nodes).length === 0 || graphLinks(graph).length === 0)
  ) {
    const error = new Error(
      'graphify_ast_empty: code corpus produced zero AST nodes or links',
    );
    error.code = 'graphify_ast_empty';
    throw error;
  }
};

export const assertASTSourceCoverage = (graph, corpusFiles) => {
  const actual = astSourceFiles(graph);
  const missing = astEligibleCorpusFiles(corpusFiles).filter(
    relativePath => !actual.has(relativePath),
  );
  if (missing.length > 0) {
    const error = new Error(
      `graphify_ast_source_missing: ${missing.join(', ')}`,
    );
    error.code = 'graphify_ast_source_missing';
    throw error;
  }
};

const runGraphifyExtraction = async (corpusDir, outputDir, graphifyBinary) => {
  try {
    await execFileAsync(
      graphifyBinary,
      graphifyExtractArgs(corpusDir, outputDir),
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

export const readGitProvenance = async repoRoot => {
  let gitCommit = 'unknown';
  let gitStatus = 'git-status-unavailable';
  try {
    const { stdout } = await execFileAsync('git', ['rev-parse', 'HEAD'], {
      cwd: repoRoot,
      encoding: 'utf8',
    });
    gitCommit = stringValue(stdout) || 'unknown';
  } catch {
    // The explicit unknown value keeps provenance fail-closed outside Git.
  }
  try {
    const { stdout } = await execFileAsync(
      'git',
      ['status', '--porcelain=v1', '--untracked-files=all'],
      { cwd: repoRoot, encoding: 'utf8' },
    );
    gitStatus = stdout.trim();
  } catch {
    // The unavailable sentinel is dirty and cannot be mistaken for a clean tree.
  }
  return {
    git_commit: gitCommit,
    git_dirty: Boolean(gitStatus),
    git_status_digest: createHash('sha256').update(gitStatus).digest('hex'),
  };
};

export const installCompletedBuild = async (temporaryRoot, derivedRoot) => {
  const parent = path.dirname(derivedRoot);
  const versionsRoot = `${derivedRoot}-versions`;
  const previousPointerRoot = `${derivedRoot}-previous`;
  await assertManagedBuildRoot(temporaryRoot);
  try {
    const versionsStat = await lstat(versionsRoot);
    if (!versionsStat.isDirectory()) {
      throw new Error(`derived_versions_root_invalid: ${versionsRoot}`);
    }
  } catch (error) {
    if (error?.code !== 'ENOENT') {
      throw error;
    }
    await mkdir(versionsRoot, { recursive: true });
  }
  const buildIdentity = createHash('sha256')
    .update(`${temporaryRoot}\u0000${Date.now()}\u0000${process.pid}`)
    .digest('hex')
    .slice(0, 12);
  const versionRoot = path.join(versionsRoot, `build-${buildIdentity}`);
  const pointerRoot = path.join(
    parent,
    `.${path.basename(derivedRoot)}-pointer-${buildIdentity}`,
  );
  const previousPointerTemp = path.join(
    parent,
    `.${path.basename(derivedRoot)}-previous-${buildIdentity}`,
  );
  const previousRestoreTemp = path.join(
    parent,
    `.${path.basename(derivedRoot)}-previous-restore-${buildIdentity}`,
  );
  let previousVersionRoot = '';
  let priorPreviousVersionRoot = '';
  let previousPointerUpdated = false;
  let legacyMoved = false;

  try {
    const previousStat = await lstat(previousPointerRoot);
    if (!previousStat.isSymbolicLink()) {
      throw new Error(
        `derived_previous_pointer_invalid: ${previousPointerRoot}`,
      );
    }
    const previousTarget = path.resolve(
      parent,
      await readlink(previousPointerRoot),
    );
    priorPreviousVersionRoot = await resolveManagedVersionTarget(
      derivedRoot,
      previousTarget,
      'derived_previous_pointer',
    );
  } catch (error) {
    if (error?.code !== 'ENOENT') {
      throw error;
    }
  }

  await rename(temporaryRoot, versionRoot);
  try {
    try {
      const current = await lstat(derivedRoot);
      if (current.isSymbolicLink()) {
        const currentTarget = await readlink(derivedRoot);
        const resolvedTarget = path.resolve(parent, currentTarget);
        previousVersionRoot = await resolveManagedVersionTarget(
          derivedRoot,
          resolvedTarget,
          'derived_pointer',
        );
      } else if (current.isDirectory()) {
        await assertManagedBuildRoot(derivedRoot);
        previousVersionRoot = path.join(
          versionsRoot,
          `legacy-${buildIdentity}`,
        );
        await rename(derivedRoot, previousVersionRoot);
        legacyMoved = true;
      } else {
        throw new Error(`derived_publish_target_invalid: ${derivedRoot}`);
      }
    } catch (error) {
      if (error?.code !== 'ENOENT') {
        throw error;
      }
    }

    if (previousVersionRoot) {
      await symlink(
        path.relative(parent, previousVersionRoot),
        previousPointerTemp,
        'dir',
      );
      await rename(previousPointerTemp, previousPointerRoot);
      previousPointerUpdated = true;
    }

    const relativeTarget = path.relative(parent, versionRoot);
    await symlink(relativeTarget, pointerRoot, 'dir');
    await rename(pointerRoot, derivedRoot);
  } catch (error) {
    const rollbackErrors = [];
    const rollback = async operation => {
      try {
        await operation();
      } catch (rollbackError) {
        rollbackErrors.push(rollbackError);
      }
    };
    await rollback(() => rm(pointerRoot, { force: true }));
    await rollback(() => rm(previousPointerTemp, { force: true }));
    if (previousPointerUpdated) {
      if (priorPreviousVersionRoot) {
        await rollback(async () => {
          await symlink(
            path.relative(parent, priorPreviousVersionRoot),
            previousRestoreTemp,
            'dir',
          );
          await rename(previousRestoreTemp, previousPointerRoot);
        });
      } else {
        await rollback(() => rm(previousPointerRoot, { force: true }));
      }
    }
    await rollback(() => rm(previousRestoreTemp, { force: true }));
    await rollback(() => rm(versionRoot, { recursive: true, force: true }));
    if (legacyMoved) {
      await rollback(() => rename(previousVersionRoot, derivedRoot));
    }
    if (rollbackErrors.length > 0) {
      const rollbackFailure = new AggregateError(
        [error, ...rollbackErrors],
        `derived_publish_rollback_failed: ${error.message}`,
      );
      rollbackFailure.code = 'derived_publish_rollback_failed';
      rollbackFailure.cause = error;
      throw rollbackFailure;
    }
    throw error;
  }
};

export const buildDerivedGraph = async (contract, options) => {
  const derivedRoot = path.resolve(
    options?.derivedRoot ?? path.join(options.repoRoot, DERIVED_RELATIVE_ROOT),
  );
  const parent = path.dirname(derivedRoot);
  const gitProvenance =
    options?.gitProvenance ?? (await readGitProvenance(options.repoRoot));
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
    if (typeof options?.graphifyExtractor === 'function') {
      await options.graphifyExtractor({
        corpusDir: corpus.corpusDir,
        corpusFiles: corpus.files,
        outputDir: temporaryRoot,
        graphifyBinary,
      });
    } else {
      await runGraphifyExtraction(
        corpus.corpusDir,
        temporaryRoot,
        graphifyBinary,
      );
    }
    const graphPath = path.join(temporaryRoot, 'graphify-out', 'graph.json');
    const astGraph = JSON.parse(await readFile(graphPath, 'utf8'));
    assertNonEmptyASTGraph(astGraph, corpus.files);
    assertASTSourceCoverage(astGraph, corpus.files);
    const mergedGraph = mergeExplicitGraph(
      astGraph,
      contract,
      options?.resolvedVersions,
    );
    const queryGraph = mergeExplicitGraph(
      astGraph,
      contract,
      options?.resolvedVersions,
      { includeQueryOverlay: true },
    );
    const queryGraphPath = path.join(
      temporaryRoot,
      'graphify-out',
      'query-graph.json',
    );
    await writeFile(
      graphPath,
      `${JSON.stringify(mergedGraph, null, 2)}\n`,
      'utf8',
    );
    await writeFile(
      queryGraphPath,
      `${JSON.stringify(queryGraph, null, 2)}\n`,
      'utf8',
    );

    const [graphifyVersion, builderDigest] = await Promise.all([
      commandOutput(graphifyBinary, ['--version'], options.repoRoot, 'unknown'),
      computeBuilderDigest(options.repoRoot),
    ]);
    const metadata = {
      schema_version: contract.schema_version,
      ...gitProvenance,
      corpus_digest: corpus.digest,
      builder_digest: builderDigest,
      corpus_files: corpus.files,
      ...derivedBuildStats(mergedGraph, queryGraph),
      graphify_version: graphifyVersion,
      generated_at: new Date().toISOString(),
    };
    await writeFile(
      path.join(temporaryRoot, 'build-meta.json'),
      `${JSON.stringify(metadata, null, 2)}\n`,
      'utf8',
    );
    await writeManagedBuildMarker(temporaryRoot);
    const verification = await verifyDerivedGraph(contract, {
      ...options,
      derivedRoot: temporaryRoot,
      gitProvenance,
    });
    if (verification.errors.length > 0) {
      const error = new Error(
        `derived_build_verification_failed: ${verification.errors.join('; ')}`,
      );
      error.code = 'derived_build_verification_failed';
      throw error;
    }
    await installCompletedBuild(temporaryRoot, derivedRoot);
    return {
      derivedRoot,
      graphPath: path.join(derivedRoot, 'graphify-out', 'graph.json'),
      queryGraphPath: path.join(
        derivedRoot,
        'graphify-out',
        'query-graph.json',
      ),
      metadata,
    };
  } catch (error) {
    await rm(temporaryRoot, { recursive: true, force: true });
    throw error;
  }
};
