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

package coze

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"

	hertzconsts "github.com/cloudwego/hertz/pkg/protocol/consts"
)

type canonicalExecutionControlIngressKind uint8

const (
	canonicalExecutionControlCreateThread canonicalExecutionControlIngressKind = iota
	canonicalExecutionControlRunSubmission
	canonicalExecutionControlRootOnly
)

const (
	canonicalExecutionControlMaxReservedHops     = 4
	canonicalExecutionControlMaxInspectedObjects = 64
	canonicalExecutionControlMaxPathBytes        = 256
	canonicalExecutionControlMaxParseDepth       = 128
	canonicalExecutionControlMaxParseUnits       = 65_536
)

var canonicalExecutionControlRetiredFields = [...]string{
	"requested_policy",
	"mode",
	"thinking_enabled",
	"reasoning_effort",
	"is_plan_mode",
	"subagent_enabled",
	"max_concurrent_subagents",
}

var (
	errCanonicalExecutionControlOrderedJSON = errors.New("invalid ordered JSON")
	errCanonicalExecutionControlParseBudget = errors.New("execution control JSON parse budget exceeded")
)

type canonicalExecutionControlJSONKind uint8

const (
	canonicalExecutionControlJSONScalar canonicalExecutionControlJSONKind = iota
	canonicalExecutionControlJSONObject
	canonicalExecutionControlJSONArray
)

type canonicalExecutionControlJSONField struct {
	name  string
	value *canonicalExecutionControlJSONValue
}

type canonicalExecutionControlJSONValue struct {
	kind   canonicalExecutionControlJSONKind
	fields []canonicalExecutionControlJSONField
}

type canonicalExecutionControlIngressBudget struct {
	inspectedObjects int
}

// The existing 1 MiB request-body limit bounds raw string and key bytes. These
// independent limits bound recursive stack growth and ordered-AST allocation
// while leaving ample room for normal request shapes. Scalar string bytes are
// deliberately not counted again as parse units.
type canonicalExecutionControlParseBudget struct {
	units int
}

func validateCanonicalExecutionControlIngress(
	raw []byte,
	kind canonicalExecutionControlIngressKind,
) *canonicalError {
	switch kind {
	case canonicalExecutionControlCreateThread,
		canonicalExecutionControlRunSubmission,
		canonicalExecutionControlRootOnly:
	default:
		return canonicalInvalidRequest(
			"Execution control ingress kind is invalid",
			"invalid_execution_control_ingress_kind",
		)
	}

	root, err := parseCanonicalExecutionControlOrderedJSON(raw)
	if errors.Is(err, errCanonicalExecutionControlParseBudget) {
		return canonicalExecutionControlBudgetExceeded()
	}
	if err != nil || root.kind != canonicalExecutionControlJSONObject {
		return nil
	}

	budget := canonicalExecutionControlIngressBudget{}
	if public := auditCanonicalExecutionControlObject(root, "", 0, true, &budget); public != nil {
		return public
	}

	switch kind {
	case canonicalExecutionControlCreateThread:
		return auditCanonicalCreateThreadExecutionControls(root, &budget)
	case canonicalExecutionControlRunSubmission:
		for _, container := range [...]string{"config", "context"} {
			value := canonicalExecutionControlObjectField(root, container)
			if value == nil || value.kind != canonicalExecutionControlJSONObject {
				continue
			}
			if public := auditCanonicalExecutionControlConfig(
				value,
				container,
				0,
				&budget,
			); public != nil {
				return public
			}
		}
	}
	return nil
}

func parseCanonicalExecutionControlOrderedJSON(
	raw []byte,
) (*canonicalExecutionControlJSONValue, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	budget := canonicalExecutionControlParseBudget{}

	value, err := decodeCanonicalExecutionControlJSONValue(decoder, &budget, 1)
	if err != nil {
		return nil, err
	}
	if _, err = decoder.Token(); !errors.Is(err, io.EOF) {
		return nil, errCanonicalExecutionControlOrderedJSON
	}
	return value, nil
}

func decodeCanonicalExecutionControlJSONValue(
	decoder *json.Decoder,
	budget *canonicalExecutionControlParseBudget,
	depth int,
) (*canonicalExecutionControlJSONValue, error) {
	if err := budget.reserveValue(depth); err != nil {
		return nil, err
	}
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	delim, isDelim := token.(json.Delim)
	if !isDelim {
		return &canonicalExecutionControlJSONValue{
			kind: canonicalExecutionControlJSONScalar,
		}, nil
	}

	switch delim {
	case '{':
		value := &canonicalExecutionControlJSONValue{
			kind: canonicalExecutionControlJSONObject,
		}
		for decoder.More() {
			keyToken, keyErr := decoder.Token()
			if keyErr != nil {
				return nil, keyErr
			}
			key, isString := keyToken.(string)
			if !isString {
				return nil, errCanonicalExecutionControlOrderedJSON
			}
			fieldValue, valueErr := decodeCanonicalExecutionControlJSONValue(
				decoder,
				budget,
				depth+1,
			)
			if valueErr != nil {
				return nil, valueErr
			}
			if reserveErr := budget.reserveField(); reserveErr != nil {
				return nil, reserveErr
			}
			value.fields = append(value.fields, canonicalExecutionControlJSONField{
				name:  key,
				value: fieldValue,
			})
		}
		closing, closingErr := decoder.Token()
		if closingErr != nil || closing != json.Delim('}') {
			return nil, errCanonicalExecutionControlOrderedJSON
		}
		return value, nil
	case '[':
		value := &canonicalExecutionControlJSONValue{
			kind: canonicalExecutionControlJSONArray,
		}
		for decoder.More() {
			_, itemErr := decodeCanonicalExecutionControlJSONValue(
				decoder,
				budget,
				depth+1,
			)
			if itemErr != nil {
				return nil, itemErr
			}
		}
		closing, closingErr := decoder.Token()
		if closingErr != nil || closing != json.Delim(']') {
			return nil, errCanonicalExecutionControlOrderedJSON
		}
		return value, nil
	default:
		return nil, errCanonicalExecutionControlOrderedJSON
	}
}

func (budget *canonicalExecutionControlParseBudget) reserveValue(depth int) error {
	if depth > canonicalExecutionControlMaxParseDepth {
		return errCanonicalExecutionControlParseBudget
	}
	return budget.reserveUnit()
}

func (budget *canonicalExecutionControlParseBudget) reserveField() error {
	return budget.reserveUnit()
}

func (budget *canonicalExecutionControlParseBudget) reserveUnit() error {
	if budget.units >= canonicalExecutionControlMaxParseUnits {
		return errCanonicalExecutionControlParseBudget
	}
	budget.units++
	return nil
}

func auditCanonicalCreateThreadExecutionControls(
	root *canonicalExecutionControlJSONValue,
	budget *canonicalExecutionControlIngressBudget,
) *canonicalError {
	coze := canonicalExecutionControlObjectField(root, "coze")
	if coze != nil && coze.kind == canonicalExecutionControlJSONObject {
		if public := auditCanonicalExecutionControlObject(coze, "coze", 0, false, budget); public != nil {
			return public
		}

		for _, runContainer := range [...]string{"initial_run", "deferred_initial_run"} {
			run := canonicalExecutionControlObjectField(coze, runContainer)
			if run == nil || run.kind != canonicalExecutionControlJSONObject {
				continue
			}
			runPath := canonicalExecutionControlPath("coze", runContainer)
			if public := auditCanonicalExecutionControlObject(run, runPath, 0, false, budget); public != nil {
				return public
			}
			for _, controlContainer := range [...]string{"config", "context"} {
				value := canonicalExecutionControlObjectField(run, controlContainer)
				if value == nil || value.kind != canonicalExecutionControlJSONObject {
					continue
				}
				if public := auditCanonicalExecutionControlConfig(
					value,
					canonicalExecutionControlPath(runPath, controlContainer),
					0,
					budget,
				); public != nil {
					return public
				}
			}
		}
	}

	for _, field := range [...]string{"initial_submission_v2", "deferred_initial_submission_v2"} {
		run := canonicalExecutionControlObjectField(root, field)
		if run == nil || run.kind != canonicalExecutionControlJSONObject {
			continue
		}
		if public := auditCanonicalExecutionControlObject(run, field, 0, false, budget); public != nil {
			return public
		}
		config := canonicalExecutionControlObjectField(run, "config")
		if config == nil || config.kind != canonicalExecutionControlJSONObject {
			continue
		}
		if public := auditCanonicalExecutionControlConfig(
			config,
			canonicalExecutionControlPath(field, "config"),
			0,
			budget,
		); public != nil {
			return public
		}
	}
	return nil
}

func auditCanonicalExecutionControlConfig(
	value *canonicalExecutionControlJSONValue,
	path string,
	reservedHops int,
	budget *canonicalExecutionControlIngressBudget,
) *canonicalError {
	if public := auditCanonicalExecutionControlObject(
		value,
		path,
		reservedHops,
		true,
		budget,
	); public != nil {
		return public
	}

	for _, container := range [...]string{"configurable", "context"} {
		nested := canonicalExecutionControlObjectField(value, container)
		if nested == nil || nested.kind != canonicalExecutionControlJSONObject {
			continue
		}
		if public := auditCanonicalExecutionControlConfig(
			nested,
			canonicalExecutionControlPath(path, container),
			reservedHops+1,
			budget,
		); public != nil {
			return public
		}
	}
	return nil
}

func auditCanonicalExecutionControlObject(
	value *canonicalExecutionControlJSONValue,
	path string,
	reservedHops int,
	controls bool,
	budget *canonicalExecutionControlIngressBudget,
) *canonicalError {
	if public := budget.inspect(path, reservedHops); public != nil {
		return public
	}
	if canonicalExecutionControlObjectHasDuplicate(value) {
		return newCanonicalError(
			hertzconsts.StatusBadRequest,
			"invalid_json",
			"Request body is not valid JSON",
			"invalid_json",
			false,
		)
	}
	if !controls {
		return nil
	}
	for _, field := range canonicalExecutionControlRetiredFields {
		if canonicalExecutionControlObjectField(value, field) == nil {
			continue
		}
		controlPath := canonicalExecutionControlPath(path, field)
		if public := budget.checkPath(controlPath); public != nil {
			return public
		}
		return newCanonicalError(
			hertzconsts.StatusUnprocessableEntity,
			"unsupported_execution_control",
			"Unsupported execution control: "+controlPath,
			"unsupported_execution_control",
			false,
		)
	}
	return nil
}

func canonicalExecutionControlObjectHasDuplicate(
	value *canonicalExecutionControlJSONValue,
) bool {
	seen := make(map[string]struct{}, len(value.fields))
	for _, field := range value.fields {
		normalized := canonicalExecutionControlASCIILower(field.name)
		if _, exists := seen[normalized]; exists {
			return true
		}
		seen[normalized] = struct{}{}
	}
	return false
}

func canonicalExecutionControlObjectField(
	value *canonicalExecutionControlJSONValue,
	name string,
) *canonicalExecutionControlJSONValue {
	for _, field := range value.fields {
		if canonicalExecutionControlASCIILower(field.name) == name {
			return field.value
		}
	}
	return nil
}

func canonicalExecutionControlASCIILower(value string) string {
	var normalized []byte
	for index := 0; index < len(value); index++ {
		character := value[index]
		if character < 'A' || character > 'Z' {
			continue
		}
		if normalized == nil {
			normalized = []byte(value)
		}
		normalized[index] = character + ('a' - 'A')
	}
	if normalized == nil {
		return value
	}
	return string(normalized)
}

func canonicalExecutionControlPath(prefix, field string) string {
	if prefix == "" {
		return field
	}
	return prefix + "." + field
}

func (budget *canonicalExecutionControlIngressBudget) inspect(
	path string,
	reservedHops int,
) *canonicalError {
	if reservedHops > canonicalExecutionControlMaxReservedHops ||
		budget.inspectedObjects >= canonicalExecutionControlMaxInspectedObjects {
		return canonicalExecutionControlBudgetExceeded()
	}
	if public := budget.checkPath(path); public != nil {
		return public
	}
	budget.inspectedObjects++
	return nil
}

func (*canonicalExecutionControlIngressBudget) checkPath(path string) *canonicalError {
	if len(path) > canonicalExecutionControlMaxPathBytes {
		return canonicalExecutionControlBudgetExceeded()
	}
	return nil
}

func canonicalExecutionControlBudgetExceeded() *canonicalError {
	return canonicalInvalidRequest(
		"Execution control validation budget exceeded",
		"execution_control_validation_budget_exceeded",
	)
}
