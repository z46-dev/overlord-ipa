import { useEffect, useState } from "react";
import { getDashboardSummary, getJobs } from "../api/client";
import type { DashboardSummary, Job, JobRun, JobsResponse } from "../api/types";
import { Card } from "../components/Card";
import { DataTable, type DataTableColumn } from "../components/DataTable";
import { StatusBadge } from "../components/StatusBadge";
import { formatDateTime, formatSchedule, jobEnabledTone, jobRunStatusLabel, jobRunStatusTone, scheduleTone, scheduleTypeLabel } from "./format";

const emptySummary: DashboardSummary = {
    hosts: 0,
    jobs: 0,
    queued_jobs: 0,
    running_jobs: 0,
    failed_jobs: 0,
    recent_runs: []
};

interface DashboardProps {
    onOpenJob: (jobID: number) => void;
}

export function Dashboard({ onOpenJob }: DashboardProps) {
    const [summary, setSummary] = useState<DashboardSummary>(emptySummary);
    const [jobs, setJobs] = useState<Job[]>([]);
    const jobNames = new Map(jobs.map((job) => [job.id, job.name]));
    const [error, setError] = useState<string>("");

    const loadDashboard = (shouldApply: () => boolean = () => true) => {
        setError("");
        Promise.all([getDashboardSummary(), getJobs()])
            .then(([summaryResponse, jobsResponse]: [DashboardSummary, JobsResponse]) => {
                if (!shouldApply()) {
                    return;
                }

                setSummary(summaryResponse);
                setJobs(jobsResponse.jobs);
            })
            .catch((err: unknown) => {
                if (shouldApply()) {
                    setError(err instanceof Error ? err.message : "Unable to load dashboard data");
                }
            });
    };

    useEffect(() => {
        let mounted = true;

        loadDashboard(() => mounted);

        return () => {
            mounted = false;
        };
    }, []);

    const columns: DataTableColumn<Job>[] = [
        {
            key: "name",
            header: "Job",
            render: (job) => (
                <button className="font-medium text-[#1f6fb2] hover:underline" type="button" onClick={() => onOpenJob(job.id)}>
                    {job.name}
                </button>
            )
        },
        { key: "schedule", header: "Schedule", render: (job) => <ScheduleCell job={job} /> },
        {
            key: "status",
            header: "Status",
            render: (job) => <StatusBadge label={job.enabled ? "Enabled" : "Disabled"} tone={jobEnabledTone(job.enabled)} />
        },
        { key: "targets", header: "Targets", render: (job) => job.target_hostgroups?.join(", ") || "None" }
    ];

    const runColumns: DataTableColumn<JobRun>[] = [
        {
            key: "status",
            header: "Status",
            render: (run) => <StatusBadge label={jobRunStatusLabel(run)} tone={jobRunStatusTone(run)} />
        },
        {
            key: "job",
            header: "Job",
            render: (run) => (
                <button className="font-medium text-[#1f6fb2] hover:underline" type="button" onClick={() => onOpenJob(Number(run.job_id))}>
                    {jobNames.get(Number(run.job_id)) ?? `Job #${run.job_id}`}
                </button>
            )
        },
        { key: "started", header: "Started", render: (run) => formatDateTime(run.start_time) },
        { key: "targets", header: "Targets", render: (run) => run.total_hosts || run.target_hosts?.length || 0 },
        {
            key: "summary",
            header: "Summary",
            render: (run) => <span className={run.error ? "text-red-800" : ""}>{run.error || run.summary || "No details"}</span>
        }
    ];

    return (
        <div className="space-y-4">
            {error ? <div className="rounded border border-red-300 bg-red-50 px-3 py-2 text-sm text-red-800">{error}</div> : null}

            <div className="grid gap-3 md:grid-cols-5">
                <Card title="Total hosts" value={summary.hosts} detail="FreeIPA inventory" />
                <Card title="Total jobs" value={summary.jobs} detail="Configured automation" />
                <Card title="Queued jobs" value={summary.queued_jobs} detail="Waiting for worker" />
                <Card title="Running jobs" value={summary.running_jobs} detail="Active executions" />
                <Card title="Failed jobs" value={summary.failed_jobs} detail="Recent failures" />
            </div>

            <section>
                <div className="mb-2 flex items-center justify-between">
                    <h2 className="text-base font-semibold text-[#1f2933]">Automation Jobs</h2>
                    <button className="rounded-sm bg-[#1f6fb2] px-3 py-1.5 text-sm font-medium text-white hover:bg-[#155a96]" type="button" onClick={() => loadDashboard()}>
                        Refresh
                    </button>
                </div>
                <DataTable columns={columns} rows={jobs.slice(0, 6)} getRowKey={(job) => job.id} emptyLabel="No jobs configured" />
            </section>

            <section>
                <div className="mb-2 flex items-center justify-between">
                    <h2 className="text-base font-semibold text-[#1f2933]">Recent Job Runs</h2>
                </div>
                <DataTable columns={runColumns} rows={summary.recent_runs ?? []} getRowKey={(run) => run.id} emptyLabel="No job runs recorded" />
            </section>
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
