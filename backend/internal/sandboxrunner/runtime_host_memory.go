// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandboxrunner

import (
	"bufio"
	"context"
	"errors"
	"os"
	"strconv"
	"strings"
)

// HostMemorySampler reads only the aggregate available-memory value needed by
// admission control. It never exposes host process, cgroup, or filesystem
// details through the Runner API.
type HostMemorySampler struct {
	readFile func(string) (*os.File, error)
}

func NewHostMemorySampler() *HostMemorySampler {
	return &HostMemorySampler{readFile: os.Open}
}

func (sampler *HostMemorySampler) AvailableMemoryMB(ctx context.Context) (int, error) {
	if sampler == nil || sampler.readFile == nil || ctx == nil || ctx.Err() != nil {
		return 0, ErrUnavailable
	}
	file, err := sampler.readFile("/proc/meminfo")
	if err != nil {
		return 0, ErrUnavailable
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		fields := strings.Fields(scanner.Text())
		if len(fields) != 3 || fields[0] != "MemAvailable:" || fields[2] != "kB" {
			continue
		}
		kilobytes, err := strconv.Atoi(fields[1])
		if err != nil || kilobytes < 0 {
			return 0, ErrUnavailable
		}
		return kilobytes / 1024, nil
	}
	if err := scanner.Err(); err != nil {
		return 0, ErrUnavailable
	}
	return 0, errors.New("sandbox runner available memory is unavailable")
}
