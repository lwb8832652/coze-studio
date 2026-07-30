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

import { join, relative, resolve } from 'node:path';
import { readFileSync, readdirSync } from 'node:fs';

import { describe, expect, it } from 'vitest';
import ts from 'typescript';

const excludedProductionDirectories = new Set(['__mocks__', '__tests__']);

const getFilesRecursively = (
  root: string,
  include: (path: string, name: string) => boolean,
  excludedDirectories = new Set<string>(),
): string[] =>
  readdirSync(root, { withFileTypes: true })
    .flatMap(entry => {
      const path = join(root, entry.name);

      if (entry.isDirectory()) {
        return excludedDirectories.has(entry.name)
          ? []
          : getFilesRecursively(path, include, excludedDirectories);
      }

      return include(path, entry.name) ? [path] : [];
    })
    .sort();

const getProductionSourceFiles = (root: string): string[] =>
  getFilesRecursively(
    root,
    (_path, name) =>
      /\.tsx?$/.test(name) && !/\.(?:spec|test)\.tsx?$/.test(name),
    excludedProductionDirectories,
  );

const repositoryRoot = resolve(__dirname, '../../../../../../..');
const appSourceRoot = resolve(__dirname, '../../..');
const appProductionFiles = getProductionSourceFiles(appSourceRoot);

const productionFiles = [
  ...getProductionSourceFiles(resolve(__dirname, '..')),
  ...getProductionSourceFiles(resolve(__dirname, '../../workbench')),
  ...getProductionSourceFiles(
    resolve(__dirname, '../../../components/workspace-sub-menu'),
  ),
  resolve(__dirname, '../../chats/task-thread-routes.ts'),
  resolve(__dirname, '../../../routes/index.tsx'),
];

const threadSurfaceFiles = [
  ...getProductionSourceFiles(resolve(__dirname, '..')),
  ...getProductionSourceFiles(resolve(__dirname, '../../workbench')),
  resolve(
    __dirname,
    '../../../components/workspace-sub-menu/workspace-task-list.tsx',
  ),
];

const canonicalTransportFiles = new Set([
  resolve(
    __dirname,
    '../../workbench/thread-client/adapters/canonical-thread-adapter.ts',
  ),
  resolve(__dirname, '../../workbench/thread-client/canonical-fetch.ts'),
  resolve(
    __dirname,
    '../../workbench/thread-client/canonical-thread-client.ts',
  ),
]);

const canonicalClientSingletonFile = resolve(
  __dirname,
  '../../workbench/thread-client/canonical-thread-client-singleton.ts',
);
const deerFlowParityRoot = resolve(
  repositoryRoot,
  'backend/internal/deerflowparity',
);
const newXClientFile = resolve(deerFlowParityRoot, 'newx_client.go');
const externalDeerFlowClientFile = resolve(
  deerFlowParityRoot,
  'deerflow_client.go',
);
const builtinDeerFlowSkillRoot = resolve(
  repositoryRoot,
  'backend/application/skill/builtin_deerflow/public/claude-to-deerflow',
);
const backendProductionContractFiles = getFilesRecursively(
  resolve(repositoryRoot, 'backend'),
  (_path, name) =>
    /\.(?:cjs|go|js|json|md|mjs|py|sh|ts|tsx|yaml|yml)$/.test(name) &&
    !name.endsWith('_test.go'),
  new Set(['testdata']),
);
const expectedExternalThreadRouteOwners = [
  externalDeerFlowClientFile,
  resolve(builtinDeerFlowSkillRoot, 'SKILL.md'),
  resolve(builtinDeerFlowSkillRoot, 'scripts/chat.sh'),
]
  .map(file => relative(repositoryRoot, file))
  .sort();

const canonicalClientSelectorIdentifiers = [
  'TaskThreadV1Client',
  'COZE_WORKBENCH_CANONICAL_API_ENABLED',
  'WORKBENCH_THREAD_CLIENT_MODE',
  'createWorkbenchThreadClient',
  'selectWorkbenchThreadClient',
  'resolveWorkbenchThreadClient',
] as const;

const forbiddenIdentifiers = [
  'WorkbenchChat',
  'GetTask',
  'ListTasks',
  'CancelTask',
  'RetryTask',
  'ListTaskEvents',
  'getTask',
  'listTasks',
  'cancelTask',
  'retryTask',
  'listTaskEvents',
  'sendWorkbenchChat',
  'buildLegacyTaskDetailPath',
  'TaskEventDisplay',
  'getTaskEventDisplay',
  'getTaskEventText',
  'getTaskEventRunID',
  'mapTaskThreadRunEventToTaskEvent',
  'mapTaskThreadRunJournalMessageToTaskEvent',
  'mergeJournalTaskEvents',
  'mergeTaskEvents',
  'parseTaskEventPayload',
  ...canonicalClientSelectorIdentifiers,
] as const;

const forbiddenPatterns = [
  new RegExp(`\\b(?:${forbiddenIdentifiers.join('|')})\\b`, 'g'),
  /\bworkbenchTask\.(?:ChatTask|TaskStatus|TaskEvent)\b/g,
  /\blegacy_task_id\b/g,
  /\bsource_task_id\b/g,
  /tasks\/:task_id\b/g,
];

const forbiddenThreadGeneratedPatterns = [
  /\bworkbenchTask\.[A-Za-z0-9_]*TaskThread[A-Za-z0-9_]*\b/g,
  /\bworkbenchThread\b/g,
];

const forbiddenThreadRoutePatterns = [
  /\/api\/workbench\/task_threads\b/g,
  /\/api\/threads\b/g,
  /\/api\/runs\b/g,
];

const forbiddenThreadInvocationPatterns = [
  /\b(?:globalThis\.)?fetch\s*\(/g,
  /\bnew\s+EventSource\b/g,
];

const sourceFinding = (file: string, symbol: string) => ({
  file: relative(repositoryRoot, file),
  symbol,
});

const canonicalClientClassNames = new Set([
  'CanonicalThreadClient',
  'CanonicalThreadCoreClient',
]);

const isCanonicalClientImportSource = (source: string): boolean =>
  source === './canonical-thread-client' ||
  source.endsWith('/canonical-thread-client') ||
  source.endsWith('/thread-client');

const canonicalClientConstructionFindings = (file: string) => {
  const source = readFileSync(file, 'utf8');
  if (![...canonicalClientClassNames].some(name => source.includes(name))) {
    return [];
  }

  const sourceFile = ts.createSourceFile(
    file,
    source,
    ts.ScriptTarget.Latest,
    true,
    file.endsWith('.tsx') ? ts.ScriptKind.TSX : ts.ScriptKind.TS,
  );
  const aliases = new Map<string, string>();
  const namespaceAliases = new Set<string>();

  for (const statement of sourceFile.statements) {
    if (
      !ts.isImportDeclaration(statement) ||
      !ts.isStringLiteral(statement.moduleSpecifier) ||
      !isCanonicalClientImportSource(statement.moduleSpecifier.text)
    ) {
      continue;
    }
    const bindings = statement.importClause?.namedBindings;
    if (bindings && ts.isNamespaceImport(bindings)) {
      namespaceAliases.add(bindings.name.text);
      continue;
    }
    if (!bindings || !ts.isNamedImports(bindings)) {
      continue;
    }
    for (const element of bindings.elements) {
      const importedName = (element.propertyName ?? element.name).text;
      if (canonicalClientClassNames.has(importedName)) {
        aliases.set(element.name.text, importedName);
      }
    }
  }

  const findings: ReturnType<typeof sourceFinding>[] = [];
  const visit = (node: ts.Node): void => {
    if (ts.isNewExpression(node)) {
      const { expression } = node;
      const isNamedImport =
        ts.isIdentifier(expression) && aliases.has(expression.text);
      const isNamespaceImport =
        ts.isPropertyAccessExpression(expression) &&
        ts.isIdentifier(expression.expression) &&
        namespaceAliases.has(expression.expression.text) &&
        canonicalClientClassNames.has(expression.name.text);

      if (isNamedImport || isNamespaceImport) {
        let importedName: string | undefined;
        if (ts.isIdentifier(expression)) {
          importedName = aliases.get(expression.text);
        } else if (ts.isPropertyAccessExpression(expression)) {
          importedName = expression.name.text;
        }
        findings.push(sourceFinding(file, importedName ?? 'unknown'));
      }
    }
    ts.forEachChild(node, visit);
  };

  visit(sourceFile);
  return findings;
};

describe('canonical Workbench task frontend contract', () => {
  it('keeps production task and workbench sources free of retired contracts', () => {
    const findings = productionFiles.flatMap(file => {
      const source = readFileSync(file, 'utf8');

      return forbiddenPatterns.flatMap(pattern =>
        Array.from(source.matchAll(pattern), match =>
          sourceFinding(file, match[0]),
        ),
      );
    });

    expect(findings).toEqual([]);
  });

  it('keeps Thread page production files behind the canonical client boundary', () => {
    const taskCenterService = resolve(
      __dirname,
      '../../task-center/service.ts',
    );
    const generatedTypeFindings = [
      ...threadSurfaceFiles,
      taskCenterService,
    ].flatMap(file => {
      const source = readFileSync(file, 'utf8');

      return forbiddenThreadGeneratedPatterns.flatMap(pattern =>
        Array.from(source.matchAll(pattern), match =>
          sourceFinding(file, match[0]),
        ),
      );
    });
    const transportFindings = threadSurfaceFiles.flatMap(file => {
      const source = readFileSync(file, 'utf8');
      const threadForbiddenPatterns = canonicalTransportFiles.has(file)
        ? forbiddenThreadRoutePatterns
        : [
            ...forbiddenThreadRoutePatterns,
            ...forbiddenThreadInvocationPatterns,
          ];

      return threadForbiddenPatterns.flatMap(pattern =>
        Array.from(source.matchAll(pattern), match =>
          sourceFinding(file, match[0]),
        ),
      );
    });

    expect({
      generatedTypeFindings,
      transportFindings,
    }).toEqual({
      generatedTypeFindings: [],
      transportFindings: [],
    });
  });

  it('constructs exactly one production canonical client with no runtime selector', () => {
    const canonicalClientCreations = appProductionFiles.flatMap(file =>
      canonicalClientConstructionFindings(file),
    );
    const selectorPattern = new RegExp(
      `\\b(?:${canonicalClientSelectorIdentifiers.join('|')})\\b`,
      'g',
    );
    const selectorFindings = appProductionFiles.flatMap(file => {
      const source = readFileSync(file, 'utf8');

      return Array.from(source.matchAll(selectorPattern), match =>
        sourceFinding(file, match[0]),
      );
    });

    expect({ canonicalClientCreations, selectorFindings }).toEqual({
      canonicalClientCreations: [
        expect.objectContaining({
          file: relative(repositoryRoot, canonicalClientSingletonFile),
        }),
      ],
      selectorFindings: [],
    });
  });

  it('keeps NewX canonical and isolates external DeerFlow route ownership', () => {
    const newXSource = readFileSync(newXClientFile, 'utf8');
    const forbiddenNewXRoutes = [
      /\/api\/threads\b/g,
      /\/api\/runs\b/g,
      /\/api\/workbench\/task_threads\b/g,
    ].flatMap(pattern =>
      Array.from(newXSource.matchAll(pattern), match => match[0]),
    );
    const externalThreadRouteOwners = backendProductionContractFiles
      .flatMap(file => {
        const source = readFileSync(file, 'utf8');
        return /\/api\/threads\b/.test(source)
          ? [relative(repositoryRoot, file)]
          : [];
      })
      .sort();

    expect(newXSource).toContain('/api/workbench/threads');
    expect(forbiddenNewXRoutes).toEqual([]);
    expect(externalThreadRouteOwners).toEqual(
      expectedExternalThreadRouteOwners,
    );
  });
});
