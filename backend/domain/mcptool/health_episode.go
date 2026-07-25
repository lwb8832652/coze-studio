// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package mcptool

import (
	"fmt"
	"strings"
)

const (
	HealthStatusHealthy                = "healthy"
	HealthStatusUnhealthy              = "unhealthy"
	HealthEpisodeFailureThreshold      = 3
	HealthEpisodeNotificationNone      = HealthEpisodeNotification("")
	HealthEpisodeNotificationDegraded  = HealthEpisodeNotification("degraded")
	HealthEpisodeNotificationRecovered = HealthEpisodeNotification("recovered")
)

type HealthEpisodeNotification string

type HealthEpisodeState struct {
	ConsecutiveFailures int
	ActiveIncidentID    string
	IncidentOpenedAt    int64
	LastRecoveredAt     int64
}

type HealthEpisodeCheck struct {
	ServerID       int64
	PreviousStatus string
	Status         string
	CheckedAt      int64
}

type HealthEpisodeTransition struct {
	State            HealthEpisodeState
	Notification     HealthEpisodeNotification
	IncidentID       string
	IncidentOpenedAt int64
	CheckedAt        int64
}

func ApplyHealthEpisodeCheck(
	prior HealthEpisodeState,
	check HealthEpisodeCheck,
) HealthEpisodeTransition {
	next := normalizeHealthEpisodeState(prior)
	transition := HealthEpisodeTransition{
		State:            next,
		Notification:     HealthEpisodeNotificationNone,
		IncidentID:       strings.TrimSpace(next.ActiveIncidentID),
		IncidentOpenedAt: next.IncidentOpenedAt,
		CheckedAt:        check.CheckedAt,
	}

	switch normalizeHealthStatus(check.Status) {
	case HealthStatusUnhealthy:
		next.ConsecutiveFailures++
		if next.ActiveIncidentID == "" &&
			next.ConsecutiveFailures >= HealthEpisodeFailureThreshold {
			openedAt := check.CheckedAt
			next.ActiveIncidentID = StableHealthIncidentID(check.ServerID, openedAt)
			next.IncidentOpenedAt = openedAt
			transition.Notification = HealthEpisodeNotificationDegraded
			transition.IncidentID = next.ActiveIncidentID
			transition.IncidentOpenedAt = next.IncidentOpenedAt
		}
	case HealthStatusHealthy:
		activeIncidentID := strings.TrimSpace(next.ActiveIncidentID)
		incidentOpenedAt := next.IncidentOpenedAt
		shouldRecordRecovery := activeIncidentID != "" ||
			next.ConsecutiveFailures > 0 ||
			normalizeHealthStatus(check.PreviousStatus) == HealthStatusUnhealthy
		next.ConsecutiveFailures = 0
		next.ActiveIncidentID = ""
		next.IncidentOpenedAt = 0
		if shouldRecordRecovery {
			next.LastRecoveredAt = check.CheckedAt
		}
		if activeIncidentID != "" {
			transition.Notification = HealthEpisodeNotificationRecovered
			transition.IncidentID = activeIncidentID
			transition.IncidentOpenedAt = incidentOpenedAt
		}
	}

	transition.State = next
	return transition
}

func StableHealthIncidentID(serverID int64, openedAt int64) string {
	if serverID < 0 {
		serverID = 0
	}
	if openedAt <= 0 {
		openedAt = 1
	}
	return fmt.Sprintf("mcp-incident-%d-%d", serverID, openedAt)
}

func normalizeHealthEpisodeState(state HealthEpisodeState) HealthEpisodeState {
	state.ActiveIncidentID = strings.TrimSpace(state.ActiveIncidentID)
	if state.ConsecutiveFailures < 0 {
		state.ConsecutiveFailures = 0
	}
	if state.ActiveIncidentID == "" {
		state.IncidentOpenedAt = 0
	}
	return state
}

func normalizeHealthStatus(status string) string {
	return strings.ToLower(strings.TrimSpace(status))
}
