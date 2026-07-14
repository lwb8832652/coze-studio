//go:build darwin || linux

// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package mcpruntime

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	stdioDebugProcessInspectionTimeout = 500 * time.Millisecond
	stdioDebugProcessListMaxBytes      = 1 << 20
)

var (
	errStdioDebugProcessInspectionUnavailable = errors.New("debug process inspection unavailable")
	errStdioDebugProcessInspectionMalformed   = errors.New("debug process inspection malformed")
	errStdioDebugProcessRootMissing           = errors.New("debug process root missing")
)

type stdioDebugProcessRecord struct {
	pid  int
	ppid int
	pgid int
}

type stdioDebugCappedBuffer struct {
	buffer bytes.Buffer
	limit  int
}

func (b *stdioDebugCappedBuffer) Write(payload []byte) (int, error) {
	if b == nil || len(payload) > b.limit-b.buffer.Len() {
		return 0, ErrSessionUnavailable
	}
	return b.buffer.Write(payload)
}

func inspectStdioDebugEscapedDescendants(rootPID int) ([]int, error) {
	if rootPID <= 0 {
		return nil, errStdioDebugProcessRootMissing
	}
	ctx, cancel := context.WithTimeout(context.Background(), stdioDebugProcessInspectionTimeout)
	defer cancel()
	command := exec.CommandContext(ctx, "/bin/ps", "-axo", "pid=,ppid=,pgid=")
	command.Env = []string{}
	output := &stdioDebugCappedBuffer{limit: stdioDebugProcessListMaxBytes}
	command.Stdout = output
	command.Stderr = io.Discard
	if err := command.Run(); err != nil || ctx.Err() != nil {
		return nil, errStdioDebugProcessInspectionUnavailable
	}
	records := make(map[int]stdioDebugProcessRecord)
	children := make(map[int][]int)
	for _, line := range strings.Split(output.buffer.String(), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 3 {
			continue
		}
		pid, pidErr := strconv.Atoi(fields[0])
		ppid, ppidErr := strconv.Atoi(fields[1])
		pgid, pgidErr := strconv.Atoi(fields[2])
		if pidErr != nil || ppidErr != nil || pgidErr != nil || pid <= 0 || ppid < 0 || pgid <= 0 {
			return nil, errStdioDebugProcessInspectionMalformed
		}
		records[pid] = stdioDebugProcessRecord{pid: pid, ppid: ppid, pgid: pgid}
		children[ppid] = append(children[ppid], pid)
	}
	if _, exists := records[rootPID]; !exists {
		return nil, errStdioDebugProcessRootMissing
	}
	queue := append([]int(nil), children[rootPID]...)
	escaped := make([]int, 0)
	seen := map[int]struct{}{rootPID: {}}
	for len(queue) > 0 {
		pid := queue[0]
		queue = queue[1:]
		if _, exists := seen[pid]; exists {
			continue
		}
		seen[pid] = struct{}{}
		record, exists := records[pid]
		if !exists {
			return nil, errStdioDebugProcessInspectionMalformed
		}
		if record.pgid != rootPID {
			escaped = append(escaped, pid)
		}
		queue = append(queue, children[pid]...)
	}
	return escaped, nil
}

func terminateStdioDebugEscapedProcesses(processIDs []int) {
	for _, pid := range processIDs {
		if pid > 0 {
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
	}
}

func waitStdioDebugProcessesGone(processIDs []int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		alive := false
		for _, pid := range processIDs {
			err := syscall.Kill(pid, 0)
			if err == nil || errors.Is(err, syscall.EPERM) {
				alive = true
				break
			}
		}
		if !alive {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(5 * time.Millisecond)
	}
}
