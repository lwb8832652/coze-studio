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
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestHTTPArtifactContentScannerPostsScopedPayloadAndReturnsResult(t *testing.T) {
	var gotHeader string
	var got map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("Authorization")
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "application/json", r.Header.Get("Content-Type"))
		require.NoError(t, json.NewDecoder(r.Body).Decode(&got))
		_, _ = w.Write([]byte(`{"scan_status":"infected","scanner_version":"1.4.0","reason":"signature matched"}`))
	}))
	defer server.Close()
	scanner, err := NewHTTPArtifactContentScanner(HTTPArtifactContentScannerOptions{
		Endpoint: server.URL,
		Token:    "scanner-token",
		Timeout:  time.Second,
		MaxBytes: 1024,
	})
	require.NoError(t, err)

	result, err := scanner.ScanArtifact(context.Background(), ArtifactScanRequest{
		SpaceID:     30,
		ThreadID:    10,
		RunID:       20,
		UserID:      40,
		ArtifactID:  100,
		FileID:      90,
		Scanner:     "clamav",
		ContentType: "text/plain",
		SizeBytes:   13,
		Content:     []byte("artifact body"),
	})

	require.NoError(t, err)
	require.Equal(t, "Bearer scanner-token", gotHeader)
	require.Equal(t, "coze.artifact_scan_request.v1", got["schema"])
	require.Equal(t, "clamav", got["scanner"])
	require.Equal(t, float64(30), got["space_id"])
	require.Equal(t, float64(10), got["thread_id"])
	require.Equal(t, float64(20), got["run_id"])
	require.Equal(t, float64(40), got["user_id"])
	require.Equal(t, float64(100), got["artifact_id"])
	require.Equal(t, float64(90), got["file_id"])
	require.Equal(t, "text/plain", got["content_type"])
	require.Equal(t, float64(13), got["size_bytes"])
	require.Equal(t, base64.StdEncoding.EncodeToString([]byte("artifact body")), got["content_base64"])
	require.NotContains(t, got, "object_uri")
	require.NotContains(t, got, "virtual_path")
	require.NotContains(t, got, "file_name")
	require.NotContains(t, got, "title")
	require.Equal(t, &ArtifactScanResult{
		ScanStatus:     "infected",
		ScannerVersion: "1.4.0",
		Reason:         "signature matched",
	}, result)
}

func TestHTTPArtifactContentScannerRejectsNonTerminalStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"scan_status":"pending"}`))
	}))
	defer server.Close()
	scanner, err := NewHTTPArtifactContentScanner(HTTPArtifactContentScannerOptions{
		Endpoint: server.URL,
		Timeout:  time.Second,
		MaxBytes: 1024,
	})
	require.NoError(t, err)

	result, err := scanner.ScanArtifact(context.Background(), ArtifactScanRequest{
		Scanner: "clamav",
		Content: []byte("artifact body"),
	})

	require.Error(t, err)
	require.Nil(t, result)
	require.Contains(t, err.Error(), "scan status")
}

func TestHTTPArtifactContentScannerRejectsOversizedContentBeforeRequest(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer server.Close()
	scanner, err := NewHTTPArtifactContentScanner(HTTPArtifactContentScannerOptions{
		Endpoint: server.URL,
		Timeout:  time.Second,
		MaxBytes: 3,
	})
	require.NoError(t, err)

	result, err := scanner.ScanArtifact(context.Background(), ArtifactScanRequest{
		Scanner: "clamav",
		Content: []byte("artifact body"),
	})

	require.Error(t, err)
	require.Nil(t, result)
	require.False(t, called)
	require.NotContains(t, err.Error(), "artifact body")
}

func TestArtifactContentScannerFromEnvBuildsHTTPScanner(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer env-token", r.Header.Get("Authorization"))
		_, _ = w.Write([]byte(`{"scan_status":"clean","scanner_version":"env-1"}`))
	}))
	defer server.Close()
	t.Setenv(agentArtifactScannerTypeEnv, "http")
	t.Setenv(agentArtifactScannerHTTPURLEnv, server.URL)
	t.Setenv(agentArtifactScannerHTTPTokenEnv, "env-token")
	t.Setenv(agentArtifactScannerHTTPTimeoutMsEnv, "1000")
	t.Setenv(agentArtifactScannerHTTPMaxBytesEnv, "1024")

	scanner := NewArtifactContentScannerFromEnv()

	require.NotNil(t, scanner)
	result, err := scanner.ScanArtifact(context.Background(), ArtifactScanRequest{
		Scanner: "clamav",
		Content: []byte("artifact body"),
	})
	require.NoError(t, err)
	require.Equal(t, "clean", result.ScanStatus)
	require.Equal(t, "env-1", result.ScannerVersion)
}

func TestClamdArtifactContentScannerStreamsContentAndReturnsClean(t *testing.T) {
	var got bytes.Buffer
	addr := startFakeClamdServer(t, "stream: OK\n", &got)
	scanner, err := NewClamdArtifactContentScanner(ClamdArtifactContentScannerOptions{
		Address:  addr,
		Timeout:  time.Second,
		MaxBytes: 1024,
	})
	require.NoError(t, err)

	result, err := scanner.ScanArtifact(context.Background(), ArtifactScanRequest{
		Scanner: "clamd",
		Content: []byte("artifact body"),
	})

	require.NoError(t, err)
	require.Equal(t, "artifact body", got.String())
	require.Equal(t, &ArtifactScanResult{
		ScanStatus:     "clean",
		ScannerVersion: "clamd",
	}, result)
}

func TestClamdArtifactContentScannerReturnsInfectedWithSignature(t *testing.T) {
	addr := startFakeClamdServer(t, "stream: Eicar-Test-Signature FOUND\n", nil)
	scanner, err := NewClamdArtifactContentScanner(ClamdArtifactContentScannerOptions{
		Address:  addr,
		Timeout:  time.Second,
		MaxBytes: 1024,
	})
	require.NoError(t, err)

	result, err := scanner.ScanArtifact(context.Background(), ArtifactScanRequest{
		Scanner: "clamd",
		Content: []byte("artifact body"),
	})

	require.NoError(t, err)
	require.Equal(t, &ArtifactScanResult{
		ScanStatus:     "infected",
		ScannerVersion: "clamd",
		Reason:         "Eicar-Test-Signature",
	}, result)
}

func TestArtifactContentScannerFromEnvBuildsClamdScanner(t *testing.T) {
	t.Setenv(agentArtifactScannerTypeEnv, "clamd")
	t.Setenv(agentArtifactScannerClamdAddrEnv, "127.0.0.1:3310")
	t.Setenv(agentArtifactScannerClamdTimeoutMsEnv, "1000")
	t.Setenv(agentArtifactScannerClamdMaxBytesEnv, "1024")

	scanner, status := NewArtifactContentScannerFromEnvWithStatus()

	require.NotNil(t, scanner)
	require.True(t, status.Enabled)
	require.Equal(t, "clamd", status.Type)
	require.True(t, status.Configured)
	require.Empty(t, status.Error)
}

func TestArtifactContentScannerFromEnvReportsInvalidHTTPConfig(t *testing.T) {
	t.Setenv(agentArtifactScannerTypeEnv, "http")
	t.Setenv(agentArtifactScannerHTTPURLEnv, "")
	t.Setenv(agentArtifactScannerHTTPTokenEnv, "scanner-secret")

	scanner, status := NewArtifactContentScannerFromEnvWithStatus()

	require.Nil(t, scanner)
	require.True(t, status.Enabled)
	require.Equal(t, "http", status.Type)
	require.False(t, status.Configured)
	require.Contains(t, status.Error, "artifact scanner endpoint is required")
	require.NotContains(t, status.Error, "scanner-secret")
}

func TestArtifactContentScannerFromEnvReportsUnknownType(t *testing.T) {
	t.Setenv(agentArtifactScannerTypeEnv, "bogus")

	scanner, status := NewArtifactContentScannerFromEnvWithStatus()

	require.Nil(t, scanner)
	require.True(t, status.Enabled)
	require.Equal(t, "bogus", status.Type)
	require.False(t, status.Configured)
	require.Contains(t, status.Error, "unsupported artifact scanner type")
}

func TestArtifactContentScannerFromEnvDefaultsToNil(t *testing.T) {
	t.Setenv(agentArtifactScannerTypeEnv, "")

	require.Nil(t, NewArtifactContentScannerFromEnv())
}

func startFakeClamdServer(t *testing.T, response string, got *bytes.Buffer) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	done := make(chan error, 1)
	t.Cleanup(func() {
		_ = ln.Close()
		select {
		case err := <-done:
			require.NoError(t, err)
		case <-time.After(time.Second):
			require.Fail(t, "fake clamd server did not finish")
		}
	})
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			done <- err
			return
		}
		defer conn.Close()
		if err := conn.SetDeadline(time.Now().Add(time.Second)); err != nil {
			done <- err
			return
		}
		prefix := make([]byte, len("nINSTREAM\n"))
		if _, err := io.ReadFull(conn, prefix); err != nil {
			done <- err
			return
		}
		if string(prefix) != "nINSTREAM\n" {
			done <- fmt.Errorf("unexpected clamd command %q", string(prefix))
			return
		}
		for {
			var sizeBytes [4]byte
			if _, err := io.ReadFull(conn, sizeBytes[:]); err != nil {
				done <- err
				return
			}
			size := binary.BigEndian.Uint32(sizeBytes[:])
			if size == 0 {
				break
			}
			if got == nil {
				if _, err := io.CopyN(io.Discard, conn, int64(size)); err != nil {
					done <- err
					return
				}
				continue
			}
			if _, err := io.CopyN(got, conn, int64(size)); err != nil {
				done <- err
				return
			}
		}
		_, err = conn.Write([]byte(response))
		done <- err
	}()

	return ln.Addr().String()
}
