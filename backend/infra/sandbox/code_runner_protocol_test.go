// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
)

func TestCodeRunnerInvokeEnvelopeUsesStrictBoundedContract(t *testing.T) {
	params := json.RawMessage(`{"query":"safe input"}`)
	wire, err := MarshalCodeRunnerInvokeEnvelope(CodeRunnerInvokeEnvelope{
		Schema:        CodeRunnerInvokeSchemaV1,
		ClientVersion: CodeRunnerClientVersionV1,
		OperationID:   "operation_test",
		Language:      "Python",
		Code:          "async def main(args):\n    return {'ok': True}",
		Params:        params,
	})
	if err != nil {
		t.Fatalf("MarshalCodeRunnerInvokeEnvelope() error = %v", err)
	}
	parsed, err := ParseCodeRunnerInvokeEnvelope(wire, len(wire))
	if err != nil {
		t.Fatalf("ParseCodeRunnerInvokeEnvelope() error = %v", err)
	}
	if parsed.OperationID != "operation_test" || parsed.Language != "Python" ||
		string(parsed.Params) != string(params) {
		t.Fatalf("parsed envelope = %#v", parsed)
	}

	for name, body := range map[string][]byte{
		"unknown field": []byte(`{"schema":"coze.sandbox.code.invoke.v1","client_version":"1","operation_id":"operation_test","language":"Python","code":"pass","params":{},"secret":"leak"}`),
		"duplicate":     []byte(`{"schema":"coze.sandbox.code.invoke.v1","client_version":"1","operation_id":"operation_test","language":"Python","language":"Python","code":"pass","params":{}}`),
		"array params":  []byte(`{"schema":"coze.sandbox.code.invoke.v1","client_version":"1","operation_id":"operation_test","language":"Python","code":"pass","params":[]}`),
		"oversized":     []byte(strings.Repeat("x", MaxCodeRunnerEnvelopeBytes+1)),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseCodeRunnerInvokeEnvelope(body, MaxCodeRunnerEnvelopeBytes); !errors.Is(err, domainsandbox.ErrInvalidInput) {
				t.Fatalf("ParseCodeRunnerInvokeEnvelope() error = %v", err)
			}
		})
	}
}

func TestCodeRunnerResultEnvelopeReturnsOnlyReviewedResult(t *testing.T) {
	wire, err := MarshalCodeRunnerResultEnvelope(map[string]any{
		"ok": true,
		"nested": map[string]any{
			"value": "safe",
		},
	})
	if err != nil {
		t.Fatalf("MarshalCodeRunnerResultEnvelope() error = %v", err)
	}
	result, err := ParseCodeRunnerResultEnvelope(wire, len(wire))
	if err != nil {
		t.Fatalf("ParseCodeRunnerResultEnvelope() error = %v", err)
	}
	if result["ok"] != true {
		t.Fatalf("result = %#v", result)
	}

	for name, body := range map[string][]byte{
		"unknown":   []byte(`{"schema":"coze.sandbox.code.result.v1","result":{},"execution_id":"secret"}`),
		"duplicate": []byte(`{"schema":"coze.sandbox.code.result.v1","result":{},"result":{}}`),
		"array":     []byte(`{"schema":"coze.sandbox.code.result.v1","result":[]}`),
		"malformed": []byte(`{"schema":`),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseCodeRunnerResultEnvelope(body, MaxCodeRunnerEnvelopeBytes); !errors.Is(err, domainsandbox.ErrInvalidInput) {
				t.Fatalf("ParseCodeRunnerResultEnvelope() error = %v", err)
			}
		})
	}
}
