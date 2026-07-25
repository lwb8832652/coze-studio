// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package systemadmin

import (
	"errors"
	"net/mail"
	"strings"
)

var ErrInvalidEmailProjection = errors.New(
	"system administrator email projection is invalid",
)

func CanonicalizeEmails(raw string, limit int) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []string{}, nil
	}
	if limit <= 0 {
		return nil, ErrInvalidEmailProjection
	}
	items := strings.Split(raw, ",")
	seen := make(map[string]struct{}, len(items))
	emails := make([]string, 0, len(items))
	for _, item := range items {
		candidate := strings.TrimSpace(item)
		address, err := mail.ParseAddress(candidate)
		if err != nil ||
			address == nil ||
			address.Name != "" ||
			!strings.EqualFold(candidate, address.Address) {
			return nil, ErrInvalidEmailProjection
		}
		email := strings.ToLower(strings.TrimSpace(address.Address))
		if email == "" {
			return nil, ErrInvalidEmailProjection
		}
		if _, exists := seen[email]; exists {
			continue
		}
		seen[email] = struct{}{}
		emails = append(emails, email)
		if len(emails) > limit {
			return nil, ErrInvalidEmailProjection
		}
	}
	return emails, nil
}

func CanonicalizeEmailCSV(raw string, limit int) (string, error) {
	emails, err := CanonicalizeEmails(raw, limit)
	if err != nil {
		return "", err
	}
	return strings.Join(emails, ","), nil
}

func CanonicalizeRequiredEmails(raw string, limit int) ([]string, error) {
	emails, err := CanonicalizeEmails(raw, limit)
	if err != nil || len(emails) == 0 {
		return nil, ErrInvalidEmailProjection
	}
	return emails, nil
}

func CanonicalizeRequiredEmailCSV(raw string, limit int) (string, error) {
	emails, err := CanonicalizeRequiredEmails(raw, limit)
	if err != nil {
		return "", err
	}
	return strings.Join(emails, ","), nil
}

func ContainsEmail(candidate string, configured string, limit int) bool {
	emails, err := CanonicalizeRequiredEmails(configured, limit)
	if err != nil {
		return false
	}
	canonicalCandidate, err := CanonicalizeEmails(candidate, 1)
	if err != nil || len(canonicalCandidate) != 1 {
		return false
	}
	for _, email := range emails {
		if email == canonicalCandidate[0] {
			return true
		}
	}
	return false
}
