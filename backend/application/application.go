/*
 * Copyright 2025 coze-dev Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package application

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/middlewares/plantask"

	"github.com/coze-dev/coze-studio/backend/application/permission"

	"github.com/coze-dev/coze-studio/backend/application/admin"
	"github.com/coze-dev/coze-studio/backend/application/agentthread"
	appannouncement "github.com/coze-dev/coze-studio/backend/application/announcement"
	"github.com/coze-dev/coze-studio/backend/application/app"
	appdevapp "github.com/coze-dev/coze-studio/backend/application/appdev"
	"github.com/coze-dev/coze-studio/backend/application/base/appinfra"
	appbilling "github.com/coze-dev/coze-studio/backend/application/billing"
	"github.com/coze-dev/coze-studio/backend/application/connector"
	"github.com/coze-dev/coze-studio/backend/application/conversation"
	appimchannel "github.com/coze-dev/coze-studio/backend/application/imchannel"
	"github.com/coze-dev/coze-studio/backend/application/knowledge"
	"github.com/coze-dev/coze-studio/backend/application/mcptool"
	"github.com/coze-dev/coze-studio/backend/application/memory"
	"github.com/coze-dev/coze-studio/backend/application/modelmgr"
	appnotification "github.com/coze-dev/coze-studio/backend/application/notification"
	"github.com/coze-dev/coze-studio/backend/application/openauth"
	"github.com/coze-dev/coze-studio/backend/application/plugin"
	"github.com/coze-dev/coze-studio/backend/application/prompt"
	appsandbox "github.com/coze-dev/coze-studio/backend/application/sandbox"
	"github.com/coze-dev/coze-studio/backend/application/scheduledtask"
	"github.com/coze-dev/coze-studio/backend/application/search"
	"github.com/coze-dev/coze-studio/backend/application/shortcutcmd"
	"github.com/coze-dev/coze-studio/backend/application/singleagent"
	"github.com/coze-dev/coze-studio/backend/application/skill"
	"github.com/coze-dev/coze-studio/backend/application/template"
	"github.com/coze-dev/coze-studio/backend/application/upload"
	"github.com/coze-dev/coze-studio/backend/application/user"
	"github.com/coze-dev/coze-studio/backend/application/workbench"
	"github.com/coze-dev/coze-studio/backend/application/workflow"
	bizconfig "github.com/coze-dev/coze-studio/backend/bizpkg/config"
	crossagent "github.com/coze-dev/coze-studio/backend/crossdomain/agent"
	singleagentImpl "github.com/coze-dev/coze-studio/backend/crossdomain/agent/impl"
	crossagentrun "github.com/coze-dev/coze-studio/backend/crossdomain/agentrun"
	agentrunImpl "github.com/coze-dev/coze-studio/backend/crossdomain/agentrun/impl"
	crossapp "github.com/coze-dev/coze-studio/backend/crossdomain/app"
	appImpl "github.com/coze-dev/coze-studio/backend/crossdomain/app/impl"
	crossconnector "github.com/coze-dev/coze-studio/backend/crossdomain/connector"
	connectorImpl "github.com/coze-dev/coze-studio/backend/crossdomain/connector/impl"
	crossconversation "github.com/coze-dev/coze-studio/backend/crossdomain/conversation"
	conversationImpl "github.com/coze-dev/coze-studio/backend/crossdomain/conversation/impl"
	crossdatabase "github.com/coze-dev/coze-studio/backend/crossdomain/database"
	databaseImpl "github.com/coze-dev/coze-studio/backend/crossdomain/database/impl"
	crossdatacopy "github.com/coze-dev/coze-studio/backend/crossdomain/datacopy"
	dataCopyImpl "github.com/coze-dev/coze-studio/backend/crossdomain/datacopy/impl"
	crossknowledge "github.com/coze-dev/coze-studio/backend/crossdomain/knowledge"
	knowledgeImpl "github.com/coze-dev/coze-studio/backend/crossdomain/knowledge/impl"
	crossmessage "github.com/coze-dev/coze-studio/backend/crossdomain/message"
	messageImpl "github.com/coze-dev/coze-studio/backend/crossdomain/message/impl"
	crosspermission "github.com/coze-dev/coze-studio/backend/crossdomain/permission"
	permissionImpl "github.com/coze-dev/coze-studio/backend/crossdomain/permission/impl"
	crossplugin "github.com/coze-dev/coze-studio/backend/crossdomain/plugin"
	pluginImpl "github.com/coze-dev/coze-studio/backend/crossdomain/plugin/impl"
	crosssearch "github.com/coze-dev/coze-studio/backend/crossdomain/search"
	searchImpl "github.com/coze-dev/coze-studio/backend/crossdomain/search/impl"
	crossupload "github.com/coze-dev/coze-studio/backend/crossdomain/upload"
	uploadImpl "github.com/coze-dev/coze-studio/backend/crossdomain/upload/impl"
	crossuser "github.com/coze-dev/coze-studio/backend/crossdomain/user"
	crossuserImpl "github.com/coze-dev/coze-studio/backend/crossdomain/user/impl"
	crossvariables "github.com/coze-dev/coze-studio/backend/crossdomain/variables"
	variablesImpl "github.com/coze-dev/coze-studio/backend/crossdomain/variables/impl"
	crossworkflow "github.com/coze-dev/coze-studio/backend/crossdomain/workflow"
	workflowImpl "github.com/coze-dev/coze-studio/backend/crossdomain/workflow/impl"
	threadrepository "github.com/coze-dev/coze-studio/backend/domain/agentthread/repository"
	infraagentthread "github.com/coze-dev/coze-studio/backend/infra/agentthread"
	infraappdev "github.com/coze-dev/coze-studio/backend/infra/appdev"
	"github.com/coze-dev/coze-studio/backend/infra/checkpoint"
	"github.com/coze-dev/coze-studio/backend/infra/document/progressbar"
	progressBarImpl "github.com/coze-dev/coze-studio/backend/infra/document/progressbar/impl/progressbar"
	"github.com/coze-dev/coze-studio/backend/infra/eventbus"
	implEventbus "github.com/coze-dev/coze-studio/backend/infra/eventbus/impl"
	infranotification "github.com/coze-dev/coze-studio/backend/infra/notification"
	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
	"github.com/coze-dev/coze-studio/backend/infra/sqlparser"
	sqlparserImpl "github.com/coze-dev/coze-studio/backend/infra/sqlparser/impl/sqlparser"
	"github.com/coze-dev/coze-studio/backend/pkg/ctxcache"
)

type eventbusImpl struct {
	resourceEventBus search.ResourceEventBus
	projectEventBus  search.ProjectEventBus
}

type basicServices struct {
	infra                 *appinfra.AppDependencies
	eventbus              *eventbusImpl
	appDevProviderRuntime *appDevProviderRuntime
	modelMgrSVC           *modelmgr.ModelmgrApplicationService
	connectorSVC          *connector.ConnectorApplicationService
	userSVC               *user.UserApplicationService
	promptSVC             *prompt.PromptApplicationService
	templateSVC           *template.ApplicationService
	openAuthSVC           *openauth.OpenAuthApplicationService
	uploadSVC             *upload.UploadService

	permissionSVC *permission.PermissionApplicationService
}

type primaryServices struct {
	basicServices *basicServices
	infra         *appinfra.AppDependencies

	pluginSVC            *plugin.PluginApplicationService
	memorySVC            *memory.MemoryApplicationServices
	knowledgeSVC         *knowledge.KnowledgeApplicationService
	workflowSVC          *workflow.ApplicationService
	shortcutSVC          *shortcutcmd.ShortcutCmdApplicationService
	agentThreadSVC       *agentthread.ApplicationService
	skillSVC             *skill.ApplicationService
	mcpToolSVC           *mcptool.ApplicationService
	mcpManagementRuntime *mcpManagementRuntime
	scheduledTaskSVC     *scheduledtask.ApplicationService
	scheduledTaskWorker  *scheduledtask.Worker
	notificationRuntime  *appnotification.Runtime
	announcementWorker   *appannouncement.Worker
	sandboxHealthMonitor *appsandbox.HealthMonitor
	workbenchSVC         *workbench.ApplicationService
	appSVC               *app.APPApplicationService
}

type complexServices struct {
	primaryServices *primaryServices
	singleAgentSVC  *singleagent.SingleAgentApplicationService
	appSVC          *app.APPApplicationService
	searchSVC       *search.SearchApplicationService
	conversationSVC *conversation.ConversationApplicationService
}

func Init(ctx context.Context) (err error) {
	ctx = ctxcache.Init(ctx)
	infra, err := appinfra.Init(ctx)
	if err != nil {
		return err
	}

	progressbar.New = progressBarImpl.NewProgressBar
	sqlparser.New = sqlparserImpl.NewSQLParser

	eventbus := initEventBus(infra)

	basicServices, err := initBasicServices(ctx, infra, eventbus)
	if err != nil {
		return fmt.Errorf("Init - initBasicServices failed, err: %v", err)
	}
	defer func() {
		if err == nil {
			return
		}
		if basicServices.appDevProviderRuntime != nil {
			rollbackCtx, cancel := context.WithTimeout(
				context.WithoutCancel(ctx),
				defaultApplicationShutdownAttemptTimeout,
			)
			defer cancel()
			if shutdownErr := basicServices.appDevProviderRuntime.Shutdown(rollbackCtx); shutdownErr == nil {
				applicationShutdowns.Unregister(basicServices.appDevProviderRuntime.shutdownOwner)
			}
		}
		clearSandboxControlPlane()
	}()
	mcpRuntimeConfig, err := agentthread.ADKMCPRuntimeBootstrapConfigFromEnv()
	if err != nil {
		return fmt.Errorf("Init - configure agent mcp runtime: %w", err)
	}
	mcpManagementEnabled, err := mcpManagementEnabledFromEnv(mcpRuntimeConfig.Enabled)
	if err != nil {
		return fmt.Errorf("Init - configure mcp management: %w", err)
	}

	primaryServices, err := initPrimaryServices(ctx, basicServices, mcpManagementEnabled)
	if err != nil {
		return fmt.Errorf("Init - initPrimaryServices failed, err: %v", err)
	}
	complexServices, err := initComplexServices(ctx, primaryServices)
	if err != nil {
		return fmt.Errorf("Init - initVitalServices failed, err: %v", err)
	}
	runtimePolicy, err := agentthread.RuntimePolicyFromEnv()
	if err != nil {
		return fmt.Errorf("Init - configure agent runtime policy: %w", err)
	}
	primaryServices.agentThreadSVC.RuntimePolicy = &runtimePolicy
	runtimeSkillProvider := agentthread.NewRuntimeSkillProvider(
		primaryServices.skillSVC.DomainSVC,
	)
	legacyAgentRunExecutor := agentthread.NewApplicationHarnessExecutor(
		primaryServices.agentThreadSVC,
		runtimeSkillProvider,
	)
	adkCancelRegistry := agentthread.NewADKCancelRegistry()
	primaryServices.agentThreadSVC.ADKCancelRegistry = adkCancelRegistry
	adkEventSink := agentthread.NewApplicationRunEventSink(
		primaryServices.agentThreadSVC,
	)
	guardrailEnforcer, guardrailStatus := agentthread.NewGuardrailEnforcerFromEnv(
		primaryServices.agentThreadSVC.GuardrailAuditRepository,
		infra.IDGenSVC,
	)
	primaryServices.agentThreadSVC.GuardrailProviderStatus = guardrailStatus
	webSearchBackend, _, err := agentthread.ADKWebSearchBackendFromEnv()
	if err != nil {
		return fmt.Errorf("Init - configure agent web search backend: %w", err)
	}
	adkRuntimeFileRegistry := agentthread.NewApplicationADKRuntimeFileRegistry(
		primaryServices.agentThreadSVC,
	)
	adkOffloadBackendFactory := agentthread.ADKOffloadBackendFactoryFunc(
		func(
			_ context.Context,
			run *agentthread.RunSummary,
			limits agentthread.ADKOffloadLimits,
		) (*agentthread.ADKOffloadBackend, error) {
			return agentthread.NewADKOffloadBackend(
				run,
				infra.OSS,
				adkRuntimeFileRegistry,
				adkEventSink,
				limits,
			)
		},
	)
	mcpWorkdirManager, mcpWorkdirPreparer, err := newMCPRuntimeSharedWorkdir(mcpRuntimeConfig)
	if err != nil {
		return fmt.Errorf("Init - configure mcp workdir runtime: %w", err)
	}
	mcpManagementRuntime, err := bindMCPManagementRuntime(
		primaryServices.mcpToolSVC,
		mcpRuntimeConfig,
		mcpWorkdirManager,
	)
	if err != nil {
		if mcpWorkdirManager != nil {
			_ = mcpWorkdirManager.Close()
		}
		return fmt.Errorf("Init - bind mcp management runtime: %w", err)
	}
	primaryServices.mcpManagementRuntime = mcpManagementRuntime
	if mcpManagementRuntime != nil || mcpWorkdirManager != nil {
		if _, err := applicationShutdowns.Register(&mcpManagementRuntimeLifecycle{
			runtime: mcpManagementRuntime, workdirManager: mcpWorkdirManager,
		}); err != nil {
			if mcpManagementRuntime != nil {
				_ = mcpManagementRuntime.Shutdown(context.Background())
			}
			if mcpWorkdirManager != nil {
				_ = mcpWorkdirManager.Close()
			}
			return fmt.Errorf("Init - register mcp management runtime shutdown: %w", err)
		}
	}
	mcpWorkdirLeaseRepository := threadrepository.NewMCPRuntimeWorkdirLeaseRepository(
		infra.DB,
	)
	mcpRuntimeAuditRecorder := agentthread.NewApplicationADKMCPRuntimeAuditRecorder(
		agentthread.ApplicationADKMCPRuntimeAuditRecorderOptions{
			Repository: threadrepository.NewMCPRuntimeAuditRepository(infra.DB),
			IDGen:      infra.IDGenSVC,
		},
	)
	mcpRuntimeHealthReporter := agentthread.ADKMCPRuntimeHealthReporterFunc(
		func(
			ctx context.Context,
			report agentthread.ADKMCPRuntimeHealthReport,
		) error {
			return primaryServices.mcpToolSVC.RecordRuntimeHealth(
				ctx,
				mcptool.MCPRuntimeHealthReport{
					ServerID:          report.ServerID,
					ExpectedUpdatedAt: report.ExpectedUpdatedAt,
					Success:           report.Success,
					ErrorCode:         report.ErrorCode,
					LatencyMs:         report.LatencyMs,
				},
			)
		},
	)
	mcpRuntimeOutputOffloader := agentthread.NewADKMCPRuntimeOutputOffloadBackendAdapter(
		agentthread.ADKMCPRuntimeOutputOffloadBackendAdapterOptions{
			BackendFactory: adkOffloadBackendFactory,
		},
	)
	mcpRuntimeExecutor, err := agentthread.NewADKMCPRuntimeToolExecutorFromConfigStrict(
		agentthread.ADKMCPRuntimeBootstrapDependencies{
			Resolver:             primaryServices.mcpToolSVC,
			LeaseRepository:      mcpWorkdirLeaseRepository,
			IDGen:                infra.IDGenSVC,
			WorkdirPreparer:      mcpWorkdirPreparer,
			SandboxBindingSource: sandboxMCPRuntimeBindingSource{},
			EventSink:            adkEventSink,
			AuditRecorder:        mcpRuntimeAuditRecorder,
			HealthReporter:       mcpRuntimeHealthReporter,
			OutputOffloader:      mcpRuntimeOutputOffloader,
			Config:               mcpRuntimeConfig,
		},
	)
	if err != nil {
		return fmt.Errorf("Init - bind mcp runtime executor: %w", err)
	}
	adkContextStore := agentthread.NewApplicationADKContextStore(
		primaryServices.agentThreadSVC,
	)
	adkPlanStore := agentthread.NewApplicationADKPlanStore(
		primaryServices.agentThreadSVC,
	)
	adaptiveExecutionRepository := threadrepository.NewAdaptiveExecutionRepository(infra.DB)
	adkAgentRunExecutor := agentthread.NewADKExecutor(
		agentthread.NewApplicationADKAgentFactory(
			nil,
			agentthread.NewDefaultADKToolProviderWithSingleAgentSubagents(
				complexServices.singleAgentSVC.DomainSVC,
				agentthread.WithDefaultADKToolProviderMCPRegistry(
					primaryServices.mcpToolSVC,
				),
				agentthread.WithDefaultADKToolProviderMCPExecutor(
					mcpRuntimeExecutor,
				),
				agentthread.WithDefaultADKToolProviderEventSink(adkEventSink),
				agentthread.WithDefaultADKToolProviderSubagentRunRecorder(
					agentthread.NewApplicationADKSubagentRunRecorder(
						primaryServices.agentThreadSVC,
					),
				),
				agentthread.WithDefaultADKToolProviderGuardrailEnforcer(
					guardrailEnforcer,
				),
				agentthread.WithDefaultADKToolProviderWebSearchBackend(
					webSearchBackend,
				),
				agentthread.WithDefaultADKToolProviderArtifactApp(
					primaryServices.agentThreadSVC,
				),
			),
			agentthread.NewADKMiddlewareAssembler(agentthread.ADKMiddlewareAssemblerOptions{
				MemoryProvider: agentthread.NewThreadMemoryProvider(
					primaryServices.agentThreadSVC,
					32,
				),
				GuardrailEnforcer:      guardrailEnforcer,
				SkillProvider:          runtimeSkillProvider,
				TranscriptStore:        adkContextStore,
				MemoryFlushQueue:       adkContextStore,
				EventSink:              adkEventSink,
				JournalContentProducer: primaryServices.agentThreadSVC,
				PlanBackendFactory: agentthread.ADKPlanBackendFactoryFunc(
					func(
						ctx context.Context,
						run *agentthread.RunSummary,
					) (plantask.Backend, error) {
						return agentthread.NewADKPlanBackend(
							run,
							adkPlanStore,
							adkEventSink,
							agentthread.WithADKAdaptivePlanBoundaryCoordinator(
								agentthread.ADKAdaptivePlanBoundaryCoordinatorFromContext(ctx),
							),
						)
					},
				),
				OffloadBackendFactory: adkOffloadBackendFactory,
			}),
			agentthread.WithADKLeadPromptOverlayProvider(
				agentthread.NewADKSingleAgentLeadPromptOverlayProvider(
					complexServices.singleAgentSVC.DomainSVC,
				),
			),
		),
		adkEventSink,
		func(run *agentthread.RunSummary) (adk.CheckPointStore, error) {
			return agentthread.NewADKCheckpointStore(
				primaryServices.agentThreadSVC,
				run,
				agentthread.WithADKJournalCheckpointStateReader(
					primaryServices.agentThreadSVC.JournalRecoveryRepository,
				),
				agentthread.WithADKSideEffectBoundary(
					primaryServices.agentThreadSVC.JournalSideEffectRepository,
					infra.IDGenSVC,
				),
				agentthread.WithADKAdaptivePlanBoundary(
					adaptiveExecutionRepository,
					infra.IDGenSVC,
					adkPlanStore,
				),
			)
		},
		agentthread.NewThreadUsageCollectorWithOptions(
			primaryServices.agentThreadSVC,
			agentthread.ThreadUsageCollectorOptions{EventSink: adkEventSink},
		),
		agentthread.WithADKCancelRegistry(adkCancelRegistry),
		agentthread.WithADKSubagentRetrySourceResolver(
			agentthread.NewApplicationADKSubagentRetrySourceResolver(
				primaryServices.agentThreadSVC,
			),
		),
		agentthread.WithADKAdaptiveBootstrapCoordinator(
			agentthread.NewAdaptiveBootstrapCoordinator(
				agentthread.AdaptiveBootstrapCoordinatorOptions{
					AttemptReader:   primaryServices.agentThreadSVC.JournalRecoveryRepository,
					Repository:      adaptiveExecutionRepository,
					SourceRunReader: primaryServices.agentThreadSVC.ThreadSVC,
					IDGen:           infra.IDGenSVC,
					Now:             func() int64 { return time.Now().UnixMilli() },
				},
			),
		),
	)
	agentRunExecutor := agentthread.NewRuntimeSelector(
		legacyAgentRunExecutor,
		adkAgentRunExecutor,
		runtimePolicy,
	)
	agentResumeRunExecutor := agentthread.NewRuntimeResumeSelector(
		legacyAgentRunExecutor,
		adkAgentRunExecutor,
		runtimePolicy,
	)
	agentthread.StartRunWorkerFromEnv(ctx, primaryServices.agentThreadSVC, agentRunExecutor)
	agentthread.StartResumeRunWorkerFromEnv(ctx, primaryServices.agentThreadSVC, agentResumeRunExecutor)
	agentthread.StartRunLeaseRecoveryWorkerFromEnv(ctx, primaryServices.agentThreadSVC)
	agentthread.StartMemoryFlushWorkerFromEnv(ctx, primaryServices.agentThreadSVC)
	agentthread.StartArtifactScanWorkerFromEnv(ctx, primaryServices.agentThreadSVC)
	primaryServices.scheduledTaskWorker.Start(ctx)
	agentthread.StartGuardrailAuditArchiveWorkerFromEnv(
		ctx,
		primaryServices.agentThreadSVC.GuardrailAuditRepository,
		primaryServices.infra.OSS,
	)
	agentthread.StartGuardrailAuditRetentionWorkerFromEnv(
		ctx,
		primaryServices.agentThreadSVC.GuardrailAuditRepository,
		primaryServices.infra.OSS,
	)
	_, journalRetentionStatus := agentthread.StartJournalRetentionWorkerFromEnvWithStatus(
		ctx,
		primaryServices.agentThreadSVC.JournalRetentionRepository,
		primaryServices.agentThreadSVC.JournalSnapshotObjectStorage,
		primaryServices.agentThreadSVC.JournalMetrics,
	)
	if journalRetentionStatus.Enabled && !journalRetentionStatus.Started {
		return fmt.Errorf("Init - start Journal retention worker: %s", journalRetentionStatus.Reason)
	}
	_, mcpWorkdirReaperStatus := agentthread.StartADKMCPRuntimeStdioWorkdirLeaseReaperWorkerFromEnvWithStatus(
		ctx,
		mcpWorkdirLeaseRepository,
		mcpWorkdirPreparer,
	)
	if mcpWorkdirReaperStatus.Enabled && !mcpWorkdirReaperStatus.Started {
		return fmt.Errorf("Init - start mcp stdio workdir lease reaper: %s", mcpWorkdirReaperStatus.Reason)
	}

	// Initialize permission service first as it's required by other services
	crosspermission.SetDefaultSVC(permissionImpl.InitDomainService(basicServices.permissionSVC.DomainSVC))

	crossconnector.SetDefaultSVC(connectorImpl.InitDomainService(basicServices.connectorSVC.DomainSVC))
	crossdatabase.SetDefaultSVC(databaseImpl.InitDomainService(primaryServices.memorySVC.DatabaseDomainSVC))
	crossknowledge.SetDefaultSVC(knowledgeImpl.InitDomainService(primaryServices.knowledgeSVC.DomainSVC))
	crossplugin.SetDefaultSVC(pluginImpl.InitDomainService(primaryServices.pluginSVC.DomainSVC, infra.OSS))
	crossvariables.SetDefaultSVC(variablesImpl.InitDomainService(primaryServices.memorySVC.VariablesDomainSVC))
	crossworkflow.SetDefaultSVC(workflowImpl.InitDomainService(primaryServices.workflowSVC.DomainSVC))
	crossconversation.SetDefaultSVC(conversationImpl.InitDomainService(complexServices.conversationSVC.ConversationDomainSVC))
	crossmessage.SetDefaultSVC(messageImpl.InitDomainService(complexServices.conversationSVC.MessageDomainSVC))
	crossagentrun.SetDefaultSVC(agentrunImpl.InitDomainService(complexServices.conversationSVC.AgentRunDomainSVC))
	crossagent.SetDefaultSVC(singleagentImpl.InitDomainService(complexServices.singleAgentSVC.DomainSVC))
	crossuser.SetDefaultSVC(crossuserImpl.InitDomainService(basicServices.userSVC.DomainSVC))
	crossdatacopy.SetDefaultSVC(dataCopyImpl.InitDomainService(basicServices.infra))
	crosssearch.SetDefaultSVC(searchImpl.InitDomainService(complexServices.searchSVC.DomainSVC))
	crossupload.SetDefaultSVC(uploadImpl.InitDomainService(basicServices.uploadSVC.UploadSVC))

	crossapp.SetDefaultSVC(appImpl.InitDomainService(complexServices.appSVC.DomainSVC))
	if primaryServices.notificationRuntime == nil {
		return fmt.Errorf("Init - notification runtime is unavailable")
	}
	billingMaintenanceWorker := appbilling.NewMaintenanceWorkerFromEnv(
		appbilling.DefaultService(),
	)
	producers := make([]applicationShutdownHook, 0, 3)
	if primaryServices.announcementWorker != nil {
		producers = append(producers, primaryServices.announcementWorker)
	}
	if billingMaintenanceWorker != nil {
		producers = append(producers, billingMaintenanceWorker)
	}
	if primaryServices.sandboxHealthMonitor != nil {
		producers = append(producers, primaryServices.sandboxHealthMonitor)
	}
	if err := registerNotificationLifecycleHooks(
		applicationShutdowns,
		primaryServices.notificationRuntime,
		producers...,
	); err != nil {
		return fmt.Errorf("Init - register notification lifecycle shutdown: %w", err)
	}
	primaryServices.notificationRuntime.Start(ctx)
	if primaryServices.announcementWorker != nil {
		primaryServices.announcementWorker.Start(ctx)
	}
	if billingMaintenanceWorker != nil {
		billingMaintenanceWorker.Start(ctx)
	}
	if primaryServices.sandboxHealthMonitor != nil {
		primaryServices.sandboxHealthMonitor.Start(ctx)
	}

	return nil
}

func registerNotificationLifecycleHooks(
	registry *applicationShutdownRegistry,
	consumer applicationShutdownHook,
	producers ...applicationShutdownHook,
) error {
	if registry == nil || consumer == nil {
		return errApplicationShutdownFailed
	}
	if _, err := registry.Register(consumer); err != nil {
		return err
	}
	for _, producer := range producers {
		if producer == nil {
			continue
		}
		if _, err := registry.Register(producer); err != nil {
			return err
		}
	}
	return nil
}

func initEventBus(infra *appinfra.AppDependencies) *eventbusImpl {
	e := &eventbusImpl{}
	eventbus.SetDefaultSVC(implEventbus.NewConsumerService())
	e.resourceEventBus = search.NewResourceEventBus(infra.ResourceEventProducer)
	e.projectEventBus = search.NewProjectEventBus(infra.AppEventProducer)

	return e
}

// initBasicServices init basic services that only depends on infra.
func initBasicServices(ctx context.Context, infra *appinfra.AppDependencies, e *eventbusImpl) (*basicServices, error) {
	uploadSVC := upload.InitService(&upload.UploadComponents{Cache: infra.CacheCli, Oss: infra.OSS, DB: infra.DB, Idgen: infra.IDGenSVC})
	openAuthSVC := openauth.InitService(infra.DB, infra.IDGenSVC)
	promptSVC := prompt.InitService(infra.DB, infra.IDGenSVC, e.resourceEventBus)
	modelMgrSVC := modelmgr.InitService(infra.OSS)
	connectorSVC := connector.InitService(infra.OSS)
	userSVC := user.InitService(ctx, infra.DB, infra.OSS, infra.IDGenSVC)
	admin.InitService(userSVC.DomainSVC)
	appbilling.InitService(infra.DB, infra.IDGenSVC)
	if err := initSandboxControlPlaneForApplication(infra); err != nil {
		return nil, err
	}
	appDevStore := infraappdev.NewPersistentStore(infra.DB, infra.OSS)
	appDevService := appdevapp.InitService(
		appDevStore,
		nil,
		infraappdev.NewChatBrokerWithPersistence(infraappdev.NewPersistentChatRepository(infra.DB)),
	)
	if err := appDevService.SetProjectRuntimeLifecycleMode(appDevProjectRuntimeLifecycleMode(os.Getenv)); err != nil {
		clearSandboxControlPlane()
		return nil, err
	}
	appDevProviderRuntime, err := initAppDevProviderRuntimeForApplication(ctx, appDevProviderWiringDependencies{
		Infra: infra, Service: appDevService, Store: appDevStore,
		Providers: SandboxRuntimeRepository, Router: SandboxRouter,
	})
	if err != nil {
		clearSandboxControlPlane()
		return nil, err
	}
	templateSVC := template.InitService(ctx, &template.ServiceComponents{
		DB:      infra.DB,
		IDGen:   infra.IDGenSVC,
		Storage: infra.OSS,
	})

	permissionSVC := permission.InitService(&permission.ServiceComponents{})

	return &basicServices{
		infra:                 infra,
		eventbus:              e,
		appDevProviderRuntime: appDevProviderRuntime,
		modelMgrSVC:           modelMgrSVC,
		connectorSVC:          connectorSVC,
		userSVC:               userSVC,
		promptSVC:             promptSVC,
		templateSVC:           templateSVC,
		openAuthSVC:           openAuthSVC,
		uploadSVC:             uploadSVC,

		permissionSVC: permissionSVC,
	}, nil
}

const mcpManagementEnabledEnv = "MCP_MANAGEMENT_ENABLED"

func mcpManagementEnabledFromEnv(runtimeEnabled bool) (bool, error) {
	value := strings.TrimSpace(os.Getenv(mcpManagementEnabledEnv))
	if value == "" {
		return runtimeEnabled, nil
	}
	enabled, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("%s must be a boolean", mcpManagementEnabledEnv)
	}
	return enabled, nil
}

func mcpCatalogOptionsFromEnv(enabled bool) ([]mcptool.MySQLCatalogOption, error) {
	if !enabled {
		return nil, nil
	}
	secret := os.Getenv(mcptool.MCPAESAuthSecretEnv)
	if secret == "" {
		return nil, fmt.Errorf("%s is required when MCP management/runtime is enabled", mcptool.MCPAESAuthSecretEnv)
	}
	codec, err := mcptool.NewAESMCPAuthCodec(secret)
	if err != nil {
		return nil, fmt.Errorf("%s must contain a valid 16, 24, or 32-byte key: %w", mcptool.MCPAESAuthSecretEnv, err)
	}

	return []mcptool.MySQLCatalogOption{mcptool.WithMySQLCatalogAuthCodec(codec)}, nil
}

// initPrimaryServices init primary services that depends on basic services.
func initPrimaryServices(ctx context.Context, basicServices *basicServices, mcpManagementEnabled bool) (*primaryServices, error) {
	pluginSVC, err := plugin.InitService(ctx, basicServices.toPluginServiceComponents())
	if err != nil {
		return nil, err
	}

	memorySVC := memory.InitService(basicServices.toMemoryServiceComponents())

	knowledgeSVC, err := knowledge.InitService(ctx,
		basicServices.toKnowledgeServiceComponents(memorySVC),
		basicServices.eventbus.resourceEventBus)
	if err != nil {
		return nil, err
	}

	workflowDomainSVC, err := workflow.InitService(ctx,
		basicServices.toWorkflowServiceComponents(pluginSVC, memorySVC, knowledgeSVC))
	if err != nil {
		return nil, err
	}

	shortcutSVC := shortcutcmd.InitService(basicServices.infra.DB, basicServices.infra.IDGenSVC)
	agentThreadSVC := agentthread.InitService(&agentthread.ServiceComponents{
		DB:              basicServices.infra.DB,
		IDGen:           basicServices.infra.IDGenSVC,
		ObjectStorage:   basicServices.infra.OSS,
		UserSpaceReader: basicServices.userSVC.DomainSVC,
	})
	agentThreadSVC.JournalFeatureGate = agentthread.NewJournalFeatureGate(
		bizconfig.Base(),
		agentthread.JournalFeatureGateOptions{},
	)
	agentThreadSVC.JournalMetrics = agentthread.NewJournalPrometheusMetricsCollectorFromEnv()
	agentThreadSVC.JournalTelemetry = agentthread.NewJournalLogTelemetry()
	if strings.EqualFold(strings.TrimSpace(os.Getenv("APP_ENV")), "production") {
		journalLimiter, limiterErr := infraagentthread.NewRedisJournalRateLimiter(
			basicServices.infra.CacheCli,
			bizconfig.Base(),
			infraagentthread.RedisJournalRateLimiterOptions{Environment: "production"},
		)
		if limiterErr != nil {
			return nil, fmt.Errorf("init Journal admission limiter: %w", limiterErr)
		}
		agentThreadSVC.JournalAdmissionLimiter = journalLimiter
		agentThreadSVC.JournalAdmissionRequired = true
	}
	mcpCatalogOptions, err := mcpCatalogOptionsFromEnv(mcpManagementEnabled)
	if err != nil {
		return nil, err
	}
	mcpToolSVC := mcptool.InitService(&mcptool.Components{
		Enabled:                     &mcpManagementEnabled,
		Catalog:                     mcptool.NewMySQLCatalog(basicServices.infra.DB, mcpCatalogOptions...),
		AuditRepository:             mcptool.NewMySQLManagementAuditRepository(basicServices.infra.DB),
		IDGen:                       basicServices.infra.IDGenSVC,
		UserSpaceRoleReader:         basicServices.userSVC.DomainSVC,
		SpaceMemberRoleReader:       mcptool.NewMySQLSpaceMemberRoleReader(basicServices.infra.DB),
		DefaultDeerFlowMCPConfigRaw: mcptool.DefaultDeerFlowMCPConfigRaw(),
	})
	skillSVC := skill.InitService(&skill.ServiceComponents{
		DB:                    basicServices.infra.DB,
		IDGen:                 basicServices.infra.IDGenSVC,
		CodeRunner:            basicServices.infra.CodeRunner,
		ToolCandidateProvider: mcpToolSVC,
		UserSpaceReader:       basicServices.userSVC.DomainSVC,
	})
	notificationRuntime, err := appnotification.NewRuntime(
		basicServices.infra.DB,
		basicServices.infra.IDGenSVC,
		appnotification.PolicyRecipientResolver{
			SystemAdmins: infranotification.NewMySQLSystemAdminRecipientSource(
				basicServices.infra.DB,
				bizconfig.SystemAdminEmails(),
			),
		},
	)
	if err != nil {
		return nil, fmt.Errorf("init notification runtime: %w", err)
	}
	notificationOutbox := notificationRuntime.Service()
	if notificationOutbox == nil || !notificationOutbox.IsConfigured() {
		return nil, fmt.Errorf("init notification runtime: service is unavailable")
	}
	announcementSVC := appannouncement.InitService(
		basicServices.infra.DB,
		basicServices.infra.IDGenSVC,
	)
	if announcementSVC == nil || !announcementSVC.IsConfigured() {
		return nil, fmt.Errorf("init announcement service: service is unavailable")
	}
	announcementWorker := appannouncement.NewWorker(
		announcementSVC,
		appannouncement.DefaultWorkerOptions(),
	)
	var sandboxHealthMonitor *appsandbox.HealthMonitor
	if SandboxSVC != nil {
		sandboxHealthMonitor, err = newSandboxHealthMonitor(
			SandboxSVC,
			infrasandbox.NewMySQLHealthMonitorRepository(
				basicServices.infra.DB,
				notificationOutbox,
			),
		)
		if err != nil {
			return nil, fmt.Errorf("init sandbox health monitor: %w", err)
		}
	}
	if _, _, err := appimchannel.InitService(&appimchannel.Components{
		DB:                            basicServices.infra.DB,
		IDGen:                         basicServices.infra.IDGenSVC,
		Roles:                         basicServices.userSVC.DomainSVC,
		AgentThreads:                  agentThreadSVC,
		RootContext:                   ctx,
		RuntimeStableFailureThreshold: appimchannel.RuntimeStableFailureThresholdFromEnv(os.Getenv),
	}); err != nil {
		return nil, fmt.Errorf("init Feishu IM channel service: %w", err)
	}
	scheduledTaskSVC, scheduledTaskWorker, err := scheduledtask.InitService(&scheduledtask.ServiceComponents{
		DB:                 basicServices.infra.DB,
		IDGen:              basicServices.infra.IDGenSVC,
		UserSpaceReader:    basicServices.userSVC.DomainSVC,
		UserProfileReader:  basicServices.userSVC.DomainSVC,
		AgentThreadClient:  agentThreadSVC,
		WorkflowDomain:     workflowDomainSVC.DomainSVC,
		NotificationOutbox: notificationOutbox,
		RootContext:        ctx,
	})
	if err != nil {
		return nil, err
	}
	workbenchSVC := workbench.InitService(&workbench.ServiceComponents{
		SkillSVC:          skillSVC,
		MCPToolSVC:        mcpToolSVC,
		SandboxRepository: SandboxRuntimeRepository,
	})

	return &primaryServices{
		basicServices:        basicServices,
		pluginSVC:            pluginSVC,
		memorySVC:            memorySVC,
		knowledgeSVC:         knowledgeSVC,
		workflowSVC:          workflowDomainSVC,
		shortcutSVC:          shortcutSVC,
		agentThreadSVC:       agentThreadSVC,
		skillSVC:             skillSVC,
		mcpToolSVC:           mcpToolSVC,
		scheduledTaskSVC:     scheduledTaskSVC,
		scheduledTaskWorker:  scheduledTaskWorker,
		notificationRuntime:  notificationRuntime,
		announcementWorker:   announcementWorker,
		sandboxHealthMonitor: sandboxHealthMonitor,
		workbenchSVC:         workbenchSVC,
		infra:                basicServices.infra,
	}, nil
}

func newSandboxHealthMonitor(
	service *appsandbox.Service,
	repository *infrasandbox.MySQLHealthMonitorRepository,
) (*appsandbox.HealthMonitor, error) {
	if service == nil {
		return nil, nil
	}
	return appsandbox.NewHealthMonitor(
		service,
		repository,
		appsandbox.DefaultHealthMonitorOptions(),
	)
}

// initComplexServices init complex services that depends on primary services.
func initComplexServices(ctx context.Context, p *primaryServices) (*complexServices, error) {
	singleAgentSVC, err := singleagent.InitService(p.toSingleAgentServiceComponents())
	if err != nil {
		return nil, err
	}

	appSVC, err := app.InitService(p.toAPPServiceComponents())
	if err != nil {
		return nil, err
	}

	searchSVC, err := search.InitService(ctx, p.toSearchServiceComponents(singleAgentSVC, appSVC))
	if err != nil {
		return nil, err
	}

	conversationSVC := conversation.InitService(p.toConversationComponents(singleAgentSVC))

	return &complexServices{
		primaryServices: p,
		singleAgentSVC:  singleAgentSVC,
		appSVC:          appSVC,
		searchSVC:       searchSVC,
		conversationSVC: conversationSVC,
	}, nil
}

func (b *basicServices) toPluginServiceComponents() *plugin.ServiceComponents {
	return &plugin.ServiceComponents{
		IDGen:    b.infra.IDGenSVC,
		DB:       b.infra.DB,
		EventBus: b.eventbus.resourceEventBus,
		OSS:      b.infra.OSS,
		UserSVC:  b.userSVC.DomainSVC,
		CacheCli: b.infra.CacheCli,
	}
}

func (b *basicServices) toKnowledgeServiceComponents(memoryService *memory.MemoryApplicationServices) *knowledge.ServiceComponents {
	return &knowledge.ServiceComponents{
		DB:                  b.infra.DB,
		IDGen:               b.infra.IDGenSVC,
		RDB:                 memoryService.RDBDomainSVC,
		Producer:            b.infra.KnowledgeEventProducer,
		SearchStoreManagers: b.infra.SearchStoreManagers,
		ParseManager:        b.infra.ParserManager,
		Storage:             b.infra.OSS,
		Rewriter:            b.infra.Rewriter,
		Reranker:            b.infra.Reranker,
		NL2Sql:              b.infra.NL2SQL,
		CacheCli:            b.infra.CacheCli,
	}
}

func (b *basicServices) toMemoryServiceComponents() *memory.ServiceComponents {
	return &memory.ServiceComponents{
		IDGen:                  b.infra.IDGenSVC,
		DB:                     b.infra.DB,
		EventBus:               b.eventbus.resourceEventBus,
		TosClient:              b.infra.OSS,
		ResourceDomainNotifier: b.eventbus.resourceEventBus,
		CacheCli:               b.infra.CacheCli,
	}
}

func (b *basicServices) toWorkflowServiceComponents(pluginSVC *plugin.PluginApplicationService, memorySVC *memory.MemoryApplicationServices, knowledgeSVC *knowledge.KnowledgeApplicationService) *workflow.ServiceComponents {
	return &workflow.ServiceComponents{
		IDGen:                    b.infra.IDGenSVC,
		DB:                       b.infra.DB,
		Cache:                    b.infra.CacheCli,
		Tos:                      b.infra.OSS,
		ImageX:                   b.infra.ImageXClient,
		DatabaseDomainSVC:        memorySVC.DatabaseDomainSVC,
		VariablesDomainSVC:       memorySVC.VariablesDomainSVC,
		PluginDomainSVC:          pluginSVC.DomainSVC,
		KnowledgeDomainSVC:       knowledgeSVC.DomainSVC,
		DomainNotifier:           b.eventbus.resourceEventBus,
		CPStore:                  checkpoint.NewRedisStore(b.infra.CacheCli),
		CodeRunner:               b.infra.CodeRunner,
		WorkflowBuildInChatModel: b.infra.WorkflowBuildInChatModel,
	}
}

func (p *primaryServices) toSingleAgentServiceComponents() *singleagent.ServiceComponents {
	return &singleagent.ServiceComponents{
		IDGen:                p.basicServices.infra.IDGenSVC,
		DB:                   p.basicServices.infra.DB,
		Cache:                p.basicServices.infra.CacheCli,
		TosClient:            p.basicServices.infra.OSS,
		ImageX:               p.basicServices.infra.ImageXClient,
		UserDomainSVC:        p.basicServices.userSVC.DomainSVC,
		EventBus:             p.basicServices.eventbus.projectEventBus,
		DatabaseDomainSVC:    p.memorySVC.DatabaseDomainSVC,
		ConnectorDomainSVC:   p.basicServices.connectorSVC.DomainSVC,
		KnowledgeDomainSVC:   p.knowledgeSVC.DomainSVC,
		PluginDomainSVC:      p.pluginSVC.DomainSVC,
		WorkflowDomainSVC:    p.workflowSVC.DomainSVC,
		VariablesDomainSVC:   p.memorySVC.VariablesDomainSVC,
		ShortcutCMDDomainSVC: p.shortcutSVC.ShortCutDomainSVC,
		CPStore:              checkpoint.NewRedisStore(p.infra.CacheCli),
	}
}

func (p *primaryServices) toSearchServiceComponents(singleAgentSVC *singleagent.SingleAgentApplicationService, appSVC *app.APPApplicationService) *search.ServiceComponents {
	infra := p.basicServices.infra

	return &search.ServiceComponents{
		DB:                   infra.DB,
		Cache:                infra.CacheCli,
		TOS:                  infra.OSS,
		ESClient:             infra.ESClient,
		ProjectEventBus:      p.basicServices.eventbus.projectEventBus,
		SingleAgentDomainSVC: singleAgentSVC.DomainSVC,
		APPDomainSVC:         appSVC.DomainSVC,
		KnowledgeDomainSVC:   p.knowledgeSVC.DomainSVC,
		PluginDomainSVC:      p.pluginSVC.DomainSVC,
		WorkflowDomainSVC:    p.workflowSVC.DomainSVC,
		UserDomainSVC:        p.basicServices.userSVC.DomainSVC,
		ConnectorDomainSVC:   p.basicServices.connectorSVC.DomainSVC,
		PromptDomainSVC:      p.basicServices.promptSVC.DomainSVC,
		DatabaseDomainSVC:    p.memorySVC.DatabaseDomainSVC,
	}
}

func (p *primaryServices) toAPPServiceComponents() *app.ServiceComponents {
	infra := p.basicServices.infra
	basic := p.basicServices
	return &app.ServiceComponents{
		IDGen:           infra.IDGenSVC,
		DB:              infra.DB,
		OSS:             infra.OSS,
		CacheCli:        infra.CacheCli,
		ProjectEventBus: basic.eventbus.projectEventBus,
		UserSVC:         basic.userSVC.DomainSVC,
		ConnectorSVC:    basic.connectorSVC.DomainSVC,
		VariablesSVC:    p.memorySVC.VariablesDomainSVC,
	}
}

func (p *primaryServices) toConversationComponents(singleAgentSVC *singleagent.SingleAgentApplicationService) *conversation.ServiceComponents {
	infra := p.basicServices.infra

	return &conversation.ServiceComponents{
		DB:                   infra.DB,
		IDGen:                infra.IDGenSVC,
		TosClient:            infra.OSS,
		ImageX:               infra.ImageXClient,
		SingleAgentDomainSVC: singleAgentSVC.DomainSVC,
	}
}
