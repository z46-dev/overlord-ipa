import type { ReactNode } from "react";

export interface DataTableColumn<T> {
    key: string;
    header: string;
    render: (row: T) => ReactNode;
    className?: string;
}

interface DataTableProps<T> {
    columns: DataTableColumn<T>[];
    rows: T[];
    getRowKey: (row: T) => string | number;
    emptyLabel?: string;
    onRowClick?: (row: T) => void;
    rowClassName?: (row: T) => string;
}

export function DataTable<T>({ columns, rows, getRowKey, emptyLabel = "No records", onRowClick, rowClassName }: DataTableProps<T>) {
    return (
        <div className="overflow-hidden rounded border border-[#d1d5db] bg-white">
            <table className="min-w-full border-collapse text-left text-sm">
                <thead className="bg-[#eef0f2] text-xs font-semibold uppercase tracking-normal text-[#4b5563]">
                    <tr>
                        {columns.map((column) => (
                            <th key={column.key} className={`border-b border-[#d1d5db] px-3 py-2 ${column.className ?? ""}`}>
                                {column.header}
                            </th>
                        ))}
                    </tr>
                </thead>
                <tbody className="divide-y divide-[#e5e7eb]">
                    {rows.length > 0 ? (
                        rows.map((row) => (
                            <tr
                                key={getRowKey(row)}
                                className={`hover:bg-[#f8fafc] ${onRowClick ? "cursor-pointer" : ""} ${rowClassName?.(row) ?? ""}`}
                                onClick={() => onRowClick?.(row)}
                            >
                                {columns.map((column) => (
                                    <td key={column.key} className={`px-3 py-2 align-top text-[#1f2933] ${column.className ?? ""}`}>
                                        {column.render(row)}
                                    </td>
                                ))}
                            </tr>
                        ))
                    ) : (
                        <tr>
                            <td className="px-3 py-6 text-center text-sm text-[#6b7280]" colSpan={columns.length}>
                                {emptyLabel}
                            </td>
                        </tr>
                    )}
                </tbody>
            </table>
        </div>
    );
}
