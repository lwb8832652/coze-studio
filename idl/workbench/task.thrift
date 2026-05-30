namespace go workbench.task

include "../base.thrift"

enum TaskStatus {
    Created = 1,
    Queued = 2,
    Running = 3,
    Succeeded = 4,
    Failed = 5,
    Canceling = 6,
    Canceled = 7,
}

struct TaskEvent {
    1: required i64 id (agw.js_conv="str", api.js_conv="true")
    2: required i64 task_id (agw.js_conv="str", api.js_conv="true")
    3: required string event_type
    4: optional string payload
    5: required i64 created_at
}

struct ChatTask {
    1: required i64 id (agw.js_conv="str", api.js_conv="true")
    2: required i64 space_id (agw.js_conv="str", api.js_conv="true")
    3: required i64 creator_id (agw.js_conv="str", api.js_conv="true")
    4: optional i64 conversation_id (agw.js_conv="str", api.js_conv="true")
    5: optional i64 message_id (agw.js_conv="str", api.js_conv="true")
    6: optional i64 skill_id (agw.js_conv="str", api.js_conv="true")
    7: required string title
    8: required TaskStatus status
    9: required i32 progress
    10: optional string input
    11: optional string result
    12: optional string error
    13: required i64 created_at
    14: required i64 updated_at
}

struct CreateTaskRequest {
    1: required i64 space_id (agw.js_conv="str", api.js_conv="true")
    2: required string title
    3: optional i64 conversation_id (agw.js_conv="str", api.js_conv="true")
    4: optional i64 message_id (agw.js_conv="str", api.js_conv="true")
    5: optional i64 skill_id (agw.js_conv="str", api.js_conv="true")
    6: optional string input
    255: optional base.Base Base (api.none="true")
}

struct CreateTaskResponse {
    1: optional ChatTask data
    253: required i64 code
    254: required string msg
    255: optional base.BaseResp BaseResp (api.none="true")
}

struct ListTasksRequest {
    1: required i64 space_id (agw.js_conv="str", api.js_conv="true")
    2: optional TaskStatus status
    3: optional i32 page
    4: optional i32 page_size
    255: optional base.Base Base (api.none="true")
}

struct ListTasksData {
    1: required list<ChatTask> tasks
    2: required i64 total
}

struct ListTasksResponse {
    1: optional ListTasksData data
    253: required i64 code
    254: required string msg
    255: optional base.BaseResp BaseResp (api.none="true")
}

struct GetTaskRequest {
    1: required i64 task_id (api.path="task_id", agw.js_conv="str", api.js_conv="true")
    255: optional base.Base Base (api.none="true")
}

struct GetTaskResponse {
    1: optional ChatTask data
    253: required i64 code
    254: required string msg
    255: optional base.BaseResp BaseResp (api.none="true")
}

struct TaskEventsData {
    1: required list<TaskEvent> events
}

struct TaskEventsResponse {
    1: optional TaskEventsData data
    253: required i64 code
    254: required string msg
    255: optional base.BaseResp BaseResp (api.none="true")
}

service WorkbenchTaskService {
    CreateTaskResponse CreateTask(1: CreateTaskRequest request)(
        api.post="/api/workbench/tasks",
        api.category="workbench"
    )
    ListTasksResponse ListTasks(1: ListTasksRequest request)(
        api.get="/api/workbench/tasks",
        api.category="workbench"
    )
    GetTaskResponse GetTask(1: GetTaskRequest request)(
        api.get="/api/workbench/tasks/:task_id",
        api.category="workbench"
    )
    GetTaskResponse CancelTask(1: GetTaskRequest request)(
        api.post="/api/workbench/tasks/:task_id/cancel",
        api.category="workbench"
    )
    GetTaskResponse RetryTask(1: GetTaskRequest request)(
        api.post="/api/workbench/tasks/:task_id/retry",
        api.category="workbench"
    )
    TaskEventsResponse ListTaskEvents(1: GetTaskRequest request)(
        api.get="/api/workbench/tasks/:task_id/events",
        api.category="workbench"
    )
}
