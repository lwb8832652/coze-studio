// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package runtime

import (
	"context"
	"errors"
	"net/netip"
	"strings"
)

var errEgressPolicyDenied = errors.New("sandbox egress policy denied")
var errEgressUnavailable = errors.New("sandbox egress policy unavailable")

// EgressResolver is deliberately called for every connection authorization.
// A caller must not cache answers: that would permit DNS rebinding after an
// initial approved resolution.
type EgressResolver interface {
	LookupNetIP(context.Context, string) ([]netip.Addr, error)
}

// ValidateEgressDestination applies the host allowlist before resolving and
// validates each resolved address afterwards. The proxy layer calls this per
// outbound connection; containers never receive raw network access.
func ValidateEgressDestination(ctx context.Context, resolver EgressResolver, allowlist []string, host string) error {
	if ctx == nil || resolver == nil || !allowlistedHost(allowlist, host) {
		return errEgressPolicyDenied
	}
	addresses, err := resolver.LookupNetIP(ctx, host)
	if err != nil || len(addresses) == 0 {
		return errEgressUnavailable
	}
	for _, address := range addresses {
		if !allowedEgressAddress(address) {
			return errEgressPolicyDenied
		}
	}
	return nil
}

func allowlistedHost(allowlist []string, host string) bool {
	host = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	if host == "" || strings.ContainsAny(host, "/:@") {
		return false
	}
	for _, entry := range allowlist {
		entry = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(entry)), ".")
		switch {
		case entry == host:
			return true
		case strings.HasPrefix(entry, "*."):
			base := strings.TrimPrefix(entry, "*.")
			if base != "" && host != base && strings.HasSuffix(host, "."+base) {
				return true
			}
		}
	}
	return false
}

func allowedEgressAddress(address netip.Addr) bool {
	if !address.IsValid() || address.IsUnspecified() || address.IsLoopback() || address.IsLinkLocalUnicast() || address.IsLinkLocalMulticast() || address.IsPrivate() || address.IsMulticast() {
		return false
	}
	if address.Is4() {
		// RFC 6598 shared address space must not route from untrusted sandboxes.
		return !netip.MustParsePrefix("100.64.0.0/10").Contains(address)
	}
	return true
}
