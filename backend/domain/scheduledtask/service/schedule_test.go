// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"testing"
	"time"

	"github.com/coze-dev/coze-studio/backend/domain/scheduledtask/entity"
	"github.com/stretchr/testify/require"
)

func TestNextExecutionAt(t *testing.T) {
	t.Parallel()

	shanghai, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	now := time.Date(2026, 7, 14, 10, 20, 30, 0, shanghai)

	tests := []struct {
		name     string
		schedule entity.Schedule
		want     time.Time
	}{
		{
			name: "one time",
			schedule: entity.Schedule{
				Type:      entity.ScheduleTypeOnce,
				Timezone:  "Asia/Shanghai",
				RunOnceAt: now.Add(30 * time.Minute).UnixMilli(),
			},
			want: now.Add(30 * time.Minute),
		},
		{
			name: "hourly",
			schedule: entity.Schedule{
				Type:     entity.ScheduleTypeHourly,
				Timezone: "Asia/Shanghai",
				Minute:   15,
			},
			want: time.Date(2026, 7, 14, 11, 15, 0, 0, shanghai),
		},
		{
			name: "daily",
			schedule: entity.Schedule{
				Type:     entity.ScheduleTypeDaily,
				Timezone: "Asia/Shanghai",
				Hour:     9,
				Minute:   0,
			},
			want: time.Date(2026, 7, 15, 9, 0, 0, 0, shanghai),
		},
		{
			name: "weekly",
			schedule: entity.Schedule{
				Type:     entity.ScheduleTypeWeekly,
				Timezone: "Asia/Shanghai",
				Weekday: int(time.Monday),
				Hour:     8,
				Minute:   30,
			},
			want: time.Date(2026, 7, 20, 8, 30, 0, 0, shanghai),
		},
		{
			name: "cron",
			schedule: entity.Schedule{
				Type:     entity.ScheduleTypeCron,
				Timezone: "Asia/Shanghai",
				CronExpr: "0 9 * * 1-5",
			},
			want: time.Date(2026, 7, 15, 9, 0, 0, 0, shanghai),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			next, err := NextExecutionAt(tt.schedule, now)
			require.NoError(t, err)
			require.Equal(t, tt.want.UnixMilli(), next)
		})
	}
}

func TestNextExecutionAtRejectsUnsafeSchedules(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 14, 10, 20, 30, 0, time.UTC)
	tests := []entity.Schedule{
		{Type: entity.ScheduleTypeOnce, Timezone: "UTC", RunOnceAt: now.Add(-time.Second).UnixMilli()},
		{Type: entity.ScheduleTypeHourly, Timezone: "UTC", Minute: 60},
		{Type: entity.ScheduleTypeDaily, Timezone: "UTC", Hour: 24},
		{Type: entity.ScheduleTypeWeekly, Timezone: "UTC", Weekday: 7},
		{Type: entity.ScheduleTypeCron, Timezone: "UTC", CronExpr: "*/1 * * * * *"},
		{Type: entity.ScheduleTypeCron, Timezone: "not/a-zone", CronExpr: "0 9 * * *"},
	}

	for _, schedule := range tests {
		_, err := NextExecutionAt(schedule, now)
		require.Error(t, err)
	}
}
