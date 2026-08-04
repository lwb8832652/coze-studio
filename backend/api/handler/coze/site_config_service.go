// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package coze

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"path"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	_ "golang.org/x/image/webp"

	"github.com/coze-dev/coze-studio/backend/api/model/admin/config"
	"github.com/coze-dev/coze-studio/backend/application/upload"
	bizConf "github.com/coze-dev/coze-studio/backend/bizpkg/config"
	"github.com/coze-dev/coze-studio/backend/pkg/logs"
)

const (
	defaultPublicSiteName        = "NewX AI"
	defaultPublicSiteDescription = "NewX AI 是面向个人与团队的智能工作空间，让任务、技能和协作沉淀为可复用的成果。"
	maxSiteLogoBytes             = 2 * 1024 * 1024
	maxSiteFaviconBytes          = 512 * 1024
)

type siteConfigBackend interface {
	GetBaseConfigWithRevision(context.Context) (*config.BasicConfiguration, string, error)
}

type siteAssetService struct {
	upload func(context.Context, []byte, string) (string, string, error)
	url    func(context.Context, string) (string, error)
	head   func(context.Context, string) error
}

func defaultSiteAssetService() siteAssetService {
	return siteAssetService{
		upload: func(ctx context.Context, data []byte, objectKey string) (string, string, error) {
			response, err := upload.SVC.UploadFile(ctx, data, objectKey)
			if err != nil {
				return "", "", err
			}
			if response == nil || response.Data == nil {
				return "", "", errors.New("site asset upload returned no data")
			}
			return response.Data.UploadURI, response.Data.UploadURL, nil
		},
		url:  upload.SVC.GetObjectURL,
		head: upload.SVC.HeadObject,
	}
}

type publicSiteConfigResponse struct {
	SiteName        string `json:"site_name"`
	SiteDescription string `json:"site_description"`
	SiteLogoURL     string `json:"site_logo_url"`
	FaviconURL      string `json:"favicon_url"`
	Revision        string `json:"revision"`
}

type siteAssetUploadResponse struct {
	URI      string `json:"uri"`
	URL      string `json:"url"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	MIMEType string `json:"mime_type"`
}

func GetPublicSiteConfig(ctx context.Context, c *app.RequestContext) {
	getPublicSiteConfig(ctx, c, bizConf.Base(), defaultSiteAssetService())
}

func getPublicSiteConfig(
	ctx context.Context,
	c *app.RequestContext,
	backend siteConfigBackend,
	assets siteAssetService,
) {
	configuration, revision, err := backend.GetBaseConfigWithRevision(ctx)
	if err != nil {
		logs.CtxErrorf(ctx, "[SiteConfig] read failed: %v", err)
		c.JSON(consts.StatusOK, publicSiteConfigResponse{
			SiteName:        defaultPublicSiteName,
			SiteDescription: defaultPublicSiteDescription,
		})
		return
	}

	response := projectPublicSiteConfig(configuration, revision)
	if configuration != nil && configuration.SiteLogoURI != nil && *configuration.SiteLogoURI != "" {
		response.SiteLogoURL = resolveSiteAssetURL(
			ctx,
			"logo",
			*configuration.SiteLogoURI,
			assets,
		)
	}
	if configuration != nil && configuration.FaviconURI != nil && *configuration.FaviconURI != "" {
		response.FaviconURL = resolveSiteAssetURL(
			ctx,
			"favicon",
			*configuration.FaviconURI,
			assets,
		)
	}
	c.JSON(consts.StatusOK, response)
}

func resolveSiteAssetURL(
	ctx context.Context,
	kind string,
	uri string,
	assets siteAssetService,
) string {
	if assets.head == nil || assets.url == nil {
		logs.CtxWarnf(ctx, "[SiteConfig] %s resolver unavailable", kind)
		return ""
	}
	if err := assets.head(ctx, uri); err != nil {
		logs.CtxWarnf(ctx, "[SiteConfig] %s object unavailable", kind)
		return ""
	}
	resolved, err := assets.url(ctx, uri)
	if err != nil {
		logs.CtxWarnf(ctx, "[SiteConfig] %s URL unavailable", kind)
		return ""
	}
	return resolved
}

func projectPublicSiteConfig(configuration *config.BasicConfiguration, revision string) publicSiteConfigResponse {
	response := publicSiteConfigResponse{
		SiteName:        defaultPublicSiteName,
		SiteDescription: defaultPublicSiteDescription,
		Revision:        revision,
	}
	if configuration == nil {
		return response
	}
	if value := strings.TrimSpace(configuration.GetSiteName()); value != "" {
		response.SiteName = value
	}
	if value := strings.TrimSpace(configuration.GetSiteDescription()); value != "" {
		response.SiteDescription = value
	}
	return response
}

func UploadSiteAsset(ctx context.Context, c *app.RequestContext) {
	uploadSiteAsset(ctx, c, defaultSiteAssetService())
}

func uploadSiteAsset(ctx context.Context, c *app.RequestContext, assets siteAssetService) {
	kind := strings.TrimSpace(c.PostForm("kind"))
	if kind != "logo" && kind != "favicon" {
		invalidParamRequestResponse(c, "kind must be logo or favicon")
		return
	}
	fileHeader, err := c.FormFile("file")
	if err != nil {
		invalidParamRequestResponse(c, "site asset file is required")
		return
	}
	file, err := fileHeader.Open()
	if err != nil {
		internalServerErrorResponse(ctx, c, err)
		return
	}
	defer file.Close()

	limit := int64(maxSiteLogoBytes)
	if kind == "favicon" {
		limit = maxSiteFaviconBytes
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		internalServerErrorResponse(ctx, c, err)
		return
	}
	if int64(len(data)) > limit {
		invalidParamRequestResponse(c, fmt.Sprintf("%s exceeds the size limit", kind))
		return
	}

	asset, err := inspectSiteAsset(kind, data)
	if err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}
	sum := sha256.Sum256(data)
	objectKey := path.Join("site-brand", kind, hex.EncodeToString(sum[:])+"."+asset.extension)
	uri, url, err := assets.upload(ctx, data, objectKey)
	if err != nil {
		internalServerErrorResponse(ctx, c, err)
		return
	}
	if strings.TrimSpace(uri) == "" || assets.head == nil {
		internalServerErrorResponse(
			ctx,
			c,
			errors.New("site asset persistence verification is unavailable"),
		)
		return
	}
	if err = assets.head(ctx, uri); err != nil {
		logs.CtxErrorf(ctx, "[SiteConfig] uploaded %s object unavailable", kind)
		internalServerErrorResponse(
			ctx,
			c,
			errors.New("site asset persistence verification failed"),
		)
		return
	}
	c.JSON(consts.StatusOK, siteAssetUploadResponse{
		URI:      uri,
		URL:      url,
		Width:    asset.width,
		Height:   asset.height,
		MIMEType: asset.mimeType,
	})
}

type inspectedSiteAsset struct {
	width     int
	height    int
	mimeType  string
	extension string
}

func inspectSiteAsset(kind string, data []byte) (inspectedSiteAsset, error) {
	if len(data) == 0 {
		return inspectedSiteAsset{}, errors.New("site asset is empty")
	}
	detected := http.DetectContentType(data)
	extension := map[string]string{
		"image/png":  "png",
		"image/jpeg": "jpg",
		"image/webp": "webp",
	}[detected]
	if extension == "" {
		return inspectedSiteAsset{}, errors.New("site asset must be PNG, JPEG, or WebP")
	}
	dimensions, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return inspectedSiteAsset{}, errors.New("site asset image data is invalid")
	}
	minDimension, maxDimension := 32, 2048
	if kind == "favicon" {
		minDimension, maxDimension = 16, 512
		if dimensions.Width != dimensions.Height {
			return inspectedSiteAsset{}, errors.New("favicon must be square")
		}
	}
	if dimensions.Width < minDimension || dimensions.Height < minDimension ||
		dimensions.Width > maxDimension || dimensions.Height > maxDimension {
		return inspectedSiteAsset{}, fmt.Errorf(
			"%s dimensions must be between %d and %d pixels",
			kind,
			minDimension,
			maxDimension,
		)
	}
	return inspectedSiteAsset{
		width:     dimensions.Width,
		height:    dimensions.Height,
		mimeType:  detected,
		extension: extension,
	}, nil
}
