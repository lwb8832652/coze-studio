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

package deerflowparity

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseSSESupportsCommentsAndMultilineData(t *testing.T) {
	t.Parallel()

	raw := ": keepalive\n" +
		"id: 11\n" +
		"event: events\n" +
		"data: {\"event_type\":\"run.started\",\n" +
		"data: \"payload\":{}}\n\n" +
		"event: end\n" +
		"data: {\"status\":\"success\"}\n\n"

	frames, err := ParseSSE(strings.NewReader(raw), SSELimits{MaxBytes: 4096, MaxFrames: 10, MaxLineBytes: 1024})
	require.NoError(t, err)
	require.Equal(t, []SSEFrame{
		{ID: "11", Event: "events", Data: []byte("{\"event_type\":\"run.started\",\n\"payload\":{}}")},
		{Event: "end", Data: []byte("{\"status\":\"success\"}")},
	}, frames)
}

func TestParseSSERejectsMalformedAndOversizedStreams(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		raw    string
		limits SSELimits
	}{
		"invalid field":   {raw: "unknown: value\n\n", limits: SSELimits{MaxBytes: 1024, MaxFrames: 2, MaxLineBytes: 128}},
		"too many frames": {raw: "event: a\n\ndata: x\n\n", limits: SSELimits{MaxBytes: 1024, MaxFrames: 1, MaxLineBytes: 128}},
		"too many bytes":  {raw: "data: " + strings.Repeat("x", 64) + "\n\n", limits: SSELimits{MaxBytes: 16, MaxFrames: 2, MaxLineBytes: 128}},
	}

	for name, test := range tests {
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := ParseSSE(strings.NewReader(test.raw), test.limits)
			require.Error(t, err)
		})
	}
}

func TestParseSSECapturesErrorFramesWithoutExposingThemAsErrors(t *testing.T) {
	t.Parallel()

	frames, err := ParseSSE(
		strings.NewReader("event: error\ndata: {\"code\":\"runtime_failed\"}\n\n"),
		SSELimits{MaxBytes: 1024, MaxFrames: 2, MaxLineBytes: 128},
	)
	require.NoError(t, err)
	require.Equal(t, "error", frames[0].Event)
}

func TestParseSSEUntilStopsAtFrameBoundary(t *testing.T) {
	t.Parallel()

	raw := "id: 1\nevent: events\ndata: first\n\n" +
		"id: 2\nevent: events\ndata: second\n\n"
	frames, err := ParseSSEUntil(
		strings.NewReader(raw),
		SSELimits{MaxBytes: 1024, MaxFrames: 10, MaxLineBytes: 128},
		1,
	)
	require.NoError(t, err)
	require.Equal(t, []SSEFrame{{ID: "1", Event: "events", Data: []byte("first")}}, frames)
}
