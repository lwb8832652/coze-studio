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

package workbench

import (
	"errors"
	"fmt"

	appmcptool "github.com/coze-dev/coze-studio/backend/application/mcptool"
	appskill "github.com/coze-dev/coze-studio/backend/application/skill"
)

type invalidArgumentError struct {
	msg string
}

func (e invalidArgumentError) Error() string {
	return e.msg
}

func InvalidArgumentErrorf(format string, args ...any) error {
	return invalidArgumentError{msg: fmt.Sprintf(format, args...)}
}

func IsClientError(err error) bool {
	var target invalidArgumentError

	return errors.As(err, &target) || appskill.IsClientError(err) || appmcptool.IsClientError(err)
}
