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

package appdev

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	appdevapp "github.com/coze-dev/coze-studio/backend/application/appdev"

	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/require"
)

func TestGenerateAppSourceWithTimeoutFallsBackWhenGeneratorBlocks(t *testing.T) {
	release := make(chan struct{})
	defer close(release)

	source, generatedByModel := generateAppSourceWithTimeout(
		context.Background(),
		"fallback-source",
		10*time.Millisecond,
		func(context.Context) (string, bool) {
			<-release
			return "late-source", true
		},
	)

	require.False(t, generatedByModel)
	require.Equal(t, "fallback-source", source)
}

func TestGenerateAppSourceWithTimeoutReturnsGeneratedSource(t *testing.T) {
	source, generatedByModel := generateAppSourceWithTimeout(
		context.Background(),
		"fallback-source",
		time.Second,
		func(context.Context) (string, bool) {
			return "model-source", true
		},
	)

	require.True(t, generatedByModel)
	require.Equal(t, "model-source", source)
}

func TestBuildAppDevUserMessageFallsBackToTextWhenPrototypeImagesDisabled(t *testing.T) {
	req := &appdevapp.ChatManagerRequest{
		Message:    "参考原型图生成首页",
		ProjectDir: writeAppDevPrototypeImage(t, t.TempDir(), "prototype.png"),
		Attachments: []appdevapp.ChatAttachment{
			{
				Name:     "prototype.png",
				Path:     "src/assets/uploads/prototype.png",
				MIMEType: "image/png",
				Type:     "prototype_image",
			},
		},
	}

	message := buildAppDevUserMessage(req, false)

	require.Equal(t, schema.User, message.Role)
	require.Empty(t, message.MultiContent)
	require.Contains(t, message.Content, "原型图 prototype.png")
}

func TestBuildAppDevUserMessageIncludesPrototypeImageWhenBase64Enabled(t *testing.T) {
	req := &appdevapp.ChatManagerRequest{
		Message:    "参考原型图生成首页",
		ProjectDir: writeAppDevPrototypeImage(t, t.TempDir(), "prototype.png"),
		Attachments: []appdevapp.ChatAttachment{
			{
				Name:     "prototype.png",
				Path:     "src/assets/uploads/prototype.png",
				MIMEType: "image/png",
				Type:     "prototype_image",
			},
		},
	}

	message := buildAppDevUserMessage(req, true)

	require.Equal(t, schema.User, message.Role)
	require.Len(t, message.MultiContent, 2)
	require.Equal(t, schema.ChatMessagePartTypeText, message.MultiContent[0].Type)
	require.Contains(t, message.MultiContent[0].Text, "参考原型图生成首页")
	require.Equal(t, schema.ChatMessagePartTypeImageURL, message.MultiContent[1].Type)
	require.NotNil(t, message.MultiContent[1].ImageURL)
	require.Equal(t, "image/png", message.MultiContent[1].ImageURL.MIMEType)
	require.True(t, strings.HasPrefix(message.MultiContent[1].ImageURL.URL, "data:image/png;base64,"))
}

func TestBuildAppDevUserMessageSkipsOversizedPrototypeImage(t *testing.T) {
	projectDir := t.TempDir()
	uploadDir := filepath.Join(projectDir, "src", "assets", "uploads")
	require.NoError(t, os.MkdirAll(uploadDir, 0o755))
	largeImage := append([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}, make([]byte, appDevMaxPrototypeImageBytes)...)
	require.NoError(t, os.WriteFile(filepath.Join(uploadDir, "large.png"), largeImage, 0o644))

	req := &appdevapp.ChatManagerRequest{
		Message:    "参考原型图生成首页",
		ProjectDir: projectDir,
		Attachments: []appdevapp.ChatAttachment{
			{
				Name:     "large.png",
				Path:     "src/assets/uploads/large.png",
				MIMEType: "image/png",
				Type:     "prototype_image",
			},
		},
	}

	message := buildAppDevUserMessage(req, true)

	require.Equal(t, schema.User, message.Role)
	require.Empty(t, message.MultiContent)
	require.Contains(t, message.Content, "large.png")
}

func TestBuildAppDevUserMessageReportsPrototypeImageCount(t *testing.T) {
	req := &appdevapp.ChatManagerRequest{
		Message:    "参考原型图生成首页",
		ProjectDir: writeAppDevPrototypeImage(t, t.TempDir(), "prototype.png"),
		Attachments: []appdevapp.ChatAttachment{
			{
				Name:     "prototype.png",
				Path:     "src/assets/uploads/prototype.png",
				MIMEType: "image/png",
				Type:     "prototype_image",
			},
		},
	}

	message, prototypeImageCount := buildAppDevUserMessageWithPrototypeImages(req, true)

	require.Len(t, message.MultiContent, 2)
	require.Equal(t, 1, prototypeImageCount)
}

func TestPrototypeImageProgressEventReportsVisualInputOrFallback(t *testing.T) {
	visualEvent := prototypeImageProgressEvent("request-1", 2, 1)
	require.NotNil(t, visualEvent)
	require.Equal(t, "tool_call_update", visualEvent.Event)
	require.Equal(t, "读取原型图", visualEvent.Data["title"])
	require.Contains(t, visualEvent.Data["summary"], "1 张原型图")
	require.Contains(t, visualEvent.Data["summary"], "视觉上下文")

	fallbackEvent := prototypeImageProgressEvent("request-2", 1, 0)
	require.NotNil(t, fallbackEvent)
	require.Equal(t, "读取原型图", fallbackEvent.Data["title"])
	require.Contains(t, fallbackEvent.Data["summary"], "未作为视觉输入")
	require.Contains(t, fallbackEvent.Data["summary"], "文字描述")

	require.Nil(t, prototypeImageProgressEvent("request-3", 0, 0))
}

func writeAppDevPrototypeImage(t *testing.T, projectDir string, name string) string {
	t.Helper()

	uploadDir := filepath.Join(projectDir, "src", "assets", "uploads")
	require.NoError(t, os.MkdirAll(uploadDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(uploadDir, name),
		[]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'},
		0o644,
	))

	return projectDir
}
