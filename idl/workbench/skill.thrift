namespace go workbench.skill

include "../base.thrift"

enum SkillType {
    Script = 1,
    Workflow = 2,
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

struct GetSkillRequest {
    1: required i64 skill_id (agw.js_conv="str", api.js_conv="true")
    255: optional base.Base Base (api.none="true")
}

struct TestRunSkillRequest {
    1: required i64 skill_id (agw.js_conv="str", api.js_conv="true")
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

service WorkbenchSkillService {
    SkillResponse CreateSkill(1: UpsertSkillRequest request)(
        api.post="/api/workbench/skills",
        api.category="workbench"
    )
    SkillResponse UpdateSkill(1: UpsertSkillRequest request)(
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
    SkillResponse GetSkill(1: GetSkillRequest request)(
        api.get="/api/workbench/skills/:skill_id",
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
