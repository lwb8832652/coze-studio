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

import { basename, resolve } from 'node:path';
import { existsSync, readFileSync } from 'node:fs';

import { describe, expect, it } from 'vitest';
import * as ts from 'typescript';

import {
  legacyTaskThreadReference,
  unwrapLegacyTaskThreadResponse,
} from './legacy-task-thread-reference';
import {
  canonicalRunSubmissionExtensions,
  pairedTransportFixtures,
  projectCanonicalTransportFixture,
  projectV1TransportFixture,
  type TransportFixtureFamily,
} from './fixtures';

const publicBoundaryFileNames = [
  'types.ts',
  'workbench-thread-client.ts',
  'index.ts',
] as const;
const productionFiles = publicBoundaryFileNames.map(fileName =>
  resolve(__dirname, '..', fileName),
);
const repositoryRoot = resolve(__dirname, '../../../../../../../..');
const canonicalRunContractFiles = {
  idl: resolve(repositoryRoot, 'idl/workbench/thread.thrift'),
  go: resolve(
    repositoryRoot,
    'backend/api/model/workbench/thread_contract/thread.go',
  ),
  typescript: resolve(
    repositoryRoot,
    'frontend/packages/arch/api-schema/src/idl/workbench/thread.ts',
  ),
} as const;

const readProductionSource = (file: string) => readFileSync(file, 'utf8');
const parseSource = (fileName: string, source: string) =>
  ts.createSourceFile(
    fileName,
    source,
    ts.ScriptTarget.Latest,
    true,
    ts.ScriptKind.TS,
  );

type NodeCheck = (node: ts.Node) => string | undefined;

const forbiddenSymbols = new Set(['workbenchTask', 'workbenchThread']);
const browserConstructors = new Set([
  'EventSource',
  'FormData',
  'Request',
  'URL',
  'URLSearchParams',
  'WebSocket',
  'XMLHttpRequest',
]);
const browserCalls = new Set(['fetch', ...browserConstructors]);
const browserRoots = new Set(['globalThis', 'self', 'window']);
const templateTokens = new Set([
  ts.SyntaxKind.TemplateHead,
  ts.SyntaxKind.TemplateMiddle,
  ts.SyntaxKind.TemplateTail,
]);
const routeLikeLiteralPatterns = [
  /(?:^|:)\/[a-zA-Z0-9_-]+(?=[/?#]|$)/,
  /^(?:GET|POST|PUT|PATCH|DELETE|HEAD|OPTIONS)\s+\/[a-zA-Z0-9_-]+(?=[/?#]|$)/i,
  /^https?:\/\/[^\s/?#]+(?=[/?#]|$)/i,
] as const;

const accessPath = (expression: ts.Expression): string[] | undefined => {
  if (
    ts.isAsExpression(expression) ||
    ts.isNonNullExpression(expression) ||
    ts.isParenthesizedExpression(expression) ||
    ts.isTypeAssertionExpression(expression)
  ) {
    return accessPath(expression.expression);
  }
  if (ts.isIdentifier(expression)) {
    return [expression.text];
  }
  if (ts.isPropertyAccessExpression(expression)) {
    const owner = accessPath(expression.expression);
    return owner && [...owner, expression.name.text];
  }
  if (
    ts.isElementAccessExpression(expression) &&
    expression.argumentExpression &&
    (ts.isStringLiteral(expression.argumentExpression) ||
      ts.isNoSubstitutionTemplateLiteral(expression.argumentExpression))
  ) {
    const owner = accessPath(expression.expression);
    return owner && [...owner, expression.argumentExpression.text];
  }
};

const isModuleString = (node: ts.StringLiteral) => {
  const { parent } = node;
  return (
    ((ts.isImportDeclaration(parent) || ts.isExportDeclaration(parent)) &&
      parent.moduleSpecifier === node) ||
    (ts.isCallExpression(parent) &&
      parent.arguments[0] === node &&
      (parent.expression.kind === ts.SyntaxKind.ImportKeyword ||
        (ts.isIdentifier(parent.expression) &&
          parent.expression.text === 'require'))) ||
    (ts.isLiteralTypeNode(parent) && ts.isImportTypeNode(parent.parent)) ||
    (ts.isExternalModuleReference(parent) && parent.expression === node)
  );
};

const checkModule: NodeCheck = node => {
  if (!ts.isStringLiteral(node) || !isModuleString(node)) {
    return;
  }
  const forbidden =
    node.text === '@coze-studio/api-schema' ||
    node.text.startsWith('@coze-studio/api-schema/') ||
    node.text.includes('legacy-task-thread-reference');
  return forbidden ? `module:${node.text}` : undefined;
};

const checkLiteral: NodeCheck = node => {
  if (
    ts.isStringLiteral(node) &&
    isModuleString(node) &&
    /^\.\.?\//.test(node.text)
  ) {
    return;
  }
  const value =
    ts.isStringLiteral(node) || ts.isNoSubstitutionTemplateLiteral(node)
      ? node.text
      : templateTokens.has(node.kind)
        ? (node as ts.TemplateLiteralToken).text
        : '';
  return routeLikeLiteralPatterns.some(pattern => pattern.test(value))
    ? `route:${value}`
    : undefined;
};

const checkSymbol: NodeCheck = node =>
  ts.isIdentifier(node) && forbiddenSymbols.has(node.text)
    ? `symbol:${node.text}`
    : undefined;

const checkTransport: NodeCheck = node => {
  if (!ts.isCallExpression(node) && !ts.isNewExpression(node)) {
    return;
  }
  const path = accessPath(node.expression);
  const names = ts.isCallExpression(node) ? browserCalls : browserConstructors;
  const global =
    path &&
    names.has(path.at(-1) ?? '') &&
    (path.length === 1 || (path.length === 2 && browserRoots.has(path[0])));
  return global ? `browser_transport:${path.join('.')}` : undefined;
};

const nodeChecks = [
  checkModule,
  checkLiteral,
  checkSymbol,
  checkTransport,
] as const;

const isTypeOnlyImport = (statement: ts.ImportDeclaration): boolean => {
  const clause = statement.importClause;
  if (!clause) {
    return false;
  }
  if (clause.isTypeOnly) {
    return true;
  }
  return (
    !clause.name &&
    !!clause.namedBindings &&
    ts.isNamedImports(clause.namedBindings) &&
    clause.namedBindings.elements.every(element => element.isTypeOnly)
  );
};

const isTypeOnlyExport = (statement: ts.ExportDeclaration): boolean =>
  statement.isTypeOnly ||
  (!!statement.exportClause &&
    ts.isNamedExports(statement.exportClause) &&
    statement.exportClause.elements.every(element => element.isTypeOnly));

const checkTypeLayerStatement = (
  statement: ts.Statement,
): string | undefined => {
  if (
    ts.isInterfaceDeclaration(statement) ||
    ts.isTypeAliasDeclaration(statement) ||
    (ts.isImportDeclaration(statement) && isTypeOnlyImport(statement)) ||
    (ts.isExportDeclaration(statement) && isTypeOnlyExport(statement))
  ) {
    return;
  }
  return `runtime_syntax:${ts.SyntaxKind[statement.kind]}`;
};

const collectNodes = <T>(
  fileName: string,
  source: string,
  project: (node: ts.Node) => readonly T[],
): T[] => {
  const values: T[] = [];
  const visit = (node: ts.Node) => {
    values.push(...project(node));
    ts.forEachChild(node, visit);
  };
  visit(parseSource(fileName, source));
  return values;
};

const analyzeBoundarySource = (fileName: string, source: string): string[] => {
  const sourceFile = parseSource(fileName, source);
  return [
    ...sourceFile.statements.flatMap(statement => {
      const finding = checkTypeLayerStatement(statement);
      return finding ? [finding] : [];
    }),
    ...collectNodes(fileName, source, node =>
      nodeChecks.flatMap(check => {
        const value = check(node);
        return value ? [value] : [];
      }),
    ),
  ];
};

const getContractLiterals = (fileName: string, source: string): string[] =>
  collectNodes(fileName, source, node => {
    const contract =
      ts.isPropertySignature(node) &&
      node.modifiers?.some(
        modifier => modifier.kind === ts.SyntaxKind.ReadonlyKeyword,
      ) &&
      ts.isIdentifier(node.name) &&
      node.name.text === 'contract' &&
      node.type &&
      ts.isLiteralTypeNode(node.type) &&
      ts.isStringLiteral(node.type.literal)
        ? node.type.literal.text
        : undefined;
    return contract ? [contract] : [];
  });

const getTypeDeclaration = (
  fileName: string,
  source: string,
  declarationName: string,
) => {
  const sourceFile = parseSource(fileName, source);
  const declaration = sourceFile.statements.find(
    statement =>
      (ts.isInterfaceDeclaration(statement) ||
        ts.isTypeAliasDeclaration(statement)) &&
      statement.name.text === declarationName,
  );
  if (!declaration) {
    throw new Error(`Missing declaration ${declarationName} in ${fileName}`);
  }
  return { declaration, sourceFile };
};

const getInterfaceShape = (
  fileName: string,
  source: string,
  interfaceName: string,
): string[] => {
  const { declaration, sourceFile } = getTypeDeclaration(
    fileName,
    source,
    interfaceName,
  );
  if (!ts.isInterfaceDeclaration(declaration)) {
    throw new Error(`Missing interface ${interfaceName} in ${fileName}`);
  }

  return declaration.members.map(member => {
    if (!ts.isPropertySignature(member) || !member.type) {
      throw new Error(`Unsupported member in ${interfaceName}`);
    }
    const name = member.name.getText(sourceFile);
    const optional = member.questionToken ? '?' : '';
    const type = member.type.getText(sourceFile).replace(/\s+/g, ' ');
    return `${name}${optional}: ${type}`;
  });
};

const getDeclarationText = (
  fileName: string,
  source: string,
  declarationName: string,
): string => {
  const { declaration, sourceFile } = getTypeDeclaration(
    fileName,
    source,
    declarationName,
  );
  return declaration.getText(sourceFile).replace(/\s+/g, ' ');
};

const getInterfacePropertyType = (
  fileName: string,
  source: string,
  property: { interfaceName: string; propertyName: string },
): string => {
  const { interfaceName, propertyName } = property;
  const shape = getInterfaceShape(fileName, source, interfaceName);
  const propertyType = shape.find(item => item.startsWith(`${propertyName}: `));
  if (!propertyType) {
    throw new Error(`Missing ${interfaceName}.${propertyName}`);
  }
  return propertyType.slice(propertyName.length + 2);
};
const inspectTypes = (fileName: string, source: string) => ({
  shape: (name: string) => getInterfaceShape(fileName, source, name),
  declaration: (name: string) => getDeclarationText(fileName, source, name),
  property: (owner: string, name: string) =>
    getInterfacePropertyType(fileName, source, {
      interfaceName: owner,
      propertyName: name,
    }),
});

type WirePath = readonly (string | number)[];
const getWirePath = (root: unknown, path: WirePath): unknown =>
  path.reduce<unknown>((value, segment) => {
    if (typeof segment === 'number') {
      return (value as unknown[])[segment];
    }
    return (value as Record<string, unknown>)[segment];
  }, root);
const setWirePath = (
  root: unknown,
  mutation: { path: WirePath; value: unknown },
): void => {
  const { path, value } = mutation;
  const owner = getWirePath(root, path.slice(0, -1));
  const field = path.at(-1);
  if (typeof field === 'number') {
    (owner as unknown[])[field] = value;
  } else if (field) {
    (owner as Record<string, unknown>)[field] = value;
  }
};

describe('WorkbenchThreadClient production boundary', () => {
  it('scans each explicit public boundary file exactly once', () => {
    expect(publicBoundaryFileNames).toEqual([
      'types.ts',
      'workbench-thread-client.ts',
      'index.ts',
    ]);
    expect(new Set(publicBoundaryFileNames).size).toBe(
      publicBoundaryFileNames.length,
    );
    expect(new Set(productionFiles).size).toBe(productionFiles.length);
    productionFiles.forEach(file => expect(existsSync(file)).toBe(true));
  });

  it('stays isolated from schemas, routes, and browser transports', () => {
    const findings = productionFiles.flatMap(file => {
      const source = readProductionSource(file);

      return analyzeBoundarySource(basename(file), source);
    });

    expect(findings).toEqual([]);
  });

  it('proves the AST detector with transport and route mutations', () => {
    const probes = [
      "import type {} from '@coze-studio/api-schema/task';",
      "void import('@coze-studio/api-schema/task');",
      "type S = import('@coze-studio/api-schema/task').S;",
      "export * from './__tests__/legacy-task-thread-reference';",
      'const client = workbenchTask;',
      'const client = workbenchThread;',
      'const path = `/api/threads/thread-1`;',
      'const path = `prefix:/api/workbench/${id}/runs`;',
      'const path = `prefix/${id}/api/workbench/threads`;',
      "fetch('/health');",
      "window.fetch('/health');",
      "globalThis.fetch('/health');",
      "globalThis['fetch']('/health');",
      "window['fetch']('/health');",
      "new EventSource('/events');",
      "new window.EventSource('/events');",
      "new globalThis['EventSource']('/events');",
      'new globalThis.XMLHttpRequest();',
      "new window['XMLHttpRequest']();",
      'const path = `prefix/${id}/task_threads/${threadId}`;',
      'type Route = `/v2/threads/${string}`;',
      'type Route = `${string}/artifacts/${string}`;',
      "type Route = '/graphql';",
      "type Route = '/graphql?op=x';",
      "type Route = '/graphql#operation';",
      "type Route = 'GET /graphql';",
      "type Route = '/rpc/tools';",
      "type Route = 'https://api.example.com/v1';",
      "import type {} from '/graphql';",
    ];
    probes.forEach(source =>
      expect(analyzeBoundarySource('probe.ts', source)).not.toEqual([]),
    );
    expect(
      analyzeBoundarySource(
        'safe.ts',
        'interface FetchOptions { XMLHttpRequest?: string }',
      ),
    ).toEqual([]);
    [
      "type Note = 'Use /graphql for API calls';",
      "type RelativePath = 'docs/graphql';",
    ].forEach(source =>
      expect(analyzeBoundarySource('safe.ts', source)).toEqual([]),
    );
  });

  it('allows only type-layer top-level syntax in the public boundary', () => {
    const safeSource = [
      "import type { Input } from './input';",
      "export type { Output } from './output';",
      'interface Contract { input: Input }',
      'type Alias = Contract;',
    ].join('\n');
    expect(analyzeBoundarySource('safe.ts', safeSource)).toEqual([]);

    const runtimeProbes = [
      "import { client } from './client';",
      "export { client } from './client';",
      'const client = 1;',
      'function createClient() {}',
      'class Client {}',
      'enum Mode { Canonical }',
    ];
    runtimeProbes.forEach(source =>
      expect(analyzeBoundarySource('probe.ts', source)).toContainEqual(
        expect.stringMatching(/^runtime_syntax:/),
      ),
    );
  });

  it('exposes canonical_v1 as its only public contract literal', () => {
    expect(
      productionFiles.flatMap(file =>
        getContractLiterals(basename(file), readProductionSource(file)),
      ),
    ).toEqual(['canonical_v1']);
  });

  it('exposes only the reviewed public Thread and Run fields', () => {
    const source = readProductionSource(productionFiles[0]);
    const types = inspectTypes('types.ts', source);

    expect(types.shape('WorkbenchThread')).toEqual([
      'thread_id: string',
      'space_id: string',
      'title: string',
      'status: string',
      'source: string',
      'progress: number',
      'last_user_message: string',
      'last_agent_message: string',
      'can_edit: boolean',
      'created_at: number',
      'updated_at: number',
      'values?: WorkbenchThreadValues',
    ]);
    expect(types.shape('WorkbenchRun')).toEqual([
      'run_id: string',
      'thread_id: string',
      'space_id: string',
      'assistant_id: string',
      'status: string',
      'metadata: string',
      'multitask_strategy: string',
      'message_id?: string',
      'attempt_kind: string',
      'source_run_id?: string',
      'parent_run_id?: string',
      'run_kind: string',
      'stream_modes: string[]',
      'on_disconnect: string',
      'durability: string',
      'terminal_reason?: string',
      'started_at?: number',
      'ended_at?: number',
      'created_at: number',
      'updated_at: number',
    ]);
    expect(types.shape('WorkbenchRunCreation')).toEqual([
      'run: WorkbenchRun',
      'message?: WorkbenchMessage',
    ]);
  });

  it('exposes only canonical human interaction response members', () => {
    const source = readProductionSource(productionFiles[0]);
    const types = inspectTypes('types.ts', source);

    expect(types.shape('HumanInteractionResponse')).toEqual([
      'schema: string',
      'interaction_id: string',
      'kind: string',
      'decision: string',
      'answer?: string',
      'choice_id?: string',
      'comment?: string',
    ]);
  });

  it('keeps lifecycle fields optional without presenter defaults', () => {
    const source = readProductionSource(productionFiles[0]);
    const types = inspectTypes('types.ts', source);
    const expectedOptionalFields = {
      WorkbenchArtifact: ['deleted_at?: number'],
      WorkbenchArtifactScanJob: [
        'available_at?: number',
        'started_at?: number',
        'ended_at?: number',
      ],
      WorkbenchMemory: [
        'run_id?: string',
        'correction_of_memory_id?: string',
        'corrected_at?: number',
        'expires_at?: number',
        'deleted_at?: number',
      ],
      WorkbenchMemoryAuditEvent: [
        'run_id?: string',
        'memory_id?: string',
        'actor_id?: string',
      ],
      WorkbenchGuardrailAuditEvent: ['run_id?: string', 'actor_id?: string'],
      WorkbenchMCPRuntimeAuditEvent: ['run_id?: string', 'server_id?: string'],
    } as const;

    for (const [interfaceName, fields] of Object.entries(
      expectedOptionalFields,
    )) {
      expect(types.shape(interfaceName)).toEqual(
        expect.arrayContaining([...fields]),
      );
    }
  });

  it('separates offset, message cursor, and run event cursor contracts', () => {
    const source = readProductionSource(productionFiles[1]);
    const client = inspectTypes('workbench-thread-client.ts', source);

    expect(client.shape('WorkbenchMessageCursorPage')).toEqual([
      'items: WorkbenchMessage[]',
      'total: number',
      'has_more: boolean',
      'next_before_seq?: string',
      'next_after_seq?: string',
    ]);
    expect(client.shape('WorkbenchCursorPage')).toEqual([
      'items: T[]',
      'total: number',
      'has_more: boolean',
      'next_cursor?: string',
    ]);
    expect(client.shape('WorkbenchCursorOptions')).toEqual([
      'cursor?: string',
      'limit?: number',
    ]);

    const messageRequest = client.declaration('ListWorkbenchMessagesRequest');
    expect(messageRequest).toContain('before_seq?: string; after_seq?: never');
    expect(messageRequest).toContain('before_seq?: never; after_seq?: string');
    expect(messageRequest).not.toMatch(
      /WorkbenchPageOptions|WorkbenchCursorOptions/,
    );

    const mixedRequests = collectNodes(
      'workbench-thread-client.ts',
      source,
      node => {
        if (
          !ts.isInterfaceDeclaration(node) ||
          !node.name.text.endsWith('Request')
        ) {
          return [];
        }
        const bases =
          node.heritageClauses?.flatMap(clause =>
            clause.types.map(type => type.expression.getText()),
          ) ?? [];
        return bases.includes('WorkbenchPageOptions') &&
          bases.includes('WorkbenchCursorOptions')
          ? [node.name.text]
          : [];
      },
    );
    expect(mixedRequests).toEqual([]);

    expect(client.declaration('ListWorkbenchRunEventsRequest')).toMatch(
      /WorkbenchRunRequest.*WorkbenchCursorOptions/s,
    );
    expect(client.declaration('SubscribeWorkbenchRunEventsRequest')).toMatch(
      /WorkbenchRunRequest.*WorkbenchCursorOptions/s,
    );
    expect(client.property('WorkbenchThreadClient', 'listMessages')).toContain(
      'Promise<WorkbenchMessageCursorPage>',
    );
    expect(client.property('WorkbenchThreadClient', 'listRunEvents')).toContain(
      'Promise<WorkbenchCursorPage<WorkbenchRunEvent>>',
    );
  });

  it('uses distinct memory writes and subagent retry naming', () => {
    const source = readProductionSource(productionFiles[1]);
    const types = inspectTypes('workbench-thread-client.ts', source);
    expect(types.shape('WorkbenchMemoryUpdate')).toEqual(['scope: string']);
    expect(types.shape('WorkbenchMemoryImportItem')).toEqual([
      'scope?: string',
    ]);

    const client = types.declaration('WorkbenchThreadClient');
    expect(client).toContain('retrySubagentRun:');
    expect(client).toContain('RetryWorkbenchSubagentRunRequest');
    expect(client).not.toMatch(/retryRun|RetryWorkbenchRunRequest/);
  });

  it('exposes explicit app-owned turn metadata and top-level retry fields', () => {
    const source = readProductionSource(productionFiles[1]);
    const types = inspectTypes('workbench-thread-client.ts', source);
    expect(types.shape('CreateWorkbenchRunRequest')).toEqual(
      expect.arrayContaining([
        "attempt_kind?: 'turn' | 'retry'",
        'source_run_id?: string',
        'message_metadata?: string',
      ]),
    );
    expect(canonicalRunSubmissionExtensions).toEqual({
      turn: {
        message_metadata: { source: 'workbench_detail_followup' },
      },
      retry: {
        attempt_kind: 'retry',
        source_run_id: '3001',
      },
    });
    expect(Object.isFrozen(canonicalRunSubmissionExtensions)).toBe(true);
  });

  it('keeps canonical Run coze fields aligned across IDL and generated outputs', () => {
    const idl = readProductionSource(canonicalRunContractFiles.idl);
    const go = readProductionSource(canonicalRunContractFiles.go);
    const typescript = readProductionSource(
      canonicalRunContractFiles.typescript,
    );

    expect(idl).toMatch(
      /struct CreateCanonicalRunRequest \{[^}]*26: optional string coze \(api\.body="coze", api\.value_type="any"\)/,
    );
    expect(idl).toMatch(
      /struct WaitCanonicalRunRequest \{[^}]*27: optional string coze \(api\.body="coze", api\.value_type="any"\)/,
    );
    expect(go).toMatch(
      /type CreateCanonicalRunRequest struct \{[^}]*Coze\s+\*string\s+`[^`]*json:"coze,omitempty"`/,
    );
    expect(go).toMatch(
      /type WaitCanonicalRunRequest struct \{[^}]*Coze\s+\*string\s+`[^`]*json:"coze,omitempty"`/,
    );
    expect(typescript).toMatch(
      /export interface CreateCanonicalRunRequest \{[^}]*\n  coze\?: any,/,
    );
    expect(typescript).toMatch(
      /export interface WaitCanonicalRunRequest \{[^}]*\n  coze\?: any,/,
    );
  });

  it('keeps one frozen V1/canonical pair per visible resource family', () => {
    expect(Object.keys(pairedTransportFixtures)).toEqual(
      (
        'thread todo message run run_event upload artifact artifact_scan_job ' +
        'token_usage memory memory_audit guardrail_audit mcp_runtime_audit ' +
        'human_interaction'
      ).split(' '),
    );

    for (const fixture of Object.values(pairedTransportFixtures)) {
      expect(
        [fixture.v1, fixture.canonical, fixture.visible].every(Object.isFrozen),
      ).toBe(true);
      expect(JSON.stringify(fixture.canonical)).not.toContain('"space_id"');
    }
  });

  it('uses the exact V1 upload response shape before normalization', () => {
    expect(pairedTransportFixtures.upload.v1.data).toMatchObject({
      success: true,
      message: 'uploaded',
      skipped_files: [],
    });
    expect(pairedTransportFixtures.upload.v1.data.files[0]).toEqual({
      file_id: '6001',
      filename: 'brief.md',
      path: '/uploads/6001',
      virtual_path: '/brief.md',
      content_type: 'text/markdown',
      size: 128,
      created_at: 1767225600000,
    });
    expect(pairedTransportFixtures.upload.v1.data.files[0]).not.toHaveProperty(
      'file_name',
    );
    expect(pairedTransportFixtures.upload.v1.data.files[0]).not.toHaveProperty(
      'size_bytes',
    );
  });

  it('uses the reviewed canonical Thread and Run wire shapes', () => {
    const thread = pairedTransportFixtures.thread.canonical;
    expect(Object.keys(thread).sort()).toEqual([
      'coze',
      'created_at',
      'interrupts',
      'metadata',
      'status',
      'thread_id',
      'updated_at',
      'values',
    ]);
    expect(thread.metadata).toEqual({ title: 'Prepare launch brief' });
    expect(thread.status).toBe('busy');
    expect(thread.interrupts).toEqual({});
    expect(Object.keys(thread.coze).sort()).toEqual([
      'can_edit',
      'initial_submission',
      'last_agent_message',
      'last_user_message',
      'product_status',
      'progress',
      'source',
    ]);
    expect(thread.coze).not.toHaveProperty('creator_id');
    expect(thread.coze).not.toHaveProperty('title');
    expect(thread.coze).toMatchObject({
      product_status: 'running',
      initial_submission: null,
      source: 'web',
    });

    const submittedThread = structuredClone(thread) as unknown as {
      coze: { initial_submission: unknown };
    };
    submittedThread.coze.initial_submission = {
      role: 'user',
      content: 'Prepare the launch brief',
    };
    expect(projectCanonicalTransportFixture('thread', submittedThread)).toEqual(
      pairedTransportFixtures.thread.visible,
    );

    const run = pairedTransportFixtures.run.canonical;
    expect(Object.keys(run).sort()).toEqual([
      'assistant_id',
      'coze',
      'created_at',
      'metadata',
      'multitask_strategy',
      'run_id',
      'status',
      'thread_id',
      'updated_at',
    ]);
    expect(Object.keys(run.coze).sort()).toEqual([
      'attempt_kind',
      'durability',
      'ended_at',
      'message_id',
      'on_disconnect',
      'parent_run_id',
      'run_kind',
      'source_run_id',
      'started_at',
      'stream_modes',
      'terminal_reason',
    ]);
    expect(run.assistant_id).toBe('agent');
    expect(run.coze).toMatchObject({
      attempt_kind: 'turn',
      run_kind: 'task',
      message_id: null,
      source_run_id: null,
      parent_run_id: null,
      terminal_reason: null,
      ended_at: null,
    });
    [thread.thread_id, run.run_id, run.thread_id].forEach(value =>
      expect(value).toMatch(/^\d+$/),
    );
  });

  it('keeps reviewed opaque identifiers as strings', () => {
    expect(pairedTransportFixtures.todo.visible.id).toBe('todo-1');
    expect(pairedTransportFixtures.run.v1.data.assistant_id).toBe(
      'assistant-a',
    );
    expect(pairedTransportFixtures.run.visible.assistant_id).toBe('agent');
    expect(
      pairedTransportFixtures.artifact_scan_job.canonical.jobs[0].worker_ref,
    ).toBe('worker-safe-1');
    expect(pairedTransportFixtures.token_usage.visible.items[0].step_id).toBe(
      'step-1',
    );
    expect(pairedTransportFixtures.memory.visible.source_id).toBe(
      'message:3001',
    );
    expect(pairedTransportFixtures.guardrail_audit.visible.target_id).toBe(
      'artifact:5001',
    );
    expect(pairedTransportFixtures.mcp_runtime_audit.visible.server_id).toBe(
      'server-main',
    );
    expect(pairedTransportFixtures.human_interaction.visible).toMatchObject({
      interaction_id: 'hi_1',
      choice_id: 'a',
    });
    expect(
      pairedTransportFixtures.run_event.v1.data.journal_messages[0].id,
    ).toBe('journal-1');

    const scan = structuredClone(
      pairedTransportFixtures.artifact_scan_job.canonical,
    );
    setWirePath(scan, { path: ['jobs', 0, 'worker_ref'], value: '' });
    expect(
      projectCanonicalTransportFixture('artifact_scan_job', scan),
    ).toMatchObject({ worker_id: '' });

    const memory = structuredClone(pairedTransportFixtures.memory.canonical);
    setWirePath(memory, { path: ['memories', 0, 'source_id'], value: '' });
    expect(projectCanonicalTransportFixture('memory', memory)).toMatchObject({
      source_id: '',
    });

    const mcpAudit = structuredClone(
      pairedTransportFixtures.mcp_runtime_audit.canonical,
    );
    setWirePath(mcpAudit, { path: ['events', 0, 'server_id'], value: '' });
    expect(
      projectCanonicalTransportFixture('mcp_runtime_audit', mcpAudit),
    ).toMatchObject({ server_id: '' });
  });

  it('models only the canonical human interaction resume contract', () => {
    const fixture = pairedTransportFixtures.human_interaction;
    const publicKeys = [
      'choice_id',
      'comment',
      'decision',
      'interaction_id',
      'kind',
      'schema',
    ];

    expect(Object.keys(fixture.v1).sort()).toEqual(publicKeys);
    expect(Object.keys(fixture.canonical).sort()).toEqual(publicKeys);
    expect(Object.keys(fixture.visible).sort()).toEqual(publicKeys);
    expect(fixture.canonical).toMatchObject({
      schema: 'coze.human_interaction_response.v1',
      interaction_id: 'hi_1',
      kind: 'confirmation',
      decision: 'approved',
      choice_id: 'a',
      comment: 'Proceed',
    });

    const forbiddenFields = [
      ['submitted_by', 'user:8601'],
      ['submitted_at', '2026-01-01T00:00:00Z'],
      ['source', 'web'],
    ] as const;
    forbiddenFields.forEach(([field, value]) => {
      const wire = structuredClone(fixture.canonical);
      setWirePath(wire, { path: [field], value });
      expect(() =>
        projectCanonicalTransportFixture('human_interaction', wire),
      ).toThrow(field);
    });
  });

  it('projects every V1 and canonical fixture to the same visible model', () => {
    for (const [family, fixture] of Object.entries(pairedTransportFixtures)) {
      const fixtureFamily = family as TransportFixtureFamily;

      expect([
        projectV1TransportFixture(fixtureFamily, fixture.v1),
        projectCanonicalTransportFixture(fixtureFamily, fixture.canonical),
      ]).toEqual([fixture.visible, fixture.visible]);
    }
  });

  it('normalizes absent canonical title and todos without presenter labels', () => {
    const v1 = structuredClone(pairedTransportFixtures.thread.v1);
    const canonical = structuredClone(pairedTransportFixtures.thread.canonical);
    setWirePath(v1, { path: ['data', 'title'], value: '' });
    delete (getWirePath(canonical, ['metadata']) as Record<string, unknown>)
      .title;
    delete (getWirePath(v1, ['data', 'values']) as Record<string, unknown>)
      .todos;
    delete (getWirePath(canonical, ['values']) as Record<string, unknown>)
      .todos;

    const projections = [
      projectV1TransportFixture('thread', v1),
      projectCanonicalTransportFixture('thread', canonical),
    ] as Array<Record<string, unknown>>;
    expect(projections[0]).toEqual(projections[1]);
    projections.forEach(projection => {
      expect(projection.title).toBe('');
      expect(projection).not.toHaveProperty('values');
    });
  });

  it('makes wire mismatches observable while dropping reviewed private data', () => {
    const uploadWire = structuredClone(pairedTransportFixtures.upload.v1);
    const uploadData = uploadWire.data as { success?: boolean };
    delete uploadData.success;
    expect(() => projectV1TransportFixture('upload', uploadWire)).toThrow(
      'success',
    );

    const tokenWire = structuredClone(
      pairedTransportFixtures.token_usage.canonical,
    ) as { aggregate: { total_tokens: number } };
    tokenWire.aggregate.total_tokens += 1;
    expect(
      projectCanonicalTransportFixture('token_usage', tokenWire),
    ).not.toEqual(pairedTransportFixtures.token_usage.visible);

    const privateWire = structuredClone(
      pairedTransportFixtures.token_usage.v1,
    ) as unknown as { data: { usage: Array<{ raw_usage?: string }> } };
    delete privateWire.data.usage[0].raw_usage;
    expect(() => projectV1TransportFixture('token_usage', privateWire)).toThrow(
      'raw_usage',
    );

    const projections = [
      projectV1TransportFixture('thread', pairedTransportFixtures.thread.v1),
      projectV1TransportFixture('run', pairedTransportFixtures.run.v1),
      projectV1TransportFixture(
        'token_usage',
        pairedTransportFixtures.token_usage.v1,
      ),
      projectV1TransportFixture(
        'run_event',
        pairedTransportFixtures.run_event.v1,
      ),
    ];
    expect(JSON.stringify(projections)).not.toMatch(
      /creator_id|worker_id|error_message|raw_usage|must be dropped|journal_messages|tool_calls/,
    );
  });

  it('rejects malformed IDs, numbers, dates, and JSON shapes', () => {
    const cases: Array<[TransportFixtureFamily, WirePath, unknown, string]> = [
      ['thread', ['coze', 'can_edit'], 'true', 'can_edit'],
      ['message', ['message_id'], Number.MAX_SAFE_INTEGER + 1, 'message_id'],
      ['message', ['thread_id'], 'thread-opaque', 'thread_id'],
      [
        'token_usage',
        ['aggregate', 'total_tokens'],
        Number.POSITIVE_INFINITY,
        'total_tokens',
      ],
      ['thread', ['created_at'], Number.NaN, 'created_at'],
      ['message', ['created_at'], 'not-a-time', 'created_at'],
      ['message', ['created_at'], '2026-02-30T00:00:00Z', 'created_at'],
      ['message', ['created_at'], '2025-02-29T12:00:00+08:00', 'created_at'],
      ['message', ['metadata'], '{invalid', 'metadata'],
      ['message', ['metadata'], [], 'metadata'],
      ['guardrail_audit', ['events', 0, 'rule_ids'], {}, 'rule_ids'],
      ['upload', ['uploads', 0, 'size_bytes'], '128', 'size_bytes'],
    ];

    cases.forEach(([family, path, value, field]) => {
      const wire = structuredClone(pairedTransportFixtures[family].canonical);
      setWirePath(wire, { path, value });
      expect(() => projectCanonicalTransportFixture(family, wire)).toThrow(
        field,
      );
    });
  });

  it('accepts a valid RFC3339 leap day with an offset', () => {
    const wire = structuredClone(pairedTransportFixtures.message.canonical);
    setWirePath(wire, {
      path: ['created_at'],
      value: '2024-02-29T12:34:56+08:00',
    });

    expect(projectCanonicalTransportFixture('message', wire)).toMatchObject({
      created_at: Date.UTC(2024, 1, 29, 4, 34, 56),
    });
  });

  it('rejects values encoded for the wrong transport', () => {
    type WireSide = 'v1' | 'canonical';
    const cases: Array<
      [WireSide, TransportFixtureFamily, WirePath, unknown, string]
    > = [
      ['canonical', 'message', ['message_id'], 2001, 'message_id'],
      ['v1', 'message', ['data', 'message_id'], 2001, 'message_id'],
      ['canonical', 'message', ['message_id'], '0', 'message_id'],
      [
        'v1',
        'message',
        ['data', 'created_at'],
        '2026-01-01T00:00:00Z',
        'created_at',
      ],
      ['canonical', 'message', ['created_at'], 1767225600000, 'created_at'],
      [
        'v1',
        'message',
        ['data', 'metadata'],
        { channel: 'workbench' },
        'metadata',
      ],
      [
        'canonical',
        'message',
        ['metadata'],
        '{"channel":"workbench"}',
        'metadata',
      ],
      [
        'canonical',
        'message',
        ['metadata'],
        { nested: Number.POSITIVE_INFINITY },
        'metadata',
      ],
      ['v1', 'message', ['data', 'metadata'], '{"nested":1e999}', 'metadata'],
      ['v1', 'message', ['data', 'metadata'], '{invalid', 'metadata'],
      ['v1', 'todo', ['data', 'id'], 1101, 'id'],
      [
        'canonical',
        'artifact_scan_job',
        ['jobs', 0, 'worker_ref'],
        8101,
        'worker_ref',
      ],
      ['canonical', 'token_usage', ['usage', 0, 'step_id'], 8301, 'step_id'],
      ['canonical', 'memory', ['memories', 0, 'source_id'], 2001, 'source_id'],
      [
        'canonical',
        'mcp_runtime_audit',
        ['events', 0, 'server_id'],
        9002,
        'server_id',
      ],
      ['canonical', 'thread', ['coze', 'source'], 'agent', 'source'],
      [
        'canonical',
        'thread',
        ['coze', 'initial_submission'],
        'message',
        'initial_submission',
      ],
      ['canonical', 'thread', ['interrupts'], [], 'interrupts'],
      ['v1', 'run', ['data', 'parent_run_id'], null, 'parent_run_id'],
      ['v1', 'run', ['data', 'ended_at'], null, 'ended_at'],
      [
        'canonical',
        'human_interaction',
        ['interaction_id'],
        '',
        'interaction_id',
      ],
    ];

    cases.forEach(([side, family, path, value, field]) => {
      const wire = structuredClone(pairedTransportFixtures[family][side]);
      setWirePath(wire, { path, value });
      const project =
        side === 'v1'
          ? projectV1TransportFixture
          : projectCanonicalTransportFixture;
      expect(() => project(family, wire)).toThrow(field);
    });
  });

  it('omits absent lifecycle values instead of inventing defaults', () => {
    const cases: Array<[TransportFixtureFamily, WirePath, string[]]> = [
      ['artifact', ['artifacts', 0], ['deleted_at']],
      [
        'artifact_scan_job',
        ['jobs', 0],
        ['available_at', 'started_at', 'ended_at'],
      ],
      [
        'memory',
        ['memories', 0],
        [
          'run_id',
          'correction_of_memory_id',
          'corrected_at',
          'expires_at',
          'deleted_at',
        ],
      ],
      ['memory_audit', ['events', 0], ['run_id', 'memory_id', 'actor_id']],
      ['guardrail_audit', ['events', 0], ['run_id', 'actor_id']],
      ['mcp_runtime_audit', ['events', 0], ['run_id', 'server_id']],
    ];

    cases.forEach(([family, path, fields]) => {
      const wire = structuredClone(pairedTransportFixtures[family].canonical);
      const resource = getWirePath(wire, path) as Record<string, unknown>;
      fields.forEach(field => delete resource[field]);

      const projected = projectCanonicalTransportFixture(
        family,
        wire,
      ) as Record<string, unknown>;
      fields.forEach(field => expect(projected).not.toHaveProperty(field));
    });
  });

  it('keeps the legacy envelope helper test-only and side-effect free', () => {
    expect(legacyTaskThreadReference.envelope).toBe('code_msg_data');
    expect(
      unwrapLegacyTaskThreadResponse({
        code: 0,
        msg: 'success',
        data: { thread_id: '1001' },
      }),
    ).toEqual({ thread_id: '1001' });
  });
});
