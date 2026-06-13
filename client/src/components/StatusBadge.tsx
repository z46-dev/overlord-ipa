type StatusTone = "neutral" | "success" | "warning" | "danger" | "info";

interface StatusBadgeProps {
    label: string;
    tone?: StatusTone;
}

const toneClasses: Record<StatusTone, string> = {
    neutral: "border-gray-300 bg-gray-100 text-gray-700",
    success: "border-emerald-300 bg-emerald-50 text-emerald-800",
    warning: "border-amber-300 bg-amber-50 text-amber-800",
    danger: "border-red-300 bg-red-50 text-red-800",
    info: "border-blue-300 bg-blue-50 text-blue-800"
};

export function StatusBadge({ label, tone = "neutral" }: StatusBadgeProps) {
    return (
        <span className={`inline-flex items-center rounded-sm border px-2 py-0.5 text-xs font-medium ${toneClasses[tone]}`}>
            {label}
        </span>
    );
}
