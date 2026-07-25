// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package billing

import (
	"testing"
	"time"
)

func TestUTCNaiveWallClockUnixMilliIgnoresLocationForSameDatabaseWallValue(t *testing.T) {
	shanghai := time.FixedZone("Asia/Shanghai", 8*60*60)
	utcBoundary := time.Date(2026, time.July, 31, 23, 59, 58, 987_000_000, time.UTC)
	shanghaiBoundary := time.Date(2026, time.July, 31, 23, 59, 58, 987_000_000, shanghai)

	utcVersion, err := UTCNaiveWallClockUnixMilli(utcBoundary)
	if err != nil {
		t.Fatalf("UTCNaiveWallClockUnixMilli(UTC) error = %v", err)
	}
	shanghaiVersion, err := UTCNaiveWallClockUnixMilli(shanghaiBoundary)
	if err != nil {
		t.Fatalf("UTCNaiveWallClockUnixMilli(Shanghai) error = %v", err)
	}
	want := time.Date(2026, time.July, 31, 23, 59, 58, 987_000_000, time.UTC).UnixMilli()
	if utcVersion != want || shanghaiVersion != want {
		t.Fatalf("projection versions = UTC %d Shanghai %d, want %d", utcVersion, shanghaiVersion, want)
	}
}

func TestSubscriptionNotificationEventUsesUTCNaiveWallClockProjectionVersion(t *testing.T) {
	shanghai := time.FixedZone("Asia/Shanghai", 8*60*60)
	route, err := NewBillingNotificationRoute(SubjectTypeWorkspace, 9001, 1001)
	if err != nil {
		t.Fatal(err)
	}
	utcBoundary := time.Date(2026, time.August, 1, 8, 0, 0, 123_000_000, time.UTC)
	shanghaiBoundary := time.Date(2026, time.August, 1, 8, 0, 0, 123_000_000, shanghai)
	utcEvent, err := SubscriptionExpiringNotificationEvent(101, utcBoundary, utcBoundary.Add(-time.Hour), route)
	if err != nil {
		t.Fatal(err)
	}
	shanghaiEvent, err := SubscriptionExpiringNotificationEvent(101, shanghaiBoundary, shanghaiBoundary.Add(-time.Hour), route)
	if err != nil {
		t.Fatal(err)
	}
	if utcEvent.AggregateVersion != shanghaiEvent.AggregateVersion || utcEvent.EventID != shanghaiEvent.EventID {
		t.Fatalf("same wall period produced different identities: UTC=%#v Shanghai=%#v", utcEvent, shanghaiEvent)
	}
}
