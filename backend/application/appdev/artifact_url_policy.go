// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package appdev

import (
	"errors"
	"net"
	"net/url"
	"strings"
)

var ErrArtifactCapabilityURLInvalid = errors.New("appdev artifact capability URL is invalid")

type ArtifactCapabilityURLPolicy interface {
	Validate(string) error
}

type artifactCapabilityURLPolicy struct {
	allowLoopbackHTTP bool
}

func NewArtifactCapabilityURLPolicy(allowLoopbackHTTP bool) ArtifactCapabilityURLPolicy {
	return artifactCapabilityURLPolicy{allowLoopbackHTTP: allowLoopbackHTTP}
}

func (policy artifactCapabilityURLPolicy) Validate(value string) error {
	if value == "" || value != strings.TrimSpace(value) {
		return ErrArtifactCapabilityURLInvalid
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.Opaque != "" ||
		parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Path == "" {
		return ErrArtifactCapabilityURLInvalid
	}
	if parsed.Scheme == "https" {
		return nil
	}
	ip := net.ParseIP(parsed.Hostname())
	if policy.allowLoopbackHTTP && parsed.Scheme == "http" && ip != nil && ip.IsLoopback() {
		return nil
	}
	return ErrArtifactCapabilityURLInvalid
}
