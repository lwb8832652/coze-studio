// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package s3

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

type s3StreamOpenerStub struct {
	body  io.ReadCloser
	err   error
	calls int
}

func (stub *s3StreamOpenerStub) OpenObjectStream(context.Context, string, string) (io.ReadCloser, error) {
	stub.calls++
	return stub.body, stub.err
}

type s3TrackingStream struct {
	reader  io.Reader
	readErr error
	closed  bool
}

func (stream *s3TrackingStream) Read(buffer []byte) (int, error) {
	if stream.readErr != nil {
		return 0, stream.readErr
	}
	return stream.reader.Read(buffer)
}
func (stream *s3TrackingStream) Close() error { stream.closed = true; return nil }

func TestS3ObjectStreamLifecycle(t *testing.T) {
	for _, content := range []string{"payload", ""} {
		stream := &s3TrackingStream{reader: strings.NewReader(content)}
		opener := &s3StreamOpenerStub{body: stream}
		client := &s3Client{bucketName: "bucket", streamOpener: opener}
		body, err := client.OpenObjectStream(context.Background(), "object")
		if err != nil {
			t.Fatal(err)
		}
		got, err := io.ReadAll(body)
		if err != nil || string(got) != content {
			t.Fatalf("read = %q, %v", got, err)
		}
		if err := body.Close(); err != nil || !stream.closed {
			t.Fatalf("close = %v closed=%v", err, stream.closed)
		}
	}
	openErr := errors.New("get failed")
	if body, err := (&s3Client{streamOpener: &s3StreamOpenerStub{err: openErr}}).OpenObjectStream(context.Background(), "object"); body != nil || !errors.Is(err, openErr) {
		t.Fatalf("get error = %#v, %v", body, err)
	}
	if body, err := (&s3Client{streamOpener: &s3StreamOpenerStub{}}).OpenObjectStream(context.Background(), "object"); body != nil || err == nil {
		t.Fatalf("nil body = %#v, %v", body, err)
	}
	readErr := errors.New("read failed")
	body, err := (&s3Client{streamOpener: &s3StreamOpenerStub{body: &s3TrackingStream{reader: strings.NewReader(""), readErr: readErr}}}).OpenObjectStream(context.Background(), "object")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := body.Read(make([]byte, 1)); !errors.Is(err, readErr) {
		t.Fatalf("read error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	opener := &s3StreamOpenerStub{body: &s3TrackingStream{reader: strings.NewReader("payload")}}
	if body, err := (&s3Client{streamOpener: opener}).OpenObjectStream(ctx, "object"); body != nil || !errors.Is(err, context.Canceled) || opener.calls != 0 {
		t.Fatalf("canceled open = %#v, %v calls=%d", body, err, opener.calls)
	}
}
