// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package mcpruntime

import (
	"context"
	"encoding/json"
)

type discoveryBudget struct {
	limits DiscoveryLimits
	items  int
	bytes  int
}

func Discover(
	ctx context.Context,
	session Session,
	limits DiscoveryLimits,
) (*DiscoveredCapabilities, error) {
	if session == nil {
		return nil, ErrSessionUnavailable
	}
	limits = discoveryLimitsWithDefaults(limits)
	budget := &discoveryBudget{limits: limits}
	result := &DiscoveredCapabilities{}
	capabilities := session.Capabilities()
	if capabilities.Tools {
		items, err := discoverTools(ctx, session, budget)
		if err != nil {
			return nil, err
		}
		result.Tools = items
	}
	if capabilities.Resources {
		items, err := discoverResources(ctx, session, budget)
		if err != nil {
			return nil, err
		}
		result.Resources = items
	}
	if capabilities.Prompts {
		items, err := discoverPrompts(ctx, session, budget)
		if err != nil {
			return nil, err
		}
		result.Prompts = items
	}
	return result, nil
}

func discoverTools(ctx context.Context, session Session, budget *discoveryBudget) ([]Tool, error) {
	return listToolsWithBudget(ctx, session, budget, "")
}

func discoverResources(ctx context.Context, session Session, budget *discoveryBudget) ([]Resource, error) {
	items := []Resource{}
	cursor := ""
	seen := map[string]struct{}{}
	for pageCount := 0; ; pageCount++ {
		if pageCount >= budget.limits.MaxPages {
			return nil, ErrDiscoveryLimitExceeded
		}
		page, err := session.ListResourcesByPage(ctx, cursor)
		if err != nil {
			return nil, ErrSessionUnavailable
		}
		if err := validateDiscoveryPage(budget, page.Items, page.NextCursor, seen); err != nil {
			return nil, err
		}
		for _, item := range page.Items {
			items = append(items, item)
		}
		if page.NextCursor == "" {
			return items, nil
		}
		cursor = page.NextCursor
	}
}

func discoverPrompts(ctx context.Context, session Session, budget *discoveryBudget) ([]Prompt, error) {
	items := []Prompt{}
	cursor := ""
	seen := map[string]struct{}{}
	for pageCount := 0; ; pageCount++ {
		if pageCount >= budget.limits.MaxPages {
			return nil, ErrDiscoveryLimitExceeded
		}
		page, err := session.ListPromptsByPage(ctx, cursor)
		if err != nil {
			return nil, ErrSessionUnavailable
		}
		if err := validateDiscoveryPage(budget, page.Items, page.NextCursor, seen); err != nil {
			return nil, err
		}
		for _, item := range page.Items {
			item.Arguments = append([]PromptArgument(nil), item.Arguments...)
			items = append(items, item)
		}
		if page.NextCursor == "" {
			return items, nil
		}
		cursor = page.NextCursor
	}
}

func (b *discoveryBudget) add(value any) error {
	pageBytes := 0
	return b.addPageItem(value, &pageBytes)
}

func (b *discoveryBudget) addPageItem(value any, pageBytes *int) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return ErrSessionUnavailable
	}
	b.items++
	b.bytes += len(encoded)
	*pageBytes += len(encoded)
	if b.items > b.limits.MaxItems || b.bytes > b.limits.MaxBytes ||
		*pageBytes > b.limits.MaxPageBytes {
		return ErrDiscoveryLimitExceeded
	}
	return nil
}

func validateDiscoveryPage[T any](
	budget *discoveryBudget,
	items []T,
	nextCursor string,
	seen map[string]struct{},
) error {
	pageBytes := 0
	for _, item := range items {
		if err := budget.addPageItem(item, &pageBytes); err != nil {
			return err
		}
	}
	if nextCursor == "" {
		return nil
	}
	cursorBytes := len([]byte(nextCursor))
	if cursorBytes > budget.limits.MaxCursorBytes {
		return ErrDiscoveryLimitExceeded
	}
	if budget.bytes > budget.limits.MaxBytes-cursorBytes ||
		pageBytes > budget.limits.MaxPageBytes-cursorBytes {
		return ErrDiscoveryLimitExceeded
	}
	budget.bytes += cursorBytes
	if _, exists := seen[nextCursor]; exists {
		return ErrDiscoveryLimitExceeded
	}
	seen[nextCursor] = struct{}{}
	return nil
}

func discoveryLimitsWithDefaults(limits DiscoveryLimits) DiscoveryLimits {
	defaults := DefaultDiscoveryLimits()
	if limits.MaxItems <= 0 {
		limits.MaxItems = defaults.MaxItems
	}
	if limits.MaxBytes <= 0 {
		limits.MaxBytes = defaults.MaxBytes
	}
	if limits.MaxPageBytes <= 0 {
		limits.MaxPageBytes = defaults.MaxPageBytes
	}
	if limits.MaxPages <= 0 {
		limits.MaxPages = defaults.MaxPages
	}
	if limits.MaxCursorBytes <= 0 {
		limits.MaxCursorBytes = defaults.MaxCursorBytes
	}
	return limits
}
