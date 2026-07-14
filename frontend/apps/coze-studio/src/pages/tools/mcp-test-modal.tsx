/*
 * Copyright 2025 coze-dev Authors
 * SPDX-License-Identifier: Apache-2.0
 */

import { useEffect, useMemo, useState } from 'react';

import type { workbenchTool } from '@coze-studio/api-schema';
import { Input, Modal, TextArea } from '@coze-arch/coze-design';

/* eslint-disable @coze-arch/max-line-per-function -- Cohesive schema-driven test modal. */

type MCPToolDefinition = workbenchTool.MCPToolDefinition;
type TestResult = workbenchTool.TestMCPToolCallData;

const JSON_INDENT = 2;

interface MCPTestModalProps {
  tool?: MCPToolDefinition;
  visible: boolean;
  onCancel: () => void;
  onRun: (argumentsJSON: string) => Promise<TestResult | undefined>;
}

interface JSONSchemaProperty {
  description?: string;
  type?: string;
}

const parseSchema = (tool?: MCPToolDefinition) => {
  try {
    const schema = JSON.parse(tool?.input_schema || '{}') as {
      properties?: Record<string, JSONSchemaProperty>;
      required?: string[];
    };
    return {
      properties: schema.properties ?? {},
      required: new Set(schema.required ?? []),
    };
  } catch (schemaError) {
    void schemaError;
    return { properties: {}, required: new Set<string>() };
  }
};

const coerceValue = (value: string, type?: string) => {
  if (!value) {
    return '';
  }
  if (type === 'number' || type === 'integer') {
    const numberValue = Number(value);
    return Number.isFinite(numberValue) ? numberValue : value;
  }
  if (type === 'boolean') {
    return value === 'true';
  }
  if (type === 'object' || type === 'array') {
    return JSON.parse(value);
  }
  return value;
};

export const MCPTestModal = ({
  tool,
  visible,
  onCancel,
  onRun,
}: MCPTestModalProps) => {
  const schema = useMemo(() => parseSchema(tool), [tool]);
  const [fieldValues, setFieldValues] = useState<Record<string, string>>({});
  const [rawArguments, setRawArguments] = useState('{}');
  const [running, setRunning] = useState(false);
  const [error, setError] = useState('');
  const [result, setResult] = useState<TestResult>();

  useEffect(() => {
    if (!visible) {
      return;
    }
    setFieldValues({});
    setRawArguments('{}');
    setError('');
    setResult(undefined);
  }, [tool, visible]);

  const handleFieldChange = (
    fieldName: string,
    value: string,
    type?: string,
  ) => {
    const nextValues = { ...fieldValues, [fieldName]: value };
    setFieldValues(nextValues);
    try {
      const nextArguments = Object.entries(nextValues).reduce<
        Record<string, unknown>
      >((acc, [name, fieldValue]) => {
        if (fieldValue !== '') {
          acc[name] = coerceValue(fieldValue, schema.properties[name]?.type);
        }
        return acc;
      }, {});
      setRawArguments(JSON.stringify(nextArguments, null, JSON_INDENT));
      setError('');
    } catch (coerceError) {
      void coerceError;
      setError('对象或数组参数必须使用有效 JSON。');
    }
  };

  const handleRun = async () => {
    try {
      const parsed = JSON.parse(rawArguments);
      if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
        throw new Error('invalid');
      }
      for (const name of schema.required) {
        if (!(name in parsed)) {
          setError(`请填写必填参数：${name}`);
          return;
        }
      }
    } catch (parseError) {
      void parseError;
      setError('请求参数必须是有效的 JSON 对象。');
      return;
    }

    setRunning(true);
    setError('');
    setResult(undefined);
    try {
      const nextResult = await onRun(rawArguments);
      setResult(nextResult);
    } catch (runError) {
      setError(runError instanceof Error ? runError.message : '试运行失败');
    } finally {
      setRunning(false);
    }
  };

  return (
    <Modal
      className="mcp-management-test-modal"
      title={`试运行 · ${tool?.name ?? ''}`}
      visible={visible}
      width={920}
      okText="运行"
      cancelText="关闭"
      confirmLoading={running}
      onCancel={onCancel}
      onOk={() => void handleRun()}
    >
      <div className="mcp-management-test-grid">
        <section>
          <div className="mcp-management-section-heading">
            <div>
              <h3>输入参数</h3>
              <p>{tool?.description || '该工具暂无描述。'}</p>
            </div>
          </div>

          {Object.entries(schema.properties).map(([name, property]) => (
            <label className="mcp-management-field" key={name}>
              <span>
                {name}
                {schema.required.has(name) ? <b> *</b> : null}
                <small> {property.type || 'string'}</small>
              </span>
              <Input
                aria-label={`参数 ${name}`}
                value={fieldValues[name] ?? ''}
                placeholder={property.description || '输入参数值'}
                onChange={value =>
                  handleFieldChange(name, value, property.type)
                }
              />
            </label>
          ))}

          {Object.keys(schema.properties).length === 0 ? (
            <div className="mcp-management-empty-compact">
              此工具没有声明参数。
            </div>
          ) : null}

          <details className="mcp-management-advanced">
            <summary>高级：编辑原始 JSON</summary>
            <TextArea
              aria-label="原始参数 JSON"
              className="mcp-management-code-input"
              value={rawArguments}
              rows={8}
              autosize={false}
              onChange={setRawArguments}
            />
          </details>
        </section>

        <section>
          <div className="mcp-management-section-heading">
            <div>
              <h3>调试结果</h3>
              <p>仅展示服务端审核过的状态、耗时和安全摘要。</p>
            </div>
          </div>
          <div className="mcp-management-test-output">
            {running ? <span>正在调用 MCP 工具...</span> : null}
            {!running && error ? (
              <div className="mcp-management-alert mcp-management-alert-error">
                {error}
              </div>
            ) : null}
            {!running && !error && result ? (
              <>
                <div className="mcp-management-result-meta">
                  <span data-status={result.status}>{result.status}</span>
                  <time>{result.latency_ms} ms</time>
                </div>
                <pre>{result.output}</pre>
              </>
            ) : null}
            {!running && !error && !result ? (
              <span>运行后将在这里展示结果。</span>
            ) : null}
          </div>
        </section>
      </div>
    </Modal>
  );
};
