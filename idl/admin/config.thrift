include "../base.thrift"
include "../app/developer_api.thrift"

namespace go admin.config


 struct GetModelListReq  {
    1: optional string keyword
    2: optional string provider_key
    3: optional string capability_type
    4: optional bool enabled
    5: optional string access_mode
    6: optional i32 page
    7: optional i32 page_size
    255: optional base.Base Base
 }

 struct GetModelListResp {
     1: list<ProviderModelList> provider_model_list
     2: optional list<ModelManagementItem> models
     3: optional i64 total

     253: required i64 code
    254: required string msg
    255: required base.BaseResp BaseResp(api.none="true")
}

struct ProviderModelList {
    1: ModelProvider provider
    2: list<Model> model_list
}

struct I18nText {
    1: string zh_cn
    2: string en_us
}

struct ModelProvider {
    1: I18nText name
    2: string icon_uri
    3: string icon_url
    4: I18nText description
    5: developer_api.ModelClass model_class
}

struct DisplayInfo {
    1: string name
    3: I18nText description
    4: i64 output_tokens
    5: i64 max_tokens
}



enum ModelType {
    LLM = 0 
    TextEmbedding = 1
    Rerank = 2
}

struct Model {
    1: i64 id
    2: ModelProvider provider
    3: DisplayInfo display_info
    4: developer_api.ModelAbility capability  
    5: Connection  connection
    6: ModelType type
    7: list<developer_api.ModelParameter> parameters
    8: ModelStatus status
    9: bool enable_base64_url
    10: i64 delete_at_ms
}

enum ThinkingType {
    Default = 0 
    Enable = 1
    Disable = 2
    Auto = 3
}


 enum ModelStatus {
    StatusDefault = 0  // Default state when not configured, equivalent to StatusInUse
    StatusInUse   = 1  // In the application, it can be used to create new
    StatusDeleted = 2 // It is offline, unusable, and cannot be created.
 }

 enum ModelAccessMode {
     ALL = 1
     RESTRICTED = 2
 }

 enum ModelGrantSubjectType {
     WORKSPACE = 1
     USER = 2
 }

 enum ModelRoutingStrategy {
     ROUND_ROBIN = 1
     WEIGHTED_ROUND_ROBIN = 2
 }

 struct ModelProviderOption {
     1: required string provider_key
     2: required I18nText name
     3: required developer_api.ModelClass model_class
     4: required string protocol
     5: required bool supports_custom_base_url
     6: required bool supports_function_call
     7: required bool supports_multimodal
     8: optional string default_base_url
 }

 struct ListModelProvidersReq {
     255: optional base.Base Base
 }

 struct ListModelProvidersResp {
     1: required list<ModelProviderOption> providers
     253: required i64 code
     254: required string msg
     255: required base.BaseResp BaseResp(api.none="true")
 }

 struct ModelEndpointInput {
     1: optional i64 id (agw.js_conv="str", api.js_conv="true")
     2: required string base_url
     3: optional string api_key
     4: optional bool clear_api_key
     5: required i32 weight
     6: required bool enabled
     7: required i32 sort_order
 }

 struct ModelEndpointView {
     1: required i64 id (agw.js_conv="str", api.js_conv="true")
     2: required string base_url
     3: required bool has_api_key
     4: required i32 weight
     5: required bool enabled
     6: required i32 sort_order
 }

 struct ModelManagementInput {
     1: required string provider_key
     2: required string name
     3: required string model_identifier
     4: optional string description
     5: required list<string> capability_types
     6: required string reasoning_mode
     7: required i64 max_context_tokens
     8: required i64 max_output_tokens
     9: required string function_call_mode
     10: required bool enabled
     11: required list<string> usage_scenarios
     12: required string protocol
     13: required ModelRoutingStrategy routing_strategy
     14: required ModelAccessMode access_mode
     15: required list<ModelEndpointInput> endpoints
     16: optional bool enable_base64_url
 }

 struct ModelManagementItem {
     1: required i64 id (agw.js_conv="str", api.js_conv="true")
     2: required string provider_key
     3: required developer_api.ModelClass model_class
     4: required string name
     5: required string model_identifier
     6: optional string description
     7: required list<string> capability_types
     8: required bool enabled
     9: required ModelAccessMode access_mode
     10: required i64 creator_id (agw.js_conv="str", api.js_conv="true")
     11: optional string creator_name
     12: required i64 updated_at_ms
     13: required i64 sort_order
 }

 struct ModelDetail {
     1: required ModelManagementItem summary
     2: required string reasoning_mode
     3: required i64 max_context_tokens
     4: required i64 max_output_tokens
     5: required string function_call_mode
     6: required list<string> usage_scenarios
     7: required string protocol
     8: required ModelRoutingStrategy routing_strategy
     9: required list<ModelEndpointView> endpoints
     10: required bool enable_base64_url
 }

 struct GetModelDetailReq {
     1: required i64 id (agw.js_conv="str", api.js_conv="true")
     255: optional base.Base Base
 }

 struct GetModelDetailResp {
     1: required ModelDetail model
     253: required i64 code
     254: required string msg
     255: required base.BaseResp BaseResp(api.none="true")
 }

 struct ModelGrantSubject {
     1: required ModelGrantSubjectType subject_type
     2: required i64 subject_id (agw.js_conv="str", api.js_conv="true")
     3: optional string name
     4: optional string description
 }

 struct GetModelGrantsReq {
     1: required i64 model_id (agw.js_conv="str", api.js_conv="true")
     255: optional base.Base Base
 }

 struct GetModelGrantsResp {
     1: required ModelAccessMode access_mode
     2: required list<ModelGrantSubject> grants
     253: required i64 code
     254: required string msg
     255: required base.BaseResp BaseResp(api.none="true")
 }

 struct SaveModelGrantsReq {
     1: required i64 model_id (agw.js_conv="str", api.js_conv="true")
     2: required ModelAccessMode access_mode
     3: required list<ModelGrantSubject> grants
     255: optional base.Base Base
 }

 struct SaveModelGrantsResp {
     253: required i64 code
     254: required string msg
     255: required base.BaseResp BaseResp(api.none="true")
 }

 struct TestModelEndpointReq {
     1: optional i64 model_id (agw.js_conv="str", api.js_conv="true")
     2: required ModelEndpointInput endpoint
     3: required string provider_key
     4: required string model_identifier
     5: required string protocol
     255: optional base.Base Base
 }

 struct TestModelEndpointResp {
     1: required bool success
     2: required i64 latency_ms
     3: optional string error_code
     4: optional string error_message
     253: required i64 code
     254: required string msg
     255: required base.BaseResp BaseResp(api.none="true")
 }

 struct UpdateModelStatusReq {
     1: required i64 id (agw.js_conv="str", api.js_conv="true")
     2: required bool enabled
     255: optional base.Base Base
 }

 struct UpdateModelStatusResp {
     253: required i64 code
     254: required string msg
     255: required base.BaseResp BaseResp(api.none="true")
 }

 struct ModelSortItem {
     1: required i64 id (agw.js_conv="str", api.js_conv="true")
     2: required i64 sort_order
 }

 struct UpdateModelSortReq {
     1: required list<ModelSortItem> items
     255: optional base.Base Base
 }

 struct UpdateModelSortResp {
     253: required i64 code
     254: required string msg
     255: required base.BaseResp BaseResp(api.none="true")
 }

 struct ModelDependencySample {
     1: required string id
     2: required string name
 }

 struct ModelDependencySummary {
     1: required string dependency_type
     2: required i64 count
     3: required list<ModelDependencySample> samples
 }

struct Connection {
    1: BaseConnectionInfo base_conn_info
    2: optional ArkConnInfo ark
    3: optional OpenAIConnInfo openai
    4: optional DeepseekConnInfo deepseek
    5: optional GeminiConnInfo gemini
    6: optional QwenConnInfo qwen
    7: optional OllamaConnInfo ollama
    8: optional ClaudeConnInfo claude
}

struct BaseConnectionInfo {
    1: string base_url
    2: string api_key
    3: string model
    4: ThinkingType thinking_type
}

struct EmbeddingInfo {
    1: i32 dims
}


struct ArkConnInfo {
    1: string region
    3: string api_type
}

struct OpenAIConnInfo {
    6: bool by_azure
    7: string api_version
}


struct GeminiConnInfo {
    1: i32 backend // "1" for BackendGeminiAPI / "2" for BackendVertexAI
    2: string project
    3: string location
}

struct DeepseekConnInfo {}

struct QwenConnInfo {}

struct OllamaConnInfo {}

struct ClaudeConnInfo {}

 struct CreateModelReq {
     1: developer_api.ModelClass model_class
     2: string model_name
     3: Connection connection
     4: bool enable_base64_url
     5: optional ModelManagementInput management


    255: optional base.Base Base
}

struct CreateModelResp {
    1: i64 id (agw.js_conv="str", api.js_conv="true")

    253: required i64 code
    254: required string msg
    255: required base.BaseResp BaseResp(api.none="true")
}

 struct DeleteModelReq {
     1: i64 id (agw.js_conv="str", api.js_conv="true")
     2: optional bool preview
     255: optional base.Base Base
 }

 struct DeleteModelResp {
     1: optional list<ModelDependencySummary> dependencies
     253: required i64 code
    254: required string msg
    255: required base.BaseResp BaseResp(api.none="true")
}

 struct UpdateModelReq {
     1: optional Model model
     2: optional i64 id (agw.js_conv="str", api.js_conv="true")
     3: optional ModelManagementInput management
     255: optional base.Base Base
 }

struct UpdateModelResp {
    253: required i64 code
    254: required string msg
    255: required base.BaseResp BaseResp(api.none="true")
}


struct SaveBasicConfigurationReq {
    1: BasicConfiguration configuration
    255: optional base.Base Base
}

struct SaveBasicConfigurationResp {
    253: required i64 code
    254: required string msg
    255: required base.BaseResp BaseResp(api.none="true")
}

struct GetBasicConfigurationReq {
    255: optional base.Base Base
}

struct GetBasicConfigurationResp {
    1: BasicConfiguration configuration

    253: required i64 code
    254: required string msg
    255: required base.BaseResp BaseResp(api.none="true")
}

enum CodeRunnerType {
    Local = 0 
    Sandbox = 1
}

struct SandboxConfig {
    1: string allow_env
    2: string allow_read
    3: string allow_write
    4: string allow_run
    5: string allow_net
    6: string allow_ffi
    7: string node_modules_dir
    8: double timeout_seconds
    9: i64 memory_limit_mb
}

struct BasicConfiguration {
    1: string admin_emails
    2: bool disable_user_registration
    3: string allow_registration_email
    4: PluginConfiguration plugin_configuration
    5: CodeRunnerType code_runner_type
    6: optional SandboxConfig sandbox_config
    7: string server_host
    8: optional string site_name
    9: optional string site_description
    10: optional string site_logo_uri
    11: optional string favicon_uri
}

struct PluginConfiguration {
    1: bool coze_saas_plugin_enabled
    2: string coze_api_token
    3: string coze_saas_api_base_url
}

struct UpdateKnowledgeConfigReq {
    1: KnowledgeConfig knowledge_config

    255: optional base.Base Base
}

struct UpdateKnowledgeConfigResp {
    253: required i64 code
    254: required string msg
    255: required base.BaseResp BaseResp(api.none="true")
}



struct GetKnowledgeConfigReq {
    255: optional base.Base Base
}

struct GetKnowledgeConfigResp {
    1: KnowledgeConfig knowledge_config

    253: required i64 code
    254: required string msg
    255: required base.BaseResp BaseResp(api.none="true")
}

struct KnowledgeConfig {
    1: EmbeddingConfig embedding_config
    2: RerankConfig rerank_config
    3: OCRConfig ocr_config
    4: ParserConfig parser_config
    5: i64 builtin_model_id
}

struct EmbeddingConfig {
    1: EmbeddingType type
    2: i32 max_batch_size
    3: EmbeddingConnection connection
}

enum EmbeddingType {
    Ark = 0 
    OpenAI = 1
    Ollama = 2
    Gemini = 3
    HTTP = 4
}

struct EmbeddingConnection {
    1: BaseConnectionInfo base_conn_info
    2: EmbeddingInfo embedding_info
    3: optional ArkConnInfo ark
    4: optional OpenAIConnInfo openai
    5: optional OllamaConnInfo ollama
    6: optional GeminiConnInfo gemini
    7: optional HttpConnection http
}

struct HttpConnection {
    1: string address
}

enum RerankType {
    VikingDB = 0 
    RRF = 1
}


struct RerankConfig {
    1: RerankType type
    2: VikingDBConfig vikingdb_config
}

struct VikingDBConfig {
    1: string ak
    2: string sk
    3: string host
    4: string region
    5: string model
}

enum OCRType {
    Volcengine = 0 
    Paddleocr = 1
}

struct OCRConfig {
    1: OCRType type
    2: string volcengine_ak
    3: string volcengine_sk
    4: string paddleocr_api_url
}

enum ParserType {
   builtin = 0 
   Paddleocr = 1
}

struct ParserConfig {
    1: ParserType type
    2: string paddleocr_structure_api_url
}



enum ObjectStorageProviderType {
    QINIU = 1
    ALIYUN_OSS = 2
    TENCENT_COS = 3
    HUAWEI_OBS = 4
    AWS_S3 = 5
    MINIO = 6
    TOS = 7
}

enum ObjectStorageHealthStatus {
    UNKNOWN = 1
    HEALTHY = 2
    UNHEALTHY = 3
}

enum ObjectStorageRuntimeSource {
    DATABASE = 1
    ENV_RESCUE = 2
}

struct ObjectStoragePublicConfig {
    1: optional string bucket
    2: optional string region
    3: optional string endpoint
    4: optional string endpoint_override
    5: optional bool force_path_style
    6: optional bool use_ssl
    7: optional string download_domain
    8: optional bool use_https
}

struct ObjectStorageCredentialInput {
    1: optional string access_key_id
    2: optional string secret_access_key
}

struct ObjectStorageHealthView {
    1: ObjectStorageHealthStatus status
    2: optional string code
    3: optional string message
    4: optional i64 latency_ms
    5: optional string checked_at
}

struct ObjectStorageConfigView {
    1: i64 id (api.js_conv='true', agw.js_conv='str')
    2: string name
    3: ObjectStorageProviderType provider_type
    4: ObjectStoragePublicConfig config
    5: bool credential_configured
    6: ObjectStorageHealthView health
    7: bool desired_active
    8: bool runtime_active
    9: bool restart_required
    10: i64 version (api.js_conv='true', agw.js_conv='str')
    11: i64 runtime_revision (api.js_conv='true', agw.js_conv='str')
    12: string created_at
    13: string updated_at
}

struct ListObjectStorageConfigsReq {}
struct ListObjectStorageConfigsResp {
    1: list<ObjectStorageConfigView> configs
    2: ObjectStorageRuntimeSource runtime_source
    3: bool restart_required

    253: required i64 code
    254: required string msg
    255: required base.BaseResp BaseResp (api.none="true")
}

struct CreateObjectStorageConfigReq {
    1: string name
    2: ObjectStorageProviderType provider_type
    3: ObjectStoragePublicConfig config
    4: ObjectStorageCredentialInput credential
}
struct CreateObjectStorageConfigResp {
    1: ObjectStorageConfigView config

    253: required i64 code
    254: required string msg
    255: required base.BaseResp BaseResp (api.none="true")
}

struct UpdateObjectStorageConfigReq {
    1: i64 id (api.js_conv='true', agw.js_conv='str')
    2: i64 expected_version (api.js_conv='true', agw.js_conv='str')
    3: string name
    4: ObjectStoragePublicConfig config
    5: optional ObjectStorageCredentialInput credential
}
struct UpdateObjectStorageConfigResp {
    1: ObjectStorageConfigView config

    253: required i64 code
    254: required string msg
    255: required base.BaseResp BaseResp (api.none="true")
}

struct TestObjectStorageConfigReq {
    1: optional i64 id (api.js_conv='true', agw.js_conv='str')
    2: optional i64 expected_version (api.js_conv='true', agw.js_conv='str')
    3: ObjectStorageProviderType provider_type
    4: ObjectStoragePublicConfig config
    5: optional ObjectStorageCredentialInput credential
}
struct TestObjectStorageConfigResp {
    1: bool success
    2: ObjectStorageHealthView health

    253: required i64 code
    254: required string msg
    255: required base.BaseResp BaseResp (api.none="true")
}

struct ActivateObjectStorageConfigReq {
    1: i64 id (api.js_conv='true', agw.js_conv='str')
    2: i64 expected_version (api.js_conv='true', agw.js_conv='str')
    3: bool migration_confirmed
}
struct ActivateObjectStorageConfigResp {
    1: ObjectStorageConfigView config

    253: required i64 code
    254: required string msg
    255: required base.BaseResp BaseResp (api.none="true")
}

struct DeleteObjectStorageConfigReq {
    1: i64 id (api.js_conv='true', agw.js_conv='str')
    2: i64 expected_version (api.js_conv='true', agw.js_conv='str')
}
struct DeleteObjectStorageConfigResp {
    253: required i64 code
    254: required string msg
    255: required base.BaseResp BaseResp (api.none="true")
}

 service ConfigService {
    GetBasicConfigurationResp GetBasicConfiguration(1:GetBasicConfigurationReq req)(api.get='/api/admin/config/basic/get', api.category="admin")
    SaveBasicConfigurationResp SaveBasicConfiguration(1:SaveBasicConfigurationReq req)(api.post='/api/admin/config/basic/save', api.category="admin")
    GetKnowledgeConfigResp GetKnowledgeConfig(1:GetKnowledgeConfigReq req)(api.get='/api/admin/config/knowledge/get', api.category="admin")
    UpdateKnowledgeConfigResp UpdateKnowledgeConfig(1:UpdateKnowledgeConfigReq req)(api.post='/api/admin/config/knowledge/save', api.category="admin")
     GetModelListResp GetModelList(1:GetModelListReq req)(api.get='/api/admin/config/model/list', api.category="admin")
     ListModelProvidersResp ListModelProviders(1:ListModelProvidersReq req)(api.get='/api/admin/config/model/providers', api.category="admin")
     GetModelListResp ListModels(1:GetModelListReq req)(api.get='/api/admin/config/model/manage/list', api.category="admin")
     GetModelDetailResp GetModelDetail(1:GetModelDetailReq req)(api.get='/api/admin/config/model/detail', api.category="admin")
     CreateModelResp CreateModel(1:CreateModelReq req)(api.post='/api/admin/config/model/create', api.category="admin")
     UpdateModelResp UpdateModel(1:UpdateModelReq req)(api.post='/api/admin/config/model/update', api.category="admin")
     TestModelEndpointResp TestModelEndpoint(1:TestModelEndpointReq req)(api.post='/api/admin/config/model/test', api.category="admin")
     UpdateModelStatusResp UpdateModelStatus(1:UpdateModelStatusReq req)(api.post='/api/admin/config/model/status', api.category="admin")
     UpdateModelSortResp UpdateModelSort(1:UpdateModelSortReq req)(api.post='/api/admin/config/model/sort', api.category="admin")
     GetModelGrantsResp GetModelGrants(1:GetModelGrantsReq req)(api.get='/api/admin/config/model/grants', api.category="admin")
     SaveModelGrantsResp SaveModelGrants(1:SaveModelGrantsReq req)(api.post='/api/admin/config/model/grants', api.category="admin")
     DeleteModelResp DeleteModel(1:DeleteModelReq req)(api.post='/api/admin/config/model/delete', api.category="admin")
    ListObjectStorageConfigsResp ListObjectStorageConfigs(1:ListObjectStorageConfigsReq req)(api.get='/api/admin/config/object-storage/list', api.category="admin")
    CreateObjectStorageConfigResp CreateObjectStorageConfig(1:CreateObjectStorageConfigReq req)(api.post='/api/admin/config/object-storage/create', api.category="admin")
    UpdateObjectStorageConfigResp UpdateObjectStorageConfig(1:UpdateObjectStorageConfigReq req)(api.post='/api/admin/config/object-storage/update', api.category="admin")
    TestObjectStorageConfigResp TestObjectStorageConfig(1:TestObjectStorageConfigReq req)(api.post='/api/admin/config/object-storage/test', api.category="admin")
    ActivateObjectStorageConfigResp ActivateObjectStorageConfig(1:ActivateObjectStorageConfigReq req)(api.post='/api/admin/config/object-storage/activate', api.category="admin")
    DeleteObjectStorageConfigResp DeleteObjectStorageConfig(1:DeleteObjectStorageConfigReq req)(api.post='/api/admin/config/object-storage/delete', api.category="admin")
 }
