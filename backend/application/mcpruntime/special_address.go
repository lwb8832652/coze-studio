// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package mcpruntime

import (
	"net/netip"

	"github.com/coze-dev/coze-studio/backend/pkg/safehttp"
)

func isSpecialPurposeAddress(address netip.Addr) bool {
	return safehttp.IsSpecialPurposeAddress(address)
}

func unsafeEmbeddedIPv4Address(address netip.Addr) bool {
	return safehttp.UnsafeEmbeddedIPv4Address(address)
}

func embeddedIPv4Address(address netip.Addr) (netip.Addr, bool) {
	return safehttp.EmbeddedIPv4Address(address)
}

func unsafeHTTPAddress(address netip.Addr, allowLoopback bool) bool {
	if allowLoopback && address.IsLoopback() {
		return false
	}
	return !safehttp.IsAddressAllowed(address, nil)
}
