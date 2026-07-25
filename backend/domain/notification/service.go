// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package notification

import (
	"fmt"
	"strconv"
	"strings"
)

type MessageDraft struct {
	EventID    string
	Scope      Scope
	SpaceID    int64
	SenderID   int64
	Category   Category
	Severity   Severity
	EventType  EventType
	Title      string
	Content    string
	TargetType TargetType
	TargetID   string
	CreatedAt  int64
}

type eventTemplate struct {
	Title                string
	DefaultContent       string
	Scope                Scope
	Category             Category
	Severity             Severity
	TargetType           TargetType
	Policies             map[RecipientPolicy]struct{}
	AllowedPayloadFields map[payloadField]struct{}
}

type payloadField string

const (
	payloadFieldResourceDisplayName  payloadField = "resource_display_name"
	payloadFieldActorDisplayName     payloadField = "actor_display_name"
	payloadFieldStatusReasonCode     payloadField = "status_reason_code"
	payloadFieldTargetID             payloadField = "target_id"
	payloadFieldAnnouncementTitle    payloadField = "announcement_title"
	payloadFieldAnnouncementBody     payloadField = "announcement_body"
	payloadFieldAnnouncementSeverity payloadField = "announcement_severity"
	payloadFieldAnnouncementRoute    payloadField = "announcement_route"
	payloadFieldWorkspaceAction      payloadField = "workspace_action"
	payloadFieldWorkspaceAudience    payloadField = "workspace_audience"
	payloadFieldSubjectDisplayName   payloadField = "subject_display_name"
	payloadFieldExplicitRecipientIDs payloadField = "explicit_recipient_ids"
)

type TemplateRegistry struct {
	templates map[EventType]eventTemplate
}

var reasonContent = map[StatusReasonCode]string{
	StatusReasonActionRequired:       "需要进一步处理",
	StatusReasonConfigurationInvalid: "配置无效",
	StatusReasonPermissionDenied:     "权限不足",
	StatusReasonQuotaInsufficient:    "可用额度不足",
	StatusReasonProviderUnavailable:  "依赖服务不可用",
	StatusReasonConnectionFailed:     "连接失败",
	StatusReasonRetryExhausted:       "重试次数已用尽",
	StatusReasonCanceledByUser:       "用户已取消",
}

func DefaultTemplateRegistry() *TemplateRegistry {
	actor := policies(RecipientActor)
	owner := policies(RecipientResourceOwner, RecipientActor)
	ownerOrExplicit := policies(
		RecipientResourceOwner,
		RecipientActor,
		RecipientExplicitInternalUsers,
	)
	adminsOrExplicit := policies(RecipientWorkspaceOwnersAdmins, RecipientExplicitInternalUsers)
	explicit := policies(RecipientExplicitInternalUsers)
	system := policies(RecipientSystemAdmins)

	return &TemplateRegistry{templates: map[EventType]eventTemplate{
		EventAgentRunAwaitingInput: template("任务等待你的输入", "任务需要补充信息后才能继续。", ScopeWorkspace, CategoryTask, SeverityWarning, TargetTaskThread, actor, payloadFields(payloadFieldResourceDisplayName, payloadFieldActorDisplayName, payloadFieldStatusReasonCode, payloadFieldTargetID)),
		EventAgentRunSucceeded:     template("任务已完成", "任务已完成，可查看结果。", ScopeWorkspace, CategoryTask, SeveritySuccess, TargetTaskThread, actor, payloadFields(payloadFieldResourceDisplayName, payloadFieldActorDisplayName, payloadFieldStatusReasonCode, payloadFieldTargetID)),
		EventAgentRunFailed:        template("任务执行失败", "任务执行失败，请稍后重试。", ScopeWorkspace, CategoryTask, SeverityError, TargetTaskThread, actor, payloadFields(payloadFieldResourceDisplayName, payloadFieldActorDisplayName, payloadFieldStatusReasonCode, payloadFieldTargetID)),
		EventAgentRunCanceled:      template("任务已取消", "任务已取消。", ScopeWorkspace, CategoryTask, SeverityInfo, TargetTaskThread, actor, payloadFields(payloadFieldResourceDisplayName, payloadFieldActorDisplayName, payloadFieldStatusReasonCode, payloadFieldTargetID)),
		EventTaskCompleted:         template("任务已完成", "任务已完成，可查看结果。", ScopeWorkspace, CategoryTask, SeveritySuccess, TargetTaskThread, actor, payloadFields(payloadFieldResourceDisplayName, payloadFieldActorDisplayName, payloadFieldStatusReasonCode, payloadFieldTargetID)),
		EventTaskFailed:            template("任务执行失败", "任务执行失败，请稍后重试。", ScopeWorkspace, CategoryTask, SeverityError, TargetTaskThread, actor, payloadFields(payloadFieldResourceDisplayName, payloadFieldActorDisplayName, payloadFieldStatusReasonCode, payloadFieldTargetID)),
		EventTaskCancelled:         template("任务已取消", "任务已取消。", ScopeWorkspace, CategoryTask, SeverityInfo, TargetTaskThread, actor, payloadFields(payloadFieldResourceDisplayName, payloadFieldActorDisplayName, payloadFieldStatusReasonCode, payloadFieldTargetID)),
		EventTaskAwaitingInput:     template("任务等待你的输入", "任务需要补充信息后才能继续。", ScopeWorkspace, CategoryTask, SeverityWarning, TargetTaskThread, actor, payloadFields(payloadFieldResourceDisplayName, payloadFieldActorDisplayName, payloadFieldStatusReasonCode, payloadFieldTargetID)),

		EventScheduledExecutionSucceeded: template("定时任务执行完成", "定时任务已执行完成。", ScopeWorkspace, CategoryScheduledTask, SeveritySuccess, TargetScheduledTaskCenter, actor, payloadFields(payloadFieldResourceDisplayName, payloadFieldActorDisplayName, payloadFieldStatusReasonCode, payloadFieldTargetID)),
		EventScheduledExecutionFailed:    template("定时任务执行失败", "定时任务执行失败，请检查配置。", ScopeWorkspace, CategoryScheduledTask, SeverityError, TargetScheduledTaskCenter, actor, payloadFields(payloadFieldResourceDisplayName, payloadFieldActorDisplayName, payloadFieldStatusReasonCode, payloadFieldTargetID)),
		EventScheduledExecutionCanceled:  template("定时任务已取消", "定时任务执行已取消。", ScopeWorkspace, CategoryScheduledTask, SeverityInfo, TargetScheduledTaskCenter, actor, payloadFields(payloadFieldResourceDisplayName, payloadFieldActorDisplayName, payloadFieldStatusReasonCode, payloadFieldTargetID)),

		EventAppDevBuildSucceeded:  template("网页应用构建完成", "网页应用构建已完成。", ScopeWorkspace, CategoryAppDev, SeveritySuccess, TargetAppDev, owner, payloadFields(payloadFieldResourceDisplayName, payloadFieldActorDisplayName, payloadFieldStatusReasonCode, payloadFieldTargetID)),
		EventAppDevBuildFailed:     template("网页应用构建失败", "网页应用构建失败，请检查项目配置。", ScopeWorkspace, CategoryAppDev, SeverityError, TargetAppDev, owner, payloadFields(payloadFieldResourceDisplayName, payloadFieldActorDisplayName, payloadFieldStatusReasonCode, payloadFieldTargetID)),
		EventAppDevDeploySucceeded: template("网页应用发布完成", "网页应用已发布。", ScopeWorkspace, CategoryAppDev, SeveritySuccess, TargetAppDev, owner, payloadFields(payloadFieldResourceDisplayName, payloadFieldActorDisplayName, payloadFieldStatusReasonCode, payloadFieldTargetID)),
		EventAppDevDeployFailed:    template("网页应用发布失败", "网页应用发布失败，请稍后重试。", ScopeWorkspace, CategoryAppDev, SeverityError, TargetAppDev, owner, payloadFields(payloadFieldResourceDisplayName, payloadFieldActorDisplayName, payloadFieldStatusReasonCode, payloadFieldTargetID)),

		EventMCPDeploymentSucceeded: template("MCP 服务部署完成", "MCP 服务已部署。", ScopeWorkspace, CategoryMCP, SeveritySuccess, TargetWorkspace, actor, payloadFields(payloadFieldResourceDisplayName, payloadFieldActorDisplayName, payloadFieldStatusReasonCode, payloadFieldTargetID)),
		EventMCPDeploymentFailed:    template("MCP 服务部署失败", "MCP 服务部署失败，请检查配置。", ScopeWorkspace, CategoryMCP, SeverityError, TargetWorkspace, actor, payloadFields(payloadFieldResourceDisplayName, payloadFieldActorDisplayName, payloadFieldStatusReasonCode, payloadFieldTargetID)),
		EventMCPConnectionDegraded:  template("MCP 服务连接异常", "MCP 服务持续不可用，请及时处理。", ScopeWorkspace, CategoryMCP, SeverityWarning, TargetWorkspace, adminsOrExplicit, payloadFields(payloadFieldResourceDisplayName, payloadFieldStatusReasonCode, payloadFieldTargetID, payloadFieldExplicitRecipientIDs)),
		EventMCPConnectionRecovered: template("MCP 服务连接恢复", "MCP 服务连接已恢复。", ScopeWorkspace, CategoryMCP, SeveritySuccess, TargetWorkspace, adminsOrExplicit, payloadFields(payloadFieldResourceDisplayName, payloadFieldStatusReasonCode, payloadFieldTargetID, payloadFieldExplicitRecipientIDs)),

		EventSkillOperationSucceeded:    template("技能操作完成", "技能异步操作已完成。", ScopeWorkspace, CategoryResource, SeveritySuccess, TargetSkill, owner, payloadFields(payloadFieldResourceDisplayName, payloadFieldActorDisplayName, payloadFieldStatusReasonCode, payloadFieldTargetID)),
		EventSkillOperationFailed:       template("技能操作失败", "技能异步操作失败，请稍后重试。", ScopeWorkspace, CategoryResource, SeverityError, TargetSkill, owner, payloadFields(payloadFieldResourceDisplayName, payloadFieldActorDisplayName, payloadFieldStatusReasonCode, payloadFieldTargetID)),
		EventPluginOperationSucceeded:   template("插件操作完成", "插件异步操作已完成。", ScopeWorkspace, CategoryResource, SeveritySuccess, TargetWorkspace, owner, payloadFields(payloadFieldResourceDisplayName, payloadFieldActorDisplayName, payloadFieldStatusReasonCode, payloadFieldTargetID)),
		EventPluginOperationFailed:      template("插件操作失败", "插件异步操作失败，请稍后重试。", ScopeWorkspace, CategoryResource, SeverityError, TargetWorkspace, owner, payloadFields(payloadFieldResourceDisplayName, payloadFieldActorDisplayName, payloadFieldStatusReasonCode, payloadFieldTargetID)),
		EventResourceOperationSucceeded: template("资源操作完成", "资源异步操作已完成。", ScopeWorkspace, CategoryResource, SeveritySuccess, TargetWorkspace, owner, payloadFields(payloadFieldResourceDisplayName, payloadFieldActorDisplayName, payloadFieldStatusReasonCode, payloadFieldTargetID)),
		EventResourceOperationFailed:    template("资源操作失败", "资源异步操作失败，请稍后重试。", ScopeWorkspace, CategoryResource, SeverityError, TargetWorkspace, owner, payloadFields(payloadFieldResourceDisplayName, payloadFieldActorDisplayName, payloadFieldStatusReasonCode, payloadFieldTargetID)),

		EventWorkspaceMembershipChanged: template("工作空间成员变更", "工作空间成员已变更。", ScopeWorkspace, CategoryWorkspace, SeverityInfo, TargetWorkspace, explicit, payloadFields(payloadFieldResourceDisplayName, payloadFieldWorkspaceAction, payloadFieldWorkspaceAudience, payloadFieldSubjectDisplayName, payloadFieldExplicitRecipientIDs)),
		EventWorkspaceRoleChanged:       template("工作空间角色变更", "工作空间成员角色已变更。", ScopeWorkspace, CategoryWorkspace, SeverityInfo, TargetWorkspace, explicit, payloadFields(payloadFieldResourceDisplayName, payloadFieldWorkspaceAction, payloadFieldWorkspaceAudience, payloadFieldSubjectDisplayName, payloadFieldExplicitRecipientIDs)),

		EventIMChannelConnectionFailed: template("飞书机器人连接异常", "飞书机器人持续连接失败，请检查配置。", ScopeWorkspace, CategoryIM, SeverityError, TargetWorkspace, adminsOrExplicit, payloadFields(payloadFieldResourceDisplayName, payloadFieldStatusReasonCode, payloadFieldTargetID, payloadFieldExplicitRecipientIDs)),
		EventIMChannelRecovered:        template("飞书机器人连接恢复", "飞书机器人连接已恢复。", ScopeWorkspace, CategoryIM, SeveritySuccess, TargetWorkspace, adminsOrExplicit, payloadFields(payloadFieldResourceDisplayName, payloadFieldStatusReasonCode, payloadFieldTargetID, payloadFieldExplicitRecipientIDs)),
		EventIMMessageDeadLettered:     template("飞书消息处理失败", "一条飞书消息已进入死信，请检查服务状态。", ScopeWorkspace, CategoryIM, SeverityError, TargetWorkspace, adminsOrExplicit, payloadFields(payloadFieldResourceDisplayName, payloadFieldStatusReasonCode, payloadFieldTargetID, payloadFieldExplicitRecipientIDs)),

		EventBillingPaymentSucceeded:    template("支付成功", "支付已完成。", ScopePersonal, CategoryBilling, SeveritySuccess, TargetBilling, owner, payloadFields(payloadFieldResourceDisplayName, payloadFieldStatusReasonCode, payloadFieldTargetID)),
		EventBillingPaymentFailed:       template("支付失败", "支付失败，请检查支付状态。", ScopePersonal, CategoryBilling, SeverityError, TargetBilling, owner, payloadFields(payloadFieldResourceDisplayName, payloadFieldStatusReasonCode, payloadFieldTargetID)),
		EventBillingOrderTimedOut:       template("订单已超时", "订单因未在有效期内完成支付而关闭。", ScopePersonal, CategoryBilling, SeverityWarning, TargetBilling, owner, payloadFields(payloadFieldResourceDisplayName, payloadFieldTargetID)),
		EventBillingSubscriptionActivated: template("订阅已激活", "你的订阅已激活。", ScopePersonal, CategoryBilling, SeveritySuccess, TargetBilling, owner, payloadFields(payloadFieldResourceDisplayName, payloadFieldStatusReasonCode, payloadFieldTargetID)),
		EventBillingSubscriptionChanged: template("订阅状态变更", "你的订阅状态已变更。", ScopePersonal, CategoryBilling, SeverityInfo, TargetBilling, owner, payloadFields(payloadFieldResourceDisplayName, payloadFieldStatusReasonCode, payloadFieldTargetID)),
		EventBillingSubscriptionExpiring: template("订阅即将到期", "你的订阅即将到期。", ScopePersonal, CategoryBilling, SeverityWarning, TargetBilling, owner, payloadFields(payloadFieldResourceDisplayName, payloadFieldTargetID)),
		EventBillingSubscriptionExpired: template("订阅已到期", "你的订阅已到期。", ScopePersonal, CategoryBilling, SeverityWarning, TargetBilling, owner, payloadFields(payloadFieldResourceDisplayName, payloadFieldTargetID)),
		EventBillingCreditAdjusted:      template("积分余额已调整", "管理员已调整积分余额。", ScopePersonal, CategoryBilling, SeverityInfo, TargetBilling, ownerOrExplicit, payloadFields(payloadFieldResourceDisplayName, payloadFieldTargetID, payloadFieldExplicitRecipientIDs)),
		EventBillingCreditLow:           template("积分余额不足", "积分余额较低，请及时补充。", ScopePersonal, CategoryBilling, SeverityWarning, TargetBilling, ownerOrExplicit, payloadFields(payloadFieldResourceDisplayName, payloadFieldStatusReasonCode, payloadFieldTargetID, payloadFieldExplicitRecipientIDs)),

		EventSystemAnnouncement:        template("系统公告", "你有一条新的系统公告。", ScopeSystem, CategorySystem, SeverityInfo, TargetNone, policies(RecipientSystemAdmins, RecipientExplicitInternalUsers), payloadFields(payloadFieldResourceDisplayName, payloadFieldAnnouncementTitle, payloadFieldAnnouncementBody, payloadFieldAnnouncementSeverity, payloadFieldAnnouncementRoute, payloadFieldExplicitRecipientIDs)),
		EventSystemProviderUnavailable: template("系统服务持续不可用", "系统关键服务持续不可用，请及时处理。", ScopeSystem, CategorySystem, SeverityError, TargetNone, system, payloadFields(payloadFieldResourceDisplayName, payloadFieldStatusReasonCode)),
		EventSystemProviderRecovered:   template("系统服务已恢复", "系统关键服务已恢复。", ScopeSystem, CategorySystem, SeveritySuccess, TargetNone, system, payloadFields(payloadFieldResourceDisplayName)),
		EventSystemOutboxBacklog:       template("通知投递积压", "通知投递出现持续积压，请及时处理。", ScopeSystem, CategorySystem, SeverityWarning, TargetNone, system, payloadFields(payloadFieldResourceDisplayName, payloadFieldStatusReasonCode)),
	}}
}

func (r *TemplateRegistry) Render(event Event) (MessageDraft, error) {
	if err := event.Validate(); err != nil {
		return MessageDraft{}, err
	}
	if r == nil {
		return MessageDraft{}, ErrUnknownEventType
	}
	definition, exists := r.templates[event.EventType]
	if !exists {
		return MessageDraft{}, ErrUnknownEventType
	}
	if err := validateAllowedPayloadFields(
		event.Payload,
		definition.AllowedPayloadFields,
	); err != nil {
		return MessageDraft{}, err
	}
	if _, allowed := definition.Policies[event.RecipientPolicy]; !allowed {
		return MessageDraft{}, fmt.Errorf("%w: recipient policy is not allowed for event", ErrInvalidEvent)
	}
	if definition.Scope == ScopeWorkspace && event.SpaceID <= 0 {
		return MessageDraft{}, fmt.Errorf("%w: workspace event requires space ID", ErrInvalidEvent)
	}
	content := definition.DefaultContent
	title := definition.Title
	severity := definition.Severity
	targetType := definition.TargetType
	targetID := strings.TrimSpace(event.Payload.TargetID)
	routeSpaceID := event.SpaceID
	standardDecorators := true
	if event.EventType == EventSystemAnnouncement &&
		event.Payload.AnnouncementTitle != "" {
		title = event.Payload.AnnouncementTitle
		content = event.Payload.AnnouncementBody
		severity = event.Payload.AnnouncementSeverity
		switch event.Payload.AnnouncementRoute.Type {
		case AnnouncementRouteWorkspaceHome:
			targetType = TargetWorkspace
			routeSpaceID = event.Payload.AnnouncementRoute.SpaceID
			targetID = strconv.FormatInt(routeSpaceID, 10)
		case AnnouncementRouteSystemAnnouncements:
			targetType = TargetSystemAnnouncements
			targetID = ""
		default:
			targetType = TargetNone
			targetID = ""
		}
	}
	if event.EventType == EventWorkspaceMembershipChanged ||
		event.EventType == EventWorkspaceRoleChanged {
		var err error
		title, content, targetType, targetID, err = renderWorkspaceMemberEvent(
			event,
		)
		if err != nil {
			return MessageDraft{}, err
		}
		standardDecorators = false
	}
	if standardDecorators {
		if resourceName := strings.TrimSpace(event.Payload.ResourceDisplayName); resourceName != "" {
			content = fmt.Sprintf("%s：%s", resourceName, content)
		}
		if actorName := strings.TrimSpace(event.Payload.ActorDisplayName); actorName != "" {
			content = fmt.Sprintf("%s 操作人：%s。", content, actorName)
		}
	}
	if standardDecorators && event.Payload.StatusReasonCode != StatusReasonNone {
		reason, exists := reasonContent[event.Payload.StatusReasonCode]
		if !exists {
			return MessageDraft{}, fmt.Errorf("%w: unsupported status reason", ErrInvalidEvent)
		}
		content = fmt.Sprintf("%s 原因：%s。", content, reason)
	}
	if len([]rune(content)) > MaxNotificationContentRunes {
		return MessageDraft{}, fmt.Errorf("%w: rendered content exceeds limit", ErrInvalidEvent)
	}
	if targetType != TargetNone &&
		targetType != TargetSystemAnnouncements &&
		targetID == "" {
		targetID = event.AggregateID
	}
	if targetType == TargetNone {
		targetID = ""
	}
	senderID := event.ActorID
	if definition.Scope == ScopeSystem {
		senderID = 0
	}
	return MessageDraft{
		EventID:    event.EventID,
		Scope:      definition.Scope,
		SpaceID:    routeSpaceID,
		SenderID:   senderID,
		Category:   definition.Category,
		Severity:   severity,
		EventType:  event.EventType,
		Title:      title,
		Content:    content,
		TargetType: targetType,
		TargetID:   targetID,
		CreatedAt:  event.OccurredAt.UnixMilli(),
	}, nil
}

func renderWorkspaceMemberEvent(
	event Event,
) (string, string, TargetType, string, error) {
	workspaceName := strings.TrimSpace(event.Payload.ResourceDisplayName)
	if workspaceName == "" {
		workspaceName = "工作空间"
	}
	subject := strings.TrimSpace(event.Payload.SubjectDisplayName)
	targetType := TargetWorkspace
	targetID := strconv.FormatInt(event.SpaceID, 10)
	if event.Payload.WorkspaceAudience == WorkspaceMemberAudienceTarget {
		switch event.Payload.WorkspaceAction {
		case WorkspaceMemberAdded:
			return "已加入工作空间",
				fmt.Sprintf("你已加入「%s」。", workspaceName),
				targetType, targetID, nil
		case WorkspaceMemberRemoved:
			return "已移出工作空间",
				fmt.Sprintf("你已被移出「%s」。", workspaceName),
				TargetNone, "", nil
		case WorkspaceMemberRoleChanged:
			return "工作空间角色已更新",
				fmt.Sprintf("你在「%s」中的角色已更新。", workspaceName),
				targetType, targetID, nil
		case WorkspaceMemberOwnershipTransferred:
			return "工作空间所有权已转移",
				fmt.Sprintf("你已成为「%s」的所有者。", workspaceName),
				targetType, targetID, nil
		}
	}
	if event.Payload.WorkspaceAudience == WorkspaceMemberAudienceAdmins {
		switch event.Payload.WorkspaceAction {
		case WorkspaceMemberAdded:
			return "工作空间成员已加入",
				fmt.Sprintf("成员「%s」已加入「%s」。", subject, workspaceName),
				targetType, targetID, nil
		case WorkspaceMemberRemoved:
			return "工作空间成员已移出",
				fmt.Sprintf("成员「%s」已移出「%s」。", subject, workspaceName),
				targetType, targetID, nil
		case WorkspaceMemberRoleChanged:
			return "工作空间成员角色已更新",
				fmt.Sprintf("成员「%s」在「%s」中的角色已更新。", subject, workspaceName),
				targetType, targetID, nil
		case WorkspaceMemberOwnershipTransferred:
			return "工作空间所有权已转移",
				fmt.Sprintf("「%s」的所有权已转移给「%s」。", workspaceName, subject),
				targetType, targetID, nil
		}
	}
	return "", "", TargetNone, "", fmt.Errorf(
		"%w: unsupported workspace notification facts",
		ErrInvalidEvent,
	)
}

// ValidateAppendable is the reliable producer boundary for the notification
// core. System administrator routing is allowed because the application
// runtime wires a server-owned persisted recipient source.
func (r *TemplateRegistry) ValidateAppendable(event Event) error {
	if _, err := r.Render(event); err != nil {
		return err
	}
	switch event.RecipientPolicy {
	case RecipientActor, RecipientExplicitInternalUsers, RecipientSystemAdmins:
		return nil
	default:
		return fmt.Errorf(
			"%w: policy %s has no core resolver",
			ErrRecipientPolicyUnavailable,
			event.RecipientPolicy,
		)
	}
}

func template(
	title string,
	content string,
	scope Scope,
	category Category,
	severity Severity,
	target TargetType,
	allowed map[RecipientPolicy]struct{},
	allowedPayloadFields map[payloadField]struct{},
) eventTemplate {
	return eventTemplate{
		Title:                title,
		DefaultContent:       content,
		Scope:                scope,
		Category:             category,
		Severity:             severity,
		TargetType:           target,
		Policies:             allowed,
		AllowedPayloadFields: allowedPayloadFields,
	}
}

func policies(values ...RecipientPolicy) map[RecipientPolicy]struct{} {
	result := make(map[RecipientPolicy]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}

func payloadFields(values ...payloadField) map[payloadField]struct{} {
	result := make(map[payloadField]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}

func validateAllowedPayloadFields(
	payload EventPayload,
	allowed map[payloadField]struct{},
) error {
	if allowed == nil {
		return fmt.Errorf("%w: payload field policy is missing", ErrInvalidEvent)
	}
	provided := map[payloadField]bool{
		payloadFieldResourceDisplayName:  strings.TrimSpace(payload.ResourceDisplayName) != "",
		payloadFieldActorDisplayName:     strings.TrimSpace(payload.ActorDisplayName) != "",
		payloadFieldStatusReasonCode:     payload.StatusReasonCode != StatusReasonNone,
		payloadFieldTargetID:             strings.TrimSpace(payload.TargetID) != "",
		payloadFieldAnnouncementTitle:    strings.TrimSpace(payload.AnnouncementTitle) != "",
		payloadFieldAnnouncementBody:     strings.TrimSpace(payload.AnnouncementBody) != "",
		payloadFieldAnnouncementSeverity: payload.AnnouncementSeverity != "",
		payloadFieldAnnouncementRoute:    payload.AnnouncementRoute != nil,
		payloadFieldWorkspaceAction:      payload.WorkspaceAction != "",
		payloadFieldWorkspaceAudience:    payload.WorkspaceAudience != "",
		payloadFieldSubjectDisplayName:   strings.TrimSpace(payload.SubjectDisplayName) != "",
		payloadFieldExplicitRecipientIDs: len(payload.ExplicitRecipientIDs) > 0,
	}
	for field, present := range provided {
		if !present {
			continue
		}
		if _, exists := allowed[field]; !exists {
			return fmt.Errorf(
				"%w: payload field %s is not allowed for event",
				ErrInvalidEvent,
				field,
			)
		}
	}
	return nil
}
