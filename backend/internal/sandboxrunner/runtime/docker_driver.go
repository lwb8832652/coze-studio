// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package runtime

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var errDriverConfiguration = errors.New("sandbox runtime driver configuration is invalid")
var errDriverUnavailable = errors.New("sandbox runtime driver is unavailable")

const dockerAPIPrefix = "/v1.43"

const (
	runtimeUser             = "10001:10001"
	runtimeCleanupCommand   = "/opt/newx/bin/runtime-cleanup"
	runtimeHealthCommand    = "/opt/newx/bin/runtime-health"
	runtimeAdapterInputDir  = "/tmp"
	runtimeAdapterInputFile = "newx-input.json"
	runtimeAdapterInputPath = runtimeAdapterInputDir + "/" + runtimeAdapterInputFile
	maxAdapterInputBytes    = 1 << 20
	maxAdapterOutputBytes   = 8 << 20
	labelReuseKeyHash       = "com.newx.sandbox.reuse-key-hash"
	labelScope              = "com.newx.sandbox.scope"
	labelImageDigest        = "com.newx.sandbox.image-digest"
	labelPolicyVersion      = "com.newx.sandbox.policy-version"
	labelSchedulerVersion   = "com.newx.sandbox.scheduler-version"
	labelCredentialVersion  = "com.newx.sandbox.credential-generation"
	labelDeploymentID       = "com.newx.sandbox.deployment-id"
	labelCPUMilli           = "com.newx.sandbox.cpu-milli"
	labelMemoryLimitMB      = "com.newx.sandbox.memory-limit-mb"
	labelPIDLimit           = "com.newx.sandbox.pid-limit"
	labelAllowNetwork       = "com.newx.sandbox.allow-network"
)

type DockerDriverConfig struct {
	Endpoint      string
	EgressNetwork string
}

// DockerDriver talks only to a dedicated rootless Docker-compatible Unix
// socket. It deliberately never reads DOCKER_HOST or environment overrides.
type DockerDriver struct {
	client        *http.Client
	egressNetwork string
}

func NewDockerDriver(config DockerDriverConfig) (*DockerDriver, error) {
	socketPath, ok := unixSocketPath(config.Endpoint)
	if !ok || (config.EgressNetwork != "" && !validRuntimeIdentifier(config.EgressNetwork)) {
		return nil, errDriverConfiguration
	}
	dialer := &net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
		return dialer.DialContext(ctx, "unix", socketPath)
	}
	// Execution and maintenance calls carry operation-specific context
	// deadlines. A client-wide timeout would abort valid long-running Docker
	// exec streams independently of the signed execution policy.
	return &DockerDriver{client: &http.Client{Transport: transport}, egressNetwork: config.EgressNetwork}, nil
}

func (driver *DockerDriver) Create(ctx context.Context, specification Specification) (Container, error) {
	if driver == nil || ctx == nil || !validSpecification(specification) || specification.AllowNetwork && driver.egressNetwork == "" {
		return Container{}, errDriverConfiguration
	}
	request := newDockerCreateRequest(specification, driver.egressNetwork)
	var response struct {
		ID string `json:"Id"`
	}
	if err := driver.requestJSON(ctx, http.MethodPost, dockerAPIPrefix+"/containers/create", request, &response, http.StatusCreated); err != nil || !validContainerID(response.ID) {
		return Container{}, errDriverUnavailable
	}
	if err := driver.requestJSON(ctx, http.MethodPost, dockerAPIPrefix+"/containers/"+url.PathEscape(response.ID)+"/start", nil, nil, http.StatusNoContent); err != nil {
		_ = driver.Destroy(context.Background(), response.ID)
		return Container{}, errDriverUnavailable
	}
	return Container{ID: response.ID, Spec: specification, State: ContainerStateRunning}, nil
}

// PrepareForReuse can invoke only the reviewed cleanup binary baked into the
// runtime image. The Driver interface exposes no caller-supplied command.
func (driver *DockerDriver) PrepareForReuse(ctx context.Context, id string) error {
	return driver.runFixedMaintenance(ctx, id, runtimeCleanupCommand)
}

func (driver *DockerDriver) Health(ctx context.Context, id string) error {
	return driver.runFixedMaintenance(ctx, id, runtimeHealthCommand)
}

// StageAdapterInput writes the server-owned canonical request to one fixed
// file in the container tmpfs. The archive has no caller-controlled paths,
// modes, links, or metadata.
func (driver *DockerDriver) StageAdapterInput(ctx context.Context, id string, input []byte) error {
	if driver == nil || ctx == nil || !validContainerID(id) || len(input) == 0 || len(input) > maxAdapterInputBytes {
		return errDriverConfiguration
	}
	archive, err := fixedInputArchive(input)
	if err != nil {
		return errDriverConfiguration
	}
	if _, err := driver.requestBytes(ctx, http.MethodPut, dockerAPIPrefix+"/containers/"+url.PathEscape(id)+"/archive?path="+url.QueryEscape(runtimeAdapterInputDir), archive, "application/x-tar", http.StatusOK, 0); err != nil {
		return errDriverUnavailable
	}
	return nil
}

// RunAdapter invokes an image-baked adapter selected by a closed enum. It
// never accepts a command, argument, shell, or host path from the caller.
func (driver *DockerDriver) RunAdapter(ctx context.Context, id string, adapter Adapter, maxOutputBytes int64) (AdapterResult, error) {
	if driver == nil || ctx == nil || !validContainerID(id) || maxOutputBytes <= 0 || maxOutputBytes > maxAdapterOutputBytes {
		return AdapterResult{}, errDriverConfiguration
	}
	command, ok := fixedAdapterCommand(adapter)
	if !ok {
		return AdapterResult{}, errDriverConfiguration
	}
	var created struct {
		ID string `json:"Id"`
	}
	request := dockerExecCreateRequest{AttachStdout: true, AttachStderr: true, User: runtimeUser, Cmd: []string{command}}
	if err := driver.requestJSON(ctx, http.MethodPost, dockerAPIPrefix+"/containers/"+url.PathEscape(id)+"/exec", request, &created, http.StatusCreated); err != nil || !validContainerID(created.ID) {
		return AdapterResult{}, errDriverUnavailable
	}
	// Docker multiplexes stdout and stderr into frames. Bound the wire response
	// as well as the decoded streams, so an unhealthy runtime cannot exhaust the
	// Runner process before validation.
	wireLimit := maxOutputBytes*8 + 8*1024
	wire, err := driver.requestBytes(ctx, http.MethodPost, dockerAPIPrefix+"/exec/"+url.PathEscape(created.ID)+"/start", mustJSON(dockerExecStartRequest{}), "application/json", http.StatusOK, wireLimit)
	if err != nil {
		return AdapterResult{}, errDriverUnavailable
	}
	stdout, stderr, err := decodeDockerMultiplexedOutput(wire, maxOutputBytes)
	if err != nil {
		return AdapterResult{}, errDriverUnavailable
	}
	var inspected struct {
		Running  bool `json:"Running"`
		ExitCode int  `json:"ExitCode"`
	}
	if err := driver.requestJSON(ctx, http.MethodGet, dockerAPIPrefix+"/exec/"+url.PathEscape(created.ID)+"/json", nil, &inspected, http.StatusOK); err != nil || inspected.Running || inspected.ExitCode < 0 {
		return AdapterResult{}, errDriverUnavailable
	}
	return AdapterResult{ExitCode: inspected.ExitCode, Stdout: stdout, Stderr: stderr}, nil
}

func (driver *DockerDriver) Terminate(ctx context.Context, id string) error {
	if !validContainerID(id) {
		return errDriverConfiguration
	}
	return driver.requestJSON(ctx, http.MethodPost, dockerAPIPrefix+"/containers/"+url.PathEscape(id)+"/kill?signal=SIGTERM", nil, nil, http.StatusNoContent)
}

func (driver *DockerDriver) WaitStopped(ctx context.Context, id string, grace time.Duration) (bool, error) {
	if driver == nil || ctx == nil || !validContainerID(id) || grace < 0 {
		return false, errDriverConfiguration
	}
	deadline := time.Now().Add(grace)
	for {
		container, err := driver.inspect(ctx, id)
		if err != nil {
			return false, errDriverUnavailable
		}
		if container.State != ContainerStateRunning {
			return true, nil
		}
		if !time.Now().Before(deadline) {
			return false, nil
		}
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func (driver *DockerDriver) ForceKill(ctx context.Context, id string) error {
	if !validContainerID(id) {
		return errDriverConfiguration
	}
	return driver.requestJSON(ctx, http.MethodPost, dockerAPIPrefix+"/containers/"+url.PathEscape(id)+"/kill?signal=SIGKILL", nil, nil, http.StatusNoContent)
}

func (driver *DockerDriver) Destroy(ctx context.Context, id string) error {
	if !validContainerID(id) {
		return errDriverConfiguration
	}
	return driver.requestJSON(ctx, http.MethodDelete, dockerAPIPrefix+"/containers/"+url.PathEscape(id)+"?force=true&v=true", nil, nil, http.StatusNoContent)
}

func (driver *DockerDriver) List(ctx context.Context) ([]Container, error) {
	if driver == nil || ctx == nil {
		return nil, errDriverConfiguration
	}
	var response []dockerContainerSummary
	if err := driver.requestJSON(ctx, http.MethodGet, dockerAPIPrefix+"/containers/json?all=true", nil, &response, http.StatusOK); err != nil {
		return nil, errDriverUnavailable
	}
	containers := make([]Container, 0, len(response))
	for _, item := range response {
		container, ok := item.container()
		if ok {
			containers = append(containers, container)
		}
	}
	return containers, nil
}

type dockerCreateRequest struct {
	Image           string            `json:"Image"`
	User            string            `json:"User"`
	ReadonlyRootfs  bool              `json:"ReadonlyRootfs"`
	NetworkDisabled bool              `json:"NetworkDisabled"`
	Labels          map[string]string `json:"Labels"`
	HostConfig      dockerHostConfig  `json:"HostConfig"`
}

type dockerHostConfig struct {
	Privileged  bool              `json:"Privileged"`
	NetworkMode string            `json:"NetworkMode"`
	UsernsMode  string            `json:"UsernsMode"`
	PidMode     string            `json:"PidMode"`
	IpcMode     string            `json:"IpcMode"`
	PidsLimit   int64             `json:"PidsLimit"`
	Memory      int64             `json:"Memory"`
	NanoCPUs    int64             `json:"NanoCpus"`
	CapDrop     []string          `json:"CapDrop"`
	SecurityOpt []string          `json:"SecurityOpt"`
	Binds       []string          `json:"Binds"`
	Devices     []any             `json:"Devices"`
	Tmpfs       map[string]string `json:"Tmpfs"`
}

func newDockerCreateRequest(specification Specification, egressNetwork string) dockerCreateRequest {
	networkMode, networkDisabled := "none", true
	if specification.AllowNetwork {
		networkMode, networkDisabled = egressNetwork, false
	}
	return dockerCreateRequest{
		Image: specification.ImageDigest, User: "10001:10001", ReadonlyRootfs: true, NetworkDisabled: networkDisabled,
		Labels: map[string]string{
			labelReuseKeyHash:      specification.ReuseKeyHash,
			labelScope:             specification.Scope,
			labelImageDigest:       specification.ImageDigest,
			labelPolicyVersion:     specification.PolicyVersion,
			labelSchedulerVersion:  strconv.FormatUint(specification.SchedulerVersion, 10),
			labelCredentialVersion: specification.CredentialGeneration,
			labelDeploymentID:      specification.DeploymentID,
			labelCPUMilli:          strconv.Itoa(specification.CPUMilli),
			labelMemoryLimitMB:     strconv.Itoa(specification.MemoryLimitMB),
			labelPIDLimit:          strconv.Itoa(specification.PIDLimit),
			labelAllowNetwork:      strconv.FormatBool(specification.AllowNetwork),
		},
		HostConfig: dockerHostConfig{Privileged: false, NetworkMode: networkMode, UsernsMode: "private", PidMode: "private", IpcMode: "private", PidsLimit: int64(specification.PIDLimit), Memory: int64(specification.MemoryLimitMB) * 1024 * 1024, NanoCPUs: int64(specification.CPUMilli) * 1_000_000, CapDrop: []string{"ALL"}, SecurityOpt: []string{"no-new-privileges", "seccomp=/etc/newx/seccomp/sandbox.json", "apparmor=newx-sandbox"}, Binds: []string{}, Devices: []any{}, Tmpfs: map[string]string{"/tmp": "rw,nosuid,nodev,noexec,size=64m,uid=10001,gid=10001,mode=1777", "/run/newx-secrets": "rw,nosuid,nodev,noexec,size=16m,uid=10001,gid=10001,mode=0700"}},
	}
}

type dockerContainerSummary struct {
	ID     string            `json:"Id"`
	Image  string            `json:"Image"`
	State  string            `json:"State"`
	Labels map[string]string `json:"Labels"`
}

func (summary dockerContainerSummary) container() (Container, bool) {
	if !validContainerID(summary.ID) || summary.Labels[labelDeploymentID] == "" {
		return Container{}, false
	}
	specification, ok := specificationFromDockerLabels(summary.Labels)
	if !ok || summary.Image != specification.ImageDigest {
		return Container{ID: summary.ID, State: ContainerStateStopped}, true
	}
	// Docker does not persist the Runner's trusted idle lease. After a Runner
	// restart, every non-running container is therefore ambiguous and must be
	// drained by Lifecycle rather than treated as reusable.
	state := ContainerStateStopped
	if summary.State == "running" {
		state = ContainerStateRunning
	}
	return Container{ID: summary.ID, Spec: specification, State: state}, true
}

func (driver *DockerDriver) inspect(ctx context.Context, id string) (Container, error) {
	var response struct {
		ID     string `json:"Id"`
		Image  string `json:"Image"`
		Config struct {
			Labels map[string]string `json:"Labels"`
		} `json:"Config"`
		State struct {
			Running bool `json:"Running"`
		} `json:"State"`
	}
	if err := driver.requestJSON(ctx, http.MethodGet, dockerAPIPrefix+"/containers/"+url.PathEscape(id)+"/json", nil, &response, http.StatusOK); err != nil {
		return Container{}, err
	}
	container, ok := (dockerContainerSummary{ID: response.ID, Image: response.Image, Labels: response.Config.Labels, State: "exited"}).container()
	if !ok {
		return Container{}, errDriverUnavailable
	}
	if response.State.Running {
		container.State = ContainerStateRunning
	}
	return container, nil
}

type dockerExecCreateRequest struct {
	AttachStdin  bool     `json:"AttachStdin"`
	AttachStdout bool     `json:"AttachStdout"`
	AttachStderr bool     `json:"AttachStderr"`
	Tty          bool     `json:"Tty"`
	User         string   `json:"User"`
	Cmd          []string `json:"Cmd"`
}

type dockerExecStartRequest struct {
	Detach bool `json:"Detach"`
	Tty    bool `json:"Tty"`
}

func (driver *DockerDriver) runFixedMaintenance(ctx context.Context, id, command string) error {
	if driver == nil || ctx == nil || !validContainerID(id) || (command != runtimeCleanupCommand && command != runtimeHealthCommand) {
		return errDriverConfiguration
	}
	var created struct {
		ID string `json:"Id"`
	}
	request := dockerExecCreateRequest{User: runtimeUser, Cmd: []string{command}}
	if err := driver.requestJSON(ctx, http.MethodPost, dockerAPIPrefix+"/containers/"+url.PathEscape(id)+"/exec", request, &created, http.StatusCreated); err != nil || !validContainerID(created.ID) {
		return errDriverUnavailable
	}
	if err := driver.requestJSON(ctx, http.MethodPost, dockerAPIPrefix+"/exec/"+url.PathEscape(created.ID)+"/start", struct{}{}, nil, http.StatusOK); err != nil {
		return errDriverUnavailable
	}
	var inspected struct {
		Running  bool `json:"Running"`
		ExitCode int  `json:"ExitCode"`
	}
	if err := driver.requestJSON(ctx, http.MethodGet, dockerAPIPrefix+"/exec/"+url.PathEscape(created.ID)+"/json", nil, &inspected, http.StatusOK); err != nil || inspected.Running || inspected.ExitCode != 0 {
		return errDriverUnavailable
	}
	return nil
}

func (driver *DockerDriver) requestJSON(ctx context.Context, method, path string, requestBody any, responseBody any, acceptedStatus int) error {
	if driver == nil || driver.client == nil || ctx == nil || !strings.HasPrefix(path, dockerAPIPrefix+"/") {
		return errDriverConfiguration
	}
	var body io.Reader
	if requestBody != nil {
		encoded, err := json.Marshal(requestBody)
		if err != nil {
			return errDriverConfiguration
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, "http://unix"+path, body)
	if err != nil {
		return errDriverConfiguration
	}
	if requestBody != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := driver.client.Do(request)
	if err != nil {
		return errDriverUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode != acceptedStatus {
		return errDriverUnavailable
	}
	if responseBody == nil {
		return nil
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, 1024*1024))
	return decoder.Decode(responseBody)
}

func (driver *DockerDriver) requestBytes(ctx context.Context, method, path string, requestBody []byte, contentType string, acceptedStatus int, maximumResponseBytes int64) ([]byte, error) {
	if driver == nil || driver.client == nil || ctx == nil || !strings.HasPrefix(path, dockerAPIPrefix+"/") || maximumResponseBytes < 0 {
		return nil, errDriverConfiguration
	}
	request, err := http.NewRequestWithContext(ctx, method, "http://unix"+path, bytes.NewReader(requestBody))
	if err != nil {
		return nil, errDriverConfiguration
	}
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	response, err := driver.client.Do(request)
	if err != nil {
		return nil, errDriverUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode != acceptedStatus {
		return nil, errDriverUnavailable
	}
	if maximumResponseBytes == 0 {
		return nil, nil
	}
	value, err := io.ReadAll(io.LimitReader(response.Body, maximumResponseBytes+1))
	if err != nil || int64(len(value)) > maximumResponseBytes {
		return nil, errDriverUnavailable
	}
	return value, nil
}

func fixedInputArchive(input []byte) ([]byte, error) {
	var archive bytes.Buffer
	writer := tar.NewWriter(&archive)
	if err := writer.WriteHeader(&tar.Header{Name: runtimeAdapterInputFile, Mode: 0o400, Size: int64(len(input)), Format: tar.FormatUSTAR}); err != nil {
		return nil, err
	}
	if _, err := writer.Write(input); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return archive.Bytes(), nil
}

func fixedAdapterCommand(adapter Adapter) (string, bool) {
	switch adapter {
	case AdapterAgentCode:
		return "/opt/newx/bin/adapter-agent-code", true
	case AdapterMCPStdio:
		return "/opt/newx/bin/adapter-mcp-stdio", true
	case AdapterPluginCode:
		return "/opt/newx/bin/adapter-plugin-code", true
	case AdapterAppDevRuntime:
		return "/opt/newx/bin/adapter-appdev-runtime", true
	default:
		return "", false
	}
}

func decodeDockerMultiplexedOutput(value []byte, maximumOutputBytes int64) ([]byte, []byte, error) {
	var stdout, stderr []byte
	for len(value) > 0 {
		if len(value) < 8 || value[1] != 0 || value[2] != 0 || value[3] != 0 {
			return nil, nil, errDriverUnavailable
		}
		stream, size := value[0], int(binary.BigEndian.Uint32(value[4:8]))
		value = value[8:]
		if size > len(value) || int64(len(stdout)+len(stderr)+size) > maximumOutputBytes {
			return nil, nil, errDriverUnavailable
		}
		switch stream {
		case 1:
			stdout = append(stdout, value[:size]...)
		case 2:
			stderr = append(stderr, value[:size]...)
		default:
			return nil, nil, errDriverUnavailable
		}
		value = value[size:]
	}
	return stdout, stderr, nil
}

func mustJSON(value any) []byte {
	encoded, _ := json.Marshal(value)
	return encoded
}

func unixSocketPath(endpoint string) (string, bool) {
	parsed, err := url.Parse(endpoint)
	return parsed.Path, err == nil && parsed.Scheme == "unix" && parsed.Host == "" && strings.HasPrefix(parsed.Path, "/")
}

func validSpecification(specification Specification) bool {
	return validHash(specification.ReuseKeyHash) && validRuntimeIdentifier(specification.Scope) && validDigestImage(specification.ImageDigest) && validRuntimeIdentifier(specification.PolicyVersion) && specification.SchedulerVersion > 0 && validRuntimeIdentifier(specification.CredentialGeneration) && validRuntimeIdentifier(specification.DeploymentID) && specification.CPUMilli > 0 && specification.CPUMilli <= 64000 && specification.MemoryLimitMB > 0 && specification.MemoryLimitMB <= 131072 && specification.PIDLimit > 0 && specification.PIDLimit <= 65535
}

func specificationFromDockerLabels(labels map[string]string) (Specification, bool) {
	if labels == nil {
		return Specification{}, false
	}
	schedulerVersion, err := strconv.ParseUint(labels[labelSchedulerVersion], 10, 64)
	if err != nil || schedulerVersion == 0 {
		return Specification{}, false
	}
	cpuMilli, cpuOK := positiveLabelInt(labels[labelCPUMilli], 64000)
	memoryLimitMB, memoryOK := positiveLabelInt(labels[labelMemoryLimitMB], 131072)
	pidLimit, pidOK := positiveLabelInt(labels[labelPIDLimit], 65535)
	allowNetwork, err := strconv.ParseBool(labels[labelAllowNetwork])
	if !cpuOK || !memoryOK || !pidOK || err != nil {
		return Specification{}, false
	}
	specification := Specification{
		ReuseKeyHash: labels[labelReuseKeyHash], Scope: labels[labelScope], ImageDigest: labels[labelImageDigest],
		PolicyVersion: labels[labelPolicyVersion], SchedulerVersion: schedulerVersion, CredentialGeneration: labels[labelCredentialVersion],
		DeploymentID: labels[labelDeploymentID], CPUMilli: cpuMilli, MemoryLimitMB: memoryLimitMB, PIDLimit: pidLimit, AllowNetwork: allowNetwork,
	}
	return specification, validSpecification(specification)
}

func positiveLabelInt(value string, maximum int) (int, bool) {
	parsed, err := strconv.Atoi(value)
	return parsed, err == nil && parsed > 0 && parsed <= maximum
}

func validHash(value string) bool {
	_, err := hex.DecodeString(value)
	return err == nil && len(value) == 64 && strings.ToLower(value) == value
}
func validDigestImage(value string) bool {
	parts := strings.Split(value, "@sha256:")
	return len(parts) == 2 && parts[0] != "" && validHash(parts[1])
}
func validRuntimeIdentifier(value string) bool {
	if value == "" || len(value) > 128 || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		if !(character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || character == '-' || character == '_' || character == '.') {
			return false
		}
	}
	return true
}

func validContainerID(value string) bool { return validRuntimeIdentifier(value) }
