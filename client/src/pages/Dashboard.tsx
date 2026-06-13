import { useEffect, useState } from "react";
import { getDashboardSummary, getJobs } from "../api/client";
import type { DashboardSummary, Job, JobsResponse } from "../api/types";
import { Card } from "../components/Card";
import { DataTable, type DataTableColumn } from "../components/DataTable";
import { StatusBadge } from "../components/StatusBadge";
import { formatSchedule, jobEnabledTone, scheduleTone, scheduleTypeLabel } from "./format";

const emptySummary: DashboardSummary = {
    hosts: 0,
    jobs: 0,
    running_jobs: 0,
    failed_jobs: 0
};

export function Dashboard() {
    const [summary, setSummary] = useState<DashboardSummary>(emptySummary);
    const [jobs, setJobs] = useState<Job[]>([]);
    const [error, setError] = useState<string>("");

    const loadDashboard = (shouldApply: () => boolean = () => true) => {
        setError("");
        Promise.all([getDashboardSummary(), getJobs()])
            .then(([summaryResponse, jobsResponse]: [DashboardSummary, JobsResponse]) => {
                if (!shouldApply()) {
                    return;
                }

                setSummary(summaryResponse);
                setJobs(jobsResponse.jobs.slice(0, 6));
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
        { key: "name", header: "Job", render: (job) => <span className="font-medium">{job.name}</span> },
        { key: "schedule", header: "Schedule", render: (job) => <ScheduleCell job={job} /> },
        {
            key: "status",
            header: "Status",
            render: (job) => <StatusBadge label={job.enabled ? "Enabled" : "Disabled"} tone={jobEnabledTone(job.enabled)} />
        },
        { key: "targets", header: "Targets", render: (job) => job.target_hostgroups?.join(", ") || "None" }
    ];

    return (
        <div className="space-y-4">
            {error ? <div className="rounded border border-red-300 bg-red-50 px-3 py-2 text-sm text-red-800">{error}</div> : null}

            <div className="grid gap-3 md:grid-cols-4">
                <Card title="Total hosts" value={summary.hosts} detail="FreeIPA inventory" />
                <Card title="Total jobs" value={summary.jobs} detail="Configured automation" />
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
                <DataTable columns={columns} rows={jobs} getRowKey={(job) => job.id} emptyLabel="No jobs configured" />
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
