/*
 * Copyright 2025 coze-dev Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * You may not use this file except in compliance with the License.
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
	"path/filepath"
	"strings"
	"time"

	domainrepo "github.com/coze-dev/coze-studio/backend/domain/agentthread/repository"
	"github.com/coze-dev/coze-studio/backend/pkg/envkey"
	"github.com/coze-dev/coze-studio/backend/pkg/logs"
)

const (
	agentThreadMCPStdioWorkdirLeaseReaperEnabledEnv    = "AGENT_THREAD_MCP_STDIO_WORKDIR_REAPER_ENABLED"
	agentThreadMCPStdioWorkdirLeaseReaperRootEnv       = "AGENT_THREAD_MCP_STDIO_WORKDIR_REAPER_ROOT"
	agentThreadMCPStdioWorkdirLeaseReaperBatchSizeEnv  = "AGENT_THREAD_MCP_STDIO_WORKDIR_REAPER_BATCH_SIZE"
	agentThreadMCPStdioWorkdirLeaseReaperIntervalMsEnv = "AGENT_THREAD_MCP_STDIO_WORKDIR_REAPER_INTERVAL_MS"
)

const defaultADKMCPRuntimeStdioWorkdirLeaseReaperInterval = 5 * time.Minute

type ADKMCPRuntimeStdioWorkdirLeaseCleaner interface {
	CleanupExpiredADKMCPRuntimeStdioWorkdirLeases(
		ctx context.Context,
	) (ADKMCPRuntimeStdioWorkdirLeaseReaperResult, error)
}

type ADKMCPRuntimeStdioWorkdirLeaseReaperWorkerOptions struct {
	Interval time.Duration
}

type ADKMCPRuntimeStdioWorkdirLeaseReaperWorker struct {
	reaper   ADKMCPRuntimeStdioWorkdirLeaseCleaner
	interval time.Duration
}

type ADKMCPRuntimeStdioWorkdirLeaseReaperWorkerEnvStatus struct {
	Enabled bool
	Started bool
	Reason  string
}

func NewADKMCPRuntimeStdioWorkdirLeaseReaperWorker(
	reaper ADKMCPRuntimeStdioWorkdirLeaseCleaner,
	options ADKMCPRuntimeStdioWorkdirLeaseReaperWorkerOptions,
) *ADKMCPRuntimeStdioWorkdirLeaseReaperWorker {
	interval := options.Interval
	if interval <= 0 {
		interval = defaultADKMCPRuntimeStdioWorkdirLeaseReaperInterval
	}

	return &ADKMCPRuntimeStdioWorkdirLeaseReaperWorker{
		reaper:   reaper,
		interval: interval,
	}
}

func (w *ADKMCPRuntimeStdioWorkdirLeaseReaperWorker) Start(ctx context.Context) {
	if w == nil || w.reaper == nil {
		return
	}

	go func() {
		ticker := time.NewTicker(w.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				w.RunOnce(ctx)
			}
		}
	}()
}

func (w *ADKMCPRuntimeStdioWorkdirLeaseReaperWorker) RunOnce(
	ctx context.Context,
) ADKMCPRuntimeStdioWorkdirLeaseReaperResult {
	if w == nil || w.reaper == nil {
		return ADKMCPRuntimeStdioWorkdirLeaseReaperResult{}
	}
	result, err := w.reaper.CleanupExpiredADKMCPRuntimeStdioWorkdirLeases(ctx)
	if err != nil {
		logs.CtxErrorf(ctx, "[mcp-stdio-workdir-reaper] cleanup failed: %v", err)

		return ADKMCPRuntimeStdioWorkdirLeaseReaperResult{}
	}
	if result.Listed > 0 {
		logs.CtxInfof(
			ctx,
			"[mcp-stdio-workdir-reaper] processed expired leases, listed=%d cleaned=%d finished=%d invalid=%d failed=%d",
			result.Listed,
			result.Cleaned,
			result.Finished,
			result.Invalid,
			result.Failed,
		)
	}

	return result
}

func StartADKMCPRuntimeStdioWorkdirLeaseReaperWorkerFromEnv(
	ctx context.Context,
	repository domainrepo.MCPRuntimeWorkdirLeaseRepository,
	preparers ...*ADKMCPRuntimeStdioFilesystemWorkdirPreparer,
) *ADKMCPRuntimeStdioWorkdirLeaseReaperWorker {
	worker, _ := StartADKMCPRuntimeStdioWorkdirLeaseReaperWorkerFromEnvWithStatus(
		ctx,
		repository,
		preparers...,
	)

	return worker
}

func StartADKMCPRuntimeStdioWorkdirLeaseReaperWorkerFromEnvWithStatus(
	ctx context.Context,
	repository domainrepo.MCPRuntimeWorkdirLeaseRepository,
	preparers ...*ADKMCPRuntimeStdioFilesystemWorkdirPreparer,
) (*ADKMCPRuntimeStdioWorkdirLeaseReaperWorker, ADKMCPRuntimeStdioWorkdirLeaseReaperWorkerEnvStatus) {
	status := ADKMCPRuntimeStdioWorkdirLeaseReaperWorkerEnvStatus{
		Enabled: envkey.GetBoolD(
			agentThreadMCPStdioWorkdirLeaseReaperEnabledEnv,
			false,
		),
	}
	if !status.Enabled {
		return nil, status
	}
	if repository == nil {
		status.Reason = "mcp stdio workdir lease repository is not configured"
		logs.CtxWarnf(ctx, "[mcp-stdio-workdir-reaper] enabled but repository is not configured")

		return nil, status
	}
	root := filepath.Clean(strings.TrimSpace(envkey.GetStringD(
		agentThreadMCPStdioWorkdirLeaseReaperRootEnv,
		"",
	)))
	if !filepath.IsAbs(root) {
		status.Reason = "mcp stdio workdir reaper root is invalid"
		logs.CtxWarnf(ctx, "[mcp-stdio-workdir-reaper] enabled but root is invalid")

		return nil, status
	}
	var preparer *ADKMCPRuntimeStdioFilesystemWorkdirPreparer
	if len(preparers) > 1 {
		status.Reason = "mcp stdio workdir reaper preparer is invalid"
		logs.CtxWarnf(ctx, "[mcp-stdio-workdir-reaper] shared preparer is invalid")
		return nil, status
	}
	if len(preparers) == 1 {
		preparer = preparers[0]
		if preparer == nil || !preparer.Valid() || preparer.Root() != root {
			status.Reason = "mcp stdio workdir reaper root does not match the locked root"
			logs.CtxWarnf(ctx, "[mcp-stdio-workdir-reaper] shared preparer root mismatch")
			return nil, status
		}
	}

	reaper := NewADKMCPRuntimeStdioWorkdirLeaseReaper(
		ADKMCPRuntimeStdioWorkdirLeaseReaperOptions{
			Repository:      repository,
			Root:            root,
			WorkdirPreparer: preparer,
			BatchSize: envkey.GetI32D(
				agentThreadMCPStdioWorkdirLeaseReaperBatchSizeEnv,
				defaultADKMCPRuntimeStdioWorkdirLeaseReaperBatchSize,
			),
		},
	)
	worker := NewADKMCPRuntimeStdioWorkdirLeaseReaperWorker(
		reaper,
		ADKMCPRuntimeStdioWorkdirLeaseReaperWorkerOptions{
			Interval: time.Duration(envkey.GetIntD(
				agentThreadMCPStdioWorkdirLeaseReaperIntervalMsEnv,
				int(defaultADKMCPRuntimeStdioWorkdirLeaseReaperInterval/time.Millisecond),
			)) * time.Millisecond,
		},
	)
	worker.Start(ctx)
	status.Started = true

	return worker, status
}
