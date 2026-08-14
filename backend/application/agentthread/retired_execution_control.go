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

package agentthread

import (
	"errors"
	"fmt"
	"sort"
)

var ErrUnsupportedExecutionControl = errors.New("unsupported execution control")

var submittedExecutionControlFields = [...]string{
	"requested_policy",
	"mode",
	"thinking_enabled",
	"reasoning_effort",
	"is_plan_mode",
	"subagent_enabled",
	"max_concurrent_subagents",
}

var submittedExecutionControlContainers = [...]string{
	"configurable",
	"context",
}

const submittedExecutionControlMaxReservedHops = 4

type unsupportedExecutionControlError struct {
	path string
}

func (e *unsupportedExecutionControlError) Error() string {
	return fmt.Sprintf("%s: %s", ErrUnsupportedExecutionControl, e.path)
}

func (e *unsupportedExecutionControlError) Unwrap() error {
	return ErrUnsupportedExecutionControl
}

func UnsupportedExecutionControlPath(err error) (string, bool) {
	var controlErr *unsupportedExecutionControlError
	if !errors.As(err, &controlErr) || controlErr.path == "" {
		return "", false
	}
	return controlErr.path, true
}

func validateSubmittedExecutionControls(config, runContext string) error {
	for _, input := range []struct {
		path string
		raw  string
	}{
		{path: "config", raw: config},
		{path: "context", raw: runContext},
	} {
		payload, err := parseDeerFlowRuntimePayload(input.raw)
		if err != nil {
			return err
		}
		if path, found := findSubmittedExecutionControl(payload, input.path, 0); found {
			return &unsupportedExecutionControlError{path: path}
		}
	}
	return nil
}

func findSubmittedExecutionControl(
	payload map[string]any,
	path string,
	reservedHops int,
) (string, bool) {
	for _, field := range submittedExecutionControlFields {
		if submittedExecutionControlHasKey(payload, field) {
			return path + "." + field, true
		}
	}
	if reservedHops >= submittedExecutionControlMaxReservedHops {
		return "", false
	}

	for _, container := range submittedExecutionControlContainers {
		for _, nested := range submittedExecutionControlObjects(payload, container) {
			if controlPath, found := findSubmittedExecutionControl(
				nested,
				path+"."+container,
				reservedHops+1,
			); found {
				return controlPath, true
			}
		}
	}
	return "", false
}

func submittedExecutionControlHasKey(payload map[string]any, expected string) bool {
	for key := range payload {
		if submittedExecutionControlASCIIEqualFold(key, expected) {
			return true
		}
	}
	return false
}

func submittedExecutionControlObjects(payload map[string]any, expected string) []map[string]any {
	keys := make([]string, 0, 1)
	for key, value := range payload {
		if !submittedExecutionControlASCIIEqualFold(key, expected) {
			continue
		}
		if _, ok := value.(map[string]any); ok {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)

	objects := make([]map[string]any, 0, len(keys))
	for _, key := range keys {
		objects = append(objects, payload[key].(map[string]any))
	}
	return objects
}

func submittedExecutionControlASCIIEqualFold(actual, expected string) bool {
	if len(actual) != len(expected) {
		return false
	}
	for index := 0; index < len(expected); index++ {
		value := actual[index]
		if value >= 'A' && value <= 'Z' {
			value += 'a' - 'A'
		}
		if value != expected[index] {
			return false
		}
	}
	return true
}
