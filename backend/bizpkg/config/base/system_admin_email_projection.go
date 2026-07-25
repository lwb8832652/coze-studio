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

	adminconfig "github.com/coze-dev/coze-studio/backend/api/model/admin/config"
	domainnotification "github.com/coze-dev/coze-studio/backend/domain/notification"
	domainsystemadmin "github.com/coze-dev/coze-studio/backend/domain/systemadmin"
	"github.com/coze-dev/coze-studio/backend/pkg/kvstore"
)

var ErrSystemAdminEmailProjectionUnavailable = errors.New(
	"system administrator email projection unavailable",
)

type systemAdminBasicConfigurationReader interface {
	GetBaseConfigWithRevision(
		context.Context,
	) (*adminconfig.BasicConfiguration, string, error)
}

// SystemAdminEmailProjection is the single fail-closed source used by HTTP
// authorization and durable notification recipient resolution.
type SystemAdminEmailProjection struct {
	reader          systemAdminBasicConfigurationReader
	bootstrapEmails []string
	bootstrapErr    error
}

func NewSystemAdminEmailProjection(
	reader systemAdminBasicConfigurationReader,
	rawBootstrap string,
) *SystemAdminEmailProjection {
	projection := &SystemAdminEmailProjection{reader: reader}
	emails, err := domainsystemadmin.CanonicalizeRequiredEmails(
		rawBootstrap,
		domainnotification.MaxExplicitRecipients,
	)
	if err != nil {
		projection.bootstrapErr = ErrSystemAdminEmailProjectionUnavailable
		return projection
	}
	projection.bootstrapEmails = emails
	return projection
}

func (p *SystemAdminEmailProjection) ListCanonicalEmails(
	ctx context.Context,
) ([]string, error) {
	if p == nil || p.reader == nil || ctx == nil {
		return nil, ErrSystemAdminEmailProjectionUnavailable
	}
	configuration, revision, err := p.reader.GetBaseConfigWithRevision(ctx)
	if err != nil {
		return nil, ErrSystemAdminEmailProjectionUnavailable
	}
	if isPersistedBasicConfigurationRevision(revision) {
		if configuration == nil {
			return nil, ErrSystemAdminEmailProjectionUnavailable
		}
		emails, canonicalizeErr := domainsystemadmin.CanonicalizeRequiredEmails(
			configuration.AdminEmails,
			domainnotification.MaxExplicitRecipients,
		)
		if canonicalizeErr != nil {
			return nil, ErrSystemAdminEmailProjectionUnavailable
		}
		return append([]string(nil), emails...), nil
	}
	if p.bootstrapErr != nil || len(p.bootstrapEmails) == 0 {
		return nil, ErrSystemAdminEmailProjectionUnavailable
	}
	return append([]string(nil), p.bootstrapEmails...), nil
}

func (p *SystemAdminEmailProjection) CanonicalEmailCSV(
	ctx context.Context,
) (string, error) {
	emails, err := p.ListCanonicalEmails(ctx)
	if err != nil {
		return "", err
	}
	return strings.Join(emails, ","), nil
}

func isPersistedBasicConfigurationRevision(revision string) bool {
	revision = strings.TrimSpace(revision)
	return revision != "" && revision != kvstore.MissingRevision
}
