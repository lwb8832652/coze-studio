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
	"os"
	"strings"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/cloudwego/hertz/pkg/route/param"
	"github.com/stretchr/testify/require"

	"github.com/coze-dev/coze-studio/backend/pkg/logs"
)

func TestCanonicalProductPaginationAcceptsIntegralOffsetPages(t *testing.T) {
	for _, test := range []struct {
		name       string
		query      string
		wantLimit  int32
		wantOffset int32
		wantPage   int32
		wantNext   string
	}{
		{name: "defaults", wantLimit: 50, wantOffset: 0, wantPage: 1, wantNext: "50"},
		{name: "integral second page", query: "?limit=25&offset=25", wantLimit: 25, wantOffset: 25, wantPage: 2, wantNext: "50"},
		{name: "capped limit stays aligned", query: "?limit=999&offset=200", wantLimit: 200, wantOffset: 200, wantPage: 2, wantNext: "400"},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			var c app.RequestContext
			c.Request.SetRequestURI("/api/workbench/artifacts" + test.query)

			page, public := canonicalProductPagination(&c)
			require.Nil(t, public)
			require.Equal(t, canonicalProductPage{Limit: test.wantLimit, Offset: test.wantOffset, Page: test.wantPage}, page)
			next := page.nextCursor(int64(test.wantOffset + test.wantLimit + 1))
			require.NotNil(t, next)
			require.Equal(t, test.wantNext, *next)
			require.Nil(t, page.nextCursor(int64(test.wantOffset+test.wantLimit)))
		})
	}
}

func TestCanonicalProductPaginationRejectsNegativeOrMisalignedOffset(t *testing.T) {
	for _, query := range []string{
		"?limit=0", "?limit=-1", "?limit=not-a-number", "?limit=2147483648",
		"?offset=-1", "?offset=not-a-number", "?offset=2147483648", "?limit=20&offset=21",
	} {
		query := query
		t.Run(query, func(t *testing.T) {
			var c app.RequestContext
			c.Request.SetRequestURI("/api/workbench/artifacts" + query)
			_, public := canonicalProductPagination(&c)
			require.NotNil(t, public)
			require.Equal(t, consts.StatusUnprocessableEntity, public.status)
			require.Equal(t, "invalid_pagination", public.Code)
		})
	}
}

func TestCanonicalProductRouteIDsRejectMalformedAndZeroIDs(t *testing.T) {
	for _, name := range []string{"thread_id", "artifact_id", "memory_id", "job_id"} {
		name := name
		for _, value := range []string{"", "0", "-1", "+1", " 1", "1.0", "not-an-id"} {
			value := value
			t.Run(name+"_"+value, func(t *testing.T) {
				c := app.NewContext(1)
				c.Params = param.Params{{Key: name, Value: value}}
				_, public := canonicalProductPathID(c, name)
				require.NotNil(t, public)
				require.Equal(t, consts.StatusBadRequest, public.status)
				require.Equal(t, "invalid_path_parameter", public.Code)
			})
		}
	}
}

func TestCanonicalProductJSONRejectsUnknownFieldsAndOversizedBodies(t *testing.T) {
	t.Run("unknown field", func(t *testing.T) {
		var c app.RequestContext
		c.Request.SetBody([]byte(`{"name":"report","provider_body":"provider-body-secret"}`))
		var request struct {
			Name string `json:"name"`
		}
		public := decodeCanonicalProductJSON(&c, &request)
		require.NotNil(t, public)
		require.Equal(t, consts.StatusUnprocessableEntity, public.status)
		require.Equal(t, "unsupported_sdk_field", public.Code)
		require.NotContains(t, public.Detail, "provider-body-secret")
	})

	t.Run("oversized body", func(t *testing.T) {
		var c app.RequestContext
		c.Request.SetBody([]byte(`{"name":"` + strings.Repeat("x", canonicalMaxRequestBytes) + `"}`))
		var request struct {
			Name string `json:"name"`
		}
		public := decodeCanonicalProductJSON(&c, &request)
		require.NotNil(t, public)
		require.Equal(t, consts.StatusRequestEntityTooLarge, public.status)
		require.Equal(t, "request_too_large", public.Code)
	})
}

func TestCanonicalProductCompletionLogContainsResourceFieldsWithoutPayload(t *testing.T) {
	var output bytes.Buffer
	logs.SetOutput(&output)
	t.Cleanup(func() { logs.SetOutput(os.Stderr) })

	var c app.RequestContext
	c.Request.SetMethod("GET")
	c.Response.SetStatusCode(consts.StatusOK)
	logCanonicalRequestCompleted(context.Background(), &c, canonicalRequestLog{
		Operation:      "artifact.list",
		RouteTemplate:  "/api/workbench/artifacts",
		ResourceType:   "artifact",
		ResourceID:     "9001",
		Limit:          50,
		Offset:         100,
		LifecycleStage: "completed",
	}, "success")

	actual := output.String()
	for _, expected := range []string{
		"event_name=workbench.api.request.completed", "resource_type=artifact", "resource_id=9001",
		"limit=50", "offset=100", "lifecycle_stage=completed",
	} {
		require.Contains(t, actual, expected)
	}
	for _, sensitive := range canonicalProductSensitiveSentinels() {
		require.NotContains(t, actual, sensitive)
	}
}
