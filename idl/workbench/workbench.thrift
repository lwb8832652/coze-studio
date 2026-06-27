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
    6: optional i64 task_id (agw.js_conv="str", api.js_conv="true")
    7: optional list<string> enable_skills
    8: optional list<string> enable_mcp
    9: optional list<string> enable_kbs
    10: optional list<string> enable_databases
    11: optional i64 model_type (agw.js_conv="str", api.js_conv="true")
    12: optional string model_name
    13: optional string runtime_settings
    255: optional base.Base Base (api.none="true")
}

struct WorkbenchChatData {
    1: required RouteTarget route_target
    2: optional string answer
    3: optional task.ChatTask task
    4: optional i64 conversation_id (agw.js_conv="str", api.js_conv="true")
    5: optional string reason
    6: optional string result_type
    7: optional string execution_type
}

struct WorkbenchChatResponse {
    1: optional WorkbenchChatData data
    253: required i64 code
    254: required string msg
    255: optional base.BaseResp BaseResp (api.none="true")
}

struct GetWorkbenchRuntimeDoctorRequest {
    1: required i64 space_id (agw.js_conv="str", api.js_conv="true")
    255: optional base.Base Base (api.none="true")
}

struct RuntimeDoctorCheck {
    1: required string name
    2: required string category
    3: required string status
    4: optional string message
}

struct RuntimeDoctorRuntimeData {
    1: required string default_mode
    2: required bool eino_adk_enabled
}

struct RuntimeDoctorWebToolStatus {
    1: required string status
    2: required bool configured
    3: optional string message
}

struct RuntimeDoctorWebToolsData {
    1: required RuntimeDoctorWebToolStatus web_fetch
    2: required RuntimeDoctorWebToolStatus web_search
}

struct RuntimeDoctorMCPToolsData {
    1: required string status
    2: required i64 total_servers
    3: required i64 enabled_servers
    4: required i64 healthy_servers
    5: required i64 unhealthy_servers
    6: required i64 unknown_servers
}

struct WorkbenchRuntimeDoctorData {
    1: required string status
    2: required RuntimeDoctorRuntimeData runtime
    3: required RuntimeDoctorWebToolsData web_tools
    4: required RuntimeDoctorMCPToolsData mcp_tools
    5: required list<RuntimeDoctorCheck> checks
}

struct WorkbenchRuntimeDoctorResponse {
    1: optional WorkbenchRuntimeDoctorData data
    253: required i64 code
    254: required string msg
    255: optional base.BaseResp BaseResp (api.none="true")
}

service WorkbenchChatService {
    WorkbenchChatResponse WorkbenchChat(1: WorkbenchChatRequest request)(
        api.post="/api/workbench/chat",
        api.category="workbench"
    )
    WorkbenchRuntimeDoctorResponse GetWorkbenchRuntimeDoctor(1: GetWorkbenchRuntimeDoctorRequest request)(
        api.get="/api/workbench/runtime_doctor",
        api.category="workbench"
    )
}

// Re-export through workbench to avoid the task include alias collision in idl/api.thrift.
service WorkbenchTaskService extends task.WorkbenchTaskService {}
