export type ScheduleType = 0 | 1 | 2 | 3;
export type JobLongevityType = 0 | 1 | 2 | 3;
export type JobActionType = 0 | 1 | 2;
export type JobRunStatus = 0 | 1 | 2 | 3 | 4 | 5 | 6 | 7;
export type JobRunTriggerType = 0 | 1 | 2 | 3;
export type JobHostResultStatus = 0 | 1 | 2 | 3 | 4 | 5;

export interface DashboardSummary {
    hosts: number;
    jobs: number;
    queued_jobs: number;
    running_jobs: number;
    failed_jobs: number;
    recent_runs: JobRun[];
}

export type AuthRole = "viewer" | "editor";

export interface AuthenticatedUser {
    username: string;
    display_name: string;
    dn: string;
    groups: string[];
    roles: AuthRole[];
    can_view: boolean;
    can_edit: boolean;
    expires_at: string;
}

export interface LoginRequest {
    username: string;
    password: string;
}

export interface Job {
    id: number;
    name: string;
    description: string;
    enabled: boolean;
    protected: boolean;
    interval_seconds: number;
    schedule_type: ScheduleType;
    cron_expr: string;
    longevity_type: JobLongevityType;
    max_runs: number;
    disable_after: string;
    target_hostgroups: string[];
    created_at: string;
    updated_at: string;
}

export interface JobActionInput {
    name: string;
    description: string;
    type: JobActionType;
    file_path: string;
    arguments: string[];
    continue_on_error: boolean;
    timeout_seconds: number;
}

export interface JobInput {
    name: string;
    description: string;
    enabled: boolean;
    interval_seconds: number;
    schedule_type: ScheduleType;
    cron_expr: string;
    longevity_type: JobLongevityType;
    max_runs: number;
    disable_after: string;
    target_hostgroups: string[];
    actions: JobActionInput[];
}

export interface JobAction {
    id: number;
    job_id: number;
    position: number;
    name: string;
    description: string;
    type: JobActionType;
    file_path: string;
    arguments: string[];
    continue_on_error: boolean;
    timeout_seconds: number;
}

export interface JobRun {
    id: number;
    job_id: number;
    status: JobRunStatus;
    trigger_type: JobRunTriggerType;
    triggered_by: string;
    start_time: string;
    end_time: string;
    target_hostgroups: string[];
    target_hosts: string[];
    total_hosts: number;
    success_hosts: number;
    failed_hosts: number;
    skipped_hosts: number;
    summary: string;
    error: string;
}

export interface JobActionRun {
    id: number;
    job_run_id: number;
    job_action_id: number;
    status: JobRunStatus;
    start_time: string;
    end_time: string;
    exit_code: number;
    stdout: string;
    stderr: string;
    error: string;
}

export interface JobHostResult {
    id: number;
    job_run_id: number;
    hostname: string;
    status: JobHostResultStatus;
    changed: boolean;
    unreachable: boolean;
    message: string;
    result_json: string;
}

export interface NetworkAddressInfo {
    ip_address: string;
    mac_address: string;
}

export interface DiskInfo {
    name: string;
    size: number;
    used: number;
    available: number;
}

export interface Host {
    id: number;
    hostname: string;
    fqdn: string;
    ipa_host_dn: string;
    hostgroups: string[];
    os_name: string;
    os_version: string;
    arch: string;
    kernel: string;
    agent_version: string;
    network_addresses: NetworkAddressInfo[];
    last_seen_at: string;
    last_inventory_at: string;
    last_health_at: string;
    last_update_at: string;
    created_at: string;
    updated_at: string;
    processor_model: string;
    processor_count: number;
    processor_cores: number;
    processor_threads: number;
    memory_mb: number;
    disks: DiskInfo[];
}

export interface ScheduledJob {
    id: number;
    name: string;
    schedule_type: ScheduleType;
    interval_seconds: number;
    cron_expr: string;
    longevity_type: JobLongevityType;
    max_runs: number;
    disable_after: string;
    enabled: boolean;
    next_run_at: string;
}

export interface SchedulerSnapshot {
    loaded_jobs: ScheduledJob[];
}

export type DataFileKind = "other" | "playbook" | "shell";

export interface DataFileInfo {
    path: string;
    name: string;
    kind: DataFileKind;
    size: number;
    modified_at: string;
    protected: boolean;
}

export interface DataFileContent extends DataFileInfo {
    content: string;
}

export interface DataFileInput {
    path: string;
    content: string;
}

export interface JobsResponse {
    jobs: Job[];
    actions: Record<string, JobAction[]>;
    runs: JobRun[];
    action_runs: JobActionRun[];
    host_results: JobHostResult[];
    scheduler: SchedulerSnapshot;
}
