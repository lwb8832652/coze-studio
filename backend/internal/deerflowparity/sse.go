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
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
)

type SSELimits struct {
	MaxBytes     int64
	MaxFrames    int
	MaxLineBytes int
}

type SSEFrame struct {
	ID    string
	Event string
	Data  []byte
}

func ParseSSE(reader io.Reader, limits SSELimits) ([]SSEFrame, error) {
	return parseSSE(reader, limits, nil)
}

func ParseSSEUntil(reader io.Reader, limits SSELimits, stopAfterFrames int) ([]SSEFrame, error) {
	if stopAfterFrames <= 0 {
		return nil, errors.New("SSE stop frame count must be positive")
	}
	return parseSSE(reader, limits, func(frames []SSEFrame) bool {
		return len(frames) >= stopAfterFrames
	})
}

func ParseSSEUntilEvents(reader io.Reader, limits SSELimits, stopAfterEvents int) ([]SSEFrame, error) {
	if stopAfterEvents <= 0 {
		return nil, errors.New("SSE stop event count must be positive")
	}
	return parseSSE(reader, limits, func(frames []SSEFrame) bool {
		count := 0
		for _, frame := range frames {
			if frame.ID != "" && frame.Event != "metadata" && frame.Event != "end" && frame.Event != "error" {
				count++
			}
		}
		return count >= stopAfterEvents
	})
}

func parseSSE(reader io.Reader, limits SSELimits, shouldStop func([]SSEFrame) bool) ([]SSEFrame, error) {
	limits = normalizedSSELimits(limits)
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 1024), limits.MaxLineBytes)
	frames := make([]SSEFrame, 0, min(limits.MaxFrames, 128))
	var current SSEFrame
	dataLines := make([]string, 0, 4)
	var bytesRead int64
	dispatch := func() error {
		if current.ID == "" && current.Event == "" && len(dataLines) == 0 {
			return nil
		}
		if len(frames) >= limits.MaxFrames {
			return errors.New("SSE frame count exceeds limit")
		}
		current.Data = []byte(strings.Join(dataLines, "\n"))
		frames = append(frames, current)
		current = SSEFrame{}
		dataLines = dataLines[:0]
		return nil
	}

	for scanner.Scan() {
		line := scanner.Text()
		bytesRead += int64(len(line) + 1)
		if bytesRead > limits.MaxBytes {
			return nil, errors.New("SSE stream exceeds byte limit")
		}
		if line == "" {
			if err := dispatch(); err != nil {
				return nil, err
			}
			if shouldStop != nil && shouldStop(frames) {
				return frames, nil
			}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		field, value, found := strings.Cut(line, ":")
		if !found {
			field = line
			value = ""
		}
		value = strings.TrimPrefix(value, " ")
		switch field {
		case "id":
			if strings.ContainsRune(value, '\x00') {
				return nil, errors.New("SSE id contains NUL")
			}
			current.ID = value
		case "event":
			current.Event = value
		case "data":
			dataLines = append(dataLines, value)
		case "retry":
			continue
		default:
			return nil, fmt.Errorf("unsupported SSE field %q", field)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, errors.New("SSE stream contains an oversized or unreadable line")
	}
	if err := dispatch(); err != nil {
		return nil, err
	}
	for _, frame := range frames {
		if len(frame.Data) > limits.MaxLineBytes || bytes.ContainsRune(frame.Data, '\x00') {
			return nil, errors.New("SSE data is invalid")
		}
	}
	return frames, nil
}

func normalizedSSELimits(limits SSELimits) SSELimits {
	if limits.MaxBytes <= 0 {
		limits.MaxBytes = defaultMaxSSEBytes
	}
	if limits.MaxFrames <= 0 {
		limits.MaxFrames = defaultMaxSSEFrames
	}
	if limits.MaxLineBytes <= 0 {
		limits.MaxLineBytes = defaultMaxSSELineBytes
	}
	return limits
}
