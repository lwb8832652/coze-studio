// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package minio

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

type minioStreamOpenerStub struct {
	body  io.ReadCloser
	err   error
	calls int
}

func (stub *minioStreamOpenerStub) OpenObjectStream(context.Context, string, string) (io.ReadCloser, error) {
	stub.calls++
	return stub.body, stub.err
}

type minioTrackingStream struct {
	reader  io.Reader
	readErr error
	closed  bool
}

func (stream *minioTrackingStream) Read(buffer []byte) (int, error) {
	if stream.readErr != nil {
		return 0, stream.readErr
	}
	return stream.reader.Read(buffer)
}
func (stream *minioTrackingStream) Close() error { stream.closed = true; return nil }

func TestMinIOObjectStreamLifecycle(t *testing.T) {
	testObjectStreamLifecycle(t, func(opener minioObjectStreamOpener) storageStreamClient {
		return &minioClient{bucketName: "bucket", streamOpener: opener}
	})
}

type storageStreamClient interface {
	OpenObjectStream(context.Context, string) (io.ReadCloser, error)
}

func testObjectStreamLifecycle(t *testing.T, client func(minioObjectStreamOpener) storageStreamClient) {
	t.Helper()
	for _, content := range []string{"payload", ""} {
		stream := &minioTrackingStream{reader: strings.NewReader(content)}
		opener := &minioStreamOpenerStub{body: stream}
		body, err := client(opener).OpenObjectStream(context.Background(), "object")
		if err != nil {
			t.Fatal(err)
		}
		got, err := io.ReadAll(body)
		if err != nil || string(got) != content {
			t.Fatalf("read = %q, %v", got, err)
		}
		if err := body.Close(); err != nil || !stream.closed {
			t.Fatalf("close = %v, closed=%v", err, stream.closed)
		}
	}
	openErr := errors.New("get failed")
	if body, err := client(&minioStreamOpenerStub{err: openErr}).OpenObjectStream(context.Background(), "object"); body != nil || !errors.Is(err, openErr) {
		t.Fatalf("get error = %#v, %v", body, err)
	}
	if body, err := client(&minioStreamOpenerStub{}).OpenObjectStream(context.Background(), "object"); body != nil || err == nil {
		t.Fatalf("nil body = %#v, %v", body, err)
	}
	readErr := errors.New("read failed")
	body, err := client(&minioStreamOpenerStub{body: &minioTrackingStream{reader: strings.NewReader(""), readErr: readErr}}).OpenObjectStream(context.Background(), "object")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := body.Read(make([]byte, 1)); !errors.Is(err, readErr) {
		t.Fatalf("read error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	opener := &minioStreamOpenerStub{body: &minioTrackingStream{reader: strings.NewReader("payload")}}
	if body, err := client(opener).OpenObjectStream(ctx, "object"); body != nil || !errors.Is(err, context.Canceled) || opener.calls != 0 {
		t.Fatalf("canceled open = %#v, %v calls=%d", body, err, opener.calls)
	}
}
