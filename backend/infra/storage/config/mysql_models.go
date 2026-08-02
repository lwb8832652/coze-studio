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

package config

import "time"

type objectStorageConfigPO struct {
	ID                  uint64     `gorm:"column:id;type:bigint unsigned;primaryKey;autoIncrement"`
	Name                string     `gorm:"column:name;size:128;not null;uniqueIndex:uk_object_storage_configs_name"`
	ProviderType        string     `gorm:"column:provider_type;size:32;not null;index:idx_object_storage_configs_provider_type"`
	ConfigJSON          string     `gorm:"column:config_json;type:json;not null"`
	CredentialSecret    string     `gorm:"column:credential_secret;type:text;not null"`
	ActiveSlot          *uint8     `gorm:"column:active_slot;uniqueIndex:uk_object_storage_configs_active_slot"`
	HealthStatus        string     `gorm:"column:health_status;size:32;not null;default:'unknown'"`
	LastHealthCode      string     `gorm:"column:last_health_code;size:64;not null;default:''"`
	LastHealthMessage   string     `gorm:"column:last_health_message;size:255;not null;default:''"`
	LastHealthLatencyMS uint32     `gorm:"column:last_health_latency_ms;type:int unsigned;not null;default:0"`
	LastHealthAt        *time.Time `gorm:"column:last_health_at"`
	Version             uint64     `gorm:"column:version;type:bigint unsigned;not null;default:1"`
	RuntimeRevision     uint64     `gorm:"column:runtime_revision;type:bigint unsigned;not null;default:1"`
	CreatedAt           time.Time  `gorm:"column:created_at;not null"`
	UpdatedAt           time.Time  `gorm:"column:updated_at;not null"`
}

func (objectStorageConfigPO) TableName() string {
	return "object_storage_configs"
}
