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

package base

import (
	"context"
	"errors"
	"strings"
	"testing"

	adminconfig "github.com/coze-dev/coze-studio/backend/api/model/admin/config"
	"github.com/coze-dev/coze-studio/backend/pkg/kvstore"
)

type mutableSystemAdminConfigurationReader struct {
	configuration *adminconfig.BasicConfiguration
	revision      string
	err           error
}

func (r *mutableSystemAdminConfigurationReader) GetBaseConfigWithRevision(
	context.Context,
) (*adminconfig.BasicConfiguration, string, error) {
	return r.configuration, r.revision, r.err
}

func TestSystemAdminEmailProjectionPersistedConfigurationHasPriority(
	t *testing.T,
) {
	reader := &mutableSystemAdminConfigurationReader{
		configuration: &adminconfig.BasicConfiguration{
			AdminEmails: " DB-ADMIN@example.com ",
		},
		revision: "7",
	}
	projection := NewSystemAdminEmailProjection(
		reader,
		"bootstrap@example.com",
	)

	emails, err := projection.ListCanonicalEmails(context.Background())
	if err != nil {
		t.Fatalf("resolve persisted administrators: %v", err)
	}
	if len(emails) != 1 || emails[0] != "db-admin@example.com" {
		t.Fatalf("persisted configuration must win, got %#v", emails)
	}
}

func TestSystemAdminEmailProjectionUsesCanonicalBootstrapBeforePersistence(
	t *testing.T,
) {
	reader := &mutableSystemAdminConfigurationReader{
		revision: kvstore.MissingRevision,
	}
	projection := NewSystemAdminEmailProjection(
		reader,
		" Bootstrap@Example.com ",
	)

	emails, err := projection.ListCanonicalEmails(context.Background())
	if err != nil {
		t.Fatalf("resolve bootstrap administrators: %v", err)
	}
	if len(emails) != 1 || emails[0] != "bootstrap@example.com" {
		t.Fatalf("bootstrap configuration was not canonicalized: %#v", emails)
	}
}

func TestSystemAdminEmailProjectionInvalidBootstrapFailsClosedWithoutLeaking(
	t *testing.T,
) {
	const secret = "credential:not-an-email"
	reader := &mutableSystemAdminConfigurationReader{
		revision: kvstore.MissingRevision,
	}
	projection := NewSystemAdminEmailProjection(reader, secret)

	_, err := projection.ListCanonicalEmails(context.Background())
	if !errors.Is(err, ErrSystemAdminEmailProjectionUnavailable) {
		t.Fatalf("expected fail-closed projection error, got %v", err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatal("projection error leaked bootstrap configuration")
	}
}

func TestSystemAdminEmailProjectionStopsBootstrapFallbackAfterPersistence(
	t *testing.T,
) {
	reader := &mutableSystemAdminConfigurationReader{
		revision: kvstore.MissingRevision,
	}
	projection := NewSystemAdminEmailProjection(
		reader,
		"bootstrap@example.com",
	)

	before, err := projection.ListCanonicalEmails(context.Background())
	if err != nil ||
		len(before) != 1 ||
		before[0] != "bootstrap@example.com" {
		t.Fatalf("unexpected bootstrap projection: %#v, %v", before, err)
	}

	reader.configuration = &adminconfig.BasicConfiguration{
		AdminEmails: "persisted@example.com",
	}
	reader.revision = "8"
	after, err := projection.ListCanonicalEmails(context.Background())
	if err != nil {
		t.Fatalf("resolve persisted administrators: %v", err)
	}
	if len(after) != 1 || after[0] != "persisted@example.com" {
		t.Fatalf("persisted projection did not replace bootstrap: %#v", after)
	}
}

func TestSystemAdminEmailProjectionDoesNotFallbackFromInvalidPersistence(
	t *testing.T,
) {
	reader := &mutableSystemAdminConfigurationReader{
		configuration: &adminconfig.BasicConfiguration{
			AdminEmails: "bad address <",
		},
		revision: "9",
	}
	projection := NewSystemAdminEmailProjection(
		reader,
		"bootstrap@example.com",
	)

	_, err := projection.ListCanonicalEmails(context.Background())
	if !errors.Is(err, ErrSystemAdminEmailProjectionUnavailable) {
		t.Fatalf("invalid persisted configuration must fail closed, got %v", err)
	}
}
