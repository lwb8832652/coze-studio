// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package application

import (
	"context"
	"testing"
)

type orderedShutdownHook struct {
	name  string
	order *[]string
}

func (h *orderedShutdownHook) Shutdown(context.Context) error {
	*h.order = append(*h.order, h.name)
	return nil
}

func (h *orderedShutdownHook) ShutdownName() string {
	return h.name
}

func TestNotificationProducersShutdownBeforeConsumer(t *testing.T) {
	registry := newApplicationShutdownRegistry()
	order := make([]string, 0, 3)
	consumer := &orderedShutdownHook{name: "notification-consumer", order: &order}
	billing := &orderedShutdownHook{name: "billing-producer", order: &order}
	monitor := &orderedShutdownHook{name: "sandbox-monitor", order: &order}

	if err := registerNotificationLifecycleHooks(
		registry,
		consumer,
		billing,
		monitor,
	); err != nil {
		t.Fatalf("registerNotificationLifecycleHooks() error = %v", err)
	}
	if err := registry.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
	want := []string{"sandbox-monitor", "billing-producer", "notification-consumer"}
	if len(order) != len(want) {
		t.Fatalf("shutdown order = %#v, want %#v", order, want)
	}
	for index := range want {
		if order[index] != want[index] {
			t.Fatalf("shutdown order = %#v, want %#v", order, want)
		}
	}
}
