// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package runtime

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
)

func TestDockerDriverRejectsNonUnixEndpointAndUnsafeSpecification(t *testing.T) {
	for _, endpoint := range []string{"", "http://127.0.0.1:2375", "unix://relative.sock", "tcp://127.0.0.1:2375"} {
		if _, err := NewDockerDriver(DockerDriverConfig{Endpoint: endpoint}); err == nil {
			t.Fatalf("NewDockerDriver(%q) unexpectedly succeeded", endpoint)
		}
	}
	driver, err := NewDockerDriver(DockerDriverConfig{Endpoint: "unix:///tmp/sandbox-runner.sock"})
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*Specification){
		"mutable image tag":      func(specification *Specification) { specification.ImageDigest = "registry.example/newx/runtime:latest" },
		"missing resource limit": func(specification *Specification) { specification.PIDLimit = 0 },
		"raw reuse key":          func(specification *Specification) { specification.ReuseKeyHash = "space-123-user-456" },
	} {
		t.Run(name, func(t *testing.T) {
			specification := validRuntimeSpecification()
			mutate(&specification)
			if _, err := driver.Create(context.Background(), specification); err == nil {
				t.Fatal("Create() unexpectedly accepted unsafe specification")
			}
		})
	}
}

func TestDockerDriverReliesOnExecutionContextInsteadOfGlobalClientTimeout(t *testing.T) {
	driver, err := NewDockerDriver(DockerDriverConfig{Endpoint: "unix:///tmp/sandbox-runner.sock"})
	if err != nil {
		t.Fatal(err)
	}
	if driver.client.Timeout != 0 {
		t.Fatalf("global HTTP timeout = %s; long executions must use their request context deadline", driver.client.Timeout)
	}
}

func TestDockerDriverCreatesStrictRootlessContainerWithNoNetwork(t *testing.T) {
	endpoint, requests, closeServer := newUnixDockerServer(t)
	defer closeServer()
	driver, err := NewDockerDriver(DockerDriverConfig{Endpoint: endpoint})
	if err != nil {
		t.Fatal(err)
	}
	container, err := driver.Create(context.Background(), validRuntimeSpecification())
	if err != nil || container.ID != "container-1" || container.State != ContainerStateRunning {
		t.Fatalf("Create() = %#v, %v; requests=%v", container, err, requests.paths())
	}
	create := requests.createBody(t)
	if create.Image != validRuntimeSpecification().ImageDigest || create.User != "10001:10001" || !create.ReadonlyRootfs || !create.NetworkDisabled || create.HostConfig.Privileged ||
		create.HostConfig.NetworkMode != "none" || create.HostConfig.UsernsMode != "private" || create.HostConfig.PidMode != "private" || create.HostConfig.IpcMode != "private" ||
		create.HostConfig.PidsLimit != int64(validRuntimeSpecification().PIDLimit) || create.HostConfig.Memory != int64(validRuntimeSpecification().MemoryLimitMB)*1024*1024 || create.HostConfig.NanoCPUs != int64(validRuntimeSpecification().CPUMilli)*1_000_000 {
		t.Fatalf("unsafe create request: %#v", create)
	}
	if len(create.HostConfig.Binds) != 0 || len(create.HostConfig.Devices) != 0 || !sameStrings(create.HostConfig.CapDrop, []string{"ALL"}) || !sameStrings(create.HostConfig.SecurityOpt, []string{"no-new-privileges", "seccomp=/etc/newx/seccomp/sandbox.json", "apparmor=newx-sandbox"}) {
		t.Fatalf("host isolation missing: %#v", create.HostConfig)
	}
	if create.HostConfig.Tmpfs["/tmp"] == "" || create.HostConfig.Tmpfs["/run/newx-secrets"] == "" || strings.Contains(strings.Join(flattenLabels(create.Labels), ","), "space-") {
		t.Fatalf("tmpfs or labels unsafe: %#v/%#v", create.HostConfig.Tmpfs, create.Labels)
	}
	if got := requests.paths(); !sameStrings(got, []string{"POST /v1.43/containers/create", "POST /v1.43/containers/container-1/start"}) {
		t.Fatalf("requests = %v", got)
	}
}

func TestDockerDriverAllowsNetworkOnlyThroughRunnerManagedEgressNetwork(t *testing.T) {
	endpoint, requests, closeServer := newUnixDockerServer(t)
	defer closeServer()
	driver, err := NewDockerDriver(DockerDriverConfig{Endpoint: endpoint, EgressNetwork: "newx-egress"})
	if err != nil {
		t.Fatal(err)
	}
	specification := validRuntimeSpecification()
	specification.AllowNetwork = true
	if _, err := driver.Create(context.Background(), specification); err != nil {
		t.Fatalf("Create() error = %v; requests=%v", err, requests.paths())
	}
	create := requests.createBody(t)
	if create.NetworkDisabled || create.HostConfig.NetworkMode != "newx-egress" {
		t.Fatalf("network request = %#v", create)
	}
	withoutEgress, err := NewDockerDriver(DockerDriverConfig{Endpoint: endpoint})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := withoutEgress.Create(context.Background(), specification); err == nil {
		t.Fatal("Create() unexpectedly allowed network without managed egress")
	}
}

func TestDockerDriverUsesOnlyFixedRuntimeMaintenanceCommands(t *testing.T) {
	endpoint, requests, closeServer := newUnixDockerServer(t)
	defer closeServer()
	driver, err := NewDockerDriver(DockerDriverConfig{Endpoint: endpoint})
	if err != nil {
		t.Fatal(err)
	}
	if err := driver.PrepareForReuse(context.Background(), "container-1"); err != nil {
		t.Fatalf("PrepareForReuse() error = %v; requests=%v", err, requests.paths())
	}
	if err := driver.Health(context.Background(), "container-1"); err != nil {
		t.Fatalf("Health() error = %v", err)
	}
	if got := requests.execCommands(); !sameStrings(got, []string{
		"/opt/newx/bin/runtime-cleanup",
		"/opt/newx/bin/runtime-health",
	}) {
		t.Fatalf("maintenance commands = %v", got)
	}
}

func TestDockerDriverRejectsFailedRuntimeMaintenanceCommand(t *testing.T) {
	endpoint, requests, closeServer := newUnixDockerServer(t)
	defer closeServer()
	requests.mu.Lock()
	requests.execExitCode = 1
	requests.mu.Unlock()
	driver, err := NewDockerDriver(DockerDriverConfig{Endpoint: endpoint})
	if err != nil {
		t.Fatal(err)
	}
	if err := driver.Health(context.Background(), "container-1"); err == nil {
		t.Fatal("Health() unexpectedly accepted a failed health command")
	}
}

func TestDockerDriverStagesBoundedInputAndRunsOnlyFixedAdapter(t *testing.T) {
	endpoint, requests, closeServer := newUnixDockerServer(t)
	defer closeServer()
	requests.setExecOutput(dockerMultiplexedOutput(1, []byte("adapter output")))
	driver, err := NewDockerDriver(DockerDriverConfig{Endpoint: endpoint})
	if err != nil {
		t.Fatal(err)
	}
	input := []byte(`{"request":"server-owned payload"}`)
	if err := driver.StageAdapterInput(context.Background(), "container-1", input); err != nil {
		t.Fatalf("StageAdapterInput() error = %v; requests=%v", err, requests.paths())
	}
	result, err := driver.RunAdapter(context.Background(), "container-1", AdapterAgentCode, 1024)
	if err != nil {
		t.Fatalf("RunAdapter() error = %v; requests=%v", err, requests.paths())
	}
	if result.ExitCode != 0 || string(result.Stdout) != "adapter output" || len(result.Stderr) != 0 {
		t.Fatalf("adapter result = %#v", result)
	}
	if got := requests.execCommands(); !sameStrings(got, []string{"/opt/newx/bin/adapter-agent-code"}) {
		t.Fatalf("adapter command = %v", got)
	}
	if got := requests.archiveInput(t); !bytes.Equal(got, input) {
		t.Fatalf("staged input = %q, want %q", got, input)
	}
}

func TestDockerDriverRejectsUnreviewedAdapterWithoutCallingRuntime(t *testing.T) {
	endpoint, requests, closeServer := newUnixDockerServer(t)
	defer closeServer()
	driver, err := NewDockerDriver(DockerDriverConfig{Endpoint: endpoint})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := driver.RunAdapter(context.Background(), "container-1", Adapter("shell"), 1024); err == nil {
		t.Fatal("RunAdapter() unexpectedly accepted unreviewed adapter")
	}
	if got := requests.paths(); len(got) != 0 {
		t.Fatalf("unreviewed adapter made runtime calls: %v", got)
	}
}

func TestDockerContainerSummaryReturnsOnlyManagedContainersAndQuarantinesInvalidLabels(t *testing.T) {
	valid := validRuntimeSpecification()
	request := newDockerCreateRequest(valid, "")
	if _, ok := (dockerContainerSummary{ID: "unrelated", Image: valid.ImageDigest}).container(); ok {
		t.Fatal("unlabeled container must not be managed")
	}
	invalidLabels := make(map[string]string, len(request.Labels))
	for key, value := range request.Labels {
		invalidLabels[key] = value
	}
	invalidLabels[labelCPUMilli] = "0"
	container, ok := (dockerContainerSummary{ID: "old-runner-container", Image: valid.ImageDigest, Labels: invalidLabels, State: "running"}).container()
	if !ok || container.ID != "old-runner-container" || container.State != ContainerStateStopped || container.Spec.ReuseKeyHash != "" {
		t.Fatalf("invalid managed container = %#v, %v", container, ok)
	}
}

func TestDockerContainerSummaryNeverTreatsPausedContainerAsReusable(t *testing.T) {
	specification := validRuntimeSpecification()
	request := newDockerCreateRequest(specification, "")
	container, ok := (dockerContainerSummary{ID: "paused-container", Image: specification.ImageDigest, Labels: request.Labels, State: "paused"}).container()
	if !ok || container.State != ContainerStateStopped {
		t.Fatalf("paused container = %#v, %v", container, ok)
	}
}

type capturedDockerRequests struct {
	mu           sync.Mutex
	requests     []capturedDockerRequest
	execExitCode int
	execOutput   []byte
}
type capturedDockerRequest struct {
	method string
	path   string
	body   []byte
}

func (requests *capturedDockerRequests) paths() []string {
	requests.mu.Lock()
	defer requests.mu.Unlock()
	values := make([]string, 0, len(requests.requests))
	for _, request := range requests.requests {
		values = append(values, request.method+" "+request.path)
	}
	return values
}
func (requests *capturedDockerRequests) createBody(t *testing.T) dockerCreateRequest {
	t.Helper()
	requests.mu.Lock()
	defer requests.mu.Unlock()
	for _, request := range requests.requests {
		if request.path == "/v1.43/containers/create" {
			var value dockerCreateRequest
			if err := json.Unmarshal(request.body, &value); err != nil {
				t.Fatal(err)
			}
			return value
		}
	}
	t.Fatal("create request was not captured")
	return dockerCreateRequest{}
}

func (requests *capturedDockerRequests) execCommands() []string {
	requests.mu.Lock()
	defer requests.mu.Unlock()
	commands := make([]string, 0, 2)
	for _, request := range requests.requests {
		if request.path != "/v1.43/containers/container-1/exec" {
			continue
		}
		var value struct {
			Cmd []string `json:"Cmd"`
		}
		if err := json.Unmarshal(request.body, &value); err == nil && len(value.Cmd) == 1 {
			commands = append(commands, value.Cmd[0])
		}
	}
	return commands
}

func (requests *capturedDockerRequests) setExecOutput(value []byte) {
	requests.mu.Lock()
	defer requests.mu.Unlock()
	requests.execOutput = append([]byte(nil), value...)
}

func (requests *capturedDockerRequests) archiveInput(t *testing.T) []byte {
	t.Helper()
	requests.mu.Lock()
	defer requests.mu.Unlock()
	for _, request := range requests.requests {
		if request.method != http.MethodPut || request.path != "/v1.43/containers/container-1/archive" {
			continue
		}
		reader := tar.NewReader(bytes.NewReader(request.body))
		header, err := reader.Next()
		if err != nil {
			t.Fatal(err)
		}
		if header.Name != runtimeAdapterInputFile || header.Mode != 0o400 {
			t.Fatalf("unsafe staged archive header: %#v", header)
		}
		value, err := io.ReadAll(reader)
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	t.Fatal("staged input archive was not captured")
	return nil
}

func newUnixDockerServer(t *testing.T) (string, *capturedDockerRequests, func()) {
	t.Helper()
	directory, err := os.MkdirTemp("/private/tmp", "newx-dr-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(directory) })
	path := filepath.Join(directory, "docker.sock")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	requests := &capturedDockerRequests{}
	server := &http.Server{Handler: http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		requests.mu.Lock()
		requests.requests = append(requests.requests, capturedDockerRequest{method: request.Method, path: request.URL.Path, body: body})
		requests.mu.Unlock()
		switch request.URL.Path {
		case "/v1.43/containers/create":
			writer.WriteHeader(http.StatusCreated)
			_, _ = writer.Write([]byte(`{"Id":"container-1"}`))
		case "/v1.43/containers/container-1/start":
			writer.WriteHeader(http.StatusNoContent)
		case "/v1.43/containers/container-1/archive":
			writer.WriteHeader(http.StatusOK)
		case "/v1.43/containers/container-1/exec":
			writer.WriteHeader(http.StatusCreated)
			_, _ = writer.Write([]byte(`{"Id":"exec-1"}`))
		case "/v1.43/exec/exec-1/start":
			requests.mu.Lock()
			output := append([]byte(nil), requests.execOutput...)
			requests.mu.Unlock()
			writer.WriteHeader(http.StatusOK)
			_, _ = writer.Write(output)
		case "/v1.43/exec/exec-1/json":
			requests.mu.Lock()
			exitCode := requests.execExitCode
			requests.mu.Unlock()
			_, _ = writer.Write([]byte(`{"Running":false,"ExitCode":` + strconv.Itoa(exitCode) + `}`))
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	})}
	go func() { _ = server.Serve(listener) }()
	return "unix://" + path, requests, func() { _ = server.Close(); _ = listener.Close() }
}

func dockerMultiplexedOutput(stream byte, value []byte) []byte {
	output := make([]byte, 8+len(value))
	output[0] = stream
	output[7] = byte(len(value))
	copy(output[8:], value)
	return output
}

func validRuntimeSpecification() Specification {
	return Specification{ReuseKeyHash: strings.Repeat("a", 64), Scope: "plugin", ImageDigest: "registry.example/newx/runtime@sha256:" + strings.Repeat("b", 64), PolicyVersion: "policy-v1", SchedulerVersion: 7, CredentialGeneration: "credential-v1", DeploymentID: "runner-test", CPUMilli: 400, MemoryLimitMB: 384, PIDLimit: 64}
}
func flattenLabels(labels map[string]string) []string {
	values := make([]string, 0, len(labels))
	for key, value := range labels {
		values = append(values, key+"="+value)
	}
	return values
}
func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
