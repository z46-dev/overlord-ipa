import { useEffect, useState } from "react";
import { getHosts } from "../api/client";
import type { Host } from "../api/types";
import { DataTable, type DataTableColumn } from "../components/DataTable";
import { StatusBadge } from "../components/StatusBadge";
import { formatDateTime } from "./format";

interface HostsProps {
    canEdit: boolean;
}

export function Hosts({ canEdit }: HostsProps) {
    const [hosts, setHosts] = useState<Host[]>([]);
    const [error, setError] = useState<string>("");

    useEffect(() => {
        let mounted = true;

        getHosts()
            .then((response) => {
                if (mounted) {
                    setHosts(response);
                }
            })
            .catch((err: unknown) => {
                if (mounted) {
                    setError(err instanceof Error ? err.message : "Unable to load hosts");
                }
            });

        return () => {
            mounted = false;
        };
    }, []);

    const columns: DataTableColumn<Host>[] = [
        { key: "hostname", header: "Host", render: (host) => <span className="font-medium">{host.hostname}</span> },
        { key: "fqdn", header: "FQDN", render: (host) => host.fqdn || "Unknown" },
        { key: "os", header: "OS", render: (host) => [host.os_name, host.os_version].filter(Boolean).join(" ") || "Unknown" },
        { key: "groups", header: "Host groups", render: (host) => host.hostgroups?.join(", ") || "None" },
        { key: "memory", header: "Memory", render: (host) => (host.memory_mb > 0 ? `${host.memory_mb} MB` : "Unknown") },
        { key: "health", header: "Health", render: () => <StatusBadge label="Unknown" tone="neutral" /> },
        { key: "last_seen", header: "Last seen", render: (host) => formatDateTime(host.last_seen_at) }
    ];

    return (
        <div className="space-y-3">
            <div className="flex items-center justify-between">
                <h2 className="text-base font-semibold text-[#1f2933]">Hosts</h2>
                <button
                    className="rounded-sm bg-[#1f6fb2] px-3 py-1.5 text-sm font-medium text-white hover:bg-[#155a96] disabled:cursor-not-allowed disabled:bg-[#9ca3af]"
                    disabled={!canEdit}
                    type="button"
                >
                    Sync Inventory
                </button>
            </div>

            {error ? <div className="rounded border border-red-300 bg-red-50 px-3 py-2 text-sm text-red-800">{error}</div> : null}
            <DataTable columns={columns} rows={hosts} getRowKey={(host) => host.id || host.hostname} emptyLabel="No hosts found" />
        </div>
    );
}
