#!/usr/bin/env node
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
import { fileURLToPath } from 'node:url';

import {
  gitChangedPaths,
  validateContract,
} from './workbench-execution-graph/contract.mjs';
import {
  buildDerivedGraph,
  DERIVED_RELATIVE_ROOT,
  verifyDerivedGraph,
} from './workbench-execution-graph/derived.mjs';

const SCRIPT_DIR = path.dirname(fileURLToPath(import.meta.url));
const DEFAULT_REPO_ROOT = path.resolve(SCRIPT_DIR, '..');
const DEFAULT_CONTRACT =
  'docs/superpowers/context/workbench-execution-graph.json';
const DEFAULT_AUTHORITY =
  'docs/superpowers/context/workbench-execution-chain.md';
const COMMANDS = new Set(['verify', 'build', 'verify-derived']);
const VALUE_OPTIONS = new Set([
  '--changed-from',
  '--repo-root',
  '--contract',
  '--authority',
  '--derived-root',
]);

export const CLI_HELP = `Usage:
  node scripts/workbench-execution-graph.mjs verify [--changed-from <ref>]
  node scripts/workbench-execution-graph.mjs build
  node scripts/workbench-execution-graph.mjs verify-derived

Options:
  --changed-from <ref>  Verify authority updates against a Git base ref (verify only)
  --repo-root <path>    Override repository root for tests
  --contract <path>     Override the machine contract path for tests
  --authority <path>    Override the authority Markdown path for tests
  --derived-root <path> Override the generated graph root for tests
  --help                Show this help
`;

const stringValue = value =>
  typeof value === 'string' ? value.trim() : '';

const parseArguments = argv => {
  if (argv.length === 0 || argv[0] === '--help' || argv[0] === 'help') {
    return { help: true, options: {} };
  }
  const command = argv[0];
  if (!COMMANDS.has(command)) {
    throw new Error(`unknown command: ${command}`);
  }

  const options = {};
  for (let index = 1; index < argv.length; index += 1) {
    const option = argv[index];
    if (option === '--help') {
      return { help: true, options: {} };
    }
    if (!VALUE_OPTIONS.has(option)) {
      throw new Error(`unknown option: ${option}`);
    }
    const value = argv[index + 1];
    if (!value || value.startsWith('--')) {
      throw new Error(`missing value for ${option}`);
    }
    options[option.slice(2).replaceAll('-', '_')] = value;
    index += 1;
  }
  if (options.changed_from && command !== 'verify') {
    throw new Error('--changed-from is only valid with verify');
  }
  return { command, help: false, options };
};

const resolveFromRoot = (repoRoot, candidate, fallback) => {
  const value = stringValue(candidate) || fallback;
  return path.isAbsolute(value) ? path.resolve(value) : path.resolve(repoRoot, value);
};

const relativeToRepo = (repoRoot, absolutePath, optionName) => {
  const relative = path.relative(repoRoot, absolutePath);
  if (
    relative === '..' ||
    relative.startsWith(`..${path.sep}`) ||
    path.isAbsolute(relative)
  ) {
    throw new Error(`${optionName} must stay inside --repo-root`);
  }
  return relative.replaceAll('\\', '/');
};

const loadContext = async options => {
  const repoRoot = path.resolve(options.repo_root ?? DEFAULT_REPO_ROOT);
  const contractPath = resolveFromRoot(
    repoRoot,
    options.contract,
    DEFAULT_CONTRACT,
  );
  const authorityPath = resolveFromRoot(
    repoRoot,
    options.authority,
    DEFAULT_AUTHORITY,
  );
  const derivedRoot = resolveFromRoot(
    repoRoot,
    options.derived_root,
    DERIVED_RELATIVE_ROOT,
  );
  const contract = JSON.parse(await readFile(contractPath, 'utf8'));
  contract.authority = {
    ...contract.authority,
    contract: relativeToRepo(repoRoot, contractPath, '--contract'),
    document: relativeToRepo(repoRoot, authorityPath, '--authority'),
  };
  return {
    authorityDocument: contract.authority.document,
    contract,
    contractPath,
    derivedRoot,
    repoRoot,
  };
};

const assertValid = result => {
  if (result.errors.length > 0) {
    throw new Error(
      `verification failed:\n${result.errors.map(error => `- ${error}`).join('\n')}`,
    );
  }
};

export const runCLI = async (
  argv,
  { stdout = process.stdout } = {},
) => {
  const parsed = parseArguments(argv);
  if (parsed.help) {
    stdout.write(CLI_HELP);
    return;
  }

  const context = await loadContext(parsed.options);
  const changedPaths = parsed.options.changed_from
    ? await gitChangedPaths(context.repoRoot, parsed.options.changed_from)
    : undefined;
  const validation = await validateContract(context.contract, {
    changedPaths,
    repoRoot: context.repoRoot,
  });
  assertValid(validation);

  if (parsed.command === 'verify') {
    stdout.write(
      `Workbench execution graph verification passed (${context.contract.nodes.length} nodes, ${context.contract.edges.length} edges, ${context.contract.chains.length} chains).\n`,
    );
    return;
  }

  if (parsed.command === 'build') {
    const built = await buildDerivedGraph(context.contract, {
      authorityDocument: context.authorityDocument,
      derivedRoot: context.derivedRoot,
      repoRoot: context.repoRoot,
      resolvedVersions: validation.resolvedVersions,
    });
    stdout.write(`Workbench execution graph built at ${built.graphPath}.\n`);
    return;
  }

  const verified = await verifyDerivedGraph(context.contract, {
    authorityDocument: context.authorityDocument,
    derivedRoot: context.derivedRoot,
    repoRoot: context.repoRoot,
    resolvedVersions: validation.resolvedVersions,
  });
  assertValid(verified);
  stdout.write('Workbench derived execution graph verification passed.\n');
};

const isMain = process.argv[1]
  ? path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)
  : false;

if (isMain) {
  runCLI(process.argv.slice(2)).catch(error => {
    process.stderr.write(`error: ${error.message}\n`);
    process.exitCode = 1;
  });
}
