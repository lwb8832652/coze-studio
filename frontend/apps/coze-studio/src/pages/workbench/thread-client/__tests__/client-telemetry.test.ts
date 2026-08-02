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

import { describe, expect, it, vi } from 'vitest';

import {
  observeWorkbenchClientOperation,
  workbenchClientIdentifiers,
  type WorkbenchClientTelemetryEvent,
} from '../client-telemetry';
import { CanonicalThreadCoreClient } from '../canonical-thread-client';
import { WorkbenchClientError, type CanonicalFetch } from '../canonical-fetch';
import {
  memoryTransportFixture,
  messageTransportFixture,
  runTransportFixture,
  uploadTransportFixture,
} from './fixtures';

const spaceID = '9001';
const threadID = '1001';
const runID = '3001';
const fileID = '6001';
const artifactID = '7001';
const memoryID = '8401';

const secrets = {
  message: 'MESSAGE-CONTENT-DO-NOT-LOG',
  memory: 'MEMORY-CONTENT-DO-NOT-LOG',
  filename: 'employee-liuwenbo-private-brief.txt',
  signedURL:
    'https://objects.example.test/private?X-Amz-Signature=SIGNED-SECRET',
  credential: 'Bearer sk-production-secret-value',
  credentialClass: 'CredentialSecret',
  credentialCode: 'access_token_secret',
  credentialTrace: 'sk-production-secret-value',
  toolResult: '{"tool_result":"DATABASE-ROW-SECRET"}',
};

const jsonResponse = (body: unknown, status = 200): Response =>
  new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json' },
  });

const queuedFetch = (...responses: Response[]) =>
  vi.fn(() => {
    const response = responses.shift();
    if (!response) {
      throw new Error('Missing queued response');
    }
    return Promise.resolve(response);
  });

const runCreationResponse = () => {
  const run = structuredClone(
    runTransportFixture.canonical,
  ) as unknown as Record<string, unknown>;
  const message = structuredClone(
    messageTransportFixture.canonical,
  ) as unknown as Record<string, unknown>;
  message.role = 'user';
  message.content = secrets.message;
  const coze = run.coze as Record<string, unknown>;
  coze.message_id = message.message_id;
  coze.submission_message = message;
  return run;
};

const expectSecretsAbsent = (value: unknown) => {
  const serialized = JSON.stringify(value);
  Object.values(secrets).forEach(secret => {
    expect(serialized).not.toContain(secret);
  });
};

describe('Workbench client operation telemetry', () => {
  it('reads only own data properties while extracting resource IDs', () => {
    const request = Object.defineProperty(
      { thread_id: threadID },
      'artifact_id',
      {
        enumerable: true,
        get: () => {
          throw new Error('telemetry must not invoke request getters');
        },
      },
    );

    expect(workbenchClientIdentifiers(request)).toEqual({
      thread_id: threadID,
    });
  });

  it('records only allowlisted success fields and never inspects operation values', async () => {
    const events: WorkbenchClientTelemetryEvent[] = [];
    const now = vi.fn().mockReturnValueOnce(100).mockReturnValueOnce(137);
    const sensitiveResult = { ...secrets };

    await expect(
      observeWorkbenchClientOperation({
        telemetry: event => events.push(event),
        operation: 'createRun',
        identifiers: {
          thread_id: threadID,
          run_id: runID,
          artifact_id: artifactID,
        },
        now,
        execute: () => Promise.resolve(sensitiveResult),
      }),
    ).resolves.toBe(sensitiveResult);

    expect(events).toEqual([
      {
        eventName: 'workbench_thread_client_operation',
        meta: {
          client_contract: 'canonical_v1',
          operation: 'createRun',
          thread_id: threadID,
          run_id: runID,
          artifact_id: artifactID,
          duration_ms: 37,
          outcome: 'success',
        },
      },
    ]);
    expectSecretsAbsent(events);
  });

  it('keeps a canonical code and trace but drops both canonical and unexpected messages', async () => {
    const events: WorkbenchClientTelemetryEvent[] = [];
    const telemetry = (event: WorkbenchClientTelemetryEvent) =>
      events.push(event);

    await expect(
      observeWorkbenchClientOperation({
        telemetry,
        operation: 'getArtifactSignedURL',
        identifiers: { thread_id: threadID, artifact_id: artifactID },
        execute: () =>
          Promise.reject(
            new WorkbenchClientError({
              message: secrets.signedURL,
              status: 429,
              code: 'rate_limited',
              traceId: 'trace-safe-429',
              retryable: true,
              outcome: 'rejected',
            }),
          ),
      }),
    ).rejects.toMatchObject({ message: secrets.signedURL });
    await expect(
      observeWorkbenchClientOperation({
        telemetry,
        operation: 'updateMemory',
        identifiers: { thread_id: threadID, memory_id: memoryID },
        execute: () => {
          const error = new TypeError(secrets.credential);
          error.name = secrets.credentialClass;
          return Promise.reject(error);
        },
      }),
    ).rejects.toThrow(secrets.credential);
    await expect(
      observeWorkbenchClientOperation({
        telemetry,
        operation: 'getArtifactSignedURL',
        identifiers: { thread_id: threadID, artifact_id: artifactID },
        execute: () =>
          Promise.reject(
            new WorkbenchClientError({
              message: secrets.toolResult,
              code: secrets.credentialCode,
              traceId: secrets.credentialTrace,
              retryable: false,
              outcome: 'failed',
            }),
          ),
      }),
    ).rejects.toThrow(secrets.toolResult);

    expect(events[0].meta).toMatchObject({
      outcome: 'rejected',
      error_code: 'rate_limited',
      trace_id: 'trace-safe-429',
    });
    expect(events[0].meta).not.toHaveProperty('error_class');
    expect(events[1].meta).toMatchObject({
      outcome: 'failed',
      error_code: 'unexpected_error',
      error_class: 'Error',
    });
    expect(events[2].meta).toMatchObject({
      outcome: 'failed',
      error_code: 'invalid_error_code',
    });
    expect(events[2].meta).not.toHaveProperty('trace_id');
    expectSecretsAbsent(events);
  });

  it('redacts request bodies, files, signed responses, credentials, and tool results across real client operations', async () => {
    const signedResponse = {
      artifact_id: artifactID,
      url: secrets.signedURL,
      expires_in_seconds: 60,
      content_type: 'text/plain',
      preview_mode: 'text',
    };
    const fetchMock = queuedFetch(
      jsonResponse(runCreationResponse()),
      jsonResponse(messageTransportFixture.canonical),
      jsonResponse(uploadTransportFixture.canonical),
      jsonResponse(signedResponse),
      jsonResponse({
        memory: memoryTransportFixture.canonical.memories[0],
        updated: true,
      }),
    );
    const events: WorkbenchClientTelemetryEvent[] = [];
    const client = new CanonicalThreadCoreClient({
      fetch: fetchMock as unknown as CanonicalFetch,
      telemetry: event => events.push(event),
    });

    await client.createRun({
      space_id: spaceID,
      thread_id: threadID,
      input: JSON.stringify({
        uploaded_files: [],
        tool_result: secrets.toolResult,
      }),
      message_content: secrets.message,
      metadata: JSON.stringify({ credential: secrets.credential }),
    });
    await client.appendMessage({
      space_id: spaceID,
      thread_id: threadID,
      run_id: runID,
      role: 'assistant',
      content: secrets.message,
    });
    await client.uploadFiles({
      space_id: spaceID,
      thread_id: threadID,
      files: [new File(['private bytes'], secrets.filename)],
    });
    await client.getArtifactSignedURL({
      space_id: spaceID,
      thread_id: threadID,
      artifact_id: artifactID,
      mode: 'download',
    });
    await client.updateMemory({
      space_id: spaceID,
      thread_id: threadID,
      run_id: runID,
      memory_id: memoryID,
      scope: 'thread',
      content: secrets.memory,
      metadata: JSON.stringify({ credential: secrets.credential }),
    });

    expect(fetchMock).toHaveBeenCalledTimes(5);
    expect(events.map(event => event.meta.operation)).toEqual([
      'createRun',
      'appendMessage',
      'uploadFiles',
      'getArtifactSignedURL',
      'updateMemory',
    ]);
    events.forEach(event => {
      expect(Object.keys(event.meta).sort()).toEqual(
        expect.arrayContaining([
          'client_contract',
          'duration_ms',
          'operation',
          'outcome',
          'thread_id',
        ]),
      );
    });
    expectSecretsAbsent(events);
  });

  it('does not let a telemetry sink failure change the business result', async () => {
    await expect(
      observeWorkbenchClientOperation({
        telemetry: () => {
          throw new Error('telemetry unavailable');
        },
        operation: 'getRun',
        identifiers: { thread_id: threadID, run_id: runID, file_id: fileID },
        execute: () => Promise.resolve('business-result'),
      }),
    ).resolves.toBe('business-result');
  });
});
