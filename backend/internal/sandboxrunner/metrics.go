// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandboxrunner

import (
	"net/http"
	"strconv"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
)

const metricsContentType = "text/plain; version=0.0.4; charset=utf-8"

// metrics is deliberately protected by the same Runner bearer token as other
// operational endpoints. It exposes only fixed, low-cardinality aggregate
// labels and never accepts labels from a request or stored execution.
func (s *Server) metrics(writer http.ResponseWriter, request *http.Request) {
	if request.URL.RawQuery != "" {
		writePublicError(writer, http.StatusBadRequest, "INVALID_REQUEST")
		return
	}
	if !authenticateBearer(request, s.config.AuthToken) {
		writePublicError(writer, http.StatusUnauthorized, "UNAUTHORIZED")
		return
	}
	if s.runtimeStatus == nil {
		writePublicError(writer, http.StatusServiceUnavailable, "UNAVAILABLE")
		return
	}
	status, err := s.runtimeStatus.RuntimeStatus(request.Context())
	if err != nil {
		writePublicError(writer, http.StatusServiceUnavailable, "UNAVAILABLE")
		return
	}
	writer.Header().Set("Content-Type", metricsContentType)
	writer.WriteHeader(http.StatusOK)
	for _, scope := range []domainsandbox.Scope{domainsandbox.ScopeAgent, domainsandbox.ScopeAppDev, domainsandbox.ScopeMCPStdio, domainsandbox.ScopePlugin} {
		_, _ = writer.Write([]byte("coze_sandbox_runner_queue_depth{scope=\"" + string(scope) + "\"} " + strconv.Itoa(status.QueuedByScope[string(scope)]) + "\n"))
	}
	writeMetric(writer, "coze_sandbox_runner_weight", "used", status.UsedWeight)
	writeMetric(writer, "coze_sandbox_runner_weight", "total", status.TotalWeight)
	writeMetric(writer, "coze_sandbox_runner_containers", "idle", status.IdleContainers)
	writeMetric(writer, "coze_sandbox_runner_containers", "active", status.ActiveContainers)
	writeMetric(writer, "coze_sandbox_runner_containers", "quarantined", status.QuarantinedContainers)
	writeMetric(writer, "coze_sandbox_runner_memory_reserve", normalizeMemoryReserveMetricState(status.MemoryReserveState), 1)
}

func writeMetric(writer http.ResponseWriter, name, state string, value int) {
	_, _ = writer.Write([]byte(name + "{state=\"" + state + "\"} " + strconv.Itoa(value) + "\n"))
}

func normalizeMemoryReserveMetricState(value string) string {
	if value == memoryReserveAvailable || value == memoryReserveBelowWatermark || value == memoryReserveUnknown {
		return value
	}
	return memoryReserveUnknown
}
