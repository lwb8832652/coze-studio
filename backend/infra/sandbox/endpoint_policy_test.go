// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"errors"
	"testing"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
)

func TestEndpointPolicyRequiresRootHTTPSOriginAndExactHost(t *testing.T) {
	t.Parallel()

	config := validRemoteProviderConfig()
	policy, err := newEndpointPolicy(config)
	if err != nil {
		t.Fatalf("new endpoint policy: %v", err)
	}
	if got := policy.urlForPath("/v1/health").String(); got != "https://sandbox.example.test/v1/health" {
		t.Fatalf("health URL = %q", got)
	}

	for _, endpoint := range []string{
		"http://sandbox.example.test",
		"https://sandbox.example.test/base",
		"https://sandbox.example.test/?query=1",
		"https://sandbox.example.test/#fragment",
		"https://user:pass@sandbox.example.test/",
		"https://BÜCHER.example/",
		"https://sandbox.example.test:/",
		"https://[sandbox.example.test]/",
		" https://sandbox.example.test/",
	} {
		invalid := config
		invalid.Endpoint = endpoint
		if _, err := newEndpointPolicy(invalid); !errors.Is(err, domainsandbox.ErrInvalidInput) {
			t.Fatalf("endpoint %q must fail closed, got %v", endpoint, err)
		}
	}

	hostMismatch := config
	hostMismatch.AllowedHosts = []string{"other.example.test"}
	if _, err := newEndpointPolicy(hostMismatch); !errors.Is(err, domainsandbox.ErrInvalidInput) {
		t.Fatalf("host mismatch must fail closed, got %v", err)
	}

	badCIDR := config
	badCIDR.AllowedPrivateCIDRs = []string{"10.0.0.1/8"}
	if _, err := newEndpointPolicy(badCIDR); !errors.Is(err, domainsandbox.ErrInvalidInput) {
		t.Fatalf("non-canonical CIDR must fail closed, got %v", err)
	}
}
