// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package safehttp

import (
	"errors"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"

	"golang.org/x/net/idna"
)

var ErrURLRejected = errors.New("safe HTTP URL is not allowed")

type PolicyOptions struct {
	AllowedHosts        []string
	AllowedPrivateCIDRs []string
	RejectedError       error
}

type DebugPolicyOptions struct {
	AllowedHosts        []string
	AllowedPrivateCIDRs []string
	RejectedError       error
}

type allowedAuthority struct {
	host    string
	port    string
	hasPort bool
}

type Policy struct {
	allowed             []allowedAuthority
	allowedPrivateCIDRs []netip.Prefix
	debugLocalHTTP      bool
	rejectedError       error
}

func NewPolicy(options PolicyOptions) (*Policy, error) {
	return newPolicy(options, false)
}

func NewDebugAuthorizedPolicy(options DebugPolicyOptions) (*Policy, error) {
	return newPolicy(PolicyOptions{
		AllowedHosts:        options.AllowedHosts,
		AllowedPrivateCIDRs: options.AllowedPrivateCIDRs,
		RejectedError:       options.RejectedError,
	}, true)
}

func newPolicy(options PolicyOptions, debugLocalHTTP bool) (*Policy, error) {
	rejectedError := options.RejectedError
	if rejectedError == nil {
		rejectedError = ErrURLRejected
	}
	if len(options.AllowedHosts) == 0 {
		return nil, rejectedError
	}
	allowed := make([]allowedAuthority, 0, len(options.AllowedHosts))
	seenAuthorities := make(map[string]struct{}, len(options.AllowedHosts))
	for _, value := range options.AllowedHosts {
		authority, err := parseAllowedAuthority(value)
		if err != nil {
			return nil, rejectedError
		}
		key := authority.host + "|" + authority.port + "|" + strconv.FormatBool(authority.hasPort)
		if _, exists := seenAuthorities[key]; exists {
			continue
		}
		seenAuthorities[key] = struct{}{}
		allowed = append(allowed, authority)
	}
	if len(allowed) == 0 {
		return nil, rejectedError
	}

	privateCIDRs := make([]netip.Prefix, 0, len(options.AllowedPrivateCIDRs))
	seenCIDRs := make(map[string]struct{}, len(options.AllowedPrivateCIDRs))
	for _, value := range options.AllowedPrivateCIDRs {
		if value == "" || strings.TrimSpace(value) != value {
			return nil, rejectedError
		}
		prefix, err := netip.ParsePrefix(value)
		if err != nil || !prefix.IsValid() || prefix.Addr().Zone() != "" || prefix.Addr().Is4In6() || prefix != prefix.Masked() {
			return nil, rejectedError
		}
		key := prefix.String()
		if _, exists := seenCIDRs[key]; exists {
			continue
		}
		seenCIDRs[key] = struct{}{}
		privateCIDRs = append(privateCIDRs, prefix)
	}

	return &Policy{
		allowed:             allowed,
		allowedPrivateCIDRs: privateCIDRs,
		debugLocalHTTP:      debugLocalHTTP,
		rejectedError:       rejectedError,
	}, nil
}

func (p *Policy) ValidateURL(value *url.URL) error {
	if p == nil || value == nil || value.Opaque != "" || value.Host == "" || value.User != nil ||
		value.Fragment != "" || value.RawFragment != "" || strings.ContainsAny(value.Host, "\\\r\n\t") {
		return p.reject()
	}
	if err := validateURLAuthority(value); err != nil {
		return p.reject()
	}
	scheme := strings.ToLower(value.Scheme)
	debugLoopbackHTTP := scheme == "http" && p.debugLocalHTTP
	if scheme != "https" && !debugLoopbackHTTP {
		return p.reject()
	}
	host, err := CanonicalHostname(value.Hostname())
	if err != nil {
		return p.reject()
	}
	if debugLoopbackHTTP && !isDebugLoopbackHost(host) {
		return p.reject()
	}
	port, err := EffectivePort(value)
	if err != nil || !p.authorityAllowed(host, port, scheme) {
		return p.reject()
	}
	if address, parseErr := netip.ParseAddr(host); parseErr == nil && !p.addressAllowed(address, debugLoopbackHTTP) {
		return p.reject()
	}
	return nil
}

func (p *Policy) ValidateRequest(request *http.Request) error {
	if request == nil || request.URL == nil || p.ValidateURL(request.URL) != nil {
		return p.reject()
	}
	if request.Host == "" {
		return nil
	}
	authority, err := parseAllowedAuthority(request.Host)
	if err != nil {
		return p.reject()
	}
	host, err := CanonicalHostname(request.URL.Hostname())
	if err != nil || authority.host != host {
		return p.reject()
	}
	port, err := EffectivePort(request.URL)
	if err != nil {
		return p.reject()
	}
	requestPort := DefaultPort(strings.ToLower(request.URL.Scheme))
	if authority.hasPort {
		requestPort = authority.port
	}
	if requestPort != port {
		return p.reject()
	}
	return nil
}

func (p *Policy) authorityAllowed(host, port, scheme string) bool {
	for _, allowed := range p.allowed {
		if allowed.host != host {
			continue
		}
		if allowed.hasPort {
			if allowed.port == port {
				return true
			}
			continue
		}
		if port == DefaultPort(scheme) {
			return true
		}
	}
	return false
}

func (p *Policy) dialAuthorityAllowed(host, port string) bool {
	if p == nil {
		return false
	}
	if p.authorityAllowed(host, port, "https") {
		return true
	}
	return p.debugLocalHTTP && isDebugLoopbackHost(host) && p.authorityAllowed(host, port, "http")
}

func (p *Policy) addressAllowed(address netip.Addr, debugLoopbackHTTP bool) bool {
	if debugLoopbackHTTP {
		return !address.Is4In6() && address.IsLoopback()
	}
	return isAddressAllowed(address, false, p.allowedPrivateCIDRs)
}

func (p *Policy) reject() error {
	if p == nil || p.rejectedError == nil {
		return ErrURLRejected
	}
	return p.rejectedError
}

func CanonicalHostname(value string) (string, error) {
	if value == "" || strings.TrimSpace(value) != value || strings.Contains(value, "%") || strings.HasSuffix(value, "..") {
		return "", ErrURLRejected
	}
	value = strings.TrimSuffix(value, ".")
	if value == "" {
		return "", ErrURLRejected
	}
	if isASCII(value) {
		if address, err := netip.ParseAddr(value); err == nil {
			if address.Zone() != "" || address.Is4In6() {
				return "", ErrURLRejected
			}
			return address.String(), nil
		}
	}
	asciiValue, err := idna.Lookup.ToASCII(value)
	if err != nil || asciiValue == "" || !isASCII(asciiValue) {
		return "", ErrURLRejected
	}
	value = asciiValue
	if address, err := netip.ParseAddr(value); err == nil {
		if address.Zone() != "" || address.Is4In6() {
			return "", ErrURLRejected
		}
		return address.String(), nil
	}
	value = strings.ToLower(value)
	if len(value) > 253 {
		return "", ErrURLRejected
	}
	if looksLikeNonCanonicalIPv4(value) {
		return "", ErrURLRejected
	}
	for _, label := range strings.Split(value, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return "", ErrURLRejected
		}
		for index := range label {
			character := label[index]
			if !isASCIIAlphaNumeric(character) && character != '-' {
				return "", ErrURLRejected
			}
		}
	}
	return value, nil
}

func isDebugLoopbackHost(host string) bool {
	if address, err := netip.ParseAddr(host); err == nil {
		return !address.Is4In6() && address.IsLoopback()
	}
	return host == "localhost" || strings.HasSuffix(host, ".localhost")
}

func looksLikeNonCanonicalIPv4(host string) bool {
	if strings.Contains(host, ":") {
		return false
	}
	parts := strings.Split(host, ".")
	if len(parts) == 0 || len(parts) > 4 {
		return false
	}
	for _, part := range parts {
		if part == "" {
			return false
		}
		candidate := part
		if strings.HasPrefix(candidate, "0x") {
			candidate = candidate[2:]
			if candidate == "" {
				return false
			}
			for _, character := range candidate {
				if !((character >= '0' && character <= '9') || (character >= 'a' && character <= 'f')) {
					return false
				}
			}
			continue
		}
		for _, character := range candidate {
			if character < '0' || character > '9' {
				return false
			}
		}
	}
	return true
}

func CanonicalPort(value string) (string, error) {
	if value == "" || strings.TrimSpace(value) != value {
		return "", ErrURLRejected
	}
	port, err := strconv.Atoi(value)
	if err != nil || port <= 0 || port > 65535 {
		return "", ErrURLRejected
	}
	return strconv.Itoa(port), nil
}

func EffectivePort(value *url.URL) (string, error) {
	if value == nil {
		return "", ErrURLRejected
	}
	scheme := strings.ToLower(value.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", ErrURLRejected
	}
	if value.Port() == "" {
		return DefaultPort(scheme), nil
	}
	return CanonicalPort(value.Port())
}

func DefaultPort(scheme string) string {
	if strings.ToLower(scheme) == "https" {
		return "443"
	}
	return "80"
}

func SameOrigin(left, right *url.URL) (bool, error) {
	if left == nil || right == nil {
		return false, ErrURLRejected
	}
	leftHost, err := CanonicalHostname(left.Hostname())
	if err != nil {
		return false, err
	}
	rightHost, err := CanonicalHostname(right.Hostname())
	if err != nil {
		return false, err
	}
	leftPort, err := EffectivePort(left)
	if err != nil {
		return false, err
	}
	rightPort, err := EffectivePort(right)
	if err != nil {
		return false, err
	}
	return strings.EqualFold(left.Scheme, right.Scheme) && leftHost == rightHost && leftPort == rightPort, nil
}

func parseAllowedAuthority(value string) (allowedAuthority, error) {
	if value == "" || strings.TrimSpace(value) != value || !isASCII(value) || strings.Contains(value, "://") || strings.ContainsAny(value, "/?#@\\") {
		return allowedAuthority{}, ErrURLRejected
	}
	host := value
	port := ""
	hasPort := false
	if strings.HasPrefix(value, "[") {
		closing := strings.IndexByte(value, ']')
		if closing <= 1 {
			return allowedAuthority{}, ErrURLRejected
		}
		host = value[1:closing]
		address, parseErr := netip.ParseAddr(host)
		if parseErr != nil || !address.Is6() || address.Is4In6() || address.Zone() != "" {
			return allowedAuthority{}, ErrURLRejected
		}
		remainder := value[closing+1:]
		if remainder != "" {
			if !strings.HasPrefix(remainder, ":") || len(remainder) == 1 {
				return allowedAuthority{}, ErrURLRejected
			}
			port = remainder[1:]
			hasPort = true
		}
	} else if strings.Count(value, ":") == 1 {
		separator := strings.LastIndexByte(value, ':')
		host, port, hasPort = value[:separator], value[separator+1:], true
	} else if strings.Count(value, ":") > 1 {
		if _, err := netip.ParseAddr(value); err != nil {
			return allowedAuthority{}, ErrURLRejected
		}
	}
	host, err := CanonicalHostname(host)
	if err != nil {
		return allowedAuthority{}, err
	}
	if hasPort {
		port, err = CanonicalPort(port)
		if err != nil {
			return allowedAuthority{}, err
		}
	}
	return allowedAuthority{host: host, port: port, hasPort: hasPort}, nil
}

func validateURLAuthority(value *url.URL) error {
	if value == nil || value.Host == "" || !isASCII(value.Host) {
		return ErrURLRejected
	}
	authority := value.Host
	if strings.HasPrefix(authority, "[") {
		closing := strings.IndexByte(authority, ']')
		if closing <= 1 {
			return ErrURLRejected
		}
		address, err := netip.ParseAddr(authority[1:closing])
		if err != nil || !address.Is6() || address.Is4In6() || address.Zone() != "" {
			return ErrURLRejected
		}
		remainder := authority[closing+1:]
		if remainder == "" {
			return nil
		}
		if !strings.HasPrefix(remainder, ":") || len(remainder) == 1 {
			return ErrURLRejected
		}
		_, err = CanonicalPort(remainder[1:])
		return err
	}
	if strings.ContainsAny(authority, "[]") || strings.Count(authority, ":") > 1 || strings.HasSuffix(authority, ":") {
		return ErrURLRejected
	}
	if port := value.Port(); port != "" {
		_, err := CanonicalPort(port)
		return err
	}
	return nil
}

func formatAuthority(host, port string, includePort bool) string {
	if includePort {
		return net.JoinHostPort(host, port)
	}
	if strings.Contains(host, ":") {
		return "[" + host + "]"
	}
	return host
}

func isASCII(value string) bool {
	for index := range value {
		if value[index] < 0x21 || value[index] > 0x7e {
			return false
		}
	}
	return true
}

func isASCIIAlphaNumeric(character byte) bool {
	return character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9'
}
