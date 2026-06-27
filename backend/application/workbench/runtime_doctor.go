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

package workbench

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	diagnosticapi "github.com/coze-dev/coze-studio/backend/api/model/workbench/diagnostic"
	skillapi "github.com/coze-dev/coze-studio/backend/api/model/workbench/skill"
	toolapi "github.com/coze-dev/coze-studio/backend/api/model/workbench/tool"
	appagentthread "github.com/coze-dev/coze-studio/backend/application/agentthread"
	appmcptool "github.com/coze-dev/coze-studio/backend/application/mcptool"
	appskill "github.com/coze-dev/coze-studio/backend/application/skill"
)

const (
	runtimeDoctorStatusReady    = "ready"
	runtimeDoctorStatusWarning  = "warning"
	runtimeDoctorStatusError    = "error"
	runtimeDoctorStatusDisabled = "disabled"
)

type workbenchMCPToolDiagnosticService interface {
	ListServers(
		ctx context.Context,
		req *toolapi.ListMCPToolServersRequest,
	) (*toolapi.ListMCPToolServersResponse, error)
}

type workbenchSkillDiagnosticService interface {
	ListSkills(
		ctx context.Context,
		req *skillapi.ListSkillsRequest,
	) (*skillapi.ListSkillsResponse, error)
}

func (s *ApplicationService) GetRuntimeDoctor(
	ctx context.Context,
	req *diagnosticapi.GetWorkbenchRuntimeDoctorRequest,
) (*diagnosticapi.WorkbenchRuntimeDoctorResponse, error) {
	if req == nil || req.SpaceID <= 0 {
		return nil, InvalidArgumentErrorf("space_id is required")
	}

	checks := make([]*diagnosticapi.RuntimeDoctorCheck, 0, 6)
	runtimeData, runtimeCheck := runtimeDoctorRuntimeData()
	checks = append(checks, runtimeCheck)

	var chatModelProvider chatModelProvider
	if s != nil {
		chatModelProvider = s.chatModelProvider
	}
	checks = append(checks, runtimeDoctorModelCheck(ctx, chatModelProvider))
	checks = append(checks, runtimeDoctorSkillCheck(ctx, s.skillDiagnosticService(), req.SpaceID))

	webTools, webChecks := runtimeDoctorWebToolsData()
	checks = append(checks, webChecks...)

	mcpTools, mcpCheck, err := s.runtimeDoctorMCPToolsData(ctx, req.SpaceID)
	if err != nil {
		return nil, err
	}
	checks = append(checks, mcpCheck)

	return &diagnosticapi.WorkbenchRuntimeDoctorResponse{
		Code: 0,
		Msg:  "success",
		Data: &diagnosticapi.WorkbenchRuntimeDoctorData{
			Status:   runtimeDoctorOverallStatus(checks),
			Runtime:  runtimeData,
			WebTools: webTools,
			MCPTools: mcpTools,
			Checks:   checks,
		},
	}, nil
}

func runtimeDoctorRuntimeData() (
	*diagnosticapi.RuntimeDoctorRuntimeData,
	*diagnosticapi.RuntimeDoctorCheck,
) {
	policy, err := appagentthread.RuntimePolicyFromEnv()
	if err != nil {
		return &diagnosticapi.RuntimeDoctorRuntimeData{},
			&diagnosticapi.RuntimeDoctorCheck{
				Name:     "runtime.eino_adk",
				Category: "runtime",
				Status:   runtimeDoctorStatusError,
				Message:  boundedRuntimeDoctorMessage(err.Error()),
			}
	}

	status := runtimeDoctorStatusDisabled
	message := "Eino ADK runtime is disabled"
	if policy.EinoADKEnabled {
		status = runtimeDoctorStatusReady
		message = "Eino ADK runtime is enabled"
	}

	return &diagnosticapi.RuntimeDoctorRuntimeData{
			DefaultMode:    string(policy.DefaultMode),
			EinoADKEnabled: policy.EinoADKEnabled,
		},
		&diagnosticapi.RuntimeDoctorCheck{
			Name:     "runtime.eino_adk",
			Category: "runtime",
			Status:   status,
			Message:  message,
		}
}

func runtimeDoctorModelCheck(
	ctx context.Context,
	provider chatModelProvider,
) (check *diagnosticapi.RuntimeDoctorCheck) {
	defer func() {
		if recovered := recover(); recovered != nil {
			check = &diagnosticapi.RuntimeDoctorCheck{
				Name:     "model.default",
				Category: "model",
				Status:   runtimeDoctorStatusError,
				Message:  boundedRuntimeDoctorMessage(fmt.Sprintf("model provider panic: %v", recovered)),
			}
		}
	}()

	if provider == nil {
		provider = defaultChatModelProvider
	}

	chatModel, configured, err := provider(ctx, 0)
	if err != nil {
		return &diagnosticapi.RuntimeDoctorCheck{
			Name:     "model.default",
			Category: "model",
			Status:   runtimeDoctorStatusError,
			Message:  boundedRuntimeDoctorMessage(err.Error()),
		}
	}
	if !configured || chatModel == nil {
		return &diagnosticapi.RuntimeDoctorCheck{
			Name:     "model.default",
			Category: "model",
			Status:   runtimeDoctorStatusDisabled,
			Message:  "Workbench default chat model is not configured",
		}
	}

	return &diagnosticapi.RuntimeDoctorCheck{
		Name:     "model.default",
		Category: "model",
		Status:   runtimeDoctorStatusReady,
		Message:  "Workbench default chat model is configured",
	}
}

func runtimeDoctorSkillCheck(
	ctx context.Context,
	svc workbenchSkillDiagnosticService,
	spaceID int64,
) *diagnosticapi.RuntimeDoctorCheck {
	check := &diagnosticapi.RuntimeDoctorCheck{
		Name:     "skills.runtime_catalog",
		Category: "skills",
	}
	if svc == nil {
		check.Status = runtimeDoctorStatusDisabled
		check.Message = "Skill service is not configured"
		return check
	}

	resp, err := svc.ListSkills(ctx, &skillapi.ListSkillsRequest{SpaceID: spaceID})
	if err != nil {
		if isRuntimeDoctorServiceUninitialized(err) {
			check.Status = runtimeDoctorStatusDisabled
			check.Message = "Skill service is not configured"
			return check
		}
		check.Status = runtimeDoctorStatusError
		check.Message = boundedRuntimeDoctorMessage(err.Error())
		return check
	}

	var skills []*skillapi.Skill
	if resp != nil && resp.Data != nil {
		skills = resp.Data.Skills
	}
	total := 0
	enabled := 0
	enabledNames := make([]string, 0, 3)
	for _, item := range skills {
		if item == nil {
			continue
		}
		total++
		if !item.Enabled {
			continue
		}
		enabled++
		if len(enabledNames) < 3 {
			if name := safeRuntimeDoctorName(item.Name); name != "" {
				enabledNames = append(enabledNames, name)
			}
		}
	}

	switch {
	case total == 0:
		check.Status = runtimeDoctorStatusDisabled
		check.Message = "0 skills configured"
	case enabled == 0:
		check.Status = runtimeDoctorStatusWarning
		check.Message = fmt.Sprintf("0 enabled / %d total skills", total)
	default:
		check.Status = runtimeDoctorStatusReady
		check.Message = fmt.Sprintf("%d enabled / %d total skills", enabled, total)
		if len(enabledNames) > 0 {
			check.Message = fmt.Sprintf("%s: %s", check.Message, strings.Join(enabledNames, ", "))
		}
	}

	return check
}

func runtimeDoctorWebToolsData() (
	*diagnosticapi.RuntimeDoctorWebToolsData,
	[]*diagnosticapi.RuntimeDoctorCheck,
) {
	webFetchStatus := &diagnosticapi.RuntimeDoctorWebToolStatus{
		Status:     runtimeDoctorStatusReady,
		Configured: true,
		Message:    "web_fetch is available through per-run allowed-host policy",
	}
	webSearchStatus := &diagnosticapi.RuntimeDoctorWebToolStatus{
		Status:     runtimeDoctorStatusDisabled,
		Configured: false,
		Message:    "web_search backend is disabled",
	}
	if _, enabled, err := appagentthread.ADKWebSearchBackendFromEnv(); err != nil {
		webSearchStatus.Status = runtimeDoctorStatusError
		webSearchStatus.Configured = false
		webSearchStatus.Message = boundedRuntimeDoctorMessage(err.Error())
	} else if enabled {
		webSearchStatus.Status = runtimeDoctorStatusReady
		webSearchStatus.Configured = true
		webSearchStatus.Message = "web_search backend is configured"
	}

	return &diagnosticapi.RuntimeDoctorWebToolsData{
			WebFetch:  webFetchStatus,
			WebSearch: webSearchStatus,
		},
		[]*diagnosticapi.RuntimeDoctorCheck{
			{
				Name:     "web_tools.web_fetch",
				Category: "web_tools",
				Status:   webFetchStatus.Status,
				Message:  webFetchStatus.Message,
			},
			{
				Name:     "web_tools.web_search",
				Category: "web_tools",
				Status:   webSearchStatus.Status,
				Message:  webSearchStatus.Message,
			},
		}
}

func (s *ApplicationService) runtimeDoctorMCPToolsData(
	ctx context.Context,
	spaceID int64,
) (
	*diagnosticapi.RuntimeDoctorMCPToolsData,
	*diagnosticapi.RuntimeDoctorCheck,
	error,
) {
	mcpToolSVC := s.mcpToolDiagnosticService()
	resp, err := mcpToolSVC.ListServers(ctx, &toolapi.ListMCPToolServersRequest{
		SpaceID: spaceID,
	})
	if err != nil {
		return nil, nil, err
	}

	data := &diagnosticapi.RuntimeDoctorMCPToolsData{}
	if resp != nil && resp.Data != nil {
		data.TotalServers = resp.Data.Total
		for _, server := range resp.Data.Servers {
			if server == nil {
				continue
			}
			if server.Enabled {
				data.EnabledServers++
			}
			switch strings.TrimSpace(server.HealthStatus) {
			case "healthy":
				data.HealthyServers++
			case "unhealthy":
				data.UnhealthyServers++
			default:
				data.UnknownServers++
			}
		}
	}
	data.Status = runtimeDoctorMCPStatus(data)

	return data,
		&diagnosticapi.RuntimeDoctorCheck{
			Name:     "mcp_tools.runtime_health",
			Category: "mcp_tools",
			Status:   data.Status,
			Message: fmt.Sprintf(
				"%d healthy, %d unhealthy, %d unknown / %d total",
				data.HealthyServers,
				data.UnhealthyServers,
				data.UnknownServers,
				data.TotalServers,
			),
		},
		nil
}

func (s *ApplicationService) mcpToolDiagnosticService() workbenchMCPToolDiagnosticService {
	if s != nil && s.mcpToolSVC != nil {
		return s.mcpToolSVC
	}
	return appmcptool.SVC
}

func (s *ApplicationService) skillDiagnosticService() workbenchSkillDiagnosticService {
	if s != nil && s.skillSVC != nil {
		return s.skillSVC
	}
	return appskill.SVC
}

func runtimeDoctorMCPStatus(data *diagnosticapi.RuntimeDoctorMCPToolsData) string {
	if data == nil || data.TotalServers == 0 {
		return runtimeDoctorStatusDisabled
	}
	if data.UnhealthyServers > 0 {
		return runtimeDoctorStatusError
	}
	if data.UnknownServers > 0 {
		return runtimeDoctorStatusWarning
	}
	return runtimeDoctorStatusReady
}

func runtimeDoctorOverallStatus(checks []*diagnosticapi.RuntimeDoctorCheck) string {
	status := runtimeDoctorStatusReady
	for _, check := range checks {
		if check == nil {
			continue
		}
		switch check.Status {
		case runtimeDoctorStatusError:
			return runtimeDoctorStatusError
		case runtimeDoctorStatusWarning:
			status = runtimeDoctorStatusWarning
		case runtimeDoctorStatusDisabled:
			if status == runtimeDoctorStatusReady {
				status = runtimeDoctorStatusWarning
			}
		}
	}
	return status
}

var (
	runtimeDoctorSecretPattern = regexp.MustCompile(`(?i)(api[_-]?key|access[_-]?token|authorization|bearer|credential|secret|password|token)\s*[:=]\s*[^,\s;]+`)
	runtimeDoctorSensitiveName = regexp.MustCompile(`(?i)\b(api[_-]?key|access[_-]?token|authorization|bearer|credential|secret|password|token)\b`)
)

func boundedRuntimeDoctorMessage(value string) string {
	value = strings.TrimSpace(value)
	value = runtimeDoctorSecretPattern.ReplaceAllString(value, "[redacted]")
	if len([]rune(value)) <= 256 {
		return value
	}

	return string([]rune(value)[:256])
}

func isRuntimeDoctorServiceUninitialized(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "service is not initialized")
}

func safeRuntimeDoctorName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	var builder strings.Builder
	previousSpace := false
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' || r == '.' {
			builder.WriteRune(r)
			previousSpace = false
			continue
		}
		if !previousSpace {
			builder.WriteRune(' ')
			previousSpace = true
		}
	}

	normalized := strings.Join(strings.Fields(builder.String()), " ")
	if runtimeDoctorSensitiveName.MatchString(normalized) {
		return ""
	}

	runes := []rune(normalized)
	if len(runes) > 64 {
		normalized = string(runes[:64])
	}

	return normalized
}
