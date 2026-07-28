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
  pairedTransportFixtures,
  projectCanonicalTransportFixture,
  projectV1TransportFixture,
  type TransportFixtureFamily,
} from './fixtures';

const productionFiles = [
  resolve(__dirname, '../types.ts'),
  resolve(__dirname, '../workbench-thread-client.ts'),
  resolve(__dirname, '../index.ts'),
];

const readProductionSource = (file: string) =>
  existsSync(file) ? readFileSync(file, 'utf8') : '';

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
  const value =
    ts.isStringLiteral(node) || ts.isNoSubstitutionTemplateLiteral(node)
      ? node.text
      : templateTokens.has(node.kind)
        ? (node as ts.TemplateLiteralToken).text
        : '';
  return value.includes('/api/') || value.includes('task_threads')
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
  visit(
    ts.createSourceFile(
      fileName,
      source,
      ts.ScriptTarget.Latest,
      true,
      ts.ScriptKind.TS,
    ),
  );
  return values;
};

const analyzeBoundarySource = (fileName: string, source: string): string[] =>
  collectNodes(fileName, source, node =>
    nodeChecks.flatMap(check => {
      const value = check(node);
      return value ? [value] : [];
    }),
  );

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
describe('WorkbenchThreadClient production boundary', () => {
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
      "new EventSource('/events');",
      "new window.EventSource('/events');",
      "new globalThis['EventSource']('/events');",
      'new globalThis.XMLHttpRequest();',
    ];
    probes.forEach(source =>
      expect(analyzeBoundarySource('probe.ts', source)).not.toEqual([]),
    );
    expect(
      analyzeBoundarySource(
        'safe.ts',
        'interface WorkbenchThreadRequest { thread_id: string }',
      ),
    ).toEqual([]);
  });

  it('exposes canonical_v1 as its only public contract literal', () => {
    expect(
      getContractLiterals(
        basename(productionFiles[1]),
        readProductionSource(productionFiles[1]),
      ),
    ).toEqual(['canonical_v1']);
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
      file_id: 'file-1',
      filename: 'brief.md',
      path: '/uploads/file-1',
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

  it('projects every V1 and canonical fixture to the same visible model', () => {
    for (const [family, fixture] of Object.entries(pairedTransportFixtures)) {
      const fixtureFamily = family as TransportFixtureFamily;

      expect([
        projectV1TransportFixture(fixtureFamily, fixture.v1),
        projectCanonicalTransportFixture(fixtureFamily, fixture.canonical),
      ]).toEqual([fixture.visible, fixture.visible]);
    }
  });

  it('makes wire mismatches observable while dropping reviewed private data', () => {
    const uploadWire = structuredClone(pairedTransportFixtures.upload.v1);
    const uploadData = uploadWire.data as { success?: boolean };
    delete uploadData.success;
    expect(() => projectV1TransportFixture('upload', uploadWire)).toThrow(
      'success',
    );

    const messageWire = structuredClone(
      pairedTransportFixtures.message.canonical,
    ) as { created_at: string };
    messageWire.created_at = 'not-a-time';
    expect(() =>
      projectCanonicalTransportFixture('message', messageWire),
    ).toThrow('created_at');

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
      /raw_usage|must be dropped|journal_messages|tool_calls/,
    );
  });

  it('keeps the legacy envelope helper test-only and side-effect free', () => {
    expect(legacyTaskThreadReference.envelope).toBe('code_msg_data');
    expect(
      unwrapLegacyTaskThreadResponse({
        code: 0,
        msg: 'success',
        data: { thread_id: 'thread-1' },
      }),
    ).toEqual({ thread_id: 'thread-1' });
  });
});
