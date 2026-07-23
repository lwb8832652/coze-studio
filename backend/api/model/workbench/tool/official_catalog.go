/*
 * Copyright 2025 coze-dev Authors
 * SPDX-License-Identifier: Apache-2.0
 */

package tool

type MCPOfficialInstallStatus string

const (
	MCPOfficialInstallStatusAvailable      MCPOfficialInstallStatus = "available"
	MCPOfficialInstallStatusInstalled      MCPOfficialInstallStatus = "installed"
	MCPOfficialInstallStatusNeedsMigration MCPOfficialInstallStatus = "needs_migration"
)

type MCPOfficialAvailability string

const (
	MCPOfficialAvailabilityInstallable     MCPOfficialAvailability = "installable"
	MCPOfficialAvailabilityAdapterRequired MCPOfficialAvailability = "adapter_required"
)

type MCPOfficialCredentialField struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Placeholder string `json:"placeholder"`
	Required    bool   `json:"required"`
	Secret      bool   `json:"secret"`
}

type MCPOfficialInstallation struct {
	ServerID        int64  `json:"server_id,string"`
	Enabled         bool   `json:"enabled"`
	HealthStatus    string `json:"health_status"`
	HealthCheckedAt int64  `json:"health_checked_at"`
	HealthLatencyMs int64  `json:"health_latency_ms"`
	UpdatedAt       int64  `json:"updated_at"`
}

// MCPOfficialCatalogEntry is intentionally connection-free. Official catalog
// responses must never expose runtime config, credentials or provider payloads.
type MCPOfficialCatalogEntry struct {
	CatalogID          string                        `json:"catalog_id"`
	Name               string                        `json:"name"`
	Description        string                        `json:"description"`
	IconURL            string                        `json:"icon_url"`
	Publisher          string                        `json:"publisher"`
	Source             string                        `json:"source"`
	ServerType         string                        `json:"server_type"`
	Tools              []*MCPToolDefinition          `json:"tools"`
	CredentialFields   []*MCPOfficialCredentialField `json:"credential_fields"`
	Availability       MCPOfficialAvailability       `json:"availability"`
	AvailabilityReason string                        `json:"availability_reason"`
	InstallStatus      MCPOfficialInstallStatus      `json:"install_status"`
	Installation       *MCPOfficialInstallation      `json:"installation,omitempty"`
}

type ListMCPOfficialCatalogRequest struct {
	SpaceID int64 `query:"space_id,required"`
}

type InstallMCPOfficialCatalogRequest struct {
	CatalogID   string            `path:"catalog_id,required" json:"-"`
	SpaceID     int64             `json:"space_id,string,required"`
	Credentials map[string]string `json:"credentials,required"`
}

type ListMCPOfficialCatalogData struct {
	Entries   []*MCPOfficialCatalogEntry `json:"entries"`
	Total     int64                      `json:"total"`
	CanManage bool                       `json:"can_manage"`
}

type ListMCPOfficialCatalogResponse struct {
	Data *ListMCPOfficialCatalogData `json:"data,omitempty"`
	Code int64                       `json:"code"`
	Msg  string                      `json:"msg"`
}
