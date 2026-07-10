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
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSanitizeChatAttachmentsKeepsPrototypeImageType(t *testing.T) {
	attachments := sanitizeChatAttachments([]ChatAttachment{
		{
			ID:       "prototype-1",
			Name:     "homepage.png",
			Path:     "src/assets/uploads/prototype-homepage.png",
			MIMEType: "image/png",
			Size:     1024,
			Type:     "prototype_image",
		},
		{
			ID:       "doc-1",
			Name:     "spec.pdf",
			Path:     "src/assets/uploads/spec.pdf",
			MIMEType: "application/pdf",
			Size:     2048,
			Type:     "prototype_image",
		},
	})

	require.Len(t, attachments, 2)
	require.Equal(t, "prototype_image", attachments[0].Type)
	require.Equal(t, "file", attachments[1].Type)
}
