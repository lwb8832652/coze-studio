// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package safetext

import (
	"errors"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

var (
	ErrInvalid = errors.New("invalid safe text")
	ErrUnsafe  = errors.New("unsafe text")
)

type Rules struct {
	MaxRunes  int
	Multiline bool
	Required  bool
}

var credentialAssignmentPattern = regexp.MustCompile(
	`(?i)(?:^|[^a-z0-9_])["']?(?:[a-z0-9]+[._-])*(api[_ -]?key|access[_ -]?token|refresh[_ -]?token|client[_ -]?secret|app[_ -]?secret|secret|password|credential|token|cookie|authorization|raw[_ -]?metadata|provider[_ -]?body)["']?\s*[:=]`,
)

var bearerSecretPattern = regexp.MustCompile(
	`(?i)\bbearer\s+[A-Za-z0-9._~+/=-]{8,}`,
)

var knownSecretPattern = regexp.MustCompile(
	`(?i)\b(sk-[A-Za-z0-9]{12,}|AKIA[A-Z0-9]{16})\b`,
)

var credentialURLPattern = regexp.MustCompile(
	`(?i)https?://[^/\s:@]+:[^/\s@]+@`,
)

func Normalize(value string, rules Rules) (string, error) {
	if !utf8.ValidString(value) || rules.MaxRunes <= 0 {
		return "", ErrInvalid
	}
	if rules.Multiline {
		value = strings.ReplaceAll(value, "\r\n", "\n")
		value = strings.ReplaceAll(value, "\r", "\n")
	}
	value = strings.TrimSpace(value)
	if value == "" {
		if rules.Required {
			return "", ErrInvalid
		}
		return "", nil
	}
	if utf8.RuneCountInString(value) > rules.MaxRunes {
		return "", ErrInvalid
	}
	for _, current := range value {
		if current == '<' || current == '>' {
			return "", ErrUnsafe
		}
		if unicode.IsControl(current) &&
			!(rules.Multiline && current == '\n') {
			return "", ErrUnsafe
		}
	}
	lower := strings.ToLower(value)
	if credentialAssignmentPattern.MatchString(value) ||
		bearerSecretPattern.MatchString(value) ||
		knownSecretPattern.MatchString(value) ||
		credentialURLPattern.MatchString(value) ||
		strings.Contains(lower, "-----begin private key-----") ||
		strings.Contains(lower, "s3://") ||
		strings.Contains(lower, "tos://") ||
		strings.Contains(lower, "minio://") {
		return "", ErrUnsafe
	}
	return value, nil
}
