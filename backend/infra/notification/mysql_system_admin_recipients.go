// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package notification

import (
	"context"

	"gorm.io/gorm"

	domainnotification "github.com/coze-dev/coze-studio/backend/domain/notification"
)

type systemAdministratorEmailProjection interface {
	ListCanonicalEmails(context.Context) ([]string, error)
}

type MySQLSystemAdminRecipientSource struct {
	db     *gorm.DB
	emails systemAdministratorEmailProjection
}

func NewMySQLSystemAdminRecipientSource(
	db *gorm.DB,
	emails systemAdministratorEmailProjection,
) *MySQLSystemAdminRecipientSource {
	return &MySQLSystemAdminRecipientSource{
		db:     db,
		emails: emails,
	}
}

func (s *MySQLSystemAdminRecipientSource) ListSystemAdministratorUserIDs(
	ctx context.Context,
) ([]int64, error) {
	if s == nil || s.db == nil || s.emails == nil || ctx == nil {
		return nil, domainnotification.ErrRecipientResolution
	}
	emails, err := s.emails.ListCanonicalEmails(ctx)
	if err != nil {
		return nil, domainnotification.ErrRecipientResolution
	}
	var rows []struct {
		ID    int64  `gorm:"column:id"`
		Email string `gorm:"column:email"`
	}
	if err := s.db.WithContext(ctx).
		Table("user").
		Select("id, email").
		Where("deleted_at IS NULL").
		Where("LOWER(email) IN ?", emails).
		Limit(domainnotification.MaxExplicitRecipients + 1).
		Find(&rows).Error; err != nil {
		return nil, domainnotification.ErrRecipientResolution
	}
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	if len(ids) == 0 {
		return nil, systemAdministratorRecipientsPendingError{}
	}
	return domainnotification.NormalizeRecipientIDs(ids)
}

type systemAdministratorRecipientsPendingError struct{}

func (systemAdministratorRecipientsPendingError) Error() string {
	return "system administrator recipients are not yet available"
}

func (systemAdministratorRecipientsPendingError) Unwrap() error {
	return domainnotification.ErrRecipientResolution
}

func (systemAdministratorRecipientsPendingError) RetryWithoutDeadLetter() bool {
	return true
}
