// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package appdev

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
)

func validArtifactGrantSpecForTest(t *testing.T) ArtifactGrantSpec {
	t.Helper()
	digest := sha256.Sum256([]byte("artifact payload"))
	return ArtifactGrantSpec{
		Audience: ArtifactGrantAudience{
			SpaceID: "1001", ProjectID: "project-a", ProviderKey: "provider-a",
			ProviderScope: domainsandbox.ScopeAppDev, Operation: "snapshot.download",
		},
		Direction: ArtifactGrantDirectionDownload,
		ObjectKey: "appdev/1001/project-a/snapshot.zip",
		Digest:    ArtifactGrantDigest(digest),
		Size:      int64(len("artifact payload")),
		MaxSize:   1024,
	}
}

func TestArtifactGrantSpecRequiresExactBoundedAudience(t *testing.T) {
	valid := validArtifactGrantSpecForTest(t)
	if _, err := NormalizeArtifactGrantSpec(valid); err != nil {
		t.Fatalf("valid spec rejected: %v", err)
	}
	for _, test := range []struct {
		name   string
		mutate func(*ArtifactGrantSpec)
	}{
		{name: "space", mutate: func(spec *ArtifactGrantSpec) { spec.Audience.SpaceID = "../1001" }},
		{name: "project", mutate: func(spec *ArtifactGrantSpec) { spec.Audience.ProjectID = "project/a" }},
		{name: "provider", mutate: func(spec *ArtifactGrantSpec) { spec.Audience.ProviderKey = "" }},
		{name: "scope", mutate: func(spec *ArtifactGrantSpec) { spec.Audience.ProviderScope = domainsandbox.ScopeAgent }},
		{name: "operation", mutate: func(spec *ArtifactGrantSpec) { spec.Audience.Operation = "../download" }},
		{name: "direction", mutate: func(spec *ArtifactGrantSpec) { spec.Direction = ArtifactGrantDirection("copy") }},
		{name: "object traversal", mutate: func(spec *ArtifactGrantSpec) { spec.ObjectKey = "appdev/../secret" }},
		{name: "empty digest", mutate: func(spec *ArtifactGrantSpec) { spec.Digest = ArtifactGrantDigest{} }},
		{name: "negative size", mutate: func(spec *ArtifactGrantSpec) { spec.Size = -1 }},
		{name: "size over grant", mutate: func(spec *ArtifactGrantSpec) { spec.Size = spec.MaxSize + 1 }},
		{name: "global maximum", mutate: func(spec *ArtifactGrantSpec) { spec.MaxSize = MaxArtifactGrantObjectBytes + 1 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			candidate := valid
			test.mutate(&candidate)
			if _, err := NormalizeArtifactGrantSpec(candidate); !errorsIsArtifactGrantInvalid(err) {
				t.Fatalf("invalid spec accepted: %#v, %v", candidate, err)
			}
		})
	}
	for _, ttl := range []time.Duration{0, -time.Second, time.Nanosecond, MaxArtifactGrantTTL + time.Nanosecond} {
		if err := ValidateArtifactGrantTTL(ttl); !errorsIsArtifactGrantInvalid(err) {
			t.Fatalf("invalid TTL %v accepted: %v", ttl, err)
		}
	}
	for _, ttl := range []time.Duration{time.Microsecond, MaxArtifactGrantTTL} {
		if err := ValidateArtifactGrantTTL(ttl); err != nil {
			t.Fatalf("valid TTL %v rejected: %v", ttl, err)
		}
	}
}

func TestArtifactGrantTokenAndInternalClaimsNeverFormatOrJSONSecrets(t *testing.T) {
	raw := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0x7b}, ArtifactGrantTokenBytes))
	token, err := ParseArtifactGrantToken(raw)
	if err != nil {
		t.Fatal(err)
	}
	record := ArtifactGrantRecord{
		Spec: validArtifactGrantSpecForTest(t), State: ArtifactGrantStateIssued,
		IssuedAt: time.Now().UTC(), ExpiresAt: time.Now().UTC().Add(time.Minute),
	}
	for _, value := range []any{token, token.Hash(), record.Spec, record} {
		for _, formatted := range []string{
			fmt.Sprintf("%v", value), fmt.Sprintf("%+v", value), fmt.Sprintf("%#v", value),
		} {
			if strings.Contains(formatted, raw) || strings.Contains(formatted, record.Spec.ObjectKey) || !strings.Contains(formatted, "REDACTED") {
				t.Fatalf("secret/internal value leaked through formatting: %q", formatted)
			}
		}
		encoded, marshalErr := json.Marshal(value)
		if marshalErr == nil || len(encoded) != 0 {
			t.Fatalf("generic JSON unexpectedly succeeded: %q, %v", encoded, marshalErr)
		}
	}
	if token.Bearer() != raw {
		t.Fatal("explicit bearer access did not return the one-time credential")
	}
}

func TestArtifactGrantDigestRequiresCanonicalSHA256(t *testing.T) {
	want := sha256.Sum256([]byte("payload"))
	parsed, err := ParseArtifactGrantDigest("sha256:" + fmt.Sprintf("%x", want[:]))
	if err != nil || parsed != ArtifactGrantDigest(want) {
		t.Fatalf("ParseArtifactGrantDigest() = %v, %v", parsed, err)
	}
	for _, invalid := range []string{"", "sha1:abcd", "sha256:abcd", "SHA256:" + fmt.Sprintf("%x", want[:])} {
		if _, err := ParseArtifactGrantDigest(invalid); !errorsIsArtifactGrantInvalid(err) {
			t.Fatalf("invalid digest %q accepted: %v", invalid, err)
		}
	}
}

func TestArtifactGrantIDIsRandomlySeparateAndRedacted(t *testing.T) {
	grantID, err := NewRandomArtifactGrantID(bytes.NewReader(bytes.Repeat([]byte{0x23}, ArtifactGrantIDBytes)))
	if err != nil {
		t.Fatal(err)
	}
	token, err := NewRandomArtifactGrantToken(bytes.NewReader(bytes.Repeat([]byte{0x24}, ArtifactGrantTokenBytes)))
	if err != nil {
		t.Fatal(err)
	}
	if grantID.Encoded() == token.Bearer() {
		t.Fatal("grant identity reused bearer token material")
	}
	for _, formatted := range []string{fmt.Sprintf("%v", grantID), fmt.Sprintf("%+v", grantID), fmt.Sprintf("%#v", grantID)} {
		if strings.Contains(formatted, grantID.Encoded()) || !strings.Contains(formatted, "REDACTED") {
			t.Fatalf("grant identity leaked: %q", formatted)
		}
	}
	if encoded, err := json.Marshal(grantID); err == nil || len(encoded) != 0 {
		t.Fatalf("grant identity JSON = %q, %v", encoded, err)
	}
}

func errorsIsArtifactGrantInvalid(err error) bool {
	return err != nil && strings.Contains(err.Error(), ErrArtifactGrantInvalid.Error())
}
