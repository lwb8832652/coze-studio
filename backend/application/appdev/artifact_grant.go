// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package appdev

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"time"

	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
)

type ArtifactGrantGenerator interface {
	GenerateToken(context.Context) (domainappdev.ArtifactGrantToken, error)
	GenerateGrantID(context.Context) (domainappdev.ArtifactGrantID, error)
}

type cryptoArtifactGrantGenerator struct{ random io.Reader }

func (g cryptoArtifactGrantGenerator) GenerateToken(ctx context.Context) (domainappdev.ArtifactGrantToken, error) {
	if err := ctx.Err(); err != nil {
		return domainappdev.ArtifactGrantToken{}, err
	}
	return domainappdev.NewRandomArtifactGrantToken(g.random)
}

func (g cryptoArtifactGrantGenerator) GenerateGrantID(ctx context.Context) (domainappdev.ArtifactGrantID, error) {
	if err := ctx.Err(); err != nil {
		return domainappdev.ArtifactGrantID{}, err
	}
	return domainappdev.NewRandomArtifactGrantID(g.random)
}

type ArtifactGrantLifecycleOption func(*ArtifactGrantLifecycleService) error

func WithArtifactGrantGenerator(generator ArtifactGrantGenerator) ArtifactGrantLifecycleOption {
	return func(service *ArtifactGrantLifecycleService) error {
		if generator == nil {
			return domainappdev.ErrArtifactGrantInvalid
		}
		service.generator = generator
		return nil
	}
}

type ArtifactGrantLifecycleService struct {
	repository domainappdev.ArtifactGrantRepository
	generator  ArtifactGrantGenerator
}

func NewArtifactGrantLifecycleService(repository domainappdev.ArtifactGrantRepository, options ...ArtifactGrantLifecycleOption) (*ArtifactGrantLifecycleService, error) {
	if repository == nil {
		return nil, domainappdev.ErrArtifactGrantInvalid
	}
	service := &ArtifactGrantLifecycleService{
		repository: repository,
		generator:  cryptoArtifactGrantGenerator{random: rand.Reader},
	}
	for _, option := range options {
		if option == nil || option(service) != nil {
			return nil, domainappdev.ErrArtifactGrantInvalid
		}
	}
	return service, nil
}

type IssueArtifactGrantRequest struct {
	Spec domainappdev.ArtifactGrantSpec
	TTL  time.Duration
}

type ArtifactGrantCapability struct {
	grantID   domainappdev.ArtifactGrantID
	token     domainappdev.ArtifactGrantToken
	audience  domainappdev.ArtifactGrantAudience
	direction domainappdev.ArtifactGrantDirection
	digest    domainappdev.ArtifactGrantDigest
	size      int64
	maxSize   int64
	issuedAt  time.Time
	expiresAt time.Time
}

func (capability *ArtifactGrantCapability) GrantID() domainappdev.ArtifactGrantID {
	if capability == nil {
		return domainappdev.ArtifactGrantID{}
	}
	return capability.grantID
}

func (capability *ArtifactGrantCapability) BearerToken() string {
	if capability == nil {
		return ""
	}
	return capability.token.Bearer()
}

func (capability *ArtifactGrantCapability) ExpiresAt() time.Time {
	if capability == nil {
		return time.Time{}
	}
	return capability.expiresAt
}

func (*ArtifactGrantCapability) String() string   { return "[REDACTED artifact grant capability]" }
func (*ArtifactGrantCapability) GoString() string { return "[REDACTED artifact grant capability]" }
func (*ArtifactGrantCapability) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "[REDACTED artifact grant capability]")
}
func (*ArtifactGrantCapability) MarshalJSON() ([]byte, error) {
	return nil, domainappdev.ErrArtifactGrantSecret
}

func (service *ArtifactGrantLifecycleService) Issue(ctx context.Context, request IssueArtifactGrantRequest) (*ArtifactGrantCapability, error) {
	if ctx == nil {
		return nil, domainappdev.ErrArtifactGrantInvalid
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	normalized, err := domainappdev.NormalizeArtifactGrantSpec(request.Spec)
	if err != nil || domainappdev.ValidateArtifactGrantTTL(request.TTL) != nil {
		return nil, domainappdev.ErrArtifactGrantInvalid
	}
	token, err := service.generator.GenerateToken(ctx)
	if err != nil || token.IsZero() {
		return nil, normalizeArtifactGrantLifecycleError(ctx, err)
	}
	grantID, err := service.generator.GenerateGrantID(ctx)
	if err != nil || grantID.IsZero() {
		return nil, normalizeArtifactGrantLifecycleError(ctx, err)
	}
	if grantID.Encoded() == token.Bearer() {
		return nil, domainappdev.ErrArtifactGrantUnavailable
	}
	record, err := service.repository.Issue(ctx, domainappdev.IssueArtifactGrantRepositoryInput{
		GrantID: grantID, TokenHash: token.Hash(), Spec: normalized, TTL: request.TTL,
	})
	if err != nil {
		return nil, normalizeArtifactGrantLifecycleError(ctx, err)
	}
	if !validArtifactGrantLifecycleRecord(record, grantID, normalized.Audience, normalized.Direction, domainappdev.ArtifactGrantStateIssued) ||
		record.Spec != normalized || record.ExpiresAt.Sub(record.IssuedAt) > request.TTL {
		return nil, domainappdev.ErrArtifactGrantUnavailable
	}
	return &ArtifactGrantCapability{
		grantID: grantID, token: token, audience: normalized.Audience, direction: normalized.Direction,
		digest: normalized.Digest, size: normalized.Size, maxSize: normalized.MaxSize,
		issuedAt: record.IssuedAt, expiresAt: record.ExpiresAt,
	}, nil
}

type ConsumeArtifactGrantRequest struct {
	GrantID   domainappdev.ArtifactGrantID
	Token     domainappdev.ArtifactGrantToken
	Audience  domainappdev.ArtifactGrantAudience
	Direction domainappdev.ArtifactGrantDirection
}

func (service *ArtifactGrantLifecycleService) Consume(ctx context.Context, request ConsumeArtifactGrantRequest) (*domainappdev.ArtifactGrantRecord, error) {
	if err := validateArtifactGrantLifecycleRequest(ctx, request.GrantID, request.Token, request.Audience, request.Direction); err != nil {
		return nil, err
	}
	record, err := service.repository.Consume(ctx, domainappdev.ConsumeArtifactGrantRepositoryInput{
		GrantID: request.GrantID, TokenHash: request.Token.Hash(), Audience: request.Audience, Direction: request.Direction,
	})
	if err != nil {
		return nil, normalizeArtifactGrantLifecycleError(ctx, err)
	}
	if !validArtifactGrantLifecycleRecord(record, request.GrantID, request.Audience, request.Direction, domainappdev.ArtifactGrantStateConsumed) {
		return nil, domainappdev.ErrArtifactGrantUnavailable
	}
	result := *record
	return &result, nil
}

type RevokeArtifactGrantRequest struct {
	GrantID   domainappdev.ArtifactGrantID
	Token     domainappdev.ArtifactGrantToken
	Audience  domainappdev.ArtifactGrantAudience
	Direction domainappdev.ArtifactGrantDirection
}

func (service *ArtifactGrantLifecycleService) Revoke(ctx context.Context, request RevokeArtifactGrantRequest) error {
	if err := validateArtifactGrantLifecycleRequest(ctx, request.GrantID, request.Token, request.Audience, request.Direction); err != nil {
		return err
	}
	record, err := service.repository.Revoke(ctx, domainappdev.RevokeArtifactGrantRepositoryInput{
		GrantID: request.GrantID, TokenHash: request.Token.Hash(), Audience: request.Audience, Direction: request.Direction,
	})
	if err != nil {
		return normalizeArtifactGrantLifecycleError(ctx, err)
	}
	if !validArtifactGrantLifecycleRecord(record, request.GrantID, request.Audience, request.Direction, domainappdev.ArtifactGrantStateRevoked) {
		return domainappdev.ErrArtifactGrantUnavailable
	}
	return nil
}

func validateArtifactGrantLifecycleRequest(ctx context.Context, grantID domainappdev.ArtifactGrantID, token domainappdev.ArtifactGrantToken, audience domainappdev.ArtifactGrantAudience, direction domainappdev.ArtifactGrantDirection) error {
	if ctx == nil || grantID.IsZero() || token.IsZero() || domainappdev.ValidateArtifactGrantAudience(audience) != nil || domainappdev.ValidateArtifactGrantDirection(direction) != nil {
		return domainappdev.ErrArtifactGrantInvalid
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

func validArtifactGrantLifecycleRecord(record *domainappdev.ArtifactGrantRecord, grantID domainappdev.ArtifactGrantID, audience domainappdev.ArtifactGrantAudience, direction domainappdev.ArtifactGrantDirection, state domainappdev.ArtifactGrantState) bool {
	if record == nil || !record.GrantID.Equal(grantID) || record.Spec.Audience != audience || record.Spec.Direction != direction ||
		record.State != state || record.IssuedAt.IsZero() || !record.ExpiresAt.After(record.IssuedAt) ||
		record.ExpiresAt.Sub(record.IssuedAt) > domainappdev.MaxArtifactGrantTTL {
		return false
	}
	normalized, err := domainappdev.NormalizeArtifactGrantSpec(record.Spec)
	return err == nil && normalized == record.Spec
}

func normalizeArtifactGrantLifecycleError(ctx context.Context, err error) error {
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	if err == nil {
		return domainappdev.ErrArtifactGrantUnavailable
	}
	for _, safe := range []error{
		domainappdev.ErrArtifactGrantAudienceMismatch,
		domainappdev.ErrArtifactGrantDirectionMismatch,
		domainappdev.ErrArtifactGrantExpired,
		domainappdev.ErrArtifactGrantRevoked,
		domainappdev.ErrArtifactGrantConsumed,
		domainappdev.ErrArtifactGrantDenied,
		domainappdev.ErrArtifactGrantInvalid,
		domainappdev.ErrArtifactGrantConflict,
		domainappdev.ErrArtifactGrantUnavailable,
	} {
		if errors.Is(err, safe) {
			return safe
		}
	}
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	return domainappdev.ErrArtifactGrantUnavailable
}
