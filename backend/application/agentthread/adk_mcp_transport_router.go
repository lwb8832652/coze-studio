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

package agentthread

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

const (
	adkMCPRuntimeTransportStdio          = "stdio"
	adkMCPRuntimeTransportSSE            = "sse"
	adkMCPRuntimeTransportStreamableHTTP = "streamable_http"
)

type ADKMCPRuntimeTransportRouterOptions struct {
	Stdio          ADKMCPRuntimeTransportInvoker
	SSE            ADKMCPRuntimeTransportInvoker
	StreamableHTTP ADKMCPRuntimeTransportInvoker
}

type ADKMCPRuntimeTransportRouter struct {
	stdio          ADKMCPRuntimeTransportInvoker
	sse            ADKMCPRuntimeTransportInvoker
	streamableHTTP ADKMCPRuntimeTransportInvoker
}

type ADKMCPRuntimeTransportInvokerFunc func(
	ctx context.Context,
	call ADKMCPRuntimeTransportCall,
) (string, error)

func (f ADKMCPRuntimeTransportInvokerFunc) InvokeADKMCPRuntimeTransport(
	ctx context.Context,
	call ADKMCPRuntimeTransportCall,
) (string, error) {
	if f == nil {
		return "", fmt.Errorf("mcp runtime transport is not configured")
	}

	return f(ctx, call)
}

func NewADKMCPRuntimeTransportRouter(
	options ADKMCPRuntimeTransportRouterOptions,
) *ADKMCPRuntimeTransportRouter {
	return &ADKMCPRuntimeTransportRouter{
		stdio:          options.Stdio,
		sse:            options.SSE,
		streamableHTTP: options.StreamableHTTP,
	}
}

func (r *ADKMCPRuntimeTransportRouter) InvokeADKMCPRuntimeTransport(
	ctx context.Context,
	call ADKMCPRuntimeTransportCall,
) (string, error) {
	transportType := normalizeADKMCPRuntimeTransportType(call)
	switch transportType {
	case adkMCPRuntimeTransportStdio:
		return invokeADKMCPRuntimeTransportHandler(
			ctx,
			call,
			r.stdioInvoker(),
			"mcp runtime stdio transport is disabled",
		)
	case adkMCPRuntimeTransportSSE:
		return invokeADKMCPRuntimeTransportHandler(
			ctx,
			call,
			r.sseInvoker(),
			"mcp runtime sse transport is disabled",
		)
	case adkMCPRuntimeTransportStreamableHTTP:
		return invokeADKMCPRuntimeTransportHandler(
			ctx,
			call,
			r.streamableHTTPInvoker(),
			"mcp runtime streamable_http transport is disabled",
		)
	default:
		return "", errors.New("unsupported mcp runtime transport")
	}
}

func (r *ADKMCPRuntimeTransportRouter) stdioInvoker() ADKMCPRuntimeTransportInvoker {
	if r == nil {
		return nil
	}

	return r.stdio
}

func (r *ADKMCPRuntimeTransportRouter) sseInvoker() ADKMCPRuntimeTransportInvoker {
	if r == nil {
		return nil
	}

	return r.sse
}

func (r *ADKMCPRuntimeTransportRouter) streamableHTTPInvoker() ADKMCPRuntimeTransportInvoker {
	if r == nil {
		return nil
	}

	return r.streamableHTTP
}

func invokeADKMCPRuntimeTransportHandler(
	ctx context.Context,
	call ADKMCPRuntimeTransportCall,
	invoker ADKMCPRuntimeTransportInvoker,
	disabledMessage string,
) (string, error) {
	if invoker == nil {
		return "", errors.New(disabledMessage)
	}

	return invoker.InvokeADKMCPRuntimeTransport(ctx, call)
}

func normalizeADKMCPRuntimeTransportType(
	call ADKMCPRuntimeTransportCall,
) string {
	if call.Server == nil {
		return ""
	}
	switch strings.ToLower(strings.TrimSpace(call.Server.ServerType)) {
	case "http", "streamable-http", adkMCPRuntimeTransportStreamableHTTP:
		return adkMCPRuntimeTransportStreamableHTTP
	case adkMCPRuntimeTransportStdio:
		return adkMCPRuntimeTransportStdio
	case adkMCPRuntimeTransportSSE:
		return adkMCPRuntimeTransportSSE
	default:
		return ""
	}
}
