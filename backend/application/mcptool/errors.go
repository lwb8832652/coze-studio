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

package mcptool

import (
	"errors"
	"fmt"
)

type clientError struct {
	message string
}

func (e clientError) Error() string {
	return e.message
}

func InvalidArgumentErrorf(format string, args ...any) error {
	return clientError{message: fmt.Sprintf(format, args...)}
}

func IsClientError(err error) bool {
	var target clientError

	return errors.As(err, &target) || errors.Is(err, ErrNotFound)
}
