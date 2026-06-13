import type { ReactNode } from "react";

interface CardProps {
    title: string;
    value: ReactNode;
    detail?: string;
}

export function Card({ title, value, detail }: CardProps) {
    return (
        <section className="rounded border border-[#d1d5db] bg-white">
            <div className="border-b border-[#d1d5db] px-4 py-2">
                <h3 className="text-xs font-semibold uppercase tracking-normal text-[#6b7280]">{title}</h3>
            </div>
            <div className="px-4 py-3">
                <div className="text-2xl font-semibold text-[#1f2933]">{value}</div>
                {detail ? <div className="mt-1 text-xs text-[#6b7280]">{detail}</div> : null}
            </div>
        </section>
    );
}
