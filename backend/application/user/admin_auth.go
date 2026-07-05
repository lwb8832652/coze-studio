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

package user

import "strings"

func IsSystemAdminEmail(email string, adminEmails string) bool {
	normalizedEmail := strings.TrimSpace(email)
	normalizedAdminEmails := strings.TrimSpace(adminEmails)
	if normalizedEmail == "" || normalizedAdminEmails == "" {
		return false
	}

	for _, adminEmail := range strings.Split(normalizedAdminEmails, ",") {
		if strings.EqualFold(normalizedEmail, strings.TrimSpace(adminEmail)) {
			return true
		}
	}

	return false
}
