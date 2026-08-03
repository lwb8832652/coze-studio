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
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coze-dev/coze-studio/backend/pkg/envkey"
)

const (
	agentArtifactScannerTypeEnv           = "AGENT_ARTIFACT_SCANNER_TYPE"
	agentArtifactScannerHTTPURLEnv        = "AGENT_ARTIFACT_SCANNER_HTTP_URL"
	agentArtifactScannerHTTPTokenEnv      = "AGENT_ARTIFACT_SCANNER_HTTP_TOKEN"
	agentArtifactScannerHTTPTimeoutMsEnv  = "AGENT_ARTIFACT_SCANNER_HTTP_TIMEOUT_MS"
	agentArtifactScannerHTTPMaxBytesEnv   = "AGENT_ARTIFACT_SCANNER_HTTP_MAX_BYTES"
	agentArtifactScannerClamdAddrEnv      = "AGENT_ARTIFACT_SCANNER_CLAMD_ADDR"
	agentArtifactScannerClamdTimeoutMsEnv = "AGENT_ARTIFACT_SCANNER_CLAMD_TIMEOUT_MS"
	agentArtifactScannerClamdMaxBytesEnv  = "AGENT_ARTIFACT_SCANNER_CLAMD_MAX_BYTES"
)

const (
	artifactScannerTypeHTTP      = "http"
	artifactScannerTypeClamd     = "clamd"
	artifactScannerTypeClamAV    = "clamav"
	defaultHTTPScannerTimeout    = 10 * time.Second
	defaultHTTPScannerMaxBytes   = int64(50 * 1024 * 1024)
	maxHTTPScannerResponseBytes  = int64(1024 * 1024)
	artifactScannerRequestSchema = "coze.artifact_scan_request.v1"
	defaultClamdScannerTimeout   = 10 * time.Second
	defaultClamdScannerMaxBytes  = int64(50 * 1024 * 1024)
	clamdScannerChunkBytes       = 32 * 1024
	maxClamdScannerResponseBytes = int64(4096)
	clamdScannerVersion          = "clamd"
)

type HTTPArtifactContentScannerOptions struct {
	Endpoint string
	Token    string
	Timeout  time.Duration
	MaxBytes int64
	Client   *http.Client
}

type ClamdArtifactContentScannerOptions struct {
	Address  string
	Timeout  time.Duration
	MaxBytes int64
}

type ArtifactScannerEnvStatus struct {
	Enabled    bool
	Type       string
	Configured bool
	Error      string
}

type httpArtifactContentScanner struct {
	endpoint string
	token    string
	client   *http.Client
	maxBytes int64
}

type clamdArtifactContentScanner struct {
	address  string
	timeout  time.Duration
	maxBytes int64
}

type httpArtifactScanRequest struct {
	Schema      string `json:"schema"`
	Scanner     string `json:"scanner"`
	SpaceID     int64  `json:"space_id,omitempty"`
	ThreadID    int64  `json:"thread_id,omitempty"`
	RunID       int64  `json:"run_id,omitempty"`
	UserID      int64  `json:"user_id,omitempty"`
	ArtifactID  int64  `json:"artifact_id,omitempty"`
	FileID      int64  `json:"file_id,omitempty"`
	ContentType string `json:"content_type,omitempty"`
	SizeBytes   int64  `json:"size_bytes"`
}

type httpArtifactScanResponse struct {
	ScanStatus     string `json:"scan_status"`
	ScannerVersion string `json:"scanner_version,omitempty"`
	Reason         string `json:"reason,omitempty"`
}

func NewArtifactContentScannerFromEnv() ArtifactContentScanner {
	scanner, _ := NewArtifactContentScannerFromEnvWithStatus()

	return scanner
}

func NewArtifactContentScannerFromEnvWithStatus() (ArtifactContentScanner, ArtifactScannerEnvStatus) {
	scannerType := strings.ToLower(strings.TrimSpace(envkey.GetStringD(agentArtifactScannerTypeEnv, "")))
	status := ArtifactScannerEnvStatus{
		Enabled: scannerType != "",
		Type:    scannerType,
	}
	switch scannerType {
	case "":
		return nil, status
	case artifactScannerTypeHTTP:
		scanner, err := NewHTTPArtifactContentScanner(HTTPArtifactContentScannerOptions{
			Endpoint: envkey.GetStringD(agentArtifactScannerHTTPURLEnv, ""),
			Token:    envkey.GetStringD(agentArtifactScannerHTTPTokenEnv, ""),
			Timeout:  time.Duration(envkey.GetIntD(agentArtifactScannerHTTPTimeoutMsEnv, int(defaultHTTPScannerTimeout/time.Millisecond))) * time.Millisecond,
			MaxBytes: int64(envkey.GetIntD(agentArtifactScannerHTTPMaxBytesEnv, int(defaultHTTPScannerMaxBytes))),
		})
		if err != nil {
			status.Error = err.Error()
			return nil, status
		}
		status.Configured = true

		return scanner, status
	case artifactScannerTypeClamd, artifactScannerTypeClamAV:
		scanner, err := NewClamdArtifactContentScanner(ClamdArtifactContentScannerOptions{
			Address:  envkey.GetStringD(agentArtifactScannerClamdAddrEnv, ""),
			Timeout:  time.Duration(envkey.GetIntD(agentArtifactScannerClamdTimeoutMsEnv, int(defaultClamdScannerTimeout/time.Millisecond))) * time.Millisecond,
			MaxBytes: int64(envkey.GetIntD(agentArtifactScannerClamdMaxBytesEnv, int(defaultClamdScannerMaxBytes))),
		})
		if err != nil {
			status.Error = err.Error()
			return nil, status
		}
		status.Configured = true

		return scanner, status
	default:
		status.Error = "unsupported artifact scanner type"
		return nil, status
	}
}

func NewHTTPArtifactContentScanner(
	opts HTTPArtifactContentScannerOptions,
) (ArtifactContentScanner, error) {
	endpoint := strings.TrimSpace(opts.Endpoint)
	if endpoint == "" {
		return nil, fmt.Errorf("artifact scanner endpoint is required")
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("artifact scanner endpoint is invalid")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("artifact scanner endpoint scheme is invalid")
	}
	if parsed.User != nil {
		return nil, fmt.Errorf("artifact scanner endpoint must not include userinfo")
	}

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = defaultHTTPScannerTimeout
	}
	maxBytes := opts.MaxBytes
	if maxBytes <= 0 {
		maxBytes = defaultHTTPScannerMaxBytes
	}
	client := opts.Client
	if client == nil {
		client = &http.Client{Timeout: timeout}
	}

	return &httpArtifactContentScanner{
		endpoint: endpoint,
		token:    strings.TrimSpace(opts.Token),
		client:   client,
		maxBytes: maxBytes,
	}, nil
}

func NewClamdArtifactContentScanner(
	opts ClamdArtifactContentScannerOptions,
) (ArtifactContentScanner, error) {
	address := strings.TrimSpace(opts.Address)
	if address == "" {
		return nil, fmt.Errorf("artifact scanner clamd address is required")
	}
	if strings.Contains(address, "://") {
		return nil, fmt.Errorf("artifact scanner clamd address is invalid")
	}
	if _, _, err := net.SplitHostPort(address); err != nil {
		return nil, fmt.Errorf("artifact scanner clamd address is invalid")
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = defaultClamdScannerTimeout
	}
	maxBytes := opts.MaxBytes
	if maxBytes <= 0 {
		maxBytes = defaultClamdScannerMaxBytes
	}

	return &clamdArtifactContentScanner{
		address:  address,
		timeout:  timeout,
		maxBytes: maxBytes,
	}, nil
}

func (s *httpArtifactContentScanner) ScanArtifact(
	ctx context.Context,
	req ArtifactScanRequest,
) (*ArtifactScanResult, error) {
	if s == nil || s.client == nil {
		return nil, fmt.Errorf("artifact scanner is not configured")
	}
	if req.SizeBytes > s.maxBytes || int64(len(req.Content)) > s.maxBytes {
		return nil, fmt.Errorf("artifact scanner content exceeds max bytes")
	}
	scannerName := strings.TrimSpace(req.Scanner)
	if scannerName == "" {
		scannerName = defaultApplicationArtifactScanner
	}
	body, err := newHTTPArtifactScanBody(req, scannerName, s.maxBytes)
	if err != nil {
		return nil, err
	}
	defer body.Close()
	httpReq, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		s.endpoint,
		body,
	)
	if err != nil {
		return nil, fmt.Errorf("artifact scanner request is invalid")
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if s.token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+s.token)
	}

	resp, err := s.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("artifact scanner request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("artifact scanner http status %d", resp.StatusCode)
	}

	var decoded httpArtifactScanResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxHTTPScannerResponseBytes)).Decode(&decoded); err != nil {
		return nil, fmt.Errorf("artifact scanner response is invalid")
	}
	status := strings.ToLower(strings.TrimSpace(decoded.ScanStatus))
	if !isTerminalArtifactScanStatus(status) {
		return nil, fmt.Errorf("artifact scanner scan status is invalid")
	}

	return &ArtifactScanResult{
		ScanStatus:     status,
		ScannerVersion: strings.TrimSpace(decoded.ScannerVersion),
		Reason:         strings.TrimSpace(decoded.Reason),
	}, nil
}

func (s *httpArtifactContentScanner) MaxArtifactBytes() int64 {
	if s == nil {
		return 0
	}
	return s.maxBytes
}

func (s *clamdArtifactContentScanner) ScanArtifact(
	ctx context.Context,
	req ArtifactScanRequest,
) (*ArtifactScanResult, error) {
	if s == nil || s.address == "" {
		return nil, fmt.Errorf("artifact scanner is not configured")
	}
	if req.SizeBytes > s.maxBytes || int64(len(req.Content)) > s.maxBytes {
		return nil, fmt.Errorf("artifact scanner content exceeds max bytes")
	}
	scanCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	dialer := net.Dialer{Timeout: s.timeout}
	conn, err := dialer.DialContext(scanCtx, "tcp", s.address)
	if err != nil {
		return nil, fmt.Errorf("artifact scanner request failed")
	}
	defer conn.Close()
	if deadline, ok := scanCtx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	if err := writeClamdInstream(conn, artifactScanContentReader(req), s.maxBytes); err != nil {
		return nil, fmt.Errorf("artifact scanner request failed")
	}
	result, err := readClamdScanResult(conn)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func (s *clamdArtifactContentScanner) MaxArtifactBytes() int64 {
	if s == nil {
		return 0
	}
	return s.maxBytes
}

func writeClamdInstream(w io.Writer, content io.Reader, maxBytes int64) error {
	if content == nil || maxBytes <= 0 {
		return fmt.Errorf("artifact scanner content is not configured")
	}
	if err := writeAll(w, []byte("nINSTREAM\n")); err != nil {
		return err
	}
	buffer := make([]byte, clamdScannerChunkBytes)
	var total int64
	for {
		chunkSize, readErr := content.Read(buffer)
		if chunkSize > 0 {
			total += int64(chunkSize)
			if total > maxBytes {
				return fmt.Errorf("artifact scanner content exceeds max bytes")
			}
			var size [4]byte
			binary.BigEndian.PutUint32(size[:], uint32(chunkSize))
			if err := writeAll(w, size[:]); err != nil {
				return err
			}
			if err := writeAll(w, buffer[:chunkSize]); err != nil {
				return err
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			return readErr
		}
	}
	var zero [4]byte

	return writeAll(w, zero[:])
}

func artifactScanContentReader(req ArtifactScanRequest) io.Reader {
	if req.ContentReader != nil {
		return req.ContentReader
	}
	return bytes.NewReader(req.Content)
}

func newHTTPArtifactScanBody(
	req ArtifactScanRequest,
	scannerName string,
	maxBytes int64,
) (io.ReadCloser, error) {
	prefix, err := json.Marshal(httpArtifactScanRequest{
		Schema:      artifactScannerRequestSchema,
		Scanner:     scannerName,
		SpaceID:     req.SpaceID,
		ThreadID:    req.ThreadID,
		RunID:       req.RunID,
		UserID:      req.UserID,
		ArtifactID:  req.ArtifactID,
		FileID:      req.FileID,
		ContentType: strings.TrimSpace(req.ContentType),
		SizeBytes:   req.SizeBytes,
	})
	if err != nil {
		return nil, err
	}
	if len(prefix) == 0 || prefix[len(prefix)-1] != '}' {
		return nil, fmt.Errorf("artifact scanner request is invalid")
	}
	prefix = append(prefix[:len(prefix)-1], []byte(`,"content_base64":"`)...)
	reader := artifactScanContentReader(req)
	pipeReader, pipeWriter := io.Pipe()
	go func() {
		if _, err := pipeWriter.Write(prefix); err != nil {
			_ = pipeWriter.CloseWithError(err)
			return
		}
		limited := &io.LimitedReader{R: reader, N: maxBytes + 1}
		encoder := base64.NewEncoder(base64.StdEncoding, pipeWriter)
		written, copyErr := io.Copy(encoder, limited)
		closeErr := encoder.Close()
		if copyErr == nil {
			copyErr = closeErr
		}
		if copyErr == nil && written > maxBytes {
			copyErr = fmt.Errorf("artifact scanner content exceeds max bytes")
		}
		if copyErr == nil {
			_, copyErr = pipeWriter.Write([]byte(`"}`))
		}
		_ = pipeWriter.CloseWithError(copyErr)
	}()
	return pipeReader, nil
}

func writeAll(w io.Writer, data []byte) error {
	for len(data) > 0 {
		n, err := w.Write(data)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		data = data[n:]
	}

	return nil
}

func readClamdScanResult(r io.Reader) (*ArtifactScanResult, error) {
	limited := io.LimitReader(r, maxClamdScannerResponseBytes)
	line, err := bufio.NewReader(limited).ReadString('\n')
	if err != nil && strings.TrimSpace(line) == "" {
		return nil, fmt.Errorf("artifact scanner response is invalid")
	}
	response := strings.TrimSpace(strings.TrimRight(line, "\x00"))
	if response == "" {
		return nil, fmt.Errorf("artifact scanner response is invalid")
	}
	if strings.HasSuffix(response, " OK") {
		return &ArtifactScanResult{
			ScanStatus:     string(artifactScanStatusClean),
			ScannerVersion: clamdScannerVersion,
		}, nil
	}
	if strings.HasSuffix(response, " FOUND") {
		return &ArtifactScanResult{
			ScanStatus:     string(artifactScanStatusInfected),
			ScannerVersion: clamdScannerVersion,
			Reason:         parseClamdSignature(response),
		}, nil
	}
	if strings.HasSuffix(response, " ERROR") {
		return nil, fmt.Errorf("artifact scanner scan failed")
	}

	return nil, fmt.Errorf("artifact scanner response is invalid")
}

func parseClamdSignature(response string) string {
	withoutStatus := strings.TrimSuffix(response, " FOUND")
	_, signature, ok := strings.Cut(withoutStatus, ":")
	if !ok {
		return ""
	}

	return strings.TrimSpace(signature)
}

func isTerminalArtifactScanStatus(status string) bool {
	switch status {
	case string(artifactScanStatusClean),
		string(artifactScanStatusBlocked),
		string(artifactScanStatusInfected),
		string(artifactScanStatusQuarantined):
		return true
	default:
		return false
	}
}
