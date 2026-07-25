// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package mcptool

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

type MySQLSpaceMemberRoleReader struct {
	db *gorm.DB
}

type mcpSpaceUserRolePO struct {
	UserID   int64 `gorm:"column:user_id"`
	RoleType int32 `gorm:"column:role_type"`
}

func (mcpSpaceUserRolePO) TableName() string {
	return "space_user"
}

func NewMySQLSpaceMemberRoleReader(db *gorm.DB) *MySQLSpaceMemberRoleReader {
	return &MySQLSpaceMemberRoleReader{db: db}
}

func (r *MySQLSpaceMemberRoleReader) ListSpaceMemberRoles(
	ctx context.Context,
	spaceID int64,
) ([]SpaceMemberRole, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("mcp space member role reader db is required")
	}
	if spaceID <= 0 {
		return nil, nil
	}
	var rows []mcpSpaceUserRolePO
	if err := r.db.WithContext(ctx).
		Select("user_id", "role_type").
		Where("space_id = ?", spaceID).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]SpaceMemberRole, 0, len(rows))
	for _, row := range rows {
		result = append(result, SpaceMemberRole{
			UserID:   row.UserID,
			RoleType: row.RoleType,
		})
	}
	return result, nil
}
