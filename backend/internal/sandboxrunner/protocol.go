// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandboxrunner

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"io"
	"mime"
	"net/url"
	"path"
	"strings"
	"time"
	"unicode/utf8"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
)

const maxRequestBytes = 1024 * 1024

type executeWireRequest struct {
	Schema         string                       `json:"schema"`
	Scope          domainsandbox.Scope          `json:"scope"`
	WorkloadKind   infrasandbox.WorkloadKind    `json:"workload_kind"`
	IdempotencyKey string                       `json:"idempotency_key"`
	Deadline       string                       `json:"deadline"`
	Policy         runtimePolicy                `json:"policy"`
	Entrypoint     string                       `json:"entrypoint"`
	Args           []string                     `json:"args"`
	Env            map[string]string            `json:"env"`
	Stdin          []byte                       `json:"stdin"`
	Files          []infrasandbox.FileReference `json:"files"`
	Artifacts      []artifactReference          `json:"artifact_references"`
}

type runtimePolicy struct {
	TimeoutSeconds          int                           `json:"timeout_seconds"`
	MemoryLimitMB           int                           `json:"memory_limit_mb"`
	CPULimit                float64                       `json:"cpu_limit"`
	MaxOutputBytes          int64                         `json:"max_output_bytes"`
	MaxConcurrency          int                           `json:"max_concurrency"`
	AllowNetwork            bool                          `json:"allow_network"`
	NetworkAllowlist        []string                      `json:"network_allowlist"`
	AllowedEnvNames         []string                      `json:"allowed_env_names"`
	VirtualReadPrefixes     []string                      `json:"virtual_read_prefixes"`
	VirtualWritePrefixes    []string                      `json:"virtual_write_prefixes"`
	AllowedExecutables      []string                      `json:"allowed_executables"`
	FFIEnabled              bool                          `json:"ffi_enabled"`
	NodeModulesMode         domainsandbox.NodeModulesMode `json:"node_modules_mode"`
	NodeModulesDirectoryRef string                        `json:"node_modules_directory_ref"`
}

type artifactReference struct {
	Direction string `json:"direction"`
	URL       string `json:"url"`
	Digest    string `json:"digest"`
	Size      int64  `json:"size"`
	MediaType string `json:"media_type"`
	ExpiresAt string `json:"expires_at"`
}

type lookupWireRequest struct {
	Schema        string                    `json:"schema"`
	Scope         domainsandbox.Scope       `json:"scope"`
	WorkloadKind  infrasandbox.WorkloadKind `json:"workload_kind"`
	OperationID   string                    `json:"operation_id"`
	RequestDigest string                    `json:"request_digest"`
	SpaceID       string                    `json:"space_id"`
	ProjectID     string                    `json:"project_id"`
	Generation    uint64                    `json:"generation"`
}

type buildWireRequest struct {
	Schema      string `json:"schema"`
	OperationID string `json:"operation_id"`
}

type artifactPublishWireRequest struct {
	Schema      string                          `json:"schema"`
	Descriptor  infrasandbox.ArtifactDescriptor `json:"descriptor"`
	UploadURL   string                          `json:"upload_url"`
	UploadToken string                          `json:"upload_token"`
	ExpiresAt   string                          `json:"expires_at"`
}

type controlRequest struct {
	Lookup   *infrasandbox.ExecutionLookupRequest
	BuildID  string
	Artifact *infrasandbox.ArtifactPublishRequest
}

func parseExecute(body []byte) (ExecuteCommand, error) {
	if len(body) == 0 || len(body) > maxRequestBytes {
		return ExecuteCommand{}, ErrProtocol
	}
	var wire executeWireRequest
	if err := decodeStrictJSON(body, &wire); err != nil || wire.Schema != infrasandbox.ExecuteSchemaV1 ||
		!validIdentifier(wire.IdempotencyKey) || wire.Entrypoint == "" ||
		!scopeMatchesWorkloadEntrypoint(wire.Scope, wire.WorkloadKind, wire.Entrypoint) {
		return ExecuteCommand{}, ErrProtocol
	}
	deadline, err := time.Parse(time.RFC3339Nano, wire.Deadline)
	if err != nil || !deadline.After(time.Now().UTC()) || deadline.After(time.Now().UTC().Add(infrasandbox.MaxExecutionDeadlineAhead)) || !validRuntimePolicy(wire.Policy) ||
		!validEntrypoint(wire.Entrypoint) || !validExecutePayload(wire, deadline.UTC()) {
		return ExecuteCommand{}, ErrProtocol
	}
	return ExecuteCommand{Scope: wire.Scope, WorkloadKind: wire.WorkloadKind, IdempotencyKey: wire.IdempotencyKey, Deadline: deadline.UTC(), Entrypoint: wire.Entrypoint, RawBody: append([]byte(nil), body...)}, nil
}

func validExecutePayload(wire executeWireRequest, deadline time.Time) bool {
	if len(wire.Args) > infrasandbox.MaxArgs || len(wire.Env) > infrasandbox.MaxEnvVars || len(wire.Stdin) > infrasandbox.MaxStdinBytes ||
		len(wire.Files) > infrasandbox.MaxFiles || len(wire.Artifacts) > infrasandbox.MaxArtifacts {
		return false
	}
	for _, argument := range wire.Args {
		if !validBoundedText(argument, infrasandbox.MaxArgumentBytes, false) {
			return false
		}
	}
	for key, value := range wire.Env {
		if !validEnvironmentKey(key) || !validBoundedText(value, infrasandbox.MaxEnvValueBytes, false) {
			return false
		}
	}
	fileIDs, filePaths := make(map[string]struct{}, len(wire.Files)), make(map[string]struct{}, len(wire.Files))
	for _, file := range wire.Files {
		if !validIdentifier(file.ID) || !validLogicalPath(file.Path) || !validDigest(file.Digest) || file.Size < 0 || file.Size > infrasandbox.MaxLogicalFileSizeBytes {
			return false
		}
		if _, exists := fileIDs[file.ID]; exists {
			return false
		}
		if _, exists := filePaths[file.Path]; exists {
			return false
		}
		fileIDs[file.ID], filePaths[file.Path] = struct{}{}, struct{}{}
	}
	for _, artifact := range wire.Artifacts {
		expiresAt, err := time.Parse(time.RFC3339Nano, artifact.ExpiresAt)
		if err != nil || (artifact.Direction != string(infrasandbox.ArtifactDirectionDownload) && artifact.Direction != string(infrasandbox.ArtifactDirectionUpload)) ||
			!validArtifactURL(artifact.URL) || !validDigest(artifact.Digest) || artifact.Size < 0 || artifact.Size > infrasandbox.MaxLogicalFileSizeBytes ||
			!validMediaType(artifact.MediaType) || !expiresAt.After(time.Now().UTC()) || expiresAt.After(deadline) {
			return false
		}
	}
	return true
}

func parseControlRequest(path string, body []byte, now time.Time) (controlRequest, error) {
	if strings.HasSuffix(path, ":keep-alive") || strings.HasSuffix(path, ":cancel") {
		var empty struct{}
		if err := decodeStrictJSON(body, &empty); err != nil {
			return controlRequest{}, ErrProtocol
		}
		return controlRequest{}, nil
	}
	if path == "/v1/executions:lookup" {
		var wire lookupWireRequest
		if err := decodeStrictJSON(body, &wire); err != nil || !validIdentifier(wire.OperationID) ||
			!scopeMatchesWorkload(wire.Scope, wire.WorkloadKind) {
			return controlRequest{}, ErrProtocol
		}
		request := infrasandbox.ExecutionLookupRequest{
			Scope: wire.Scope, WorkloadKind: wire.WorkloadKind, OperationID: wire.OperationID,
		}
		switch wire.Schema {
		case infrasandbox.ExecutionLookupSchemaV1:
			if wire.SpaceID != "" || wire.ProjectID != "" || wire.Generation != 0 {
				return controlRequest{}, ErrProtocol
			}
			digest, err := hex.DecodeString(wire.RequestDigest)
			if err != nil {
				return controlRequest{}, ErrProtocol
			}
			request.RequestDigest, err = infrasandbox.ExecutionRequestDigestFromBytes(digest)
			if err != nil {
				return controlRequest{}, ErrProtocol
			}
		case infrasandbox.ExecutionLookupLegacySchemaV1:
			if wire.RequestDigest != "" {
				return controlRequest{}, ErrProtocol
			}
			request.Legacy, request.SpaceID, request.ProjectID, request.Generation = true, wire.SpaceID, wire.ProjectID, wire.Generation
		default:
			return controlRequest{}, ErrProtocol
		}
		if request, err := infrasandbox.NormalizeExecutionLookupRequest(request); err != nil {
			return controlRequest{}, ErrProtocol
		} else {
			return controlRequest{Lookup: &request}, nil
		}
	}
	if strings.HasSuffix(path, "/builds") {
		var wire buildWireRequest
		if err := decodeStrictJSON(body, &wire); err != nil || wire.Schema != infrasandbox.AppDevBuildSchemaV1 || !validBuildOperationID(wire.OperationID) {
			return controlRequest{}, ErrProtocol
		}
		return controlRequest{BuildID: wire.OperationID}, nil
	}
	if strings.HasSuffix(path, "/artifacts:publish") {
		var wire artifactPublishWireRequest
		if err := decodeStrictJSON(body, &wire); err != nil || wire.Schema != infrasandbox.ArtifactPublishSchemaV1 || !validCredential(wire.UploadToken) {
			return controlRequest{}, ErrProtocol
		}
		descriptor, err := infrasandbox.NormalizeArtifactDescriptor(wire.Descriptor)
		if err != nil {
			return controlRequest{}, ErrProtocol
		}
		endpoint, err := url.Parse(wire.UploadURL)
		if err != nil || !validArtifactURL(wire.UploadURL) || endpoint.Scheme != "https" || endpoint.Host == "" || endpoint.User != nil {
			return controlRequest{}, ErrProtocol
		}
		expiresAt, err := time.Parse(time.RFC3339Nano, wire.ExpiresAt)
		if err != nil || !expiresAt.After(now) {
			return controlRequest{}, ErrProtocol
		}
		return controlRequest{Artifact: &infrasandbox.ArtifactPublishRequest{
			Descriptor: descriptor, UploadURL: wire.UploadURL, Token: wire.UploadToken, ExpiresAt: expiresAt.UTC(),
		}}, nil
	}
	return controlRequest{}, ErrProtocol
}

func decodeStrictJSON(body []byte, output any) error {
	if err := rejectDuplicateJSONKeys(body); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(output); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return ErrProtocol
	}
	return nil
}

func rejectDuplicateJSONKeys(body []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	var scan func() error
	scan = func() error {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		delimiter, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		switch delimiter {
		case '{':
			seen := map[string]struct{}{}
			for decoder.More() {
				key, keyErr := decoder.Token()
				if keyErr != nil {
					return keyErr
				}
				value, ok := key.(string)
				if !ok {
					return ErrProtocol
				}
				if _, exists := seen[value]; exists {
					return ErrProtocol
				}
				seen[value] = struct{}{}
				if err := scan(); err != nil {
					return err
				}
			}
		case '[':
			for decoder.More() {
				if err := scan(); err != nil {
					return err
				}
			}
		}
		_, err = decoder.Token()
		return err
	}
	if err := scan(); err != nil {
		return err
	}
	_, err := decoder.Token()
	if err != io.EOF {
		return ErrProtocol
	}
	return nil
}

func scopeMatchesWorkload(scope domainsandbox.Scope, workload infrasandbox.WorkloadKind) bool {
	return (scope == domainsandbox.ScopeAgent && workload == infrasandbox.WorkloadAgent) ||
		(scope == domainsandbox.ScopeAppDev && workload == infrasandbox.WorkloadAppDev) ||
		(scope == domainsandbox.ScopeMCPStdio && workload == infrasandbox.WorkloadMCPStdio) ||
		(scope == domainsandbox.ScopePlugin && workload == infrasandbox.WorkloadPlugin)
}

func scopeMatchesWorkloadEntrypoint(scope domainsandbox.Scope, workload infrasandbox.WorkloadKind, entrypoint string) bool {
	return scopeMatchesWorkload(scope, workload) &&
		(scope != domainsandbox.ScopePlugin || entrypoint == infrasandbox.PluginCodeRunnerEntrypoint)
}

func validExecutionID(value string) bool {
	return validIdentifier(value)
}

func validIdentifier(value string) bool {
	if len(value) == 0 || len(value) > 128 || !isIdentifierAlphaNumeric(value[0]) {
		return false
	}
	for index := 1; index < len(value); index++ {
		character := value[index]
		if !isIdentifierAlphaNumeric(character) && character != '.' && character != '_' && character != '-' && character != ':' {
			return false
		}
	}
	return true
}

func isIdentifierAlphaNumeric(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' || value >= '0' && value <= '9'
}

func validRuntimePolicy(value runtimePolicy) bool {
	_, err := domainsandbox.NormalizeRuntimePolicy(domainsandbox.RuntimePolicy{
		TimeoutSeconds: value.TimeoutSeconds, MemoryLimitMB: value.MemoryLimitMB, CPULimit: value.CPULimit,
		MaxOutputBytes: value.MaxOutputBytes, MaxConcurrency: value.MaxConcurrency, AllowNetwork: value.AllowNetwork,
		NetworkAllowlist: value.NetworkAllowlist, AllowedEnvNames: value.AllowedEnvNames,
		VirtualReadPrefixes: value.VirtualReadPrefixes, VirtualWritePrefixes: value.VirtualWritePrefixes,
		AllowedExecutables: value.AllowedExecutables, FFIEnabled: value.FFIEnabled,
		NodeModulesMode: value.NodeModulesMode, NodeModulesDirectoryRef: value.NodeModulesDirectoryRef,
	})
	return err == nil
}

func validEntrypoint(value string) bool {
	return validLogicalPath(value)
}

func validLogicalPath(value string) bool {
	if len(value) == 0 || len(value) > infrasandbox.MaxLogicalPathBytes || !utf8.ValidString(value) || path.IsAbs(value) ||
		path.Clean(value) != value || value == "." || strings.ContainsAny(value, "\\:\x00") {
		return false
	}
	for _, segment := range strings.Split(value, "/") {
		if segment == "" || segment == "." || segment == ".." || !validBoundedText(segment, infrasandbox.MaxLogicalPathBytes, false) {
			return false
		}
	}
	return true
}

func validDigest(value string) bool {
	if len(value) != infrasandbox.MaxDigestBytes || !strings.HasPrefix(value, "sha256:") {
		return false
	}
	for index := len("sha256:"); index < len(value); index++ {
		if value[index] < '0' || value[index] > '9' {
			if value[index] < 'a' || value[index] > 'f' {
				return false
			}
		}
	}
	return true
}

func validEnvironmentKey(value string) bool {
	if len(value) == 0 || len(value) > infrasandbox.MaxEnvKeyBytes || !isASCIIAlpha(value[0]) && value[0] != '_' {
		return false
	}
	for index := 1; index < len(value); index++ {
		if !isASCIIAlpha(value[index]) && (value[index] < '0' || value[index] > '9') && value[index] != '_' {
			return false
		}
	}
	return true
}

func isASCIIAlpha(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z'
}

func validBoundedText(value string, maximum int, allowLineBreaks bool) bool {
	if len(value) > maximum || !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if character == 0 || character == 0x7f || character < 0x20 && !(allowLineBreaks && (character == '\n' || character == '\r' || character == '\t')) {
			return false
		}
	}
	return true
}

func validArtifactURL(value string) bool {
	if value == "" || len(value) > infrasandbox.MaxArtifactReferenceURLBytes || !validBoundedText(value, infrasandbox.MaxArtifactReferenceURLBytes, false) {
		return false
	}
	parsed, err := url.Parse(value)
	return err == nil && parsed.Scheme == "https" && parsed.Host != "" && parsed.User == nil && parsed.Fragment == "" && parsed.RawQuery == "" && parsed.Path != ""
}

func validBuildOperationID(value string) bool {
	return value != "" && value == strings.TrimSpace(value) && len(value) <= infrasandbox.MaxIdentifierBytes &&
		value != "." && value != ".." && utf8.ValidString(value) && !strings.ContainsAny(value, "/\\?#%\x00\r\n\t")
}

func validCredential(value string) bool {
	if len(value) == 0 || len(value) > infrasandbox.MaxProviderCredentialBytes {
		return false
	}
	for index := range value {
		if value[index] < 0x21 || value[index] > 0x7e {
			return false
		}
	}
	return true
}

func validMediaType(value string) bool {
	if value == "" || len(value) > infrasandbox.MaxMediaTypeBytes || !validBoundedText(value, infrasandbox.MaxMediaTypeBytes, false) {
		return false
	}
	mediaType, _, err := mime.ParseMediaType(value)
	return err == nil && strings.Contains(mediaType, "/")
}
