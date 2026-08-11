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
	"fmt"
	"strings"
	"testing"

	hertzconsts "github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/stretchr/testify/require"
)

func TestCanonicalExecutionControlIngressValidator(t *testing.T) {
	retiredFields := []string{
		"requested_policy",
		"mode",
		"thinking_enabled",
		"reasoning_effort",
		"is_plan_mode",
		"subagent_enabled",
		"max_concurrent_subagents",
	}
	auditedPaths := []struct {
		name         string
		kind         canonicalExecutionControlIngressKind
		bodyTemplate string
		pathTemplate string
	}{
		{
			name:         "root only root",
			kind:         canonicalExecutionControlRootOnly,
			bodyTemplate: `{"%s":true}`,
			pathTemplate: "%s",
		},
		{
			name:         "run config",
			kind:         canonicalExecutionControlRunSubmission,
			bodyTemplate: `{"config":{"%s":true}}`,
			pathTemplate: "config.%s",
		},
		{
			name:         "run context",
			kind:         canonicalExecutionControlRunSubmission,
			bodyTemplate: `{"context":{"%s":true}}`,
			pathTemplate: "context.%s",
		},
		{
			name:         "run config reserved nested",
			kind:         canonicalExecutionControlRunSubmission,
			bodyTemplate: `{"config":{"configurable":{"context":{"%s":true}}}}`,
			pathTemplate: "config.configurable.context.%s",
		},
		{
			name:         "run context reserved nested",
			kind:         canonicalExecutionControlRunSubmission,
			bodyTemplate: `{"context":{"context":{"configurable":{"%s":true}}}}`,
			pathTemplate: "context.context.configurable.%s",
		},
		{
			name:         "create immediate config",
			kind:         canonicalExecutionControlCreateThread,
			bodyTemplate: `{"coze":{"initial_run":{"config":{"%s":true}}}}`,
			pathTemplate: "coze.initial_run.config.%s",
		},
		{
			name:         "create immediate context",
			kind:         canonicalExecutionControlCreateThread,
			bodyTemplate: `{"coze":{"initial_run":{"context":{"%s":true}}}}`,
			pathTemplate: "coze.initial_run.context.%s",
		},
		{
			name:         "create deferred config",
			kind:         canonicalExecutionControlCreateThread,
			bodyTemplate: `{"coze":{"deferred_initial_run":{"config":{"%s":true}}}}`,
			pathTemplate: "coze.deferred_initial_run.config.%s",
		},
		{
			name:         "create deferred context",
			kind:         canonicalExecutionControlCreateThread,
			bodyTemplate: `{"coze":{"deferred_initial_run":{"context":{"%s":true}}}}`,
			pathTemplate: "coze.deferred_initial_run.context.%s",
		},
	}
	for _, auditedPath := range auditedPaths {
		auditedPath := auditedPath
		for _, field := range retiredFields {
			field := field
			t.Run(auditedPath.name+" "+field, func(t *testing.T) {
				public := validateCanonicalExecutionControlIngress(
					[]byte(fmt.Sprintf(auditedPath.bodyTemplate, field)),
					auditedPath.kind,
				)
				assertCanonicalExecutionControlUnsupported(
					t,
					public,
					fmt.Sprintf(auditedPath.pathTemplate, field),
				)
			})
		}
	}

	t.Run("mixed case keys use canonical lowercase path", func(t *testing.T) {
		public := validateCanonicalExecutionControlIngress(
			[]byte(`{"CoZe":{"DeFeRrEd_InItIaL_RuN":{"CoNtExT":{"CoNfIgUrAbLe":{"ReQuEsTeD_PoLiCy":"fast"}}}}}`),
			canonicalExecutionControlCreateThread,
		)
		assertCanonicalExecutionControlUnsupported(
			t,
			public,
			"coze.deferred_initial_run.context.configurable.requested_policy",
		)
	})

	for index := 0; index < len(retiredFields)-1; index++ {
		earlier := retiredFields[index]
		later := retiredFields[index+1]
		t.Run("field priority "+earlier+" before "+later, func(t *testing.T) {
			public := validateCanonicalExecutionControlIngress(
				[]byte(fmt.Sprintf(`{"%s":true,"%s":true}`, later, earlier)),
				canonicalExecutionControlRootOnly,
			)
			assertCanonicalExecutionControlUnsupported(t, public, earlier)
		})
	}

	t.Run("root control wins over nested config and context", func(t *testing.T) {
		public := validateCanonicalExecutionControlIngress(
			[]byte(`{"config":{"requested_policy":"nested"},"context":{"mode":"nested"},"max_concurrent_subagents":2}`),
			canonicalExecutionControlRunSubmission,
		)
		assertCanonicalExecutionControlUnsupported(t, public, "max_concurrent_subagents")
	})

	t.Run("direct config control wins over reserved nested control", func(t *testing.T) {
		public := validateCanonicalExecutionControlIngress(
			[]byte(`{"config":{"configurable":{"requested_policy":"nested"},"max_concurrent_subagents":2}}`),
			canonicalExecutionControlRunSubmission,
		)
		assertCanonicalExecutionControlUnsupported(t, public, "config.max_concurrent_subagents")
	})

	t.Run("configurable wins over context regardless of source order", func(t *testing.T) {
		public := validateCanonicalExecutionControlIngress(
			[]byte(`{"config":{"context":{"requested_policy":"context"},"configurable":{"max_concurrent_subagents":2}}}`),
			canonicalExecutionControlRunSubmission,
		)
		assertCanonicalExecutionControlUnsupported(
			t,
			public,
			"config.configurable.max_concurrent_subagents",
		)
	})

	t.Run("create config wins over context regardless of source order", func(t *testing.T) {
		public := validateCanonicalExecutionControlIngress(
			[]byte(`{"coze":{"initial_run":{"context":{"requested_policy":"context"},"config":{"max_concurrent_subagents":2}}}}`),
			canonicalExecutionControlCreateThread,
		)
		assertCanonicalExecutionControlUnsupported(
			t,
			public,
			"coze.initial_run.config.max_concurrent_subagents",
		)
	})

	t.Run("path priority ignores source order", func(t *testing.T) {
		public := validateCanonicalExecutionControlIngress(
			[]byte(`{"context":{"requested_policy":"fast"},"config":{"max_concurrent_subagents":2}}`),
			canonicalExecutionControlRunSubmission,
		)
		assertCanonicalExecutionControlUnsupported(t, public, "config.max_concurrent_subagents")
	})

	t.Run("create path priority ignores source order", func(t *testing.T) {
		public := validateCanonicalExecutionControlIngress(
			[]byte(`{"coze":{"deferred_initial_run":{"config":{"requested_policy":"fast"}},"initial_run":{"context":{"max_concurrent_subagents":2}}}}`),
			canonicalExecutionControlCreateThread,
		)
		assertCanonicalExecutionControlUnsupported(t, public, "coze.initial_run.context.max_concurrent_subagents")
	})

	for _, test := range []struct {
		name string
		kind canonicalExecutionControlIngressKind
		body string
	}{
		{
			name: "root normalized duplicate",
			kind: canonicalExecutionControlRunSubmission,
			body: `{"Config":{},"config":{}}`,
		},
		{
			name: "path container normalized duplicate",
			kind: canonicalExecutionControlCreateThread,
			body: `{"coze":{"Initial_Run":{"config":{}},"initial_run":{"context":{}}}}`,
		},
		{
			name: "reserved container normalized duplicate",
			kind: canonicalExecutionControlRunSubmission,
			body: `{"config":{"Configurable":{},"configurable":{}}}`,
		},
		{
			name: "control normalized duplicate wins over control hit",
			kind: canonicalExecutionControlRootOnly,
			body: `{"MODE":"first","mode":"second"}`,
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			public := validateCanonicalExecutionControlIngress([]byte(test.body), test.kind)
			require.NotNil(t, public)
			require.Equal(t, hertzconsts.StatusBadRequest, public.status)
			require.Equal(t, "invalid_json", public.Code)
			require.Equal(t, "invalid_json", public.errorClass)
			require.False(t, public.Retryable)
		})
	}

	for _, body := range []string{
		``,
		`{`,
		`[]`,
		`null`,
		`{"config":`,
		`{"mode":true} trailing`,
		`{"mode":true}{"mode":false}`,
	} {
		body := body
		t.Run("malformed is left to binder "+body, func(t *testing.T) {
			require.Nil(t, validateCanonicalExecutionControlIngress(
				[]byte(body),
				canonicalExecutionControlRunSubmission,
			))
		})
	}

	t.Run("unknown ingress kind fails closed", func(t *testing.T) {
		public := validateCanonicalExecutionControlIngress(
			[]byte(`{"config":{"mode":"legacy"}}`),
			canonicalExecutionControlIngressKind(255),
		)
		require.NotNil(t, public)
		require.Equal(t, hertzconsts.StatusUnprocessableEntity, public.status)
		require.Equal(t, "invalid_request", public.Code)
		require.Equal(t, "invalid_execution_control_ingress_kind", public.errorClass)
		require.Equal(t, "Execution control ingress kind is invalid", public.Detail)
		require.False(t, public.Retryable)
	})

	for _, test := range []struct {
		name string
		kind canonicalExecutionControlIngressKind
		body string
	}{
		{
			name: "string containing retired field",
			kind: canonicalExecutionControlRunSubmission,
			body: `{"input":{"messages":[{"role":"user","content":"requested_policy and mode are words"}]}}`,
		},
		{
			name: "array objects",
			kind: canonicalExecutionControlRunSubmission,
			body: `{"config":[{"mode":"legacy"}],"context":{"configurable":[{"reasoning_effort":"high"}]}}`,
		},
		{
			name: "run config resource object",
			kind: canonicalExecutionControlRunSubmission,
			body: `{"config":{"resource":{"mode":"resource business field"}}}`,
		},
		{
			name: "run context resource object",
			kind: canonicalExecutionControlRunSubmission,
			body: `{"context":{"resource":{"reasoning_effort":"resource business field"}}}`,
		},
		{
			name: "create config resource object",
			kind: canonicalExecutionControlCreateThread,
			body: `{"coze":{"initial_run":{"config":{"resource":{"mode":"resource business field"}}}}}`,
		},
		{
			name: "ordinary root resource object",
			kind: canonicalExecutionControlCreateThread,
			body: `{"metadata":{"context":{"mode":"metadata"}},"resource":{"config":{"reasoning_effort":"high"}},"tools":{"is_plan_mode":true}}`,
		},
		{
			name: "valid runtime resource configuration inside run config",
			kind: canonicalExecutionControlRunSubmission,
			body: `{"config":{"runtime":"eino_adk","model":{"id":"model"},"skill":{"id":"skill"},"mcp":{"id":"mcp"},"knowledge":{"id":"knowledge"},"database":{"id":"database"}}}`,
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			require.Nil(t, validateCanonicalExecutionControlIngress([]byte(test.body), test.kind))
		})
	}
}

func TestCanonicalExecutionControlIngressBudgets(t *testing.T) {
	t.Run("validator rejects parse depth over budget in irrelevant object", func(t *testing.T) {
		const excessiveParseDepth = 160
		body := `{"config":{"resource":` +
			strings.Repeat(`{"child":`, excessiveParseDepth) +
			`null` +
			strings.Repeat(`}`, excessiveParseDepth) +
			`}}`

		public := validateCanonicalExecutionControlIngress(
			[]byte(body),
			canonicalExecutionControlRunSubmission,
		)
		assertCanonicalExecutionControlBudgetExceeded(t, public)
	})

	t.Run("validator rejects parse node amplification in irrelevant array", func(t *testing.T) {
		const excessiveParseNodes = 70_000
		body := `{"config":{"resource":[` +
			strings.Repeat(`0,`, excessiveParseNodes) +
			`0]}}`

		public := validateCanonicalExecutionControlIngress(
			[]byte(body),
			canonicalExecutionControlRunSubmission,
		)
		assertCanonicalExecutionControlBudgetExceeded(t, public)
	})

	t.Run("reserved hop boundary", func(t *testing.T) {
		budget := canonicalExecutionControlIngressBudget{}
		require.Nil(t, budget.inspect("config.configurable.context.configurable.context", 4))
		assertCanonicalExecutionControlBudgetExceeded(
			t,
			budget.inspect("config.configurable.context.configurable.context.configurable", 5),
		)
	})

	t.Run("inspected object boundary", func(t *testing.T) {
		budget := canonicalExecutionControlIngressBudget{}
		for inspected := 1; inspected <= 64; inspected++ {
			require.Nil(t, budget.inspect("config", 0), inspected)
		}
		assertCanonicalExecutionControlBudgetExceeded(t, budget.inspect("config", 0))
	})

	t.Run("path byte boundary", func(t *testing.T) {
		budget := canonicalExecutionControlIngressBudget{}
		require.Nil(t, budget.inspect(strings.Repeat("a", 256), 0))

		budget = canonicalExecutionControlIngressBudget{}
		assertCanonicalExecutionControlBudgetExceeded(
			t,
			budget.inspect(strings.Repeat("a", 257), 0),
		)
	})

	t.Run("validator allows four reserved hops", func(t *testing.T) {
		body := `{"config":{"configurable":{"context":{"configurable":{"context":{"mode":"legacy"}}}}}}`
		public := validateCanonicalExecutionControlIngress(
			[]byte(body),
			canonicalExecutionControlRunSubmission,
		)
		assertCanonicalExecutionControlUnsupported(
			t,
			public,
			"config.configurable.context.configurable.context.mode",
		)
	})

	t.Run("validator rejects fifth reserved hop", func(t *testing.T) {
		body := `{"config":{"configurable":{"context":{"configurable":{"context":{"configurable":{"mode":"legacy"}}}}}}}`
		public := validateCanonicalExecutionControlIngress(
			[]byte(body),
			canonicalExecutionControlRunSubmission,
		)
		assertCanonicalExecutionControlBudgetExceeded(t, public)
	})
}

func assertCanonicalExecutionControlUnsupported(t *testing.T, public *canonicalError, path string) {
	t.Helper()
	require.NotNil(t, public)
	require.Equal(t, hertzconsts.StatusUnprocessableEntity, public.status)
	require.Equal(t, "unsupported_execution_control", public.Code)
	require.Equal(t, "unsupported_execution_control", public.errorClass)
	require.Equal(t, "Unsupported execution control: "+path, public.Detail)
	require.False(t, public.Retryable)
}

func assertCanonicalExecutionControlBudgetExceeded(t *testing.T, public *canonicalError) {
	t.Helper()
	require.NotNil(t, public)
	require.Equal(t, hertzconsts.StatusUnprocessableEntity, public.status)
	require.Equal(t, "invalid_request", public.Code)
	require.Equal(t, "execution_control_validation_budget_exceeded", public.errorClass)
	require.Equal(t, "Execution control validation budget exceeded", public.Detail)
	require.False(t, public.Retryable)
}
