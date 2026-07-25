include "../base.thrift"
include "../app/bot_common.thrift"
include "shortcut_command.thrift"
include "prompt_resource.thrift"

namespace go playground

struct UpdateDraftBotInfoAgwResponse {
    1: required UpdateDraftBotInfoAgwData data,

    253: required i64                   code,
    254: required string                msg,
    255: required base.BaseResp BaseResp (api.none="true")
}

struct UpdateDraftBotInfoAgwData {
    1: optional bool   has_change       // Is there any change?
    2:          bool   check_not_pass   // True: The machine audit verification failed
    3: optional Branch branch           // Which branch is it currently on?
    4: optional bool   same_with_online
    5: optional string check_not_pass_msg // The machine audit verification failed the copy.
}

// branch
enum Branch {
    Undefined     = 0
    PersonalDraft = 1 // draft
    Base          = 2 // Space draft
    Publish       = 3 // Online version, used in diff scenarios
}

struct UpdateDraftBotInfoAgwRequest {
    1: optional bot_common.BotInfoForUpdate bot_info
    2: optional i64   base_commit_version (api.js_conv='true',agw.js_conv="str")

    255: base.Base Base (api.none="true")
}

struct GetDraftBotInfoAgwRequest {
    1: required i64  bot_id  (api.js_conv='true',agw.js_conv="str") // Draft bot_id
    2: optional string  version  // Check the history, the id of the historical version, corresponding to the id of the bot_draft_history
    3: optional string  commit_version // Query specifies commit_version version, pre-release use, seems to be the same thing as version, but the acquisition logic is different

    255: base.Base Base (api.none="true")
}

struct GetDraftBotInfoAgwResponse {
    1: required GetDraftBotInfoAgwData data,

    253: required i64                   code,
    254: required string                msg,
    255: required base.BaseResp BaseResp (api.none="true")
}

struct GetDraftBotInfoAgwData {
    1: required bot_common.BotInfo bot_info // core bot data
    2: optional BotOptionData bot_option_data // bot option information
    3: optional bool            has_unpublished_change // Are there any unpublished changes?
    4: optional BotMarketStatus bot_market_status      // The product status after the bot is put on the shelves
    5: optional bool            in_collaboration       // Is the bot in multiplayer cooperative mode?
    6: optional bool            same_with_online       // Is the content committed consistent with the online content?
    7: optional bool            editable               // For frontend, permission related, can the current user edit this bot
    8: optional bool            deletable              // For frontend, permission related, can the current user delete this bot
    9: optional UserInfo        publisher              // Is the publisher of the latest release version
    10:         bool has_publish // Has it been published?
    11:         i64 space_id    (api.js_conv='true',agw.js_conv="str")  // Space ID
    12:         list<BotConnectorInfo> connectors    // Published business line details
    13: optional Branch              branch          // What branch did you get the content of?
    14: optional string              commit_version  // If branch=PersonalDraft, the version number of checkout/rebase; if branch = base, the committed version
    15: optional string              committer_name  // For the front end, the most recent author
    16: optional string              commit_time     // For frontend, commit time
    17: optional string              publish_time    // For frontend, release time
    18: optional BotCollaboratorStatus collaborator_status // Multi-person collaboration related operation permissions
    19: optional AuditInfo           latest_audit_info // Details of the most recent review
    20: optional string              app_id // Douyin's doppelganger bot will have appId.
}

struct BotOptionData {
    1: optional map<i64,ModelDetail>        model_detail_map      // model details
    2: optional map<i64,PluginDetal>        plugin_detail_map     // plugin details
    3: optional map<i64,PluginAPIDetal>     plugin_api_detail_map // Plugin API Details
    4: optional map<i64,WorkflowDetail>     workflow_detail_map   // Workflow Details
    5: optional map<string,KnowledgeDetail> knowledge_detail_map  // Knowledge Details
    6: optional list<shortcut_command.ShortcutCommand>   shortcut_command_list  // Quick command list
}


struct ModelDetail {
    1: optional string name           // Model display name (to the user)
    2: optional string model_name     // Model name (for internal)
    3: optional i64    model_id       (agw.js_conv="str" api.js_conv="true") // Model ID
    4: optional i64    model_family   // Model Category
    5: optional string model_icon_url // IconURL
}

struct PluginDetal {
    1: optional i64    id            (agw.js_conv="str" api.js_conv="true")
    2: optional string name
    3: optional string description
    4: optional string icon_url
    5: optional i64    plugin_type (agw.js_conv="str" api.js_conv="true")
    6: optional i64    plugin_status (agw.js_conv="str" api.js_conv="true")
    7: optional bool   is_official
    9: optional bot_common.PluginFrom plugin_from // 
}

struct PluginAPIDetal {
    1: optional i64                   id          (agw.js_conv="str" api.js_conv="true")
    2: optional string                name
    3: optional string                description
    4: optional list<PluginParameter> parameters
    5: optional i64                   plugin_id   (agw.js_conv="str" api.js_conv="true")
}

struct PluginParameter {
    1: optional string                name
    2: optional string                description
    3: optional bool                  is_required
    4: optional string                type
    5: optional list<PluginParameter> sub_parameters
    6: optional string                sub_type       // If Type is an array, there is a subtype
    7: optional i64                   assist_type
}

struct WorkflowDetail {
    1: optional i64    id          (agw.js_conv="str" api.js_conv="true")
    2: optional string name
    3: optional string description
    4: optional string icon_url
    5: optional i64    status
    6: optional i64    type        // Type 1: Official Template
    7: optional i64    plugin_id   (agw.js_conv="str" api.js_conv="true") // Plugin ID corresponding to workfklow
    8: optional bool   is_official
    9: optional PluginAPIDetal api_detail
}

struct KnowledgeDetail {
    1: optional string id
    2: optional string name
    3: optional string icon_url
    4: DataSetType format_type
}


enum DataSetType {
    Text = 0 // Text
    Table = 1 // table
    Image = 2 // image
}


enum BotMarketStatus {
    Offline = 0 // offline
    Online  = 1 // put on the shelves
}

struct UserInfo {
    1: i64    user_id   (api.js_conv='true',agw.js_conv="str")  // user id
    2: string name     // user name
    3: string icon_url // user icon
}

struct BotConnectorInfo {
    1:          string                 id
    2:          string                 name
    3:          string                 icon
    4:          ConnectorDynamicStatus connector_status
    5: optional string                 share_link
}

enum ConnectorDynamicStatus {
    Normal          = 0
    Offline         = 1
    TokenDisconnect = 2
}


struct BotCollaboratorStatus {
    1: bool commitable    // Can the current user submit?
    2: bool operateable   // Is the current user operable?
    3: bool manageable    // Can the current user manage collaborators?
}

struct AuditInfo {
    1: optional AuditStatus audit_status
    2: optional string publish_id
    3: optional string commit_version
}

struct AuditResult {
    1: AuditStatus AuditStatus ,
    2: string      AuditMessage,
}

enum AuditStatus {
    Auditing = 0, // Under review.
    Success  = 1, // approved
    Failed   = 2, // audit failed
}


// Onboarding json structure
struct OnboardingContent {
    1: optional string       prologue            // Introductory remarks (C-end usage scenarios, only 1; background scenarios, possibly multiple)
    2: optional list<string> suggested_questions // suggestion question
    3: optional bot_common.SuggestedQuestionsShowMode suggested_questions_show_mode
}

enum ScopeType {
    All  = 0 // All under the enterprise (effective under the enterprise)
    Self = 1 // I joined (both companies and individuals are valid, no default self is passed on)
}

struct GetSpaceListV2Request {
    1: optional string search_word                      // Search term
    2: optional i64 enterprise_id (api.js_conv='true',agw.js_conv="str") // Enterprise ID
    3: optional i64 organization_id (api.js_conv='true',agw.js_conv="str") // organization id
    4: optional ScopeType scope_type                    // range type
    5: optional i32 page                                // paging information
    6: optional i32 size                                // Paging size -- if page and size are not passed on, it is considered not paging

    255: optional base.Base Base (api.none="true")
}

enum SpaceType {
    Personal = 1 // individual
    Team     = 2 // group
}

enum SpaceMode {
    Normal = 0
    DevMode = 1
}

enum SpaceTag {
    Professional  =  1  // Professional Edition
}

enum SpaceRoleType {
    Default = 0 // default
    Owner   = 1 // owner
    Admin   = 2 // administrator
    Member  = 3 // ordinary member
}

// Application management list
enum SpaceApplyStatus {
    All              =  0     // all
    Joined           =  1     // Joined
    Confirming       =  2     // Confirming
    Rejected         =  3     // Rejected
}

struct AppIDInfo{
    1: string id
    2: string name
    3: string icon
}

struct ConnectorInfo{
    1: string id
    2: string name
    3: string icon
}

struct BotSpaceV2 {
    1: i64                 id (api.js_conv='true',agw.js_conv="str") // Space id, newly created as 0
    2: list<AppIDInfo>     app_ids        // publishing platform
    3: string              name           // space name
    4: string              description    // spatial description
    5: string              icon_url       // icon url
    6: SpaceType           space_type     // space type
    7: list<ConnectorInfo> connectors     // publishing platform
    8: bool                hide_operation // Whether to hide New, Copy Delete buttons
    9: i32                 role_type      // Role in team 1-owner 2-admin 3-member
    10: optional SpaceMode space_mode     // Spatial Mode
    11: bool               display_local_plugin // Whether to display the end-side plug-in creation entry
    12: SpaceRoleType      space_role_type // Role type, enumeration
    13: optional SpaceTag  space_tag       // spatial label
    14: optional i64    enterprise_id (api.js_conv='true',agw.js_conv="str") // Enterprise ID
    15: optional i64    organization_id (api.js_conv='true',agw.js_conv="str") // organization id
    16: optional i64    owner_user_id (api.js_conv='true',agw.js_conv="str") // Space owner uid
    17: optional string    owner_name      // Space owner nickname
    18: optional string    owner_user_name // Space owner username
    19: optional string    owner_icon_url  // Space owner image
    20: optional SpaceApplyStatus space_apply_status // The current visiting user joins the space status
    21: optional i64       total_member_num // The total number of space members, only the organization space can be queried.
    22: bool               allow_develop // Whether AppDev is enabled for the workspace.
    23: bool               receive_publish // Whether the workspace accepts published resources.
}

struct SpaceInfo {
    1: list<BotSpaceV2> bot_space_list     // User joins space list
    2: bool             has_personal_space // Is there any personal space available?
    3: i32              team_space_num     // Number of team spaces created by individuals
    4: i32              max_team_space_num // The maximum number of spaces an individual can create
    5: list<BotSpaceV2> recently_used_space_list // list of recently used spaces
    6: optional i32 total                        // Effective when paging
    7: optional bool has_more                    // Effective when paging
}

struct GetSpaceListV2Response {
    1:   SpaceInfo       data
    253: required i64    code
    254: required string msg
    255: required base.BaseResp BaseResp
}

struct GetImagexShortUrlResponse{
    1             :      GetImagexShortUrlData data
    253: required i64    code
    254: required string msg
    255: required base.BaseResp BaseResp (api.none="true")
}

struct GetImagexShortUrlData {
    1: map<string,UrlInfo>   url_info //Audit status, key uri, value url and, audit status

}

struct UrlInfo {
    1: string url
    2: bool   review_status

}


enum GetImageScene {
    Onboarding = 0
    BackgroundImage = 1
}

struct GetImagexShortUrlRequest{
    1: list<string> uris
    2: GetImageScene scene

    255: base.Base Base (api.none="true"),
}



struct UserBasicInfo {
    1: required i64                  UserId         (api.js_conv='true',agw.js_conv="str", api.body="user_id")
    3: required string               Username       (api.body="user_name") // nickname
    4: required string               UserAvatar     (api.body="user_avatar") // avatar
    5: optional string               UserUniqueName (api.body="user_unique_name") // user name
    6: optional bot_common.UserLabel UserLabel      (api.body="user_label") // user tag
    7: optional i64                  CreateTime     (api.body="create_time") // user creation time
}

struct MGetUserBasicInfoRequest {
    1 : required list<string> UserIds (agw.js_conv="str", api.js_conv="true", api.body="user_ids")
    2 : optional bool NeedUserStatus (api.body="need_user_status")
    3 : optional bool NeedEnterpriseIdentity (api.body="need_enterprise_identity") // Whether enterprise authentication information is required, the default is true when the front end is called through AGW
    4 : optional bool NeedVolcanoUserName (api.body="need_volcano_user_name") // Do you need a volcano username?

    255: optional base.Base Base (api.none="true")
}

struct MGetUserBasicInfoResponse {
    1 : optional map<string, UserBasicInfo> UserBasicInfoMap (api.body="id_user_info_map")

    253:          i64           code
    254:          string        msg

    255: optional base.BaseResp BaseResp (api.none="true")
}

struct GetBotPopupInfoRequest {
    1: required list<BotPopupType> bot_popup_types
    2: required i64                bot_id          (agw.js_conv="str" api.js_conv="true")

    255: base.Base Base (api.none="true")
}

struct GetBotPopupInfoResponse {
    1:   required BotPopupInfoData data

    253: required i64              code
    254: required string           msg
    255: required base.BaseResp BaseResp (api.none="true")
}

struct BotPopupInfoData {
    1: required map<BotPopupType,i64> bot_popup_count_info
}

enum BotPopupType {
    AutoGenBeforePublish = 1
}


struct UpdateBotPopupInfoResponse {
    253: required i64    code
    254: required string msg
    255: required base.BaseResp BaseResp (api.none="true")
}


struct UpdateBotPopupInfoRequest {
    1: required BotPopupType bot_popup_type
    2: required i64          bot_id         (agw.js_conv="str" api.js_conv="true")

    255: base.Base Base (api.none="true")
}

struct ReportUserBehaviorRequest {
    1: required i64 ResourceID (api.body = "resource_id",api.js_conv="true")
    2: required SpaceResourceType ResourceType (api.body="resource_type")
    3: required BehaviorType BehaviorType (api.body="behavior_type")
    4: optional i64 SpaceID (agw.js_conv="str",api.js_conv="true",api.body="space_id",agw.key="space_id") // This requirement must be passed on

    255: base.Base Base (api.none="true")
}

struct ReportUserBehaviorResponse {
    253: required i64    code
    254: required string msg
    255: required base.BaseResp BaseResp
}

enum SpaceResourceType {
    DraftBot = 1
    Project = 2
    Space   = 3
    DouyinAvatarBot = 4
}

enum BehaviorType {
    Visit = 1
    Edit = 2
}

struct FileInfo {
    1 : string url
    2 : string uri
}
enum GetFileUrlsScene {
    shorcutIcon = 1
}
struct GetFileUrlsRequest {
    1 :  GetFileUrlsScene scene
    255: base.Base Base
}

struct GetFileUrlsResponse {
    1 : list<FileInfo> file_list
    253: i64 code
    254: string msg
    255: base.BaseResp BaseResp
}

enum NoticeRankType {
    All = 0
    Unread = 1
}

enum NoticeSenderType {
    Bot = 1
}

enum ReadStatus {
    Unread = 1
    Read = 2
}

enum NoticeSeverity {
    Info = 1
    Success = 2
    Warning = 3
    Error = 4
}

enum NoticeCategory {
    Task = 1
    ScheduledTask = 2
    AppDev = 3
    MCP = 4
    Resource = 5
    Workspace = 6
    IM = 7
    Billing = 8
    System = 9
}

enum NoticeRoute {
    None = 0
    TaskThread = 1
    ScheduledTaskCenter = 2
    AppDev = 3
    Skill = 4
    Workspace = 5
    Billing = 6
    SystemAnnouncements = 7
}

enum AdminAnnouncementRouteType {
    None = 0
    WorkspaceHome = 1
    SystemAnnouncements = 2
}

struct AdminAnnouncementRoute {
    1: required AdminAnnouncementRouteType type
    2: optional string space_id
}

struct AdminAnnouncementAudience {
    1: required string type
    2: optional list<string> target_ids
}

struct AdminAnnouncement {
    1: required string id
    2: required string title
    3: required string body
    4: required string severity
    5: required AdminAnnouncementRoute route
    6: required AdminAnnouncementAudience audience
    7: required string status
    8: required string projection_status
    9: optional string scheduled_at
    10: optional string publish_requested_at
    11: optional string snapshot_at
    12: optional string published_at
    13: optional string cancelled_at
    14: required string created_by
    15: required string updated_by
    16: required i64 recipient_count
    17: required i64 projected_count
    18: optional string last_error_code
    19: required i64 version
    20: required string created_at
    21: required string updated_at
}

struct AdminAnnouncementAuditEvent {
    1: required string id
    2: required string announcement_id
    3: required string actor_id
    4: required string action
    5: optional string from_status
    6: optional string to_status
    7: required string projection_status
    8: required string result
    9: optional string error_code
    10: required i64 recipient_count
    11: required i64 projected_count
    12: required string created_at
}

struct AdminAnnouncementData {
    1: required AdminAnnouncement announcement
    2: optional bool replayed
}

struct AdminAnnouncementListData {
    1: required list<AdminAnnouncement> announcements
    2: required i64 total
}

struct AdminAnnouncementPublicationData {
    1: required AdminAnnouncement announcement
    2: required bool deferred
    3: optional string error_code
    4: optional bool replayed
}

struct AdminAnnouncementReplayData {
    1: required i64 processed
    2: required i64 completed
    3: required i64 failed
    4: required i64 deferred
    5: optional map<string, i64> error_codes
}

struct AdminAnnouncementAuditListData {
    1: required list<AdminAnnouncementAuditEvent> audit_events
    2: required i64 total
}

struct AdminAnnouncementResponse {
    1: required i64 code
    2: required string msg
    3: optional string error_code
    4: optional AdminAnnouncementData data
}

struct AdminAnnouncementListResponse {
    1: required i64 code
    2: required string msg
    3: optional string error_code
    4: optional AdminAnnouncementListData data
}

struct AdminAnnouncementPublicationResponse {
    1: required i64 code
    2: required string msg
    3: optional string error_code
    4: optional AdminAnnouncementPublicationData data
}

struct AdminAnnouncementReplayResponse {
    1: required i64 code
    2: required string msg
    3: optional string error_code
    4: optional AdminAnnouncementReplayData data
}

struct AdminAnnouncementAuditListResponse {
    1: required i64 code
    2: required string msg
    3: optional string error_code
    4: optional AdminAnnouncementAuditListData data
}

struct CreateAdminAnnouncementRequest {
    1: required string title (api.body = "title")
    2: required string body (api.body = "body")
    3: required string severity (api.body = "severity")
    4: required AdminAnnouncementRoute route (api.body = "route")
    5: required AdminAnnouncementAudience audience (api.body = "audience")
    6: required string idempotency_key (api.body = "idempotency_key")
}

struct ListAdminAnnouncementsRequest {
    1: optional string status (api.query = "status")
    2: optional i64 offset (api.query = "offset")
    3: optional i64 limit (api.query = "limit")
}

struct GetAdminAnnouncementRequest {
    1: required string id (api.path = "id")
}

struct UpdateAdminAnnouncementRequest {
    1: required string id (api.path = "id")
    2: required i64 expected_version (api.body = "expected_version")
    3: required string title (api.body = "title")
    4: required string body (api.body = "body")
    5: required string severity (api.body = "severity")
    6: required AdminAnnouncementRoute route (api.body = "route")
    7: required AdminAnnouncementAudience audience (api.body = "audience")
}

struct ScheduleAdminAnnouncementRequest {
    1: required string id (api.path = "id")
    2: required i64 expected_version (api.body = "expected_version")
    3: required string scheduled_at (api.body = "scheduled_at")
}

struct PublishAdminAnnouncementRequest {
    1: required string id (api.path = "id")
    2: required i64 expected_version (api.body = "expected_version")
    3: required string idempotency_key (api.body = "idempotency_key")
}

struct CancelAdminAnnouncementRequest {
    1: required string id (api.path = "id")
    2: required i64 expected_version (api.body = "expected_version")
}

struct ReplayAdminAnnouncementsRequest {
    1: optional string announcement_id (api.body = "announcement_id")
}

struct ListAdminAnnouncementAuditEventsRequest {
    1: required string id (api.path = "id")
    2: optional i64 offset (api.query = "offset")
    3: optional i64 limit (api.query = "limit")
}

enum NoticeReadMode {
    NoticeIDs = 1
    Snapshot = 2
}

struct NoticeSender {
    1: optional NoticeSenderType sender_type
    2: optional string sender_id
    3: optional string sender_name
    4: optional string sender_icon_url
}

struct Notice {
    1: optional string id
    2: optional string content
    3: optional string jump_link // Deprecated compatibility field; clients construct paths from route.
    4: optional ReadStatus read_status
    5: optional NoticeSender sender
    6: optional string create_time
    7: optional NoticeSeverity severity
    8: optional NoticeCategory category
    9: optional NoticeRoute route
    10: optional string route_space_id
    11: optional string route_target_id
}

struct NoticeMarkReadRequest {
    1: optional list<string> notice_ids // Required only for NoticeIDs mode.
    2: required NoticeReadMode read_mode
    3: optional string snapshot_cutoff // Required only for Snapshot mode.
}

struct NoticeMarkReadResponse {
    252: optional string error_code
    253: required i64 code
    254: required string msg
}

struct GetNoticeListData {
    1: optional list<Notice> notice_list
    2: optional string next_cursor
    3: optional bool has_more
    4: required string snapshot_cutoff
}

struct GetNoticeListRequest {
    1: required string cursor
    2: optional i32 count
    3: optional NoticeRankType notice_rank_type
}

struct GetNoticeListResponse {
    1: optional GetNoticeListData data
    252: optional string error_code
    253: required i64 code
    254: required string msg
}

struct GetNoticeUnreadCountRequest {}

struct GetNoticeUnreadCountData {
    1: optional i32 unread_count
}

struct GetNoticeUnreadCountResponse {
    1: optional GetNoticeUnreadCountData data
    252: optional string error_code
    253: required i64 code
    254: required string msg
}

service PlaygroundService {
    AdminAnnouncementResponse CreateAdminAnnouncement(1: CreateAdminAnnouncementRequest request) (api.post = "/api/admin/announcements", api.category = "admin")
    AdminAnnouncementListResponse ListAdminAnnouncements(1: ListAdminAnnouncementsRequest request) (api.get = "/api/admin/announcements", api.category = "admin")
    AdminAnnouncementReplayResponse ReplayAdminAnnouncements(1: ReplayAdminAnnouncementsRequest request) (api.post = "/api/admin/announcements/replay", api.category = "admin")
    AdminAnnouncementResponse GetAdminAnnouncement(1: GetAdminAnnouncementRequest request) (api.get = "/api/admin/announcements/:id", api.category = "admin")
    AdminAnnouncementResponse UpdateAdminAnnouncement(1: UpdateAdminAnnouncementRequest request) (api.put = "/api/admin/announcements/:id", api.category = "admin")
    AdminAnnouncementResponse ScheduleAdminAnnouncement(1: ScheduleAdminAnnouncementRequest request) (api.post = "/api/admin/announcements/:id/schedule", api.category = "admin")
    AdminAnnouncementPublicationResponse PublishAdminAnnouncement(1: PublishAdminAnnouncementRequest request) (api.post = "/api/admin/announcements/:id/publish", api.category = "admin")
    AdminAnnouncementResponse CancelAdminAnnouncement(1: CancelAdminAnnouncementRequest request) (api.post = "/api/admin/announcements/:id/cancel", api.category = "admin")
    AdminAnnouncementAuditListResponse ListAdminAnnouncementAuditEvents(1: ListAdminAnnouncementAuditEventsRequest request) (api.get = "/api/admin/announcements/:id/audit-events", api.category = "admin")
    UpdateDraftBotInfoAgwResponse UpdateDraftBotInfoAgw(1:UpdateDraftBotInfoAgwRequest request)(api.post='/api/playground_api/draftbot/update_draft_bot_info', api.category="draftbot",agw.preserve_base="true")
    GetDraftBotInfoAgwResponse GetDraftBotInfoAgw(1:GetDraftBotInfoAgwRequest request)(api.post='/api/playground_api/draftbot/get_draft_bot_info', api.category="draftbot",agw.preserve_base="true")
    GetImagexShortUrlResponse GetImagexShortUrl (1:GetImagexShortUrlRequest request)(api.post='/api/playground_api/get_imagex_url', api.category="file",agw.preserve_base="true")

    // public popup_info
    GetBotPopupInfoResponse GetBotPopupInfo (1:GetBotPopupInfoRequest request)(api.post='/api/playground_api/operate/get_bot_popup_info', api.category="account",agw.preserve_base="true")
    UpdateBotPopupInfoResponse UpdateBotPopupInfo (1:UpdateBotPopupInfoRequest request)(api.post='/api/playground_api/operate/update_bot_popup_info', api.category="account",agw.preserve_base="true")
    ReportUserBehaviorResponse ReportUserBehavior(1:ReportUserBehaviorRequest request)(api.post='/api/playground_api/report_user_behavior', api.category="playground_api",agw.preserve_base="true")

   // Create shortcut instructions
    shortcut_command.CreateUpdateShortcutCommandResponse CreateUpdateShortcutCommand(1: shortcut_command.CreateUpdateShortcutCommandRequest req)(api.post='/api/playground_api/create_update_shortcut_command', api.category="playground_api", agw.preserve_base="true")
    GetFileUrlsResponse GetFileUrls(1: GetFileUrlsRequest req)(api.post='/api/playground_api/get_file_list', api.category="playground_api", agw.preserve_base="true")
    NoticeMarkReadResponse NoticeMarkRead(1: NoticeMarkReadRequest req)(api.post='/api/playground_api/notice/mark_read', api.category="notice", agw.preserve_base="true")
    GetNoticeListResponse GetNoticeList(1: GetNoticeListRequest req)(api.post='/api/playground_api/notice/get_list', api.category="notice", agw.preserve_base="true")
    GetNoticeUnreadCountResponse GetNoticeUnreadCount(1: GetNoticeUnreadCountRequest req)(api.post='/api/playground_api/notice/get_unread_count', api.category="notice", agw.preserve_base="true")


    // prompt resource
    prompt_resource.GetOfficialPromptResourceListResponse GetOfficialPromptResourceList(1:prompt_resource.GetOfficialPromptResourceListRequest request)(api.post='/api/playground_api/get_official_prompt_list', api.category="prompt_resource",agw.preserve_base="true")
    prompt_resource.GetPromptResourceInfoResponse GetPromptResourceInfo(1:prompt_resource.GetPromptResourceInfoRequest request)(api.get='/api/playground_api/get_prompt_resource_info', api.category="prompt_resource",agw.preserve_base="true")
    prompt_resource.UpsertPromptResourceResponse UpsertPromptResource(1:prompt_resource.UpsertPromptResourceRequest request)(api.post='/api/playground_api/upsert_prompt_resource', api.category="prompt_resource",agw.preserve_base="true")
    prompt_resource.DeletePromptResourceResponse DeletePromptResource(1:prompt_resource.DeletePromptResourceRequest request)(api.post='/api/playground_api/delete_prompt_resource', api.category="prompt_resource",agw.preserve_base="true")

    GetSpaceListV2Response GetSpaceListV2(1:GetSpaceListV2Request request)(api.post='/api/playground_api/space/list', api.category="space",agw.preserve_base="true")
    MGetUserBasicInfoResponse MGetUserBasicInfo(1: MGetUserBasicInfoRequest request) (api.post='/api/playground_api/mget_user_info', api.category="playground_api",agw.preserve_base="true")

}
