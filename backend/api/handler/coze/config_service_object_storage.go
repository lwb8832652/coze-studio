/*
 * Copyright 2025 coze-dev Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package coze

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/cloudwego/hertz/pkg/app"

	adminconfig "github.com/coze-dev/coze-studio/backend/api/model/admin/config"
	appobjectstorage "github.com/coze-dev/coze-studio/backend/application/objectstorage"
	domain "github.com/coze-dev/coze-studio/backend/domain/storageconfig"
	"github.com/coze-dev/coze-studio/backend/pkg/logs"
)

const maxObjectStorageBodyBytes = 128 * 1024

var errObjectStorageBodyTooLarge = errors.New("object storage request body too large")

type objectStorageAdminService interface {
	List(context.Context) (*appobjectstorage.ListResult, error)
	Create(context.Context, appobjectstorage.CreateRequest) (*appobjectstorage.ConfigView, error)
	Update(context.Context, appobjectstorage.UpdateRequest) (*appobjectstorage.ConfigView, error)
	Test(context.Context, appobjectstorage.TestRequest) (*appobjectstorage.TestResult, error)
	Activate(context.Context, appobjectstorage.ActivateRequest) (*appobjectstorage.ConfigView, error)
	Delete(context.Context, appobjectstorage.DeleteRequest) error
}

type applicationObjectStorageAdminService struct{}

func (applicationObjectStorageAdminService) current() (*appobjectstorage.Service, error) {
	if appobjectstorage.SVC == nil {
		return nil, domain.ErrCredentialUnavailable
	}
	return appobjectstorage.SVC, nil
}

func (a applicationObjectStorageAdminService) List(ctx context.Context) (*appobjectstorage.ListResult, error) {
	service, err := a.current()
	if err != nil {
		return nil, err
	}
	return service.List(ctx)
}

func (a applicationObjectStorageAdminService) Create(ctx context.Context, request appobjectstorage.CreateRequest) (*appobjectstorage.ConfigView, error) {
	service, err := a.current()
	if err != nil {
		return nil, err
	}
	return service.Create(ctx, request)
}

func (a applicationObjectStorageAdminService) Update(ctx context.Context, request appobjectstorage.UpdateRequest) (*appobjectstorage.ConfigView, error) {
	service, err := a.current()
	if err != nil {
		return nil, err
	}
	return service.Update(ctx, request)
}

func (a applicationObjectStorageAdminService) Test(ctx context.Context, request appobjectstorage.TestRequest) (*appobjectstorage.TestResult, error) {
	service, err := a.current()
	if err != nil {
		return nil, err
	}
	return service.Test(ctx, request)
}

func (a applicationObjectStorageAdminService) Activate(ctx context.Context, request appobjectstorage.ActivateRequest) (*appobjectstorage.ConfigView, error) {
	service, err := a.current()
	if err != nil {
		return nil, err
	}
	return service.Activate(ctx, request)
}

func (a applicationObjectStorageAdminService) Delete(ctx context.Context, request appobjectstorage.DeleteRequest) error {
	service, err := a.current()
	if err != nil {
		return err
	}
	return service.Delete(ctx, request)
}

type objectStorageAdminHandler struct {
	service objectStorageAdminService
}

func newObjectStorageAdminHandler(service objectStorageAdminService) *objectStorageAdminHandler {
	if service == nil {
		service = applicationObjectStorageAdminService{}
	}
	return &objectStorageAdminHandler{service: service}
}

func defaultObjectStorageAdminHandler() *objectStorageAdminHandler {
	return newObjectStorageAdminHandler(applicationObjectStorageAdminService{})
}

func (h *objectStorageAdminHandler) list(ctx context.Context, c *app.RequestContext) {
	result, err := h.service.List(ctx)
	if err != nil {
		objectStorageError(ctx, c, err)
		return
	}
	c.JSON(http.StatusOK, &adminconfig.ListObjectStorageConfigsResp{
		Configs:         objectStorageConfigViewsToAPI(result.Configs),
		RuntimeSource:   objectStorageRuntimeSourceToAPI(result.RuntimeSource),
		RestartRequired: result.RestartRequired,
		Code:            0,
		Msg:             "success",
	})
}

func (h *objectStorageAdminHandler) create(ctx context.Context, c *app.RequestContext) {
	var request adminconfig.CreateObjectStorageConfigReq
	if err := decodeObjectStorageJSON(c, &request); err != nil {
		objectStorageError(ctx, c, err)
		return
	}
	provider, err := objectStorageProviderFromAPI(request.ProviderType)
	if err != nil {
		objectStorageError(ctx, c, err)
		return
	}
	if request.Config == nil || request.Credential == nil {
		objectStorageError(ctx, c, domain.ErrConfigInvalid)
		return
	}
	result, err := h.service.Create(ctx, appobjectstorage.CreateRequest{
		Name:         request.Name,
		ProviderType: provider,
		PublicConfig: objectStoragePublicConfigFromAPI(request.Config),
		Credential:   objectStorageCredentialFromAPI(request.Credential),
	})
	if err != nil {
		objectStorageError(ctx, c, err)
		return
	}
	c.JSON(http.StatusOK, &adminconfig.CreateObjectStorageConfigResp{
		Config: objectStorageConfigViewToAPI(result),
		Code:   0,
		Msg:    "success",
	})
}

func (h *objectStorageAdminHandler) update(ctx context.Context, c *app.RequestContext) {
	var request adminconfig.UpdateObjectStorageConfigReq
	if err := decodeObjectStorageJSON(c, &request); err != nil {
		objectStorageError(ctx, c, err)
		return
	}
	if request.Config == nil {
		objectStorageError(ctx, c, domain.ErrConfigInvalid)
		return
	}
	id, expectedVersion, err := objectStorageIDAndVersion(request.ID, request.ExpectedVersion)
	if err != nil {
		objectStorageError(ctx, c, err)
		return
	}
	result, err := h.service.Update(ctx, appobjectstorage.UpdateRequest{
		ID:              id,
		ExpectedVersion: expectedVersion,
		Name:            request.Name,
		PublicConfig:    objectStoragePublicConfigFromAPI(request.Config),
		Credential:      objectStorageCredentialFromAPI(request.Credential),
	})
	if err != nil {
		objectStorageError(ctx, c, err)
		return
	}
	c.JSON(http.StatusOK, &adminconfig.UpdateObjectStorageConfigResp{
		Config: objectStorageConfigViewToAPI(result),
		Code:   0,
		Msg:    "success",
	})
}

func (h *objectStorageAdminHandler) test(ctx context.Context, c *app.RequestContext) {
	var request adminconfig.TestObjectStorageConfigReq
	if err := decodeObjectStorageJSON(c, &request); err != nil {
		objectStorageError(ctx, c, err)
		return
	}
	provider, err := objectStorageProviderFromAPI(request.ProviderType)
	if err != nil {
		objectStorageError(ctx, c, err)
		return
	}
	if request.Config == nil {
		objectStorageError(ctx, c, domain.ErrConfigInvalid)
		return
	}
	id, expectedVersion, err := objectStorageOptionalIDAndVersion(request.ID, request.ExpectedVersion)
	if err != nil {
		objectStorageError(ctx, c, err)
		return
	}
	result, err := h.service.Test(ctx, appobjectstorage.TestRequest{
		ID:              id,
		ExpectedVersion: expectedVersion,
		ProviderType:    provider,
		PublicConfig:    objectStoragePublicConfigFromAPI(request.Config),
		Credential:      objectStorageCredentialFromAPI(request.Credential),
	})
	if err != nil {
		objectStorageError(ctx, c, err)
		return
	}
	c.JSON(http.StatusOK, &adminconfig.TestObjectStorageConfigResp{
		Success: result.Success,
		Health:  objectStorageHealthToAPI(result.Health),
		Code:    0,
		Msg:     "success",
	})
}

func (h *objectStorageAdminHandler) activate(ctx context.Context, c *app.RequestContext) {
	var request adminconfig.ActivateObjectStorageConfigReq
	if err := decodeObjectStorageJSON(c, &request); err != nil {
		objectStorageError(ctx, c, err)
		return
	}
	id, expectedVersion, err := objectStorageIDAndVersion(request.ID, request.ExpectedVersion)
	if err != nil {
		objectStorageError(ctx, c, err)
		return
	}
	result, err := h.service.Activate(ctx, appobjectstorage.ActivateRequest{
		ID:                 id,
		ExpectedVersion:    expectedVersion,
		MigrationConfirmed: request.MigrationConfirmed,
	})
	if err != nil {
		objectStorageError(ctx, c, err)
		return
	}
	c.JSON(http.StatusOK, &adminconfig.ActivateObjectStorageConfigResp{
		Config: objectStorageConfigViewToAPI(result),
		Code:   0,
		Msg:    "success",
	})
}

func (h *objectStorageAdminHandler) delete(ctx context.Context, c *app.RequestContext) {
	var request adminconfig.DeleteObjectStorageConfigReq
	if err := decodeObjectStorageJSON(c, &request); err != nil {
		objectStorageError(ctx, c, err)
		return
	}
	id, expectedVersion, err := objectStorageIDAndVersion(request.ID, request.ExpectedVersion)
	if err != nil {
		objectStorageError(ctx, c, err)
		return
	}
	if err := h.service.Delete(ctx, appobjectstorage.DeleteRequest{ID: id, ExpectedVersion: expectedVersion}); err != nil {
		objectStorageError(ctx, c, err)
		return
	}
	c.JSON(http.StatusOK, &adminconfig.DeleteObjectStorageConfigResp{Code: 0, Msg: "success"})
}

func objectStorageIDAndVersion(rawID, rawVersion int64) (uint64, uint64, error) {
	if rawID <= 0 || rawVersion <= 0 {
		return 0, 0, domain.ErrConfigInvalid
	}
	return uint64(rawID), uint64(rawVersion), nil
}

func objectStorageOptionalIDAndVersion(rawID, rawVersion *int64) (uint64, uint64, error) {
	var id uint64
	var version uint64
	if rawID != nil {
		if *rawID <= 0 {
			return 0, 0, domain.ErrConfigInvalid
		}
		id = uint64(*rawID)
	}
	if rawVersion != nil {
		if *rawVersion <= 0 {
			return 0, 0, domain.ErrConfigInvalid
		}
		version = uint64(*rawVersion)
	}
	return id, version, nil
}

func objectStorageProviderFromAPI(provider adminconfig.ObjectStorageProviderType) (domain.ProviderType, error) {
	switch provider {
	case adminconfig.ObjectStorageProviderType_QINIU:
		return domain.ProviderQiniu, nil
	case adminconfig.ObjectStorageProviderType_ALIYUN_OSS:
		return domain.ProviderAliyunOSS, nil
	case adminconfig.ObjectStorageProviderType_TENCENT_COS:
		return domain.ProviderTencentCOS, nil
	case adminconfig.ObjectStorageProviderType_HUAWEI_OBS:
		return domain.ProviderHuaweiOBS, nil
	case adminconfig.ObjectStorageProviderType_AWS_S3:
		return domain.ProviderAWSS3, nil
	case adminconfig.ObjectStorageProviderType_MINIO:
		return domain.ProviderMinIO, nil
	case adminconfig.ObjectStorageProviderType_TOS:
		return domain.ProviderTOS, nil
	default:
		return "", domain.ErrProviderUnsupported
	}
}

func objectStorageProviderToAPI(provider domain.ProviderType) adminconfig.ObjectStorageProviderType {
	switch provider {
	case domain.ProviderQiniu:
		return adminconfig.ObjectStorageProviderType_QINIU
	case domain.ProviderAliyunOSS:
		return adminconfig.ObjectStorageProviderType_ALIYUN_OSS
	case domain.ProviderTencentCOS:
		return adminconfig.ObjectStorageProviderType_TENCENT_COS
	case domain.ProviderHuaweiOBS:
		return adminconfig.ObjectStorageProviderType_HUAWEI_OBS
	case domain.ProviderAWSS3:
		return adminconfig.ObjectStorageProviderType_AWS_S3
	case domain.ProviderMinIO:
		return adminconfig.ObjectStorageProviderType_MINIO
	case domain.ProviderTOS:
		return adminconfig.ObjectStorageProviderType_TOS
	default:
		return 0
	}
}

func objectStorageRuntimeSourceToAPI(source domain.RuntimeSource) adminconfig.ObjectStorageRuntimeSource {
	if source == domain.RuntimeSourceEnvRescue {
		return adminconfig.ObjectStorageRuntimeSource_ENV_RESCUE
	}
	return adminconfig.ObjectStorageRuntimeSource_DATABASE
}

func objectStorageHealthStatusToAPI(status domain.HealthStatus) adminconfig.ObjectStorageHealthStatus {
	switch status {
	case domain.HealthHealthy:
		return adminconfig.ObjectStorageHealthStatus_HEALTHY
	case domain.HealthUnhealthy:
		return adminconfig.ObjectStorageHealthStatus_UNHEALTHY
	default:
		return adminconfig.ObjectStorageHealthStatus_UNKNOWN
	}
}

func objectStoragePublicConfigFromAPI(input *adminconfig.ObjectStoragePublicConfig) domain.PublicConfig {
	if input == nil {
		return domain.PublicConfig{}
	}
	return domain.PublicConfig{
		Bucket:           input.GetBucket(),
		Region:           input.GetRegion(),
		Endpoint:         input.GetEndpoint(),
		EndpointOverride: input.GetEndpointOverride(),
		ForcePathStyle:   input.GetForcePathStyle(),
		UseSSL:           input.GetUseSsl(),
		DownloadDomain:   input.GetDownloadDomain(),
		UseHTTPS:         input.GetUseHTTPS(),
	}
}

func objectStoragePublicConfigToAPI(input domain.PublicConfig) *adminconfig.ObjectStoragePublicConfig {
	output := &adminconfig.ObjectStoragePublicConfig{
		ForcePathStyle: boolPtr(input.ForcePathStyle),
		UseSsl:         boolPtr(input.UseSSL),
		UseHTTPS:       boolPtr(input.UseHTTPS),
	}
	if input.Bucket != "" {
		output.Bucket = stringPtr(input.Bucket)
	}
	if input.Region != "" {
		output.Region = stringPtr(input.Region)
	}
	if input.Endpoint != "" {
		output.Endpoint = stringPtr(input.Endpoint)
	}
	if input.EndpointOverride != "" {
		output.EndpointOverride = stringPtr(input.EndpointOverride)
	}
	if input.DownloadDomain != "" {
		output.DownloadDomain = stringPtr(input.DownloadDomain)
	}
	return output
}

func objectStorageCredentialFromAPI(input *adminconfig.ObjectStorageCredentialInput) domain.CredentialInput {
	if input == nil {
		return domain.CredentialInput{}
	}
	return domain.CredentialInput{
		AccessKeyID:     input.GetAccessKeyID(),
		SecretAccessKey: input.GetSecretAccessKey(),
	}
}

func objectStorageConfigViewsToAPI(input []appobjectstorage.ConfigView) []*adminconfig.ObjectStorageConfigView {
	output := make([]*adminconfig.ObjectStorageConfigView, 0, len(input))
	for index := range input {
		output = append(output, objectStorageConfigViewToAPI(&input[index]))
	}
	return output
}

func objectStorageConfigViewToAPI(input *appobjectstorage.ConfigView) *adminconfig.ObjectStorageConfigView {
	if input == nil {
		return nil
	}
	return &adminconfig.ObjectStorageConfigView{
		ID:                   int64(input.ID),
		Name:                 input.Name,
		ProviderType:         objectStorageProviderToAPI(input.ProviderType),
		Config:               objectStoragePublicConfigToAPI(input.PublicConfig),
		CredentialConfigured: input.CredentialConfigured,
		Health:               objectStorageHealthToAPI(input.Health),
		DesiredActive:        input.DesiredActive,
		RuntimeActive:        input.RuntimeActive,
		RestartRequired:      input.RestartRequired,
		Version:              int64(input.Version),
		RuntimeRevision:      int64(input.RuntimeRevision),
		CreatedAt:            input.CreatedAt,
		UpdatedAt:            input.UpdatedAt,
	}
}

func objectStorageHealthToAPI(input domain.Health) *adminconfig.ObjectStorageHealthView {
	output := &adminconfig.ObjectStorageHealthView{
		Status: objectStorageHealthStatusToAPI(input.Status),
	}
	if input.Code != "" {
		output.Code = stringPtr(input.Code)
	}
	if input.Message != "" {
		output.Message = stringPtr(input.Message)
	}
	if input.LatencyMS != 0 {
		latency := int64(input.LatencyMS)
		output.LatencyMs = &latency
	}
	if input.CheckedAt != nil && !input.CheckedAt.IsZero() {
		output.CheckedAt = stringPtr(input.CheckedAt.UTC().Format(timeFormatRFC3339Nano))
	}
	return output
}

func decodeObjectStorageJSON(c *app.RequestContext, target any) error {
	contentType, _, err := mime.ParseMediaType(string(c.Request.Header.ContentType()))
	if err != nil || contentType != "application/json" {
		return domain.ErrConfigInvalid
	}
	if contentLength := c.Request.Header.ContentLength(); contentLength > maxObjectStorageBodyBytes {
		return errObjectStorageBodyTooLarge
	}
	var body []byte
	if c.Request.IsBodyStream() {
		body, err = io.ReadAll(io.LimitReader(c.Request.BodyStream(), maxObjectStorageBodyBytes+1))
		_ = c.Request.CloseBodyStream()
	} else {
		body = c.Request.Body()
	}
	if err != nil || len(body) == 0 {
		return domain.ErrConfigInvalid
	}
	if len(body) > maxObjectStorageBodyBytes {
		return errObjectStorageBodyTooLarge
	}
	if !utf8.Valid(body) {
		return domain.ErrConfigInvalid
	}
	if err := validateObjectStorageJSON(body, reflect.TypeOf(target)); err != nil {
		return domain.ErrConfigInvalid
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return domain.ErrConfigInvalid
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return domain.ErrConfigInvalid
	}
	return nil
}

type objectStorageJSONField struct {
	typ    reflect.Type
	quoted bool
}

func validateObjectStorageJSON(body []byte, targetType reflect.Type) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	var raw json.RawMessage
	if err := decoder.Decode(&raw); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return domain.ErrConfigInvalid
	}
	return validateObjectStorageJSONValue(raw, targetType, false)
}

func validateObjectStorageJSONValue(raw json.RawMessage, targetType reflect.Type, quoted bool) error {
	for targetType.Kind() == reflect.Pointer {
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return nil
		}
		targetType = targetType.Elem()
	}
	trimmed := bytes.TrimSpace(raw)
	if bytes.Equal(trimmed, []byte("null")) {
		if targetType.Kind() == reflect.Slice || targetType.Kind() == reflect.Map || targetType.Kind() == reflect.Interface {
			return nil
		}
		return domain.ErrConfigInvalid
	}
	if quoted {
		return validateObjectStorageJSONStringEncodedValue(trimmed, targetType)
	}
	switch targetType.Kind() {
	case reflect.Struct:
		return validateObjectStorageJSONObject(trimmed, targetType)
	case reflect.Slice, reflect.Array:
		var values []json.RawMessage
		if err := json.Unmarshal(trimmed, &values); err != nil {
			return err
		}
		for _, value := range values {
			if err := validateObjectStorageJSONValue(value, targetType.Elem(), false); err != nil {
				return err
			}
		}
		return nil
	case reflect.Map:
		if targetType.Key().Kind() != reflect.String {
			return domain.ErrConfigInvalid
		}
		return validateObjectStorageJSONMap(trimmed, targetType.Elem())
	default:
		value := reflect.New(targetType).Interface()
		return json.Unmarshal(trimmed, value)
	}
}

func validateObjectStorageJSONStringEncodedValue(raw []byte, targetType reflect.Type) error {
	var encoded string
	if err := json.Unmarshal(raw, &encoded); err != nil {
		return err
	}
	switch targetType.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		_, err := strconv.ParseInt(encoded, 10, targetType.Bits())
		return err
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		_, err := strconv.ParseUint(encoded, 10, targetType.Bits())
		return err
	default:
		value := reflect.New(targetType).Interface()
		return json.Unmarshal([]byte(encoded), value)
	}
}

func validateObjectStorageJSONObject(raw []byte, targetType reflect.Type) error {
	fields := make(map[string]objectStorageJSONField)
	for index := 0; index < targetType.NumField(); index++ {
		field := targetType.Field(index)
		if field.PkgPath != "" {
			continue
		}
		name := field.Name
		quoted := false
		tag := strings.Split(field.Tag.Get("json"), ",")
		if tag[0] != "" {
			if tag[0] == "-" {
				continue
			}
			name = tag[0]
		}
		for _, option := range tag[1:] {
			if option == "string" {
				quoted = true
			}
		}
		fields[name] = objectStorageJSONField{typ: field.Type, quoted: quoted}
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	opening, err := decoder.Token()
	if err != nil || opening != json.Delim('{') {
		return domain.ErrConfigInvalid
	}
	seen := make(map[string]struct{})
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		name, ok := token.(string)
		field, known := fields[name]
		if !ok || !known {
			return domain.ErrConfigInvalid
		}
		if _, duplicate := seen[name]; duplicate {
			return domain.ErrConfigInvalid
		}
		seen[name] = struct{}{}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return err
		}
		if err := validateObjectStorageJSONValue(value, field.typ, field.quoted); err != nil {
			return err
		}
	}
	closing, err := decoder.Token()
	if err != nil || closing != json.Delim('}') {
		return domain.ErrConfigInvalid
	}
	return nil
}

func validateObjectStorageJSONMap(raw []byte, valueType reflect.Type) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	opening, err := decoder.Token()
	if err != nil || opening != json.Delim('{') {
		return domain.ErrConfigInvalid
	}
	seen := make(map[string]struct{})
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		name, ok := token.(string)
		if !ok {
			return domain.ErrConfigInvalid
		}
		if _, duplicate := seen[name]; duplicate {
			return domain.ErrConfigInvalid
		}
		seen[name] = struct{}{}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return err
		}
		if err := validateObjectStorageJSONValue(value, valueType, false); err != nil {
			return err
		}
	}
	_, err = decoder.Token()
	return err
}

func objectStorageError(ctx context.Context, c *app.RequestContext, err error) {
	status, code, message := objectStorageErrorContract(err)
	if status == http.StatusInternalServerError {
		logs.CtxErrorf(ctx, "[ObjectStorageConfig] request failed: %s", code)
	}
	c.AbortWithStatusJSON(status, map[string]any{
		"code":       status,
		"error_code": code,
		"msg":        message,
	})
}

func objectStorageErrorContract(err error) (int, string, string) {
	switch {
	case errors.Is(err, errObjectStorageBodyTooLarge):
		return http.StatusRequestEntityTooLarge, "OBJECT_STORAGE_REQUEST_TOO_LARGE", "object storage request body is too large"
	case errors.Is(err, domain.ErrConfigInvalid):
		return http.StatusBadRequest, domain.ErrorCodeOf(domain.ErrConfigInvalid), "object storage config is invalid"
	case errors.Is(err, domain.ErrProviderUnsupported):
		return http.StatusBadRequest, domain.ErrorCodeOf(domain.ErrProviderUnsupported), "object storage provider is unsupported"
	case errors.Is(err, domain.ErrNotFound):
		return http.StatusNotFound, domain.ErrorCodeOf(domain.ErrNotFound), "object storage config was not found"
	case errors.Is(err, domain.ErrVersionConflict):
		return http.StatusConflict, domain.ErrorCodeOf(domain.ErrVersionConflict), "object storage config version is stale"
	case errors.Is(err, domain.ErrConnectionFailed):
		return http.StatusBadRequest, domain.ErrorCodeOf(domain.ErrConnectionFailed), "object storage connection failed"
	case errors.Is(err, domain.ErrActiveDeleteForbidden):
		return http.StatusConflict, domain.ErrorCodeOf(domain.ErrActiveDeleteForbidden), "active object storage config cannot be deleted"
	case errors.Is(err, domain.ErrCredentialUnavailable):
		return http.StatusServiceUnavailable, domain.ErrorCodeOf(domain.ErrCredentialUnavailable), "object storage credential is unavailable"
	case errors.Is(err, domain.ErrMigrationConfirmation):
		return http.StatusConflict, domain.ErrorCodeOf(domain.ErrMigrationConfirmation), "object storage migration confirmation is required"
	default:
		return http.StatusInternalServerError, "OBJECT_STORAGE_INTERNAL", "internal server error"
	}
}

func ListObjectStorageConfigs(ctx context.Context, c *app.RequestContext) {
	defaultObjectStorageAdminHandler().list(ctx, c)
}

func CreateObjectStorageConfig(ctx context.Context, c *app.RequestContext) {
	defaultObjectStorageAdminHandler().create(ctx, c)
}

func UpdateObjectStorageConfig(ctx context.Context, c *app.RequestContext) {
	defaultObjectStorageAdminHandler().update(ctx, c)
}

func TestObjectStorageConfig(ctx context.Context, c *app.RequestContext) {
	defaultObjectStorageAdminHandler().test(ctx, c)
}

func ActivateObjectStorageConfig(ctx context.Context, c *app.RequestContext) {
	defaultObjectStorageAdminHandler().activate(ctx, c)
}

func DeleteObjectStorageConfig(ctx context.Context, c *app.RequestContext) {
	defaultObjectStorageAdminHandler().delete(ctx, c)
}

const timeFormatRFC3339Nano = "2006-01-02T15:04:05.999999999Z07:00"

func stringPtr(value string) *string { return &value }

func boolPtr(value bool) *bool { return &value }
