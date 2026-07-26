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
import { readFile } from 'node:fs/promises';
import path from 'node:path';
import { promisify } from 'node:util';

export const CURRENT_STATUS = 'current';

const execFileAsync = promisify(execFile);

const FRAMEWORK_LAYERS = new Set(['framework', 'framework_extension']);
const ALLOWED_RUNTIME_SCOPES = new Set([
  'canonical_runtime',
  'conditional_runtime_extension',
  'transport_contract',
  'persistence_runtime',
  'integration_ingress',
  'optional_observability',
  'ui_only',
  'compatibility_contract',
  'historical_compatibility',
  'build_or_test_only',
]);
const NONCANONICAL_EXECUTOR_SCOPES = new Set([
  'conditional_runtime_extension',
  'compatibility_contract',
  'historical_compatibility',
  'ui_only',
  'optional_observability',
  'build_or_test_only',
]);

const ALLOWED_RELATIONS = new Set([
  'routes_to',
  'calls',
  'delegates_to',
  'persists_via',
  'claims',
  'executes',
  'emits',
  'maps_to',
  'streams_to',
  'projects_to',
  'cancels',
  'resumes',
  'retries',
  'audits',
  'implemented_with',
  'configures',
  'adapts_to',
  'observes',
  'excludes',
  'precedes',
]);

const asArray = value => (Array.isArray(value) ? value : []);

const stringValue = value =>
  typeof value === 'string' ? value.trim() : '';

const isFrameworkNode = node => FRAMEWORK_LAYERS.has(node?.layer);

const readRepoFile = async (repoRoot, relativePath) =>
  readFile(normalizeRepoPath(repoRoot, relativePath), 'utf8');

const regexEscape = value =>
  value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');

const duplicateErrors = (items, type) => {
  const seen = new Set();
  const errors = [];
  for (const item of items) {
    const id = stringValue(item?.id);
    if (!id) {
      errors.push(`${type}_id_missing: ${type} requires a stable id`);
      continue;
    }
    if (seen.has(id)) {
      errors.push(`duplicate_${type}_id: ${id}`);
    }
    seen.add(id);
  }
  return errors;
};

export const normalizeRepoPath = (repoRoot, candidate) => {
  const root = path.resolve(repoRoot);
  const raw = stringValue(candidate);
  if (!raw || path.isAbsolute(raw)) {
    throw new Error('repository path must be non-empty and relative');
  }

  const absolute = path.resolve(root, raw);
  const relative = path.relative(root, absolute);
  if (relative === '..' || relative.startsWith(`..${path.sep}`)) {
    throw new Error('repository path escapes the repository root');
  }

  return absolute;
};

export const resolveVersion = async (node, options) => {
  const source = node?.version_source;
  const sourceType = stringValue(source?.type);
  if (sourceType === 'platform') {
    return stringValue(node?.version);
  }

  const content = await readRepoFile(options.repoRoot, source?.path);
  switch (sourceType) {
    case 'json_dependency': {
      const manifest = JSON.parse(content);
      const dependencyName = stringValue(source?.name);
      for (const section of [
        'dependencies',
        'devDependencies',
        'peerDependencies',
        'optionalDependencies',
      ]) {
        const version = stringValue(manifest?.[section]?.[dependencyName]);
        if (version) {
          return version;
        }
      }
      throw new Error(`dependency ${dependencyName} is missing`);
    }
    case 'go_module': {
      const moduleName = stringValue(source?.name);
      const match = content.match(
        new RegExp(`^\\s*${regexEscape(moduleName)}\\s+(\\S+)`, 'm'),
      );
      if (!match) {
        throw new Error(`Go module ${moduleName} is missing`);
      }
      return match[1];
    }
    case 'go_directive': {
      const match = content.match(/^go\s+(\S+)/m);
      if (!match) {
        throw new Error('Go directive is missing');
      }
      return match[1];
    }
    case 'text_pattern': {
      const pattern = stringValue(source?.pattern);
      if (!pattern) {
        throw new Error('text pattern is missing');
      }
      const match = content.match(new RegExp(pattern, 'm'));
      if (!match) {
        throw new Error(`text pattern did not match: ${pattern}`);
      }
      const group = Number.isInteger(source?.group) ? source.group : 0;
      if (match[group] === undefined) {
        throw new Error(`text pattern capture group ${group} is missing`);
      }
      return stringValue(match[group]);
    }
    default:
      throw new Error(`unsupported version source type: ${sourceType}`);
  }
};

export const validateFrameworkScopes = contract => {
  const errors = [];
  const nodes = asArray(contract?.nodes);
  const nodesByID = new Map(nodes.map(node => [node?.id, node]));

  for (const node of nodes) {
    const nodeID = stringValue(node?.id) || '<missing-node-id>';
    const runtimeScope = stringValue(node?.runtime_scope);
    if (runtimeScope && !ALLOWED_RUNTIME_SCOPES.has(runtimeScope)) {
      errors.push(`framework_runtime_scope_invalid: ${nodeID}: ${runtimeScope}`);
    }
    if (!isFrameworkNode(node)) {
      continue;
    }
    if (!runtimeScope) {
      errors.push(`framework_runtime_scope_missing: ${nodeID}`);
    }
    if (!stringValue(node?.package)) {
      errors.push(`framework_package_missing: ${nodeID}`);
    }
    if (!stringValue(node?.version)) {
      errors.push(`framework_version_missing: ${nodeID}`);
    }
    if (!node?.version_source || !stringValue(node.version_source.type)) {
      errors.push(`framework_version_source_missing: ${nodeID}`);
    }

    if (runtimeScope === 'canonical_runtime') {
      const versionPath = stringValue(node?.version_source?.path);
      const productionAnchor = asArray(node?.source_anchors).some(anchor => {
        const anchorPath = stringValue(anchor?.path);
        return (
          anchorPath &&
          anchorPath !== versionPath &&
          /\.(?:go|[cm]?[jt]sx?)$/.test(anchorPath)
        );
      });
      if (!productionAnchor) {
        errors.push(`canonical_framework_anchor_missing: ${nodeID}`);
      }
    }
  }

  for (const chain of asArray(contract?.chains)) {
    if (chain?.canonical_executor !== true) {
      continue;
    }
    for (const nodeID of asArray(chain?.ordered_node_ids)) {
      const runtimeScope = stringValue(nodesByID.get(nodeID)?.runtime_scope);
      if (NONCANONICAL_EXECUTOR_SCOPES.has(runtimeScope)) {
        errors.push(
          `noncanonical_runtime_in_execution_chain: ${chain.id}: ${nodeID}: ${runtimeScope}`,
        );
      }
    }
  }

  return errors;
};

const validateForbiddenCurrentNodes = contract => {
  const errors = [];
  const forbiddenTerms = asArray(contract?.scope?.forbidden_current_terms)
    .map(term => stringValue(term).toLowerCase())
    .filter(Boolean);

  for (const node of asArray(contract?.nodes)) {
    if (node?.production_status !== CURRENT_STATUS) {
      continue;
    }
    const searchable = `${stringValue(node?.id)} ${stringValue(node?.label)}`.toLowerCase();
    for (const term of forbiddenTerms) {
      if (searchable.includes(term)) {
        errors.push(`forbidden_current_node: ${node.id}: ${term}`);
      }
    }
  }

  return errors;
};

export const validateMiddlewareOrder = async (contract, options) => {
  const chain = asArray(contract?.chains).find(
    item => item?.id === 'framework.eino_middleware_order',
  );
  if (!chain) {
    return [];
  }

  const middlewareNodes = asArray(contract?.nodes).filter(
    node => node?.layer === 'eino_middleware',
  );
  const sourcePath = stringValue(middlewareNodes[0]?.source_anchors?.[0]?.path);
  if (!sourcePath) {
    return ['middleware_order_source_missing: framework.eino_middleware_order'];
  }

  let content;
  try {
    content = await readRepoFile(options.repoRoot, sourcePath);
  } catch (error) {
    return [`middleware_order_source_invalid: ${error.message}`];
  }
  const block = content.match(
    /var\s+adkMiddlewareOrder\s*=\s*\[\]ADKMiddlewareName\s*{([\s\S]*?)\n}/,
  );
  if (!block) {
    return ['middleware_order_source_invalid: adkMiddlewareOrder block is missing'];
  }

  const nodeByConstant = new Map();
  for (const node of middlewareNodes) {
    for (const anchor of asArray(node?.source_anchors)) {
      const locator = stringValue(anchor?.locator);
      const match = locator.match(/^(ADKMiddleware\w+),$/);
      if (match) {
        nodeByConstant.set(match[1], node.id);
      }
    }
  }
  const constants = [...block[1].matchAll(/^\s*(ADKMiddleware\w+),\s*$/gm)].map(
    match => match[1],
  );
  const expected = constants.map(constant => nodeByConstant.get(constant));
  if (expected.some(nodeID => !nodeID)) {
    return [
      `middleware_order_source_invalid: unmapped constants: ${constants
        .filter((constant, index) => !expected[index])
        .join(',')}`,
    ];
  }
  if (
    JSON.stringify(expected) !== JSON.stringify(asArray(chain.ordered_node_ids))
  ) {
    return [
      `middleware_order_mismatch: expected ${expected.join(' -> ')}`,
    ];
  }

  return [];
};

const globMatches = (candidate, pattern) => {
  const normalizedCandidate = candidate.replaceAll('\\', '/');
  const normalizedPattern = stringValue(pattern).replaceAll('\\', '/');
  let source = '^';
  for (let index = 0; index < normalizedPattern.length; index += 1) {
    const character = normalizedPattern[index];
    if (character === '*' && normalizedPattern[index + 1] === '*') {
      source += '.*';
      index += 1;
    } else if (character === '*') {
      source += '[^/]*';
    } else if (character === '?') {
      source += '[^/]';
    } else {
      source += regexEscape(character);
    }
  }
  return new RegExp(`${source}$`).test(normalizedCandidate);
};

export const evaluateAuthorityChanges = (changedPaths, scope) => {
  const changed = new Set(
    asArray(changedPaths).map(candidate => stringValue(candidate).replaceAll('\\', '/')),
  );
  const authorityFiles = asArray(scope?.authority_files).map(candidate =>
    stringValue(candidate).replaceAll('\\', '/'),
  );
  const changedAuthorityCount = authorityFiles.filter(candidate =>
    changed.has(candidate),
  ).length;
  if (
    changedAuthorityCount > 0 &&
    changedAuthorityCount !== authorityFiles.length
  ) {
    return ['authority_files_must_change_together'];
  }

  const monitoredChanged = [...changed].some(candidate =>
    asArray(scope?.monitored_paths).some(pattern => globMatches(candidate, pattern)),
  );
  if (monitoredChanged && changedAuthorityCount === 0) {
    return ['authority_files_not_updated'];
  }

  return [];
};

export const gitChangedPaths = async (repoRoot, baseRef) => {
  const commands = [
    ['diff', '--name-only', '--diff-filter=ACMR'],
    ['diff', '--cached', '--name-only', '--diff-filter=ACMR'],
  ];
  if (stringValue(baseRef)) {
    commands.push([
      'diff',
      '--name-only',
      '--diff-filter=ACMR',
      `${baseRef}...HEAD`,
    ]);
  }

  const paths = new Set();
  for (const args of commands) {
    const { stdout } = await execFileAsync('git', args, {
      cwd: repoRoot,
      encoding: 'utf8',
    });
    for (const candidate of stdout.split(/\r?\n/)) {
      const normalized = stringValue(candidate).replaceAll('\\', '/');
      if (normalized) {
        paths.add(normalized);
      }
    }
  }
  return [...paths].sort();
};

export const validateSourceAnchor = async (anchor, options) => {
  const errors = [];
  const owner = stringValue(options?.owner) || 'anchor';
  const relativePath = stringValue(anchor?.path);
  const locator = stringValue(anchor?.locator);

  let absolutePath;
  try {
    absolutePath = normalizeRepoPath(options.repoRoot, relativePath);
  } catch (error) {
    errors.push(`anchor_path_outside_repo: ${owner}: ${error.message}`);
    return errors;
  }

  let content;
  try {
    content = await readFile(absolutePath, 'utf8');
  } catch {
    errors.push(`anchor_path_missing: ${owner}: ${relativePath}`);
    return errors;
  }

  if (!locator) {
    errors.push(`anchor_locator_missing: ${owner}: ${relativePath}`);
  } else if (!content.includes(locator)) {
    errors.push(`anchor_locator_not_found: ${owner}: ${relativePath}: ${locator}`);
  }

  return errors;
};

const validateEvidenceList = async (evidence, options) => {
  const errors = [];
  for (const [index, anchor] of asArray(evidence).entries()) {
    errors.push(
      ...(await validateSourceAnchor(anchor, {
        ...options,
        owner: `${options.owner}.evidence[${index}]`,
      })),
    );
  }
  return errors;
};

export const validateOrderedChain = (chain, nodesByID, edgesByID) => {
  const errors = [];
  const chainID = stringValue(chain?.id) || '<missing-chain-id>';
  const nodeIDs = asArray(chain?.ordered_node_ids);
  const edgeIDs = asArray(chain?.ordered_edge_ids);

  if (nodeIDs.length < 2) {
    errors.push(`chain_nodes_missing: ${chainID}`);
    return errors;
  }
  if (edgeIDs.length !== nodeIDs.length - 1) {
    errors.push(`chain_edge_count_invalid: ${chainID}`);
  }

  for (const nodeID of nodeIDs) {
    const node = nodesByID.get(nodeID);
    if (!node) {
      errors.push(`chain_node_missing: ${chainID}: ${nodeID}`);
    } else if (node.production_status !== CURRENT_STATUS) {
      errors.push(`chain_node_not_current: ${chainID}: ${nodeID}`);
    }
  }

  for (const [index, edgeID] of edgeIDs.entries()) {
    const edge = edgesByID.get(edgeID);
    if (!edge) {
      errors.push(`chain_edge_missing: ${chainID}: ${edgeID}`);
      continue;
    }
    const expectedFrom = nodeIDs[index];
    const expectedTo = nodeIDs[index + 1];
    if (edge.from !== expectedFrom || edge.to !== expectedTo) {
      errors.push(
        `chain_edge_order_invalid: ${chainID}: ${edgeID}: expected ${expectedFrom} -> ${expectedTo}`,
      );
    }
    if (!asArray(edge.chain_ids).includes(chainID)) {
      errors.push(`chain_edge_membership_missing: ${chainID}: ${edgeID}`);
    }
  }

  if (chain.entry !== nodeIDs[0]) {
    errors.push(`chain_entry_mismatch: ${chainID}`);
  }
  if (chain.terminal !== nodeIDs.at(-1)) {
    errors.push(`chain_terminal_mismatch: ${chainID}`);
  }

  return errors;
};

export const validateContract = async (contract, options) => {
  const errors = [];
  const warnings = [];
  const resolvedVersions = {};
  const repoRoot = options?.repoRoot;

  if (!repoRoot) {
    return {
      errors: ['repo_root_missing: repository root is required'],
      warnings,
      resolvedVersions: {},
    };
  }
  if (contract?.schema_version !== 1) {
    errors.push(`schema_version_unsupported: ${String(contract?.schema_version)}`);
  }

  const nodes = asArray(contract?.nodes);
  const edges = asArray(contract?.edges);
  const chains = asArray(contract?.chains);
  errors.push(...duplicateErrors(nodes, 'node'));
  errors.push(...duplicateErrors(edges, 'edge'));
  errors.push(...duplicateErrors(chains, 'chain'));
  errors.push(...validateFrameworkScopes(contract));
  errors.push(...validateForbiddenCurrentNodes(contract));

  const nodesByID = new Map();
  for (const node of nodes) {
    const nodeID = stringValue(node?.id);
    if (nodeID && !nodesByID.has(nodeID)) {
      nodesByID.set(nodeID, node);
    }
    if (node?.production_status !== CURRENT_STATUS) {
      warnings.push(`node_not_current: ${nodeID || '<missing-node-id>'}`);
    }
    const sourceAnchors = asArray(node?.source_anchors);
    if (sourceAnchors.length === 0) {
      errors.push(`node_source_anchor_missing: ${nodeID || '<missing-node-id>'}`);
    }
    for (const [index, anchor] of sourceAnchors.entries()) {
      errors.push(
        ...(await validateSourceAnchor(anchor, {
          repoRoot,
          owner: `node.${nodeID}.source_anchors[${index}]`,
        })),
      );
    }
    if (asArray(node?.evidence).length === 0) {
      errors.push(`node_evidence_missing: ${nodeID || '<missing-node-id>'}`);
    }
    errors.push(
      ...(await validateEvidenceList(node?.evidence, {
        repoRoot,
        owner: `node.${nodeID}`,
      })),
    );

    if (isFrameworkNode(node)) {
      try {
        const resolvedVersion = await resolveVersion(node, { repoRoot });
        resolvedVersions[nodeID] = resolvedVersion;
        if (resolvedVersion !== stringValue(node?.version)) {
          errors.push(
            `framework_version_mismatch: ${nodeID}: declared ${stringValue(node?.version)} resolved ${resolvedVersion}`,
          );
        }
      } catch (error) {
        errors.push(`framework_version_source_invalid: ${nodeID}: ${error.message}`);
      }
    }
  }

  const edgesByID = new Map();
  for (const edge of edges) {
    const edgeID = stringValue(edge?.id);
    if (edgeID && !edgesByID.has(edgeID)) {
      edgesByID.set(edgeID, edge);
    }
    if (!nodesByID.has(edge?.from)) {
      errors.push(`edge_source_missing: ${edgeID || '<missing-edge-id>'}: ${String(edge?.from)}`);
    }
    if (!nodesByID.has(edge?.to)) {
      errors.push(`edge_target_missing: ${edgeID || '<missing-edge-id>'}: ${String(edge?.to)}`);
    }
    if (!ALLOWED_RELATIONS.has(edge?.relation)) {
      errors.push(`edge_relation_invalid: ${edgeID || '<missing-edge-id>'}: ${String(edge?.relation)}`);
    }
    if (edge?.confidence !== 'extracted') {
      errors.push(`edge_confidence_invalid: ${edgeID || '<missing-edge-id>'}`);
    }
    if (asArray(edge?.chain_ids).length === 0) {
      errors.push(`edge_chain_ids_missing: ${edgeID || '<missing-edge-id>'}`);
    }
    if (asArray(edge?.evidence).length === 0) {
      errors.push(`edge_evidence_missing: ${edgeID || '<missing-edge-id>'}`);
    }
    errors.push(
      ...(await validateEvidenceList(edge?.evidence, {
        repoRoot,
        owner: `edge.${edgeID}`,
      })),
    );
  }

  for (const chain of chains) {
    const chainID = stringValue(chain?.id);
    if (chain?.production_status !== CURRENT_STATUS) {
      errors.push(`chain_not_current: ${chainID || '<missing-chain-id>'}`);
    }
    if (asArray(chain?.test_evidence).length === 0) {
      errors.push(`chain_test_evidence_missing: ${chainID || '<missing-chain-id>'}`);
    }
    errors.push(
      ...(await validateEvidenceList(chain?.test_evidence, {
        repoRoot,
        owner: `chain.${chainID}`,
      })),
    );
    errors.push(...validateOrderedChain(chain, nodesByID, edgesByID));
  }

  errors.push(...(await validateMiddlewareOrder(contract, { repoRoot })));

  if (Array.isArray(options?.changedPaths)) {
    errors.push(
      ...evaluateAuthorityChanges(options.changedPaths, {
        ...contract?.scope,
        authority_files: [
          contract?.authority?.contract,
          contract?.authority?.document,
        ],
      }),
    );
  }

  return {
    errors: errors.sort(),
    warnings: warnings.sort(),
    resolvedVersions,
  };
};
