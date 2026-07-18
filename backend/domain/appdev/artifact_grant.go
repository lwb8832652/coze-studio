// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package appdev

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
)

const (
	ArtifactGrantIDBytes        = 32
	ArtifactGrantTokenBytes     = 32
	MinArtifactGrantTTL         = time.Microsecond
	MaxArtifactGrantTTL         = 120 * time.Second
	MaxArtifactGrantObjectBytes = 100 * 1024 * 1024
)

var (
	ErrArtifactGrantInvalid           = errors.New("appdev artifact grant is invalid")
	ErrArtifactGrantDenied            = errors.New("appdev artifact grant is denied")
	ErrArtifactGrantAudienceMismatch  = fmt.Errorf("%w: audience mismatch", ErrArtifactGrantDenied)
	ErrArtifactGrantDirectionMismatch = fmt.Errorf("%w: direction mismatch", ErrArtifactGrantDenied)
	ErrArtifactGrantExpired           = fmt.Errorf("%w: expired", ErrArtifactGrantDenied)
	ErrArtifactGrantRevoked           = fmt.Errorf("%w: revoked", ErrArtifactGrantDenied)
	ErrArtifactGrantConsumed          = fmt.Errorf("%w: consumed", ErrArtifactGrantDenied)
	ErrArtifactGrantConflict          = errors.New("appdev artifact grant conflict")
	ErrArtifactGrantUnavailable       = errors.New("appdev artifact grant unavailable")
	ErrArtifactGrantStorage           = errors.New("appdev artifact storage unavailable")
	ErrArtifactGrantSecret            = errors.New("appdev artifact grant secret cannot be serialized")
)

type ArtifactGrantDirection string

const (
	ArtifactGrantDirectionUpload   ArtifactGrantDirection = "upload"
	ArtifactGrantDirectionDownload ArtifactGrantDirection = "download"
)

type ArtifactGrantState string

const (
	ArtifactGrantStateIssued    ArtifactGrantState = "issued"
	ArtifactGrantStateConsuming ArtifactGrantState = "consuming"
	ArtifactGrantStateConsumed  ArtifactGrantState = "consumed"
	ArtifactGrantStateRevoked   ArtifactGrantState = "revoked"
)

type ArtifactGrantAudience struct {
	SpaceID       string
	ProjectID     string
	ProviderKey   string
	ProviderScope domainsandbox.Scope
	Operation     string
}

func ValidateArtifactGrantAudience(audience ArtifactGrantAudience) error {
	if !validArtifactGrantSpaceID(audience.SpaceID) ||
		!validArtifactGrantIdentifier(audience.ProjectID, 128, false) ||
		!validArtifactGrantIdentifier(audience.ProviderKey, 128, false) ||
		audience.ProviderScope != domainsandbox.ScopeAppDev ||
		!validArtifactGrantIdentifier(audience.Operation, 128, true) {
		return ErrArtifactGrantInvalid
	}
	return nil
}

func ValidateArtifactGrantDirection(direction ArtifactGrantDirection) error {
	if direction != ArtifactGrantDirectionUpload && direction != ArtifactGrantDirectionDownload {
		return ErrArtifactGrantInvalid
	}
	return nil
}

type ArtifactGrantDigest [sha256.Size]byte

func ParseArtifactGrantDigest(value string) (ArtifactGrantDigest, error) {
	const prefix = "sha256:"
	if !strings.HasPrefix(value, prefix) || len(value) != len(prefix)+sha256.Size*2 {
		return ArtifactGrantDigest{}, ErrArtifactGrantInvalid
	}
	raw := value[len(prefix):]
	decoded, err := hex.DecodeString(raw)
	if err != nil || hex.EncodeToString(decoded) != raw || len(decoded) != sha256.Size {
		return ArtifactGrantDigest{}, ErrArtifactGrantInvalid
	}
	var digest ArtifactGrantDigest
	copy(digest[:], decoded)
	return digest, nil
}

func (d ArtifactGrantDigest) IsZero() bool {
	var zero ArtifactGrantDigest
	return subtle.ConstantTimeCompare(d[:], zero[:]) == 1
}

func (d ArtifactGrantDigest) String() string { return "sha256:" + hex.EncodeToString(d[:]) }

type ArtifactGrantSpec struct {
	Audience  ArtifactGrantAudience
	Direction ArtifactGrantDirection
	ObjectKey string
	Digest    ArtifactGrantDigest
	Size      int64
	MaxSize   int64
}

func NormalizeArtifactGrantSpec(input ArtifactGrantSpec) (ArtifactGrantSpec, error) {
	normalized := input
	normalized.Audience.SpaceID = strings.TrimSpace(input.Audience.SpaceID)
	normalized.Audience.ProjectID = strings.TrimSpace(input.Audience.ProjectID)
	normalized.Audience.ProviderKey = strings.TrimSpace(input.Audience.ProviderKey)
	normalized.Audience.Operation = strings.TrimSpace(input.Audience.Operation)
	if ValidateArtifactGrantAudience(normalized.Audience) != nil || ValidateArtifactGrantDirection(normalized.Direction) != nil ||
		!ValidArtifactGrantObjectKey(normalized.ObjectKey) || normalized.Digest.IsZero() ||
		normalized.Size < 0 || normalized.MaxSize <= 0 || normalized.Size > normalized.MaxSize ||
		normalized.MaxSize > MaxArtifactGrantObjectBytes {
		return ArtifactGrantSpec{}, ErrArtifactGrantInvalid
	}
	return normalized, nil
}

func ValidateArtifactGrantTTL(ttl time.Duration) error {
	if ttl < MinArtifactGrantTTL || ttl > MaxArtifactGrantTTL {
		return ErrArtifactGrantInvalid
	}
	return nil
}

func ValidArtifactGrantObjectKey(value string) bool {
	if value == "" || len(value) > 1024 || !utf8.ValidString(value) || strings.TrimSpace(value) != value ||
		strings.HasPrefix(value, "/") || strings.Contains(value, "\\") || path.Clean(value) != value {
		return false
	}
	for _, component := range strings.Split(value, "/") {
		if component == "" || component == "." || component == ".." || containsArtifactGrantControl(component) {
			return false
		}
	}
	return true
}

func (s ArtifactGrantSpec) String() string   { return fmt.Sprintf("%v", s) }
func (s ArtifactGrantSpec) GoString() string { return fmt.Sprintf("%v", s) }
func (s ArtifactGrantSpec) Format(state fmt.State, _ rune) {
	_, _ = fmt.Fprintf(state, "{Audience:%s/%s Provider:%s Scope:%s Operation:%s Direction:%s ObjectKey:[REDACTED] Digest:%s Size:%d MaxSize:%d}",
		s.Audience.SpaceID, s.Audience.ProjectID, s.Audience.ProviderKey, s.Audience.ProviderScope,
		s.Audience.Operation, s.Direction, s.Digest.String(), s.Size, s.MaxSize)
}
func (ArtifactGrantSpec) MarshalJSON() ([]byte, error) { return nil, ErrArtifactGrantSecret }

type ArtifactGrantRecord struct {
	GrantID   ArtifactGrantID
	Spec      ArtifactGrantSpec
	State     ArtifactGrantState
	IssuedAt  time.Time
	ExpiresAt time.Time
}

func (r ArtifactGrantRecord) String() string   { return fmt.Sprintf("%v", r) }
func (r ArtifactGrantRecord) GoString() string { return fmt.Sprintf("%v", r) }
func (r ArtifactGrantRecord) Format(state fmt.State, _ rune) {
	_, _ = fmt.Fprintf(state, "{Spec:%v State:%s IssuedAt:%s ExpiresAt:%s [REDACTED]}",
		r.Spec, r.State, r.IssuedAt.UTC().Format(time.RFC3339Nano), r.ExpiresAt.UTC().Format(time.RFC3339Nano))
}
func (ArtifactGrantRecord) MarshalJSON() ([]byte, error) { return nil, ErrArtifactGrantSecret }

type ArtifactGrantID struct{ value [ArtifactGrantIDBytes]byte }

func NewRandomArtifactGrantID(random io.Reader) (ArtifactGrantID, error) {
	if random == nil {
		return ArtifactGrantID{}, ErrArtifactGrantInvalid
	}
	var value [ArtifactGrantIDBytes]byte
	if _, err := io.ReadFull(random, value[:]); err != nil {
		return ArtifactGrantID{}, ErrArtifactGrantUnavailable
	}
	return ArtifactGrantID{value: value}, nil
}

func ParseArtifactGrantID(value string) (ArtifactGrantID, error) {
	if value == "" || strings.TrimSpace(value) != value {
		return ArtifactGrantID{}, ErrArtifactGrantInvalid
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(decoded) != ArtifactGrantIDBytes || base64.RawURLEncoding.EncodeToString(decoded) != value {
		return ArtifactGrantID{}, ErrArtifactGrantInvalid
	}
	var raw [ArtifactGrantIDBytes]byte
	copy(raw[:], decoded)
	return ArtifactGrantID{value: raw}, nil
}

func (id ArtifactGrantID) Encoded() string { return base64.RawURLEncoding.EncodeToString(id.value[:]) }
func (id ArtifactGrantID) IsZero() bool {
	var zero [ArtifactGrantIDBytes]byte
	return subtle.ConstantTimeCompare(id.value[:], zero[:]) == 1
}
func (id ArtifactGrantID) Equal(other ArtifactGrantID) bool {
	return subtle.ConstantTimeCompare(id.value[:], other.value[:]) == 1
}
func (id ArtifactGrantID) Bytes() []byte { return append([]byte(nil), id.value[:]...) }
func (ArtifactGrantID) String() string   { return "[REDACTED artifact grant identity]" }
func (ArtifactGrantID) GoString() string { return "[REDACTED artifact grant identity]" }
func (ArtifactGrantID) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "[REDACTED artifact grant identity]")
}
func (ArtifactGrantID) MarshalJSON() ([]byte, error) { return nil, ErrArtifactGrantSecret }

type ArtifactGrantToken struct{ bearer string }

func NewRandomArtifactGrantToken(random io.Reader) (ArtifactGrantToken, error) {
	if random == nil {
		return ArtifactGrantToken{}, ErrArtifactGrantInvalid
	}
	raw := make([]byte, ArtifactGrantTokenBytes)
	if _, err := io.ReadFull(random, raw); err != nil {
		return ArtifactGrantToken{}, ErrArtifactGrantUnavailable
	}
	return ParseArtifactGrantToken(base64.RawURLEncoding.EncodeToString(raw))
}

func ParseArtifactGrantToken(value string) (ArtifactGrantToken, error) {
	if value == "" || strings.TrimSpace(value) != value {
		return ArtifactGrantToken{}, ErrArtifactGrantInvalid
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(decoded) != ArtifactGrantTokenBytes || base64.RawURLEncoding.EncodeToString(decoded) != value {
		return ArtifactGrantToken{}, ErrArtifactGrantInvalid
	}
	return ArtifactGrantToken{bearer: value}, nil
}

func (t ArtifactGrantToken) Bearer() string { return t.bearer }
func (t ArtifactGrantToken) IsZero() bool   { return t.bearer == "" }
func (t ArtifactGrantToken) Hash() ArtifactGrantTokenHash {
	return ArtifactGrantTokenHash(sha256.Sum256([]byte(t.bearer)))
}
func (ArtifactGrantToken) String() string   { return "[REDACTED artifact grant token]" }
func (ArtifactGrantToken) GoString() string { return "[REDACTED artifact grant token]" }
func (ArtifactGrantToken) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "[REDACTED artifact grant token]")
}
func (ArtifactGrantToken) MarshalJSON() ([]byte, error) { return nil, ErrArtifactGrantSecret }

type ArtifactGrantTokenHash [sha256.Size]byte

func (h ArtifactGrantTokenHash) IsZero() bool {
	var zero ArtifactGrantTokenHash
	return subtle.ConstantTimeCompare(h[:], zero[:]) == 1
}
func (h ArtifactGrantTokenHash) Equal(other ArtifactGrantTokenHash) bool {
	return subtle.ConstantTimeCompare(h[:], other[:]) == 1
}
func (h ArtifactGrantTokenHash) Bytes() []byte { return append([]byte(nil), h[:]...) }
func (ArtifactGrantTokenHash) String() string  { return "[REDACTED artifact grant token hash]" }
func (ArtifactGrantTokenHash) GoString() string {
	return "[REDACTED artifact grant token hash]"
}
func (ArtifactGrantTokenHash) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "[REDACTED artifact grant token hash]")
}
func (ArtifactGrantTokenHash) MarshalJSON() ([]byte, error) { return nil, ErrArtifactGrantSecret }

type IssueArtifactGrantRepositoryInput struct {
	GrantID   ArtifactGrantID
	TokenHash ArtifactGrantTokenHash
	Spec      ArtifactGrantSpec
	TTL       time.Duration
}

type ConsumeArtifactGrantRepositoryInput struct {
	GrantID   ArtifactGrantID
	TokenHash ArtifactGrantTokenHash
	Audience  ArtifactGrantAudience
	Direction ArtifactGrantDirection
}

type RevokeArtifactGrantRepositoryInput struct {
	GrantID   ArtifactGrantID
	TokenHash ArtifactGrantTokenHash
	Audience  ArtifactGrantAudience
	Direction ArtifactGrantDirection
}

func (input IssueArtifactGrantRepositoryInput) Format(state fmt.State, _ rune) {
	_, _ = fmt.Fprintf(state, "{GrantID:[REDACTED] TokenHash:[REDACTED] Spec:%v TTL:%s}", input.Spec, input.TTL)
}
func (IssueArtifactGrantRepositoryInput) MarshalJSON() ([]byte, error) {
	return nil, ErrArtifactGrantSecret
}
func (input ConsumeArtifactGrantRepositoryInput) Format(state fmt.State, _ rune) {
	_, _ = fmt.Fprintf(state, "{GrantID:[REDACTED] TokenHash:[REDACTED] Audience:%s/%s/%s/%s/%s Direction:%s}",
		input.Audience.SpaceID, input.Audience.ProjectID, input.Audience.ProviderKey,
		input.Audience.ProviderScope, input.Audience.Operation, input.Direction)
}
func (ConsumeArtifactGrantRepositoryInput) MarshalJSON() ([]byte, error) {
	return nil, ErrArtifactGrantSecret
}
func (input RevokeArtifactGrantRepositoryInput) Format(state fmt.State, _ rune) {
	_, _ = fmt.Fprintf(state, "{GrantID:[REDACTED] TokenHash:[REDACTED] Audience:%s/%s/%s/%s/%s Direction:%s}",
		input.Audience.SpaceID, input.Audience.ProjectID, input.Audience.ProviderKey,
		input.Audience.ProviderScope, input.Audience.Operation, input.Direction)
}
func (RevokeArtifactGrantRepositoryInput) MarshalJSON() ([]byte, error) {
	return nil, ErrArtifactGrantSecret
}

type ArtifactGrantRepository interface {
	Issue(context.Context, IssueArtifactGrantRepositoryInput) (*ArtifactGrantRecord, error)
	Consume(context.Context, ConsumeArtifactGrantRepositoryInput) (*ArtifactGrantRecord, error)
	Revoke(context.Context, RevokeArtifactGrantRepositoryInput) (*ArtifactGrantRecord, error)
}

func validArtifactGrantSpaceID(value string) bool {
	if value == "" || len(value) > 20 || value[0] == '0' {
		return false
	}
	parsed, err := strconv.ParseUint(value, 10, 63)
	return err == nil && parsed > 0
}

func validArtifactGrantIdentifier(value string, maxBytes int, allowDot bool) bool {
	if value == "" || len(value) > maxBytes || !utf8.ValidString(value) || strings.TrimSpace(value) != value ||
		value == "." || value == ".." || strings.Contains(value, "..") {
		return false
	}
	for index := 0; index < len(value); index++ {
		character := value[index]
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || character == '-' || character == '_' || (allowDot && character == '.') {
			continue
		}
		return false
	}
	return true
}

func containsArtifactGrantControl(value string) bool {
	for _, character := range value {
		if character < 0x20 || character == 0x7f {
			return true
		}
	}
	return false
}
