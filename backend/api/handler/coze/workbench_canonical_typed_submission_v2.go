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
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	hertzconsts "github.com/cloudwego/hertz/pkg/protocol/consts"

	threadcontract "github.com/coze-dev/coze-studio/backend/api/model/workbench/thread_contract"
)

const (
	canonicalV2ObjectKind byte = '{'
	canonicalV2ArrayKind  byte = '['
	canonicalV2StringKind byte = 's'
	canonicalV2NumberKind byte = 'n'
	canonicalV2BoolKind   byte = 'b'
	canonicalV2NullKind   byte = '0'

	canonicalV2MaxDepth     = 128
	canonicalV2MaxParseUnit = 65536
	canonicalV2MaxSafeKey   = 64
)

type canonicalV2Node struct {
	path string
	raw  json.RawMessage
	kind byte
	obj  map[string]*canonicalV2Node
	arr  []*canonicalV2Node

	order []string
	value any
	start int64
	end   int64
}

type canonicalV2FieldRule struct {
	required bool
	kind     byte
	object   map[string]canonicalV2FieldRule
	element  *canonicalV2FieldRule
}

type canonicalV2ParseBudget struct {
	root  string
	units int
}

// Presence is deliberately separate from the generated value. Generated Go
// values cannot distinguish an omitted optional list from every zero value.
type canonicalTypedV2Presence map[string]struct{}

type canonicalTypedRunV2 struct {
	Value    threadcontract.CanonicalRunSubmissionV2
	Presence canonicalTypedV2Presence
}

type canonicalTypedInitialV2 struct {
	Value    threadcontract.CanonicalInitialRunSubmissionV2
	Presence canonicalTypedV2Presence
}

type canonicalTypedHumanV2 struct {
	Value    threadcontract.CanonicalHumanInteractionResponseV2
	Presence canonicalTypedV2Presence
}

func decodeCanonicalTypedRunSubmissionV2(raw []byte) (*canonicalTypedRunV2, *canonicalError) {
	node, public := decodeCanonicalV2Object(raw, "submission_v2", canonicalTypedRunV2Rules())
	if public != nil {
		return nil, public
	}
	return extractCanonicalTypedRunV2(node)
}

func decodeCanonicalTypedInitialSubmissionV2(raw []byte, field string) (*canonicalTypedInitialV2, *canonicalError) {
	node, public := decodeCanonicalV2Object(raw, field, canonicalTypedInitialV2Rules())
	if public != nil {
		return nil, public
	}
	return extractCanonicalTypedInitialV2(node)
}

func decodeCanonicalTypedHumanResponseV2(raw []byte) (*canonicalTypedHumanV2, *canonicalError) {
	node, public := decodeCanonicalV2Object(raw, "response_v2", canonicalTypedHumanV2Rules())
	if public != nil {
		return nil, public
	}
	return extractCanonicalTypedHumanV2(node)
}

func decodeCanonicalV2Object(
	raw []byte,
	root string,
	rules map[string]canonicalV2FieldRule,
) (*canonicalV2Node, *canonicalError) {
	body := bytes.TrimSpace(raw)
	node, public := decodeCanonicalV2Node(body, root)
	if public != nil {
		return nil, public
	}
	if node.kind != canonicalV2ObjectKind {
		return nil, canonicalTypedV2Invalid(root)
	}
	// encoding/json replaces invalid UTF-8 in decoded strings. Inspect the raw
	// slices before closed-shape validation so a hostile key cannot be
	// misclassified as an unsupported field or echoed through its decoded form.
	if path := canonicalV2InvalidUTF8Path(body, node); path != "" {
		return nil, canonicalTypedV2Invalid(path)
	}
	if public = validateCanonicalV2Object(node, rules); public != nil {
		return nil, public
	}
	canonicalV2AttachRaw(body, node)
	return node, nil
}

func decodeCanonicalV2Node(raw []byte, root string) (*canonicalV2Node, *canonicalError) {
	decoder := json.NewDecoder(bytes.NewReader(bytes.TrimSpace(raw)))
	decoder.UseNumber()
	budget := &canonicalV2ParseBudget{root: root}
	node, public := readCanonicalV2NodeWithBudget(decoder, root, 1, budget)
	if public != nil {
		return nil, public
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return nil, canonicalTypedV2JSONError(true)
	}
	return node, nil
}

func readCanonicalV2Node(decoder *json.Decoder, path string) (*canonicalV2Node, *canonicalError) {
	return readCanonicalV2NodeWithBudget(
		decoder,
		path,
		1,
		&canonicalV2ParseBudget{root: path},
	)
}

func readCanonicalV2NodeWithBudget(
	decoder *json.Decoder,
	path string,
	depth int,
	budget *canonicalV2ParseBudget,
) (*canonicalV2Node, *canonicalError) {
	if public := budget.consume(depth, 1); public != nil {
		return nil, public
	}
	start := decoder.InputOffset()
	token, err := decoder.Token()
	if err != nil {
		return nil, canonicalTypedV2JSONError(false)
	}

	node := &canonicalV2Node{path: path, start: start, value: token}
	switch value := token.(type) {
	case json.Delim:
		switch value {
		case '{':
			node.kind = canonicalV2ObjectKind
			node.obj = make(map[string]*canonicalV2Node)
			for decoder.More() {
				if public := budget.consume(depth, 1); public != nil {
					return nil, public
				}
				keyToken, keyErr := decoder.Token()
				if keyErr != nil {
					return nil, canonicalTypedV2JSONError(false)
				}
				key, ok := keyToken.(string)
				if !ok {
					return nil, canonicalTypedV2JSONError(false)
				}
				childPath := canonicalV2ChildPath(path, key)
				if _, duplicate := node.obj[key]; duplicate {
					return nil, canonicalTypedV2Invalid(childPath)
				}
				child, childErr := readCanonicalV2NodeWithBudget(decoder, childPath, depth+1, budget)
				if childErr != nil {
					return nil, childErr
				}
				node.obj[key] = child
				node.order = append(node.order, key)
			}
			closing, closeErr := decoder.Token()
			if closeErr != nil || closing != json.Delim('}') {
				return nil, canonicalTypedV2JSONError(false)
			}
		case '[':
			node.kind = canonicalV2ArrayKind
			for index := 0; decoder.More(); index++ {
				child, childErr := readCanonicalV2NodeWithBudget(
					decoder,
					canonicalV2IndexPath(path, index),
					depth+1,
					budget,
				)
				if childErr != nil {
					return nil, childErr
				}
				node.arr = append(node.arr, child)
			}
			closing, closeErr := decoder.Token()
			if closeErr != nil || closing != json.Delim(']') {
				return nil, canonicalTypedV2JSONError(false)
			}
		default:
			return nil, canonicalTypedV2JSONError(false)
		}
	case string:
		node.kind = canonicalV2StringKind
	case json.Number:
		node.kind = canonicalV2NumberKind
	case bool:
		node.kind = canonicalV2BoolKind
	case nil:
		node.kind = canonicalV2NullKind
	default:
		return nil, canonicalTypedV2JSONError(false)
	}
	node.end = decoder.InputOffset()
	return node, nil
}

func (budget *canonicalV2ParseBudget) consume(depth, units int) *canonicalError {
	if budget == nil || depth > canonicalV2MaxDepth || units > canonicalV2MaxParseUnit-budget.units {
		root := "typed_submission_v2"
		if budget != nil && budget.root != "" {
			root = budget.root
		}
		return canonicalTypedV2Invalid(root)
	}
	budget.units += units
	return nil
}

func validateCanonicalV2Object(node *canonicalV2Node, rules map[string]canonicalV2FieldRule) *canonicalError {
	if node == nil {
		return canonicalTypedV2Invalid("typed_submission_v2")
	}
	if node.kind != canonicalV2ObjectKind {
		return canonicalTypedV2Invalid(node.path)
	}
	for _, name := range node.order {
		if _, ok := rules[name]; !ok {
			return canonicalUnsupportedField(canonicalV2ChildPath(node.path, name))
		}
	}

	names := canonicalV2RuleNamesInSchemaOrder(node.path, rules)
	for _, name := range names {
		rule := rules[name]
		child, present := node.obj[name]
		path := canonicalV2ChildPath(node.path, name)
		if !present {
			if rule.required {
				return canonicalTypedV2Invalid(path)
			}
			continue
		}
		if child.kind == canonicalV2NullKind || child.kind != rule.kind {
			return canonicalTypedV2Invalid(path)
		}
		if public := validateCanonicalV2Rule(child, rule); public != nil {
			return public
		}
	}
	return nil
}

func validateCanonicalV2Rule(node *canonicalV2Node, rule canonicalV2FieldRule) *canonicalError {
	switch rule.kind {
	case canonicalV2ObjectKind:
		return validateCanonicalV2Object(node, rule.object)
	case canonicalV2ArrayKind:
		for _, child := range node.arr {
			if child.kind == canonicalV2NullKind || rule.element == nil || child.kind != rule.element.kind {
				return canonicalTypedV2Invalid(child.path)
			}
			if public := validateCanonicalV2Rule(child, *rule.element); public != nil {
				return public
			}
		}
	}
	return nil
}

func extractCanonicalTypedRunV2(node *canonicalV2Node) (*canonicalTypedRunV2, *canonicalError) {
	var value threadcontract.CanonicalRunSubmissionV2
	if public := canonicalV2UnmarshalGenerated(node, &value); public != nil {
		return nil, public
	}
	return &canonicalTypedRunV2{Value: value, Presence: canonicalV2Presence(node)}, nil
}

func extractCanonicalTypedInitialV2(node *canonicalV2Node) (*canonicalTypedInitialV2, *canonicalError) {
	var value threadcontract.CanonicalInitialRunSubmissionV2
	if public := canonicalV2UnmarshalGenerated(node, &value); public != nil {
		return nil, public
	}
	return &canonicalTypedInitialV2{Value: value, Presence: canonicalV2Presence(node)}, nil
}

func extractCanonicalTypedHumanV2(node *canonicalV2Node) (*canonicalTypedHumanV2, *canonicalError) {
	var value threadcontract.CanonicalHumanInteractionResponseV2
	if public := canonicalV2UnmarshalGenerated(node, &value); public != nil {
		return nil, public
	}
	return &canonicalTypedHumanV2{Value: value, Presence: canonicalV2Presence(node)}, nil
}

func canonicalV2UnmarshalGenerated(node *canonicalV2Node, value any) *canonicalError {
	normalized, public := canonicalV2GeneratedJSONValue(node)
	if public != nil {
		return public
	}
	raw, err := json.Marshal(normalized)
	if err != nil {
		return canonicalTypedV2Invalid(node.path)
	}
	if err = json.Unmarshal(raw, value); err != nil {
		return canonicalTypedV2Invalid(node.path)
	}
	return nil
}

func canonicalV2GeneratedJSONValue(node *canonicalV2Node) (any, *canonicalError) {
	switch node.kind {
	case canonicalV2ObjectKind:
		value := make(map[string]any, len(node.obj))
		for _, name := range node.order {
			childValue, public := canonicalV2GeneratedJSONValue(node.obj[name])
			if public != nil {
				return nil, public
			}
			value[name] = childValue
		}
		return value, nil
	case canonicalV2ArrayKind:
		value := make([]any, 0, len(node.arr))
		for _, child := range node.arr {
			childValue, public := canonicalV2GeneratedJSONValue(child)
			if public != nil {
				return nil, public
			}
			value = append(value, childValue)
		}
		return value, nil
	case canonicalV2StringKind:
		text := node.value.(string)
		if strings.HasSuffix(node.path, ".model_type") ||
			strings.HasSuffix(node.path, ".file_id") ||
			strings.HasSuffix(node.path, ".source_run_id") {
			if _, err := strconv.ParseInt(text, 10, 64); err != nil {
				return nil, canonicalTypedV2Invalid(node.path)
			}
		}
		// The generated Hertz tag for list<i64> does not decode JSON strings
		// element-by-element. Convert only this IDL-declared string-converted list
		// to exact JSON integer digits before populating the generated Go value.
		if strings.Contains(node.path, ".candidate_model_ids[") {
			parsed, err := strconv.ParseInt(text, 10, 64)
			if err != nil {
				return nil, canonicalTypedV2Invalid(node.path)
			}
			return json.Number(strconv.FormatInt(parsed, 10)), nil
		}
		return text, nil
	case canonicalV2NumberKind, canonicalV2BoolKind:
		return node.value, nil
	default:
		return nil, canonicalTypedV2Invalid(node.path)
	}
}

func canonicalV2Presence(node *canonicalV2Node) canonicalTypedV2Presence {
	presence := make(canonicalTypedV2Presence)
	var visit func(*canonicalV2Node)
	visit = func(current *canonicalV2Node) {
		if current == nil {
			return
		}
		if current != node {
			presence[current.path] = struct{}{}
		}
		for _, name := range current.order {
			visit(current.obj[name])
		}
		for _, child := range current.arr {
			visit(child)
		}
	}
	visit(node)
	return presence
}

func canonicalV2InvalidUTF8Path(raw []byte, node *canonicalV2Node) string {
	for _, name := range node.order {
		if path := canonicalV2InvalidUTF8Path(raw, node.obj[name]); path != "" {
			return path
		}
	}
	for _, child := range node.arr {
		if path := canonicalV2InvalidUTF8Path(raw, child); path != "" {
			return path
		}
	}
	if node.start >= 0 && node.end >= node.start && node.end <= int64(len(raw)) &&
		!utf8.Valid(raw[node.start:node.end]) {
		return node.path
	}
	return ""
}

func canonicalV2AttachRaw(raw []byte, node *canonicalV2Node) {
	if node.start >= 0 && node.end >= node.start && node.end <= int64(len(raw)) {
		node.raw = append(node.raw[:0], raw[node.start:node.end]...)
	}
	for _, name := range node.order {
		canonicalV2AttachRaw(raw, node.obj[name])
	}
	for _, child := range node.arr {
		canonicalV2AttachRaw(raw, child)
	}
}

func canonicalV2RuleNamesInSchemaOrder(
	path string,
	rules map[string]canonicalV2FieldRule,
) []string {
	var preferred []string
	switch {
	case path == "submission_v2":
		preferred = []string{"schema_version", "kind", "input", "composer", "config", "lineage", "metadata"}
	case path == "initial_submission_v2" || path == "deferred_initial_submission_v2":
		preferred = []string{"schema_version", "input", "composer", "config", "metadata"}
	case path == "response_v2":
		preferred = []string{"schema", "interaction_id", "kind", "decision", "answer", "choice_id", "comment"}
	case strings.HasSuffix(path, ".input"):
		preferred = []string{"message", "uploaded_files"}
	case strings.Contains(path, ".uploaded_files["):
		preferred = []string{"file_id"}
	case strings.HasSuffix(path, ".composer"):
		preferred = []string{"model_type", "model_name", "explicit_enable_skills", "allowed_skills", "enable_mcp", "enable_kbs", "enable_databases", "allowed_mcp_tools"}
	case strings.HasSuffix(path, ".config"):
		preferred = []string{"runtime", "memory_retrieval", "skills", "mcp_tools", "web_tools", "model_retry", "model_failover", "token_usage"}
	case strings.HasSuffix(path, ".memory_retrieval"):
		preferred = []string{"limit", "candidate_limit", "scopes", "min_confidence"}
	case strings.HasSuffix(path, ".skills") || strings.HasSuffix(path, ".mcp_tools"):
		preferred = []string{"enabled", "visibility"}
	case strings.HasSuffix(path, ".web_tools"):
		preferred = []string{"enabled", "visibility", "http", "search"}
	case strings.HasSuffix(path, ".http"):
		preferred = []string{"enabled", "allowed_hosts", "timeout_ms", "max_response_bytes"}
	case strings.HasSuffix(path, ".search"):
		preferred = []string{"enabled", "max_results"}
	case strings.HasSuffix(path, ".model_retry"):
		preferred = []string{"max_retries", "backoff_ms", "retry_empty_output", "retry_finish_reasons"}
	case strings.HasSuffix(path, ".model_failover"):
		preferred = []string{"candidate_model_ids", "max_retries", "failover_empty_output", "failover_finish_reasons"}
	case strings.HasSuffix(path, ".token_usage"):
		preferred = []string{"enabled"}
	case strings.HasSuffix(path, ".lineage"):
		preferred = []string{"source_run_id"}
	case strings.HasSuffix(path, ".metadata"):
		preferred = []string{"source"}
	}

	ordered := make([]string, 0, len(rules))
	seen := make(map[string]struct{}, len(rules))
	for _, name := range preferred {
		if _, ok := rules[name]; ok {
			ordered = append(ordered, name)
			seen[name] = struct{}{}
		}
	}
	remainder := make([]string, 0, len(rules)-len(ordered))
	for name := range rules {
		if _, ok := seen[name]; !ok {
			remainder = append(remainder, name)
		}
	}
	sort.Strings(remainder)
	return append(ordered, remainder...)
}

func validateCanonicalTypedRunVersionMixing(raw []byte) *canonicalError {
	root, ok := canonicalV2RootRawMessages(raw)
	if !ok || root["submission_v2"] == nil {
		return nil
	}
	for _, field := range []string{"input", "command", "metadata", "config", "context", "coze"} {
		if root[field] != nil {
			return canonicalTypedV2Mixed("submission_v2")
		}
	}
	return nil
}

func validateCanonicalTypedThreadVersionMixing(raw []byte) *canonicalError {
	root, ok := canonicalV2RootRawMessages(raw)
	if !ok {
		return nil
	}
	initial := root["initial_submission_v2"] != nil
	deferred := root["deferred_initial_submission_v2"] != nil
	if initial && deferred {
		return canonicalTypedV2Mixed("initial_submission_v2")
	}
	coze, cozeOK := canonicalV2RawObject(root["coze"])
	legacy := cozeOK && (coze["initial_run"] != nil || coze["deferred_initial_run"] != nil)
	if initial && legacy {
		return canonicalTypedV2Mixed("initial_submission_v2")
	}
	if deferred && legacy {
		return canonicalTypedV2Mixed("deferred_initial_submission_v2")
	}
	return nil
}

func canonicalV2RootRawMessages(raw []byte) (map[string]json.RawMessage, bool) {
	return canonicalV2RawObject(bytes.TrimSpace(raw))
}

func canonicalV2RawObject(raw []byte) (map[string]json.RawMessage, bool) {
	if len(raw) == 0 || raw[0] != '{' {
		return nil, false
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	var value map[string]json.RawMessage
	if err := decoder.Decode(&value); err != nil {
		return nil, false
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return nil, false
	}
	return value, true
}

func canonicalTypedV2Rules() map[string]canonicalV2FieldRule {
	return map[string]canonicalV2FieldRule{
		"runtime": canonicalV2Required(canonicalV2StringKind),
		"memory_retrieval": canonicalV2RequiredObject(map[string]canonicalV2FieldRule{
			"limit":           canonicalV2Required(canonicalV2NumberKind),
			"candidate_limit": canonicalV2Required(canonicalV2NumberKind),
			"scopes":          canonicalV2RequiredStringArray(),
			"min_confidence":  canonicalV2Required(canonicalV2NumberKind),
		}),
		"skills":    canonicalV2RequiredObject(canonicalV2VisibilityRules()),
		"mcp_tools": canonicalV2RequiredObject(canonicalV2VisibilityRules()),
		"web_tools": canonicalV2RequiredObject(map[string]canonicalV2FieldRule{
			"enabled":    canonicalV2Required(canonicalV2BoolKind),
			"visibility": canonicalV2Required(canonicalV2StringKind),
			"http": canonicalV2RequiredObject(map[string]canonicalV2FieldRule{
				"enabled":            canonicalV2Required(canonicalV2BoolKind),
				"allowed_hosts":      canonicalV2RequiredStringArray(),
				"timeout_ms":         canonicalV2Required(canonicalV2NumberKind),
				"max_response_bytes": canonicalV2Required(canonicalV2NumberKind),
			}),
			"search": canonicalV2RequiredObject(map[string]canonicalV2FieldRule{
				"enabled":     canonicalV2Required(canonicalV2BoolKind),
				"max_results": canonicalV2Required(canonicalV2NumberKind),
			}),
		}),
		"model_retry": canonicalV2OptionalObject(map[string]canonicalV2FieldRule{
			"max_retries":          canonicalV2Required(canonicalV2NumberKind),
			"backoff_ms":           canonicalV2Required(canonicalV2NumberKind),
			"retry_empty_output":   canonicalV2Required(canonicalV2BoolKind),
			"retry_finish_reasons": canonicalV2RequiredStringArray(),
		}),
		"model_failover": canonicalV2OptionalObject(map[string]canonicalV2FieldRule{
			"candidate_model_ids":     canonicalV2RequiredStringArray(),
			"max_retries":             canonicalV2Required(canonicalV2NumberKind),
			"failover_empty_output":   canonicalV2Required(canonicalV2BoolKind),
			"failover_finish_reasons": canonicalV2RequiredStringArray(),
		}),
		"token_usage": canonicalV2RequiredObject(map[string]canonicalV2FieldRule{
			"enabled": canonicalV2Required(canonicalV2BoolKind),
		}),
	}
}

func canonicalTypedRunV2Rules() map[string]canonicalV2FieldRule {
	return map[string]canonicalV2FieldRule{
		"schema_version": canonicalV2Required(canonicalV2StringKind),
		"kind":           canonicalV2Required(canonicalV2StringKind),
		"input":          canonicalV2RequiredObject(canonicalTypedInputV2Rules()),
		"composer":       canonicalV2RequiredObject(canonicalTypedComposerV2Rules()),
		"config":         canonicalV2RequiredObject(canonicalTypedV2Rules()),
		"lineage": canonicalV2OptionalObject(map[string]canonicalV2FieldRule{
			"source_run_id": canonicalV2Required(canonicalV2StringKind),
		}),
		"metadata": canonicalV2OptionalObject(canonicalTypedMetadataV2Rules()),
	}
}

func canonicalTypedInitialV2Rules() map[string]canonicalV2FieldRule {
	return map[string]canonicalV2FieldRule{
		"schema_version": canonicalV2Required(canonicalV2StringKind),
		"input":          canonicalV2RequiredObject(canonicalTypedInputV2Rules()),
		"composer":       canonicalV2RequiredObject(canonicalTypedComposerV2Rules()),
		"config":         canonicalV2RequiredObject(canonicalTypedV2Rules()),
		"metadata":       canonicalV2OptionalObject(canonicalTypedMetadataV2Rules()),
	}
}

func canonicalTypedHumanV2Rules() map[string]canonicalV2FieldRule {
	return map[string]canonicalV2FieldRule{
		"schema":         canonicalV2Required(canonicalV2StringKind),
		"interaction_id": canonicalV2Required(canonicalV2StringKind),
		"kind":           canonicalV2Required(canonicalV2StringKind),
		"decision":       canonicalV2Required(canonicalV2StringKind),
		"answer":         canonicalV2Optional(canonicalV2StringKind),
		"choice_id":      canonicalV2Optional(canonicalV2StringKind),
		"comment":        canonicalV2Optional(canonicalV2StringKind),
	}
}

func canonicalTypedInputV2Rules() map[string]canonicalV2FieldRule {
	file := canonicalV2FieldRule{
		kind: canonicalV2ObjectKind,
		object: map[string]canonicalV2FieldRule{
			"file_id": canonicalV2Required(canonicalV2StringKind),
		},
	}
	return map[string]canonicalV2FieldRule{
		"message":        canonicalV2Required(canonicalV2StringKind),
		"uploaded_files": {required: true, kind: canonicalV2ArrayKind, element: &file},
	}
}

func canonicalTypedComposerV2Rules() map[string]canonicalV2FieldRule {
	return map[string]canonicalV2FieldRule{
		"model_type":             canonicalV2Optional(canonicalV2StringKind),
		"model_name":             canonicalV2Optional(canonicalV2StringKind),
		"explicit_enable_skills": canonicalV2OptionalStringArray(),
		"allowed_skills":         canonicalV2RequiredStringArray(),
		"enable_mcp":             canonicalV2RequiredStringArray(),
		"enable_kbs":             canonicalV2RequiredStringArray(),
		"enable_databases":       canonicalV2RequiredStringArray(),
		"allowed_mcp_tools":      canonicalV2RequiredStringArray(),
	}
}

func canonicalTypedMetadataV2Rules() map[string]canonicalV2FieldRule {
	return map[string]canonicalV2FieldRule{
		"source": canonicalV2Required(canonicalV2StringKind),
	}
}

func canonicalV2VisibilityRules() map[string]canonicalV2FieldRule {
	return map[string]canonicalV2FieldRule{
		"enabled":    canonicalV2Required(canonicalV2BoolKind),
		"visibility": canonicalV2Required(canonicalV2StringKind),
	}
}

func canonicalV2Required(kind byte) canonicalV2FieldRule {
	return canonicalV2FieldRule{required: true, kind: kind}
}

func canonicalV2Optional(kind byte) canonicalV2FieldRule {
	return canonicalV2FieldRule{kind: kind}
}

func canonicalV2RequiredObject(fields map[string]canonicalV2FieldRule) canonicalV2FieldRule {
	return canonicalV2FieldRule{required: true, kind: canonicalV2ObjectKind, object: fields}
}

func canonicalV2OptionalObject(fields map[string]canonicalV2FieldRule) canonicalV2FieldRule {
	return canonicalV2FieldRule{kind: canonicalV2ObjectKind, object: fields}
}

func canonicalV2RequiredStringArray() canonicalV2FieldRule {
	element := canonicalV2Required(canonicalV2StringKind)
	return canonicalV2FieldRule{required: true, kind: canonicalV2ArrayKind, element: &element}
}

func canonicalV2OptionalStringArray() canonicalV2FieldRule {
	element := canonicalV2Required(canonicalV2StringKind)
	return canonicalV2FieldRule{kind: canonicalV2ArrayKind, element: &element}
}

func canonicalV2ChildPath(parent, child string) string {
	child = canonicalV2SafeKey(child)
	if parent == "" {
		return child
	}
	return parent + "." + child
}

func canonicalV2SafeKey(key string) string {
	if len(key) == 0 || len(key) > canonicalV2MaxSafeKey {
		return "<unsupported>"
	}
	for index := 0; index < len(key); index++ {
		char := key[index]
		if index == 0 {
			if (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') && char != '_' {
				return "<unsupported>"
			}
			continue
		}
		if (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') &&
			(char < '0' || char > '9') && char != '_' {
			return "<unsupported>"
		}
	}
	return key
}

func canonicalV2IndexPath(parent string, index int) string {
	return parent + "[" + strconv.Itoa(index) + "]"
}

func canonicalTypedV2JSONError(trailing bool) *canonicalError {
	if trailing {
		return newCanonicalError(
			hertzconsts.StatusBadRequest,
			"invalid_json",
			"Request body must contain one JSON object",
			"trailing_json",
			false,
		)
	}
	return newCanonicalError(
		hertzconsts.StatusBadRequest,
		"invalid_json",
		"Request body is not valid JSON",
		"invalid_json",
		false,
	)
}

func canonicalTypedV2Invalid(path string) *canonicalError {
	return canonicalInvalidRequest("Invalid typed submission field: "+path, "invalid_typed_submission")
}

func canonicalTypedV2Mixed(path string) *canonicalError {
	return canonicalInvalidRequest("Mixed submission versions: "+path, "mixed_submission_versions")
}
