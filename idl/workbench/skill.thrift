namespace go workbench.skill

include "../base.thrift"

enum SkillType {
    Script = 1,
    Workflow = 2,
    DeerSkill = 3,
    PublicSkill = 4,
    CustomSkill = 5,
}

struct Skill {
    1: required i64 id (agw.js_conv="str", api.js_conv="true")
    2: required i64 space_id (agw.js_conv="str", api.js_conv="true")
    3: required string name
    4: required string description
    5: required SkillType type
    6: required string version
    7: required bool enabled
    8: required string input_schema
    9: required string output_schema
    10: required string executor
    11: required string permissions
    12: required i64 created_at
    13: required i64 updated_at
}

struct UpsertSkillRequest {
    1: optional i64 id (agw.js_conv="str", api.js_conv="true")
    2: required i64 space_id (agw.js_conv="str", api.js_conv="true")
    3: required string name
    4: required string description
    5: required SkillType type
    6: required string version
    7: required bool enabled
    8: required string input_schema
    9: required string output_schema
    10: required string executor
    11: required string permissions
    255: optional base.Base Base (api.none="true")
}

struct UpdateSkillRequest {
    1: required i64 id (api.path="id", agw.js_conv="str", api.js_conv="true")
    2: required i64 space_id (agw.js_conv="str", api.js_conv="true")
    3: required string name
    4: required string description
    5: required SkillType type
    6: required string version
    7: required bool enabled
    8: required string input_schema
    9: required string output_schema
    10: required string executor
    11: required string permissions
    255: optional base.Base Base (api.none="true")
}

struct ImportSkillRequest {
    1: required i64 space_id (agw.js_conv="str", api.js_conv="true")
    2: required string file_name
    3: required string content
    255: optional base.Base Base (api.none="true")
}

struct SkillResponse {
    1: optional Skill data
    253: required i64 code
    254: required string msg
    255: optional base.BaseResp BaseResp (api.none="true")
}

struct ListSkillsRequest {
    1: required i64 space_id (agw.js_conv="str", api.js_conv="true")
    2: optional SkillType type
    3: optional bool enabled
    255: optional base.Base Base (api.none="true")
}

struct ListSkillsData {
    1: required list<Skill> skills
}

struct ListSkillsResponse {
    1: optional ListSkillsData data
    253: required i64 code
    254: required string msg
    255: optional base.BaseResp BaseResp (api.none="true")
}

struct SkillVersion {
    1: required i64 id (agw.js_conv="str", api.js_conv="true")
    2: required i64 skill_id (agw.js_conv="str", api.js_conv="true")
    3: required string version
    4: required string skill_md
    5: required string input_schema
    6: required string output_schema
    7: required string executor
    8: required string permissions
    9: required i64 created_at
}

struct ListSkillVersionsRequest {
    1: required i64 skill_id (api.path="skill_id", agw.js_conv="str", api.js_conv="true")
    255: optional base.Base Base (api.none="true")
}

struct ListSkillVersionsData {
    1: required list<SkillVersion> versions
}

struct ListSkillVersionsResponse {
    1: optional ListSkillVersionsData data
    253: required i64 code
    254: required string msg
    255: optional base.BaseResp BaseResp (api.none="true")
}

struct SkillVersionResponse {
    1: optional SkillVersion data
    253: required i64 code
    254: required string msg
    255: optional base.BaseResp BaseResp (api.none="true")
}

struct SkillResource {
    1: required i64 id (agw.js_conv="str", api.js_conv="true")
    2: required i64 skill_id (agw.js_conv="str", api.js_conv="true")
    3: required i64 version_id (agw.js_conv="str", api.js_conv="true")
    4: required string path
    5: required string content_base64
    6: required i64 size
    7: required string sha256
    8: required i64 created_at
}

struct ListSkillVersionResourcesRequest {
    1: required i64 skill_id (api.path="skill_id", agw.js_conv="str", api.js_conv="true")
    2: required i64 version_id (api.path="version_id", agw.js_conv="str", api.js_conv="true")
    255: optional base.Base Base (api.none="true")
}

struct ListSkillVersionResourcesData {
    1: required list<SkillResource> resources
}

struct ListSkillVersionResourcesResponse {
    1: optional ListSkillVersionResourcesData data
    253: required i64 code
    254: required string msg
    255: optional base.BaseResp BaseResp (api.none="true")
}

struct UpdateSkillVersionResourceRequest {
    1: required i64 skill_id (api.path="skill_id", agw.js_conv="str", api.js_conv="true")
    2: required i64 version_id (api.path="version_id", agw.js_conv="str", api.js_conv="true")
    3: required string path
    4: required string content_base64
    255: optional base.Base Base (api.none="true")
}

struct UpdateSkillVersionContentRequest {
    1: required i64 skill_id (api.path="skill_id", agw.js_conv="str", api.js_conv="true")
    2: required i64 version_id (api.path="version_id", agw.js_conv="str", api.js_conv="true")
    3: required string skill_md
    255: optional base.Base Base (api.none="true")
}

struct ExportSkillVersionRequest {
    1: required i64 skill_id (api.path="skill_id", agw.js_conv="str", api.js_conv="true")
    2: required i64 version_id (api.path="version_id", agw.js_conv="str", api.js_conv="true")
    255: optional base.Base Base (api.none="true")
}

struct ExportSkillVersionData {
    1: required string file_name
    2: required string content_base64
    3: required string content_type
}

struct ExportSkillVersionResponse {
    1: optional ExportSkillVersionData data
    253: required i64 code
    254: required string msg
    255: optional base.BaseResp BaseResp (api.none="true")
}

struct RollbackSkillVersionRequest {
    1: required i64 skill_id (api.path="skill_id", agw.js_conv="str", api.js_conv="true")
    2: required i64 version_id (api.path="version_id", agw.js_conv="str", api.js_conv="true")
    255: optional base.Base Base (api.none="true")
}

struct GetSkillRequest {
    1: required i64 skill_id (api.path="skill_id", agw.js_conv="str", api.js_conv="true")
    255: optional base.Base Base (api.none="true")
}

struct TestRunSkillRequest {
    1: required i64 skill_id (api.path="skill_id", agw.js_conv="str", api.js_conv="true")
    2: required string input
    255: optional base.Base Base (api.none="true")
}

struct TestRunSkillData {
    1: optional string output
    2: optional i64 task_id (agw.js_conv="str", api.js_conv="true")
}

struct TestRunSkillResponse {
    1: optional TestRunSkillData data
    253: required i64 code
    254: required string msg
    255: optional base.BaseResp BaseResp (api.none="true")
}

struct ExportSkillData {
    1: required string file_name
    2: required string content
}

struct ExportSkillResponse {
    1: optional ExportSkillData data
    253: required i64 code
    254: required string msg
    255: optional base.BaseResp BaseResp (api.none="true")
}

struct ListSkillToolCandidatesRequest {
    1: required i64 space_id (agw.js_conv="str", api.js_conv="true")
    255: optional base.Base Base (api.none="true")
}

struct SkillToolCandidate {
    1: required string name
    2: required string display_name
    3: required string description
    4: required string category
    5: required string visibility
    6: optional string source
    7: optional string source_id
    8: optional string source_name
}

struct ListSkillToolCandidatesData {
    1: required list<SkillToolCandidate> tools
}

struct ListSkillToolCandidatesResponse {
    1: optional ListSkillToolCandidatesData data
    253: required i64 code
    254: required string msg
    255: optional base.BaseResp BaseResp (api.none="true")
}

service WorkbenchSkillService {
    SkillResponse CreateSkill(1: UpsertSkillRequest request)(
        api.post="/api/workbench/skills",
        api.category="workbench"
    )
    SkillResponse UpdateSkill(1: UpdateSkillRequest request)(
        api.put="/api/workbench/skills/:id",
        api.category="workbench"
    )
    SkillResponse ImportSkill(1: ImportSkillRequest request)(
        api.post="/api/workbench/skills/import",
        api.category="workbench"
    )
    ListSkillsResponse ListSkills(1: ListSkillsRequest request)(
        api.get="/api/workbench/skills",
        api.category="workbench"
    )
    ListSkillToolCandidatesResponse ListSkillToolCandidates(1: ListSkillToolCandidatesRequest request)(
        api.get="/api/workbench/skills/tool_candidates",
        api.category="workbench"
    )
    SkillResponse GetSkill(1: GetSkillRequest request)(
        api.get="/api/workbench/skills/:skill_id",
        api.category="workbench"
    )
    SkillResponse DeleteSkill(1: GetSkillRequest request)(
        api.delete="/api/workbench/skills/:skill_id",
        api.category="workbench"
    )
    ListSkillVersionsResponse ListSkillVersions(1: ListSkillVersionsRequest request)(
        api.get="/api/workbench/skills/:skill_id/versions",
        api.category="workbench"
    )
    ListSkillVersionResourcesResponse ListSkillVersionResources(1: ListSkillVersionResourcesRequest request)(
        api.get="/api/workbench/skills/:skill_id/versions/:version_id/resources",
        api.category="workbench"
    )
    SkillVersionResponse UpdateSkillVersionResource(1: UpdateSkillVersionResourceRequest request)(
        api.put="/api/workbench/skills/:skill_id/versions/:version_id/resources",
        api.category="workbench"
    )
    SkillVersionResponse UpdateSkillVersionContent(1: UpdateSkillVersionContentRequest request)(
        api.put="/api/workbench/skills/:skill_id/versions/:version_id/content",
        api.category="workbench"
    )
    ExportSkillVersionResponse ExportSkillVersion(1: ExportSkillVersionRequest request)(
        api.get="/api/workbench/skills/:skill_id/versions/:version_id/export",
        api.category="workbench"
    )
    SkillResponse RollbackSkillVersion(1: RollbackSkillVersionRequest request)(
        api.post="/api/workbench/skills/:skill_id/versions/:version_id/rollback",
        api.category="workbench"
    )
    ExportSkillResponse ExportSkill(1: GetSkillRequest request)(
        api.get="/api/workbench/skills/:skill_id/export",
        api.category="workbench"
    )
    TestRunSkillResponse TestRunSkill(1: TestRunSkillRequest request)(
        api.post="/api/workbench/skills/:skill_id/test_run",
        api.category="workbench"
    )
}
