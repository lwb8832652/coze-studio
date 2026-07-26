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

import { readFile } from 'node:fs/promises';
import path from 'node:path';

export const CURRENT_STATUS = 'current';

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

  return {
    errors: errors.sort(),
    warnings: warnings.sort(),
    resolvedVersions: {},
  };
};
