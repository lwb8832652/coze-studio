namespace go workbench.model

include "../base.thrift"

enum WorkspaceModelScope {
    System = 1,
    Space = 2,
}

struct WorkspaceModelProviderOption {
    1: required string key
    2: required string name
    3: required i32 model_class
    4: optional string icon_url
}

struct WorkspaceModelEndpointInput {
    1: optional i64 id (agw.js_conv="str", api.js_conv="true")
    2: required string base_url
    3: optional string api_key
    4: optional i32 weight = 1
    5: optional bool enabled = true
}

struct WorkspaceModelEndpointView {
    1: required i64 id (agw.js_conv="str", api.js_conv="true")
    2: required string base_url
    3: required bool credential_configured
    4: required i32 weight
    5: required bool enabled
}

struct WorkspaceModel {
    1: required i64 id (agw.js_conv="str", api.js_conv="true")
    2: required WorkspaceModelScope scope
    3: required string provider_key
    4: required i32 model_class
    5: required string display_name
    6: required string model_identifier
    7: required bool enabled
    8: required bool credential_configured
    9: required bool can_manage
    10: optional string description
    11: optional string protocol
    12: optional list<string> capabilities
    13: optional list<string> usage_scenarios
    14: optional i64 max_context_tokens
    15: optional i64 max_output_tokens
    16: optional string function_call_mode
    17: optional list<WorkspaceModelEndpointView> endpoints
    18: optional i64 creator_id (agw.js_conv="str", api.js_conv="true")
    19: optional i64 updated_at
}

struct WorkspaceModelDraft {
    1: required string provider_key
    2: required i32 model_class
    3: required string display_name
    4: required string model_identifier
    5: optional string description
    6: required string protocol
    7: required list<WorkspaceModelEndpointInput> endpoints
    8: optional list<string> capabilities
    9: optional list<string> usage_scenarios
    10: optional i64 max_context_tokens
    11: optional i64 max_output_tokens
    12: optional string function_call_mode
    13: optional bool enabled = true
}

struct ListWorkspaceModelsRequest {
    1: required i64 space_id (agw.js_conv="str", api.js_conv="true")
    2: optional WorkspaceModelScope scope
    3: optional string keyword
    255: optional base.Base Base (api.none="true")
}

struct ListWorkspaceModelsData {
    1: required list<WorkspaceModel> system_models
    2: required list<WorkspaceModel> workspace_models
    3: required list<WorkspaceModelProviderOption> providers
    4: required bool can_manage
}

struct ListWorkspaceModelsResponse {
    1: optional ListWorkspaceModelsData data
    253: required i64 code
    254: required string msg
    255: optional base.BaseResp BaseResp (api.none="true")
}

struct GetWorkspaceModelRequest {
    1: required i64 model_id (api.path="model_id", agw.js_conv="str", api.js_conv="true")
    2: required i64 space_id (agw.js_conv="str", api.js_conv="true")
    255: optional base.Base Base (api.none="true")
}

struct WorkspaceModelResponse {
    1: optional WorkspaceModel data
    253: required i64 code
    254: required string msg
    255: optional base.BaseResp BaseResp (api.none="true")
}

struct UpsertWorkspaceModelRequest {
    1: required i64 space_id (agw.js_conv="str", api.js_conv="true")
    2: optional i64 model_id (agw.js_conv="str", api.js_conv="true")
    3: required WorkspaceModelDraft model
    255: optional base.Base Base (api.none="true")
}

struct TestWorkspaceModelRequest {
    1: required i64 space_id (agw.js_conv="str", api.js_conv="true")
    2: optional i64 model_id (agw.js_conv="str", api.js_conv="true")
    3: required WorkspaceModelDraft model
    255: optional base.Base Base (api.none="true")
}

struct TestWorkspaceModelData {
    1: required bool success
    2: required i64 duration_ms
    3: optional string error_code
    4: optional string message
}

struct TestWorkspaceModelResponse {
    1: optional TestWorkspaceModelData data
    253: required i64 code
    254: required string msg
    255: optional base.BaseResp BaseResp (api.none="true")
}

struct SetWorkspaceModelStatusRequest {
    1: required i64 model_id (api.path="model_id", agw.js_conv="str", api.js_conv="true")
    2: required i64 space_id (agw.js_conv="str", api.js_conv="true")
    3: required bool enabled
    255: optional base.Base Base (api.none="true")
}

struct DeleteWorkspaceModelRequest {
    1: required i64 model_id (api.path="model_id", agw.js_conv="str", api.js_conv="true")
    2: required i64 space_id (agw.js_conv="str", api.js_conv="true")
    255: optional base.Base Base (api.none="true")
}

struct WorkspaceModelMutationResponse {
    253: required i64 code
    254: required string msg
    255: optional base.BaseResp BaseResp (api.none="true")
}

service WorkbenchModelService {
    ListWorkspaceModelsResponse ListWorkspaceModels(1: ListWorkspaceModelsRequest request)(
        api.get="/api/workbench/models",
        api.category="workbench"
    )
    WorkspaceModelResponse GetWorkspaceModel(1: GetWorkspaceModelRequest request)(
        api.get="/api/workbench/models/:model_id",
        api.category="workbench"
    )
    WorkspaceModelResponse UpsertWorkspaceModel(1: UpsertWorkspaceModelRequest request)(
        api.post="/api/workbench/models",
        api.category="workbench"
    )
    TestWorkspaceModelResponse TestWorkspaceModel(1: TestWorkspaceModelRequest request)(
        api.post="/api/workbench/models/test",
        api.category="workbench"
    )
    WorkspaceModelMutationResponse SetWorkspaceModelStatus(1: SetWorkspaceModelStatusRequest request)(
        api.post="/api/workbench/models/:model_id/status",
        api.category="workbench"
    )
    WorkspaceModelMutationResponse DeleteWorkspaceModel(1: DeleteWorkspaceModelRequest request)(
        api.delete="/api/workbench/models/:model_id",
        api.category="workbench"
    )
}
