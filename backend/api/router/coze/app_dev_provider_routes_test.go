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

package coze_test

import (
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/stretchr/testify/require"

	appdevapi "github.com/coze-dev/coze-studio/backend/api/model/appdev"
	cozerouter "github.com/coze-dev/coze-studio/backend/api/router/coze"
)

func TestAppDevProviderRoutesAreOwnedByMasterIDLSource(t *testing.T) {
	content, err := os.ReadFile("../../../../idl/appdev/app_dev.thrift")
	require.NoError(t, err)
	source := string(content)
	for _, route := range []string{
		`api.post="/api/app-dev/spaces/:space_id/projects/:project_id/build"`,
		`api.get="/api/app-dev/spaces/:space_id/projects/:project_id/build"`,
		`api.get="/api/app-dev/spaces/:space_id/projects/:project_id/release"`,
		`api.post="/api/app-dev/spaces/:space_id/projects/:project_id/snapshots/:snapshot_id/restore"`,
		`api.post="/api/app-dev/spaces/:space_id/projects/:project_id/runtime/start"`,
		`api.get="/api/app-dev/spaces/:space_id/projects/:project_id/runtime/status"`,
		`api.post="/api/app-dev/spaces/:space_id/projects/:project_id/runtime/keep-alive"`,
		`api.post="/api/app-dev/spaces/:space_id/projects/:project_id/runtime/restart"`,
		`api.post="/api/app-dev/spaces/:space_id/projects/:project_id/runtime/stop"`,
		`api.get="/api/app-dev/spaces/:space_id/projects/:project_id/runtime/logs"`,
	} {
		require.Contains(t, strings.ReplaceAll(source, " ", ""), route)
	}
	require.NotContains(t, source, "/build/artifact")
}

func TestAppDevProviderRoutesUseDedicatedSafeIDLContracts(t *testing.T) {
	content, err := os.ReadFile("../../../../idl/appdev/app_dev.thrift")
	require.NoError(t, err)
	source := string(content)
	for _, contract := range []string{
		"struct AppDevProviderScopeRequest",
		"struct AppDevProviderOperationRequest",
		"struct AppDevSnapshotRestoreRequest",
		"struct AppDevProviderRuntimeResponse",
		"struct AppDevProviderBuildResponse",
		"struct AppDevProviderLogsResponse",
		"struct AppDevReleaseStreamResponse",
		`api.header = "Idempotency-Key"`,
		`json:\"generation\"`, `json:\"state\"`, `json:\"can_start\"`,
		`json:\"release_available\"`, `json:\"updated_at,omitempty\"`,
	} {
		require.Contains(t, source, contract)
	}
	for _, method := range []string{
		"BuildAppDevProject", "GetAppDevBuildStatus", "DownloadAppDevRelease", "RestoreAppDevSnapshot",
		"StartAppDevRuntime", "GetAppDevRuntimeStatus", "KeepAliveAppDevRuntime", "RestartAppDevRuntime", "StopAppDevRuntime", "ListAppDevRuntimeLogs",
	} {
		require.NotContains(t, source, "AppDevRouteResponse "+method)
	}
	for _, forbidden := range []string{"object_key", "digest", "provider_id", "provider_execution", "checkpoint", "token"} {
		require.NotContains(t, strings.ToLower(source), forbidden)
	}

	script, err := os.ReadFile("../../../scripts/verify_api_codegen.sh")
	require.NoError(t, err)
	verification := string(script)
	require.Contains(t, verification, "v0.9.7")
	require.Contains(t, verification, "0.4.5")
	require.Contains(t, verification, "--exclude_file")
	require.Contains(t, verification, "CODEGEN_DETERMINISTIC_SHA256=PASS")
	require.Contains(t, verification, "HANDLERS_EXCLUDED_SHA256=PASS")
	require.Contains(t, verification, `[[ "${hz_version}" == "hz version v0.9.7" ]]`)
	require.Contains(t, verification, `[[ "${thriftgo_version}" == "thriftgo 0.4.5" ]]`)
	require.Contains(t, verification, `temporary_root=`)
	require.Contains(t, verification, `run_one_backend=`)
	require.Contains(t, verification, `run_two_backend=`)
	require.Contains(t, verification, `copy_codegen_inputs`)
	require.Contains(t, verification, `copy_handwritten_handlers`)
	require.Contains(t, verification, `select_sha256`)
	require.Contains(t, verification, `KEEP_API_CODEGEN_TMP`)
	require.Contains(t, verification, `STALE_GENERATED_FIXTURE=PASS`)
	require.Contains(t, verification, `REAL_VS_CLEAN_RUN1`)
	require.Contains(t, verification, `CLEAN_RUN1_VS_CLEAN_RUN2`)
	require.Contains(t, verification, `api/router/coze/middleware.go`)
	require.Contains(t, verification, `stale_fixture_path="${shadow_real_backend}/api/model/appdev/stale_generated_fixture.go"`)
	require.Contains(t, verification, `compare_generated_trees "SHADOW_REAL_STALE"`)
	require.Contains(t, verification, `discover_generated_files`)
	require.Contains(t, verification, `handwritten_generated_excludes`)
	require.Contains(t, verification, `unexpected generated path in actual:`)
	require.NotContains(t, verification, `independent_generated_outputs`)
	require.NotContains(t, verification, `cp -R "${backend_dir}/api"`)
	require.NotContains(t, verification, `cp -R "${backend_dir}/api/model"`)
	require.NotContains(t, verification, `cd "${backend_dir}"`)
}

func TestGeneratedAppDevProviderDTOsMatchSafeWireContract(t *testing.T) {
	operation := reflect.TypeOf(appdevapi.AppDevProviderOperationRequest{})
	operationID, ok := operation.FieldByName("OperationID")
	require.True(t, ok)
	require.Equal(t, "Idempotency-Key,required", operationID.Tag.Get("header"))
	require.Equal(t, "-", operationID.Tag.Get("json"))

	runtime := reflect.TypeOf(appdevapi.AppDevProviderRuntimeResponse{})
	for field, tag := range map[string]string{
		"Generation": "generation", "State": "state", "CanStart": "can_start",
		"Recovering": "recovering", "Stopping": "stopping", "PreviewURL": "preview_url,omitempty", "SafeMessage": "safe_message,omitempty",
	} {
		actual, found := runtime.FieldByName(field)
		require.Truef(t, found, "missing runtime field %s", field)
		require.Equal(t, tag, actual.Tag.Get("json"))
	}
	require.Equal(t, reflect.Pointer, runtime.Field(5).Type.Kind())

	build := reflect.TypeOf(appdevapi.AppDevProviderBuildResponse{})
	for field, tag := range map[string]string{
		"Generation": "generation", "State": "state", "ReleaseAvailable": "release_available", "Size": "size",
		"UpdatedAt": "updated_at,omitempty", "Stale": "stale", "SafeMessage": "safe_message,omitempty",
	} {
		actual, found := build.FieldByName(field)
		require.Truef(t, found, "missing build field %s", field)
		require.Equal(t, tag, actual.Tag.Get("json"))
	}
	require.Zero(t, reflect.TypeOf(appdevapi.AppDevReleaseStreamResponse{}).NumField())
}

func TestAppDevProviderRoutesKeepExistingPathsAndMethods(t *testing.T) {
	h := server.Default()
	cozerouter.Register(h)
	base := "/api/app-dev/spaces/1001/projects/project-safe"

	for _, route := range []struct {
		method string
		path   string
	}{
		{http.MethodPost, base + "/build"},
		{http.MethodGet, base + "/build"},
		{http.MethodGet, base + "/release"},
		{http.MethodPost, base + "/snapshots/snapshot-safe/restore"},
		{http.MethodPost, base + "/runtime/start"},
		{http.MethodGet, base + "/runtime/status"},
		{http.MethodPost, base + "/runtime/keep-alive"},
		{http.MethodPost, base + "/runtime/restart"},
		{http.MethodPost, base + "/runtime/stop"},
		{http.MethodGet, base + "/runtime/logs"},
	} {
		response := ut.PerformRequest(h.Engine, route.method, route.path, nil)
		require.Equalf(t, http.StatusUnauthorized, response.Code, "%s %s", route.method, route.path)
	}

	for _, route := range []struct {
		method string
		path   string
	}{
		{http.MethodPut, base + "/build"},
		{http.MethodPost, base + "/release"},
		{http.MethodGet, base + "/build/artifact"},
	} {
		response := ut.PerformRequest(h.Engine, route.method, route.path, nil)
		require.Equalf(t, http.StatusNotFound, response.Code, "%s %s", route.method, route.path)
	}
}
