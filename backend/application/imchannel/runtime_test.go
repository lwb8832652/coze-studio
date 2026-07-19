// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package imchannel

import (
	"reflect"
	"testing"

	domain "github.com/coze-dev/coze-studio/backend/domain/imchannel"
)

func TestOfficialChannelFactoryConfiguresEventDispatcher(t *testing.T) {
	feishuChannel, err := (OfficialChannelFactory{}).New(
		&domain.Config{AppID: "cli_test"},
		"test-secret",
	)
	if err != nil {
		t.Fatalf("create official Feishu channel: %v", err)
	}

	channelValue := reflect.ValueOf(feishuChannel)
	if channelValue.Kind() != reflect.Pointer || channelValue.IsNil() {
		t.Fatalf("unexpected channel value: %T", feishuChannel)
	}
	wsClient := channelValue.Elem().FieldByName("wsClient")
	if !wsClient.IsValid() || wsClient.IsNil() {
		t.Fatal("official Feishu channel has no WebSocket client")
	}
	eventHandler := wsClient.Elem().FieldByName("eventHandler")
	if !eventHandler.IsValid() || eventHandler.IsNil() {
		t.Fatal("official Feishu WebSocket client has no event dispatcher")
	}
}
