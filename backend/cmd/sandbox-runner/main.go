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
	os.Exit(run(os.Args[1:], os.Getenv))
}

type migrationCheckFunc func(context.Context, func(string) string) error

func run(args []string, getenv func(string) string) int {
	return runWithMigrationCheck(args, getenv, sandboxrunner.CheckRequiredMigration)
}

func runWithMigrationCheck(args []string, getenv func(string) string, checkMigration migrationCheckFunc) int {
	if len(args) == 1 && args[0] == "migration-status" {
		if checkMigration == nil || checkMigration(context.Background(), getenv) != nil {
			log.Print("sandbox runner migration status is unavailable")
			return 1
		}
		log.Print("sandbox runner migration status is applied")
		return 0
	}
	if len(args) != 0 {
		log.Print("sandbox runner command is invalid")
		return 1
	}

	config, err := sandboxrunner.LoadConfig(getenv)
	if err != nil {
		log.Print("sandbox runner configuration is invalid")
		return 1
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	runtime, err := sandboxrunner.NewProcessRuntime(ctx, config)
	if err != nil {
		log.Print("sandbox runner runtime dependencies are unavailable")
		return 1
	}
	if err := runtime.Run(ctx, config); err != nil {
		log.Print("sandbox runner stopped")
		return 1
	}
	return 0
}
