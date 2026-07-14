// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"fmt"
	"strings"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/coze-dev/coze-studio/backend/domain/scheduledtask/entity"
)

func NextExecutionAt(schedule entity.Schedule, now time.Time) (int64, error) {
	location, err := time.LoadLocation(schedule.Timezone)
	if err != nil {
		return 0, fmt.Errorf("invalid timezone: %w", err)
	}
	now = now.In(location)

	var next time.Time
	switch schedule.Type {
	case entity.ScheduleTypeOnce:
		next = time.UnixMilli(schedule.RunOnceAt).In(location)
		if !next.After(now) {
			return 0, fmt.Errorf("one-time execution must be in the future")
		}
	case entity.ScheduleTypeHourly:
		if schedule.Minute < 0 || schedule.Minute > 59 {
			return 0, fmt.Errorf("minute must be between 0 and 59")
		}
		next = time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), schedule.Minute, 0, 0, location)
		if !next.After(now) {
			next = next.Add(time.Hour)
		}
	case entity.ScheduleTypeDaily:
		if err := validateClock(schedule.Hour, schedule.Minute); err != nil {
			return 0, err
		}
		next = time.Date(now.Year(), now.Month(), now.Day(), schedule.Hour, schedule.Minute, 0, 0, location)
		if !next.After(now) {
			next = next.AddDate(0, 0, 1)
		}
	case entity.ScheduleTypeWeekly:
		if schedule.Weekday < int(time.Sunday) || schedule.Weekday > int(time.Saturday) {
			return 0, fmt.Errorf("weekday must be between 0 and 6")
		}
		if err := validateClock(schedule.Hour, schedule.Minute); err != nil {
			return 0, err
		}
		days := (schedule.Weekday - int(now.Weekday()) + 7) % 7
		next = time.Date(now.Year(), now.Month(), now.Day(), schedule.Hour, schedule.Minute, 0, 0, location).AddDate(0, 0, days)
		if !next.After(now) {
			next = next.AddDate(0, 0, 7)
		}
	case entity.ScheduleTypeCron:
		if len(strings.Fields(schedule.CronExpr)) != 5 {
			return 0, fmt.Errorf("cron expression must contain exactly five fields")
		}
		parsed, parseErr := cron.ParseStandard("CRON_TZ=" + schedule.Timezone + " " + schedule.CronExpr)
		if parseErr != nil {
			return 0, fmt.Errorf("invalid cron expression: %w", parseErr)
		}
		next = parsed.Next(now)
		if next.IsZero() {
			return 0, fmt.Errorf("cron expression has no next execution")
		}
	default:
		return 0, fmt.Errorf("unsupported schedule type %q", schedule.Type)
	}

	return next.UnixMilli(), nil
}

func validateClock(hour, minute int) error {
	if hour < 0 || hour > 23 {
		return fmt.Errorf("hour must be between 0 and 23")
	}
	if minute < 0 || minute > 59 {
		return fmt.Errorf("minute must be between 0 and 59")
	}
	return nil
}
