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
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

const runtimeMetricErrorModelFailed = "model_failed"

type RuntimeModelCallMetricsConfig struct {
	Runtime     string
	ModelFamily string
}

type runtimeInstrumentedChatModel struct {
	base      model.BaseChatModel
	collector RuntimeMetricsCollector
	config    RuntimeModelCallMetricsConfig
}

func newRuntimeInstrumentedChatModel(
	base model.BaseChatModel,
	collector RuntimeMetricsCollector,
	config RuntimeModelCallMetricsConfig,
) model.BaseChatModel {
	if base == nil || collector == nil {
		return base
	}

	return &runtimeInstrumentedChatModel{
		base:      base,
		collector: collector,
		config:    config,
	}
}

func (m *runtimeInstrumentedChatModel) Generate(
	ctx context.Context,
	input []*schema.Message,
	options ...model.Option,
) (*schema.Message, error) {
	startedAt := time.Now()
	message, err := m.base.Generate(ctx, input, options...)
	m.record(ctx, startedAt, err)

	return message, err
}

func (m *runtimeInstrumentedChatModel) Stream(
	ctx context.Context,
	input []*schema.Message,
	options ...model.Option,
) (*schema.StreamReader[*schema.Message], error) {
	startedAt := time.Now()
	stream, err := m.base.Stream(ctx, input, options...)
	m.record(ctx, startedAt, err)

	return stream, err
}

func (m *runtimeInstrumentedChatModel) record(
	ctx context.Context,
	startedAt time.Time,
	err error,
) {
	if m == nil || m.collector == nil {
		return
	}
	result := runtimeMetricResultSuccess
	errorCode := runtimeMetricErrorNone
	if err != nil {
		result = runtimeMetricResultFailed
		errorCode = runtimeMetricErrorModelFailed
	}
	m.collector.RecordRuntimeModelCall(ctx, RuntimeModelCallMetricsObservation{
		Runtime:     m.config.Runtime,
		ModelFamily: m.config.ModelFamily,
		Result:      result,
		ErrorCode:   errorCode,
		LatencyMs:   time.Since(startedAt).Milliseconds(),
	})
}
