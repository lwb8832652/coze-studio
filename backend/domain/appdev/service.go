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

package appdev

import (
	"errors"
	"path"
	"strings"
)

var (
	ErrInvalidPath = errors.New("invalid appdev file path")
	ErrNotFound    = errors.New("appdev resource not found")
)

func NormalizeRelativePath(filePath string) (string, error) {
	normalized := strings.TrimSpace(strings.ReplaceAll(filePath, "\\", "/"))
	if normalized == "" {
		return "", ErrInvalidPath
	}

	if len(normalized) > 512 {
		return "", ErrInvalidPath
	}

	if strings.ContainsRune(normalized, 0) {
		return "", ErrInvalidPath
	}

	if strings.HasPrefix(normalized, "/") {
		return "", ErrInvalidPath
	}

	cleaned := path.Clean(normalized)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", ErrInvalidPath
	}

	return cleaned, nil
}
