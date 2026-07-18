// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package entity

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strings"
	"unicode"
)

type CodeRuntime string

const (
	CodeRuntimePython     CodeRuntime = "python"
	CodeRuntimeJavaScript CodeRuntime = "javascript"

	MaxCodeFileCount     = 64
	MaxCodePathLength    = 512
	MaxCodeFileSize      = 256 * 1024
	MaxCodeBundleSize    = 256 * 1024
	MaxCodeSchemaSize    = 64 * 1024
	MaxCodeVersionLength = 64

	DefaultCodeSchemaJSON = `{"type":"object","properties":{}}`
)

type CodeFile struct {
	Path    string
	Content []byte
	Size    int64
	SHA256  string
}

type CodeDraft struct {
	PluginID             int64
	SpaceID              int64
	Runtime              CodeRuntime
	EntryFile            string
	SourceBundleRef      string
	InputSchemaJSON      string
	OutputSchemaJSON     string
	Revision             int64
	LastDebuggedRevision int64
	Files                []*CodeFile
}

type CodeVersion struct {
	PluginID         int64
	SpaceID          int64
	Version          string
	Runtime          CodeRuntime
	EntryFile        string
	SourceBundleRef  string
	InputSchemaJSON  string
	OutputSchemaJSON string
	SourceRevision   int64
	CreatedBy        int64
	Files            []*CodeFile
}

func PrepareCodeDraft(draft *CodeDraft) (*CodeDraft, error) {
	if draft == nil {
		return nil, fmt.Errorf("code draft is required")
	}
	if draft.PluginID <= 0 {
		return nil, fmt.Errorf("plugin id is required")
	}
	if draft.SpaceID <= 0 {
		return nil, fmt.Errorf("space id is required")
	}
	if err := validateCodeRuntime(draft.Runtime); err != nil {
		return nil, err
	}
	inputSchemaJSON, err := canonicalizeCodeSchemaJSON(draft.InputSchemaJSON)
	if err != nil {
		return nil, fmt.Errorf("invalid input schema: %w", err)
	}
	outputSchemaJSON, err := canonicalizeCodeSchemaJSON(draft.OutputSchemaJSON)
	if err != nil {
		return nil, fmt.Errorf("invalid output schema: %w", err)
	}
	if len(draft.Files) == 0 {
		return nil, fmt.Errorf("at least one code file is required")
	}
	if len(draft.Files) > MaxCodeFileCount {
		return nil, fmt.Errorf("code file count exceeds %d", MaxCodeFileCount)
	}

	entryFile, err := normalizeCodePath(draft.EntryFile)
	if err != nil {
		return nil, fmt.Errorf("invalid entry file: %w", err)
	}

	files := make([]*CodeFile, 0, len(draft.Files))
	seen := make(map[string]struct{}, len(draft.Files))
	totalSize := 0
	entryExists := false
	for _, file := range draft.Files {
		if file == nil {
			return nil, fmt.Errorf("code file is required")
		}
		filePath, err := normalizeCodePath(file.Path)
		if err != nil {
			return nil, fmt.Errorf("invalid code file path: %w", err)
		}
		if _, exists := seen[filePath]; exists {
			return nil, fmt.Errorf("duplicate code file path %q", filePath)
		}
		seen[filePath] = struct{}{}
		if len(file.Content) > MaxCodeFileSize {
			return nil, fmt.Errorf("code file %q exceeds %d bytes", filePath, MaxCodeFileSize)
		}
		totalSize += len(file.Content)
		if totalSize > MaxCodeBundleSize {
			return nil, fmt.Errorf("code bundle exceeds %d bytes", MaxCodeBundleSize)
		}
		digest := sha256.Sum256(file.Content)
		files = append(files, &CodeFile{
			Path:    filePath,
			Content: append([]byte(nil), file.Content...),
			Size:    int64(len(file.Content)),
			SHA256:  hex.EncodeToString(digest[:]),
		})
		entryExists = entryExists || filePath == entryFile
	}
	if !entryExists {
		return nil, fmt.Errorf("entry file %q does not exist", entryFile)
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})

	prepared := &CodeDraft{
		PluginID:             draft.PluginID,
		SpaceID:              draft.SpaceID,
		Runtime:              draft.Runtime,
		EntryFile:            entryFile,
		InputSchemaJSON:      inputSchemaJSON,
		OutputSchemaJSON:     outputSchemaJSON,
		Revision:             draft.Revision,
		LastDebuggedRevision: draft.LastDebuggedRevision,
		Files:                files,
	}
	prepared.SourceBundleRef = codeBundleDigest(prepared)
	return prepared, nil
}

func CloneCodeFiles(files []*CodeFile) []*CodeFile {
	cloned := make([]*CodeFile, 0, len(files))
	for _, file := range files {
		if file == nil {
			continue
		}
		cloned = append(cloned, &CodeFile{
			Path:    file.Path,
			Content: append([]byte(nil), file.Content...),
			Size:    file.Size,
			SHA256:  file.SHA256,
		})
	}
	return cloned
}

func validateCodeRuntime(runtime CodeRuntime) error {
	switch runtime {
	case CodeRuntimePython, CodeRuntimeJavaScript:
		return nil
	default:
		return fmt.Errorf("unsupported code runtime %q", runtime)
	}
}

func normalizeCodePath(value string) (string, error) {
	if value == "" {
		return "", fmt.Errorf("path is required")
	}
	if strings.TrimSpace(value) != value {
		return "", fmt.Errorf("path must not contain surrounding whitespace")
	}
	if len(value) > MaxCodePathLength {
		return "", fmt.Errorf("path exceeds %d characters", MaxCodePathLength)
	}
	if strings.Contains(value, "\\") {
		return "", fmt.Errorf("backslashes are not allowed")
	}
	if path.IsAbs(value) {
		return "", fmt.Errorf("absolute paths are not allowed")
	}
	if len(value) >= 2 &&
		((value[0] >= 'a' && value[0] <= 'z') || (value[0] >= 'A' && value[0] <= 'Z')) &&
		value[1] == ':' {
		return "", fmt.Errorf("windows drive paths are not allowed")
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return "", fmt.Errorf("control characters are not allowed")
		}
	}
	for _, segment := range strings.Split(value, "/") {
		if segment == "" {
			return "", fmt.Errorf("empty path segments are not allowed")
		}
		if segment == ".." {
			return "", fmt.Errorf("parent traversal is not allowed")
		}
		if segment == "." {
			return "", fmt.Errorf("relative path segments are not allowed")
		}
		if strings.TrimSpace(segment) != segment {
			return "", fmt.Errorf("path segments must not contain surrounding whitespace")
		}
	}
	cleaned := path.Clean(value)
	if cleaned != value {
		return "", fmt.Errorf("path must be normalized")
	}
	return cleaned, nil
}

func canonicalizeCodeSchemaJSON(value string) (string, error) {
	if value == "" {
		return DefaultCodeSchemaJSON, nil
	}
	if len(value) > MaxCodeSchemaSize {
		return "", fmt.Errorf("schema exceeds %d bytes", MaxCodeSchemaSize)
	}
	var decoded any
	if err := json.Unmarshal([]byte(value), &decoded); err != nil {
		return "", fmt.Errorf("schema must be valid JSON: %w", err)
	}
	object, ok := decoded.(map[string]any)
	if !ok || object == nil {
		return "", fmt.Errorf("schema must be a JSON object")
	}
	canonical, err := json.Marshal(object)
	if err != nil {
		return "", fmt.Errorf("canonicalize schema: %w", err)
	}
	if len(canonical) > MaxCodeSchemaSize {
		return "", fmt.Errorf("canonical schema exceeds %d bytes", MaxCodeSchemaSize)
	}
	if string(canonical) == `{"properties":{},"type":"object"}` {
		return DefaultCodeSchemaJSON, nil
	}
	return string(canonical), nil
}

func codeBundleDigest(draft *CodeDraft) string {
	hash := sha256.New()
	writeField := func(value []byte) {
		var length [8]byte
		binary.BigEndian.PutUint64(length[:], uint64(len(value)))
		_, _ = hash.Write(length[:])
		_, _ = hash.Write(value)
	}
	writeField([]byte(draft.Runtime))
	writeField([]byte(draft.EntryFile))
	writeField([]byte(draft.InputSchemaJSON))
	writeField([]byte(draft.OutputSchemaJSON))
	for _, file := range draft.Files {
		writeField([]byte(file.Path))
		writeField(file.Content)
	}
	return hex.EncodeToString(hash.Sum(nil))
}
