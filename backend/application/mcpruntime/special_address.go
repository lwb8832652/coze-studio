// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package mcpruntime

import "net/netip"

var (
	ipv4TranslatedPrefix = netip.MustParsePrefix("::ffff:0:0:0/96")
	nat64WellKnownPrefix = netip.MustParsePrefix("64:ff9b::/96")
	nat64LocalUsePrefix  = netip.MustParsePrefix("64:ff9b:1::/48")
	sixToFourPrefix      = netip.MustParsePrefix("2002::/16")
	teredoPrefix         = netip.MustParsePrefix("2001::/32")
)

var specialPurposeIPv4Prefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("169.254.0.0/16"),
	netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.31.196.0/24"),
	netip.MustParsePrefix("192.52.193.0/24"),
	netip.MustParsePrefix("192.88.99.0/24"),
	netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("192.175.48.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("224.0.0.0/4"),
	netip.MustParsePrefix("240.0.0.0/4"),
}

var specialPurposeIPv6Prefixes = []netip.Prefix{
	netip.MustParsePrefix("::/96"),
	netip.MustParsePrefix("::ffff:0:0/96"),
	ipv4TranslatedPrefix,
	nat64WellKnownPrefix,
	nat64LocalUsePrefix,
	netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("2001::/23"),
	netip.MustParsePrefix("2001:db8::/32"),
	sixToFourPrefix,
	netip.MustParsePrefix("2620:4f:8000::/48"),
	netip.MustParsePrefix("3fff::/20"),
	netip.MustParsePrefix("5f00::/16"),
	netip.MustParsePrefix("fc00::/7"),
	netip.MustParsePrefix("fe80::/10"),
	netip.MustParsePrefix("fec0::/10"),
	netip.MustParsePrefix("ff00::/8"),
}

func isSpecialPurposeAddress(address netip.Addr) bool {
	prefixes := specialPurposeIPv6Prefixes
	if address.Is4() {
		prefixes = specialPurposeIPv4Prefixes
	}
	for _, prefix := range prefixes {
		if prefix.Contains(address) {
			return true
		}
	}
	return false
}

func unsafeEmbeddedIPv4Address(address netip.Addr) bool {
	return !address.IsValid() || !address.Is4() || isSpecialPurposeAddress(address)
}

// embeddedIPv4Address extracts IPv4 payloads from address forms that can
// otherwise disguise access to an internal or metadata endpoint.
func embeddedIPv4Address(address netip.Addr) (netip.Addr, bool) {
	if !address.IsValid() {
		return netip.Addr{}, false
	}
	if address.Is4In6() {
		return address.Unmap(), true
	}
	if !address.Is6() {
		return netip.Addr{}, false
	}
	bytes16 := address.As16()
	switch {
	case ipv4TranslatedPrefix.Contains(address):
		return netip.AddrFrom4([4]byte{bytes16[12], bytes16[13], bytes16[14], bytes16[15]}), true
	case nat64WellKnownPrefix.Contains(address):
		return netip.AddrFrom4([4]byte{bytes16[12], bytes16[13], bytes16[14], bytes16[15]}), true
	case nat64LocalUsePrefix.Contains(address):
		return netip.AddrFrom4([4]byte{bytes16[6], bytes16[7], bytes16[9], bytes16[10]}), true
	case sixToFourPrefix.Contains(address):
		return netip.AddrFrom4([4]byte{bytes16[2], bytes16[3], bytes16[4], bytes16[5]}), true
	case teredoPrefix.Contains(address):
		return netip.AddrFrom4([4]byte{^bytes16[12], ^bytes16[13], ^bytes16[14], ^bytes16[15]}), true
	default:
		return netip.Addr{}, false
	}
}
