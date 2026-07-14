// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package mcptool

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	toolapi "github.com/coze-dev/coze-studio/backend/api/model/workbench/tool"
)

type mutatingCapabilityDiscoverer struct {
	mutate func() error
	result *DiscoveredCapabilities
}

func (d *mutatingCapabilityDiscoverer) Discover(
	_ context.Context,
	_ MCPServerConnection,
) (*DiscoveredCapabilities, error) {
	if err := d.mutate(); err != nil {
		return nil, err
	}
	return d.result, nil
}

func TestApplicationServiceDiscoverCASDoesNotOverwriteConcurrentConfigEdit(t *testing.T) {
	ctx := managementContext(7)
	catalog := NewInMemoryCatalog()
	server := managementServer(41, 10)
	server.UpdatedAt = 100
	server.Tools = []*toolapi.MCPToolDefinition{{Name: "original", InputSchema: `{}`}}
	require.NoError(t, catalog.Upsert(ctx, server))
	discoverer := &mutatingCapabilityDiscoverer{
		mutate: func() error {
			current, err := catalog.Get(ctx, 41)
			if err != nil {
				return err
			}
			current.Config = `{"url":"https://concurrent-edit.example.com"}`
			current.UpdatedAt = 101
			return catalog.Upsert(ctx, current)
		},
		result: &DiscoveredCapabilities{Tools: []*toolapi.MCPToolDefinition{{
			Name: "discovered", InputSchema: `{}`,
		}}},
	}
	svc := NewApplicationService(&Components{
		Catalog:              catalog,
		UserSpaceRoleReader:  ownerRoleReader(10),
		CapabilityDiscoverer: discoverer,
	})

	_, err := svc.Discover(ctx, 41)

	require.ErrorIs(t, err, ErrMCPConflict)
	stored, getErr := catalog.Get(ctx, 41)
	require.NoError(t, getErr)
	require.JSONEq(t, `{"url":"https://concurrent-edit.example.com"}`, stored.Config)
	require.Equal(t, "original", stored.Tools[0].Name)
}

func TestApplicationServiceRuntimeUnavailableDoesNotChangeHealth(t *testing.T) {
	ctx := managementContext(7)
	catalog := NewInMemoryCatalog()
	server := managementServer(41, 10)
	server.HealthStatus = "healthy"
	server.HealthCheckedAt = 77
	server.HealthLatencyMs = 9
	require.NoError(t, catalog.Upsert(ctx, server))
	svc := NewApplicationService(&Components{
		Catalog:             catalog,
		UserSpaceRoleReader: ownerRoleReader(10),
	})

	_, err := svc.TestCall(ctx, &toolapi.TestMCPToolCallRequest{
		ServerID: 41, ToolName: "search", Arguments: `{}`,
	})

	require.ErrorIs(t, err, ErrRuntimeUnavailable)
	stored, getErr := catalog.Get(ctx, 41)
	require.NoError(t, getErr)
	require.Equal(t, "healthy", stored.HealthStatus)
	require.Equal(t, int64(77), stored.HealthCheckedAt)
	require.Equal(t, int64(9), stored.HealthLatencyMs)
}

func TestDiscoverCapabilitiesRejectsInvalidOrUnboundedPayloads(t *testing.T) {
	largeResources := make([]*toolapi.MCPResource, 0, 300)
	for i := 0; i < 300; i++ {
		largeResources = append(largeResources, &toolapi.MCPResource{
			URI:         fmt.Sprintf("resource://%d", i),
			Description: strings.Repeat("x", maxMCPDescriptionRunes),
		})
	}
	tests := []struct {
		name   string
		result *DiscoveredCapabilities
	}{
		{name: "empty tool name", result: &DiscoveredCapabilities{Tools: []*toolapi.MCPToolDefinition{{InputSchema: `{}`}}}},
		{name: "invalid tool schema", result: &DiscoveredCapabilities{Tools: []*toolapi.MCPToolDefinition{{Name: "search", InputSchema: `[]`}}}},
		{name: "normalized duplicate tools", result: &DiscoveredCapabilities{Tools: []*toolapi.MCPToolDefinition{
			{Name: "foo-bar", InputSchema: `{}`},
			{Name: "foo_bar", InputSchema: `{}`},
		}}},
		{name: "oversized field", result: &DiscoveredCapabilities{Prompts: []*toolapi.MCPPrompt{{
			Name: "summarize", Description: strings.Repeat("x", maxMCPDescriptionRunes+1),
		}}}},
		{name: "oversized serialized payload", result: &DiscoveredCapabilities{Resources: largeResources}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := discoverCapabilities(context.Background(), &managementDiscoverer{result: tt.result}, MCPServerConnection{})
			require.Error(t, err)
			require.True(t,
				errors.Is(err, ErrCapabilityDiscoveryInvalid) || errors.Is(err, ErrCapabilityDiscoveryTooLarge),
			)
		})
	}
}

func TestApplicationServiceDiscoverValidationFailureDoesNotWriteCatalog(t *testing.T) {
	ctx := managementContext(7)
	catalog := NewInMemoryCatalog()
	server := managementServer(41, 10)
	server.Tools = []*toolapi.MCPToolDefinition{{Name: "original", InputSchema: `{}`}}
	require.NoError(t, catalog.Upsert(ctx, server))
	svc := NewApplicationService(&Components{
		Catalog:             catalog,
		UserSpaceRoleReader: ownerRoleReader(10),
		CapabilityDiscoverer: &managementDiscoverer{result: &DiscoveredCapabilities{
			Tools: []*toolapi.MCPToolDefinition{
				{Name: "foo-bar", InputSchema: `{}`},
				{Name: "foo_bar", InputSchema: `{}`},
			},
		}},
	})

	_, err := svc.Discover(ctx, 41)

	require.Error(t, err)
	stored, getErr := catalog.Get(ctx, 41)
	require.NoError(t, getErr)
	require.Equal(t, "original", stored.Tools[0].Name)
}
