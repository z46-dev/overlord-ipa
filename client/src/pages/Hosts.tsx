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
        { key: "cpu", header: "CPU", render: (host) => formatCPU(host) },
        { key: "memory", header: "Memory", render: (host) => (host.memory_mb > 0 ? `${host.memory_mb} MB` : "Unknown") },
        { key: "network", header: "Network", render: (host) => formatNetwork(host) },
        { key: "disks", header: "Disks", render: (host) => formatDisks(host) },
        { key: "health", header: "Health", render: (host) => <HealthBadge host={host} /> },
        { key: "last_inventory", header: "Last inventory", render: (host) => formatDateTime(host.last_inventory_at) },
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

function HealthBadge({ host }: { host: Host }) {
    if (!host.last_health_at || host.last_health_at.startsWith("0001-01-01")) {
        return <StatusBadge label="Unknown" tone="neutral" />;
    }

    return <StatusBadge label="Checked" tone="success" />;
}

function formatCPU(host: Host): string {
    const model = host.processor_model || "Unknown CPU";
    const cores = host.processor_cores > 0 ? `${host.processor_cores}c` : "";
    const threads = host.processor_threads > 0 ? `${host.processor_threads}t` : "";
    const count = [cores, threads].filter(Boolean).join("/");

    return count ? `${model} (${count})` : model;
}

function formatNetwork(host: Host): string {
    if (!host.network_addresses || host.network_addresses.length === 0) {
        return "Unknown";
    }

    return host.network_addresses
        .map((address) => [address.ip_address, address.mac_address].filter(Boolean).join(" / "))
        .join(", ");
}

function formatDisks(host: Host): string {
    if (!host.disks || host.disks.length === 0) {
        return "Unknown";
    }

    return host.disks
        .slice(0, 2)
        .map((disk) => `${disk.name}: ${formatBytes(disk.used)} / ${formatBytes(disk.size)}`)
        .join(", ");
}

function formatBytes(value: number): string {
    if (value >= 1024 * 1024 * 1024) {
        return `${Math.round(value / 1024 / 1024 / 1024)} GB`;
    }

    if (value >= 1024 * 1024) {
        return `${Math.round(value / 1024 / 1024)} MB`;
    }

    if (value >= 1024) {
        return `${Math.round(value / 1024)} KB`;
    }

    return `${value} B`;
}
