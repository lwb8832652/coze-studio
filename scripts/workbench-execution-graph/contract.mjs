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
import { readFile } from 'node:fs/promises';
import path from 'node:path';
import { promisify } from 'node:util';

export const CURRENT_STATUS = 'current';

const CANONICAL_CONTRACT_PATH =
  'docs/superpowers/context/workbench-execution-graph.json';
const WORKBENCH_PROFILE = 'workbench_execution_v1';
const WORKBENCH_PROFILE_STRUCTURE_DIGEST =
  'aaf0b0af87ba8b667715ce021576c395db0dfd51ec5e9993416a49f0a3782557';
const REQUIRED_CHAIN_IDS = [
  'entry.workbench_immediate',
  'entry.workbench_deferred',
  'entry.task_detail_followup',
  'entry.scheduled_task',
  'entry.feishu_message',
  'entry.workbench_canonical_thread_http',
  'entry.workbench_canonical_run_http',
  'entry.workbench_canonical_run_stream',
  'run.atomic_create',
  'run.pending_worker_execute',
  'run.event_projection',
  'control.cancel',
  'control.human_resume',
  'control.subagent_retry',
  'control.checkpoint_resume',
  'control.lease_recovery',
  'control.multitask_rollback',
  'data.memory',
  'data.artifact',
  'data.token_usage',
  'data.guardrail_audit',
  'data.mcp_runtime_audit',
  'framework.canonical_stack',
  'framework.eino_sdk',
  'framework.eino_middleware_order',
  'boundary.runtime_compatibility',
];
const REQUIRED_QUERY_IDS = [
  'query.framework_inventory',
  'query.frontend_to_eino',
  'query.eino_sdk_execution',
  'query.middleware_order',
  'query.control_recovery',
  'query.data_capabilities',
  'query.integration_ingress',
  'query.runtime_boundaries',
  'query.canonical_thread_http',
];
const REQUIRED_EXCLUSION_IDS = [
  'exclude.k2',
  'exclude.chattask',
  'exclude.langgraph_sdk',
  'exclude.deerflow_runtime',
  'exclude.legacy_new_run',
  'exclude.unwired_adaptive_boundary',
  'exclude.external_queue',
  'exclude.build_tools',
  'exclude.taskthread_v1_routes',
  'exclude.langgraph_thread_routes',
];
const REQUIRED_NODE_IDS = [
  'frontend.client.singleton',
  'frontend.api.create_thread',
  'frontend.api.upload_files',
  'frontend.api.create_run',
  'frontend.events.run_subscription',
  'contract.workbench.route_surface',
  'contract.workbench_canonical_thread.thrift',
  'contract.workbench_scheduled_task.thrift',
  'http.workbench.canonical_thread',
  'http.workbench.canonical_run',
  'http.workbench.canonical_run_stream',
  'framework.fetch_stream',
  'compat.deerflow_config',
  'integration.scheduled.execute',
  'integration.scheduled.start_new',
  'integration.scheduled.start_in_thread',
  'integration.feishu.process_event',
  'integration.feishu.agent_execute',
  'integration.feishu.start_run',
  'application.create_thread',
  'application.create_task_thread',
  'application.create_run',
  'runtime.new_run_policy',
  'runtime.selector.execute',
  'runtime.adk_executor.execute',
  'historical.legacy_runtime',
];
const REQUIRED_EDGE_IDS = [
  'edge.create_thread_uses_singleton',
  'edge.create_run_uses_singleton',
  'edge.run_subscription_uses_singleton',
  'edge.canonical_create_thread_routes_handler',
  'edge.canonical_thread_submission_calls_create_task_thread',
  'edge.canonical_create_run_routes_handler',
  'edge.canonical_run_handler_calls_app_create_run',
  'edge.repo_event_streams_canonical',
  'edge.canonical_stream_to_subscription',
  'edge.run_subscription_uses_fetch_stream',
  'edge.canonical_route_surface_maps_contract',
  'edge.scheduled_route_surface_maps_contract',
  'edge.scheduled_execute_routes_new',
  'edge.scheduled_calls_create_thread',
  'edge.scheduled_execute_routes_existing',
  'edge.scheduled_existing_calls_create_run',
  'edge.feishu_event_calls_agent',
  'edge.feishu_agent_calls_start_run',
  'edge.feishu_start_calls_create_thread',
  'edge.feishu_start_calls_create_run',
  'edge.app_run_calls_new_run_policy',
  'edge.new_run_policy_precedes_selector',
  'edge.selector_executes_adk',
  'edge.adk_execute_delegates_adaptive_bootstrap',
  'edge.adaptive_bootstrap_precedes_agent_build',
  'edge.deerflow_configures_eino',
  'edge.legacy_is_historical_only',
  'edge.legacy_excluded_from_new_runs',
];
const REQUIRED_EDGE_SHAPES = {
  'edge.create_thread_uses_singleton': [
    'frontend.api.create_thread',
    'implemented_with',
    'frontend.client.singleton',
  ],
  'edge.create_run_uses_singleton': [
    'frontend.api.create_run',
    'implemented_with',
    'frontend.client.singleton',
  ],
  'edge.run_subscription_uses_singleton': [
    'frontend.events.run_subscription',
    'implemented_with',
    'frontend.client.singleton',
  ],
  'edge.canonical_create_thread_routes_handler': [
    'frontend.api.create_thread',
    'routes_to',
    'http.workbench.canonical_thread',
  ],
  'edge.canonical_thread_submission_calls_create_task_thread': [
    'http.workbench.canonical_thread',
    'delegates_to',
    'application.create_task_thread',
  ],
  'edge.canonical_create_run_routes_handler': [
    'frontend.api.create_run',
    'routes_to',
    'http.workbench.canonical_run',
  ],
  'edge.canonical_run_handler_calls_app_create_run': [
    'http.workbench.canonical_run',
    'delegates_to',
    'application.create_run',
  ],
  'edge.repo_event_streams_canonical': [
    'repository.create_run_event',
    'streams_to',
    'http.workbench.canonical_run_stream',
  ],
  'edge.canonical_stream_to_subscription': [
    'http.workbench.canonical_run_stream',
    'streams_to',
    'frontend.events.run_subscription',
  ],
  'edge.run_subscription_uses_fetch_stream': [
    'frontend.events.run_subscription',
    'implemented_with',
    'framework.fetch_stream',
  ],
  'edge.canonical_route_surface_maps_contract': [
    'contract.workbench.route_surface',
    'maps_to',
    'contract.workbench_canonical_thread.thrift',
  ],
  'edge.scheduled_route_surface_maps_contract': [
    'contract.workbench.route_surface',
    'maps_to',
    'contract.workbench_scheduled_task.thrift',
  ],
  'edge.scheduled_execute_routes_new': [
    'integration.scheduled.execute',
    'routes_to',
    'integration.scheduled.start_new',
  ],
  'edge.scheduled_calls_create_thread': [
    'integration.scheduled.start_new',
    'calls',
    'application.create_task_thread',
  ],
  'edge.scheduled_execute_routes_existing': [
    'integration.scheduled.execute',
    'routes_to',
    'integration.scheduled.start_in_thread',
  ],
  'edge.scheduled_existing_calls_create_run': [
    'integration.scheduled.start_in_thread',
    'calls',
    'application.create_run',
  ],
  'edge.feishu_event_calls_agent': [
    'integration.feishu.process_event',
    'calls',
    'integration.feishu.agent_execute',
  ],
  'edge.feishu_agent_calls_start_run': [
    'integration.feishu.agent_execute',
    'calls',
    'integration.feishu.start_run',
  ],
  'edge.feishu_start_calls_create_thread': [
    'integration.feishu.start_run',
    'calls',
    'application.create_task_thread',
  ],
  'edge.feishu_start_calls_create_run': [
    'integration.feishu.start_run',
    'calls',
    'application.create_run',
  ],
  'edge.app_run_calls_new_run_policy': [
    'application.create_run',
    'delegates_to',
    'runtime.new_run_policy',
  ],
  'edge.new_run_policy_precedes_selector': [
    'runtime.new_run_policy',
    'precedes',
    'runtime.selector.execute',
  ],
  'edge.selector_executes_adk': [
    'runtime.selector.execute',
    'executes',
    'runtime.adk_executor.execute',
  ],
  'edge.adk_execute_delegates_adaptive_bootstrap': [
    'runtime.adk_executor.execute',
    'delegates_to',
    'application.adaptive_mode_free_contract_boundary',
  ],
  'edge.adaptive_bootstrap_precedes_agent_build': [
    'application.adaptive_mode_free_contract_boundary',
    'precedes',
    'runtime.adk_agent_factory.build',
  ],
  'edge.deerflow_configures_eino': [
    'compat.deerflow_config',
    'configures',
    'runtime.selector.execute',
  ],
  'edge.legacy_is_historical_only': [
    'runtime.selector.execute',
    'routes_to',
    'historical.legacy_runtime',
  ],
  'edge.legacy_excluded_from_new_runs': [
    'runtime.new_run_policy',
    'excludes',
    'historical.legacy_runtime',
  ],
};
const REQUIRED_FORBIDDEN_TERMS = ['k2', 'chattask'];

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

const stringValue = value => (typeof value === 'string' ? value.trim() : '');

const isFrameworkNode = node => FRAMEWORK_LAYERS.has(node?.layer);

const readRepoFile = async (repoRoot, relativePath) =>
  readFile(normalizeRepoPath(repoRoot, relativePath), 'utf8');

const regexEscape = value => value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');

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

const missingRequiredIDs = (items, requiredIDs, type) => {
  const actual = new Set(asArray(items).map(item => stringValue(item?.id)));
  return requiredIDs
    .filter(id => !actual.has(id))
    .map(id => `profile_${type}_missing: ${id}`);
};

const stableJSONValue = value => {
  if (Array.isArray(value)) {
    return value.map(stableJSONValue);
  }
  if (value && typeof value === 'object') {
    return Object.fromEntries(
      Object.keys(value)
        .sort()
        .map(key => [key, stableJSONValue(value[key])]),
    );
  }
  return value;
};

const canonicalProfileStructureDigest = contract => {
  const byStableID = (left, right) =>
    stringValue(left?.id).localeCompare(stringValue(right?.id));
  const projection = {
    schema_version: contract?.schema_version,
    profile: contract?.profile,
    authority_rule: contract?.authority?.rule,
    scope: contract?.scope,
    nodes: [...asArray(contract?.nodes)].sort(byStableID),
    edges: [...asArray(contract?.edges)].sort(byStableID),
    chains: [...asArray(contract?.chains)].sort(byStableID),
    exclusions: [...asArray(contract?.exclusions)].sort(byStableID),
    required_queries: [...asArray(contract?.required_queries)].sort(byStableID),
  };
  return createHash('sha256')
    .update(JSON.stringify(stableJSONValue(projection)))
    .digest('hex');
};

const validateCanonicalProfile = (contract, required) => {
  if (!required) {
    return [];
  }

  const errors = [];
  if (contract?.profile !== WORKBENCH_PROFILE) {
    errors.push(`canonical_profile_mismatch: expected ${WORKBENCH_PROFILE}`);
  }
  const structureDigest = canonicalProfileStructureDigest(contract);
  if (structureDigest !== WORKBENCH_PROFILE_STRUCTURE_DIGEST) {
    errors.push(
      `canonical_profile_structure_mismatch: expected ${WORKBENCH_PROFILE_STRUCTURE_DIGEST} actual ${structureDigest}`,
    );
  }
  for (const [field, expected] of [
    ['canonical_runtime', 'eino_adk'],
    ['queue_backend', 'mysql'],
    ['sensitive_content_policy', 'bounded_metadata_only'],
  ]) {
    if (contract?.scope?.[field] !== expected) {
      errors.push(`profile_scope_mismatch: ${field}: expected ${expected}`);
    }
  }
  const forbiddenTerms = new Set(
    asArray(contract?.scope?.forbidden_current_terms)
      .map(term => stringValue(term).toLowerCase())
      .filter(Boolean),
  );
  for (const term of REQUIRED_FORBIDDEN_TERMS) {
    if (!forbiddenTerms.has(term)) {
      errors.push(`profile_forbidden_term_missing: ${term}`);
    }
    for (const node of asArray(contract?.nodes)) {
      const searchable = `${stringValue(node?.id)} ${stringValue(
        node?.label,
      )}`.toLowerCase();
      if (
        node?.production_status === CURRENT_STATUS &&
        searchable.includes(term)
      ) {
        errors.push(`profile_forbidden_current_node: ${node.id}: ${term}`);
      }
    }
  }
  errors.push(
    ...missingRequiredIDs(contract?.nodes, REQUIRED_NODE_IDS, 'node'),
    ...missingRequiredIDs(contract?.edges, REQUIRED_EDGE_IDS, 'edge'),
    ...missingRequiredIDs(contract?.chains, REQUIRED_CHAIN_IDS, 'chain'),
    ...missingRequiredIDs(
      contract?.required_queries,
      REQUIRED_QUERY_IDS,
      'query',
    ),
    ...missingRequiredIDs(
      contract?.exclusions,
      REQUIRED_EXCLUSION_IDS,
      'exclusion',
    ),
  );
  const edgesByID = new Map(
    asArray(contract?.edges).map(edge => [stringValue(edge?.id), edge]),
  );
  for (const [edgeID, [from, relation, to]] of Object.entries(
    REQUIRED_EDGE_SHAPES,
  )) {
    const edge = edgesByID.get(edgeID);
    if (
      edge &&
      (edge.from !== from || edge.relation !== relation || edge.to !== to)
    ) {
      errors.push(`profile_edge_shape_mismatch: ${edgeID}`);
    }
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
      errors.push(
        `framework_runtime_scope_invalid: ${nodeID}: ${runtimeScope}`,
      );
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
    const searchable = `${stringValue(node?.id)} ${stringValue(
      node?.label,
    )}`.toLowerCase();
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
    return [
      'middleware_order_source_invalid: adkMiddlewareOrder block is missing',
    ];
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
    return [`middleware_order_mismatch: expected ${expected.join(' -> ')}`];
  }

  return [];
};

const normalizeGitPath = candidate =>
  typeof candidate === 'string' ? candidate.split(path.sep).join('/') : '';

const globMatches = (candidate, pattern) => {
  const normalizedCandidate = normalizeGitPath(candidate);
  const normalizedPattern = normalizeGitPath(stringValue(pattern));
  let source = '^';
  for (let index = 0; index < normalizedPattern.length; index += 1) {
    const character = normalizedPattern[index];
    if (character === '*' && normalizedPattern[index + 1] === '*') {
      source += '[\\s\\S]*';
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

const validateMonitoredCoverage = contract => {
  const contractPath = stringValue(contract?.authority?.contract).replaceAll(
    '\\',
    '/',
  );
  if (contractPath !== CANONICAL_CONTRACT_PATH) {
    return [];
  }

  const referenced = new Set();
  const addPath = candidate => {
    const value = stringValue(candidate).replaceAll('\\', '/');
    if (value) {
      referenced.add(value);
    }
  };
  const addAnchors = anchors => {
    for (const anchor of asArray(anchors)) {
      addPath(anchor?.path);
    }
  };
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

  const authorityFiles = new Set(
    [contract?.authority?.contract, contract?.authority?.document]
      .map(candidate => stringValue(candidate).replaceAll('\\', '/'))
      .filter(Boolean),
  );
  const patterns = asArray(contract?.scope?.monitored_paths);
  return [...referenced]
    .filter(relativePath => !authorityFiles.has(relativePath))
    .filter(
      relativePath =>
        !patterns.some(pattern => globMatches(relativePath, pattern)),
    )
    .sort()
    .map(relativePath => `contract_source_unmonitored: ${relativePath}`);
};

export const evaluateAuthorityChanges = (changedPaths, scope) => {
  const changed = new Set(
    asArray(changedPaths).map(normalizeGitPath).filter(Boolean),
  );
  const authorityFiles = asArray(scope?.authority_files).map(candidate =>
    normalizeGitPath(stringValue(candidate)),
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
    asArray(scope?.monitored_paths).some(pattern =>
      globMatches(candidate, pattern),
    ),
  );
  if (monitoredChanged && changedAuthorityCount === 0) {
    return ['authority_files_not_updated'];
  }

  return [];
};

export const gitChangedPaths = async (repoRoot, baseRef) => {
  const commands = [
    ['diff', '--no-renames', '--name-only', '--diff-filter=ACDMRT', '-z', '--'],
    [
      'diff',
      '--cached',
      '--no-renames',
      '--name-only',
      '--diff-filter=ACDMRT',
      '-z',
      '--',
    ],
    ['ls-files', '--others', '--exclude-standard', '-z'],
  ];
  if (stringValue(baseRef)) {
    commands.push([
      'diff',
      '--no-renames',
      '--name-only',
      '--diff-filter=ACDMRT',
      '-z',
      `${baseRef}...HEAD`,
      '--',
    ]);
  }

  const paths = new Set();
  for (const args of commands) {
    const { stdout } = await execFileAsync('git', args, {
      cwd: repoRoot,
      encoding: 'utf8',
    });
    for (const candidate of stdout.split('\0')) {
      const normalized = normalizeGitPath(candidate);
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
    errors.push(
      `anchor_locator_not_found: ${owner}: ${relativePath}: ${locator}`,
    );
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

  for (const edgeID of asArray(chain?.required_side_edge_ids)) {
    const edge = edgesByID.get(edgeID);
    if (!edge) {
      errors.push(`chain_side_edge_missing: ${chainID}: ${edgeID}`);
    } else if (!asArray(edge?.chain_ids).includes(chainID)) {
      errors.push(`chain_side_edge_membership_missing: ${chainID}: ${edgeID}`);
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
    errors.push(
      `schema_version_unsupported: ${String(contract?.schema_version)}`,
    );
  }

  const nodes = asArray(contract?.nodes);
  const edges = asArray(contract?.edges);
  const chains = asArray(contract?.chains);
  errors.push(...duplicateErrors(nodes, 'node'));
  errors.push(...duplicateErrors(edges, 'edge'));
  errors.push(...duplicateErrors(chains, 'chain'));
  errors.push(
    ...duplicateErrors(contract?.required_queries, 'required_query'),
    ...duplicateErrors(contract?.exclusions, 'exclusion'),
    ...validateCanonicalProfile(
      contract,
      options?.requireCanonicalProfile !== false,
    ),
  );
  errors.push(...validateFrameworkScopes(contract));
  errors.push(...validateForbiddenCurrentNodes(contract));
  errors.push(...validateMonitoredCoverage(contract));

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
      errors.push(
        `node_source_anchor_missing: ${nodeID || '<missing-node-id>'}`,
      );
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
            `framework_version_mismatch: ${nodeID}: declared ${stringValue(
              node?.version,
            )} resolved ${resolvedVersion}`,
          );
        }
      } catch (error) {
        errors.push(
          `framework_version_source_invalid: ${nodeID}: ${error.message}`,
        );
      }
    }
  }

  const chainIDs = new Set(chains.map(chain => stringValue(chain?.id)));
  const edgesByID = new Map();
  for (const edge of edges) {
    const edgeID = stringValue(edge?.id);
    if (edgeID && !edgesByID.has(edgeID)) {
      edgesByID.set(edgeID, edge);
    }
    if (!nodesByID.has(edge?.from)) {
      errors.push(
        `edge_source_missing: ${edgeID || '<missing-edge-id>'}: ${String(
          edge?.from,
        )}`,
      );
    }
    if (!nodesByID.has(edge?.to)) {
      errors.push(
        `edge_target_missing: ${edgeID || '<missing-edge-id>'}: ${String(
          edge?.to,
        )}`,
      );
    }
    if (!ALLOWED_RELATIONS.has(edge?.relation)) {
      errors.push(
        `edge_relation_invalid: ${edgeID || '<missing-edge-id>'}: ${String(
          edge?.relation,
        )}`,
      );
    }
    if (edge?.confidence !== 'extracted') {
      errors.push(`edge_confidence_invalid: ${edgeID || '<missing-edge-id>'}`);
    }
    if (asArray(edge?.chain_ids).length === 0) {
      errors.push(`edge_chain_ids_missing: ${edgeID || '<missing-edge-id>'}`);
    }
    for (const chainID of asArray(edge?.chain_ids)) {
      if (!chainIDs.has(chainID)) {
        errors.push(
          `edge_chain_missing: ${edgeID || '<missing-edge-id>'}: ${chainID}`,
        );
      }
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

  for (const query of asArray(contract?.required_queries)) {
    const queryID = stringValue(query?.id) || '<missing-query-id>';
    if (!stringValue(query?.question)) {
      errors.push(`query_question_missing: ${queryID}`);
    }
    if (asArray(query?.required_node_ids).length === 0) {
      errors.push(`query_required_nodes_missing: ${queryID}`);
    }
    for (const nodeID of asArray(query?.required_node_ids)) {
      if (!nodesByID.has(nodeID)) {
        errors.push(`query_required_node_missing: ${queryID}: ${nodeID}`);
      }
    }
    if (asArray(query?.required_edge_ids).length === 0) {
      errors.push(`query_required_edges_missing: ${queryID}`);
    }
    for (const edgeID of asArray(query?.required_edge_ids)) {
      if (!edgesByID.has(edgeID)) {
        errors.push(`query_required_edge_missing: ${queryID}: ${edgeID}`);
      }
    }
    const expandedTerms = asArray(query?.expanded_terms).map(stringValue);
    for (const nodeID of asArray(query?.smoke_node_ids)) {
      const node = nodesByID.get(nodeID);
      if (!node) {
        errors.push(`query_smoke_node_missing: ${queryID}: ${nodeID}`);
        continue;
      }
      const label = stringValue(node?.label);
      if (!expandedTerms.includes(label)) {
        errors.push(`query_smoke_term_missing: ${queryID}: ${label}`);
      }
    }
  }

  for (const chain of chains) {
    const chainID = stringValue(chain?.id);
    if (chain?.production_status !== CURRENT_STATUS) {
      errors.push(`chain_not_current: ${chainID || '<missing-chain-id>'}`);
    }
    if (asArray(chain?.test_evidence).length === 0) {
      errors.push(
        `chain_test_evidence_missing: ${chainID || '<missing-chain-id>'}`,
      );
    }
    errors.push(
      ...(await validateEvidenceList(chain?.test_evidence, {
        repoRoot,
        owner: `chain.${chainID}`,
      })),
    );
    errors.push(...validateOrderedChain(chain, nodesByID, edgesByID));
  }

  for (const exclusion of asArray(contract?.exclusions)) {
    const exclusionID = stringValue(exclusion?.id) || '<missing-exclusion-id>';
    if (asArray(exclusion?.evidence).length === 0) {
      errors.push(`exclusion_evidence_missing: ${exclusionID}`);
    }
    errors.push(
      ...(await validateEvidenceList(exclusion?.evidence, {
        repoRoot,
        owner: `exclusion.${exclusionID}`,
      })),
    );
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
