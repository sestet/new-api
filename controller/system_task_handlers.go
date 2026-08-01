package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

// RegisterScheduledSystemTasks wires the periodic channel test, upstream model
// update, and async task polling (Midjourney / Suno / video) jobs into the
// system task framework so a DB lease dedups execution across multiple master
// instances and each run is recorded as one task row. Call this before
// service.StartSystemTaskRunner.
func RegisterScheduledSystemTasks() {
	service.RegisterSystemTaskHandler(channelTestHandler{})
	service.RegisterSystemTaskHandler(modelUpdateHandler{})
	service.RegisterSystemTaskHandler(midjourneyPollHandler{})
	service.RegisterSystemTaskHandler(asyncTaskPollHandler{})
	service.RegisterSystemTaskHandler(upstreamBillingReconcileHandler{})
	service.RegisterSystemTaskHandler(sub2APITokenRefreshHandler{})
	service.RegisterSystemTaskHandler(upstreamCostRateRefreshHandler{})
}

// channelTestHandler runs the scheduled "test all channels" job. Enablement and
// cadence still come from the monitor settings; only the execution path moved
// into the system task runner.
type channelTestHandler struct{}

func (channelTestHandler) Type() string { return model.SystemTaskTypeChannelTest }

func (channelTestHandler) Enabled() bool {
	return operation_setting.GetMonitorSetting().AutoTestChannelEnabled
}

func (channelTestHandler) Interval() time.Duration {
	minutes := operation_setting.GetMonitorSetting().AutoTestChannelMinutes
	if minutes <= 0 {
		minutes = 10
	}
	return time.Duration(minutes * float64(time.Minute))
}

func (channelTestHandler) NewPayload() any { return nil }

// channelTestTaskPayload controls one channel_test run. A nil/empty payload is a
// scheduled run, which uses the configured monitor ChannelTestMode and does not
// notify. A manual "test all channels" trigger sets Mode=scheduled_all and
// Notify=true to reproduce the legacy manual behavior (test every channel and
// notify root on completion).
type channelTestTaskPayload struct {
	Mode   string `json:"mode,omitempty"`
	Notify bool   `json:"notify,omitempty"`
}

func (channelTestHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	payload := channelTestTaskPayload{}
	if err := task.DecodePayload(&payload); err != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, nil, err)
		return
	}
	summary, err := runChannelTestTask(ctx, payload.Mode, payload.Notify, service.NewSystemTaskProgressReporter(task, runnerID))
	if err != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, nil, err)
		return
	}
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, summary, nil)
}

// modelUpdateHandler runs the scheduled upstream model update detection job.
type modelUpdateHandler struct{}

func (modelUpdateHandler) Type() string { return model.SystemTaskTypeModelUpdate }

func (modelUpdateHandler) Enabled() bool {
	return common.GetEnvOrDefaultBool("CHANNEL_UPSTREAM_MODEL_UPDATE_TASK_ENABLED", true)
}

func (modelUpdateHandler) Interval() time.Duration {
	intervalMinutes := common.GetEnvOrDefault(
		"CHANNEL_UPSTREAM_MODEL_UPDATE_TASK_INTERVAL_MINUTES",
		channelUpstreamModelUpdateTaskDefaultIntervalMinutes,
	)
	if intervalMinutes < 1 {
		intervalMinutes = channelUpstreamModelUpdateTaskDefaultIntervalMinutes
	}
	return time.Duration(intervalMinutes) * time.Minute
}

func (modelUpdateHandler) NewPayload() any { return nil }

// modelUpdateTaskPayload controls one model_update run. A scheduled run
// (Manual=false) respects the per-channel minimum check interval and may
// auto-apply detected models when a channel has auto-sync enabled. A manual
// "detect all" trigger sets Manual=true to reproduce the legacy detect-all
// semantics: force a re-check regardless of the interval and never auto-apply,
// so the admin reviews and applies changes explicitly.
type modelUpdateTaskPayload struct {
	Manual bool `json:"manual,omitempty"`
}

func (modelUpdateHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	payload := modelUpdateTaskPayload{}
	if err := task.DecodePayload(&payload); err != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, nil, err)
		return
	}
	summary := runChannelUpstreamModelUpdateTaskOnce(ctx, payload.Manual, !payload.Manual, service.NewSystemTaskProgressReporter(task, runnerID))
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, summary, nil)
}

// midjourneyPollHandler runs one Midjourney polling pass per scheduled run.
// Enabled() folds the "are there unfinished tasks?" check into enablement so the
// scheduler creates no row when the system is idle; only when at least one
// Midjourney task is in progress does a row get scheduled.
type midjourneyPollHandler struct{}

func (midjourneyPollHandler) Type() string { return model.SystemTaskTypeMidjourneyPoll }

func (midjourneyPollHandler) Enabled() bool {
	return constant.UpdateTask && model.HasUnfinishedMidjourneyTasks()
}

func (midjourneyPollHandler) Interval() time.Duration { return 15 * time.Second }

func (midjourneyPollHandler) NewPayload() any { return nil }

func (midjourneyPollHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	summary := runMidjourneyTaskUpdateOnce(ctx, service.NewSystemTaskProgressReporter(task, runnerID))
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, summary, nil)
}

// asyncTaskPollHandler runs one async-task (Suno/video) polling pass per
// scheduled run. Like midjourneyPollHandler, Enabled() folds in the unfinished
// task existence check so an idle system schedules no rows.
type asyncTaskPollHandler struct{}

func (asyncTaskPollHandler) Type() string { return model.SystemTaskTypeAsyncTaskPoll }

func (asyncTaskPollHandler) Enabled() bool {
	return constant.UpdateTask && model.HasUnfinishedSyncTasks()
}

func (asyncTaskPollHandler) Interval() time.Duration { return 15 * time.Second }

func (asyncTaskPollHandler) NewPayload() any { return nil }

func (asyncTaskPollHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	summary := service.RunTaskPollingOnce(ctx, service.NewSystemTaskProgressReporter(task, runnerID))
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, summary, nil)
}

type upstreamBillingReconcileHandler struct{}

type upstreamBillingReconcileTaskPayload struct {
	CredentialID int `json:"credential_id,omitempty"`
}

func (upstreamBillingReconcileHandler) Type() string {
	return model.SystemTaskTypeUpstreamBillingReconcile
}

func (upstreamBillingReconcileHandler) Enabled() bool {
	if !common.GetEnvOrDefaultBool("UPSTREAM_BILLING_RECONCILE_TASK_ENABLED", true) {
		return false
	}
	createdAfter := common.GetTimestamp() - int64(service.UpstreamBillingReconcileLookback().Seconds())
	return model.HasUpstreamBillingReconcileWork(createdAfter)
}

func (upstreamBillingReconcileHandler) Interval() time.Duration {
	minutes := common.GetEnvOrDefault("UPSTREAM_BILLING_RECONCILE_TASK_INTERVAL_MINUTES", 1)
	if minutes < 1 {
		minutes = 1
	}
	return time.Duration(minutes) * time.Minute
}

func (upstreamBillingReconcileHandler) NewPayload() any { return nil }

func (upstreamBillingReconcileHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	payload := upstreamBillingReconcileTaskPayload{}
	if err := task.DecodePayload(&payload); err != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, nil, err)
		return
	}
	var summary service.UpstreamBillingReconcileSummary
	if payload.CredentialID > 0 {
		summary = service.RunUpstreamBillingReconcileForCredential(ctx, payload.CredentialID, 200, 0)
	} else {
		summary = service.RunUpstreamBillingReconcile(ctx, 0, 0)
	}
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, summary, nil)
}

type sub2APITokenRefreshHandler struct{}

func (sub2APITokenRefreshHandler) Type() string {
	return model.SystemTaskTypeSub2APITokenRefresh
}

func (sub2APITokenRefreshHandler) Enabled() bool {
	if !common.GetEnvOrDefaultBool("SUB2API_TOKEN_REFRESH_TASK_ENABLED", true) {
		return false
	}
	return service.HasSub2APICredentialRefreshWork()
}

func (sub2APITokenRefreshHandler) Interval() time.Duration {
	minutes := common.GetEnvOrDefault("SUB2API_TOKEN_REFRESH_TASK_INTERVAL_MINUTES", 15)
	if minutes < 1 {
		minutes = 15
	}
	return time.Duration(minutes) * time.Minute
}

func (sub2APITokenRefreshHandler) NewPayload() any { return nil }

func (sub2APITokenRefreshHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	summary := service.RefreshDueSub2APICredentials(ctx)
	status := model.SystemTaskStatusSucceeded
	var runErr error
	if summary.Failed > 0 && summary.Refreshed == 0 {
		status = model.SystemTaskStatusFailed
		runErr = fmt.Errorf("failed to refresh %d sub2api billing credentials", summary.Failed)
	}
	finishSystemTaskHandler(task, runnerID, status, summary, runErr)
}

type upstreamCostRateRefreshHandler struct{}

func (upstreamCostRateRefreshHandler) Type() string {
	return model.SystemTaskTypeUpstreamCostRateRefresh
}

func (upstreamCostRateRefreshHandler) Enabled() bool {
	return common.GetEnvOrDefaultBool("UPSTREAM_COST_RATE_REFRESH_TASK_ENABLED", true) && service.HasUpstreamCostRateRefreshWork()
}

func (upstreamCostRateRefreshHandler) Interval() time.Duration {
	minutes := common.GetEnvOrDefault("UPSTREAM_COST_RATE_REFRESH_TASK_INTERVAL_MINUTES", 360)
	if minutes < 5 {
		minutes = 360
	}
	return time.Duration(minutes) * time.Minute
}

func (upstreamCostRateRefreshHandler) NewPayload() any { return nil }

func (upstreamCostRateRefreshHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	summary := service.RefreshUpstreamCostRates(ctx, service.NewSystemTaskProgressReporter(task, runnerID))
	status := model.SystemTaskStatusSucceeded
	var runErr error
	if summary.Failed > 0 && summary.Updated == 0 {
		status = model.SystemTaskStatusFailed
		runErr = fmt.Errorf("failed to refresh %d upstream cost rates", summary.Failed)
	}
	finishSystemTaskHandler(task, runnerID, status, summary, runErr)
}

func finishSystemTaskHandler(task *model.SystemTask, runnerID string, status model.SystemTaskStatus, result any, runErr error) {
	errorMessage := ""
	if runErr != nil {
		errorMessage = runErr.Error()
	}
	if err := model.FinishSystemTask(task.TaskID, runnerID, status, result, errorMessage); err != nil {
		common.SysLog(fmt.Sprintf("system task %s failed to persist result: %v", task.TaskID, err))
	}
}
