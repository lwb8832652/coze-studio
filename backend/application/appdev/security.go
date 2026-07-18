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
	"os"
	"regexp"
	"strings"
)

const maxAppDevVisibleOutputBytes = 16 * 1024

var (
	appDevSecretAssignmentPattern = regexp.MustCompile(`(?i)\b([a-z0-9_.-]*(?:api[_-]?key|token|secret|password|credential)[a-z0-9_.-]*)\s*([=:])\s*("[^"]*"|'[^']*'|[^\s,;]+)`)
	appDevBearerPattern           = regexp.MustCompile(`(?i)\b(authorization\s*:\s*bearer|bearer)\s+[a-z0-9._~+/=-]+`)
	appDevURLCredentialPattern    = regexp.MustCompile(`(?i)([a-z][a-z0-9+.-]*://)[^/@\s]+@`)
)

func SanitizeAppDevOutput(value string) string {
	sanitized := appDevSecretAssignmentPattern.ReplaceAllString(value, `${1}${2}[REDACTED]`)
	sanitized = appDevBearerPattern.ReplaceAllString(sanitized, `${1} [REDACTED]`)
	sanitized = appDevURLCredentialPattern.ReplaceAllString(sanitized, `${1}[REDACTED]@`)

	if home, err := os.UserHomeDir(); err == nil && home != "" && home != "/" {
		sanitized = strings.ReplaceAll(sanitized, home, "$HOME")
	}
	if tempDir := os.TempDir(); tempDir != "" && tempDir != "/" {
		sanitized = strings.ReplaceAll(sanitized, tempDir, "$TMPDIR")
	}
	if len(sanitized) > maxAppDevVisibleOutputBytes {
		sanitized = sanitized[:maxAppDevVisibleOutputBytes] + "\n[output truncated]"
	}
	return sanitized
}

func IsAppDevHostExecutionEnabled() bool {
	return os.Getenv("APP_ENV") == "debug" && os.Getenv("APP_DEV_HOST_RUNTIME_ENABLED") == "true"
}
