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
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/coze-dev/coze-studio/backend/internal/deerflowparity"
)

type commandConfig struct {
	deerFlowURL             string
	deerFlowSourceDir       string
	deerFlowRuntimeAttested bool
	deerFlowEnvironment     string
	newXURL                 string
	newXEnvironment         string
	email                   string
	password                string
	spaceID                 string
	caseIDs                 []string
	format                  string
	outputPath              string
	allowRemote             bool
	timeout                 time.Duration
}

func main() {
	if err := run(os.Args[1:], os.Getenv, os.Stdout); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "acceptance failed:", err)
		os.Exit(1)
	}
}

func run(args []string, getenv func(string) string, stdout io.Writer) error {
	config, err := parseConfig(args, getenv)
	if err != nil {
		return err
	}
	suite, err := deerflowparity.LoadCases()
	if err != nil {
		return err
	}
	ctx := context.Background()
	deerFlowRevision, err := verifyDeerFlowSourceRevision(ctx, config.deerFlowSourceDir, suite.DeerFlowRevision)
	if err != nil {
		return err
	}
	suite, err = selectCases(suite, config.caseIDs)
	if err != nil {
		return err
	}
	clientOptions := deerflowparity.ClientOptions{
		Timeout:     config.timeout,
		AllowRemote: config.allowRemote,
	}
	reference, err := deerflowparity.NewDeerFlowClient(config.deerFlowURL, clientOptions)
	if err != nil {
		return fmt.Errorf("configure DeerFlow client: %w", err)
	}
	candidate, err := deerflowparity.NewNewXClient(config.newXURL, clientOptions)
	if err != nil {
		return fmt.Errorf("configure NewX client: %w", err)
	}
	runner, err := deerflowparity.NewRunner(reference, candidate, deerflowparity.RunnerOptions{
		Credentials:          deerflowparity.Credentials{Email: config.email, Password: config.password},
		NewXSpaceID:          config.spaceID,
		DeerFlowRevision:     deerFlowRevision,
		RevisionProvenance:   deerflowparity.RevisionProvenanceVerifiedSourceAttestedRuntime,
		ReferenceEnvironment: config.deerFlowEnvironment,
		CandidateEnvironment: config.newXEnvironment,
		CaseTimeout:          config.timeout,
	})
	if err != nil {
		return err
	}
	report, err := runner.RunSuite(ctx, suite)
	if err != nil {
		return err
	}
	if config.outputPath == "" {
		if err := writeReport(stdout, config.format, report); err != nil {
			return err
		}
		return reportGateError(report)
	}
	if err := writeReportFile(config.outputPath, config.format, report); err != nil {
		return err
	}
	return reportGateError(report)
}

func reportGateError(report deerflowparity.SuiteReport) error {
	if len(report.Cases) == 0 {
		return errors.New("acceptance gate did not pass: report contains no cases")
	}
	different := 0
	blocked := 0
	unknown := 0
	for _, testCase := range report.Cases {
		switch testCase.Comparison.Status {
		case deerflowparity.StatusAligned, deerflowparity.StatusStronger:
		case deerflowparity.StatusDifferent:
			different++
		case deerflowparity.StatusBlocked:
			blocked++
		default:
			unknown++
		}
	}
	if different > 0 || blocked > 0 || unknown > 0 {
		return fmt.Errorf(
			"acceptance gate did not pass: different=%d blocked=%d unknown=%d",
			different,
			blocked,
			unknown,
		)
	}
	return nil
}

func parseConfig(args []string, getenv func(string) string) (commandConfig, error) {
	flags := flag.NewFlagSet("deerflow-parity-acceptance", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	casesValue := flags.String("cases", "semantic-core", "comma-separated acceptance case IDs")
	formatValue := flags.String("format", "markdown", "report format: markdown or json")
	outputValue := flags.String("out", "", "report output path; stdout when empty")
	allowRemote := flags.Bool("allow-remote", false, "allow non-loopback acceptance endpoints")
	timeout := flags.Duration("timeout", 3*time.Minute, "per-case and HTTP timeout")
	if err := flags.Parse(args); err != nil {
		return commandConfig{}, errors.New("command flags are invalid")
	}
	if flags.NArg() != 0 {
		return commandConfig{}, errors.New("positional arguments are not supported")
	}

	config := commandConfig{
		deerFlowURL:             strings.TrimSpace(getenv("DEERFLOW_PARITY_DEERFLOW_URL")),
		deerFlowSourceDir:       strings.TrimSpace(getenv("DEERFLOW_PARITY_DEERFLOW_SOURCE_DIR")),
		deerFlowRuntimeAttested: strings.EqualFold(strings.TrimSpace(getenv("DEERFLOW_PARITY_DEERFLOW_RUNTIME_ATTESTED")), "true"),
		deerFlowEnvironment:     environmentLabel(getenv("DEERFLOW_PARITY_DEERFLOW_ENV_LABEL"), "deerflow"),
		newXURL:                 strings.TrimSpace(getenv("DEERFLOW_PARITY_NEWX_URL")),
		newXEnvironment:         environmentLabel(getenv("DEERFLOW_PARITY_NEWX_ENV_LABEL"), "newx"),
		email:                   strings.TrimSpace(getenv("DEERFLOW_PARITY_EMAIL")),
		password:                getenv("DEERFLOW_PARITY_PASSWORD"),
		spaceID:                 strings.TrimSpace(getenv("DEERFLOW_PARITY_NEWX_SPACE_ID")),
		caseIDs:                 parseCaseIDs(*casesValue),
		format:                  strings.ToLower(strings.TrimSpace(*formatValue)),
		outputPath:              strings.TrimSpace(*outputValue),
		allowRemote:             *allowRemote,
		timeout:                 *timeout,
	}
	missing := make([]string, 0, 6)
	for name, value := range map[string]string{
		"DEERFLOW_PARITY_DEERFLOW_URL":        config.deerFlowURL,
		"DEERFLOW_PARITY_DEERFLOW_SOURCE_DIR": config.deerFlowSourceDir,
		"DEERFLOW_PARITY_NEWX_URL":            config.newXURL,
		"DEERFLOW_PARITY_EMAIL":               config.email,
		"DEERFLOW_PARITY_PASSWORD":            config.password,
		"DEERFLOW_PARITY_NEWX_SPACE_ID":       config.spaceID,
	} {
		if strings.TrimSpace(value) == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		slices.Sort(missing)
		return commandConfig{}, fmt.Errorf("missing required environment variables: %s", strings.Join(missing, ", "))
	}
	if !config.deerFlowRuntimeAttested {
		return commandConfig{}, errors.New("DEERFLOW_PARITY_DEERFLOW_RUNTIME_ATTESTED must explicitly be true")
	}
	if len(config.caseIDs) == 0 {
		return commandConfig{}, errors.New("at least one case must be selected")
	}
	if config.format != "markdown" && config.format != "json" {
		return commandConfig{}, errors.New("report format must be markdown or json")
	}
	if config.timeout < time.Second || config.timeout > 30*time.Minute {
		return commandConfig{}, errors.New("timeout is outside allowed bounds")
	}
	return config, nil
}

func verifyDeerFlowSourceRevision(ctx context.Context, sourceDir, expected string) (string, error) {
	resolved, err := filepath.Abs(strings.TrimSpace(sourceDir))
	if err != nil {
		return "", errors.New("resolve DeerFlow source directory failed")
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return "", errors.New("DeerFlow source directory is unavailable")
	}
	revisionOutput, err := exec.CommandContext(ctx, "git", "-C", resolved, "rev-parse", "HEAD").Output()
	if err != nil {
		return "", errors.New("read DeerFlow source revision failed")
	}
	actual := strings.TrimSpace(string(revisionOutput))
	statusOutput, err := exec.CommandContext(
		ctx,
		"git", "-C", resolved, "status", "--porcelain=v1", "--untracked-files=all",
	).Output()
	if err != nil {
		return "", errors.New("verify DeerFlow source worktree failed")
	}
	clean := len(bytes.TrimSpace(statusOutput)) == 0
	if err := validateDeerFlowRevisionEvidence(expected, actual, clean); err != nil {
		return "", err
	}
	return actual, nil
}

func validateDeerFlowRevisionEvidence(expected, actual string, clean bool) error {
	if len(actual) != 40 || actual != strings.ToLower(actual) {
		return errors.New("DeerFlow source revision is invalid")
	}
	if actual != expected {
		return errors.New("DeerFlow source revision does not match the locked fixture")
	}
	if !clean {
		return errors.New("DeerFlow source checkout has tracked changes")
	}
	return nil
}

func environmentLabel(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func parseCaseIDs(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" || value == "semantic-core" {
		return deerflowparity.SemanticCoreCaseIDs()
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" && !slices.Contains(result, part) {
			result = append(result, part)
		}
	}
	return result
}

func selectCases(suite *deerflowparity.Suite, selected []string) (*deerflowparity.Suite, error) {
	copySuite := *suite
	copySuite.Cases = make([]deerflowparity.Case, 0, len(selected))
	for _, id := range selected {
		found := false
		for _, testCase := range suite.Cases {
			if testCase.ID == id {
				copySuite.Cases = append(copySuite.Cases, testCase)
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("acceptance case %q is unknown", id)
		}
	}
	return &copySuite, nil
}

func writeReport(writer io.Writer, format string, report deerflowparity.SuiteReport) error {
	if format == "json" {
		return deerflowparity.WriteJSONReport(writer, report)
	}
	return deerflowparity.WriteMarkdownReport(writer, report)
}

func writeReportFile(path, format string, report deerflowparity.SuiteReport) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o750); err != nil {
		return errors.New("create report directory failed")
	}
	temporary, err := os.CreateTemp(directory, ".deerflow-parity-*.tmp")
	if err != nil {
		return errors.New("create temporary report failed")
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return errors.New("secure temporary report failed")
	}
	if err := writeReport(temporary, format, report); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return errors.New("close temporary report failed")
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return errors.New("publish report failed")
	}
	return nil
}
