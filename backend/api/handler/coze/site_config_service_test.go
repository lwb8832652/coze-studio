// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package coze

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"strings"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"

	"github.com/coze-dev/coze-studio/backend/api/model/admin/config"
)

func TestProjectPublicSiteConfigUsesSafeDefaults(t *testing.T) {
	response := projectPublicSiteConfig(nil, "revision-1")
	if response.SiteName != defaultPublicSiteName {
		t.Fatalf("expected default site name, got %q", response.SiteName)
	}
	if response.SiteDescription != defaultPublicSiteDescription {
		t.Fatalf("expected default site description, got %q", response.SiteDescription)
	}
	if response.Revision != "revision-1" {
		t.Fatalf("expected revision to be preserved, got %q", response.Revision)
	}
}

func TestProjectPublicSiteConfigReadsConfiguredTextOnly(t *testing.T) {
	name := "Acme AI"
	description := "Acme intelligent workspace"
	logoURI := "site-brand/logo/private.png"
	faviconURI := "site-brand/favicon/private.png"
	response := projectPublicSiteConfig(&config.BasicConfiguration{
		SiteName:        &name,
		SiteDescription: &description,
		SiteLogoURI:     &logoURI,
		FaviconURI:      &faviconURI,
		AdminEmails:     "admin@example.com",
		ServerHost:      "https://internal.example.com",
	}, "revision-2")

	if response.SiteName != name || response.SiteDescription != description {
		t.Fatalf("configured branding was not projected: %#v", response)
	}
	if response.SiteLogoURL != "" {
		t.Fatalf("storage URI must not be exposed by the text projection")
	}
	payload, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("marshal public response: %v", err)
	}
	for _, forbidden := range []string{
		"site_logo_uri",
		"favicon_uri",
		"private.png",
		"admin@example.com",
		"internal.example.com",
	} {
		if strings.Contains(string(payload), forbidden) {
			t.Fatalf("public response leaked %q: %s", forbidden, payload)
		}
	}
}

func TestInspectSiteAssetValidatesDimensions(t *testing.T) {
	validLogo := encodePNG(t, 128, 64)
	asset, err := inspectSiteAsset("logo", validLogo)
	if err != nil {
		t.Fatalf("expected valid logo: %v", err)
	}
	if asset.width != 128 || asset.height != 64 || asset.mimeType != "image/png" {
		t.Fatalf("unexpected inspected logo: %#v", asset)
	}

	if _, err = inspectSiteAsset("favicon", validLogo); err == nil {
		t.Fatal("expected non-square favicon to be rejected")
	}

	if _, err = inspectSiteAsset("logo", []byte("not an image")); err == nil {
		t.Fatal("expected invalid image bytes to be rejected")
	}
}

func TestInspectSiteAssetAcceptsTransparentLogoAndEnforcesBounds(t *testing.T) {
	transparentLogo := encodePNGColor(t, 32, 32, color.RGBA{R: 36, G: 144, B: 75, A: 0})
	if _, err := inspectSiteAsset("logo", transparentLogo); err != nil {
		t.Fatalf("transparent logo should be accepted: %v", err)
	}

	tests := []struct {
		name string
		kind string
		data []byte
	}{
		{name: "empty", kind: "logo", data: nil},
		{name: "unsupported mime", kind: "logo", data: []byte("plain text")},
		{name: "logo below minimum", kind: "logo", data: encodePNG(t, 31, 32)},
		{name: "logo above maximum", kind: "logo", data: encodePNG(t, 2049, 32)},
		{name: "favicon below minimum", kind: "favicon", data: encodePNG(t, 15, 15)},
		{name: "favicon above maximum", kind: "favicon", data: encodePNG(t, 513, 513)},
		{name: "favicon not square", kind: "favicon", data: encodePNG(t, 32, 33)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := inspectSiteAsset(tt.kind, tt.data); err == nil {
				t.Fatalf("inspectSiteAsset(%q) unexpectedly accepted invalid asset", tt.kind)
			}
		})
	}
	if _, err := inspectSiteAsset("favicon", encodePNG(t, 16, 16)); err != nil {
		t.Fatalf("minimum favicon should be accepted: %v", err)
	}
	if _, err := inspectSiteAsset("favicon", encodePNG(t, 512, 512)); err != nil {
		t.Fatalf("maximum favicon should be accepted: %v", err)
	}
}

func TestUploadSiteAssetUsesStableServerOwnedKeyAndRejectsOversizedFiles(t *testing.T) {
	data := encodePNG(t, 128, 64)
	var uploadedData []byte
	var uploadedKeys []string
	assets := siteAssetService{
		upload: func(_ context.Context, body []byte, objectKey string) (string, string, error) {
			uploadedData = append([]byte(nil), body...)
			uploadedKeys = append(uploadedKeys, objectKey)
			return "tos://" + objectKey, "https://assets.example.com/logo.png", nil
		},
	}

	for range 2 {
		requestContext := newSiteAssetUploadContext(t, "logo", "logo.png", data)
		uploadSiteAsset(context.Background(), requestContext, assets)
		if requestContext.Response.StatusCode() != 200 {
			t.Fatalf(
				"upload status = %d, body = %s",
				requestContext.Response.StatusCode(),
				requestContext.Response.Body(),
			)
		}
		var response siteAssetUploadResponse
		if err := json.Unmarshal(requestContext.Response.Body(), &response); err != nil {
			t.Fatalf("decode upload response: %v", err)
		}
		if response.URI == "" || response.URL == "" || response.MIMEType != "image/png" {
			t.Fatalf("unexpected upload response: %#v", response)
		}
	}
	if !bytes.Equal(uploadedData, data) {
		t.Fatal("uploaded bytes do not match inspected image")
	}
	if len(uploadedKeys) != 2 || uploadedKeys[0] != uploadedKeys[1] {
		t.Fatalf("content-addressed object key is not stable: %#v", uploadedKeys)
	}
	if !strings.HasPrefix(uploadedKeys[0], "site-brand/logo/") ||
		!strings.HasSuffix(uploadedKeys[0], ".png") {
		t.Fatalf("object key is outside the controlled logo prefix: %q", uploadedKeys[0])
	}

	oversizedContext := newSiteAssetUploadContext(
		t,
		"logo",
		"oversized.png",
		make([]byte, maxSiteLogoBytes+1),
	)
	uploadSiteAsset(context.Background(), oversizedContext, siteAssetService{
		upload: func(context.Context, []byte, string) (string, string, error) {
			t.Fatal("oversized asset must not reach storage")
			return "", "", nil
		},
	})
	if oversizedContext.Response.StatusCode() != 400 {
		t.Fatalf(
			"oversized upload status = %d, body = %s",
			oversizedContext.Response.StatusCode(),
			oversizedContext.Response.Body(),
		)
	}
}

func encodePNG(t *testing.T, width, height int) []byte {
	return encodePNGColor(t, width, height, color.RGBA{R: 36, G: 144, B: 75, A: 255})
}

func encodePNGColor(t *testing.T, width, height int, fill color.RGBA) []byte {
	t.Helper()
	canvas := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			canvas.Set(x, y, fill)
		}
	}
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, canvas); err != nil {
		t.Fatalf("encode test image: %v", err)
	}
	return buffer.Bytes()
}

func newSiteAssetUploadContext(
	t *testing.T,
	kind string,
	filename string,
	data []byte,
) *app.RequestContext {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("kind", kind); err != nil {
		t.Fatalf("write asset kind: %v", err)
	}
	filePart, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("create multipart file: %v", err)
	}
	if _, err = filePart.Write(data); err != nil {
		t.Fatalf("write multipart file: %v", err)
	}
	if err = writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	requestContext := app.NewContext(0)
	requestContext.Request.Header.SetContentTypeBytes(
		[]byte(writer.FormDataContentType()),
	)
	requestContext.Request.SetBody(body.Bytes())
	return requestContext
}
