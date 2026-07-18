// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"unicode/utf8"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
)

const (
	CodeRunnerInvokeSchemaV1   = "coze.sandbox.code.invoke.v1"
	CodeRunnerResultSchemaV1   = "coze.sandbox.code.result.v1"
	CodeRunnerClientVersionV1  = "1"
	MaxCodeRunnerEnvelopeBytes = 1024 * 1024

	maxCodeRunnerCodeBytes   = 256 * 1024
	maxCodeRunnerParamsBytes = 256 * 1024
)

type CodeRunnerInvokeEnvelope struct {
	Schema        string          `json:"schema"`
	ClientVersion string          `json:"client_version"`
	OperationID   string          `json:"operation_id"`
	Language      string          `json:"language"`
	Code          string          `json:"code"`
	Params        json.RawMessage `json:"params"`
}

type CodeRunnerResultEnvelope struct {
	Schema string          `json:"schema"`
	Result json.RawMessage `json:"result"`
}

func NormalizeCodeRunnerInvokeEnvelope(
	input CodeRunnerInvokeEnvelope,
) (CodeRunnerInvokeEnvelope, error) {
	if input.Schema != CodeRunnerInvokeSchemaV1 ||
		input.ClientVersion != CodeRunnerClientVersionV1 ||
		!validIdentifier(input.OperationID) ||
		(input.Language != "Python" && input.Language != "JavaScript") ||
		input.Code == "" ||
		len(input.Code) > maxCodeRunnerCodeBytes ||
		!utf8.ValidString(input.Code) ||
		strings.IndexByte(input.Code, 0) >= 0 {
		return CodeRunnerInvokeEnvelope{}, domainsandbox.ErrInvalidInput
	}
	params, err := normalizeCodeRunnerJSONObject(input.Params, maxCodeRunnerParamsBytes)
	if err != nil {
		return CodeRunnerInvokeEnvelope{}, err
	}
	return CodeRunnerInvokeEnvelope{
		Schema:        CodeRunnerInvokeSchemaV1,
		ClientVersion: CodeRunnerClientVersionV1,
		OperationID:   input.OperationID,
		Language:      input.Language,
		Code:          input.Code,
		Params:        params,
	}, nil
}

func MarshalCodeRunnerInvokeEnvelope(input CodeRunnerInvokeEnvelope) ([]byte, error) {
	normalized, err := NormalizeCodeRunnerInvokeEnvelope(input)
	if err != nil {
		return nil, err
	}
	wire, err := json.Marshal(normalized)
	if err != nil || len(wire) > MaxCodeRunnerEnvelopeBytes {
		return nil, domainsandbox.ErrInvalidInput
	}
	return wire, nil
}

func ParseCodeRunnerInvokeEnvelope(
	body []byte,
	maxBytes int,
) (CodeRunnerInvokeEnvelope, error) {
	if !validCodeRunnerEnvelopeSize(body, maxBytes) {
		return CodeRunnerInvokeEnvelope{}, domainsandbox.ErrInvalidInput
	}
	var wire CodeRunnerInvokeEnvelope
	if err := decodeStrictCodeRunnerJSON(body, &wire); err != nil {
		return CodeRunnerInvokeEnvelope{}, domainsandbox.ErrInvalidInput
	}
	return NormalizeCodeRunnerInvokeEnvelope(wire)
}

func MarshalCodeRunnerResultEnvelope(result map[string]any) ([]byte, error) {
	if result == nil {
		result = map[string]any{}
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return nil, domainsandbox.ErrInvalidInput
	}
	normalized, err := normalizeCodeRunnerJSONObject(raw, MaxCodeRunnerEnvelopeBytes)
	if err != nil {
		return nil, err
	}
	wire, err := json.Marshal(CodeRunnerResultEnvelope{
		Schema: CodeRunnerResultSchemaV1,
		Result: normalized,
	})
	if err != nil || len(wire) > MaxCodeRunnerEnvelopeBytes {
		return nil, domainsandbox.ErrInvalidInput
	}
	return wire, nil
}

func ParseCodeRunnerResultEnvelope(
	body []byte,
	maxBytes int,
) (map[string]any, error) {
	if !validCodeRunnerEnvelopeSize(body, maxBytes) {
		return nil, domainsandbox.ErrInvalidInput
	}
	var wire CodeRunnerResultEnvelope
	if err := decodeStrictCodeRunnerJSON(body, &wire); err != nil ||
		wire.Schema != CodeRunnerResultSchemaV1 {
		return nil, domainsandbox.ErrInvalidInput
	}
	raw, err := normalizeCodeRunnerJSONObject(wire.Result, maxBytes)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	result := map[string]any{}
	if err := decoder.Decode(&result); err != nil {
		return nil, domainsandbox.ErrInvalidInput
	}
	return result, nil
}

func validCodeRunnerEnvelopeSize(body []byte, maxBytes int) bool {
	if maxBytes <= 0 || maxBytes > MaxCodeRunnerEnvelopeBytes {
		maxBytes = MaxCodeRunnerEnvelopeBytes
	}
	return len(body) > 0 && len(body) <= maxBytes
}

func normalizeCodeRunnerJSONObject(
	raw json.RawMessage,
	maxBytes int,
) (json.RawMessage, error) {
	if len(raw) == 0 || len(raw) > maxBytes || len(raw) > MaxCodeRunnerEnvelopeBytes {
		return nil, domainsandbox.ErrInvalidInput
	}
	if err := rejectDuplicateCodeRunnerJSONKeys(raw); err != nil {
		return nil, domainsandbox.ErrInvalidInput
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var object map[string]any
	if err := decoder.Decode(&object); err != nil || object == nil {
		return nil, domainsandbox.ErrInvalidInput
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, domainsandbox.ErrInvalidInput
	}
	normalized, err := json.Marshal(object)
	if err != nil || len(normalized) > maxBytes {
		return nil, domainsandbox.ErrInvalidInput
	}
	return normalized, nil
}

func decodeStrictCodeRunnerJSON(body []byte, output any) error {
	if err := rejectDuplicateCodeRunnerJSONKeys(body); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	if err := decoder.Decode(output); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return domainsandbox.ErrInvalidInput
	}
	return nil
}

func rejectDuplicateCodeRunnerJSONKeys(body []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := scanCodeRunnerJSONValue(decoder); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return domainsandbox.ErrInvalidInput
	}
	return nil
}

func scanCodeRunnerJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		keys := map[string]struct{}{}
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return domainsandbox.ErrInvalidInput
			}
			if _, duplicate := keys[key]; duplicate {
				return domainsandbox.ErrInvalidInput
			}
			keys[key] = struct{}{}
			if err := scanCodeRunnerJSONValue(decoder); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim('}') {
			return domainsandbox.ErrInvalidInput
		}
	case '[':
		for decoder.More() {
			if err := scanCodeRunnerJSONValue(decoder); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim(']') {
			return domainsandbox.ErrInvalidInput
		}
	default:
		return domainsandbox.ErrInvalidInput
	}
	return nil
}
