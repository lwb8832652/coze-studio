// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package billing

import (
	"context"
	"errors"

	"gorm.io/gorm"

	domainbilling "github.com/coze-dev/coze-studio/backend/domain/billing"
)

func (r *UserRepository) GetOrderForUser(ctx context.Context, userID int64, orderNo string) (*domainbilling.Order, error) {
	if r == nil || userID <= 0 || orderNo == "" {
		return nil, domainbilling.ErrInvalidInput
	}
	var row orderPO
	err := r.db.WithContext(ctx).Where("user_id = ? AND order_no = ?", userID, orderNo).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domainbilling.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return row.toDomain(), nil
}
