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

type ChatMode int64

const (
	ChatModeAuto  ChatMode = 1
	ChatModeAsk   ChatMode = 2
	ChatModeAgent ChatMode = 3
)

type IntentKind string

const (
	IntentChat  IntentKind = "chat"
	IntentSkill IntentKind = "skill"
	IntentTask  IntentKind = "task"
	IntentAgent IntentKind = "agent"
)

type Intent struct {
	Kind          IntentKind
	RequiresAsync bool
}

type RouteTarget string

const (
	RouteChatDirect  RouteTarget = "chat_direct"
	RouteSkillEngine RouteTarget = "skill_engine"
	RouteTaskEngine  RouteTarget = "task_engine"
	RouteAgentEngine RouteTarget = "agent_engine"
)

type Decision struct {
	Target RouteTarget
	Reason string
}

func DecideRoute(mode ChatMode, intent Intent) Decision {
	switch mode {
	case ChatModeAsk:
		return Decision{Target: RouteChatDirect, Reason: "ask mode routes directly to quick chat"}
	case ChatModeAgent:
		return Decision{Target: RouteAgentEngine, Reason: "agent mode routes to agent engine"}
	}

	if intent.RequiresAsync {
		return Decision{Target: RouteTaskEngine, Reason: "async intent routes to task engine"}
	}

	switch intent.Kind {
	case IntentSkill:
		return Decision{Target: RouteSkillEngine, Reason: "skill intent routes to skill engine"}
	case IntentAgent:
		return Decision{Target: RouteAgentEngine, Reason: "agent intent routes to agent engine"}
	case IntentTask:
		return Decision{Target: RouteTaskEngine, Reason: "task intent routes to task engine"}
	default:
		return Decision{Target: RouteChatDirect, Reason: "chat intent routes directly"}
	}
}
