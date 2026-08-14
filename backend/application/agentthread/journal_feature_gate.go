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
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	adminconfig "github.com/coze-dev/coze-studio/backend/api/model/admin/config"
	domainentity "github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
)

type JournalFeature string

const (
	JournalFeatureProjection JournalFeature = "journal_projection"
	JournalFeatureUI         JournalFeature = "journal_ui"
	JournalFeatureSnapshots  JournalFeature = "journal_snapshots"
	JournalFeatureRecovery   JournalFeature = "checkpoint_recovery"

	defaultJournalFeatureGateCacheTTL = 30 * time.Second
)

type JournalRuntimeConfigurationProvider interface {
	GetBaseConfig(context.Context) (*adminconfig.BasicConfiguration, error)
}

type JournalFeatureGateOptions struct {
	CacheTTL time.Duration
	Now      func() time.Time
}

type JournalEnrollmentInput struct {
	SpaceID     int64
	RunKind     domainentity.RunKind
	ParentRunID int64
	RunConfig   string
}

type JournalEnrollmentDecision struct {
	Enrolled          bool
	SnapshotsEnabled  bool
	EnrollmentVersion string
	RolloutCohort     string
}

type JournalFeatureGateEvaluator interface {
	MasterEnabled(context.Context, JournalFeature) (bool, error)
	Enabled(context.Context, JournalFeature, int64) (bool, error)
	DecideEnrollment(context.Context, JournalEnrollmentInput) (JournalEnrollmentDecision, error)
}

type JournalFeatureGate struct {
	provider JournalRuntimeConfigurationProvider
	cacheTTL time.Duration
	now      func() time.Time

	mu        sync.Mutex
	cached    *adminconfig.JournalRuntimeConfiguration
	expiresAt time.Time
}

func NewJournalFeatureGate(
	provider JournalRuntimeConfigurationProvider,
	options JournalFeatureGateOptions,
) *JournalFeatureGate {
	cacheTTL := options.CacheTTL
	if cacheTTL <= 0 || cacheTTL > defaultJournalFeatureGateCacheTTL {
		cacheTTL = defaultJournalFeatureGateCacheTTL
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}
	return &JournalFeatureGate{provider: provider, cacheTTL: cacheTTL, now: now}
}

func (g *JournalFeatureGate) Invalidate() {
	if g == nil {
		return
	}
	g.mu.Lock()
	g.cached = nil
	g.expiresAt = time.Time{}
	g.mu.Unlock()
}

func JournalRolloutBucket(spaceID int64, feature JournalFeature) int32 {
	digest := sha256.Sum256([]byte(fmt.Sprintf("%d:%s", spaceID, feature)))
	return int32(binary.BigEndian.Uint64(digest[:8]) % 10000)
}

func (g *JournalFeatureGate) MasterEnabled(ctx context.Context, feature JournalFeature) (bool, error) {
	configuration, err := g.configuration(ctx)
	if err != nil {
		return false, err
	}
	enabled, _, err := journalFeatureConfiguration(configuration, feature)
	return enabled, err
}

func (g *JournalFeatureGate) Enabled(
	ctx context.Context,
	feature JournalFeature,
	spaceID int64,
) (bool, error) {
	if spaceID <= 0 {
		return false, nil
	}
	configuration, err := g.configuration(ctx)
	if err != nil {
		return false, err
	}
	enabled, rollout, err := journalFeatureConfiguration(configuration, feature)
	if err != nil || !enabled || rollout <= 0 {
		return false, err
	}
	if rollout >= 10000 {
		return true, nil
	}
	return JournalRolloutBucket(spaceID, feature) < rollout, nil
}

func (g *JournalFeatureGate) DecideEnrollment(
	ctx context.Context,
	input JournalEnrollmentInput,
) (JournalEnrollmentDecision, error) {
	if input.SpaceID <= 0 || input.ParentRunID != 0 || input.RunKind != domainentity.RunKindTask {
		return JournalEnrollmentDecision{}, nil
	}
	runtime, err := journalEnrollmentRuntime(input.RunConfig)
	if err != nil {
		return JournalEnrollmentDecision{}, err
	}
	if runtime != RuntimeModeEinoADK {
		return JournalEnrollmentDecision{}, nil
	}
	enrolled, err := g.Enabled(ctx, JournalFeatureProjection, input.SpaceID)
	if err != nil || !enrolled {
		return JournalEnrollmentDecision{}, err
	}
	snapshots, err := g.Enabled(ctx, JournalFeatureSnapshots, input.SpaceID)
	if err != nil {
		return JournalEnrollmentDecision{}, err
	}
	return JournalEnrollmentDecision{
		Enrolled:          true,
		SnapshotsEnabled:  snapshots,
		EnrollmentVersion: domainentity.JournalSchemaVersion,
		RolloutCohort:     "treatment",
	}, nil
}

func journalEnrollmentRuntime(rawConfig string) (RuntimeMode, error) {
	if strings.TrimSpace(rawConfig) == "" {
		return "", nil
	}
	var config struct {
		Runtime string `json:"runtime"`
	}
	if err := json.Unmarshal([]byte(rawConfig), &config); err != nil {
		return "", fmt.Errorf("parse journal enrollment runtime: %w", err)
	}
	runtime := RuntimeMode(strings.ToLower(strings.TrimSpace(config.Runtime)))
	switch runtime {
	case "", RuntimeModeLegacy, RuntimeModeEinoADK:
		return runtime, nil
	default:
		return "", fmt.Errorf("unsupported journal enrollment runtime: %s", runtime)
	}
}

func (g *JournalFeatureGate) configuration(
	ctx context.Context,
) (*adminconfig.JournalRuntimeConfiguration, error) {
	if g == nil || g.provider == nil {
		return &adminconfig.JournalRuntimeConfiguration{}, nil
	}
	now := g.now()
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.cached != nil && now.Before(g.expiresAt) {
		cloned := *g.cached
		return &cloned, nil
	}
	base, err := g.provider.GetBaseConfig(ctx)
	if err != nil {
		return nil, err
	}
	configuration := &adminconfig.JournalRuntimeConfiguration{}
	if base != nil && base.JournalRuntimeConfiguration != nil {
		cloned := *base.JournalRuntimeConfiguration
		configuration = &cloned
	}
	g.cached = configuration
	g.expiresAt = now.Add(g.cacheTTL)
	cloned := *configuration
	return &cloned, nil
}

func journalFeatureConfiguration(
	configuration *adminconfig.JournalRuntimeConfiguration,
	feature JournalFeature,
) (bool, int32, error) {
	if configuration == nil {
		return false, 0, nil
	}
	switch feature {
	case JournalFeatureProjection:
		return configuration.JournalProjection, configuration.JournalProjectionRolloutBasisPoints, nil
	case JournalFeatureUI:
		return configuration.JournalUI, configuration.JournalUIRolloutBasisPoints, nil
	case JournalFeatureSnapshots:
		return configuration.JournalSnapshots, configuration.JournalSnapshotsRolloutBasisPoints, nil
	case JournalFeatureRecovery:
		return configuration.CheckpointRecovery, configuration.CheckpointRecoveryRolloutBasisPoints, nil
	default:
		return false, 0, fmt.Errorf("unsupported journal feature %q", feature)
	}
}
