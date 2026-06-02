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

package domain

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCookieDomainFromHostKeepsLocalhostHostOnly(t *testing.T) {
	require.Empty(t, CookieDomainFromHost("localhost:8888"))
	require.Empty(t, CookieDomainFromHost("127.0.0.1:8080"))
	require.Empty(t, CookieDomainFromHost("[::1]:8888"))
	require.Empty(t, CookieDomainFromHost("::1"))
	require.Equal(t, "example.com", CookieDomainFromHost("example.com"))
	require.Equal(t, "example.com", CookieDomainFromHost("example.com:8888"))
}
