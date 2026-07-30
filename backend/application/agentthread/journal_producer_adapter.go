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
	"strings"

	domainentity "github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	domainservice "github.com/coze-dev/coze-studio/backend/domain/agentthread/service"
)

func journalProjectionToDomainRequest(
	event RunEvent,
	projection *JournalEventProjection,
) *domainservice.AppendJournalEventRequest {
	if projection == nil {
		return nil
	}
	request := &domainservice.AppendJournalEventRequest{
		ThreadID:       event.ThreadID,
		RunID:          event.RunID,
		IdempotencyKey: projection.IdempotencyKey,
		SchemaVersion:  domainentity.JournalSchemaVersion,
		Status:         projection.Status,
		Visibility:     domainentity.JournalVisibilityUser,
		PayloadVersion: domainentity.JournalPayloadVersion,
		EventType:      projection.EventType,
		Payload:        projection.Payload,
	}
	if strings.HasPrefix(projection.EventType, "action.") {
		request.ActionID = projection.ActionID
		request.Phase = projection.Phase
		request.Operation = projection.Operation
		request.Target = projection.Target
		request.Milestone = projection.Milestone
	}
	return request
}
