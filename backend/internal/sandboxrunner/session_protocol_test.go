// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandboxrunner

import (
	"fmt"
	"strings"
	"testing"
	"time"

	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
)

func TestParseSessionAcquireRejectsPhysicalWorkspaceAndAcceptsCanonicalBody(t *testing.T) {
	request, err := parseSessionAcquire([]byte(validAcquireSessionBody()), "runner-a")
	if err != nil || request.Identity.DeploymentID != "runner-a" || request.Claims.OperationID != "request_01" {
		t.Fatalf("parseSessionAcquire() = %#v, %v", request, err)
	}
	if _, err := parseSessionAcquire([]byte(`{"schema":"coze.sandbox.session_acquire.v1","deployment_id":"runner-a","provider_id":41,"scope":"agent","space_id":42,"user_id":43,"thread_id":"thread_01","run_id":"run_01","operation_id":"request_01","profile":"core","upstream_shell_id":"secret"}`), "runner-a"); err == nil {
		t.Fatal("parseSessionAcquire() accepted upstream shell id")
	}
}

func TestSessionStableIdentityRejectsClaimsForAnotherDeployment(t *testing.T) {
	parsed, err := parseSessionAcquire([]byte(validAcquireSessionBody()), "runner-a")
	if err != nil {
		t.Fatal(err)
	}
	parsed.Claims.DeploymentID = "runner-b"
	if stableIdentityMatchesClaims(parsed.Identity, parsed.Claims) {
		t.Fatal("stableIdentityMatchesClaims() accepted another deployment")
	}
}

func TestParseSessionOperationEnforcesLogicalPathsDeadlinesAndSDKBounds(t *testing.T) {
	now := time.Date(2026, 8, 13, 10, 0, 0, 0, time.UTC)
	valid := func(kind SessionOperationKind, payload string) string {
		return fmt.Sprintf(`{"schema":"coze.sandbox.session_operation.v1","operation_id":"operation_01","kind":%q,"deadline":%q,"payload":%s}`,
			kind, now.Add(time.Minute).Format(time.RFC3339Nano), payload)
	}
	if parsed, err := parseSessionOperation([]byte(valid(SessionOperationExec,
		`{"argv":["printf","ok"],"env":{"LANG":"C.UTF-8"},"cwd":"/mnt/user-data/workspace","max_output_bytes":1024}`)), now, "LANG"); err != nil || parsed.Kind != SessionOperationExec {
		t.Fatalf("parseSessionOperation(valid exec) = %#v, %v", parsed, err)
	}
	for name, body := range map[string]string{
		"exec cwd outside workspace": valid(SessionOperationExec, `{"command":"pwd","cwd":"/mnt/user-data/uploads","max_output_bytes":1024}`),
		"read physical path":         valid(SessionOperationRead, `{"path":"/mnt/user-data/42/43/thread_01/workspace/private","max_bytes":1024}`),
		"write global skills":        valid(SessionOperationWrite, `{"path":"/mnt/skills/tool/SKILL.md","content":"dW5zYWZl"}`),
		"glob traversal":             valid(SessionOperationGlob, `{"path":"/mnt/user-data/workspace","pattern":"../*.go","limit":10}`),
		"read above public cap":      valid(SessionOperationRead, fmt.Sprintf(`{"path":"/mnt/user-data/workspace/a","max_bytes":%d}`, infrasandbox.MaxSessionFileBytes+1)),
		"deadline too far":           strings.Replace(valid(SessionOperationRead, `{"path":"/mnt/user-data/workspace/a","max_bytes":1}`), now.Add(time.Minute).Format(time.RFC3339Nano), now.Add(infrasandbox.MaxSessionDeadlineAhead+time.Second).Format(time.RFC3339Nano), 1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseSessionOperation([]byte(body), now, "LANG"); err == nil {
				t.Fatal("parseSessionOperation() unexpectedly accepted request")
			}
		})
	}
}

func TestParseSessionConfigurationUsesCompleteStrictSettingsSnapshot(t *testing.T) {
	settings := `{"core_enabled":true,"interactive_enabled":false,"host_shell_enabled":false,"core_weight":1,"heavy_weight":2,"per_user_active_limit":1,"idle_session_limit":20,"idle_shell_limit":4,"session_idle_ttl_seconds":1200,"shell_idle_ttl_seconds":300,"command_timeout_seconds":600,"cancel_grace_seconds":5,"workspace_quota_mb":2048}`
	valid := `{"schema":"coze.sandbox.session_configuration.v1","expected_version":1,"settings":` + settings + `}`
	if parsed, err := parseSessionConfiguration([]byte(valid)); err != nil || parsed.ExpectedVersion != 1 || !parsed.Settings.CoreEnabled {
		t.Fatalf("parseSessionConfiguration() = %#v, %v", parsed, err)
	}
	for _, invalid := range []string{
		strings.Replace(valid, `"expected_version":1`, `"expected_version":0`, 1),
		strings.Replace(valid, `"interactive_enabled":false`, `"interactive_enabled":true`, 1),
		strings.TrimSuffix(valid, "}") + `,"credential":"secret"}`,
	} {
		if _, err := parseSessionConfiguration([]byte(invalid)); err == nil {
			t.Fatal("parseSessionConfiguration() unexpectedly accepted invalid snapshot")
		}
	}
}
