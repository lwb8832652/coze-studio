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
	"errors"
	"fmt"

	"github.com/cloudwego/eino/adk"
)

type RunInterruptedError struct {
	CheckpointKey  string
	Interrupts     []ADKInterruptItem
	EventPersisted bool
}

func (e *RunInterruptedError) Error() string {
	if e == nil {
		return "agent run interrupted"
	}

	return fmt.Sprintf(
		"agent run interrupted at checkpoint %s with %d interrupt target(s)",
		e.CheckpointKey,
		len(e.Interrupts),
	)
}

type RunCanceledError struct {
	EventPersisted bool
}

func (e *RunCanceledError) Error() string {
	return "agent run canceled"
}

type ArtifactContentBlockedByScanError struct {
	ScanStatus string
	Reason     string
}

func (e *ArtifactContentBlockedByScanError) Error() string {
	return "artifact scan status blocks content read"
}

var ErrArtifactScanJobRetryNotAllowed = errors.New(
	"artifact scan job cannot be retried",
)

func isADKCancellationError(err error) bool {
	var canceled *adk.CancelError
	if errors.As(err, &canceled) {
		return true
	}
	var streamCanceled *adk.StreamCanceledError
	return errors.As(err, &streamCanceled)
}
