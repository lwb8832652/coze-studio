namespace go workbench.chat

include "../base.thrift"
include "./task.thrift"
include "./thread.thrift"

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

struct RuntimeDoctorModelCapabilities {
    1: required bool native_tool_search
    2: required bool thinking
    3: required bool reasoning
    4: required bool vision
    5: required bool pdf
    6: required bool file
    7: required bool audio
    8: required bool video
}

struct RuntimeDoctorModelData {
    1: required string status
    2: required bool configured
    3: required string live_probe
    4: optional RuntimeDoctorModelCapabilities capabilities
    5: optional string message
}

struct RuntimeDoctorSandboxScopeData {
    1: required string scope
    2: required bool configured
    3: required bool available
    4: required bool selected
    5: required string health_status
    6: required string reason_code
    7: optional string provider_type
    8: optional string provider_ref
    9: optional string checked_at
}

struct RuntimeDoctorSandboxData {
    1: required string status
    2: required string runner_type
    3: required string network
    4: required string process
    5: required string ffi
    6: required string node_modules
    7: optional string message
    8: optional list<RuntimeDoctorSandboxScopeData> scopes
}

struct WorkbenchRuntimeDoctorData {
    1: required string status
    2: required RuntimeDoctorRuntimeData runtime
    3: required RuntimeDoctorWebToolsData web_tools
    4: required RuntimeDoctorMCPToolsData mcp_tools
    5: required list<RuntimeDoctorCheck> checks
    6: required RuntimeDoctorModelData model
    7: required RuntimeDoctorSandboxData sandbox
}

struct WorkbenchRuntimeDoctorResponse {
    1: optional WorkbenchRuntimeDoctorData data
    253: required i64 code
    254: required string msg
    255: optional base.BaseResp BaseResp (api.none="true")
}

service WorkbenchChatService {
    WorkbenchRuntimeDoctorResponse GetWorkbenchRuntimeDoctor(1: GetWorkbenchRuntimeDoctorRequest request)(
        api.get="/api/workbench/runtime_doctor",
        api.category="workbench"
    )
}

// Re-export through workbench to avoid the task include alias collision in idl/api.thrift.
service WorkbenchTaskService extends task.WorkbenchTaskService {}
service WorkbenchCanonicalThreadService extends thread.WorkbenchCanonicalThreadService {}
