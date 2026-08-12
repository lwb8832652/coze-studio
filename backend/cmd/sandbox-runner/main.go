// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"log"
	"os"

	"github.com/coze-dev/coze-studio/backend/internal/sandboxrunner"
)

func main() {
	if _, err := sandboxrunner.LoadConfig(os.Getenv); err != nil {
		log.Fatal("sandbox runner configuration is invalid")
	}
	// The scheduler, Redis store, and rootless lifecycle manager are composed by
	// the later Runner runtime tasks. Refuse to bind before those dependencies
	// exist rather than advertising a healthy execution service with no runtime.
	log.Fatal("sandbox runner runtime dependencies are not configured")
}
