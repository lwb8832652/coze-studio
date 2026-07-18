// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"
)

const (
	MaxEndpointURLBytes  = 4096
	maxEndpointHintBytes = 255
)

var (
	ErrEndpointHintInvalid      = errors.New("sandbox endpoint hint is invalid")
	ErrSecretProjectionInvalid  = errors.New("sandbox secret projection is invalid")
	ErrSecretReplacementInvalid = errors.New("sandbox secret replacement is invalid")
)

type SecretEnvelopeInput struct {
	Active      bool
	NeedsRewrap bool
}

type SecretProjectionInput struct {
	EndpointHint          string
	EndpointEnvelope      *SecretEnvelopeInput
	CredentialFingerprint string
	CredentialEnvelope    *SecretEnvelopeInput
}

type SecretProjection struct {
	EndpointHint          string `json:"endpoint_hint"`
	CredentialConfigured  bool   `json:"credential_configured"`
	CredentialFingerprint string `json:"credential_fingerprint"`
	Active                bool   `json:"active"`
	NeedsRewrap           bool   `json:"needs_rewrap"`
}

type SecretField string

const (
	SecretFieldEndpoint   SecretField = "endpoint"
	SecretFieldCredential SecretField = "credential"
)

type SecretReplacementMode string

const (
	SecretReplacementOmitted SecretReplacementMode = "omitted"
	SecretReplacementReplace SecretReplacementMode = "replace"
	SecretReplacementRemove  SecretReplacementMode = "remove"
)

type SecretReplacementInput struct {
	Supplied bool
	Remove   bool
	Value    string
}

type SecretReplacement struct {
	mode  SecretReplacementMode
	value string
}

// DeriveEndpointHint returns display-only metadata. It is not an SSRF
// validation primitive and must never authorize a network destination.
func DeriveEndpointHint(rawEndpoint string) (string, error) {
	if len(rawEndpoint) == 0 ||
		len(rawEndpoint) > MaxEndpointURLBytes ||
		strings.TrimSpace(rawEndpoint) != rawEndpoint ||
		strings.ContainsAny(rawEndpoint, "\x00\r\n") {
		return "", ErrEndpointHintInvalid
	}
	parsed, err := url.Parse(rawEndpoint)
	if err != nil ||
		parsed == nil ||
		parsed.Scheme != "https" ||
		parsed.Host == "" ||
		parsed.Opaque != "" ||
		parsed.User != nil ||
		parsed.RawPath != "" ||
		parsed.Path != "" && parsed.Path != "/" ||
		parsed.RawQuery != "" ||
		parsed.ForceQuery ||
		parsed.Fragment != "" ||
		parsed.RawFragment != "" ||
		strings.ContainsAny(rawEndpoint, "?#") ||
		!asciiEndpointAuthority(parsed.Host) {
		return "", ErrEndpointHintInvalid
	}
	hostname := strings.ToLower(parsed.Hostname())
	if hostname == "" || strings.Contains(hostname, "%") {
		return "", ErrEndpointHintInvalid
	}
	port := parsed.Port()
	if !validEndpointAuthority(parsed.Host, hostname, port) || !validSafePort(port) {
		return "", ErrEndpointHintInvalid
	}
	maskedHost, ipv6, ok := maskedEndpointHost(hostname)
	if !ok {
		return "", ErrEndpointHintInvalid
	}
	if ipv6 {
		maskedHost = "[" + maskedHost + "]"
	}
	hint := "https://" + maskedHost
	if port != "" {
		hint += ":" + port
	}
	if !validProjectedEndpointHint(hint) {
		return "", ErrEndpointHintInvalid
	}
	return hint, nil
}

func ProjectSecrets(input SecretProjectionInput) (SecretProjection, error) {
	if input.EndpointEnvelope == nil {
		if input.EndpointHint != "" {
			return SecretProjection{}, ErrSecretProjectionInvalid
		}
	} else if !validProjectedEndpointHint(input.EndpointHint) || !validProjectionEnvelope(*input.EndpointEnvelope) {
		return SecretProjection{}, ErrSecretProjectionInvalid
	}
	if input.CredentialEnvelope == nil {
		if input.CredentialFingerprint != "" {
			return SecretProjection{}, ErrSecretProjectionInvalid
		}
	} else if !validCredentialFingerprint(input.CredentialFingerprint) || !validProjectionEnvelope(*input.CredentialEnvelope) {
		return SecretProjection{}, ErrSecretProjectionInvalid
	}

	configuredSecrets := 0
	active := true
	needsRewrap := false
	for _, envelope := range []*SecretEnvelopeInput{input.EndpointEnvelope, input.CredentialEnvelope} {
		if envelope == nil {
			continue
		}
		configuredSecrets++
		active = active && envelope.Active
		needsRewrap = needsRewrap || envelope.NeedsRewrap
	}
	if configuredSecrets == 0 {
		active = false
	}
	return SecretProjection{
		EndpointHint:          input.EndpointHint,
		CredentialConfigured:  input.CredentialEnvelope != nil,
		CredentialFingerprint: input.CredentialFingerprint,
		Active:                active,
		NeedsRewrap:           needsRewrap,
	}, nil
}

func ResolveSecretReplacement(field SecretField, input SecretReplacementInput) (SecretReplacement, error) {
	if field != SecretFieldEndpoint && field != SecretFieldCredential {
		return SecretReplacement{}, ErrSecretReplacementInvalid
	}
	if !input.Supplied {
		if input.Remove || input.Value != "" {
			return SecretReplacement{}, ErrSecretReplacementInvalid
		}
		return SecretReplacement{mode: SecretReplacementOmitted}, nil
	}
	if input.Remove {
		if field != SecretFieldCredential || input.Value != "" {
			return SecretReplacement{}, ErrSecretReplacementInvalid
		}
		return SecretReplacement{mode: SecretReplacementRemove}, nil
	}
	if input.Value == "" {
		return SecretReplacement{}, ErrSecretReplacementInvalid
	}
	return SecretReplacement{mode: SecretReplacementReplace, value: input.Value}, nil
}

func (r SecretReplacement) Mode() SecretReplacementMode {
	return r.mode
}

func (r SecretReplacement) Value() (string, bool) {
	if r.mode != SecretReplacementReplace {
		return "", false
	}
	return r.value, true
}

func maskedEndpointHost(hostname string) (string, bool, bool) {
	if ip := net.ParseIP(hostname); ip != nil {
		if ip.To4() != nil {
			return "***.***.***.***", false, true
		}
		if ip.To16() == nil {
			return "", false, false
		}
		return "****", true, true
	}
	if !validDNSHostname(hostname) {
		return "", false, false
	}
	if allNumericDottedHostname(hostname) {
		return "", false, false
	}
	labels := strings.Split(hostname, ".")
	if len(labels) == 1 {
		return "***", false, true
	}
	return "***." + labels[len(labels)-1], false, true
}

func allNumericDottedHostname(hostname string) bool {
	if !strings.Contains(hostname, ".") {
		return false
	}
	for _, label := range strings.Split(hostname, ".") {
		if label == "" {
			return false
		}
		for index := range label {
			if label[index] < '0' || label[index] > '9' {
				return false
			}
		}
	}
	return true
}

func validEndpointAuthority(authority, hostname, port string) bool {
	ip := net.ParseIP(hostname)
	if ip != nil && ip.To4() == nil {
		if !strings.HasPrefix(authority, "[") {
			return false
		}
		closing := strings.LastIndex(authority, "]")
		if closing < 0 || !strings.EqualFold(authority[1:closing], hostname) {
			return false
		}
		suffix := authority[closing+1:]
		return suffix == "" && port == "" || port != "" && suffix == ":"+port
	}
	if strings.ContainsAny(authority, "[]") || strings.Count(authority, ":") > 1 {
		return false
	}
	hostPart := authority
	if strings.Contains(authority, ":") {
		if port == "" || !strings.HasSuffix(authority, ":"+port) {
			return false
		}
		hostPart = strings.TrimSuffix(authority, ":"+port)
	} else if port != "" {
		return false
	}
	return strings.EqualFold(hostPart, hostname)
}

func asciiEndpointAuthority(authority string) bool {
	for index := range authority {
		if authority[index] <= 0x20 || authority[index] >= 0x7f {
			return false
		}
	}
	return true
}

func validSafePort(port string) bool {
	if port == "" {
		return true
	}
	if len(port) > 1 && port[0] == '0' {
		return false
	}
	for index := range port {
		if port[index] < '0' || port[index] > '9' {
			return false
		}
	}
	value, err := strconv.Atoi(port)
	return err == nil && value >= 1 && value <= 65535
}

func validDNSHostname(hostname string) bool {
	if len(hostname) == 0 || len(hostname) > 253 {
		return false
	}
	for _, label := range strings.Split(hostname, ".") {
		if !validDNSLabel(label) {
			return false
		}
	}
	return true
}

func validDNSLabel(label string) bool {
	if len(label) == 0 ||
		len(label) > 63 ||
		!isDNSAlphaNumeric(label[0]) ||
		!isDNSAlphaNumeric(label[len(label)-1]) {
		return false
	}
	for index := 1; index < len(label)-1; index++ {
		if !isDNSAlphaNumeric(label[index]) && label[index] != '-' {
			return false
		}
	}
	return true
}

func isDNSAlphaNumeric(character byte) bool {
	return character >= 'a' && character <= 'z' || character >= '0' && character <= '9'
}

func validProjectedEndpointHint(hint string) bool {
	const prefix = "https://"
	if len(hint) <= len(prefix) ||
		len(hint) > maxEndpointHintBytes ||
		!strings.HasPrefix(hint, prefix) ||
		!asciiEndpointAuthority(strings.TrimPrefix(hint, prefix)) {
		return false
	}
	authority := strings.TrimPrefix(hint, prefix)
	if strings.ContainsAny(authority, "/?#@") {
		return false
	}

	host := authority
	port := ""
	if strings.HasPrefix(authority, "[") {
		const ipv6Mask = "[****]"
		if !strings.HasPrefix(authority, ipv6Mask) {
			return false
		}
		suffix := strings.TrimPrefix(authority, ipv6Mask)
		if suffix != "" {
			if !strings.HasPrefix(suffix, ":") {
				return false
			}
			port = strings.TrimPrefix(suffix, ":")
			if port == "" {
				return false
			}
		}
		host = ipv6Mask
	} else {
		if strings.ContainsAny(authority, "[]") || strings.Count(authority, ":") > 1 {
			return false
		}
		if separator := strings.LastIndex(authority, ":"); separator >= 0 {
			host = authority[:separator]
			port = authority[separator+1:]
			if port == "" {
				return false
			}
		}
	}
	if !validSafePort(port) {
		return false
	}
	switch host {
	case "***", "***.***.***.***", "[****]":
		return true
	default:
		if !strings.HasPrefix(host, "***.") {
			return false
		}
		suffix := strings.TrimPrefix(host, "***.")
		return !strings.Contains(suffix, ".") && validDNSLabel(suffix)
	}
}

func validProjectionEnvelope(input SecretEnvelopeInput) bool {
	return input.Active != input.NeedsRewrap
}

func validCredentialFingerprint(value string) bool {
	if len(value) != 32 {
		return false
	}
	for index := range value {
		if !(value[index] >= '0' && value[index] <= '9') &&
			!(value[index] >= 'a' && value[index] <= 'f') {
			return false
		}
	}
	return true
}
