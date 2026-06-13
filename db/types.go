package db

import "time"

type (
	ScheduleType        uint8
	JobLongevityType    uint8
	JobActionType       uint8
	JobRunStatus        uint8
	JobRunTriggerType   uint8
	JobHostResultStatus uint8

	Job struct {
		ID               int              `gosqlite:"id,primary,increment,unique" json:"id"`
		Name             string           `gosqlite:"name,unique" json:"name"`
		Description      string           `gosqlite:"description" json:"description"`
		Enabled          bool             `gosqlite:"enabled" json:"enabled"`
		Protected        bool             `gosqlite:"protected" json:"protected"`
		IntervalSeconds  int64            `gosqlite:"interval_seconds" json:"interval_seconds"`
		ScheduleType     ScheduleType     `gosqlite:"schedule_type" json:"schedule_type"`
		CronExpr         string           `gosqlite:"cron_expr" json:"cron_expr"`
		LongevityType    JobLongevityType `gosqlite:"longevity_type" json:"longevity_type"`
		MaxRuns          int              `gosqlite:"max_runs" json:"max_runs"`
		DisableAfter     time.Time        `gosqlite:"disable_after" json:"disable_after"`
		TargetHostgroups []string         `gosqlite:"target_hostgroups" json:"target_hostgroups"`
		CreatedAt        time.Time        `gosqlite:"created_at" json:"created_at"`
		UpdatedAt        time.Time        `gosqlite:"updated_at" json:"updated_at"`
	}

	JobAction struct {
		ID              int           `gosqlite:"id,primary,increment,unique" json:"id"`
		JobID           uint64        `gosqlite:"job_id,fkey:Job.id,ondelete:cascade" json:"job_id"`
		Position        int           `gosqlite:"position" json:"position"`
		Name            string        `gosqlite:"name" json:"name"`
		Description     string        `gosqlite:"description" json:"description"`
		Type            JobActionType `gosqlite:"type" json:"type"`
		FilePath        string        `gosqlite:"file_path" json:"file_path"`
		Arguments       []string      `gosqlite:"arguments" json:"arguments"`
		ContinueOnError bool          `gosqlite:"continue_on_error" json:"continue_on_error"`
		TimeoutSeconds  int64         `gosqlite:"timeout_seconds" json:"timeout_seconds"`
	}

	JobRun struct {
		ID               int               `gosqlite:"id,primary,increment,unique" json:"id"`
		JobID            uint64            `gosqlite:"job_id,fkey:Job.id" json:"job_id"`
		Status           JobRunStatus      `gosqlite:"status" json:"status"`
		TriggerType      JobRunTriggerType `gosqlite:"trigger_type" json:"trigger_type"`
		TriggeredBy      string            `gosqlite:"triggered_by" json:"triggered_by"`
		StartTime        time.Time         `gosqlite:"start_time" json:"start_time"`
		EndTime          time.Time         `gosqlite:"end_time" json:"end_time"`
		TargetHostgroups []string          `gosqlite:"target_hostgroups" json:"target_hostgroups"`
		TargetHosts      []string          `gosqlite:"target_hosts" json:"target_hosts"`
		TotalHosts       int               `gosqlite:"total_hosts" json:"total_hosts"`
		SuccessHosts     int               `gosqlite:"success_hosts" json:"success_hosts"`
		FailedHosts      int               `gosqlite:"failed_hosts" json:"failed_hosts"`
		SkippedHosts     int               `gosqlite:"skipped_hosts" json:"skipped_hosts"`
		Summary          string            `gosqlite:"summary" json:"summary"`
		Error            string            `gosqlite:"error" json:"error"`
	}

	JobActionRun struct {
		ID          int          `gosqlite:"id,primary,increment,unique" json:"id"`
		JobRunID    uint64       `gosqlite:"job_run_id,fkey:JobRun.id,ondelete:cascade" json:"job_run_id"`
		JobActionID uint64       `gosqlite:"job_action_id,fkey:JobAction.id" json:"job_action_id"`
		Status      JobRunStatus `gosqlite:"status" json:"status"`
		StartTime   time.Time    `gosqlite:"start_time" json:"start_time"`
		EndTime     time.Time    `gosqlite:"end_time" json:"end_time"`
		ExitCode    int          `gosqlite:"exit_code" json:"exit_code"`
		Stdout      string       `gosqlite:"stdout" json:"stdout"`
		Stderr      string       `gosqlite:"stderr" json:"stderr"`
		Error       string       `gosqlite:"error" json:"error"`
	}

	JobHostResult struct {
		ID          int                 `gosqlite:"id,primary,increment,unique" json:"id"`
		JobRunID    uint64              `gosqlite:"job_run_id,fkey:JobRun.id,ondelete:cascade" json:"job_run_id"`
		Hostname    string              `gosqlite:"hostname" json:"hostname"`
		Status      JobHostResultStatus `gosqlite:"status" json:"status"`
		Changed     bool                `gosqlite:"changed" json:"changed"`
		Unreachable bool                `gosqlite:"unreachable" json:"unreachable"`
		Message     string              `gosqlite:"message" json:"message"`
		ResultJSON  string              `gosqlite:"result_json" json:"result_json"`
	}

	NetworkAddressInfo struct {
		IPAddress  string `json:"ip_address"`
		MACAddress string `json:"mac_address"`
	}

	DiskInfo struct {
		Name      string `json:"name"`
		Size      int64  `json:"size"`
		Used      int64  `json:"used"`
		Available int64  `json:"available"`
	}

	Host struct {
		ID               int                  `gosqlite:"id,primary,increment,unique" json:"id"`
		Hostname         string               `gosqlite:"hostname,unique" json:"hostname"`
		FQDN             string               `gosqlite:"fqdn" json:"fqdn"`
		IPAHostDN        string               `gosqlite:"ipa_host_dn" json:"ipa_host_dn"`
		Hostgroups       []string             `gosqlite:"hostgroups" json:"hostgroups"`
		OSName           string               `gosqlite:"os_name" json:"os_name"`
		OSVersion        string               `gosqlite:"os_version" json:"os_version"`
		Arch             string               `gosqlite:"arch" json:"arch"`
		Kernel           string               `gosqlite:"kernel" json:"kernel"`
		AgentVersion     string               `gosqlite:"agent_version" json:"agent_version"`
		NetworkAddresses []NetworkAddressInfo `gosqlite:"network_addresses" json:"network_addresses"`
		LastSeenAt       time.Time            `gosqlite:"last_seen_at" json:"last_seen_at"`
		LastInventoryAt  time.Time            `gosqlite:"last_inventory_at" json:"last_inventory_at"`
		LastHealthAt     time.Time            `gosqlite:"last_health_at" json:"last_health_at"`
		LastUpdateAt     time.Time            `gosqlite:"last_update_at" json:"last_update_at"`
		CreatedAt        time.Time            `gosqlite:"created_at" json:"created_at"`
		UpdatedAt        time.Time            `gosqlite:"updated_at" json:"updated_at"`
		ProcessorModel   string               `gosqlite:"processor_model" json:"processor_model"`
		ProcessorCount   int                  `gosqlite:"processor_count" json:"processor_count"`
		ProcessorCores   int                  `gosqlite:"processor_cores" json:"processor_cores"`
		ProcessorThreads int                  `gosqlite:"processor_threads" json:"processor_threads"`
		MemoryMB         int                  `gosqlite:"memory_mb" json:"memory_mb"`
		Disks            []DiskInfo           `gosqlite:"disks" json:"disks"`
	}
)

const (
	ScheduleTypeUnknown ScheduleType = iota
	ScheduleTypeInterval
	ScheduleTypeManual
	ScheduleTypeCron
)

const (
	JobLongevityTypeUnknown JobLongevityType = iota
	JobLongevityTypePermanent
	JobLongevityTypeMaxRuns
	JobLongevityTypeUntil
)

const (
	JobActionTypeUnknown JobActionType = iota
	JobActionTypeAnsiblePlaybook
	JobActionTypeShell
)

const (
	JobRunStatusUnknown JobRunStatus = iota
	JobRunStatusQueued
	JobRunStatusRunning
	JobRunStatusSuccess
	JobRunStatusFailed
	JobRunStatusPartial
	JobRunStatusCanceled
	JobRunStatusTimeout
)

const (
	JobRunTriggerTypeUnknown JobRunTriggerType = iota
	JobRunTriggerTypeSchedule
	JobRunTriggerTypeManual
	JobRunTriggerTypeAPI
)

const (
	JobHostResultStatusUnknown JobHostResultStatus = iota
	JobHostResultStatusSuccess
	JobHostResultStatusFailed
	JobHostResultStatusUnreachable
	JobHostResultStatusSkipped
	JobHostResultStatusChanged
)
