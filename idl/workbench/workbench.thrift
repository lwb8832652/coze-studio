namespace go workbench.chat

include "../base.thrift"
include "./task.thrift"

enum ChatMode {
    Auto = 1,
    Ask = 2,
    Agent = 3,
}

enum RouteTarget {
    ChatDirect = 1,
    AgentEngine = 2,
    SkillEngine = 3,
    TaskEngine = 4,
}

struct WorkbenchChatRequest {
    1: required i64 space_id (agw.js_conv="str", api.js_conv="true")
    2: optional i64 conversation_id (agw.js_conv="str", api.js_conv="true")
    3: required string message
    4: required ChatMode mode
    5: optional i64 selected_skill_id (agw.js_conv="str", api.js_conv="true")
    255: optional base.Base Base (api.none="true")
}

struct WorkbenchChatData {
    1: required RouteTarget route_target
    2: optional string answer
    3: optional task.ChatTask task
    4: optional i64 conversation_id (agw.js_conv="str", api.js_conv="true")
    5: optional string reason
}

struct WorkbenchChatResponse {
    1: optional WorkbenchChatData data
    253: required i64 code
    254: required string msg
    255: optional base.BaseResp BaseResp (api.none="true")
}

service WorkbenchChatService {
    WorkbenchChatResponse WorkbenchChat(1: WorkbenchChatRequest request)(
        api.post="/api/workbench/chat",
        api.category="workbench"
    )
}

service WorkbenchTaskService extends task.WorkbenchTaskService {}
