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
	"strings"

	domainentity "github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	domainservice "github.com/coze-dev/coze-studio/backend/domain/agentthread/service"
	"github.com/coze-dev/coze-studio/backend/pkg/envkey"
)

const agentArtifactScanOutageFailModeEnv = "AGENT_ARTIFACT_SCAN_OUTAGE_FAIL_MODE"

type ArtifactScanOutageFailMode string

const (
	ArtifactScanOutageFailModeClosed            ArtifactScanOutageFailMode = "closed"
	ArtifactScanOutageFailModeOpenNonExecutable ArtifactScanOutageFailMode = "open_non_executable"
)

type ArtifactScanReadPolicyConfig struct {
	OutageFailMode ArtifactScanOutageFailMode
}

func NewArtifactScanReadPolicyConfigFromEnv() ArtifactScanReadPolicyConfig {
	return ArtifactScanReadPolicyConfig{
		OutageFailMode: normalizeArtifactScanOutageFailMode(
			envkey.GetStringD(agentArtifactScanOutageFailModeEnv, ""),
		),
	}
}

func normalizeArtifactScanOutageFailMode(value string) ArtifactScanOutageFailMode {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case string(ArtifactScanOutageFailModeOpenNonExecutable):
		return ArtifactScanOutageFailModeOpenNonExecutable
	default:
		return ArtifactScanOutageFailModeClosed
	}
}

func normalizeArtifactScanReadPolicyConfig(
	config ArtifactScanReadPolicyConfig,
) ArtifactScanReadPolicyConfig {
	config.OutageFailMode = normalizeArtifactScanOutageFailMode(
		string(config.OutageFailMode),
	)

	return config
}

func artifactScanReadPolicy(
	status artifactScanStatus,
	artifact *domainentity.AgentArtifact,
	config ArtifactScanReadPolicyConfig,
) artifactScanReadPolicyDecision {
	decision := artifactScanBaseReadPolicy(status)
	if decision.Allowed {
		return decision
	}
	config = normalizeArtifactScanReadPolicyConfig(config)
	if config.OutageFailMode != ArtifactScanOutageFailModeOpenNonExecutable {
		return decision
	}
	if !isArtifactScanOutageStatus(status) || !isArtifactNonExecutableForScanOutage(artifact) {
		return decision
	}
	decision.Allowed = true
	decision.Override = true
	decision.FailMode = string(config.OutageFailMode)

	return decision
}

func artifactScanBaseReadPolicy(status artifactScanStatus) artifactScanReadPolicyDecision {
	switch status {
	case artifactScanStatusClean:
		return artifactScanReadPolicyDecision{Allowed: true}
	case artifactScanStatusPending:
		return artifactScanReadPolicyDecision{Reason: "scan_pending"}
	case artifactScanStatusFailed:
		return artifactScanReadPolicyDecision{Reason: "scan_failed"}
	case artifactScanStatusBlocked:
		return artifactScanReadPolicyDecision{Reason: "scan_blocked"}
	case artifactScanStatusInfected:
		return artifactScanReadPolicyDecision{Reason: "scan_infected"}
	case artifactScanStatusQuarantined:
		return artifactScanReadPolicyDecision{Reason: "scan_quarantined"}
	default:
		return artifactScanReadPolicyDecision{Reason: "scan_unknown"}
	}
}

func isArtifactScanOutageStatus(status artifactScanStatus) bool {
	switch status {
	case artifactScanStatusUnknown, artifactScanStatusPending, artifactScanStatusFailed:
		return true
	default:
		return false
	}
}

func isArtifactNonExecutableForScanOutage(artifact *domainentity.AgentArtifact) bool {
	if artifact == nil || strings.TrimSpace(artifact.DetectedContentType) == "" ||
		artifact.ScannedSizeBytes == nil || *artifact.ScannedSizeBytes <= 0 ||
		!validApplicationArtifactHash(artifact.ContentHash) {
		return false
	}
	determined := domainservice.DetermineArtifactPreviewMode(artifact.DetectedContentType)
	switch artifact.PreviewMode {
	case domainentity.AgentArtifactPreviewModeText:
		return determined == domainentity.AgentArtifactPreviewModeText
	case domainentity.AgentArtifactPreviewModeImage:
		return determined == domainentity.AgentArtifactPreviewModeImage
	default:
		return false
	}
}
