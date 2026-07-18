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

type providerPO struct {
	ID                         uint64     `gorm:"column:id;type:bigint unsigned;primaryKey;autoIncrement;index:idx_sandbox_providers_created_id,priority:2;index:idx_sandbox_providers_updated_id,priority:2"`
	ProviderKey                string     `gorm:"column:provider_key;size:64;not null;uniqueIndex:uk_sandbox_providers_provider_key"`
	Name                       string     `gorm:"column:name;size:128;not null"`
	ProviderType               string     `gorm:"column:provider_type;size:32;not null"`
	EndpointSecret             *string    `gorm:"column:endpoint_secret;type:text"`
	EndpointHint               string     `gorm:"column:endpoint_hint;size:255;not null;default:''"`
	CredentialSecret           *string    `gorm:"column:credential_secret;type:text"`
	CredentialFingerprint      string     `gorm:"column:credential_fingerprint;size:32;not null;default:''"`
	ScopesJSON                 string     `gorm:"column:scopes_json;type:json;not null"`
	PolicyJSON                 string     `gorm:"column:policy_json;type:json;not null"`
	MaxConcurrency             uint32     `gorm:"column:max_concurrency;type:int unsigned;not null;default:1"`
	Status                     string     `gorm:"column:status;size:32;not null;index:idx_sandbox_providers_status_deleted,priority:1"`
	HealthStatus               string     `gorm:"column:health_status;size:32;not null"`
	LastHealthCapabilitiesJSON string     `gorm:"column:last_health_capabilities_json;type:json;not null"`
	LastHealthCode             string     `gorm:"column:last_health_code;size:64;not null;default:''"`
	LastHealthMessage          string     `gorm:"column:last_health_message;size:255;not null;default:''"`
	LastHealthLatencyMS        uint32     `gorm:"column:last_health_latency_ms;type:int unsigned;not null;default:0"`
	LastHealthAt               *time.Time `gorm:"column:last_health_at"`
	LegacySourceHash           *string    `gorm:"column:legacy_source_hash;size:64;uniqueIndex:uk_sandbox_providers_legacy_source_hash"`
	Version                    uint64     `gorm:"column:version;type:bigint unsigned;not null;default:1"`
	CreatedBy                  uint64     `gorm:"column:created_by;type:bigint unsigned;not null"`
	UpdatedBy                  uint64     `gorm:"column:updated_by;type:bigint unsigned;not null"`
	CreatedAt                  time.Time  `gorm:"column:created_at;not null;index:idx_sandbox_providers_created_id,priority:1"`
	UpdatedAt                  time.Time  `gorm:"column:updated_at;not null;index:idx_sandbox_providers_updated_id,priority:1"`
	DeletedAt                  *time.Time `gorm:"column:deleted_at;index:idx_sandbox_providers_status_deleted,priority:2"`
}

func (providerPO) TableName() string {
	return "sandbox_providers"
}

type providerDefaultPO struct {
	Scope      string    `gorm:"column:scope;size:32;primaryKey"`
	ProviderID uint64    `gorm:"column:provider_id;type:bigint unsigned;not null;index:idx_sandbox_provider_defaults_provider_id"`
	Version    uint64    `gorm:"column:version;type:bigint unsigned;not null;default:1"`
	UpdatedBy  uint64    `gorm:"column:updated_by;type:bigint unsigned;not null"`
	CreatedAt  time.Time `gorm:"column:created_at;not null"`
	UpdatedAt  time.Time `gorm:"column:updated_at;not null"`
}

func (providerDefaultPO) TableName() string {
	return "sandbox_provider_defaults"
}

type providerAuditEventPO struct {
	EventID      uint64    `gorm:"column:event_id;type:bigint unsigned;primaryKey;autoIncrement;index:idx_sandbox_audit_provider_created_event,priority:3;index:idx_sandbox_audit_created_event,priority:2;index:idx_sandbox_audit_action_created_event,priority:3;index:idx_sandbox_audit_result_created_event,priority:3"`
	ProviderID   *uint64   `gorm:"column:provider_id;type:bigint unsigned;index:idx_sandbox_audit_provider_created_event,priority:1"`
	ActorUserID  uint64    `gorm:"column:actor_user_id;type:bigint unsigned;not null"`
	Action       string    `gorm:"column:action;size:64;not null;index:idx_sandbox_audit_action_created_event,priority:1"`
	Result       string    `gorm:"column:result;size:64;not null;index:idx_sandbox_audit_result_created_event,priority:1"`
	RequestID    string    `gorm:"column:request_id;size:128;not null"`
	MetadataJSON string    `gorm:"column:metadata_json;type:json;not null"`
	CreatedAt    time.Time `gorm:"column:created_at;not null;index:idx_sandbox_audit_provider_created_event,priority:2;index:idx_sandbox_audit_created_event,priority:1;index:idx_sandbox_audit_action_created_event,priority:2;index:idx_sandbox_audit_result_created_event,priority:2"`
}

func (providerAuditEventPO) TableName() string {
	return "sandbox_provider_audit_events"
}
