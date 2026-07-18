// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

// Package sandboxcontract contains dependency-neutral sandbox persistence limits.
package sandboxcontract

// MaxExecutionCheckpointEnvelopeBytes is the single persistence and protection
// boundary for a sealed execution checkpoint envelope.
const MaxExecutionCheckpointEnvelopeBytes = 8 * 1024
