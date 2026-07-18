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

package sandbox

import "time"

type Scope string

const (
	ScopeAgent    Scope = "agent"
	ScopeMCPStdio Scope = "mcp_stdio"
	ScopeAppDev   Scope = "appdev"
)

type ProviderType string

const (
	ProviderTypeRemoteHTTP ProviderType = "remote_http"
	ProviderTypeLocalDebug ProviderType = "local_debug"
)

type ProviderStatus string

const (
	ProviderStatusDisabled ProviderStatus = "disabled"
	ProviderStatusEnabled  ProviderStatus = "enabled"
)

type HealthStatus string

const (
	HealthStatusUnknown   HealthStatus = "unknown"
	HealthStatusHealthy   HealthStatus = "healthy"
	HealthStatusDegraded  HealthStatus = "degraded"
	HealthStatusUnhealthy HealthStatus = "unhealthy"
)

type NodeModulesMode string

const (
	NodeModulesModeDisabled          NodeModulesMode = "disabled"
	NodeModulesModeApprovedDirectory NodeModulesMode = "approved_directory"
)

type Provider struct {
	ID                    int64
	ProviderKey           string
	Name                  string
	Type                  ProviderType
	EndpointSecret        string
	EndpointHint          string
	CredentialSecret      string
	CredentialFingerprint string
	Scopes                []Scope
	Policy                RuntimePolicy
	Status                ProviderStatus
	Health                HealthSnapshot
	LegacySourceHash      string
	Version               uint64
	CreatedBy             int64
	UpdatedBy             int64
	CreatedAt             time.Time
	UpdatedAt             time.Time
	DeletedAt             *time.Time
}

// CreateProviderInput intentionally omits repository-owned identity, version,
// timestamps, deletion state, status, and health.
type CreateProviderInput struct {
	ProviderKey           string
	Name                  string
	Type                  ProviderType
	EndpointSecret        string
	EndpointHint          string
	CredentialSecret      string
	CredentialFingerprint string
	Scopes                []Scope
	Policy                RuntimePolicy
	LegacySourceHash      string
	ActorUserID           int64
}

// UpdateProviderInput intentionally omits ProviderKey and lifecycle fields.
// ProviderID and ExpectedVersion identify one compare-and-swap mutation.
type UpdateProviderInput struct {
	ProviderID            int64
	ExpectedVersion       uint64
	Name                  string
	Type                  ProviderType
	EndpointSecret        string
	EndpointHint          string
	CredentialSecret      string
	CredentialFingerprint string
	Scopes                []Scope
	Policy                RuntimePolicy
	ResetHealth           bool
	ActorUserID           int64
}

type UpdateProviderStatusInput struct {
	ProviderID      int64
	ExpectedVersion uint64
	Status          ProviderStatus
	ActorUserID     int64
}

type UpdateProviderHealthInput struct {
	ProviderID      int64
	ExpectedVersion uint64
	Health          HealthSnapshot
	ActorUserID     int64
}

type DeleteProviderInput struct {
	ProviderID      int64
	ExpectedVersion uint64
	ActorUserID     int64
}

type ProviderDefault struct {
	Scope      Scope
	ProviderID int64
	Version    uint64
	UpdatedBy  int64
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type SetProviderDefaultInput struct {
	Scope           Scope
	ProviderID      int64
	ExpectedVersion uint64
	ActorUserID     int64
}

type ProviderAuditEvent struct {
	ID          int64
	ProviderID  int64
	ActorUserID int64
	Action      string
	Result      string
	RequestID   string
	Metadata    map[string]string
	CreatedAt   time.Time
}

// AppendProviderAuditEventInput omits repository-owned ID and CreatedAt.
type AppendProviderAuditEventInput struct {
	ProviderID  int64
	ActorUserID int64
	Action      string
	Result      string
	RequestID   string
	Metadata    map[string]string
}

type RuntimePolicy struct {
	TimeoutSeconds          int             `json:"timeout_seconds"`
	MemoryLimitMB           int             `json:"memory_limit_mb"`
	CPULimit                float64         `json:"cpu_limit"`
	MaxOutputBytes          int64           `json:"max_output_bytes"`
	MaxConcurrency          int             `json:"max_concurrency"`
	AllowNetwork            bool            `json:"allow_network"`
	NetworkAllowlist        []string        `json:"network_allowlist"`
	AllowedEnvNames         []string        `json:"allowed_env_names"`
	VirtualReadPrefixes     []string        `json:"virtual_read_prefixes"`
	VirtualWritePrefixes    []string        `json:"virtual_write_prefixes"`
	AllowedExecutables      []string        `json:"allowed_executables"`
	FFIEnabled              bool            `json:"ffi_enabled"`
	NodeModulesMode         NodeModulesMode `json:"node_modules_mode"`
	NodeModulesDirectoryRef string          `json:"node_modules_directory_ref"`

	// Legacy fields remain internal migration/runtime compatibility metadata.
	// Admin APIs never accept or project them.
	AllowEnv       []string `json:"-"`
	AllowRead      []string `json:"-"`
	AllowWrite     []string `json:"-"`
	AllowRun       []string `json:"-"`
	AllowFFI       []string `json:"-"`
	NodeModulesDir string   `json:"-"`
}

type HealthSnapshot struct {
	Status        HealthStatus
	Capabilities  []Scope
	ReasonCode    string
	Message       string
	LatencyMillis int64
	CheckedAt     time.Time
}
