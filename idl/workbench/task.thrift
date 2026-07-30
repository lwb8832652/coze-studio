namespace go workbench.task

include "../base.thrift"

enum ScheduledTaskTargetType {
    Agent = 1,
    Workflow = 2,
}

enum ScheduledTaskScheduleType {
    Once = 1,
    Hourly = 2,
    Daily = 3,
    Weekly = 4,
    Cron = 5,
}

enum ScheduledTaskStatus {
    Enabled = 1,
    Disabled = 2,
    Completed = 3,
}

enum ScheduledTaskExecutionStatus {
    Queued = 1,
    Running = 2,
    Succeeded = 3,
    Failed = 4,
    Canceled = 5,
}

struct ScheduledTask {
    1: required i64 id (agw.js_conv="str", api.js_conv="true")
    2: required i64 space_id (agw.js_conv="str", api.js_conv="true")
    3: required i64 creator_id (agw.js_conv="str", api.js_conv="true")
    4: required string name
    5: required ScheduledTaskTargetType target_type
    6: required i64 target_id (agw.js_conv="str", api.js_conv="true")
    7: required string target_name
    8: optional string target_icon_uri
    9: required ScheduledTaskScheduleType schedule_type
    10: optional string cron_expr
    11: required string timezone
    12: optional i64 run_once_at
    13: optional i32 minute
    14: optional i32 hour
    15: optional i32 weekday
    16: required string payload
    17: required bool keep_conversation
    18: required ScheduledTaskStatus status
    19: required i64 execution_count
    20: required i64 max_executions
    21: required i64 latest_execution_at
    22: required i64 next_execution_at
    23: required i64 created_at
    24: required i64 updated_at
    25: required i64 version
    26: optional ScheduledTaskExecutionStatus latest_execution_status
    27: optional string creator_name
}

struct ScheduledTaskExecution {
    1: required i64 id (agw.js_conv="str", api.js_conv="true")
    2: required i64 task_id (agw.js_conv="str", api.js_conv="true")
    3: required string trigger_type
    4: required i64 scheduled_at
    5: required ScheduledTaskExecutionStatus status
    6: required i32 attempt
    7: optional i64 thread_id (agw.js_conv="str", api.js_conv="true")
    8: optional i64 run_id (agw.js_conv="str", api.js_conv="true")
    9: optional i64 workflow_execution_id (agw.js_conv="str", api.js_conv="true")
    10: optional string error_code
    11: optional string error_message
    12: required i64 started_at
    13: required i64 finished_at
    14: required i64 created_at
}

struct ScheduledTaskTarget {
    1: required i64 id (agw.js_conv="str", api.js_conv="true")
    2: required ScheduledTaskTargetType type
    3: required string name
    4: optional string icon_uri
    5: required bool published
    6: optional string input_schema
}

struct ScheduledTaskCronPreset {
    1: required string id
    2: required string label
    3: required ScheduledTaskScheduleType schedule_type
    4: optional string cron_expr
}

struct CreateScheduledTaskRequest {
    1: required i64 space_id (agw.js_conv="str", api.js_conv="true")
    2: required string name
    3: required ScheduledTaskTargetType target_type
    4: required i64 target_id (agw.js_conv="str", api.js_conv="true")
    5: required ScheduledTaskScheduleType schedule_type
    6: optional string cron_expr
    7: required string timezone
    8: optional i64 run_once_at
    9: optional i32 minute
    10: optional i32 hour
    11: optional i32 weekday
    12: required string payload
    13: optional bool keep_conversation
    14: optional i64 max_executions
    255: optional base.Base Base (api.none="true")
}

struct UpdateScheduledTaskRequest {
    1: required i64 task_id (api.path="task_id", agw.js_conv="str", api.js_conv="true")
    2: required i64 space_id (agw.js_conv="str", api.js_conv="true")
    3: required string name
    4: required ScheduledTaskTargetType target_type
    5: required i64 target_id (agw.js_conv="str", api.js_conv="true")
    6: required ScheduledTaskScheduleType schedule_type
    7: optional string cron_expr
    8: required string timezone
    9: optional i64 run_once_at
    10: optional i32 minute
    11: optional i32 hour
    12: optional i32 weekday
    13: required string payload
    14: optional bool keep_conversation
    15: optional i64 max_executions
    16: required i64 version
    255: optional base.Base Base (api.none="true")
}

struct GetScheduledTaskRequest {
    1: required i64 task_id (api.path="task_id", agw.js_conv="str", api.js_conv="true")
    2: required i64 space_id (agw.js_conv="str", api.js_conv="true")
    255: optional base.Base Base (api.none="true")
}

struct ScheduledTaskActionRequest {
    1: required i64 task_id (api.path="task_id", agw.js_conv="str", api.js_conv="true")
    2: required i64 space_id (agw.js_conv="str", api.js_conv="true")
    255: optional base.Base Base (api.none="true")
}

struct ListScheduledTasksRequest {
    1: required i64 space_id (agw.js_conv="str", api.js_conv="true")
    2: optional ScheduledTaskTargetType target_type
    3: optional string keyword
    4: optional ScheduledTaskStatus status
    5: optional i32 page
    6: optional i32 page_size
    255: optional base.Base Base (api.none="true")
}

struct ListScheduledTaskExecutionsRequest {
    1: required i64 task_id (api.path="task_id", agw.js_conv="str", api.js_conv="true")
    2: required i64 space_id (agw.js_conv="str", api.js_conv="true")
    3: optional i32 page
    4: optional i32 page_size
    255: optional base.Base Base (api.none="true")
}

struct ListScheduledTaskTargetsRequest {
    1: required i64 space_id (agw.js_conv="str", api.js_conv="true")
    2: required ScheduledTaskTargetType target_type
    3: optional string keyword
    4: optional i32 page
    5: optional i32 page_size
    255: optional base.Base Base (api.none="true")
}

struct ListScheduledTaskCronPresetsRequest {
    1: optional string timezone
    255: optional base.Base Base (api.none="true")
}

struct ScheduledTaskResponse {
    1: optional ScheduledTask data
    253: required i64 code
    254: required string msg
    255: optional base.BaseResp BaseResp (api.none="true")
}

struct ScheduledTaskExecutionResponse {
    1: optional ScheduledTaskExecution data
    253: required i64 code
    254: required string msg
    255: optional base.BaseResp BaseResp (api.none="true")
}

struct ListScheduledTasksData {
    1: required list<ScheduledTask> tasks
    2: required i64 total
}

struct ListScheduledTasksResponse {
    1: optional ListScheduledTasksData data
    253: required i64 code
    254: required string msg
    255: optional base.BaseResp BaseResp (api.none="true")
}

struct ListScheduledTaskExecutionsData {
    1: required list<ScheduledTaskExecution> executions
    2: required i64 total
}

struct ListScheduledTaskExecutionsResponse {
    1: optional ListScheduledTaskExecutionsData data
    253: required i64 code
    254: required string msg
    255: optional base.BaseResp BaseResp (api.none="true")
}

struct ListScheduledTaskTargetsData {
    1: required list<ScheduledTaskTarget> targets
    2: required i64 total
}

struct ListScheduledTaskTargetsResponse {
    1: optional ListScheduledTaskTargetsData data
    253: required i64 code
    254: required string msg
    255: optional base.BaseResp BaseResp (api.none="true")
}

struct ListScheduledTaskCronPresetsResponse {
    1: required list<ScheduledTaskCronPreset> data
    253: required i64 code
    254: required string msg
    255: optional base.BaseResp BaseResp (api.none="true")
}

service WorkbenchTaskService {
    ScheduledTaskResponse CreateScheduledTask(1: CreateScheduledTaskRequest request)(
        api.post="/api/workbench/scheduled_tasks", api.category="workbench"
    )
    ListScheduledTasksResponse ListScheduledTasks(1: ListScheduledTasksRequest request)(
        api.get="/api/workbench/scheduled_tasks", api.category="workbench"
    )
    ListScheduledTaskTargetsResponse ListScheduledTaskTargets(1: ListScheduledTaskTargetsRequest request)(
        api.get="/api/workbench/scheduled_task_targets", api.category="workbench"
    )
    ListScheduledTaskCronPresetsResponse ListScheduledTaskCronPresets(1: ListScheduledTaskCronPresetsRequest request)(
        api.get="/api/workbench/scheduled_task_cron_presets", api.category="workbench"
    )
    ScheduledTaskResponse GetScheduledTask(1: GetScheduledTaskRequest request)(
        api.get="/api/workbench/scheduled_tasks/:task_id", api.category="workbench"
    )
    ScheduledTaskResponse UpdateScheduledTask(1: UpdateScheduledTaskRequest request)(
        api.put="/api/workbench/scheduled_tasks/:task_id", api.category="workbench"
    )
    ScheduledTaskResponse DeleteScheduledTask(1: ScheduledTaskActionRequest request)(
        api.delete="/api/workbench/scheduled_tasks/:task_id", api.category="workbench"
    )
    ScheduledTaskResponse EnableScheduledTask(1: ScheduledTaskActionRequest request)(
        api.post="/api/workbench/scheduled_tasks/:task_id/enable", api.category="workbench"
    )
    ScheduledTaskResponse DisableScheduledTask(1: ScheduledTaskActionRequest request)(
        api.post="/api/workbench/scheduled_tasks/:task_id/disable", api.category="workbench"
    )
    ScheduledTaskExecutionResponse ExecuteScheduledTask(1: ScheduledTaskActionRequest request)(
        api.post="/api/workbench/scheduled_tasks/:task_id/execute", api.category="workbench"
    )
    ListScheduledTaskExecutionsResponse ListScheduledTaskExecutions(1: ListScheduledTaskExecutionsRequest request)(
        api.get="/api/workbench/scheduled_tasks/:task_id/executions", api.category="workbench"
    )
}
