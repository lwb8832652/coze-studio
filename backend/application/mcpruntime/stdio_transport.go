// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package mcpruntime

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os/exec"
	"sync"
	"time"

	mcptransport "github.com/mark3labs/mcp-go/client/transport"
	mcpsdk "github.com/mark3labs/mcp-go/mcp"
)

const (
	defaultStdioMaxFrameBytes = 8 << 20
	maximumStdioMaxFrameBytes = 64 << 20
)

type StdioTransportOptions struct {
	Command               string
	Args                  []string
	Env                   []string
	WorkingDir            string
	CommandPolicy         *StdioCommandPolicy
	ExecutionMode         StdioExecutionMode
	MaxFrameBytes         int
	ProcessTerminateGrace time.Duration
	ProcessKillWait       time.Duration
}

type stdioResponseDelivery struct {
	response *mcptransport.JSONRPCResponse
	err      error
}

type safeStdioTransport struct {
	command               string
	args                  []string
	env                   []string
	workingDir            string
	commandPolicy         *StdioCommandPolicy
	invocation            ResolvedStdioInvocation
	maxFrameBytes         int
	inbound               *stdioInboundLimiter
	inboundCloseWait      time.Duration
	processTerminateGrace time.Duration
	processKillWait       time.Duration

	stateMu             sync.Mutex
	started             bool
	closed              bool
	terminal            error
	ctx                 context.Context
	cancel              context.CancelFunc
	cmd                 *exec.Cmd
	stdin               io.WriteCloser
	stdout              io.ReadCloser
	stderr              io.ReadCloser
	responses           map[string]chan stdioResponseDelivery
	done                chan struct{}
	readDone            chan struct{}
	drainDone           chan struct{}
	waitDone            chan struct{}
	processID           int
	doneOnce            sync.Once
	closeOnce           sync.Once
	terminateMu         sync.Mutex
	terminated          bool
	primaryWaitComplete bool
	escapedProcessIDs   []int
	inspectionUncertain bool
	inspectionFailure   error

	writeMu sync.Mutex

	notificationMu      sync.RWMutex
	notificationHandler func(notification mcpsdk.JSONRPCNotification)
	requestMu           sync.RWMutex
	requestHandler      mcptransport.RequestHandler
}

func NewSafeStdioTransport(options StdioTransportOptions) (mcptransport.Interface, error) {
	if !options.ExecutionMode.AllowsHostExecution() {
		return nil, ErrStdioHostExecutionDenied
	}
	maxFrameBytes := options.MaxFrameBytes
	if maxFrameBytes <= 0 {
		maxFrameBytes = defaultStdioMaxFrameBytes
	}
	if maxFrameBytes > maximumStdioMaxFrameBytes {
		return nil, ErrInvalidConnection
	}
	terminateGrace := options.ProcessTerminateGrace
	if terminateGrace <= 0 {
		terminateGrace = 250 * time.Millisecond
	}
	killWait := options.ProcessKillWait
	if killWait <= 0 {
		killWait = 5 * time.Second
	}
	command := options.Command
	args := append([]string(nil), options.Args...)
	var invocation ResolvedStdioInvocation
	if options.CommandPolicy != nil {
		resolved, err := options.CommandPolicy.ResolveInvocation(command, args, options.WorkingDir)
		if err != nil {
			return nil, err
		}
		invocation = resolved
		command = resolved.Command
		args = append([]string(nil), resolved.Args...)
	} else {
		resolved, err := inspectExecutable(command)
		if err != nil {
			return nil, ErrStdioCommandDenied
		}
		invocation = ResolvedStdioInvocation{
			Command: resolved.path, Args: args, executable: resolved,
		}
		command = resolved.path
	}
	if _, err := NewCommandContextWithEnv(
		context.Background(),
		command,
		args,
		options.Env,
		options.WorkingDir,
	); err != nil {
		return nil, err
	}
	return &safeStdioTransport{
		command:               command,
		args:                  args,
		env:                   append([]string(nil), options.Env...),
		workingDir:            options.WorkingDir,
		commandPolicy:         options.CommandPolicy,
		invocation:            invocation,
		maxFrameBytes:         maxFrameBytes,
		inbound:               newStdioInboundLimiter(),
		inboundCloseWait:      defaultStdioInboundCloseWait,
		processTerminateGrace: terminateGrace,
		processKillWait:       killWait,
		responses:             make(map[string]chan stdioResponseDelivery),
		done:                  make(chan struct{}),
		readDone:              make(chan struct{}),
		drainDone:             make(chan struct{}),
		waitDone:              make(chan struct{}),
	}, nil
}

func (t *safeStdioTransport) Start(ctx context.Context) error {
	if t == nil {
		return ErrSessionUnavailable
	}
	t.stateMu.Lock()
	defer t.stateMu.Unlock()
	if t.closed {
		return ErrSessionUnavailable
	}
	if t.started {
		return nil
	}
	command := t.command
	args := append([]string(nil), t.args...)
	if t.commandPolicy != nil {
		resolved, err := t.commandPolicy.ResolveInvocation(command, args, t.workingDir)
		if err != nil || !sameResolvedStdioInvocation(t.invocation, resolved) {
			return ErrStdioCommandDenied
		}
		command = resolved.Command
		args = append([]string(nil), resolved.Args...)
	} else {
		current, err := inspectExecutable(command)
		if err != nil || !sameExecutableSnapshot(t.invocation.executable, current) {
			return ErrStdioCommandDenied
		}
		command = current.path
	}
	runCtx, cancel := context.WithCancel(ctx)
	cmd, err := NewCommandContextWithEnv(runCtx, command, args, t.env, t.workingDir)
	if err != nil {
		cancel()
		return ErrSessionUnavailable
	}
	if err := configureStdioDebugProcessGroup(cmd); err != nil {
		cancel()
		return ErrSessionUnavailable
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return ErrSessionUnavailable
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		cancel()
		return ErrSessionUnavailable
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		cancel()
		return ErrSessionUnavailable
	}
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		_ = stderr.Close()
		cancel()
		return ErrSessionUnavailable
	}
	t.ctx = runCtx
	t.cancel = cancel
	t.cmd = cmd
	t.processID = cmd.Process.Pid
	t.stdin = stdin
	t.stdout = stdout
	t.stderr = stderr
	t.started = true
	go t.drainStderr(stderr)
	go t.readResponses(stdout)
	go func() {
		_ = cmd.Wait()
		close(t.waitDone)
	}()
	return nil
}

func (t *safeStdioTransport) SendRequest(
	ctx context.Context,
	request mcptransport.JSONRPCRequest,
) (*mcptransport.JSONRPCResponse, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	encoded, err := json.Marshal(request)
	if err != nil || len(encoded) > t.maxFrameBytes {
		return nil, ErrStdioFrameLimitExceeded
	}
	id := request.ID.String()
	delivery := make(chan stdioResponseDelivery, 1)
	t.stateMu.Lock()
	if !t.started || t.closed || t.terminal != nil || t.stdin == nil {
		err = t.fixedTerminalLocked()
		t.stateMu.Unlock()
		return nil, err
	}
	if _, duplicate := t.responses[id]; duplicate {
		t.stateMu.Unlock()
		return nil, ErrSessionUnavailable
	}
	t.responses[id] = delivery
	t.stateMu.Unlock()

	if err := t.writeFrame(encoded); err != nil {
		t.removeResponse(id)
		return nil, err
	}
	select {
	case <-ctx.Done():
		t.removeResponse(id)
		return nil, ctx.Err()
	case result := <-delivery:
		return result.response, result.err
	case <-t.done:
		t.removeResponse(id)
		return nil, t.fixedTerminal()
	}
}

func (t *safeStdioTransport) SendNotification(
	ctx context.Context,
	notification mcpsdk.JSONRPCNotification,
) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	encoded, err := json.Marshal(notification)
	if err != nil || len(encoded) > t.maxFrameBytes {
		return ErrStdioFrameLimitExceeded
	}
	return t.writeFrame(encoded)
}

func (t *safeStdioTransport) SetNotificationHandler(
	handler func(notification mcpsdk.JSONRPCNotification),
) {
	t.notificationMu.Lock()
	t.notificationHandler = handler
	t.notificationMu.Unlock()
}

func (t *safeStdioTransport) SetRequestHandler(handler mcptransport.RequestHandler) {
	t.requestMu.Lock()
	t.requestHandler = handler
	t.requestMu.Unlock()
}

func (t *safeStdioTransport) Close() error {
	if t == nil {
		return nil
	}
	t.closeOnce.Do(func() {
		t.stateMu.Lock()
		t.closed = true
		t.stateMu.Unlock()
		t.finish(ErrSessionUnavailable)
		if t.inbound != nil {
			t.inbound.close()
			_ = t.inbound.wait(t.inboundCloseWait)
		}
	})
	return t.terminateProcessGroupAndWait()
}

func (t *safeStdioTransport) terminateProcessGroupAndWait() error {
	t.terminateMu.Lock()
	defer t.terminateMu.Unlock()
	if t.terminated {
		return nil
	}
	if t.primaryWaitComplete {
		terminateStdioDebugEscapedProcesses(t.escapedProcessIDs)
		if t.inspectionUncertain || !waitStdioDebugProcessesGone(t.escapedProcessIDs, t.processKillWait) {
			return ErrSessionUnavailable
		}
		t.terminated = true
		return nil
	}
	t.stateMu.Lock()
	started := t.started
	processID := t.processID
	cancel := t.cancel
	stdin := t.stdin
	stdout := t.stdout
	stderr := t.stderr
	waitDone := t.waitDone
	readDone := t.readDone
	drainDone := t.drainDone
	t.stateMu.Unlock()
	if !started {
		t.terminated = true
		return nil
	}
	escapedProcessIDs, inspectErr := inspectStdioDebugEscapedDescendants(processID)
	if inspectErr != nil {
		t.inspectionUncertain = true
		t.inspectionFailure = inspectErr
	} else if len(escapedProcessIDs) > 0 {
		t.escapedProcessIDs = append([]int(nil), escapedProcessIDs...)
	}
	if stdin != nil {
		_ = stdin.Close()
	}
	if processID > 0 {
		_ = signalStdioDebugProcessGroup(processID, false)
	}
	timer := time.NewTimer(t.processTerminateGrace)
	<-timer.C
	if processID > 0 {
		_ = signalStdioDebugProcessGroup(processID, true)
	}
	terminateStdioDebugEscapedProcesses(t.escapedProcessIDs)
	if cancel != nil {
		cancel()
	}
	if stdout != nil {
		_ = stdout.Close()
	}
	if stderr != nil {
		_ = stderr.Close()
	}
	deadline := time.NewTimer(t.processKillWait)
	defer deadline.Stop()
	if !waitForStdioProcessChannel(waitDone, deadline.C) ||
		!waitForStdioProcessChannel(readDone, deadline.C) ||
		!waitForStdioProcessChannel(drainDone, deadline.C) {
		return ErrSessionUnavailable
	}
	t.primaryWaitComplete = true
	if t.inspectionUncertain || len(t.escapedProcessIDs) > 0 {
		return ErrSessionUnavailable
	}
	t.terminated = true
	return nil
}

func waitForStdioProcessChannel(done <-chan struct{}, timeout <-chan time.Time) bool {
	select {
	case <-done:
		return true
	case <-timeout:
		return false
	}
}

func (t *safeStdioTransport) GetSessionId() string {
	return ""
}

func (t *safeStdioTransport) readResponses(stdout io.Reader) {
	defer close(t.readDone)
	reader := bufio.NewReaderSize(stdout, t.maxFrameBytes+2)
	for {
		line, readErr := reader.ReadSlice('\n')
		if readErr == bufio.ErrBufferFull {
			t.abort(ErrStdioFrameLimitExceeded)
			return
		}
		if len(line) > 0 {
			frame := trimStdioFrameEnding(line)
			if len(frame) > t.maxFrameBytes {
				t.abort(ErrStdioFrameLimitExceeded)
				return
			}
			if len(frame) > 0 {
				t.handleFrame(frame)
			}
		}
		switch readErr {
		case nil:
			continue
		case io.EOF:
			t.finish(ErrSessionUnavailable)
			return
		default:
			t.finish(ErrSessionUnavailable)
			return
		}
	}
}

func (t *safeStdioTransport) handleFrame(frame []byte) {
	var base struct {
		ID     *mcpsdk.RequestId `json:"id,omitempty"`
		Method string            `json:"method,omitempty"`
	}
	if json.Unmarshal(frame, &base) != nil {
		return
	}
	if base.Method != "" && base.ID == nil {
		var notification mcpsdk.JSONRPCNotification
		if json.Unmarshal(frame, &notification) != nil {
			return
		}
		t.notificationMu.RLock()
		handler := t.notificationHandler
		t.notificationMu.RUnlock()
		if handler != nil {
			handler(notification)
		}
		return
	}
	if base.Method != "" && base.ID != nil {
		var request mcptransport.JSONRPCRequest
		if json.Unmarshal(frame, &request) == nil {
			t.handleIncomingRequest(request)
		}
		return
	}
	var response mcptransport.JSONRPCResponse
	if json.Unmarshal(frame, &response) != nil {
		return
	}
	id := response.ID.String()
	t.stateMu.Lock()
	channel, exists := t.responses[id]
	if exists {
		delete(t.responses, id)
	}
	t.stateMu.Unlock()
	if exists {
		channel <- stdioResponseDelivery{response: &response}
	}
}

func (t *safeStdioTransport) handleIncomingRequest(request mcptransport.JSONRPCRequest) {
	if t.inbound == nil || !t.inbound.tryAcquire(time.Now()) {
		t.abort(ErrStdioInboundRequestLimitExceeded)
		return
	}
	t.requestMu.RLock()
	handler := t.requestHandler
	t.requestMu.RUnlock()
	go func() {
		defer t.inbound.release()
		if handler == nil {
			t.sendResponse(*mcptransport.NewJSONRPCErrorResponse(
				request.ID,
				mcpsdk.METHOD_NOT_FOUND,
				"No request handler configured",
				nil,
			))
			return
		}
		t.stateMu.Lock()
		ctx := t.ctx
		t.stateMu.Unlock()
		if ctx == nil {
			ctx = context.Background()
		}
		response, err := handler(ctx, request)
		if err != nil {
			t.sendResponse(*mcptransport.NewJSONRPCErrorResponse(
				request.ID,
				mcpsdk.INTERNAL_ERROR,
				"request handler failed",
				nil,
			))
			return
		}
		if response != nil {
			t.sendResponse(*response)
		}
	}()
}

func (t *safeStdioTransport) sendResponse(response mcptransport.JSONRPCResponse) {
	encoded, err := json.Marshal(response)
	if err != nil || len(encoded) > t.maxFrameBytes {
		t.abort(ErrStdioFrameLimitExceeded)
		return
	}
	_ = t.writeFrame(encoded)
}

func (t *safeStdioTransport) writeFrame(encoded []byte) error {
	if len(encoded) > t.maxFrameBytes {
		return ErrStdioFrameLimitExceeded
	}
	t.stateMu.Lock()
	stdin := t.stdin
	if !t.started || t.closed || t.terminal != nil || stdin == nil {
		err := t.fixedTerminalLocked()
		t.stateMu.Unlock()
		return err
	}
	t.stateMu.Unlock()

	frame := make([]byte, len(encoded)+1)
	copy(frame, encoded)
	frame[len(encoded)] = '\n'
	t.writeMu.Lock()
	defer t.writeMu.Unlock()
	for len(frame) > 0 {
		written, err := stdin.Write(frame)
		if err != nil || written <= 0 {
			return ErrSessionUnavailable
		}
		frame = frame[written:]
	}
	return nil
}

func (t *safeStdioTransport) removeResponse(id string) {
	t.stateMu.Lock()
	delete(t.responses, id)
	t.stateMu.Unlock()
}

func (t *safeStdioTransport) abort(err error) {
	if t.inbound != nil {
		t.inbound.close()
	}
	t.stateMu.Lock()
	cancel := t.cancel
	stdin := t.stdin
	stdout := t.stdout
	stderr := t.stderr
	t.stateMu.Unlock()
	if cancel != nil {
		cancel()
	}
	if stdin != nil {
		_ = stdin.Close()
	}
	if stdout != nil {
		_ = stdout.Close()
	}
	if stderr != nil {
		_ = stderr.Close()
	}
	t.finish(err)
	go func() { _ = t.Close() }()
}

func (t *safeStdioTransport) finish(err error) {
	if !errors.Is(err, ErrStdioFrameLimitExceeded) &&
		!errors.Is(err, ErrStdioInboundRequestLimitExceeded) {
		err = ErrSessionUnavailable
	}
	t.stateMu.Lock()
	if t.terminal == nil {
		t.terminal = err
	}
	fixed := t.terminal
	channels := make([]chan stdioResponseDelivery, 0, len(t.responses))
	for id, channel := range t.responses {
		channels = append(channels, channel)
		delete(t.responses, id)
	}
	t.stateMu.Unlock()
	for _, channel := range channels {
		channel <- stdioResponseDelivery{err: fixed}
	}
	t.doneOnce.Do(func() { close(t.done) })
}

func (t *safeStdioTransport) fixedTerminal() error {
	t.stateMu.Lock()
	defer t.stateMu.Unlock()
	return t.fixedTerminalLocked()
}

func (t *safeStdioTransport) fixedTerminalLocked() error {
	if errors.Is(t.terminal, ErrStdioFrameLimitExceeded) {
		return ErrStdioFrameLimitExceeded
	}
	if errors.Is(t.terminal, ErrStdioInboundRequestLimitExceeded) {
		return ErrStdioInboundRequestLimitExceeded
	}
	return ErrSessionUnavailable
}

func (t *safeStdioTransport) drainStderr(stderr io.Reader) {
	defer close(t.drainDone)
	_, _ = io.Copy(io.Discard, stderr)
}

func trimStdioFrameEnding(frame []byte) []byte {
	if len(frame) > 0 && frame[len(frame)-1] == '\n' {
		frame = frame[:len(frame)-1]
	}
	if len(frame) > 0 && frame[len(frame)-1] == '\r' {
		frame = frame[:len(frame)-1]
	}
	return frame
}

var _ mcptransport.BidirectionalInterface = (*safeStdioTransport)(nil)
