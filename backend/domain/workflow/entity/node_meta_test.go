// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package entity

import "testing"

func TestCommentNodeUsesBundledIcon(t *testing.T) {
	meta := NodeTypeMetas[NodeTypeComment]
	if meta == nil {
		t.Fatal("comment node metadata is missing")
	}
	const want = "default_icon/workflow_icon/icon-comment.svg"
	if meta.IconURI != want {
		t.Fatalf("comment node IconURI = %q, want %q", meta.IconURI, want)
	}
}
