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

package memory

import (
	"testing"

	crossuser "github.com/coze-dev/coze-studio/backend/crossdomain/user"
)

func TestHasSpaceAccess_AllowsAnyMemberSpace(t *testing.T) {
	spaces := []*crossuser.EntitySpace{
		{ID: 100},
		{ID: 200},
	}

	if !hasSpaceAccess(spaces, 200) {
		t.Fatal("expected a member of a non-first space to have access")
	}
}

func TestHasSpaceAccess_RejectsUnknownSpace(t *testing.T) {
	spaces := []*crossuser.EntitySpace{{ID: 100}}

	if hasSpaceAccess(spaces, 300) {
		t.Fatal("expected an unknown space to be rejected")
	}
}
