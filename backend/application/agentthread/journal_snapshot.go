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

package agentthread

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	domainrepo "github.com/coze-dev/coze-studio/backend/domain/agentthread/repository"
	infrastorage "github.com/coze-dev/coze-studio/backend/infra/storage"
	"github.com/google/uuid"
)

const (
	JournalSnapshotCacheControl         = "private, no-store"
	JournalErrorCodeSnapshotUnavailable = "SNAPSHOT_UNAVAILABLE"
	JournalErrorCodeNoPermission        = "NO_PERMISSION"
	journalSnapshotObjectPrefix         = "journal-snapshots/staging"
	journalSnapshotMaxContentBytes      = 16 * 1024 * 1024
	journalSnapshotMaxSummaryBytes      = 128 * 1024
	journalSnapshotInlineContentBytes   = 64 * 1024
	journalSnapshotFragmentBytes        = 32 * 1024
	journalSnapshotInlineFragmentBytes  = 8 * 1024
	journalSnapshotDefaultFragmentLimit = 100
	journalSnapshotMaxFragmentLimit     = 200
	journalSnapshotSignedURLTTLSeconds  = 60
	journalSnapshotReservationTTL       = 15 * time.Minute
)

type JournalSnapshotFragmentKind = entity.JournalSnapshotFragmentKind

const (
	JournalSnapshotFragmentKindDocumentBlock    = entity.JournalSnapshotFragmentKindDocumentBlock
	JournalSnapshotFragmentKindDocumentChapters = entity.JournalSnapshotFragmentKindDocumentChapters
	JournalSnapshotFragmentKindTerminalStdout   = entity.JournalSnapshotFragmentKindTerminalStdout
	JournalSnapshotFragmentKindTerminalStderr   = entity.JournalSnapshotFragmentKindTerminalStderr
	JournalSnapshotFragmentKindCodeLines        = entity.JournalSnapshotFragmentKindCodeLines
	JournalSnapshotFragmentKindCodeHighlights   = entity.JournalSnapshotFragmentKindCodeHighlights
	JournalSnapshotFragmentKindSkillItems       = entity.JournalSnapshotFragmentKindSkillItems
	JournalSnapshotFragmentKindBrowserThumbnail = entity.JournalSnapshotFragmentKindBrowserThumbnail
	JournalSnapshotFragmentKindBrowserSnapshot  = entity.JournalSnapshotFragmentKindBrowserSnapshot
	JournalSnapshotFragmentKindBrowserAnalysis  = entity.JournalSnapshotFragmentKindBrowserAnalysis
)

var (
	ErrJournalSnapshotUnsafeContent       = errors.New("journal snapshot content is unsafe")
	ErrJournalSnapshotNoPermission        = errors.New("journal snapshot access denied")
	ErrJournalSnapshotUnavailable         = errors.New("journal snapshot is unavailable")
	ErrJournalSnapshotAuditUnavailable    = errors.New("journal snapshot access audit is unavailable")
	ErrJournalSnapshotActionUnavailable   = errors.New("journal snapshot action is unavailable")
	ErrJournalSnapshotIdempotencyConflict = errors.New("journal snapshot action idempotency target conflict")

	journalSnapshotIDPattern     = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,64}$`)
	journalSnapshotKeyPattern    = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,191}$`)
	journalRevisionPattern       = regexp.MustCompile(`^[A-Fa-f0-9]{7,64}$`)
	journalSourceRevisionPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
	journalUnsafeHTMLPattern     = regexp.MustCompile(`(?is)<\s*(?:script|style|iframe|object|embed|svg|math|link|meta|form|input|button|video|audio|html|body)\b`)
	journalAnyHTMLPattern        = regexp.MustCompile(`(?s)<\s*/?\s*[A-Za-z!][^>]*>`)
	journalANSIPattern           = regexp.MustCompile(`\x1b(?:\[[0-?]*[ -/]*[@-~]|\][^\a]*(?:\a|\x1b\\))`)
	journalCredentialPathPattern = regexp.MustCompile(`(?i)(?:^|/)(?:\.env(?:\..*)?|id_(?:rsa|dsa|ecdsa|ed25519)|credentials?|secrets?\.(?:json|ya?ml|toml)|\.npmrc|\.pypirc|[^/]+\.(?:pem|key|p12|pfx))$`)
	journalSecretPattern         = regexp.MustCompile(`(?i)(-----BEGIN(?: [A-Z0-9]+)? PRIVATE KEY-----|authorization\s*:\s*bearer\s+\S+|bearer\s+[A-Za-z0-9._~+/=-]{8,}|["']?(?:api[_-]?key|access[_-]?token|client[_-]?secret|aws[_-]?secret[_-]?access[_-]?key|secret[_-]?(?:key|token)|private[_-]?key|credential|password|(?:[a-z0-9][a-z0-9_-]*_)?(?:secret|token))["']?\s*[:=]\s*["']?\S+|(?:^|[^A-Za-z0-9])(?:sk|ghp|github_pat|xox[baprs])[-_][A-Za-z0-9_-]{4,})`)
	journalShellEnvPattern       = regexp.MustCompile(`(?m)(^|[\s;&|])([A-Za-z_][A-Za-z0-9_]*)=(?:"[^"\r\n]*"|'[^'\r\n]*'|[^\s;&|]+)`)
	journalURLUserInfoPattern    = regexp.MustCompile(`(?i)([a-z][a-z0-9+.-]*://)[^/\s:@]+:[^@/\s]+@`)
	journalCLIUserPattern        = regexp.MustCompile(`(?i)((?:^|[\s;&|])(?:-u|--user)(?:=|\s+))(?:"[^"\r\n]*"|'[^'\r\n]*'|[^\s;&|]+)`)
	journalCLIPasswordPattern    = regexp.MustCompile(`(?i)((?:^|[\s;&|])--password(?:=|\s+))(?:"[^"\r\n]*"|'[^'\r\n]*'|[^\s;&|]+)`)
	journalCLIShortPassword      = regexp.MustCompile(`(?i)((?:^|[\s;&|])-p)([^\s;&|]+)`)
	journalUnsafeSchemePattern   = regexp.MustCompile(`(?i)(?:javascript|data|vbscript)\s*:`)
)

type JournalContentProducer interface {
	ProduceJournalContent(
		ctx context.Context,
		req JournalRuntimeContentSubmission,
	) (*entity.JournalContentSnapshot, *entity.JournalEvent, error)
}

type JournalSnapshotAttemptReader interface {
	GetActiveJournalAttempt(context.Context, int64) (*entity.RunAttempt, error)
}

// JournalRuntimeContentSubmission accepts only typed producer data. Execution
// scope, Attempt, ACL, idempotency and trace identity are resolved server-side.
type JournalRuntimeContentSubmission struct {
	Run         *RunSummary
	Revision    uint32
	Status      entity.JournalContentStatus
	ContentType entity.JournalSnapshotContentType
	ErrorCode   string
	Source      JournalSnapshotSource
	Action      JournalContentAction
	Content     JournalTypedSnapshotContent
}

type JournalContentSubmission struct {
	SpaceID        int64
	ThreadID       int64
	RunID          int64
	AttemptID      string
	SnapshotID     string
	Revision       uint32
	IdempotencyKey string
	TraceID        string
	ACLDomain      string
	Status         entity.JournalContentStatus
	ContentType    entity.JournalSnapshotContentType
	ErrorCode      string
	Source         JournalSnapshotSource
	Action         JournalContentAction
	Content        JournalTypedSnapshotContent
}

type JournalContentAction struct {
	ActionID             string `json:"action_id"`
	MilestoneID          string `json:"milestone_id,omitempty"`
	Operation            string `json:"operation"`
	Target               string `json:"target"`
	DisplayVerbRunning   string `json:"display_verb_running"`
	DisplayVerbCompleted string `json:"display_verb_completed"`
	ParentEventID        int64  `json:"-"`
}

type JournalSnapshotSource struct {
	ResourceType string
	ResourceID   string
	Revision     string
}

type JournalTypedSnapshotContent struct {
	Document *JournalDocumentContent `json:"document,omitempty"`
	Terminal *JournalTerminalContent `json:"terminal,omitempty"`
	Code     *JournalCodeContent     `json:"code,omitempty"`
	Skill    *JournalSkillContent    `json:"skill,omitempty"`
	Browser  *JournalBrowserContent  `json:"browser,omitempty"`
}

type JournalDocumentChapter struct {
	ChapterID string `json:"chapter_id"`
	Title     string `json:"title"`
	Level     int32  `json:"level"`
}

type JournalDocumentContent struct {
	Token             string                   `json:"token,omitempty"`
	Title             string                   `json:"title"`
	Format            string                   `json:"format,omitempty"`
	Chapters          []JournalDocumentChapter `json:"chapters,omitempty"`
	Content           string                   `json:"content,omitempty"`
	ActiveBlock       string                   `json:"active_block,omitempty"`
	Revision          string                   `json:"revision,omitempty"`
	SyncStatus        string                   `json:"sync_status,omitempty"`
	OriginalURL       string                   `json:"-"`
	OriginalObjectKey string                   `json:"-"`
}

type JournalTerminalContent struct {
	SessionID        string `json:"session_id,omitempty"`
	Command          string `json:"command"`
	WorkingDirectory string `json:"working_directory,omitempty"`
	StartedAt        int64  `json:"started_at,omitempty"`
	FinishedAt       int64  `json:"finished_at,omitempty"`
	Stdout           string `json:"stdout,omitempty"`
	Stderr           string `json:"stderr,omitempty"`
	ExitCode         *int32 `json:"exit_code,omitempty"`
	DurationMS       int64  `json:"duration_ms,omitempty"`
}

type JournalCodeHighlight struct {
	StartLine int32  `json:"start_line"`
	EndLine   int32  `json:"end_line"`
	Kind      string `json:"kind,omitempty"`
}

type JournalCodeContent struct {
	Repository string                 `json:"repository"`
	Revision   string                 `json:"revision"`
	Path       string                 `json:"path"`
	Language   string                 `json:"language,omitempty"`
	Content    string                 `json:"content,omitempty"`
	StartLine  int32                  `json:"start_line,omitempty"`
	EndLine    int32                  `json:"end_line,omitempty"`
	Highlights []JournalCodeHighlight `json:"highlights,omitempty"`
}

type JournalSkillSummary struct {
	SkillID           string `json:"skill_id"`
	Name              string `json:"name"`
	Description       string `json:"description,omitempty"`
	InvocationSummary string `json:"-"`
}

type JournalSkillContent struct {
	Skills []JournalSkillSummary `json:"skills"`
}

type JournalBrowserContent struct {
	CaptureID              string   `json:"capture_id"`
	Resource               string   `json:"resource,omitempty"`
	Title                  string   `json:"title,omitempty"`
	Thumbnail              []byte   `json:"thumbnail,omitempty"`
	StaticSnapshot         []byte   `json:"static_snapshot,omitempty"`
	MIMEType               string   `json:"mime_type"`
	Analysis               []string `json:"analysis,omitempty"`
	Index                  int32    `json:"index,omitempty"`
	Total                  int32    `json:"total,omitempty"`
	Redacted               bool     `json:"redacted"`
	RedactionEvidenceID    string   `json:"redaction_evidence_id"`
	RedactionPolicyVersion string   `json:"redaction_policy_version"`
}

type JournalBrowserRedactionEvidence struct {
	CaptureID              string
	RedactionEvidenceID    string
	RedactionPolicyVersion string
	MIMEType               string
	StaticSnapshotHash     string
	ThumbnailHash          string
	PublicFieldsHash       string
}

type JournalBrowserRedactionVerifier interface {
	VerifyJournalBrowserRedaction(context.Context, JournalBrowserRedactionEvidence) error
}

type JournalSnapshotAccessRequest struct {
	SpaceID            int64
	ThreadID           int64
	RunID              int64
	ViewerID           int64
	SnapshotID         string
	SourceResourceType string
	SourceResourceID   string
}

type JournalSnapshotAuthorizer interface {
	AuthorizeJournalSnapshotAccess(context.Context, JournalSnapshotAccessRequest) error
}

type JournalSnapshotRuntimeFileReader interface {
	GetFileByID(context.Context, int64) (*entity.AgentFile, error)
}

type JournalSnapshotArtifactReader interface {
	GetArtifact(context.Context, int64, int64) (*entity.AgentArtifact, error)
}

type JournalSnapshotArtifactCapabilityIssuer interface {
	CreateArtifactSignedURL(
		context.Context,
		*CreateArtifactSignedURLRequest,
	) (*CreateArtifactSignedURLResponse, error)
}

type ThreadOwnerJournalSnapshotAuthorizer struct {
	threadAuthorizer    ThreadAuthorizer
	workspaceAuthorizer WorkspaceAuthorizer
}

func NewThreadOwnerJournalSnapshotAuthorizer(
	threadAuthorizer ThreadAuthorizer,
	workspaceAuthorizer WorkspaceAuthorizer,
) *ThreadOwnerJournalSnapshotAuthorizer {
	return &ThreadOwnerJournalSnapshotAuthorizer{
		threadAuthorizer: threadAuthorizer, workspaceAuthorizer: workspaceAuthorizer,
	}
}

func (a *ThreadOwnerJournalSnapshotAuthorizer) AuthorizeJournalSnapshotAccess(
	ctx context.Context,
	req JournalSnapshotAccessRequest,
) error {
	if a == nil || a.threadAuthorizer == nil || a.workspaceAuthorizer == nil {
		return ErrJournalSnapshotUnavailable
	}
	if err := a.workspaceAuthorizer.AuthorizeWorkspaceAccess(ctx, WorkspaceAccessRequest{
		ViewerID: req.ViewerID, SpaceID: req.SpaceID,
	}); err != nil {
		return journalSnapshotAuthorizationError(err)
	}
	err := a.threadAuthorizer.AuthorizeThreadAccess(ctx, ThreadAccessRequest{
		ViewerID: req.ViewerID, SpaceID: req.SpaceID,
		ThreadID: req.ThreadID, RunID: req.RunID,
	})
	if err != nil {
		return journalSnapshotAuthorizationError(err)
	}
	return nil
}

func journalSnapshotAuthorizationError(err error) error {
	if errors.Is(err, ErrThreadAccessDenied) || errors.Is(err, ErrJournalSnapshotNoPermission) {
		return ErrJournalSnapshotNoPermission
	}
	return fmt.Errorf("%w: authorization backend failed", ErrJournalSnapshotUnavailable)
}

type JournalSnapshotObjectInfo struct {
	Key          string
	LastModified int64
}

type JournalSnapshotObjectStorage interface {
	PutJournalSnapshotObject(context.Context, string, []byte) error
	OpenJournalSnapshotObject(context.Context, string) (io.ReadCloser, error)
	DeleteJournalSnapshotObject(context.Context, string) error
	ListJournalSnapshotObjects(
		context.Context,
		string,
		string,
		int,
	) ([]JournalSnapshotObjectInfo, string, error)
}

type journalSnapshotStorageAdapter struct {
	storage infrastorage.Storage
}

func newJournalSnapshotStorageAdapter(
	storage infrastorage.Storage,
) JournalSnapshotObjectStorage {
	if storage == nil {
		return nil
	}
	if _, ok := storage.(infrastorage.StreamingStorage); !ok {
		return nil
	}
	return &journalSnapshotStorageAdapter{storage: storage}
}

func (a *journalSnapshotStorageAdapter) PutJournalSnapshotObject(
	ctx context.Context,
	key string,
	content []byte,
) error {
	return a.storage.PutObject(ctx, key, content)
}

func (a *journalSnapshotStorageAdapter) OpenJournalSnapshotObject(
	ctx context.Context,
	key string,
) (io.ReadCloser, error) {
	streaming, ok := a.storage.(infrastorage.StreamingStorage)
	if !ok {
		return nil, ErrJournalSnapshotUnavailable
	}
	return streaming.OpenObjectStream(ctx, key)
}

func (a *journalSnapshotStorageAdapter) DeleteJournalSnapshotObject(
	ctx context.Context,
	key string,
) error {
	return a.storage.DeleteObject(ctx, key)
}

func (a *journalSnapshotStorageAdapter) ListJournalSnapshotObjects(
	ctx context.Context,
	prefix string,
	cursor string,
	limit int,
) ([]JournalSnapshotObjectInfo, string, error) {
	page, err := a.storage.ListObjectsPaginated(ctx, &infrastorage.ListObjectsPaginatedInput{
		Prefix: prefix, Cursor: cursor, PageSize: limit,
	})
	if err != nil {
		return nil, "", err
	}
	objects := make([]JournalSnapshotObjectInfo, 0, len(page.Files))
	for _, file := range page.Files {
		if file == nil {
			continue
		}
		objects = append(objects, JournalSnapshotObjectInfo{
			Key: file.Key, LastModified: file.LastModified.UnixMilli(),
		})
	}
	return objects, page.Cursor, nil
}

type GetJournalSnapshotRequest struct {
	SpaceID    int64
	ThreadID   int64
	RunID      int64
	ViewerID   int64
	SnapshotID string
	Cursor     string
	Limit      int
	TraceID    string
}

type JournalSnapshotFragmentView struct {
	FragmentID    string
	FragmentIndex int32
	Kind          JournalSnapshotFragmentKind
	Content       string
	BinaryContent []byte
	MIMEType      string
	BlockID       string
	Stream        string
	StartLine     int32
	EndLine       int32
	ItemStart     int32
	ItemEnd       int32
	Chapters      []JournalDocumentChapter
	Highlights    []JournalCodeHighlight
	Skills        []JournalSkillSummary
	Analysis      []string
	ByteStart     int64
	ByteEnd       int64
	SizeBytes     int64
	ContentHash   string
}

type journalSnapshotFragmentMetadata struct {
	BlockID   string `json:"block_id,omitempty"`
	Stream    string `json:"stream,omitempty"`
	StartLine int32  `json:"start_line,omitempty"`
	EndLine   int32  `json:"end_line,omitempty"`
	ItemStart int32  `json:"item_start,omitempty"`
	ItemEnd   int32  `json:"item_end,omitempty"`
}

type journalSnapshotSemanticFragment struct {
	Kind        JournalSnapshotFragmentKind
	Metadata    journalSnapshotFragmentMetadata
	MIMEType    string
	Content     []byte
	ByteStart   int64
	ByteEnd     int64
	ForceObject bool
}

type JournalSnapshotEnvelope struct {
	ContentType  entity.JournalSnapshotContentType
	SnapshotID   string
	EventID      int64
	AttemptID    string
	IsFragmented bool
	Status       entity.JournalContentStatus
	CreatedAt    int64
	Visibility   entity.JournalVisibility
	ErrorCode    string
	Fragments    []JournalSnapshotFragmentView
	HasMore      bool
	NextCursor   string
	Content      JournalTypedSnapshotContent
	CacheControl string
}

type journalSnapshotEventPayload struct {
	Type string                            `json:"type"`
	Data journalSnapshotActionEventPayload `json:"data"`
}

type journalSnapshotActionEventPayload struct {
	ActionID             string `json:"action_id"`
	MilestoneID          string `json:"milestone_id,omitempty"`
	Operation            string `json:"operation"`
	Target               string `json:"target"`
	DisplayVerbRunning   string `json:"display_verb_running"`
	DisplayVerbCompleted string `json:"display_verb_completed"`
	ContentType          string `json:"content_type"`
	Revision             uint32 `json:"revision"`
}

type AuditJournalSnapshotActionRequest struct {
	SpaceID        int64
	ThreadID       int64
	RunID          int64
	ViewerID       int64
	SnapshotID     string
	Action         entity.JournalSnapshotAction
	FragmentID     string
	IdempotencyKey string
	TraceID        string
}

type JournalSnapshotActionGrant struct {
	SnapshotID       string
	Action           entity.JournalSnapshotAction
	Allowed          bool
	AuditedAt        int64
	CopyText         string
	DownloadURL      string
	DownloadContent  []byte
	DownloadMIMEType string
}

func (s *ApplicationService) ProduceJournalContent(
	ctx context.Context,
	req JournalRuntimeContentSubmission,
) (*entity.JournalContentSnapshot, *entity.JournalEvent, error) {
	if s == nil || s.JournalSnapshotAttemptReader == nil || req.Run == nil {
		return nil, nil, ErrJournalSnapshotUnavailable
	}
	run := req.Run
	if run.SpaceID <= 0 || run.ThreadID <= 0 || run.RunID <= 0 {
		return nil, nil, fmt.Errorf("journal content producer run scope is invalid")
	}
	attempt, err := s.JournalSnapshotAttemptReader.GetActiveJournalAttempt(ctx, run.RunID)
	if errors.Is(err, domainrepo.ErrJournalNotEnrolled) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	if !journalAttemptMatchesRuntimeRun(attempt, run) {
		return nil, nil, ErrJournalSnapshotUnavailable
	}
	if !attempt.SnapshotsEnabled ||
		attempt.ProjectionState != entity.JournalProjectionStateHealthy {
		return nil, nil, nil
	}

	revision := req.Revision
	if revision == 0 {
		revision = 1
	}
	actionID := publicIdentifier(req.Action.ActionID, 191)
	if actionID == "" {
		identity := fmt.Sprintf(
			"%d:%s:%s:%s:%s",
			run.RunID,
			req.Source.ResourceType,
			req.Source.ResourceID,
			req.Source.Revision,
			req.Action.Operation,
		)
		actionID = "content-" + journalSnapshotHash([]byte(identity))[:32]
	}
	req.Action.ActionID = actionID
	traceID := ""
	if attempt.TraceID != nil {
		traceID = *attempt.TraceID
	}
	idempotencyIdentity := fmt.Sprintf(
		"%d:%s:%s:%d:%s:%s",
		run.RunID,
		attempt.AttemptID,
		actionID,
		revision,
		req.Status,
		req.ContentType,
	)

	return s.SubmitJournalContent(ctx, JournalContentSubmission{
		SpaceID: run.SpaceID, ThreadID: run.ThreadID, RunID: run.RunID,
		AttemptID: attempt.AttemptID,
		Revision:  revision,
		IdempotencyKey: "journal-content:" +
			journalSnapshotHash([]byte(idempotencyIdentity))[:40],
		TraceID: traceID, Status: req.Status, ContentType: req.ContentType,
		ErrorCode: req.ErrorCode, Source: req.Source, Action: req.Action,
		Content: req.Content,
	})
}

func journalAttemptMatchesRuntimeRun(
	attempt *entity.RunAttempt,
	run *RunSummary,
) bool {
	if attempt == nil || run == nil || attempt.ThreadID != run.ThreadID ||
		attempt.AttemptID == "" || !attempt.Status.IsActive() {
		return false
	}
	if run.ParentRunID <= 0 {
		return attempt.JournalRunID == run.RunID &&
			attempt.ExecutionRunID == run.RunID
	}
	if run.PlanScopeRunID > 0 && attempt.JournalRunID != run.PlanScopeRunID {
		return false
	}
	return attempt.JournalRunID > 0 && attempt.ExecutionRunID > 0
}

func (s *ApplicationService) SubmitJournalContent(
	ctx context.Context,
	req JournalContentSubmission,
) (*entity.JournalContentSnapshot, *entity.JournalEvent, error) {
	if s == nil || s.JournalSnapshotRepository == nil ||
		s.JournalSnapshotIDGenerator == nil {
		return nil, nil, ErrJournalSnapshotUnavailable
	}
	if req.SpaceID <= 0 || req.ThreadID <= 0 || req.RunID <= 0 ||
		!validJournalSnapshotIdentifier(req.AttemptID, 64, true) ||
		!validJournalSnapshotIdentifier(req.IdempotencyKey, 191, false) || !req.Status.Valid() {
		return nil, nil, fmt.Errorf("journal content submission identity is invalid")
	}
	if strings.TrimSpace(req.SnapshotID) != "" {
		return nil, nil, fmt.Errorf("journal snapshot id is server generated")
	}
	action, err := prepareJournalContentAction(req.Action)
	if err != nil {
		return nil, nil, err
	}
	revision := req.Revision
	if revision == 0 {
		revision = 1
	}

	eventID, err := s.JournalSnapshotIDGenerator.GenID(ctx)
	if err != nil {
		return nil, nil, err
	}
	snapshotID := journalSnapshotServerID(req, action.ActionID, revision)

	contentType, content, mimeType, originalObjectKey, err := prepareJournalTypedSnapshotContent(
		req.ContentType,
		req.Status,
		req.Content,
	)
	if err != nil {
		return nil, nil, err
	}
	if content.Browser != nil {
		if err := s.verifyJournalBrowserRedaction(ctx, content.Browser); err != nil {
			return nil, nil, err
		}
	}
	contentBytes, err := json.Marshal(content)
	if err != nil {
		return nil, nil, err
	}
	if len(contentBytes) > journalSnapshotMaxContentBytes {
		return nil, nil, fmt.Errorf("journal snapshot exceeds the maximum content length")
	}
	now := s.journalSnapshotNow()
	aclDomain := strings.TrimSpace(req.ACLDomain)
	if aclDomain == "" {
		aclDomain = fmt.Sprintf("space:%d/thread:%d", req.SpaceID, req.ThreadID)
	}
	if !validJournalSnapshotKey(aclDomain, 191, false) {
		return nil, nil, fmt.Errorf("journal snapshot ACL domain is invalid")
	}
	errorCode := publicIdentifier(req.ErrorCode, 64)
	switch req.Status {
	case entity.JournalContentStatusError:
		if errorCode == "" {
			errorCode = JournalErrorCodeSnapshotUnavailable
		}
	case entity.JournalContentStatusNoPermission:
		errorCode = JournalErrorCodeNoPermission
	default:
		errorCode = ""
	}
	contentHash := journalSnapshotHash(contentBytes)
	snapshot := &entity.JournalContentSnapshot{
		SnapshotID: snapshotID, SpaceID: req.SpaceID, ThreadID: req.ThreadID,
		RunID: req.RunID, AttemptID: strings.TrimSpace(req.AttemptID), EventID: eventID,
		ActionID: action.ActionID,
		Revision: revision, ContentType: contentType, Status: req.Status,
		Visibility: entity.JournalVisibilityUser, ErrorCode: errorCode,
		MIMEType: mimeType, Encoding: "utf-8",
		Compression:   entity.JournalSnapshotCompressionIdentity,
		ContentLength: int64(len(contentBytes)),
		ContentHash:   contentHash, ACLDomain: aclDomain,
		SourceResourceType: publicIdentifier(req.Source.ResourceType, 64),
		SourceResourceID:   publicIdentifier(req.Source.ResourceID, 191),
		SourceRevision:     strings.ToLower(publicIdentifier(req.Source.Revision, 128)),
		OriginalObjectKey:  originalObjectKey,
		CleanupState:       entity.JournalSnapshotCleanupStateActive, CreatedAt: now,
	}
	snapshot.ExpiresAt = now + entity.JournalSnapshotRetentionMillis
	if err := validateJournalSnapshotSourceIdentity(snapshot); err != nil {
		return nil, nil, err
	}
	if snapshot.OriginalObjectKey != "" &&
		!journalSnapshotOriginalObjectKeyAllowed(snapshot, snapshot.OriginalObjectKey) {
		return nil, nil, ErrJournalSnapshotUnsafeContent
	}
	reservationToken := uuid.NewString()
	reservation, err := s.JournalSnapshotRepository.ReserveJournalSnapshot(
		ctx,
		domainrepo.ReserveJournalSnapshotRequest{
			Reservation: &entity.JournalSnapshotReservation{
				SnapshotID: snapshot.SnapshotID, ReservationToken: reservationToken,
				SpaceID: snapshot.SpaceID, ThreadID: snapshot.ThreadID, RunID: snapshot.RunID,
				AttemptID: snapshot.AttemptID, ActionID: snapshot.ActionID,
				Revision: snapshot.Revision, EventID: snapshot.EventID,
				IdempotencyKey: strings.TrimSpace(req.IdempotencyKey),
				ContentHash:    snapshot.ContentHash, ACLDomain: snapshot.ACLDomain,
				StagingPrefix: journalSnapshotStagingPrefix(snapshot, reservationToken),
				ExpiresAt:     now + journalSnapshotReservationTTL.Milliseconds(), CreatedAt: now,
			},
			Now: now,
		},
	)
	if err != nil {
		return nil, nil, err
	}
	if reservation.Replayed {
		if reservation.Snapshot == nil || reservation.Event == nil {
			return nil, nil, ErrJournalSnapshotUnavailable
		}
		return reservation.Snapshot, reservation.Event, nil
	}
	if reservation.Reservation == nil {
		return nil, nil, ErrJournalSnapshotUnavailable
	}
	snapshot.EventID = reservation.Reservation.EventID
	eventID = reservation.Reservation.EventID
	reservationToken = reservation.Reservation.ReservationToken

	fragments, err := s.stageJournalSnapshotContent(
		ctx,
		snapshot,
		content,
		contentBytes,
		reservation.Reservation.StagingPrefix,
	)
	if err != nil {
		return nil, nil, err
	}
	snapshot.FragmentCount = uint32(len(fragments))
	eventType, phase := journalSnapshotActionEventIdentity(req.Status)
	eventStatus := journalSnapshotEventStatus(req.Status)
	payload, err := json.Marshal(journalSnapshotEventPayload{
		Type: string(contentType),
		Data: journalSnapshotActionEventPayload{
			ActionID: action.ActionID, MilestoneID: action.MilestoneID,
			Operation: action.Operation, Target: action.Target,
			DisplayVerbRunning:   action.DisplayVerbRunning,
			DisplayVerbCompleted: action.DisplayVerbCompleted,
			ContentType:          string(contentType), Revision: snapshot.Revision,
		},
	})
	if err != nil {
		return nil, nil, err
	}
	event := &entity.JournalEvent{
		ID: eventID, ThreadID: req.ThreadID, RunID: req.RunID,
		AttemptID: snapshot.AttemptID, IdempotencyKey: strings.TrimSpace(req.IdempotencyKey),
		ParentEventID: action.ParentEventID,
		SchemaVersion: entity.JournalSchemaVersion, Status: eventStatus,
		OccurredAtUnixNano: now * int64(time.Millisecond),
		Visibility:         entity.JournalVisibilityUser,
		PayloadVersion:     entity.JournalPayloadVersion,
		SnapshotID:         snapshotID, TraceID: publicIdentifier(req.TraceID, 128),
		ActionID: action.ActionID, Phase: phase, Operation: action.Operation,
		Target: action.Target, Milestone: action.MilestoneID,
		EventType: eventType, Payload: string(payload), CreatedAt: now,
	}
	created, appended, _, err := s.JournalSnapshotRepository.CreateJournalSnapshot(
		ctx,
		domainrepo.CreateJournalSnapshotRequest{
			Snapshot: snapshot, Fragments: fragments, Event: event,
			ReservationToken: reservationToken,
		},
	)
	if err != nil {
		return nil, nil, err
	}
	return created, appended, nil
}

func (s *ApplicationService) stageJournalSnapshotContent(
	ctx context.Context,
	snapshot *entity.JournalContentSnapshot,
	typed JournalTypedSnapshotContent,
	content []byte,
	stagingPrefix string,
) ([]*entity.JournalSnapshotFragment, error) {
	if len(content) <= journalSnapshotInlineContentBytes {
		snapshot.ContentJSON = string(content)
		return nil, nil
	}
	if s.JournalSnapshotObjectStorage == nil {
		return nil, ErrJournalSnapshotUnavailable
	}
	snapshot.ObjectKey = fmt.Sprintf(
		"%smanifest-%s",
		stagingPrefix,
		snapshot.ContentHash[:16],
	)
	if err := s.JournalSnapshotObjectStorage.PutJournalSnapshotObject(
		ctx,
		snapshot.ObjectKey,
		content,
	); err != nil {
		return nil, err
	}
	semantic, err := buildJournalSnapshotSemanticFragments(typed)
	if err != nil {
		return nil, err
	}
	summary, err := json.Marshal(journalSnapshotContentSummary(typed))
	if err != nil || len(summary) == 0 || len(summary) > journalSnapshotMaxSummaryBytes {
		return nil, ErrJournalSnapshotUnsafeContent
	}
	snapshot.SummaryJSON = string(summary)
	snapshot.SummaryHash = journalSnapshotHash(summary)
	fragments := make([]*entity.JournalSnapshotFragment, 0, len(semantic))
	for index, item := range semantic {
		if len(item.Content) == 0 || len(item.Content) > journalSnapshotFragmentBytes ||
			!item.Kind.Valid() {
			return nil, ErrJournalSnapshotUnsafeContent
		}
		metadata, err := json.Marshal(item.Metadata)
		if err != nil {
			return nil, err
		}
		if string(metadata) == "{}" {
			metadata = nil
		}
		fragmentHash := journalSnapshotHash(item.Content)
		fragment := &entity.JournalSnapshotFragment{
			FragmentID: journalSnapshotFragmentID(snapshot.SnapshotID, index),
			SnapshotID: snapshot.SnapshotID, FragmentIndex: int32(index),
			Kind: item.Kind, MetadataJSON: string(metadata), MIMEType: item.MIMEType,
			ByteStart: item.ByteStart, ByteEnd: item.ByteEnd,
			SizeBytes: int64(len(item.Content)), ContentHash: fragmentHash,
			CreatedAt: snapshot.CreatedAt,
		}
		if !item.ForceObject && len(item.Content) <= journalSnapshotInlineFragmentBytes {
			fragment.InlineContent = string(item.Content)
		} else {
			fragment.ObjectKey = journalSnapshotStagingKey(stagingPrefix, index, fragmentHash)
			if err := s.JournalSnapshotObjectStorage.PutJournalSnapshotObject(
				ctx,
				fragment.ObjectKey,
				item.Content,
			); err != nil {
				return nil, err
			}
		}
		fragments = append(fragments, fragment)
	}
	snapshot.IsFragmented = len(fragments) > 0
	return fragments, nil
}

type journalSnapshotTextChunk struct {
	Content   string
	StartLine int32
	EndLine   int32
	ByteStart int64
	ByteEnd   int64
}

type journalSnapshotItemChunk struct {
	Content   []byte
	ItemStart int32
	ItemEnd   int32
}

func buildJournalSnapshotSemanticFragments(
	typed JournalTypedSnapshotContent,
) ([]journalSnapshotSemanticFragment, error) {
	fragments := make([]journalSnapshotSemanticFragment, 0)
	switch {
	case typed.Document != nil:
		for index, chunk := range splitJournalSnapshotText(
			typed.Document.Content,
			"\n\n",
			journalSnapshotFragmentBytes,
			1,
		) {
			fragments = append(fragments, journalSnapshotSemanticFragment{
				Kind: JournalSnapshotFragmentKindDocumentBlock,
				Metadata: journalSnapshotFragmentMetadata{
					BlockID:   fmt.Sprintf("block-%04d", index),
					StartLine: chunk.StartLine, EndLine: chunk.EndLine,
				},
				Content: []byte(chunk.Content), ByteStart: chunk.ByteStart, ByteEnd: chunk.ByteEnd,
			})
		}
		chunks, err := chunkJournalSnapshotItems(typed.Document.Chapters)
		if err != nil {
			return nil, err
		}
		for _, chunk := range chunks {
			fragments = append(fragments, journalSnapshotSemanticFragment{
				Kind: JournalSnapshotFragmentKindDocumentChapters,
				Metadata: journalSnapshotFragmentMetadata{
					ItemStart: chunk.ItemStart, ItemEnd: chunk.ItemEnd,
				},
				Content: chunk.Content,
				ByteEnd: int64(len(chunk.Content)),
			})
		}
	case typed.Terminal != nil:
		fragments = appendJournalSnapshotTextFragments(
			fragments,
			JournalSnapshotFragmentKindTerminalStdout,
			"stdout",
			typed.Terminal.Stdout,
			1,
		)
		fragments = appendJournalSnapshotTextFragments(
			fragments,
			JournalSnapshotFragmentKindTerminalStderr,
			"stderr",
			typed.Terminal.Stderr,
			1,
		)
	case typed.Code != nil:
		startLine := typed.Code.StartLine
		if startLine < 1 {
			startLine = 1
		}
		fragments = appendJournalSnapshotTextFragments(
			fragments,
			JournalSnapshotFragmentKindCodeLines,
			"",
			typed.Code.Content,
			startLine,
		)
		chunks, err := chunkJournalSnapshotItems(typed.Code.Highlights)
		if err != nil {
			return nil, err
		}
		for _, chunk := range chunks {
			fragments = append(fragments, journalSnapshotSemanticFragment{
				Kind: JournalSnapshotFragmentKindCodeHighlights,
				Metadata: journalSnapshotFragmentMetadata{
					ItemStart: chunk.ItemStart, ItemEnd: chunk.ItemEnd,
				},
				Content: chunk.Content,
				ByteEnd: int64(len(chunk.Content)),
			})
		}
	case typed.Skill != nil:
		chunks, err := chunkJournalSnapshotItems(typed.Skill.Skills)
		if err != nil {
			return nil, err
		}
		for _, chunk := range chunks {
			fragments = append(fragments, journalSnapshotSemanticFragment{
				Kind: JournalSnapshotFragmentKindSkillItems,
				Metadata: journalSnapshotFragmentMetadata{
					ItemStart: chunk.ItemStart, ItemEnd: chunk.ItemEnd,
				},
				Content: chunk.Content,
				ByteEnd: int64(len(chunk.Content)),
			})
		}
	case typed.Browser != nil:
		fragments = appendJournalSnapshotBinaryFragments(
			fragments,
			JournalSnapshotFragmentKindBrowserSnapshot,
			typed.Browser.MIMEType,
			typed.Browser.StaticSnapshot,
		)
		fragments = appendJournalSnapshotBinaryFragments(
			fragments,
			JournalSnapshotFragmentKindBrowserThumbnail,
			typed.Browser.MIMEType,
			typed.Browser.Thumbnail,
		)
		chunks, err := chunkJournalSnapshotItems(typed.Browser.Analysis)
		if err != nil {
			return nil, err
		}
		for _, chunk := range chunks {
			fragments = append(fragments, journalSnapshotSemanticFragment{
				Kind: JournalSnapshotFragmentKindBrowserAnalysis,
				Metadata: journalSnapshotFragmentMetadata{
					ItemStart: chunk.ItemStart, ItemEnd: chunk.ItemEnd,
				},
				Content: chunk.Content,
				ByteEnd: int64(len(chunk.Content)),
			})
		}
	}
	return fragments, nil
}

func appendJournalSnapshotTextFragments(
	fragments []journalSnapshotSemanticFragment,
	kind JournalSnapshotFragmentKind,
	stream string,
	content string,
	startLine int32,
) []journalSnapshotSemanticFragment {
	for _, chunk := range splitJournalSnapshotText(
		content,
		"\n",
		journalSnapshotFragmentBytes,
		startLine,
	) {
		fragments = append(fragments, journalSnapshotSemanticFragment{
			Kind: kind,
			Metadata: journalSnapshotFragmentMetadata{
				Stream: stream, StartLine: chunk.StartLine, EndLine: chunk.EndLine,
			},
			Content:   []byte(chunk.Content),
			ByteStart: chunk.ByteStart, ByteEnd: chunk.ByteEnd,
		})
	}
	return fragments
}

func appendJournalSnapshotBinaryFragments(
	fragments []journalSnapshotSemanticFragment,
	kind JournalSnapshotFragmentKind,
	mimeType string,
	content []byte,
) []journalSnapshotSemanticFragment {
	for start := 0; start < len(content); start += journalSnapshotFragmentBytes {
		end := min(start+journalSnapshotFragmentBytes, len(content))
		fragments = append(fragments, journalSnapshotSemanticFragment{
			Kind: kind,
			Metadata: journalSnapshotFragmentMetadata{
				ItemStart: int32(start), ItemEnd: int32(end),
			},
			MIMEType: mimeType, Content: append([]byte(nil), content[start:end]...),
			ByteStart: int64(start), ByteEnd: int64(end),
			ForceObject: true,
		})
	}
	return fragments
}

func splitJournalSnapshotText(
	content string,
	delimiter string,
	maxBytes int,
	startLine int32,
) []journalSnapshotTextChunk {
	chunks := make([]journalSnapshotTextChunk, 0)
	var current strings.Builder
	line := startLine
	chunkStart := line
	byteOffset := int64(0)
	flush := func() {
		if current.Len() == 0 {
			return
		}
		value := current.String()
		endLine := line + int32(strings.Count(value, "\n"))
		if strings.HasSuffix(value, "\n") && endLine > chunkStart {
			endLine--
		}
		chunks = append(chunks, journalSnapshotTextChunk{
			Content: value, StartLine: chunkStart, EndLine: max(chunkStart, endLine),
			ByteStart: byteOffset, ByteEnd: byteOffset + int64(len(value)),
		})
		byteOffset += int64(len(value))
		line += int32(strings.Count(value, "\n"))
		chunkStart = line
		current.Reset()
	}
	for offset := 0; offset < len(content); {
		unitEnd := len(content)
		if delimiter != "" {
			if relativeEnd := strings.Index(content[offset:], delimiter); relativeEnd >= 0 {
				unitEnd = offset + relativeEnd + len(delimiter)
			}
		}
		unit := content[offset:unitEnd]
		offset = unitEnd
		if unit == "" {
			continue
		}
		if current.Len() > 0 && current.Len()+len(unit) > maxBytes {
			flush()
		}
		if len(unit) <= maxBytes {
			current.WriteString(unit)
			continue
		}
		for _, chunk := range splitJournalSnapshotUTF8([]byte(unit), maxBytes) {
			if current.Len() > 0 {
				flush()
			}
			current.Write(chunk)
			flush()
		}
	}
	if content == "" {
		return chunks
	}
	flush()
	return chunks
}

func chunkJournalSnapshotItems[T any](items []T) ([]journalSnapshotItemChunk, error) {
	chunks := make([]journalSnapshotItemChunk, 0)
	if len(items) == 0 {
		return chunks, nil
	}

	start := 0
	var current bytes.Buffer
	current.Grow(journalSnapshotFragmentBytes)
	current.WriteByte('[')
	flush := func(end int) {
		current.WriteByte(']')
		chunks = append(chunks, journalSnapshotItemChunk{
			Content:   append([]byte(nil), current.Bytes()...),
			ItemStart: int32(start), ItemEnd: int32(end),
		})
		start = end
		current.Reset()
		current.Grow(journalSnapshotFragmentBytes)
		current.WriteByte('[')
	}

	for index, item := range items {
		encoded, err := json.Marshal(item)
		if err != nil {
			return nil, err
		}
		separatorBytes := 0
		if index > start {
			separatorBytes = 1
		}
		if current.Len()+separatorBytes+len(encoded)+1 > journalSnapshotFragmentBytes {
			if index == start {
				return nil, ErrJournalSnapshotUnsafeContent
			}
			flush(index)
			separatorBytes = 0
		}
		if current.Len()+separatorBytes+len(encoded)+1 > journalSnapshotFragmentBytes {
			return nil, ErrJournalSnapshotUnsafeContent
		}
		if separatorBytes > 0 {
			current.WriteByte(',')
		}
		current.Write(encoded)
	}
	flush(len(items))
	return chunks, nil
}

func (s *ApplicationService) GetJournalSnapshot(
	ctx context.Context,
	req GetJournalSnapshotRequest,
) (*JournalSnapshotEnvelope, error) {
	if s == nil || s.JournalSnapshotRepository == nil || req.SpaceID <= 0 ||
		req.ThreadID <= 0 || req.RunID <= 0 || req.ViewerID <= 0 ||
		!journalSnapshotIDPattern.MatchString(strings.TrimSpace(req.SnapshotID)) {
		return nil, domainrepo.ErrJournalSnapshotNotFound
	}
	snapshot, err := s.JournalSnapshotRepository.GetJournalSnapshot(
		ctx,
		domainrepo.GetJournalSnapshotRequest{
			SpaceID: req.SpaceID, ThreadID: req.ThreadID,
			RunID: req.RunID, SnapshotID: req.SnapshotID,
		},
	)
	if err != nil {
		if errors.Is(err, domainrepo.ErrJournalSnapshotNotFound) {
			if auditErr := s.recordJournalSnapshotAudit(ctx, nil, req.SpaceID, req.ThreadID,
				req.RunID, req.ViewerID, req.SnapshotID,
				entity.JournalSnapshotActionReadContent,
				journalSnapshotReadIdempotency(req.TraceID, req.ViewerID, s.journalSnapshotNow()),
				journalSnapshotTargetHash(req.SnapshotID, entity.JournalSnapshotActionReadContent, ""),
				req.TraceID, entity.JournalSnapshotPermissionDenied); auditErr != nil {
				return nil, auditErr
			}
		}
		return nil, err
	}

	permission := entity.JournalSnapshotPermissionAllowed
	authorizationUnavailable := false
	if err := s.authorizeJournalSnapshot(ctx, snapshot, req.ViewerID); err != nil {
		permission = entity.JournalSnapshotPermissionDenied
		authorizationUnavailable = !errors.Is(err, ErrJournalSnapshotNoPermission)
	}
	if permission == entity.JournalSnapshotPermissionAllowed &&
		snapshot.ExpiresAt > 0 && snapshot.ExpiresAt <= s.journalSnapshotNow() {
		permission = entity.JournalSnapshotPermissionExpired
	}
	if err := s.recordJournalSnapshotAudit(
		ctx,
		snapshot,
		req.SpaceID,
		req.ThreadID,
		req.RunID,
		req.ViewerID,
		req.SnapshotID,
		entity.JournalSnapshotActionReadContent,
		journalSnapshotReadIdempotency(req.TraceID, req.ViewerID, s.journalSnapshotNow()),
		journalSnapshotTargetHash(snapshot.SnapshotID, entity.JournalSnapshotActionReadContent, ""),
		req.TraceID,
		permission,
	); err != nil {
		return nil, err
	}
	if authorizationUnavailable {
		return nil, ErrJournalSnapshotUnavailable
	}
	if permission == entity.JournalSnapshotPermissionDenied {
		return journalSnapshotRestrictedEnvelope(
			snapshot,
			entity.JournalContentStatusNoPermission,
			JournalErrorCodeNoPermission,
		), nil
	}
	if permission == entity.JournalSnapshotPermissionExpired {
		return journalSnapshotRestrictedEnvelope(
			snapshot,
			entity.JournalContentStatusError,
			JournalErrorCodeSnapshotUnavailable,
		), nil
	}
	return s.loadJournalSnapshotEnvelope(ctx, snapshot, req.Cursor, req.Limit)
}

func (s *ApplicationService) AuditJournalSnapshotAction(
	ctx context.Context,
	req AuditJournalSnapshotActionRequest,
) (*JournalSnapshotActionGrant, error) {
	if s == nil || s.JournalSnapshotRepository == nil || req.ViewerID <= 0 ||
		!validJournalSnapshotIdentifier(req.IdempotencyKey, 191, false) ||
		!req.Action.UserAction() {
		return nil, ErrJournalSnapshotActionUnavailable
	}
	if (req.Action == entity.JournalSnapshotActionDownloadFragment &&
		strings.TrimSpace(req.FragmentID) == "") ||
		(req.Action != entity.JournalSnapshotActionDownloadFragment &&
			strings.TrimSpace(req.FragmentID) != "") {
		return nil, ErrJournalSnapshotActionUnavailable
	}
	snapshot, err := s.JournalSnapshotRepository.GetJournalSnapshot(
		ctx,
		domainrepo.GetJournalSnapshotRequest{
			SpaceID: req.SpaceID, ThreadID: req.ThreadID,
			RunID: req.RunID, SnapshotID: req.SnapshotID,
		},
	)
	if err != nil {
		if errors.Is(err, domainrepo.ErrJournalSnapshotNotFound) {
			targetHash := journalSnapshotTargetHash(
				req.SnapshotID,
				req.Action,
				req.FragmentID,
			)
			if auditErr := s.recordJournalSnapshotAudit(
				ctx,
				nil,
				req.SpaceID,
				req.ThreadID,
				req.RunID,
				req.ViewerID,
				req.SnapshotID,
				req.Action,
				req.IdempotencyKey,
				targetHash,
				req.TraceID,
				entity.JournalSnapshotPermissionDenied,
			); auditErr != nil {
				return nil, auditErr
			}
		}
		return nil, err
	}
	permission := entity.JournalSnapshotPermissionAllowed
	authorizationUnavailable := false
	if err := s.authorizeJournalSnapshot(ctx, snapshot, req.ViewerID); err != nil {
		permission = entity.JournalSnapshotPermissionDenied
		authorizationUnavailable = !errors.Is(err, ErrJournalSnapshotNoPermission)
	}
	if permission == entity.JournalSnapshotPermissionAllowed &&
		snapshot.ExpiresAt > 0 && snapshot.ExpiresAt <= s.journalSnapshotNow() {
		permission = entity.JournalSnapshotPermissionExpired
	}
	var capabilityErr error
	if permission == entity.JournalSnapshotPermissionAllowed {
		capabilityErr = s.validateJournalSnapshotActionCapability(snapshot, req)
		if capabilityErr != nil {
			permission = entity.JournalSnapshotPermissionDenied
		}
	}
	targetHash := journalSnapshotActionTargetHash(snapshot, req.Action, req.FragmentID)
	audit, err := s.recordJournalSnapshotAuditWithResult(
		ctx,
		snapshot,
		req.ViewerID,
		req.Action,
		req.IdempotencyKey,
		targetHash,
		req.TraceID,
		permission,
	)
	if err != nil {
		return nil, err
	}
	if authorizationUnavailable {
		return nil, ErrJournalSnapshotUnavailable
	}
	if capabilityErr != nil {
		return nil, capabilityErr
	}
	grant := &JournalSnapshotActionGrant{
		SnapshotID: snapshot.SnapshotID, Action: req.Action,
		Allowed:   audit.PermissionResult == entity.JournalSnapshotPermissionAllowed,
		AuditedAt: audit.CreatedAt,
	}
	if !grant.Allowed {
		return grant, nil
	}
	if err := s.populateJournalSnapshotActionGrant(ctx, snapshot, req, grant); err != nil {
		return nil, err
	}
	return grant, nil
}

func (s *ApplicationService) validateJournalSnapshotActionCapability(
	snapshot *entity.JournalContentSnapshot,
	req AuditJournalSnapshotActionRequest,
) error {
	if snapshot == nil {
		return ErrJournalSnapshotActionUnavailable
	}
	switch req.Action {
	case entity.JournalSnapshotActionCopyCommand,
		entity.JournalSnapshotActionCopyOutput:
		if snapshot.ContentType != entity.JournalSnapshotContentTypeTerminal {
			return ErrJournalSnapshotActionUnavailable
		}
	case entity.JournalSnapshotActionCopyCode:
		if snapshot.ContentType != entity.JournalSnapshotContentTypeCode {
			return ErrJournalSnapshotActionUnavailable
		}
	case entity.JournalSnapshotActionOpenOriginal:
		if snapshot.ContentType != entity.JournalSnapshotContentTypeDocument ||
			snapshot.SourceResourceType != "artifact" ||
			s.JournalSnapshotArtifactCapabilityIssuer == nil ||
			!validJournalObjectReference(snapshot.OriginalObjectKey) {
			return ErrJournalSnapshotActionUnavailable
		}
		artifactID, err := strconv.ParseInt(snapshot.SourceResourceID, 10, 64)
		if err != nil || artifactID <= 0 {
			return ErrJournalSnapshotActionUnavailable
		}
	case entity.JournalSnapshotActionDownloadFragment:
		if !snapshot.IsFragmented || snapshot.FragmentCount == 0 ||
			!validJournalSnapshotIdentifier(req.FragmentID, 191, false) {
			return ErrJournalSnapshotActionUnavailable
		}
	default:
		return ErrJournalSnapshotActionUnavailable
	}
	return nil
}

func (s *ApplicationService) ReapOrphanedJournalSnapshotObjects(
	ctx context.Context,
	spaceID int64,
	olderThan int64,
) (int, error) {
	if s == nil || s.JournalSnapshotRepository == nil ||
		s.JournalSnapshotObjectStorage == nil || spaceID <= 0 || olderThan <= 0 {
		return 0, ErrJournalSnapshotUnavailable
	}
	if _, err := s.JournalSnapshotRepository.DeleteExpiredJournalSnapshotReservations(
		ctx,
		spaceID,
		s.journalSnapshotNow(),
		journalSnapshotMaxFragmentLimit,
	); err != nil {
		return 0, err
	}
	prefix := fmt.Sprintf("%s/%d/", journalSnapshotObjectPrefix, spaceID)
	cursor := ""
	deleted := 0
	for {
		objects, next, err := s.JournalSnapshotObjectStorage.ListJournalSnapshotObjects(
			ctx,
			prefix,
			cursor,
			200,
		)
		if err != nil {
			return deleted, err
		}
		for _, object := range objects {
			if object.LastModified <= 0 || object.LastModified > olderThan {
				continue
			}
			committed, err := s.JournalSnapshotRepository.IsJournalSnapshotObjectProtected(
				ctx,
				spaceID,
				object.Key,
				s.journalSnapshotNow(),
			)
			if err != nil {
				return deleted, err
			}
			if committed {
				continue
			}
			if err := s.JournalSnapshotObjectStorage.DeleteJournalSnapshotObject(
				ctx,
				object.Key,
			); err != nil {
				return deleted, err
			}
			deleted++
		}
		if next == "" || next == cursor {
			return deleted, nil
		}
		cursor = next
	}
}

func prepareJournalTypedSnapshotContent(
	explicitType entity.JournalSnapshotContentType,
	status entity.JournalContentStatus,
	content JournalTypedSnapshotContent,
) (entity.JournalSnapshotContentType, JournalTypedSnapshotContent, string, string, error) {
	count := 0
	if content.Document != nil {
		count++
	}
	if content.Terminal != nil {
		count++
	}
	if content.Code != nil {
		count++
	}
	if content.Skill != nil {
		count++
	}
	if content.Browser != nil {
		count++
	}
	if count > 1 || (count == 0 && !explicitType.Valid()) {
		return "", JournalTypedSnapshotContent{}, "", "", fmt.Errorf(
			"journal snapshot must contain exactly one typed view",
		)
	}
	if count > 0 && (status == entity.JournalContentStatusError ||
		status == entity.JournalContentStatusNoPermission ||
		status == entity.JournalContentStatusEmpty) {
		return "", JournalTypedSnapshotContent{}, "", "", fmt.Errorf(
			"non-content journal snapshot states cannot carry content",
		)
	}
	if count == 0 {
		if status == entity.JournalContentStatusReady || status == entity.JournalContentStatusStreaming {
			return "", JournalTypedSnapshotContent{}, "", "", fmt.Errorf(
				"ready and streaming snapshots require typed content",
			)
		}
		return explicitType, JournalTypedSnapshotContent{}, "application/json", "", nil
	}

	switch {
	case content.Document != nil:
		if explicitType.Valid() && explicitType != entity.JournalSnapshotContentTypeDocument {
			return "", JournalTypedSnapshotContent{}, "", "", fmt.Errorf("journal snapshot content type mismatch")
		}
		safe, objectKey, err := sanitizeJournalDocument(content.Document)
		return entity.JournalSnapshotContentTypeDocument,
			JournalTypedSnapshotContent{Document: safe}, "text/markdown", objectKey, err
	case content.Terminal != nil:
		if explicitType.Valid() && explicitType != entity.JournalSnapshotContentTypeTerminal {
			return "", JournalTypedSnapshotContent{}, "", "", fmt.Errorf("journal snapshot content type mismatch")
		}
		safe, err := sanitizeJournalTerminal(content.Terminal)
		return entity.JournalSnapshotContentTypeTerminal,
			JournalTypedSnapshotContent{Terminal: safe}, "text/plain", "", err
	case content.Code != nil:
		if explicitType.Valid() && explicitType != entity.JournalSnapshotContentTypeCode {
			return "", JournalTypedSnapshotContent{}, "", "", fmt.Errorf("journal snapshot content type mismatch")
		}
		safe, err := sanitizeJournalCode(content.Code)
		return entity.JournalSnapshotContentTypeCode,
			JournalTypedSnapshotContent{Code: safe}, "text/plain", "", err
	case content.Skill != nil:
		if explicitType.Valid() && explicitType != entity.JournalSnapshotContentTypeSkill {
			return "", JournalTypedSnapshotContent{}, "", "", fmt.Errorf("journal snapshot content type mismatch")
		}
		safe, err := sanitizeJournalSkill(content.Skill)
		return entity.JournalSnapshotContentTypeSkill,
			JournalTypedSnapshotContent{Skill: safe}, "application/json", "", err
	case content.Browser != nil:
		if explicitType.Valid() && explicitType != entity.JournalSnapshotContentTypeBrowser {
			return "", JournalTypedSnapshotContent{}, "", "", fmt.Errorf("journal snapshot content type mismatch")
		}
		safe, err := sanitizeJournalBrowser(content.Browser)
		mimeType := "application/octet-stream"
		if safe != nil {
			mimeType = safe.MIMEType
		}
		return entity.JournalSnapshotContentTypeBrowser,
			JournalTypedSnapshotContent{Browser: safe}, mimeType, "", err
	default:
		return "", JournalTypedSnapshotContent{}, "", "", fmt.Errorf(
			"journal snapshot typed view is missing",
		)
	}
}

func sanitizeJournalDocument(
	content *JournalDocumentContent,
) (*JournalDocumentContent, string, error) {
	if content == nil {
		return nil, "", ErrJournalSnapshotUnsafeContent
	}
	safe := *content
	safe.Token = publicIdentifier(safe.Token, 128)
	safe.Title = publicLabel(safe.Title, 512)
	if safe.Title == "" || !utf8.ValidString(safe.Content) ||
		journalUnsafeHTMLPattern.MatchString(safe.Content) ||
		journalAnyHTMLPattern.MatchString(safe.Content) ||
		journalUnsafeSchemePattern.MatchString(safe.Content) ||
		strings.ContainsRune(safe.Content, '\x00') {
		return nil, "", ErrJournalSnapshotUnsafeContent
	}
	safe.Content = sanitizeJournalVisibleText(safe.Content, journalSnapshotMaxContentBytes)
	safe.ActiveBlock = publicIdentifier(safe.ActiveBlock, 128)
	safe.Revision = publicIdentifier(safe.Revision, 128)
	switch safe.SyncStatus {
	case "", "syncing", "synced", "stale", "error":
	default:
		safe.SyncStatus = ""
	}
	if safe.Format == "" {
		safe.Format = "markdown"
	}
	if safe.Format != "markdown" && safe.Format != "text" {
		return nil, "", ErrJournalSnapshotUnsafeContent
	}
	if strings.TrimSpace(safe.OriginalURL) != "" {
		return nil, "", ErrJournalSnapshotUnsafeContent
	}
	safe.OriginalURL = ""
	objectKey := strings.TrimSpace(safe.OriginalObjectKey)
	if objectKey != "" && !validJournalObjectReference(objectKey) {
		return nil, "", ErrJournalSnapshotUnsafeContent
	}
	safe.OriginalObjectKey = ""
	for i := range safe.Chapters {
		safe.Chapters[i].ChapterID = publicIdentifier(safe.Chapters[i].ChapterID, 128)
		safe.Chapters[i].Title = publicLabel(safe.Chapters[i].Title, 512)
		if safe.Chapters[i].ChapterID == "" || safe.Chapters[i].Title == "" ||
			safe.Chapters[i].Level < 0 {
			return nil, "", ErrJournalSnapshotUnsafeContent
		}
	}
	return &safe, objectKey, nil
}

func sanitizeJournalTerminal(
	content *JournalTerminalContent,
) (*JournalTerminalContent, error) {
	if content == nil || !utf8.ValidString(content.Command) ||
		!utf8.ValidString(content.Stdout) || !utf8.ValidString(content.Stderr) {
		return nil, ErrJournalSnapshotUnsafeContent
	}
	safe := *content
	safe.SessionID = publicIdentifier(safe.SessionID, 128)
	safe.Command = sanitizeJournalVisibleText(safe.Command, 64*1024)
	if safe.Command == "" {
		return nil, ErrJournalSnapshotUnsafeContent
	}
	if publicStringIsSensitive(safe.WorkingDirectory) {
		safe.WorkingDirectory = "[internal path]"
	} else {
		safe.WorkingDirectory = publicLabel(safe.WorkingDirectory, 1024)
	}
	safe.Stdout = sanitizeJournalVisibleText(safe.Stdout, journalSnapshotMaxContentBytes)
	safe.Stderr = sanitizeJournalVisibleText(safe.Stderr, journalSnapshotMaxContentBytes)
	if safe.FinishedAt > 0 && safe.StartedAt > 0 && safe.FinishedAt >= safe.StartedAt {
		safe.DurationMS = safe.FinishedAt - safe.StartedAt
	}
	return &safe, nil
}

func sanitizeJournalCode(content *JournalCodeContent) (*JournalCodeContent, error) {
	if content == nil || !utf8.ValidString(content.Content) ||
		strings.ContainsRune(content.Content, '\x00') {
		return nil, ErrJournalSnapshotUnsafeContent
	}
	safe := *content
	safe.Repository = publicIdentifier(safe.Repository, 191)
	safe.Revision = strings.TrimSpace(safe.Revision)
	safe.Path = path.Clean(strings.TrimSpace(safe.Path))
	if safe.Repository == "" || !journalRevisionPattern.MatchString(safe.Revision) ||
		safe.Path == "." || strings.HasPrefix(safe.Path, "/") ||
		safe.Path == ".." || strings.HasPrefix(safe.Path, "../") ||
		journalCredentialPathPattern.MatchString(safe.Path) ||
		journalSecretPattern.MatchString(safe.Content) {
		return nil, ErrJournalSnapshotUnsafeContent
	}
	safe.Language = publicIdentifier(safe.Language, 64)
	safe.Content = publicCleanString(safe.Content, journalSnapshotMaxContentBytes)
	if safe.StartLine < 0 || safe.EndLine < 0 ||
		(safe.EndLine > 0 && safe.StartLine > safe.EndLine) {
		return nil, ErrJournalSnapshotUnsafeContent
	}
	for i := range safe.Highlights {
		if safe.Highlights[i].StartLine < 1 ||
			safe.Highlights[i].EndLine < safe.Highlights[i].StartLine {
			return nil, ErrJournalSnapshotUnsafeContent
		}
		safe.Highlights[i].Kind = publicIdentifier(safe.Highlights[i].Kind, 64)
	}
	return &safe, nil
}

func sanitizeJournalSkill(content *JournalSkillContent) (*JournalSkillContent, error) {
	if content == nil {
		return nil, ErrJournalSnapshotUnsafeContent
	}
	safe := &JournalSkillContent{Skills: make([]JournalSkillSummary, 0, len(content.Skills))}
	for _, skill := range content.Skills {
		summary := JournalSkillSummary{
			SkillID:     publicIdentifier(skill.SkillID, 128),
			Name:        publicLabel(skill.Name, 191),
			Description: publicLabel(skill.Description, 512),
		}
		if summary.SkillID == "" || summary.Name == "" {
			return nil, ErrJournalSnapshotUnsafeContent
		}
		safe.Skills = append(safe.Skills, summary)
	}
	return safe, nil
}

func sanitizeJournalBrowser(content *JournalBrowserContent) (*JournalBrowserContent, error) {
	if content == nil || !content.Redacted || publicIdentifier(content.CaptureID, 128) == "" {
		return nil, ErrJournalSnapshotUnsafeContent
	}
	safe := *content
	safe.CaptureID = publicIdentifier(safe.CaptureID, 128)
	safe.RedactionEvidenceID = publicIdentifier(safe.RedactionEvidenceID, 128)
	safe.RedactionPolicyVersion = publicIdentifier(safe.RedactionPolicyVersion, 64)
	if safe.RedactionEvidenceID == "" || safe.RedactionPolicyVersion == "" {
		return nil, ErrJournalSnapshotUnsafeContent
	}
	safe.Title = publicLabel(safe.Title, 512)
	if safe.Resource != "" {
		resource, err := sanitizeJournalHTTPSURL(safe.Resource)
		if err != nil {
			return nil, err
		}
		safe.Resource = resource
	}
	if !validJournalBitmap(safe.MIMEType, safe.StaticSnapshot) ||
		(len(safe.Thumbnail) > 0 && !validJournalBitmap(safe.MIMEType, safe.Thumbnail)) {
		return nil, ErrJournalSnapshotUnsafeContent
	}
	for i := range safe.Analysis {
		safe.Analysis[i] = sanitizeJournalVisibleText(safe.Analysis[i], 4096)
	}
	if safe.Index < 0 || safe.Total < 0 || (safe.Total > 0 && safe.Index > safe.Total) {
		return nil, ErrJournalSnapshotUnsafeContent
	}
	return &safe, nil
}

func (s *ApplicationService) verifyJournalBrowserRedaction(
	ctx context.Context,
	content *JournalBrowserContent,
) error {
	if s == nil || s.JournalBrowserRedactionVerifier == nil || content == nil {
		return ErrJournalSnapshotUnsafeContent
	}
	evidence := JournalBrowserRedactionEvidence{
		CaptureID:              content.CaptureID,
		RedactionEvidenceID:    content.RedactionEvidenceID,
		RedactionPolicyVersion: content.RedactionPolicyVersion,
		MIMEType:               content.MIMEType,
		StaticSnapshotHash:     journalSnapshotHash(content.StaticSnapshot),
	}
	if len(content.Thumbnail) > 0 {
		evidence.ThumbnailHash = journalSnapshotHash(content.Thumbnail)
	}
	publicFields, err := json.Marshal(struct {
		Resource string   `json:"resource,omitempty"`
		Title    string   `json:"title,omitempty"`
		Analysis []string `json:"analysis,omitempty"`
		Index    int32    `json:"index,omitempty"`
		Total    int32    `json:"total,omitempty"`
		Redacted bool     `json:"redacted"`
	}{
		Resource: content.Resource,
		Title:    content.Title,
		Analysis: content.Analysis,
		Index:    content.Index,
		Total:    content.Total,
		Redacted: content.Redacted,
	})
	if err != nil {
		return ErrJournalSnapshotUnsafeContent
	}
	evidence.PublicFieldsHash = journalSnapshotHash(publicFields)
	if err := s.JournalBrowserRedactionVerifier.VerifyJournalBrowserRedaction(
		ctx,
		evidence,
	); err != nil {
		return ErrJournalSnapshotUnsafeContent
	}
	return nil
}

func sanitizeJournalVisibleText(value string, limit int) string {
	value = journalANSIPattern.ReplaceAllString(value, "")
	value = journalURLUserInfoPattern.ReplaceAllString(value, "$1[redacted]@")
	value = journalCLIUserPattern.ReplaceAllString(value, "$1[redacted]")
	value = journalCLIPasswordPattern.ReplaceAllString(value, "$1[redacted]")
	value = journalCLIShortPassword.ReplaceAllString(value, "$1[redacted]")
	value = journalShellEnvPattern.ReplaceAllString(value, "$1$2=[redacted]")
	value = journalSecretPattern.ReplaceAllString(value, "[redacted]")
	value = publicAbsolutePathPattern.ReplaceAllString(value, " [internal path]")
	return publicCleanString(value, limit)
}

func sanitizeJournalHTTPSURL(value string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return "", ErrJournalSnapshotUnsafeContent
	}
	parsed.RawQuery = ""
	parsed.ForceQuery = false
	parsed.Fragment = ""
	return parsed.String(), nil
}

func validateJournalSignedURL(value string) (string, error) {
	value = strings.TrimSpace(value)
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil ||
		strings.ContainsAny(value, "\r\n\x00") {
		return "", ErrJournalSnapshotUnsafeContent
	}
	return value, nil
}

func validJournalObjectReference(value string) bool {
	value = strings.TrimSpace(value)
	cleaned := path.Clean(value)
	return value != "" && cleaned == value && cleaned != "." && cleaned != ".." &&
		!strings.HasPrefix(cleaned, "/") && !strings.HasPrefix(cleaned, "../") &&
		!strings.ContainsAny(value, "\r\n\x00")
}

func journalSnapshotStagingObjectKeyAllowed(spaceID int64, objectKey string) bool {
	if spaceID <= 0 || !validJournalObjectReference(objectKey) {
		return false
	}
	return strings.HasPrefix(
		objectKey,
		fmt.Sprintf("%s/%d/", journalSnapshotObjectPrefix, spaceID),
	)
}

func journalSnapshotOriginalObjectKeyAllowed(
	snapshot *entity.JournalContentSnapshot,
	objectKey string,
) bool {
	if snapshot == nil || !validJournalObjectReference(objectKey) {
		return false
	}
	switch snapshot.SourceResourceType {
	case "runtime_file", "artifact":
		return strings.HasPrefix(
			objectKey,
			fmt.Sprintf(
				"agent-runtime/%d/%d/runs/%d/",
				snapshot.SpaceID,
				snapshot.ThreadID,
				snapshot.RunID,
			),
		)
	default:
		return false
	}
}

func validateJournalSnapshotSourceIdentity(snapshot *entity.JournalContentSnapshot) error {
	if snapshot == nil {
		return ErrJournalSnapshotUnsafeContent
	}
	if snapshot.SourceResourceType == "" && snapshot.SourceResourceID == "" &&
		snapshot.SourceRevision == "" {
		return nil
	}
	if (snapshot.SourceResourceType != "runtime_file" &&
		snapshot.SourceResourceType != "artifact") ||
		snapshot.SourceResourceID == "" ||
		!journalSourceRevisionPattern.MatchString(snapshot.SourceRevision) {
		return ErrJournalSnapshotUnsafeContent
	}
	return nil
}

func validJournalBitmap(mimeType string, content []byte) bool {
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case "image/png":
		return len(content) >= 8 && bytes.Equal(content[:8], []byte("\x89PNG\r\n\x1a\n"))
	case "image/jpeg":
		return len(content) >= 3 && content[0] == 0xff && content[1] == 0xd8 && content[2] == 0xff
	case "image/webp":
		return len(content) >= 12 && string(content[:4]) == "RIFF" && string(content[8:12]) == "WEBP"
	default:
		return false
	}
}

func (s *ApplicationService) authorizeJournalSnapshot(
	ctx context.Context,
	snapshot *entity.JournalContentSnapshot,
	viewerID int64,
) error {
	if s == nil || snapshot == nil || s.JournalSnapshotAuthorizer == nil {
		return ErrJournalSnapshotNoPermission
	}
	err := s.JournalSnapshotAuthorizer.AuthorizeJournalSnapshotAccess(
		ctx,
		JournalSnapshotAccessRequest{
			SpaceID: snapshot.SpaceID, ThreadID: snapshot.ThreadID,
			RunID: snapshot.RunID, ViewerID: viewerID,
			SnapshotID:         snapshot.SnapshotID,
			SourceResourceType: snapshot.SourceResourceType,
			SourceResourceID:   snapshot.SourceResourceID,
		},
	)
	if err != nil {
		if errors.Is(err, ErrJournalSnapshotNoPermission) {
			return ErrJournalSnapshotNoPermission
		}
		return ErrJournalSnapshotUnavailable
	}
	switch snapshot.SourceResourceType {
	case "":
		return nil
	case "runtime_file":
		fileID, parseErr := strconv.ParseInt(snapshot.SourceResourceID, 10, 64)
		if parseErr != nil || fileID <= 0 || s.JournalSnapshotRuntimeFileReader == nil {
			return ErrJournalSnapshotNoPermission
		}
		file, readErr := s.JournalSnapshotRuntimeFileReader.GetFileByID(ctx, fileID)
		if readErr != nil || file == nil || file.ID != fileID ||
			file.SpaceID != snapshot.SpaceID || file.ThreadID != snapshot.ThreadID ||
			file.RunID != snapshot.RunID || file.Status != entity.AgentFileStatusActive ||
			strings.ToLower(strings.TrimSpace(file.Digest)) != snapshot.SourceRevision {
			return ErrJournalSnapshotNoPermission
		}
		if snapshot.OriginalObjectKey != "" &&
			file.ObjectURI != snapshot.OriginalObjectKey {
			return ErrJournalSnapshotNoPermission
		}
		return nil
	case "artifact":
		artifactID, parseErr := strconv.ParseInt(snapshot.SourceResourceID, 10, 64)
		if parseErr != nil || artifactID <= 0 || s.ArtifactAuthorizer == nil ||
			s.JournalSnapshotArtifactReader == nil {
			return ErrJournalSnapshotNoPermission
		}
		artifact, readErr := s.JournalSnapshotArtifactReader.GetArtifact(
			ctx,
			snapshot.ThreadID,
			artifactID,
		)
		if readErr != nil || artifact == nil || artifact.ID != artifactID ||
			artifact.SpaceID != snapshot.SpaceID || artifact.ThreadID != snapshot.ThreadID ||
			artifact.RunID != snapshot.RunID || artifact.DeletedAt != 0 ||
			journalArtifactContentHash(artifact.Metadata) != snapshot.SourceRevision {
			return ErrJournalSnapshotNoPermission
		}
		if snapshot.OriginalObjectKey != "" &&
			artifact.ObjectURI != snapshot.OriginalObjectKey {
			return ErrJournalSnapshotNoPermission
		}
		if err := s.authorizeArtifactAccess(ctx, ArtifactAccessRequest{
			ThreadID: snapshot.ThreadID, ArtifactID: artifactID,
			SpaceID: snapshot.SpaceID, ViewerID: viewerID,
			Operation: ArtifactAccessOperationRead,
		}); err != nil {
			return ErrJournalSnapshotNoPermission
		}
		return nil
	default:
		return ErrJournalSnapshotNoPermission
	}
}

func journalArtifactContentHash(metadata string) string {
	var value struct {
		ContentHash string `json:"content_hash"`
	}
	if err := json.Unmarshal([]byte(metadata), &value); err != nil {
		return ""
	}
	value.ContentHash = strings.ToLower(strings.TrimSpace(value.ContentHash))
	if !journalSourceRevisionPattern.MatchString(value.ContentHash) {
		return ""
	}
	return value.ContentHash
}

func (s *ApplicationService) recordJournalSnapshotAudit(
	ctx context.Context,
	snapshot *entity.JournalContentSnapshot,
	spaceID, threadID, runID, actorID int64,
	snapshotID string,
	action entity.JournalSnapshotAction,
	idempotencyKey, targetHash, traceID string,
	permission entity.JournalSnapshotPermissionResult,
) error {
	if snapshot != nil {
		_, err := s.recordJournalSnapshotAuditWithResult(
			ctx,
			snapshot,
			actorID,
			action,
			idempotencyKey,
			targetHash,
			traceID,
			permission,
		)
		return err
	}
	_, _, err := s.JournalSnapshotRepository.RecordJournalSnapshotAccess(
		ctx,
		&entity.JournalSnapshotAccessAudit{
			SpaceID: spaceID, ThreadID: threadID, RunID: runID,
			SnapshotID: strings.TrimSpace(snapshotID), Action: action,
			ActorID: actorID, PermissionResult: permission,
			IdempotencyKey: strings.TrimSpace(idempotencyKey),
			TargetHash:     strings.TrimSpace(targetHash),
			TraceID:        publicIdentifier(traceID, 128), CreatedAt: s.journalSnapshotNow(),
		},
	)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrJournalSnapshotAuditUnavailable, err)
	}
	return nil
}

func (s *ApplicationService) recordJournalSnapshotAuditWithResult(
	ctx context.Context,
	snapshot *entity.JournalContentSnapshot,
	actorID int64,
	action entity.JournalSnapshotAction,
	idempotencyKey, targetHash, traceID string,
	permission entity.JournalSnapshotPermissionResult,
) (*entity.JournalSnapshotAccessAudit, error) {
	if s == nil || s.JournalSnapshotRepository == nil || snapshot == nil {
		return nil, ErrJournalSnapshotAuditUnavailable
	}
	journalRunID := snapshot.JournalRunID
	if journalRunID <= 0 {
		journalRunID = snapshot.RunID
	}
	audit, replayed, err := s.JournalSnapshotRepository.RecordJournalSnapshotAccess(
		ctx,
		&entity.JournalSnapshotAccessAudit{
			SpaceID: snapshot.SpaceID, ThreadID: snapshot.ThreadID,
			RunID: journalRunID, AttemptID: snapshot.AttemptID,
			SnapshotID: snapshot.SnapshotID, ContentType: snapshot.ContentType,
			Action: action, ActorID: actorID, PermissionResult: permission,
			IdempotencyKey: strings.TrimSpace(idempotencyKey),
			TargetHash:     strings.TrimSpace(targetHash),
			TraceID:        publicIdentifier(traceID, 128), CreatedAt: s.journalSnapshotNow(),
		},
	)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrJournalSnapshotAuditUnavailable, err)
	}
	if replayed && (audit == nil || audit.TargetHash != strings.TrimSpace(targetHash)) {
		return nil, ErrJournalSnapshotIdempotencyConflict
	}
	if replayed && audit != nil && audit.PermissionResult != permission {
		derivedKey := strings.TrimSpace(idempotencyKey) + ":reauth:" + string(permission)
		if len(derivedKey) > 191 {
			derivedKey = journalSnapshotHash([]byte(derivedKey))
		}
		audit, _, err = s.JournalSnapshotRepository.RecordJournalSnapshotAccess(
			ctx,
			&entity.JournalSnapshotAccessAudit{
				SpaceID: snapshot.SpaceID, ThreadID: snapshot.ThreadID,
				RunID: journalRunID, AttemptID: snapshot.AttemptID,
				SnapshotID: snapshot.SnapshotID, ContentType: snapshot.ContentType,
				Action: action, ActorID: actorID, PermissionResult: permission,
				IdempotencyKey: derivedKey, TargetHash: strings.TrimSpace(targetHash),
				TraceID:   publicIdentifier(traceID, 128),
				CreatedAt: s.journalSnapshotNow(),
			},
		)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrJournalSnapshotAuditUnavailable, err)
		}
	}
	return audit, nil
}

func (s *ApplicationService) loadJournalSnapshotEnvelope(
	ctx context.Context,
	snapshot *entity.JournalContentSnapshot,
	cursor string,
	limit int,
) (*JournalSnapshotEnvelope, error) {
	afterIndex, limit, err := normalizeJournalSnapshotPagination(cursor, limit)
	if err != nil {
		return nil, err
	}
	envelope := journalSnapshotRestrictedEnvelope(snapshot, snapshot.Status, snapshot.ErrorCode)
	if !snapshot.IsFragmented {
		content, err := s.loadCompleteJournalSnapshotContent(ctx, snapshot)
		if err != nil {
			return nil, err
		}
		typed, err := s.decodeJournalTypedSnapshotContent(ctx, content)
		if err != nil {
			return nil, err
		}
		envelope.Content = typed
		return envelope, nil
	}
	typed, err := decodeJournalSnapshotSummary(snapshot)
	if err != nil {
		return nil, ErrJournalSnapshotUnavailable
	}
	envelope.Content = typed
	page, err := s.JournalSnapshotRepository.ListJournalSnapshotFragments(
		ctx,
		domainrepo.ListJournalSnapshotFragmentsRequest{
			SpaceID: snapshot.SpaceID, SnapshotID: snapshot.SnapshotID,
			AfterIndex: afterIndex, Limit: limit,
		},
	)
	if err != nil {
		return nil, err
	}
	if !validJournalSnapshotFragmentPage(page, afterIndex, limit, int(snapshot.FragmentCount)) {
		return nil, ErrJournalSnapshotUnavailable
	}
	for _, fragment := range page.Fragments {
		fragmentContent, err := s.loadJournalSnapshotFragment(ctx, snapshot.SpaceID, fragment)
		if err != nil {
			return nil, err
		}
		view, err := journalSnapshotFragmentPublicView(snapshot, fragment, fragmentContent)
		if err != nil {
			return nil, err
		}
		envelope.Fragments = append(envelope.Fragments, view)
	}
	envelope.HasMore = page.HasMore
	if page.HasMore {
		envelope.NextCursor = strconv.FormatInt(int64(page.NextIndex), 10)
	}
	return envelope, nil
}

func decodeJournalSnapshotSummary(
	snapshot *entity.JournalContentSnapshot,
) (JournalTypedSnapshotContent, error) {
	if snapshot == nil || !snapshot.IsFragmented || snapshot.SummaryJSON == "" ||
		len(snapshot.SummaryJSON) > journalSnapshotMaxSummaryBytes ||
		len(snapshot.SummaryHash) != sha256.Size*2 ||
		journalSnapshotHash([]byte(snapshot.SummaryJSON)) != snapshot.SummaryHash ||
		!json.Valid([]byte(snapshot.SummaryJSON)) {
		return JournalTypedSnapshotContent{}, ErrJournalSnapshotUnavailable
	}
	var typed JournalTypedSnapshotContent
	if err := json.Unmarshal([]byte(snapshot.SummaryJSON), &typed); err != nil {
		return JournalTypedSnapshotContent{}, ErrJournalSnapshotUnavailable
	}
	valid := false
	switch snapshot.ContentType {
	case entity.JournalSnapshotContentTypeDocument:
		valid = typed.Document != nil && typed.Terminal == nil && typed.Code == nil &&
			typed.Skill == nil && typed.Browser == nil && typed.Document.Content == "" &&
			len(typed.Document.Chapters) == 0
	case entity.JournalSnapshotContentTypeTerminal:
		valid = typed.Terminal != nil && typed.Document == nil && typed.Code == nil &&
			typed.Skill == nil && typed.Browser == nil && typed.Terminal.Stdout == "" &&
			typed.Terminal.Stderr == ""
	case entity.JournalSnapshotContentTypeCode:
		valid = typed.Code != nil && typed.Document == nil && typed.Terminal == nil &&
			typed.Skill == nil && typed.Browser == nil && typed.Code.Content == "" &&
			len(typed.Code.Highlights) == 0
	case entity.JournalSnapshotContentTypeSkill:
		valid = typed.Skill != nil && typed.Document == nil && typed.Terminal == nil &&
			typed.Code == nil && typed.Browser == nil && len(typed.Skill.Skills) == 0
	case entity.JournalSnapshotContentTypeBrowser:
		valid = typed.Browser != nil && typed.Document == nil && typed.Terminal == nil &&
			typed.Code == nil && typed.Skill == nil && len(typed.Browser.Thumbnail) == 0 &&
			len(typed.Browser.StaticSnapshot) == 0 && len(typed.Browser.Analysis) == 0
	}
	if !valid {
		return JournalTypedSnapshotContent{}, ErrJournalSnapshotUnavailable
	}
	return typed, nil
}

func validJournalSnapshotFragmentPage(
	page *domainrepo.ListJournalSnapshotFragmentsResult,
	afterIndex int32,
	limit int,
	total int,
) bool {
	if page == nil || afterIndex < -1 || limit < 1 || total < 1 {
		return false
	}
	start := int64(afterIndex) + 1
	if start < 0 || start > int64(total) {
		return false
	}
	remaining := total - int(start)
	expectedCount := min(limit, remaining)
	if len(page.Fragments) != expectedCount ||
		page.HasMore != (remaining > expectedCount) {
		return false
	}
	expectedIndex := afterIndex
	for _, fragment := range page.Fragments {
		expectedIndex++
		if fragment == nil || fragment.FragmentIndex != expectedIndex {
			return false
		}
	}
	return page.NextIndex == expectedIndex
}

func (s *ApplicationService) decodeJournalTypedSnapshotContent(
	ctx context.Context,
	content []byte,
) (JournalTypedSnapshotContent, error) {
	var typed JournalTypedSnapshotContent
	if err := json.Unmarshal(content, &typed); err != nil {
		return JournalTypedSnapshotContent{}, ErrJournalSnapshotUnavailable
	}
	if typed.Browser != nil {
		if err := s.verifyJournalBrowserRedaction(ctx, typed.Browser); err != nil {
			return JournalTypedSnapshotContent{}, err
		}
	}
	return typed, nil
}

func journalSnapshotContentSummary(
	typed JournalTypedSnapshotContent,
) JournalTypedSnapshotContent {
	summary := typed
	if typed.Document != nil {
		value := *typed.Document
		value.Content = ""
		value.Chapters = nil
		summary.Document = &value
	}
	if typed.Terminal != nil {
		value := *typed.Terminal
		value.Stdout = ""
		value.Stderr = ""
		summary.Terminal = &value
	}
	if typed.Code != nil {
		value := *typed.Code
		value.Content = ""
		value.Highlights = nil
		summary.Code = &value
	}
	if typed.Skill != nil {
		summary.Skill = &JournalSkillContent{Skills: []JournalSkillSummary{}}
	}
	if typed.Browser != nil {
		value := *typed.Browser
		value.Thumbnail = nil
		value.StaticSnapshot = nil
		value.Analysis = nil
		summary.Browser = &value
	}
	return summary
}

func journalSnapshotFragmentPublicView(
	snapshot *entity.JournalContentSnapshot,
	fragment *entity.JournalSnapshotFragment,
	content []byte,
) (JournalSnapshotFragmentView, error) {
	if snapshot == nil || fragment == nil ||
		!journalSnapshotFragmentMatchesContentType(snapshot.ContentType, fragment.Kind) ||
		fragment.FragmentID != journalSnapshotFragmentID(
			snapshot.SnapshotID,
			int(fragment.FragmentIndex),
		) {
		return JournalSnapshotFragmentView{}, ErrJournalSnapshotUnavailable
	}
	metadata := journalSnapshotFragmentMetadata{}
	if fragment.MetadataJSON != "" &&
		json.Unmarshal([]byte(fragment.MetadataJSON), &metadata) != nil {
		return JournalSnapshotFragmentView{}, ErrJournalSnapshotUnavailable
	}
	canonicalMetadata, err := json.Marshal(metadata)
	if err != nil {
		return JournalSnapshotFragmentView{}, ErrJournalSnapshotUnavailable
	}
	if string(canonicalMetadata) == "{}" {
		canonicalMetadata = nil
	}
	if string(canonicalMetadata) != fragment.MetadataJSON ||
		!validJournalSnapshotFragmentMetadata(fragment.Kind, fragment, metadata) {
		return JournalSnapshotFragmentView{}, ErrJournalSnapshotUnavailable
	}
	view := JournalSnapshotFragmentView{
		FragmentID: fragment.FragmentID, FragmentIndex: fragment.FragmentIndex,
		Kind: fragment.Kind, MIMEType: fragment.MIMEType,
		BlockID: metadata.BlockID, Stream: metadata.Stream,
		StartLine: metadata.StartLine, EndLine: metadata.EndLine,
		ItemStart: metadata.ItemStart, ItemEnd: metadata.ItemEnd,
		ByteStart: fragment.ByteStart, ByteEnd: fragment.ByteEnd,
		SizeBytes: fragment.SizeBytes, ContentHash: fragment.ContentHash,
	}
	switch fragment.Kind {
	case JournalSnapshotFragmentKindDocumentBlock,
		JournalSnapshotFragmentKindTerminalStdout,
		JournalSnapshotFragmentKindTerminalStderr,
		JournalSnapshotFragmentKindCodeLines:
		view.Content = string(content)
	case JournalSnapshotFragmentKindDocumentChapters:
		if err := json.Unmarshal(content, &view.Chapters); err != nil {
			return JournalSnapshotFragmentView{}, ErrJournalSnapshotUnavailable
		}
		if len(view.Chapters) != int(metadata.ItemEnd-metadata.ItemStart) {
			return JournalSnapshotFragmentView{}, ErrJournalSnapshotUnavailable
		}
	case JournalSnapshotFragmentKindCodeHighlights:
		if err := json.Unmarshal(content, &view.Highlights); err != nil {
			return JournalSnapshotFragmentView{}, ErrJournalSnapshotUnavailable
		}
		if len(view.Highlights) != int(metadata.ItemEnd-metadata.ItemStart) {
			return JournalSnapshotFragmentView{}, ErrJournalSnapshotUnavailable
		}
	case JournalSnapshotFragmentKindSkillItems:
		if err := json.Unmarshal(content, &view.Skills); err != nil {
			return JournalSnapshotFragmentView{}, ErrJournalSnapshotUnavailable
		}
		if len(view.Skills) != int(metadata.ItemEnd-metadata.ItemStart) {
			return JournalSnapshotFragmentView{}, ErrJournalSnapshotUnavailable
		}
	case JournalSnapshotFragmentKindBrowserThumbnail,
		JournalSnapshotFragmentKindBrowserSnapshot:
		view.BinaryContent = append([]byte(nil), content...)
	case JournalSnapshotFragmentKindBrowserAnalysis:
		if err := json.Unmarshal(content, &view.Analysis); err != nil {
			return JournalSnapshotFragmentView{}, ErrJournalSnapshotUnavailable
		}
		if len(view.Analysis) != int(metadata.ItemEnd-metadata.ItemStart) {
			return JournalSnapshotFragmentView{}, ErrJournalSnapshotUnavailable
		}
	default:
		return JournalSnapshotFragmentView{}, ErrJournalSnapshotUnavailable
	}
	return view, nil
}

func validJournalSnapshotFragmentMetadata(
	kind JournalSnapshotFragmentKind,
	fragment *entity.JournalSnapshotFragment,
	metadata journalSnapshotFragmentMetadata,
) bool {
	if fragment == nil || fragment.FragmentIndex < 0 {
		return false
	}
	validLines := metadata.StartLine >= 1 && metadata.EndLine >= metadata.StartLine
	validItems := metadata.ItemStart >= 0 && metadata.ItemEnd > metadata.ItemStart
	switch kind {
	case JournalSnapshotFragmentKindDocumentBlock:
		return metadata.BlockID != "" && validLines
	case JournalSnapshotFragmentKindTerminalStdout:
		return metadata.Stream == "stdout" && validLines
	case JournalSnapshotFragmentKindTerminalStderr:
		return metadata.Stream == "stderr" && validLines
	case JournalSnapshotFragmentKindCodeLines:
		return validLines
	case JournalSnapshotFragmentKindDocumentChapters,
		JournalSnapshotFragmentKindCodeHighlights,
		JournalSnapshotFragmentKindSkillItems,
		JournalSnapshotFragmentKindBrowserAnalysis:
		return validItems
	case JournalSnapshotFragmentKindBrowserThumbnail,
		JournalSnapshotFragmentKindBrowserSnapshot:
		return validItems && int64(metadata.ItemStart) == fragment.ByteStart &&
			int64(metadata.ItemEnd) == fragment.ByteEnd
	default:
		return false
	}
}

func journalSnapshotFragmentMatchesContentType(
	contentType entity.JournalSnapshotContentType,
	kind JournalSnapshotFragmentKind,
) bool {
	switch contentType {
	case entity.JournalSnapshotContentTypeDocument:
		return kind == JournalSnapshotFragmentKindDocumentBlock ||
			kind == JournalSnapshotFragmentKindDocumentChapters
	case entity.JournalSnapshotContentTypeTerminal:
		return kind == JournalSnapshotFragmentKindTerminalStdout ||
			kind == JournalSnapshotFragmentKindTerminalStderr
	case entity.JournalSnapshotContentTypeCode:
		return kind == JournalSnapshotFragmentKindCodeLines ||
			kind == JournalSnapshotFragmentKindCodeHighlights
	case entity.JournalSnapshotContentTypeSkill:
		return kind == JournalSnapshotFragmentKindSkillItems
	case entity.JournalSnapshotContentTypeBrowser:
		return kind == JournalSnapshotFragmentKindBrowserThumbnail ||
			kind == JournalSnapshotFragmentKindBrowserSnapshot ||
			kind == JournalSnapshotFragmentKindBrowserAnalysis
	default:
		return false
	}
}

func (s *ApplicationService) loadJournalSnapshotFragment(
	ctx context.Context,
	spaceID int64,
	fragment *entity.JournalSnapshotFragment,
) ([]byte, error) {
	if fragment == nil {
		return nil, ErrJournalSnapshotUnavailable
	}
	if fragment.ObjectKey == "" {
		content := []byte(fragment.InlineContent)
		if !validStoredJournalSnapshotFragment(fragment, content) {
			return nil, ErrJournalSnapshotUnavailable
		}
		return content, nil
	}
	if s.JournalSnapshotObjectStorage == nil {
		return nil, ErrJournalSnapshotUnavailable
	}
	if !journalSnapshotStagingObjectKeyAllowed(spaceID, fragment.ObjectKey) {
		return nil, ErrJournalSnapshotUnavailable
	}
	content, err := readJournalSnapshotObject(
		ctx,
		s.JournalSnapshotObjectStorage,
		fragment.ObjectKey,
		journalSnapshotFragmentBytes,
	)
	if err != nil || !validStoredJournalSnapshotFragment(fragment, content) {
		return nil, ErrJournalSnapshotUnavailable
	}
	return content, nil
}

func validStoredJournalSnapshotFragment(
	fragment *entity.JournalSnapshotFragment,
	content []byte,
) bool {
	if fragment == nil || !fragment.Kind.Valid() ||
		len(content) > journalSnapshotFragmentBytes ||
		fragment.SizeBytes != int64(len(content)) ||
		fragment.ByteStart < 0 || fragment.ByteEnd < fragment.ByteStart ||
		fragment.ByteEnd-fragment.ByteStart != fragment.SizeBytes ||
		len(fragment.ContentHash) != sha256.Size*2 ||
		(fragment.MetadataJSON != "" && !json.Valid([]byte(fragment.MetadataJSON))) {
		return false
	}
	switch fragment.Kind {
	case JournalSnapshotFragmentKindBrowserThumbnail,
		JournalSnapshotFragmentKindBrowserSnapshot:
		if fragment.MIMEType != "image/png" && fragment.MIMEType != "image/jpeg" &&
			fragment.MIMEType != "image/webp" {
			return false
		}
	default:
		if !utf8.Valid(content) {
			return false
		}
	}
	return journalSnapshotHash(content) == fragment.ContentHash
}

func (s *ApplicationService) loadCompleteJournalSnapshotContent(
	ctx context.Context,
	snapshot *entity.JournalContentSnapshot,
) ([]byte, error) {
	if snapshot.ContentJSON != "" {
		return validateStoredJournalSnapshotContent(snapshot, []byte(snapshot.ContentJSON))
	}
	if snapshot.ObjectKey != "" {
		if s.JournalSnapshotObjectStorage == nil {
			return nil, ErrJournalSnapshotUnavailable
		}
		if !journalSnapshotStagingObjectKeyAllowed(snapshot.SpaceID, snapshot.ObjectKey) {
			return nil, ErrJournalSnapshotUnavailable
		}
		content, err := readJournalSnapshotObject(
			ctx,
			s.JournalSnapshotObjectStorage,
			snapshot.ObjectKey,
			journalSnapshotMaxContentBytes,
		)
		if err != nil || len(content) > journalSnapshotMaxContentBytes || !json.Valid(content) {
			return nil, ErrJournalSnapshotUnavailable
		}
		if snapshot.ContentHash != "" && journalSnapshotHash(content) != snapshot.ContentHash {
			return nil, ErrJournalSnapshotUnavailable
		}
		return validateStoredJournalSnapshotContent(snapshot, content)
	}
	if !snapshot.IsFragmented {
		return nil, ErrJournalSnapshotUnavailable
	}
	var result []byte
	after := int32(-1)
	for {
		page, err := s.JournalSnapshotRepository.ListJournalSnapshotFragments(
			ctx,
			domainrepo.ListJournalSnapshotFragmentsRequest{
				SpaceID: snapshot.SpaceID, SnapshotID: snapshot.SnapshotID,
				AfterIndex: after, Limit: 200,
			},
		)
		if err != nil {
			return nil, err
		}
		for _, fragment := range page.Fragments {
			content, err := s.loadJournalSnapshotFragment(ctx, snapshot.SpaceID, fragment)
			if err != nil {
				return nil, err
			}
			result = append(result, []byte(content)...)
			if len(result) > journalSnapshotMaxContentBytes {
				return nil, ErrJournalSnapshotUnavailable
			}
		}
		if !page.HasMore {
			break
		}
		if page.NextIndex <= after {
			return nil, ErrJournalSnapshotUnavailable
		}
		after = page.NextIndex
	}
	if !json.Valid(result) || journalSnapshotHash(result) != snapshot.ContentHash {
		return nil, ErrJournalSnapshotUnavailable
	}
	return validateStoredJournalSnapshotContent(snapshot, result)
}

func readJournalSnapshotObject(
	ctx context.Context,
	storage JournalSnapshotObjectStorage,
	objectKey string,
	maxBytes int,
) ([]byte, error) {
	if storage == nil || maxBytes < 1 {
		return nil, ErrJournalSnapshotUnavailable
	}
	reader, err := storage.OpenJournalSnapshotObject(ctx, objectKey)
	if err != nil || reader == nil {
		return nil, ErrJournalSnapshotUnavailable
	}
	content, readErr := io.ReadAll(io.LimitReader(reader, int64(maxBytes)+1))
	closeErr := reader.Close()
	if readErr != nil || closeErr != nil || len(content) > maxBytes {
		return nil, ErrJournalSnapshotUnavailable
	}
	return content, nil
}

func validateStoredJournalSnapshotContent(
	snapshot *entity.JournalContentSnapshot,
	content []byte,
) ([]byte, error) {
	if snapshot == nil || len(content) > journalSnapshotMaxContentBytes ||
		!utf8.Valid(content) || !json.Valid(content) {
		return nil, ErrJournalSnapshotUnavailable
	}
	if snapshot.ContentHash != "" && journalSnapshotHash(content) != snapshot.ContentHash {
		return nil, ErrJournalSnapshotUnavailable
	}
	var typed JournalTypedSnapshotContent
	if err := json.Unmarshal(content, &typed); err != nil {
		return nil, ErrJournalSnapshotUnavailable
	}
	_, safe, _, _, err := prepareJournalTypedSnapshotContent(
		snapshot.ContentType,
		snapshot.Status,
		typed,
	)
	if err != nil {
		return nil, ErrJournalSnapshotUnsafeContent
	}
	canonical, err := json.Marshal(safe)
	if err != nil {
		return nil, ErrJournalSnapshotUnavailable
	}
	return canonical, nil
}

func (s *ApplicationService) populateJournalSnapshotActionGrant(
	ctx context.Context,
	snapshot *entity.JournalContentSnapshot,
	req AuditJournalSnapshotActionRequest,
	grant *JournalSnapshotActionGrant,
) error {
	switch req.Action {
	case entity.JournalSnapshotActionOpenOriginal:
		if snapshot.SourceResourceType != "artifact" ||
			s.JournalSnapshotArtifactCapabilityIssuer == nil {
			return ErrJournalSnapshotActionUnavailable
		}
		artifactID, err := strconv.ParseInt(snapshot.SourceResourceID, 10, 64)
		if err != nil || artifactID <= 0 {
			return ErrJournalSnapshotActionUnavailable
		}
		response, err := s.JournalSnapshotArtifactCapabilityIssuer.CreateArtifactSignedURL(
			ctx,
			&CreateArtifactSignedURLRequest{
				ThreadID: snapshot.ThreadID, ArtifactID: artifactID,
				Mode: ArtifactContentModeDownload, SpaceID: snapshot.SpaceID,
				ViewerID: req.ViewerID, TTLSeconds: journalSnapshotSignedURLTTLSeconds,
			},
		)
		if err != nil {
			return err
		}
		if response == nil {
			return ErrJournalSnapshotActionUnavailable
		}
		grant.DownloadURL, err = validateJournalSignedURL(response.URL)
		if err != nil {
			return err
		}
		return nil
	case entity.JournalSnapshotActionDownloadFragment:
		return s.populateJournalSnapshotFragmentGrant(ctx, snapshot, req.FragmentID, grant)
	}
	content, err := s.loadCompleteJournalSnapshotContent(ctx, snapshot)
	if err != nil {
		return err
	}
	var typed JournalTypedSnapshotContent
	if err := json.Unmarshal(content, &typed); err != nil {
		return ErrJournalSnapshotUnavailable
	}
	switch req.Action {
	case entity.JournalSnapshotActionCopyCommand:
		if typed.Terminal == nil {
			return ErrJournalSnapshotActionUnavailable
		}
		grant.CopyText = typed.Terminal.Command
	case entity.JournalSnapshotActionCopyOutput:
		if typed.Terminal == nil {
			return ErrJournalSnapshotActionUnavailable
		}
		grant.CopyText = strings.TrimSpace(strings.Join([]string{
			typed.Terminal.Stdout,
			typed.Terminal.Stderr,
		}, "\n"))
	case entity.JournalSnapshotActionCopyCode:
		if typed.Code == nil {
			return ErrJournalSnapshotActionUnavailable
		}
		grant.CopyText = typed.Code.Content
	default:
		return ErrJournalSnapshotActionUnavailable
	}
	return nil
}

func (s *ApplicationService) populateJournalSnapshotFragmentGrant(
	ctx context.Context,
	snapshot *entity.JournalContentSnapshot,
	fragmentID string,
	grant *JournalSnapshotActionGrant,
) error {
	const pageLimit = 200
	after := int32(-1)
	for {
		page, err := s.JournalSnapshotRepository.ListJournalSnapshotFragments(
			ctx,
			domainrepo.ListJournalSnapshotFragmentsRequest{
				SpaceID: snapshot.SpaceID, SnapshotID: snapshot.SnapshotID,
				AfterIndex: after, Limit: pageLimit,
			},
		)
		if err != nil {
			return err
		}
		if !validJournalSnapshotFragmentPage(
			page,
			after,
			pageLimit,
			int(snapshot.FragmentCount),
		) {
			return ErrJournalSnapshotUnavailable
		}
		for _, fragment := range page.Fragments {
			if fragment.FragmentID != fragmentID {
				continue
			}
			switch fragment.Kind {
			case JournalSnapshotFragmentKindDocumentChapters,
				JournalSnapshotFragmentKindCodeHighlights,
				JournalSnapshotFragmentKindSkillItems,
				JournalSnapshotFragmentKindBrowserAnalysis:
				return ErrJournalSnapshotActionUnavailable
			}
			if fragment.ObjectKey != "" && !journalSnapshotStagingObjectKeyAllowed(
				snapshot.SpaceID,
				fragment.ObjectKey,
			) {
				return ErrJournalSnapshotNoPermission
			}
			content, err := s.loadJournalSnapshotFragment(
				ctx,
				snapshot.SpaceID,
				fragment,
			)
			if err != nil {
				return err
			}
			if _, err := journalSnapshotFragmentPublicView(
				snapshot,
				fragment,
				content,
			); err != nil {
				return err
			}
			grant.DownloadContent = append([]byte(nil), content...)
			grant.DownloadMIMEType = fragment.MIMEType
			if grant.DownloadMIMEType == "" {
				grant.DownloadMIMEType = snapshot.MIMEType
			}
			if grant.DownloadMIMEType == "" {
				grant.DownloadMIMEType = "application/octet-stream"
			}
			return nil
		}
		if !page.HasMore {
			return ErrJournalSnapshotActionUnavailable
		}
		after = page.NextIndex
	}
}

func journalSnapshotRestrictedEnvelope(
	snapshot *entity.JournalContentSnapshot,
	status entity.JournalContentStatus,
	errorCode string,
) *JournalSnapshotEnvelope {
	return &JournalSnapshotEnvelope{
		ContentType: snapshot.ContentType, SnapshotID: snapshot.SnapshotID,
		EventID: snapshot.EventID, AttemptID: snapshot.AttemptID,
		IsFragmented: snapshot.IsFragmented, Status: status,
		CreatedAt: snapshot.CreatedAt, Visibility: snapshot.Visibility,
		ErrorCode: errorCode, CacheControl: JournalSnapshotCacheControl,
	}
}

func journalSnapshotEventStatus(status entity.JournalContentStatus) string {
	switch status {
	case entity.JournalContentStatusLoading, entity.JournalContentStatusStreaming:
		return "running"
	case entity.JournalContentStatusError, entity.JournalContentStatusNoPermission:
		return "failed"
	default:
		return "completed"
	}
}

func journalSnapshotActionEventIdentity(status entity.JournalContentStatus) (string, string) {
	switch status {
	case entity.JournalContentStatusLoading:
		return "action.started", "started"
	case entity.JournalContentStatusStreaming:
		return "action.progress", "progress"
	default:
		return "action.terminal", "terminal"
	}
}

func prepareJournalContentAction(action JournalContentAction) (JournalContentAction, error) {
	action.ActionID = publicIdentifier(action.ActionID, 191)
	action.MilestoneID = publicIdentifier(action.MilestoneID, 191)
	action.Operation = publicIdentifier(action.Operation, 128)
	action.Target = publicLabel(action.Target, 512)
	action.DisplayVerbRunning = publicLabel(action.DisplayVerbRunning, 512)
	action.DisplayVerbCompleted = publicLabel(action.DisplayVerbCompleted, 512)
	if action.ActionID == "" || action.Operation == "" || action.Target == "" ||
		action.DisplayVerbRunning == "" || action.DisplayVerbCompleted == "" ||
		action.ParentEventID < 0 {
		return JournalContentAction{}, fmt.Errorf("journal content action identity is invalid")
	}
	return action, nil
}

func journalSnapshotServerID(
	req JournalContentSubmission,
	actionID string,
	revision uint32,
) string {
	identity := fmt.Sprintf(
		"%d:%d:%d:%s:%s:%d",
		req.SpaceID,
		req.ThreadID,
		req.RunID,
		strings.TrimSpace(req.AttemptID),
		actionID,
		revision,
	)
	return uuid.NewHash(sha256.New(), uuid.NameSpaceOID, []byte(identity), 8).String()
}

func journalSnapshotStagingPrefix(
	snapshot *entity.JournalContentSnapshot,
	reservationToken string,
) string {
	aclHash := journalSnapshotHash([]byte(snapshot.ACLDomain))[:16]
	day := time.UnixMilli(snapshot.CreatedAt).UTC().Format("20060102")
	return fmt.Sprintf(
		"%s/%d/%s/%s/%s/%s/%s/",
		journalSnapshotObjectPrefix,
		snapshot.SpaceID,
		day,
		aclHash,
		snapshot.ContentHash,
		snapshot.SnapshotID,
		reservationToken,
	)
}

func journalSnapshotStagingKey(
	stagingPrefix string,
	fragmentIndex int,
	fragmentHash string,
) string {
	return fmt.Sprintf(
		"%s%04d-%s",
		stagingPrefix,
		fragmentIndex,
		fragmentHash[:16],
	)
}

func journalSnapshotFragmentID(snapshotID string, fragmentIndex int) string {
	return fmt.Sprintf(
		"fragment-%s-%04d",
		journalSnapshotHash([]byte(snapshotID))[:32],
		fragmentIndex,
	)
}

func splitJournalSnapshotUTF8(content []byte, size int) [][]byte {
	if len(content) == 0 {
		return [][]byte{{}}
	}
	chunks := make([][]byte, 0, (len(content)+size-1)/size)
	for start := 0; start < len(content); {
		end := start + size
		if end >= len(content) {
			end = len(content)
		} else {
			for end > start && !utf8.Valid(content[start:end]) {
				end--
			}
			if end == start {
				end = start + size
			}
		}
		chunks = append(chunks, append([]byte(nil), content[start:end]...))
		start = end
	}
	return chunks
}

func journalSnapshotHash(content []byte) string {
	digest := sha256.Sum256(content)
	return hex.EncodeToString(digest[:])
}

func journalSnapshotTargetHash(
	snapshotID string,
	action entity.JournalSnapshotAction,
	target string,
) string {
	return journalSnapshotHash([]byte(strings.Join([]string{
		strings.TrimSpace(snapshotID),
		string(action),
		strings.TrimSpace(target),
	}, "\x00")))
}

func journalSnapshotActionTargetHash(
	snapshot *entity.JournalContentSnapshot,
	action entity.JournalSnapshotAction,
	fragmentID string,
) string {
	if snapshot == nil {
		return journalSnapshotTargetHash("", action, fragmentID)
	}
	target := strings.TrimSpace(fragmentID)
	if action == entity.JournalSnapshotActionOpenOriginal {
		target = strings.Join([]string{
			snapshot.SourceResourceType,
			snapshot.SourceResourceID,
			snapshot.SourceRevision,
			journalSnapshotHash([]byte(snapshot.OriginalObjectKey)),
		}, ":")
	}
	return journalSnapshotTargetHash(snapshot.SnapshotID, action, target)
}

func journalSnapshotReadIdempotency(traceID string, viewerID, now int64) string {
	traceID = publicIdentifier(traceID, 128)
	if traceID != "" {
		return "read:" + traceID
	}
	return fmt.Sprintf("read:%d:%d", viewerID, now)
}

func parseJournalSnapshotCursor(cursor string) (int32, error) {
	cursor = strings.TrimSpace(cursor)
	if cursor == "" {
		return -1, nil
	}
	value, err := strconv.ParseInt(cursor, 10, 32)
	if err != nil || value < -1 {
		return 0, fmt.Errorf("journal snapshot cursor is invalid")
	}
	return int32(value), nil
}

func normalizeJournalSnapshotPagination(cursor string, limit int) (int32, int, error) {
	afterIndex, err := parseJournalSnapshotCursor(cursor)
	if err != nil {
		return 0, 0, err
	}
	if limit == 0 {
		limit = journalSnapshotDefaultFragmentLimit
	}
	if limit < 1 || limit > journalSnapshotMaxFragmentLimit {
		return 0, 0, fmt.Errorf("journal snapshot fragment pagination is invalid")
	}
	return afterIndex, limit, nil
}

func validJournalSnapshotKey(value string, limit int, allowEmpty bool) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return allowEmpty
	}
	if len(value) > limit {
		return false
	}
	return strings.IndexFunc(value, func(r rune) bool {
		return r < 0x20 || r == 0x7f
	}) < 0
}

func validJournalSnapshotIdentifier(value string, limit int, allowEmpty bool) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return allowEmpty
	}
	return len(value) <= limit && journalSnapshotKeyPattern.MatchString(value)
}

func (s *ApplicationService) journalSnapshotNow() int64 {
	if s != nil && s.JournalSnapshotNow != nil {
		return s.JournalSnapshotNow()
	}
	return time.Now().UnixMilli()
}
