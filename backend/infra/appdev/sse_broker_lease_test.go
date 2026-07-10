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

package appdev

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	appdevapp "github.com/coze-dev/coze-studio/backend/application/appdev"
)

type leaseRecordingPersistence struct {
	ChatPersistence
	touches chan string
}

func (p *leaseRecordingPersistence) TouchSession(
	_ context.Context,
	_ *appdevapp.ChatManagerRequest,
	requestID string,
) error {
	p.touches <- requestID
	return nil
}

func TestChatBrokerRenewsLeaseUntilGenerationStops(t *testing.T) {
	previousInterval := appDevChatLeaseHeartbeatInterval
	previousTimeout := appDevChatLeaseHeartbeatTimeout
	appDevChatLeaseHeartbeatInterval = 5 * time.Millisecond
	appDevChatLeaseHeartbeatTimeout = 100 * time.Millisecond
	defer func() {
		appDevChatLeaseHeartbeatInterval = previousInterval
		appDevChatLeaseHeartbeatTimeout = previousTimeout
	}()

	persistence := &leaseRecordingPersistence{touches: make(chan string, 1)}
	broker := NewChatBrokerWithPersistence(persistence)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		broker.renewChatLease(ctx, &appdevapp.ChatManagerRequest{SpaceID: "1001", ProjectID: "project-1"}, "request-1")
		close(done)
	}()

	select {
	case requestID := <-persistence.touches:
		require.Equal(t, "request-1", requestID)
	case <-time.After(time.Second):
		t.Fatal("chat lease was not renewed")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("chat lease heartbeat did not stop with generation")
	}
}
