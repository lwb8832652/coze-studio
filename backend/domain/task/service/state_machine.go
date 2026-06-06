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

package service

import "github.com/coze-dev/coze-studio/backend/domain/task/entity"

var allowedTransitions = map[entity.Status]map[entity.Status]bool{
	entity.StatusCreated: {
		entity.StatusQueued:   true,
		entity.StatusRunning:  true,
		entity.StatusFailed:   true,
		entity.StatusCanceled: true,
	},
	entity.StatusQueued: {
		entity.StatusRunning:  true,
		entity.StatusFailed:   true,
		entity.StatusCanceled: true,
	},
	entity.StatusRunning: {
		entity.StatusSucceeded: true,
		entity.StatusFailed:    true,
		entity.StatusCanceling: true,
	},
	entity.StatusCanceling: {
		entity.StatusCanceled: true,
		entity.StatusFailed:   true,
	},
	entity.StatusFailed: {
		entity.StatusQueued: true,
	},
}

func CanTransition(from, to entity.Status) bool {
	return allowedTransitions[from][to]
}

func EnsureTransition(from, to entity.Status) error {
	if CanTransition(from, to) {
		return nil
	}
	return InvalidArgumentErrorf("cannot transition task from %s to %s", from, to)
}
