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
)

func TestCanCommitGeneratedSource(t *testing.T) {
	t.Run("active request can commit", func(t *testing.T) {
		if !canCommitGeneratedSource(context.Background(), true) {
			t.Fatal("expected active generation to be committable")
		}
	})

	t.Run("cancelled context cannot commit", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		if canCommitGeneratedSource(ctx, true) {
			t.Fatal("cancelled generation must not mutate project files")
		}
	})

	t.Run("superseded request cannot commit", func(t *testing.T) {
		if canCommitGeneratedSource(context.Background(), false) {
			t.Fatal("superseded generation must not mutate project files")
		}
	})
}

func TestCancelledHistoryMessage(t *testing.T) {
	message := cancelledHistoryMessage("request-1")

	if message.ID != "cancelled_request-1" {
		t.Fatalf("unexpected cancellation message id: %q", message.ID)
	}
	if message.Type != "assistant" {
		t.Fatalf("unexpected cancellation message type: %q", message.Type)
	}
	if message.Content != "任务已取消，未修改项目文件。" {
		t.Fatalf("unexpected cancellation message content: %q", message.Content)
	}
}
