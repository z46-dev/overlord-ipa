import { FormEvent, useEffect, useLayoutEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { createJob, getDataFiles, getHostGroups, getJobs, runJob, updateJob } from "../api/client";
import type { DataFileInfo, Job, JobAction, JobActionInput, JobActionType, JobInput, JobLongevityType, JobRun, SchedulerSnapshot, ScheduleType } from "../api/types";
import { DataTable, type DataTableColumn } from "../components/DataTable";
import { StatusBadge } from "../components/StatusBadge";
import { cronExpressionValid, formatCronDescription, formatDateTime, formatSchedule, jobEnabledTone, jobRunStatusLabel, jobRunStatusTone, scheduleTone, scheduleTypeLabel } from "./format";

interface JobsProps {
    canEdit: boolean;
    openJobID?: number | null;
    onOpenJobHandled?: () => void;
}

const actionTypes: Array<{ value: JobActionType; label: string }> = [
    { value: 1, label: "Ansible playbook" },
    { value: 2, label: "Shell" }
];

const scheduleOptions: Array<{ value: ScheduleType; label: string }> = [
    { value: 1, label: "Interval" },
    { value: 3, label: "Cron" },
    { value: 2, label: "Manual" }
];

const longevityOptions: Array<{ value: JobLongevityType; label: string }> = [
    { value: 1, label: "Permanent" },
    { value: 2, label: "Run count" },
    { value: 3, label: "Until date" }
];

function newActionInput(): JobActionInput {
    return {
        name: "Ansible playbook",
        description: "",
        type: 1,
        file_path: "",
        arguments: [],
        continue_on_error: false,
        timeout_seconds: 600
    };
}

function newJobInput(): JobInput {
    return {
        name: "",
        description: "",
        enabled: false,
        interval_seconds: 300,
        schedule_type: 2,
        cron_expr: "",
        longevity_type: 1,
        max_runs: 0,
        disable_after: zeroTime(),
        target_hostgroups: [],
        actions: [newActionInput()]
    };
}

function actionToInput(action: JobAction | undefined): JobActionInput {
    if (!action) {
        return newActionInput();
    }

    return {
        name: action.name,
        description: action.description,
        type: action.type,
        file_path: action.file_path,
        arguments: action.arguments ?? [],
        continue_on_error: action.continue_on_error,
        timeout_seconds: action.timeout_seconds
    };
}

export function Jobs({ canEdit, openJobID, onOpenJobHandled }: JobsProps) {
    const [jobs, setJobs] = useState<Job[]>([]);
    const [jobActions, setJobActions] = useState<Record<string, JobAction[]>>({});
    const [jobRuns, setJobRuns] = useState<JobRun[]>([]);
    const [scheduler, setScheduler] = useState<SchedulerSnapshot>({ loaded_jobs: [] });
    const [hostGroups, setHostGroups] = useState<string[]>([]);
    const [dataFiles, setDataFiles] = useState<DataFileInfo[]>([]);
    const [error, setError] = useState<string>("");
    const [notice, setNotice] = useState<string>("");
    const [formError, setFormError] = useState<string>("");
    const [input, setInput] = useState<JobInput>(newJobInput());
    const [editingJob, setEditingJob] = useState<Job | null>(null);
    const [editorOpen, setEditorOpen] = useState(false);
    const [saving, setSaving] = useState(false);
    const [runningJobID, setRunningJobID] = useState<number | null>(null);
    const [hostGroupQuery, setHostGroupQuery] = useState("");
    const [hostGroupOpen, setHostGroupOpen] = useState(false);

    const loadJobs = () => {
        getJobs()
            .then((response) => {
                setJobs(response.jobs);
                setJobActions(response.actions ?? {});
                setJobRuns(response.runs ?? []);
                setScheduler(response.scheduler);
            })
            .catch((err: unknown) => {
                setError(err instanceof Error ? err.message : "Unable to load jobs");
            });
    };

    const loadDataFiles = () => {
        getDataFiles()
            .then(setDataFiles)
            .catch((err: unknown) => {
                setFormError(err instanceof Error ? err.message : "Unable to load data files");
            });
    };

    useEffect(() => {
        loadJobs();
        if (canEdit) {
            loadDataFiles();
        }

        getHostGroups()
            .then(setHostGroups)
            .catch((err: unknown) => {
                setFormError(err instanceof Error ? err.message : "Unable to load host groups");
            });
    }, [canEdit]);

    useEffect(() => {
        if (!openJobID) {
            return;
        }

        if (!canEdit) {
            onOpenJobHandled?.();
            return;
        }

        if (jobs.length === 0) {
            return;
        }

        const job = jobs.find((item) => item.id === openJobID);
        if (!job) {
            return;
        }

        openEditJob(job);
        onOpenJobHandled?.();
    }, [openJobID, canEdit, jobs, jobActions]);

    const loadedJobIDs = new Set(scheduler.loaded_jobs.map((job) => job.id));
    const availableHostGroups = hostGroups.filter((group) => {
        if (input.target_hostgroups.includes(group)) {
            return false;
        }

        return group.toLowerCase().includes(hostGroupQuery.toLowerCase());
    });
    const selectedAction = input.actions[0] ?? newActionInput();
    const latestRunByJobID = new Map<number, JobRun>();
    for (const run of jobRuns) {
        if (!latestRunByJobID.has(Number(run.job_id))) {
            latestRunByJobID.set(Number(run.job_id), run);
        }
    }

    const availableActionFiles = dataFiles.filter((file) => {
        if (selectedAction.type === 1) {
            return file.kind === "playbook";
        }

        if (selectedAction.type === 2) {
            return file.kind === "shell";
        }

        return false;
    });

    const openNewJob = () => {
        setEditingJob(null);
        setFormError("");
        setHostGroupQuery("");
        setHostGroupOpen(false);
        setInput(newJobInput());
        loadDataFiles();
        setEditorOpen(true);
    };

    const openEditJob = (job: Job) => {
        setEditingJob(job);
        setFormError("");
        setHostGroupQuery("");
        setHostGroupOpen(false);
        setInput({
            name: job.name,
            description: job.description,
            enabled: job.enabled,
            interval_seconds: job.interval_seconds || 300,
            schedule_type: job.schedule_type,
            cron_expr: job.cron_expr,
            longevity_type: job.longevity_type || 1,
            max_runs: job.max_runs || 0,
            disable_after: job.disable_after || zeroTime(),
            target_hostgroups: job.target_hostgroups ?? [],
            actions: [actionToInput(jobActions[String(job.id)]?.[0])]
        });
        loadDataFiles();
        setEditorOpen(true);
    };

    const closeEditor = () => {
        setEditorOpen(false);
        setEditingJob(null);
        setFormError("");
        setHostGroupQuery("");
        setHostGroupOpen(false);
        setInput(newJobInput());
    };

    const submit = (event: FormEvent<HTMLFormElement>) => {
        event.preventDefault();
        setFormError("");
        setSaving(true);

        const request = editingJob ? updateJob(editingJob.id, input) : createJob(input);
        request
            .then(() => {
                closeEditor();
                loadJobs();
            })
            .catch((err: unknown) => {
                setFormError(err instanceof Error ? err.message : "Unable to save job");
            })
            .finally(() => {
                setSaving(false);
            });
    };

    const runSelectedJob = (job: Job) => {
        setError("");
        setNotice("");
        setRunningJobID(job.id);

        runJob(job.id)
            .then((run) => {
                setNotice(`Queued ${job.name} as run #${run.id}`);
                loadJobs();
            })
            .catch((err: unknown) => {
                setError(err instanceof Error ? err.message : "Unable to run job");
            })
            .finally(() => {
                setRunningJobID(null);
            });
    };

    const setAction = (patch: Partial<JobActionInput>) => {
        setInput({
            ...input,
            actions: [{ ...selectedAction, ...patch }]
        });
    };

    const changeActionType = (type: JobActionType) => {
        setAction({
            type,
            name: type === 1 ? "Ansible playbook" : "Shell script",
            file_path: ""
        });
    };

    const setScheduleType = (scheduleType: ScheduleType) => {
        setInput({
            ...input,
            schedule_type: scheduleType,
            interval_seconds: scheduleType === 1 ? input.interval_seconds || 300 : input.interval_seconds,
            cron_expr: scheduleType === 3 ? input.cron_expr : ""
        });
    };

    const setLongevityType = (longevityType: JobLongevityType) => {
        setInput({
            ...input,
            longevity_type: longevityType,
            max_runs: longevityType === 2 ? input.max_runs || 1 : 0,
            disable_after: longevityType === 3 ? input.disable_after : zeroTime()
        });
    };

    const addHostGroup = (group: string) => {
        if (!group || input.target_hostgroups.includes(group)) {
            return;
        }

        setInput({
            ...input,
            target_hostgroups: [...input.target_hostgroups, group]
        });
        setHostGroupQuery("");
        setHostGroupOpen(true);
    };

    const removeHostGroup = (group: string) => {
        setInput({
            ...input,
            target_hostgroups: input.target_hostgroups.filter((selected) => selected !== group)
        });
    };

    const removeLastHostGroup = () => {
        if (hostGroupQuery || input.target_hostgroups.length === 0) {
            return;
        }

        setInput({
            ...input,
            target_hostgroups: input.target_hostgroups.slice(0, -1)
        });
    };

    const columns: DataTableColumn<Job>[] = [
        { key: "name", header: "Name", render: (job) => <span className="font-medium">{job.name}</span> },
        { key: "description", header: "Description", render: (job) => job.description || "No description" },
        { key: "schedule", header: "Schedule", render: (job) => <ScheduleCell job={job} /> },
        { key: "longevity", header: "Longevity", render: (job) => formatLongevity(job) },
        { key: "targets", header: "Targets", render: (job) => job.target_hostgroups?.join(", ") || "None" },
        { key: "action_file", header: "Action file", render: (job) => jobActions[String(job.id)]?.[0]?.file_path || "None" },
        { key: "last_run", header: "Last run", render: (job) => <LastRunCell run={latestRunByJobID.get(job.id)} /> },
        { key: "protected", header: "Type", render: (job) => <StatusBadge label={job.protected ? "Protected" : "Custom"} tone="neutral" /> },
        {
            key: "enabled",
            header: "Enabled",
            render: (job) => <StatusBadge label={job.enabled ? "Enabled" : "Disabled"} tone={jobEnabledTone(job.enabled)} />
        },
        {
            key: "scheduler",
            header: "Scheduler",
            render: (job) => (
                <StatusBadge label={loadedJobIDs.has(job.id) ? "Loaded" : "Not loaded"} tone={loadedJobIDs.has(job.id) ? "info" : "neutral"} />
            )
        },
        {
            key: "run",
            header: "",
            className: "w-24 text-right",
            render: (job) =>
                canEdit ? (
                    <button
                        className="rounded-sm border border-[#1f6fb2] px-2 py-1 text-xs font-medium text-[#1f6fb2] hover:bg-[#e8f1fa] disabled:cursor-not-allowed disabled:border-[#9ca3af] disabled:text-[#6b7280]"
                        disabled={runningJobID === job.id}
                        type="button"
                        onClick={(event) => {
                            event.stopPropagation();
                            runSelectedJob(job);
                        }}
                    >
                        {runningJobID === job.id ? "Queuing" : "Run"}
                    </button>
                ) : null
        }
    ];

    return (
        <div className="space-y-3">
            <div className="flex items-center justify-between">
                <h2 className="text-base font-semibold text-[#1f2933]">Jobs</h2>
                <button
                    className="rounded-sm bg-[#1f6fb2] px-3 py-1.5 text-sm font-medium text-white hover:bg-[#155a96] disabled:cursor-not-allowed disabled:bg-[#9ca3af]"
                    disabled={!canEdit}
                    type="button"
                    onClick={openNewJob}
                >
                    New Job
                </button>
            </div>

            {error ? <div className="rounded border border-red-300 bg-red-50 px-3 py-2 text-sm text-red-800">{error}</div> : null}
            {notice ? <div className="rounded border border-[#9dbfe0] bg-[#e8f1fa] px-3 py-2 text-sm text-[#155a96]">{notice}</div> : null}
            <DataTable columns={columns} rows={jobs} getRowKey={(job) => job.id} emptyLabel="No jobs configured" onRowClick={canEdit ? openEditJob : undefined} />

            {canEdit && editorOpen ? (
                <div
                    className="fixed inset-0 z-50 flex items-start justify-center bg-black/35 px-4 pb-8 pt-16"
                    role="presentation"
                    onMouseDown={(event) => {
                        if (event.target === event.currentTarget) {
                            closeEditor();
                        }
                    }}
                >
                    <form className="w-full max-w-6xl rounded-sm border border-[#9ca3af] bg-white shadow-xl" role="dialog" aria-modal="true" onSubmit={submit}>
                        <div className="flex items-center justify-between border-b border-[#d1d5db] bg-[#eef0f2] px-3 py-2">
                            <h3 className="text-sm font-semibold text-[#1f2933]">{editingJob ? `Edit ${editingJob.name}` : "New Job"}</h3>
                            <button className="text-sm font-medium text-[#1f6fb2]" type="button" onClick={closeEditor}>
                                Close
                            </button>
                        </div>

                        <div className="max-h-[72vh] overflow-y-auto overflow-x-visible p-3">
                            {formError ? <div className="mb-3 rounded border border-red-300 bg-red-50 px-3 py-2 text-sm text-red-800">{formError}</div> : null}

                            <div className="grid gap-3 md:grid-cols-6">
                                <label className="text-sm font-medium md:col-span-2">
                                    Name
                                    <input
                                        className="mt-1 w-full rounded-sm border border-[#d1d5db] px-2 py-1.5"
                                        disabled={editingJob?.protected}
                                        value={input.name}
                                        onChange={(event) => setInput({ ...input, name: event.target.value })}
                                    />
                                </label>
                                <label className="text-sm font-medium md:col-span-3">
                                    Description
                                    <input
                                        className="mt-1 w-full rounded-sm border border-[#d1d5db] px-2 py-1.5"
                                        value={input.description}
                                        onChange={(event) => setInput({ ...input, description: event.target.value })}
                                    />
                                </label>
                                <label className="flex items-center gap-2 pt-6 text-sm font-medium">
                                    <input checked={input.enabled} type="checkbox" onChange={(event) => setInput({ ...input, enabled: event.target.checked })} />
                                    Enabled
                                </label>
                                <ScheduleEditor input={input} onChange={setInput} onTypeChange={setScheduleType} />

                                <LongevityEditor input={input} onChange={setInput} onTypeChange={setLongevityType} />

                                <HostGroupSelect
                                    availableGroups={availableHostGroups}
                                    open={hostGroupOpen}
                                    query={hostGroupQuery}
                                    selectedGroups={input.target_hostgroups}
                                    onAdd={addHostGroup}
                                    onOpenChange={setHostGroupOpen}
                                    onQueryChange={setHostGroupQuery}
                                    onRemove={removeHostGroup}
                                    onRemoveLast={removeLastHostGroup}
                                />

                                {!editingJob?.protected ? (
                                    <>
                                        <label className="text-sm font-medium md:col-span-2">
                                            Action
                                            <select
                                                className="mt-1 w-full rounded-sm border border-[#d1d5db] px-2 py-1.5"
                                                value={selectedAction.type}
                                                onChange={(event) => changeActionType(Number(event.target.value) as JobActionType)}
                                            >
                                                {actionTypes.map((type) => (
                                                    <option key={type.value} value={type.value}>
                                                        {type.label}
                                                    </option>
                                                ))}
                                            </select>
                                        </label>
                                        <label className="text-sm font-medium md:col-span-2">
                                            Action file
                                            <select
                                                className="mt-1 w-full rounded-sm border border-[#d1d5db] px-2 py-1.5"
                                                value={selectedAction.file_path}
                                                onChange={(event) => setAction({ file_path: event.target.value })}
                                            >
                                                <option value="">Select file</option>
                                                {availableActionFiles.map((file) => (
                                                    <option key={file.path} value={file.path}>
                                                        {file.path}
                                                    </option>
                                                ))}
                                            </select>
                                        </label>
                                        <label className="text-sm font-medium md:col-span-1">
                                            Action name
                                            <input
                                                className="mt-1 w-full rounded-sm border border-[#d1d5db] px-2 py-1.5"
                                                value={selectedAction.name}
                                                onChange={(event) => setAction({ name: event.target.value })}
                                            />
                                        </label>
                                        <label className="text-sm font-medium md:col-span-1">
                                            Timeout seconds
                                            <input
                                                className="mt-1 w-full rounded-sm border border-[#d1d5db] px-2 py-1.5"
                                                min={1}
                                                type="number"
                                                value={selectedAction.timeout_seconds}
                                                onChange={(event) => setAction({ timeout_seconds: Number(event.target.value) })}
                                            />
                                        </label>
                                        <label className="text-sm font-medium md:col-span-6">
                                            Arguments
                                            <textarea
                                                className="mt-1 h-20 w-full rounded-sm border border-[#d1d5db] px-2 py-1.5 font-mono text-xs"
                                                value={selectedAction.arguments.join("\n")}
                                                onChange={(event) => setAction({ arguments: event.target.value.split("\n").map((value) => value.trim()).filter(Boolean) })}
                                            />
                                        </label>
                                    </>
                                ) : null}
                            </div>
                        </div>

                        <div className="flex justify-end gap-2 border-t border-[#d1d5db] px-3 py-2">
                            <button className="rounded-sm border border-[#d1d5db] px-3 py-1.5 text-sm" type="button" onClick={closeEditor}>
                                Cancel
                            </button>
                            <button className="rounded-sm bg-[#1f6fb2] px-3 py-1.5 text-sm font-medium text-white disabled:bg-[#9ca3af]" disabled={saving} type="submit">
                                {saving ? "Saving" : "Save"}
                            </button>
                        </div>
                    </form>
                </div>
            ) : null}
        </div>
    );
}

function ScheduleCell({ job }: { job: Job }) {
    return (
        <div className="flex items-center gap-2">
            <StatusBadge label={scheduleTypeLabel(job)} tone={scheduleTone(job)} />
            <span className="text-sm text-[#1f2933]">{formatSchedule(job)}</span>
        </div>
    );
}

function LastRunCell({ run }: { run?: JobRun }) {
    if (!run) {
        return <span className="text-sm text-[#6b7280]">Never</span>;
    }

    return (
        <div className="space-y-1">
            <div className="flex items-center gap-2">
                <StatusBadge label={jobRunStatusLabel(run)} tone={jobRunStatusTone(run)} />
                <span className="text-xs text-[#6b7280]">{formatDateTime(run.start_time)}</span>
            </div>
            {run.error ? <div className="max-w-80 truncate text-xs text-red-800">{run.error}</div> : null}
        </div>
    );
}

function ScheduleEditor({ input, onChange, onTypeChange }: { input: JobInput; onChange: (input: JobInput) => void; onTypeChange: (scheduleType: ScheduleType) => void }) {
    return (
        <div className="md:col-span-6">
            <div className="mb-1 text-sm font-medium">Schedule</div>
            <div className="overflow-hidden rounded-sm border border-[#d1d5db] bg-white">
                <div className="grid md:grid-cols-3">
                    {scheduleOptions.map((option) => (
                        <button className={segmentButtonClass(input.schedule_type === option.value)} key={option.value} type="button" onClick={() => onTypeChange(option.value)}>
                            {option.label}
                        </button>
                    ))}
                </div>
                <div className="border-t border-[#d1d5db] bg-[#f8fafc] px-3 py-2">
                    {input.schedule_type === 1 ? (
                        <label className="flex items-center gap-3 text-sm font-medium">
                            Interval seconds
                            <input
                                className="w-32 rounded-sm border border-[#d1d5db] bg-white px-2 py-1.5 text-sm text-[#1f2933]"
                                min={1}
                                type="number"
                                value={input.interval_seconds}
                                onChange={(event) => onChange({ ...input, interval_seconds: Number(event.target.value) })}
                            />
                        </label>
                    ) : null}

                    {input.schedule_type === 3 ? (
                        <div className="grid gap-2 md:grid-cols-[minmax(220px,340px)_1fr]">
                            <input
                                className="rounded-sm border border-[#d1d5db] bg-white px-2 py-1.5 font-mono text-xs text-[#1f2933]"
                                value={input.cron_expr}
                                onChange={(event) => onChange({ ...input, cron_expr: event.target.value })}
                            />
                            <div className={`self-center text-xs ${cronExpressionValid(input.cron_expr) ? "text-[#6b7280]" : "text-red-700"}`}>
                                {formatCronDescription(input.cron_expr)}
                            </div>
                        </div>
                    ) : null}

                    {input.schedule_type === 2 ? <div className="h-8" /> : null}
                </div>
            </div>
        </div>
    );
}

function LongevityEditor({ input, onChange, onTypeChange }: { input: JobInput; onChange: (input: JobInput) => void; onTypeChange: (longevityType: JobLongevityType) => void }) {
    return (
        <div className="md:col-span-6">
            <div className="mb-1 text-sm font-medium">Longevity</div>
            <div className="overflow-hidden rounded-sm border border-[#d1d5db] bg-white">
                <div className="grid md:grid-cols-3">
                    {longevityOptions.map((option) => (
                        <button className={segmentButtonClass(input.longevity_type === option.value)} key={option.value} type="button" onClick={() => onTypeChange(option.value)}>
                            {option.label}
                        </button>
                    ))}
                </div>
                <div className="border-t border-[#d1d5db] bg-[#f8fafc] px-3 py-2">
                    {input.longevity_type === 1 ? <div className="h-8" /> : null}

                    {input.longevity_type === 2 ? (
                        <label className="flex items-center gap-3 text-sm font-medium">
                            Max runs
                            <input
                                className="w-32 rounded-sm border border-[#d1d5db] bg-white px-2 py-1.5 text-sm text-[#1f2933]"
                                min={1}
                                type="number"
                                value={input.max_runs}
                                onChange={(event) => onChange({ ...input, max_runs: Number(event.target.value) })}
                            />
                        </label>
                    ) : null}

                    {input.longevity_type === 3 ? (
                        <label className="flex items-center gap-3 text-sm font-medium">
                            Disable after
                            <input
                                className="rounded-sm border border-[#d1d5db] bg-white px-2 py-1.5 text-sm text-[#1f2933]"
                                type="datetime-local"
                                value={toDateTimeLocal(input.disable_after)}
                                onChange={(event) => onChange({ ...input, disable_after: fromDateTimeLocal(event.target.value) })}
                            />
                        </label>
                    ) : null}
                </div>
            </div>
        </div>
    );
}

function segmentButtonClass(active: boolean): string {
    if (active) {
        return "h-10 border-r border-[#d1d5db] border-b-2 border-b-[#1f6fb2] bg-[#e8f1fa] px-3 text-left text-sm font-semibold text-[#1f2933] last:border-r-0";
    }

    return "h-10 border-r border-[#d1d5db] border-b-2 border-b-transparent bg-white px-3 text-left text-sm font-medium text-[#1f2933] hover:bg-[#eef0f2] last:border-r-0";
}

function formatLongevity(job: Job): string {
    switch (job.longevity_type || 1) {
        case 1:
            return "Permanent";
        case 2:
            return job.max_runs > 0 ? `${job.max_runs} runs` : "Run count";
        case 3:
            return job.disable_after && !job.disable_after.startsWith("0001-01-01") ? `Until ${new Date(job.disable_after).toLocaleString()}` : "Until date";
        default:
            return "Unknown";
    }
}

function zeroTime(): string {
    return "0001-01-01T00:00:00Z";
}

function toDateTimeLocal(value: string): string {
    if (!value || value.startsWith("0001-01-01")) {
        return "";
    }

    const date = new Date(value);
    if (Number.isNaN(date.getTime())) {
        return "";
    }

    const offset = date.getTimezoneOffset() * 60000;
    return new Date(date.getTime() - offset).toISOString().slice(0, 16);
}

function fromDateTimeLocal(value: string): string {
    if (!value) {
        return zeroTime();
    }

    return new Date(value).toISOString();
}

interface HostGroupSelectProps {
    availableGroups: string[];
    open: boolean;
    query: string;
    selectedGroups: string[];
    onAdd: (group: string) => void;
    onOpenChange: (open: boolean) => void;
    onQueryChange: (query: string) => void;
    onRemove: (group: string) => void;
    onRemoveLast: () => void;
}

function HostGroupSelect({
    availableGroups,
    open,
    query,
    selectedGroups,
    onAdd,
    onOpenChange,
    onQueryChange,
    onRemove,
    onRemoveLast
}: HostGroupSelectProps) {
    const rootRef = useRef<HTMLDivElement | null>(null);
    const [menuBox, setMenuBox] = useState({ left: 0, top: 0, width: 0 });

    useLayoutEffect(() => {
        const updateMenuBox = () => {
            const rect = rootRef.current?.getBoundingClientRect();
            if (!rect) {
                return;
            }

            setMenuBox({
                left: rect.left,
                top: rect.bottom + 4,
                width: rect.width
            });
        };

        if (!open) {
            return;
        }

        updateMenuBox();
        window.addEventListener("resize", updateMenuBox);
        window.addEventListener("scroll", updateMenuBox, true);

        return () => {
            window.removeEventListener("resize", updateMenuBox);
            window.removeEventListener("scroll", updateMenuBox, true);
        };
    }, [open, selectedGroups.length, query]);

    return (
        <div
            ref={rootRef}
            className="relative md:col-span-6"
            onBlur={(event) => {
                if (!event.currentTarget.contains(event.relatedTarget as Node | null)) {
                    onOpenChange(false);
                }
            }}
        >
            <label className="block text-sm font-medium">
                Host groups
                <div className="mt-1 flex min-h-9 flex-wrap items-center gap-1 rounded-sm border border-[#d1d5db] bg-white px-2 py-1">
                    {selectedGroups.map((group) => (
                        <span className="inline-flex items-center gap-1 rounded-sm border border-[#b8c4cf] bg-[#eef0f2] px-2 py-0.5 text-xs text-[#1f2933]" key={group}>
                            {group}
                            <button className="text-[#6b7280] hover:text-[#1f2933]" type="button" onClick={() => onRemove(group)} aria-label={`Remove ${group}`}>
                                x
                            </button>
                        </span>
                    ))}
                    <input
                        className="min-w-40 flex-1 border-0 bg-transparent px-1 py-0.5 text-sm outline-none"
                        placeholder={selectedGroups.length === 0 ? "Select host groups" : ""}
                        value={query}
                        onChange={(event) => {
                            onQueryChange(event.target.value);
                            onOpenChange(true);
                        }}
                        onFocus={() => onOpenChange(true)}
                        onClick={() => onOpenChange(true)}
                        onKeyDown={(event) => {
                            if (event.key === "Backspace") {
                                onRemoveLast();
                            }

                            if (event.key === "Enter" && availableGroups.length > 0) {
                                event.preventDefault();
                                onAdd(availableGroups[0]);
                            }
                        }}
                    />
                </div>
            </label>

            {open && menuBox.width > 0 ? createPortal(<HostGroupMenu availableGroups={availableGroups} box={menuBox} onAdd={onAdd} />, document.body) : null}
        </div>
    );
}

function HostGroupMenu({ availableGroups, box, onAdd }: { availableGroups: string[]; box: { left: number; top: number; width: number }; onAdd: (group: string) => void }) {
    return (
        <div
            className="fixed z-[80] max-h-56 overflow-y-auto rounded-sm border border-[#9ca3af] bg-white shadow-lg"
            style={{
                left: box.left,
                top: box.top,
                width: box.width
            }}
        >
            {availableGroups.length > 0 ? (
                availableGroups.map((group) => (
                    <button
                        className="block w-full border-b border-[#e5e7eb] px-3 py-2 text-left text-sm last:border-b-0 hover:bg-[#eef0f2]"
                        key={group}
                        type="button"
                        onMouseDown={(event) => event.preventDefault()}
                        onClick={() => onAdd(group)}
                    >
                        {group}
                    </button>
                ))
            ) : (
                <div className="px-3 py-2 text-sm text-[#6b7280]">No matches</div>
            )}
        </div>
    );
}
