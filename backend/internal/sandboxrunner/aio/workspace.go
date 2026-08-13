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

package aio

import (
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
)

const (
	logicalUserDataRoot  = "/mnt/user-data"
	logicalWorkspaceRoot = logicalUserDataRoot + "/workspace"
	logicalUploadsRoot   = logicalUserDataRoot + "/uploads"
	logicalOutputsRoot   = logicalUserDataRoot + "/outputs"
	logicalSkillsRoot    = "/mnt/skills"

	maxWorkspaceLogicalPathBytes = infrasandbox.MaxLogicalPathBytes
)

type WorkspaceOperation uint8

const (
	WorkspaceOperationRead WorkspaceOperation = iota + 1
	WorkspaceOperationList
	WorkspaceOperationGlob
	WorkspaceOperationGrep
	WorkspaceOperationDownload
	WorkspaceOperationWrite
	WorkspaceOperationReplace
)

type WorkspaceMapper struct {
	threadRoot    string
	workspaceRoot string
	uploadsRoot   string
	outputsRoot   string
}

func NewWorkspaceMapper(ref domainsandbox.SessionRef) (*WorkspaceMapper, error) {
	normalized, err := domainsandbox.NormalizeSessionRef(ref)
	if err != nil || normalized != ref {
		return nil, domainsandbox.ErrInvalidInput
	}

	threadRoot := logicalUserDataRoot + "/" + strconv.FormatInt(ref.Key.SpaceID, 10) +
		"/" + strconv.FormatInt(ref.Key.UserID, 10) + "/" + ref.Key.ThreadID
	return &WorkspaceMapper{
		threadRoot:    threadRoot,
		workspaceRoot: threadRoot + "/workspace",
		uploadsRoot:   threadRoot + "/uploads",
		outputsRoot:   threadRoot + "/outputs",
	}, nil
}

func (mapper WorkspaceMapper) ResolveExecCWD(logical string) (string, error) {
	if !validCanonicalPath(logical, maxWorkspaceLogicalPathBytes) {
		return "", domainsandbox.ErrInvalidInput
	}
	suffix, ok := pathSuffix(logical, logicalWorkspaceRoot)
	if !ok {
		return "", domainsandbox.ErrInvalidInput
	}
	return mapper.workspaceRoot + suffix, nil
}

func (mapper WorkspaceMapper) ResolveFilePath(operation WorkspaceOperation, logical string) (string, error) {
	if !operation.valid() || !validCanonicalPath(logical, maxWorkspaceLogicalPathBytes) {
		return "", domainsandbox.ErrInvalidInput
	}

	if suffix, ok := pathSuffix(logical, logicalWorkspaceRoot); ok {
		return mapper.workspaceRoot + suffix, nil
	}
	if suffix, ok := pathSuffix(logical, logicalUploadsRoot); ok {
		return mapper.uploadsRoot + suffix, nil
	}
	if suffix, ok := pathSuffix(logical, logicalOutputsRoot); ok {
		return mapper.outputsRoot + suffix, nil
	}
	if suffix, ok := pathSuffix(logical, logicalSkillsRoot); ok && operation.readOnly() {
		return logicalSkillsRoot + suffix, nil
	}
	return "", domainsandbox.ErrInvalidInput
}

func (mapper WorkspaceMapper) ReverseFilePath(physical string) (string, error) {
	maximum := len(mapper.threadRoot) + 1 + maxWorkspaceLogicalPathBytes
	if !validCanonicalPath(physical, maximum) {
		return "", domainsandbox.ErrInvalidInput
	}

	if suffix, ok := pathSuffix(physical, mapper.workspaceRoot); ok {
		return validateReverseMappedPath(logicalWorkspaceRoot + suffix)
	}
	if suffix, ok := pathSuffix(physical, mapper.uploadsRoot); ok {
		return validateReverseMappedPath(logicalUploadsRoot + suffix)
	}
	if suffix, ok := pathSuffix(physical, mapper.outputsRoot); ok {
		return validateReverseMappedPath(logicalOutputsRoot + suffix)
	}
	if suffix, ok := pathSuffix(physical, logicalSkillsRoot); ok {
		return validateReverseMappedPath(logicalSkillsRoot + suffix)
	}
	return "", domainsandbox.ErrInvalidInput
}

func validateReverseMappedPath(logical string) (string, error) {
	if !validCanonicalPath(logical, maxWorkspaceLogicalPathBytes) {
		return "", domainsandbox.ErrInvalidInput
	}
	return logical, nil
}

func (mapper WorkspaceMapper) physicalThreadRoot() string {
	return mapper.threadRoot
}

func (mapper WorkspaceMapper) physicalWorkspaceRoot() string {
	return mapper.workspaceRoot
}

func (WorkspaceMapper) String() string {
	return "aio.WorkspaceMapper{workspace:<redacted>}"
}

func (WorkspaceMapper) GoString() string {
	return "aio.WorkspaceMapper{workspace:<redacted>}"
}

func (WorkspaceMapper) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "aio.WorkspaceMapper{workspace:<redacted>}")
}

func (operation WorkspaceOperation) valid() bool {
	return operation >= WorkspaceOperationRead && operation <= WorkspaceOperationReplace
}

func (operation WorkspaceOperation) readOnly() bool {
	switch operation {
	case WorkspaceOperationRead,
		WorkspaceOperationList,
		WorkspaceOperationGlob,
		WorkspaceOperationGrep,
		WorkspaceOperationDownload:
		return true
	default:
		return false
	}
}

func pathSuffix(value, root string) (string, bool) {
	if value == root {
		return "", true
	}
	if strings.HasPrefix(value, root+"/") {
		return value[len(root):], true
	}
	return "", false
}

func validCanonicalPath(value string, maximum int) bool {
	if value == "" || len(value) > maximum || !utf8.ValidString(value) || !path.IsAbs(value) ||
		path.Clean(value) != value || strings.ContainsRune(value, '\\') {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}
