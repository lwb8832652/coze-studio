// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package systemadmin

import (
	"errors"
	"testing"
)

func TestCanonicalizeEmailsIsStrictAndFailClosed(t *testing.T) {
	for _, test := range []struct {
		name string
		raw string
		want []string
		wantErr bool
	}{
		{name: "empty configuration", raw: "", want: []string{}},
		{name: "canonical and deduplicated", raw: " ADMIN@EXAMPLE.TEST ,admin@example.test ", want: []string{"admin@example.test"}},
		{name: "bad item", raw: "admin@example.test,bad address <", wantErr: true},
		{name: "empty middle item", raw: "admin@example.test,,other@example.test", wantErr: true},
		{name: "trailing comma", raw: "admin@example.test,", wantErr: true},
		{name: "display name", raw: "Admin <admin@example.test>", wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := CanonicalizeEmails(test.raw, 10)
			if test.wantErr {
				if !errors.Is(err, ErrInvalidEmailProjection) {
					t.Fatalf("CanonicalizeEmails() error = %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("CanonicalizeEmails() error = %v", err)
			}
			if len(got) != len(test.want) {
				t.Fatalf("CanonicalizeEmails() = %#v, want %#v", got, test.want)
			}
			for index := range got {
				if got[index] != test.want[index] {
					t.Fatalf("CanonicalizeEmails() = %#v, want %#v", got, test.want)
				}
			}
		})
	}
}

func TestContainsEmailUsesTheSameCanonicalProjection(t *testing.T) {
	if !ContainsEmail(
		"ADMIN@example.test",
		" admin@example.test,other@example.test ",
		10,
	) {
		t.Fatal("canonical administrator was not recognized")
	}
	for _, invalid := range []string{
		"admin@example.test,",
		"admin@example.test,,other@example.test",
		"admin@example.test,bad address <",
	} {
		if ContainsEmail("admin@example.test", invalid, 10) {
			t.Fatalf("invalid administrator projection granted access: %q", invalid)
		}
	}
}

func TestCanonicalizeRequiredEmailsRejectsEmptyAndOverLimit(t *testing.T) {
	for _, test := range []struct {
		name  string
		raw   string
		limit int
	}{
		{name: "empty", raw: "", limit: 10},
		{
			name:  "over limit",
			raw:   "admin-a@example.test,admin-b@example.test,admin-c@example.test",
			limit: 2,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := CanonicalizeRequiredEmailCSV(
				test.raw,
				test.limit,
			); !errors.Is(err, ErrInvalidEmailProjection) {
				t.Fatalf("CanonicalizeRequiredEmailCSV() error = %v", err)
			}
		})
	}
}
