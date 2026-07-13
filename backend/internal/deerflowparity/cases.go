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

package deerflowparity

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
)

const lockedDeerFlowRevision = "5851f8250eb150ca23134c79b11ebc5073ac2789"

//go:embed testdata/semantic_core_cases.json
var semanticCoreCases []byte

func LoadCases() (*Suite, error) {
	suite, err := DecodeCases(strings.NewReader(string(semanticCoreCases)))
	if err != nil {
		return nil, err
	}
	if !slices.Equal(suite.CaseIDs(), SemanticCoreCaseIDs()) {
		return nil, errors.New("semantic core fixture coverage or order is invalid")
	}
	return suite, nil
}

func DecodeCases(reader io.Reader) (*Suite, error) {
	decoder := json.NewDecoder(io.LimitReader(reader, 128*1024))
	decoder.DisallowUnknownFields()

	var suite Suite
	if err := decoder.Decode(&suite); err != nil {
		return nil, fmt.Errorf("decode acceptance cases: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, errors.New("decode acceptance cases: trailing JSON value")
	}
	if err := validateSuite(&suite); err != nil {
		return nil, err
	}
	return &suite, nil
}

func validateSuite(suite *Suite) error {
	if suite == nil {
		return errors.New("acceptance suite is nil")
	}
	if suite.Schema != ContractSchemaV1 {
		return fmt.Errorf("unsupported acceptance schema %q", suite.Schema)
	}
	if suite.Scope != ScopeSemanticCore {
		return fmt.Errorf("unsupported acceptance scope %q", suite.Scope)
	}
	if suite.DeerFlowRevision != lockedDeerFlowRevision {
		return errors.New("deerflow revision does not match the locked baseline")
	}
	if len(suite.Cases) == 0 || len(suite.Cases) > 32 {
		return errors.New("acceptance case count is outside bounds")
	}

	seen := make(map[string]struct{}, len(suite.Cases))
	for index := range suite.Cases {
		testCase := &suite.Cases[index]
		if _, ok := seen[testCase.ID]; ok {
			return fmt.Errorf("duplicate acceptance case %q", testCase.ID)
		}
		seen[testCase.ID] = struct{}{}
		if err := validateCase(testCase); err != nil {
			return fmt.Errorf("validate acceptance case %q: %w", testCase.ID, err)
		}
	}
	return nil
}

func validateCase(testCase *Case) error {
	if testCase == nil || strings.TrimSpace(testCase.ID) == "" {
		return errors.New("case id is required")
	}
	if len(testCase.ID) > 80 || strings.ContainsAny(testCase.ID, "\r\n\t") {
		return errors.New("case id is invalid")
	}
	if !validMode(testCase.Mode) {
		return fmt.Errorf("unsupported mode %q", testCase.Mode)
	}
	prompt := strings.TrimSpace(testCase.InputPrompt)
	if prompt == "" || len(prompt) > 1000 {
		return errors.New("input prompt is outside bounds")
	}
	if len(testCase.Actions) == 0 || len(testCase.Actions) > 8 || testCase.Actions[0].Type != ActionRun {
		return errors.New("actions must start with one bounded run action")
	}
	for _, action := range testCase.Actions {
		if err := validateAction(action); err != nil {
			return err
		}
	}
	if len(testCase.Expect.RequiredTerminal) == 0 || len(testCase.Expect.RequiredTerminal) > 3 {
		return errors.New("required terminal states are outside bounds")
	}
	if len(testCase.Expect.RequiredEvents) == 0 || len(testCase.Expect.RequiredEvents) > 32 {
		return errors.New("required events are outside bounds")
	}
	if len(testCase.Expect.EventOrder) == 0 || len(testCase.Expect.EventOrder) > 32 {
		return errors.New("event ordering is outside bounds")
	}
	for _, pair := range testCase.Expect.EventOrder {
		if len(pair) != 2 || strings.TrimSpace(pair[0]) == "" || strings.TrimSpace(pair[1]) == "" {
			return errors.New("event ordering must contain non-empty pairs")
		}
	}
	if testCase.Expect.Children.Minimum < 0 || testCase.Expect.Children.Minimum > 16 {
		return errors.New("child minimum is outside bounds")
	}
	return nil
}

func validateAction(action Action) error {
	switch action.Type {
	case ActionRun, ActionReloadState:
		if action.Answer != "" || action.AfterEvent != "" || action.AfterEvents != 0 {
			return fmt.Errorf("action %q has unsupported parameters", action.Type)
		}
	case ActionFollowUp:
		if strings.TrimSpace(action.Answer) == "" || len(action.Answer) > 500 {
			return errors.New("follow-up answer is outside bounds")
		}
	case ActionCancel:
		if strings.TrimSpace(action.AfterEvent) == "" {
			return errors.New("cancel action requires an event boundary")
		}
	case ActionReconnect:
		if action.AfterEvents < 1 || action.AfterEvents > 200 {
			return errors.New("reconnect event count is outside bounds")
		}
	default:
		return fmt.Errorf("unsupported action %q", action.Type)
	}
	return nil
}

func validMode(mode Mode) bool {
	switch mode {
	case ModeFlash, ModeThinking, ModePro, ModeUltra:
		return true
	default:
		return false
	}
}
