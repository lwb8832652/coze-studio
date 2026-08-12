// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package runtime

import (
	"context"
	"errors"
	"net/netip"
	"testing"
)

func TestValidateEgressDestinationFailsClosedForUntrustedResolutions(t *testing.T) {
	tests := []struct {
		name string
		ip   string
	}{
		{name: "loopback", ip: "127.0.0.1"},
		{name: "ipv6 loopback", ip: "::1"},
		{name: "private", ip: "10.0.0.8"},
		{name: "link local", ip: "169.254.169.254"},
		{name: "carrier grade nat", ip: "100.64.0.1"},
		{name: "multicast", ip: "224.0.0.1"},
		{name: "unspecified", ip: "0.0.0.0"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resolver := &fakeEgressResolver{addresses: []netip.Addr{netip.MustParseAddr(test.ip)}}
			if err := ValidateEgressDestination(context.Background(), resolver, []string{"api.example.com"}, "api.example.com"); !errors.Is(err, errEgressPolicyDenied) {
				t.Fatalf("ValidateEgressDestination() error = %v, want policy denied", err)
			}
		})
	}
}

func TestValidateEgressDestinationRequiresAllowlistAndRevalidatesDNS(t *testing.T) {
	resolver := &fakeEgressResolver{addresses: []netip.Addr{netip.MustParseAddr("203.0.113.10")}}
	if err := ValidateEgressDestination(context.Background(), resolver, []string{"*.example.com"}, "api.example.com"); err != nil {
		t.Fatalf("validated public destination: %v", err)
	}
	if resolver.calls != 1 {
		t.Fatalf("resolver calls = %d, want 1", resolver.calls)
	}
	if err := ValidateEgressDestination(context.Background(), resolver, []string{"*.example.com"}, "example.com"); !errors.Is(err, errEgressPolicyDenied) {
		t.Fatalf("root hostname wildcard error = %v, want policy denied", err)
	}
	if err := ValidateEgressDestination(context.Background(), resolver, []string{"api.example.com"}, "other.example.com"); !errors.Is(err, errEgressPolicyDenied) {
		t.Fatalf("unlisted hostname error = %v, want policy denied", err)
	}
	resolver.addresses = []netip.Addr{netip.MustParseAddr("127.0.0.1")}
	if err := ValidateEgressDestination(context.Background(), resolver, []string{"api.example.com"}, "api.example.com"); !errors.Is(err, errEgressPolicyDenied) {
		t.Fatalf("DNS rebinding error = %v, want policy denied", err)
	}
}

func TestValidateEgressDestinationFailsClosedOnResolverFailure(t *testing.T) {
	resolver := &fakeEgressResolver{err: errors.New("dns unavailable")}
	if err := ValidateEgressDestination(context.Background(), resolver, []string{"api.example.com"}, "api.example.com"); !errors.Is(err, errEgressUnavailable) {
		t.Fatalf("ValidateEgressDestination() error = %v, want unavailable", err)
	}
}

type fakeEgressResolver struct {
	addresses []netip.Addr
	err       error
	calls     int
}

func (resolver *fakeEgressResolver) LookupNetIP(context.Context, string) ([]netip.Addr, error) {
	resolver.calls++
	return append([]netip.Addr(nil), resolver.addresses...), resolver.err
}
