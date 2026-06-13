import { FormEvent, useEffect, useState } from "react";
import { createJob, getDataFiles, getHostGroups, getJobs, updateJob } from "../api/client";
import type { DataFileInfo, Job, JobAction, JobActionInput, JobActionType, JobInput, SchedulerSnapshot, ScheduleType } from "../api/types";
import { DataTable, type DataTableColumn } from "../components/DataTable";
import { StatusBadge } from "../components/StatusBadge";
import { cronExpressionValid, formatCronDescription, formatSchedule, jobEnabledTone, scheduleTone, scheduleTypeLabel } from "./format";

interface JobsProps {
    canEdit: boolean;
}

const actionTypes: Array<{ value: JobActionType; label: string }> = [
    { value: 1, label: "Ansible playbook" },
    { value: 2, label: "Shell" }
];

const scheduleOptions: Array<{ value: ScheduleType; label: string; detail: string }> = [
    { value: 1, label: "Interval", detail: "Repeats every fixed number of seconds" },
    { value: 3, label: "Cron", detail: "Runs from a cron expression" },
    { value: 2, label: "Manual", detail: "Runs only when started by an editor" }
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

export function Jobs({ canEdit }: JobsProps) {
    const [jobs, setJobs] = useState<Job[]>([]);
    const [jobActions, setJobActions] = useState<Record<string, JobAction[]>>({});
    const [scheduler, setScheduler] = useState<SchedulerSnapshot>({ loaded_jobs: [] });
    const [hostGroups, setHostGroups] = useState<string[]>([]);
    const [dataFiles, setDataFiles] = useState<DataFileInfo[]>([]);
    const [error, setError] = useState<string>("");
    const [formError, setFormError] = useState<string>("");
    const [input, setInput] = useState<JobInput>(newJobInput());
    const [editingJob, setEditingJob] = useState<Job | null>(null);
    const [editorOpen, setEditorOpen] = useState(false);
    const [saving, setSaving] = useState(false);
    const [hostGroupQuery, setHostGroupQuery] = useState("");
    const [hostGroupOpen, setHostGroupOpen] = useState(false);

    const loadJobs = () => {
        getJobs()
            .then((response) => {
                setJobs(response.jobs);
                setJobActions(response.actions ?? {});
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

    const loadedJobIDs = new Set(scheduler.loaded_jobs.map((job) => job.id));
    const availableHostGroups = hostGroups.filter((group) => {
        if (input.target_hostgroups.includes(group)) {
            return false;
        }

        return group.toLowerCase().includes(hostGroupQuery.toLowerCase());
    });
    const selectedAction = input.actions[0] ?? newActionInput();
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
        { key: "targets", header: "Targets", render: (job) => job.target_hostgroups?.join(", ") || "None" },
        { key: "action_file", header: "Action file", render: (job) => jobActions[String(job.id)]?.[0]?.file_path || "None" },
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
            key: "actions",
            header: "",
            render: (job) => (
                <button className="text-sm font-medium text-[#1f6fb2] disabled:text-[#9ca3af]" disabled={!canEdit} type="button" onClick={() => openEditJob(job)}>
                    Edit
                </button>
            )
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
            <DataTable columns={columns} rows={jobs} getRowKey={(job) => job.id} emptyLabel="No jobs configured" />

            {canEdit && editorOpen ? (
                <div
                    className="fixed inset-0 z-50 flex items-start justify-center bg-black/35 px-4 py-10"
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
                                <div className="md:col-span-6">
                                    <div className="mb-1 text-sm font-medium">Schedule</div>
                                    <div className="grid overflow-hidden rounded-sm border border-[#d1d5db] bg-white md:grid-cols-3">
                                        {scheduleOptions.map((option) => (
                                            <button
                                                className={scheduleButtonClass(input.schedule_type === option.value)}
                                                key={option.value}
                                                type="button"
                                                onClick={() => setScheduleType(option.value)}
                                            >
                                                <span className="block text-sm font-semibold">{option.label}</span>
                                                <span className="mt-0.5 block text-xs font-normal opacity-80">{option.detail}</span>
                                            </button>
                                        ))}
                                    </div>
                                </div>

                                {input.schedule_type === 1 ? (
                                    <label className="text-sm font-medium md:col-span-2">
                                        Interval seconds
                                        <input
                                            className="mt-1 w-full rounded-sm border border-[#d1d5db] px-2 py-1.5"
                                            min={1}
                                            type="number"
                                            value={input.interval_seconds}
                                            onChange={(event) => setInput({ ...input, interval_seconds: Number(event.target.value) })}
                                        />
                                    </label>
                                ) : null}

                                {input.schedule_type === 3 ? (
                                    <div className="md:col-span-3">
                                        <label className="block text-sm font-medium">
                                            Cron expression
                                            <input
                                                className="mt-1 w-full rounded-sm border border-[#d1d5db] px-2 py-1.5 font-mono text-xs"
                                                value={input.cron_expr}
                                                onChange={(event) => setInput({ ...input, cron_expr: event.target.value })}
                                            />
                                        </label>
                                        <div className={`mt-1 text-xs ${cronExpressionValid(input.cron_expr) ? "text-[#6b7280]" : "text-red-700"}`}>
                                            {formatCronDescription(input.cron_expr)}
                                        </div>
                                    </div>
                                ) : null}

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

function scheduleButtonClass(active: boolean): string {
    if (active) {
        return "min-h-[3.25rem] border-r border-[#155a96] bg-[#1f6fb2] px-3 py-2 text-left text-white last:border-r-0";
    }

    return "min-h-[3.25rem] border-r border-[#d1d5db] bg-white px-3 py-2 text-left text-[#1f2933] hover:bg-[#eef0f2] last:border-r-0";
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
    return (
        <div
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

            {open ? (
                <div className="absolute bottom-full z-30 mb-1 max-h-56 w-full overflow-y-auto rounded-sm border border-[#9ca3af] bg-white shadow-lg">
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
            ) : null}
        </div>
    );
}
