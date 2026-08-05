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

package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/coze-dev/coze-studio/backend/internal/deerflowparity"
	"github.com/stretchr/testify/require"
)

func TestParseConfigReadsSecretsOnlyFromEnvironment(t *testing.T) {
	t.Parallel()

	environment := map[string]string{
		"DEERFLOW_PARITY_DEERFLOW_URL":              "http://127.0.0.1:2026",
		"DEERFLOW_PARITY_DEERFLOW_SOURCE_DIR":       "/tmp/deer-flow",
		"DEERFLOW_PARITY_DEERFLOW_RUNTIME_ATTESTED": "true",
		"DEERFLOW_PARITY_DEERFLOW_ENV_LABEL":        "local_deerflow",
		"DEERFLOW_PARITY_NEWX_URL":                  "http://127.0.0.1:8888",
		"DEERFLOW_PARITY_NEWX_ENV_LABEL":            "newx_debug",
		"DEERFLOW_PARITY_EMAIL":                     "user@example.com",
		"DEERFLOW_PARITY_PASSWORD":                  "private-password",
		"DEERFLOW_PARITY_NEWX_SPACE_ID":             "7656103552997130240",
	}
	config, err := parseConfig([]string{
		"-cases", "core.pro.direct,core.stream.reconnect",
		"-format", "json",
		"-out", "report.json",
	}, func(key string) string { return environment[key] })
	require.NoError(t, err)
	require.Equal(t, "private-password", config.password)
	require.Equal(t, "/tmp/deer-flow", config.deerFlowSourceDir)
	require.True(t, config.deerFlowRuntimeAttested)
	require.Equal(t, "local_deerflow", config.deerFlowEnvironment)
	require.Equal(t, "newx_debug", config.newXEnvironment)
	require.Equal(t, []string{"core.pro.direct", "core.stream.reconnect"}, config.caseIDs)
	require.Equal(t, "json", config.format)
	require.Equal(t, "report.json", config.outputPath)
}

func TestValidateDeerFlowRevisionEvidenceRequiresExactCleanCheckout(t *testing.T) {
	t.Parallel()

	expected := "5851f8250eb150ca23134c79b11ebc5073ac2789"
	require.NoError(t, validateDeerFlowRevisionEvidence(expected, expected, true))
	require.ErrorContains(t, validateDeerFlowRevisionEvidence(expected, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", true), "does not match")
	require.ErrorContains(t, validateDeerFlowRevisionEvidence(expected, expected, false), "tracked changes")
}

func TestVerifyDeerFlowSourceRevisionRejectsUntrackedChanges(t *testing.T) {
	t.Parallel()

	sourceDir := t.TempDir()
	runGitForTest(t, sourceDir, "init")
	runGitForTest(t, sourceDir, "config", "user.email", "acceptance@example.com")
	runGitForTest(t, sourceDir, "config", "user.name", "Acceptance Test")
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "tracked.txt"), []byte("locked\n"), 0o600))
	runGitForTest(t, sourceDir, "add", "tracked.txt")
	runGitForTest(t, sourceDir, "commit", "-m", "locked fixture")
	revision := strings.TrimSpace(runGitForTest(t, sourceDir, "rev-parse", "HEAD"))

	verified, err := verifyDeerFlowSourceRevision(context.Background(), sourceDir, revision)
	require.NoError(t, err)
	require.Equal(t, revision, verified)

	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "untracked.py"), []byte("print('override')\n"), 0o600))
	_, err = verifyDeerFlowSourceRevision(context.Background(), sourceDir, revision)
	require.ErrorContains(t, err, "source checkout")
}

func runGitForTest(t *testing.T, directory string, arguments ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", directory}, arguments...)...)
	output, err := command.CombinedOutput()
	require.NoError(t, err, string(output))
	return string(output)
}

func TestParseConfigRejectsMissingEnvironmentWithoutLeakingValues(t *testing.T) {
	t.Parallel()

	_, err := parseConfig(nil, func(key string) string {
		if key == "DEERFLOW_PARITY_EMAIL" {
			return "private@example.com"
		}
		return ""
	})
	require.Error(t, err)
	require.NotContains(t, err.Error(), "private@example.com")
}

func TestParseConfigDoesNotDefinePasswordFlag(t *testing.T) {
	t.Parallel()

	_, err := parseConfig([]string{"-password", "secret"}, func(string) string { return "configured" })
	require.Error(t, err)
	require.NotContains(t, err.Error(), "secret")
}

func TestReportGateErrorFailsForDifferentOrBlockedCases(t *testing.T) {
	t.Parallel()

	for name, status := range map[string]deerflowparity.ComparisonStatus{
		"different": deerflowparity.StatusDifferent,
		"blocked":   deerflowparity.StatusBlocked,
	} {
		status := status
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			err := reportGateError(deerflowparity.SuiteReport{Cases: []deerflowparity.CaseReport{{
				Comparison: deerflowparity.ComparisonResult{Status: status},
			}}})
			require.ErrorContains(t, err, "gate did not pass")
		})
	}

	require.NoError(t, reportGateError(deerflowparity.SuiteReport{Cases: []deerflowparity.CaseReport{
		{Comparison: deerflowparity.ComparisonResult{Status: deerflowparity.StatusAligned}},
		{Comparison: deerflowparity.ComparisonResult{Status: deerflowparity.StatusStronger}},
	}}))
}

func TestReportGateErrorFailsClosedForEmptyOrUnknownResults(t *testing.T) {
	t.Parallel()

	require.ErrorContains(t, reportGateError(deerflowparity.SuiteReport{}), "no cases")
	require.ErrorContains(t, reportGateError(deerflowparity.SuiteReport{Cases: []deerflowparity.CaseReport{{
		Comparison: deerflowparity.ComparisonResult{},
	}}}), "unknown=1")
}
