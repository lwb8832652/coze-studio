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
	"encoding/json"
	"fmt"
	"strings"
)

const adkWriteFileToolName = "write_file"
const adkPresentFilesToolName = "present_files"

const adkWriteFileInputSchema = `{
  "type":"object",
  "properties":{
    "file_path":{
      "type":"string",
      "description":"Absolute output file path under /mnt/user-data/outputs, for example /mnt/user-data/outputs/report.md."
    },
    "content":{
      "type":"string",
      "description":"Complete file content to write."
    },
    "content_type":{
      "type":"string",
      "description":"Optional MIME type, for example text/markdown; charset=utf-8."
    }
  },
  "required":["file_path","content"]
}`

const adkPresentFilesInputSchema = `{
  "type":"object",
  "properties":{
    "filepaths":{
      "type":"array",
      "description":"Output file paths to present to the user. Only /mnt/user-data/outputs/* paths are allowed.",
      "items":{"type":"string"}
    }
  },
  "required":["filepaths"]
}`

type ADKArtifactToolCatalog struct {
	app *ApplicationService
}

type adkWriteFileInput struct {
	FilePath    string `json:"file_path"`
	Content     string `json:"content"`
	ContentType string `json:"content_type"`
}

type adkPresentFilesInput struct {
	FilePaths []string `json:"filepaths"`
}

type adkArtifactToolInvoker struct {
	app *ApplicationService
}

func NewADKArtifactToolCatalog(app *ApplicationService) *ADKArtifactToolCatalog {
	return &ADKArtifactToolCatalog{app: app}
}

func (c *ADKArtifactToolCatalog) LoadADKRuntimeTools(
	_ context.Context,
	_ *RunSummary,
) ([]ADKRuntimeToolDefinition, error) {
	if c == nil || c.app == nil {
		return nil, fmt.Errorf("artifact tool catalog is not configured")
	}
	invoker := &adkArtifactToolInvoker{app: c.app}
	return []ADKRuntimeToolDefinition{
		{
			Name:        adkWriteFileToolName,
			Description: "Write a user-visible output file under /mnt/user-data/outputs. Use present_files after writing files the user should see.",
			InputSchema: adkWriteFileInputSchema,
			Visibility:  ADKRuntimeToolVisibilityStatic,
			Invoker:     invoker,
		},
		{
			Name:        adkPresentFilesToolName,
			Description: "Present output files to the user as task artifacts. Only files under /mnt/user-data/outputs can be presented.",
			InputSchema: adkPresentFilesInputSchema,
			Visibility:  ADKRuntimeToolVisibilityStatic,
			Invoker:     invoker,
		},
	}, nil
}

func (i *adkArtifactToolInvoker) InvokeADKRuntimeTool(
	ctx context.Context,
	call ADKRuntimeToolCall,
) (string, error) {
	if i == nil || i.app == nil {
		return "", fmt.Errorf("artifact tool invoker is not configured")
	}
	if call.Run == nil {
		return "", fmt.Errorf("artifact tool run is required")
	}
	switch strings.TrimSpace(call.Name) {
	case adkWriteFileToolName:
		var input adkWriteFileInput
		if err := json.Unmarshal([]byte(strings.TrimSpace(call.Arguments)), &input); err != nil {
			return "", fmt.Errorf("write_file arguments are invalid: %w", err)
		}
		resp, err := i.app.WriteOutputFile(ctx, &WriteOutputFileRequest{
			Run:         call.Run,
			FilePath:    input.FilePath,
			Content:     input.Content,
			ContentType: input.ContentType,
		})
		if err != nil {
			return "", err
		}
		return resp.Notice, nil
	case adkPresentFilesToolName:
		var input adkPresentFilesInput
		if err := json.Unmarshal([]byte(strings.TrimSpace(call.Arguments)), &input); err != nil {
			return "", fmt.Errorf("present_files arguments are invalid: %w", err)
		}
		resp, err := i.app.PresentOutputFiles(ctx, &PresentOutputFilesRequest{
			Run:       call.Run,
			FilePaths: input.FilePaths,
		})
		if err != nil {
			return "", err
		}
		return resp.Notice, nil
	default:
		return "", fmt.Errorf("unsupported artifact tool: %s", call.Name)
	}
}
