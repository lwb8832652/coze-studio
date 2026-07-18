// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package tos

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

type tosStreamOpenerStub struct {
	body  io.ReadCloser
	err   error
	calls int
}

func (stub *tosStreamOpenerStub) OpenObjectStream(context.Context, string, string) (io.ReadCloser, error) {
	stub.calls++
	return stub.body, stub.err
}

type tosTrackingStream struct {
	reader  io.Reader
	readErr error
	closed  bool
}

func (stream *tosTrackingStream) Read(buffer []byte) (int, error) {
	if stream.readErr != nil {
		return 0, stream.readErr
	}
	return stream.reader.Read(buffer)
}
func (stream *tosTrackingStream) Close() error { stream.closed = true; return nil }

func TestTOSObjectStreamLifecycle(t *testing.T) {
	for _, content := range []string{"payload", ""} {
		stream := &tosTrackingStream{reader: strings.NewReader(content)}
		opener := &tosStreamOpenerStub{body: stream}
		client := &tosClient{bucketName: "bucket", streamOpener: opener}
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
	if body, err := (&tosClient{streamOpener: &tosStreamOpenerStub{err: openErr}}).OpenObjectStream(context.Background(), "object"); body != nil || !errors.Is(err, openErr) {
		t.Fatalf("get error = %#v, %v", body, err)
	}
	if body, err := (&tosClient{streamOpener: &tosStreamOpenerStub{}}).OpenObjectStream(context.Background(), "object"); body != nil || err == nil {
		t.Fatalf("nil body = %#v, %v", body, err)
	}
	readErr := errors.New("read failed")
	body, err := (&tosClient{streamOpener: &tosStreamOpenerStub{body: &tosTrackingStream{reader: strings.NewReader(""), readErr: readErr}}}).OpenObjectStream(context.Background(), "object")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := body.Read(make([]byte, 1)); !errors.Is(err, readErr) {
		t.Fatalf("read error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	opener := &tosStreamOpenerStub{body: &tosTrackingStream{reader: strings.NewReader("payload")}}
	if body, err := (&tosClient{streamOpener: opener}).OpenObjectStream(ctx, "object"); body != nil || !errors.Is(err, context.Canceled) || opener.calls != 0 {
		t.Fatalf("canceled open = %#v, %v calls=%d", body, err, opener.calls)
	}
}
