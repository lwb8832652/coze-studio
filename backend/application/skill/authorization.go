/*
 * Copyright 2025 coze-dev Authors
 * Licensed under the Apache License, Version 2.0 (the "License");
 */

package skill

import (
	"context"
	"errors"
	"fmt"

	"github.com/coze-dev/coze-studio/backend/application/base/ctxutil"
	"github.com/coze-dev/coze-studio/backend/domain/skill/entity"
	userentity "github.com/coze-dev/coze-studio/backend/domain/user/entity"
)

var (
	ErrSkillAccessDenied = errors.New("skill workspace access denied")
	ErrSkillReadOnly     = errors.New("builtin skill is read-only")
)

type UserSpaceReader interface {
	GetUserSpaceList(ctx context.Context, userID int64) ([]*userentity.Space, error)
}

func (s *ApplicationService) authorizeSpace(ctx context.Context, spaceID int64) error {
	if s.UserSpaceReader == nil {
		return nil
	}
	userID := ctxutil.GetUIDFromCtx(ctx)
	if userID == nil || *userID <= 0 || spaceID <= 0 {
		return ErrSkillAccessDenied
	}
	spaces, err := s.UserSpaceReader.GetUserSpaceList(ctx, *userID)
	if err != nil {
		return fmt.Errorf("resolve skill workspace role: %w", err)
	}
	for _, space := range spaces {
		if space != nil && space.ID == spaceID && space.RoleType >= 1 && space.RoleType <= 3 {
			return nil
		}
	}
	return ErrSkillAccessDenied
}

func (s *ApplicationService) authorizeSpaceWrite(ctx context.Context, spaceID int64) error {
	if s.UserSpaceReader == nil {
		return nil
	}
	userID := ctxutil.GetUIDFromCtx(ctx)
	if userID == nil || *userID <= 0 || spaceID <= 0 {
		return ErrSkillAccessDenied
	}
	spaces, err := s.UserSpaceReader.GetUserSpaceList(ctx, *userID)
	if err != nil {
		return fmt.Errorf("resolve skill workspace development permission: %w", err)
	}
	for _, space := range spaces {
		if space != nil && space.ID == spaceID && space.RoleType >= 1 && space.RoleType <= 3 && space.AllowDevelop {
			return nil
		}
	}
	return ErrSkillAccessDenied
}

func (s *ApplicationService) authorizeSkill(ctx context.Context, skillID int64) error {
	if s.UserSpaceReader == nil {
		return nil
	}
	skill, err := s.DomainSVC.Get(ctx, skillID)
	if err != nil {
		return err
	}
	return s.authorizeSpace(ctx, skill.SpaceID)
}

func (s *ApplicationService) authorizeSkillWrite(ctx context.Context, skillID int64) (*entity.Skill, error) {
	if err := s.requireDomainSVC(); err != nil {
		return nil, err
	}
	skill, err := s.DomainSVC.Get(ctx, skillID)
	if err != nil {
		return nil, err
	}
	if err := s.authorizeSpaceWrite(ctx, skill.SpaceID); err != nil {
		return nil, err
	}
	if skill.Type == entity.TypeDeerSkill || skill.Type == entity.TypePublicSkill {
		return nil, ErrSkillReadOnly
	}
	return skill, nil
}

func IsAccessDenied(err error) bool {
	return errors.Is(err, ErrSkillAccessDenied) || errors.Is(err, ErrSkillReadOnly)
}
