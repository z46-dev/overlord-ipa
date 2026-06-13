import cronstrue from "cronstrue";
import type { Job } from "../api/types";

export function formatSchedule(job: Job): string {
    switch (job.schedule_type) {
        case 1:
            return job.interval_seconds > 0 ? `Every ${formatDuration(job.interval_seconds)}` : "Interval";
        case 2:
            return "Manual";
        case 3:
            return formatCronDescription(job.cron_expr);
        default:
            return "Unknown";
    }
}

export function formatCronDescription(expression: string): string {
    if (!expression.trim()) {
        return "Cron";
    }

    try {
        return cronstrue.toString(expression, {
            throwExceptionOnParseError: true,
            verbose: true
        });
    } catch {
        return "Invalid cron expression";
    }
}

export function cronExpressionValid(expression: string): boolean {
    if (!expression.trim()) {
        return false;
    }

    try {
        cronstrue.toString(expression, { throwExceptionOnParseError: true });
        return true;
    } catch {
        return false;
    }
}

export function scheduleTypeLabel(job: Job): string {
    switch (job.schedule_type) {
        case 1:
            return "Interval";
        case 2:
            return "Manual";
        case 3:
            return "Cron";
        default:
            return "Unknown";
    }
}

export function scheduleTone(job: Job): "neutral" | "success" | "warning" | "danger" | "info" {
    switch (job.schedule_type) {
        case 1:
            return "info";
        case 2:
            return "neutral";
        case 3:
            return "warning";
        default:
            return "danger";
    }
}

export function jobEnabledTone(enabled: boolean): "success" | "neutral" {
    return enabled ? "success" : "neutral";
}

export function formatDateTime(value: string): string {
    if (!value || value.startsWith("0001-01-01")) {
        return "Never";
    }

    const date = new Date(value);
    if (Number.isNaN(date.getTime())) {
        return "Unknown";
    }

    return date.toLocaleString();
}

function formatDuration(seconds: number): string {
    if (seconds % 86400 === 0) {
        return `${seconds / 86400}d`;
    }

    if (seconds % 3600 === 0) {
        return `${seconds / 3600}h`;
    }

    if (seconds % 60 === 0) {
        return `${seconds / 60}m`;
    }

    return `${seconds}s`;
}
