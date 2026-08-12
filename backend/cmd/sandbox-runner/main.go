// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/coze-dev/coze-studio/backend/internal/sandboxrunner"
)

func main() {
	config, err := sandboxrunner.LoadConfig(os.Getenv)
	if err != nil {
		log.Fatal("sandbox runner configuration is invalid")
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	runtime, err := sandboxrunner.NewProcessRuntime(ctx, config)
	if err != nil {
		log.Fatal("sandbox runner runtime dependencies are unavailable")
	}
	if err := runtime.Run(ctx, config); err != nil {
		log.Fatal("sandbox runner stopped")
	}
}
