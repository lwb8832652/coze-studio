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

package conv

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
	"unicode"

	"github.com/coze-dev/coze-studio/backend/pkg/lang/ptr"
)

// StrToInt64E returns strconv.ParseInt(v, 10, 64)
func StrToInt64(v string) (int64, error) {
	return strconv.ParseInt(v, 10, 64)
}

// Int64ToStr returns strconv.FormatInt(v, 10) result
func Int64ToStr(v int64) string {
	return strconv.FormatInt(v, 10)
}

func StrToFloat64(v string) (float64, error) {
	return strconv.ParseFloat(v, 64)
}

func StrToFloat64D(v string, defaultValue float64) float64 {
	toV, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return defaultValue
	}

	return toV
}

// StrToInt64 returns strconv.ParseInt(v, 10, 64)'s value.
// if error occurs, returns defaultValue as result.
func StrToInt64D(v string, defaultValue int64) int64 {
	toV, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return defaultValue
	}
	return toV
}

const debugJSONRedactedValue = "[REDACTED]"

// DebugJsonToStr serializes debug data while removing credential-bearing fields.
func DebugJsonToStr(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}

	var value any
	decoder := json.NewDecoder(bytes.NewReader(b))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return ""
	}

	redactDebugJSON(value)
	redacted, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(redacted)
}

func redactDebugJSON(value any) {
	switch typed := value.(type) {
	case map[string]any:
		for key, item := range typed {
			if isSensitiveDebugJSONKey(key) {
				typed[key] = debugJSONRedactedValue
				continue
			}
			redactDebugJSON(item)
		}
	case []any:
		for _, item := range typed {
			redactDebugJSON(item)
		}
	}
}

func isSensitiveDebugJSONKey(key string) bool {
	var normalized strings.Builder
	normalized.Grow(len(key))
	for _, char := range key {
		if unicode.IsLetter(char) || unicode.IsDigit(char) {
			normalized.WriteRune(unicode.ToLower(char))
		}
	}

	compact := normalized.String()
	return strings.Contains(compact, "credential") ||
		strings.Contains(compact, "password") ||
		strings.HasSuffix(compact, "apikey") ||
		strings.HasSuffix(compact, "accesskey") ||
		strings.HasSuffix(compact, "secretkey") ||
		strings.HasSuffix(compact, "authorization") ||
		strings.HasSuffix(compact, "cookie") ||
		strings.HasSuffix(compact, "token") ||
		strings.HasSuffix(compact, "clientsecret") ||
		strings.HasSuffix(compact, "privatekey")
}

func BoolToInt(p bool) int {
	if p == true {
		return 1
	}

	return 0
}

// BoolToIntPointer returns 1 or 0 as pointer
func BoolToIntPointer(p *bool) *int {
	if p == nil {
		return nil
	}

	if *p == true {
		return ptr.Of(int(1))
	}

	return ptr.Of(int(0))
}
