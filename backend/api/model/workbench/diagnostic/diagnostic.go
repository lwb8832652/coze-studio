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

package diagnostic

type GetWorkbenchRuntimeDoctorRequest struct {
	SpaceID int64 `query:"space_id,required"`
}

type RuntimeDoctorCheck struct {
	Name     string `json:"name"`
	Category string `json:"category"`
	Status   string `json:"status"`
	Message  string `json:"message,omitempty"`
}

type RuntimeDoctorRuntimeData struct {
	DefaultMode    string `json:"default_mode"`
	EinoADKEnabled bool   `json:"eino_adk_enabled"`
}

type RuntimeDoctorModelCapabilities struct {
	NativeToolSearch bool `json:"native_tool_search"`
	Thinking         bool `json:"thinking"`
	Reasoning        bool `json:"reasoning"`
	Vision           bool `json:"vision"`
	PDF              bool `json:"pdf"`
	File             bool `json:"file"`
	Audio            bool `json:"audio"`
	Video            bool `json:"video"`
}

type RuntimeDoctorModelData struct {
	Status       string                          `json:"status"`
	Configured   bool                            `json:"configured"`
	LiveProbe    string                          `json:"live_probe"`
	Capabilities *RuntimeDoctorModelCapabilities `json:"capabilities,omitempty"`
	Message      string                          `json:"message,omitempty"`
}

type RuntimeDoctorSandboxData struct {
	Status      string                           `json:"status"`
	RunnerType  string                           `json:"runner_type"`
	Network     string                           `json:"network"`
	Process     string                           `json:"process"`
	FFI         string                           `json:"ffi"`
	NodeModules string                           `json:"node_modules"`
	Message     string                           `json:"message,omitempty"`
	Scopes      []*RuntimeDoctorSandboxScopeData `json:"scopes,omitempty"`
}

type RuntimeDoctorSandboxScopeData struct {
	Scope        string `json:"scope"`
	Configured   bool   `json:"configured"`
	Available    bool   `json:"available"`
	Selected     bool   `json:"selected"`
	HealthStatus string `json:"health_status"`
	ReasonCode   string `json:"reason_code"`
	ProviderType string `json:"provider_type,omitempty"`
	ProviderRef  string `json:"provider_ref,omitempty"`
	CheckedAt    string `json:"checked_at,omitempty"`
}

type RuntimeDoctorWebToolStatus struct {
	Status     string `json:"status"`
	Configured bool   `json:"configured"`
	Message    string `json:"message,omitempty"`
}

type RuntimeDoctorWebToolsData struct {
	WebFetch  *RuntimeDoctorWebToolStatus `json:"web_fetch"`
	WebSearch *RuntimeDoctorWebToolStatus `json:"web_search"`
}

type RuntimeDoctorMCPToolsData struct {
	Status           string `json:"status"`
	TotalServers     int64  `json:"total_servers"`
	EnabledServers   int64  `json:"enabled_servers"`
	HealthyServers   int64  `json:"healthy_servers"`
	UnhealthyServers int64  `json:"unhealthy_servers"`
	UnknownServers   int64  `json:"unknown_servers"`
}

type WorkbenchRuntimeDoctorData struct {
	Status   string                     `json:"status"`
	Runtime  *RuntimeDoctorRuntimeData  `json:"runtime"`
	Model    *RuntimeDoctorModelData    `json:"model"`
	Sandbox  *RuntimeDoctorSandboxData  `json:"sandbox"`
	WebTools *RuntimeDoctorWebToolsData `json:"web_tools"`
	MCPTools *RuntimeDoctorMCPToolsData `json:"mcp_tools"`
	Checks   []*RuntimeDoctorCheck      `json:"checks"`
}

type WorkbenchRuntimeDoctorResponse struct {
	Data *WorkbenchRuntimeDoctorData `json:"data,omitempty"`
	Code int64                       `json:"code"`
	Msg  string                      `json:"msg"`
}
