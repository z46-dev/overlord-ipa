import CodeMirror from "@uiw/react-codemirror";
import { StreamLanguage } from "@codemirror/language";
import { yaml } from "@codemirror/lang-yaml";
import { shell } from "@codemirror/legacy-modes/mode/shell";
import { ChangeEvent, useEffect, useMemo, useState } from "react";
import { deleteDataFile, downloadDataFile, getDataFile, getDataFiles, saveDataFile } from "../api/client";
import type { DataFileContent, DataFileInfo } from "../api/types";
import { DataTable, type DataTableColumn } from "../components/DataTable";
import { StatusBadge } from "../components/StatusBadge";
import { formatDateTime } from "./format";

interface DataProps {
    canEdit: boolean;
}

const emptyFile: DataFileContent = {
    path: "",
    name: "",
    kind: "other",
    size: 0,
    modified_at: "",
    protected: false,
    content: ""
};

export function Data({ canEdit }: DataProps) {
    const [files, setFiles] = useState<DataFileInfo[]>([]);
    const [selectedFile, setSelectedFile] = useState<DataFileContent>(emptyFile);
    const [newPath, setNewPath] = useState("");
    const [error, setError] = useState("");
    const [saving, setSaving] = useState(false);

    const editorExtensions = useMemo(() => {
        if (selectedFile.kind === "playbook") {
            return [yaml()];
        }

        if (selectedFile.kind === "shell") {
            return [StreamLanguage.define(shell)];
        }

        return [];
    }, [selectedFile.kind]);

    const loadFiles = () => {
        getDataFiles()
            .then((response) => {
                setFiles(response);
            })
            .catch((err: unknown) => {
                setError(err instanceof Error ? err.message : "Unable to load data files");
            });
    };

    useEffect(() => {
        loadFiles();
    }, []);

    const openFile = (path: string) => {
        setError("");
        getDataFile(path)
            .then((response) => {
                setSelectedFile(response);
                setNewPath(response.path);
            })
            .catch((err: unknown) => {
                setError(err instanceof Error ? err.message : "Unable to open data file");
            });
    };

    const newFile = () => {
        setSelectedFile(emptyFile);
        setNewPath("");
        setError("");
    };

    const saveFile = () => {
        setError("");
        setSaving(true);

        saveDataFile({
            path: newPath,
            content: selectedFile.content
        })
            .then((response) => {
                setSelectedFile(response);
                setNewPath(response.path);
                loadFiles();
            })
            .catch((err: unknown) => {
                setError(err instanceof Error ? err.message : "Unable to save data file");
            })
            .finally(() => {
                setSaving(false);
            });
    };

    const removeFile = () => {
        if (!selectedFile.path) {
            return;
        }

        setError("");
        deleteDataFile(selectedFile.path)
            .then(() => {
                setSelectedFile(emptyFile);
                setNewPath("");
                loadFiles();
            })
            .catch((err: unknown) => {
                setError(err instanceof Error ? err.message : "Unable to delete data file");
            });
    };

    const downloadFile = (path: string) => {
        setError("");
        downloadDataFile(path).catch((err: unknown) => {
            setError(err instanceof Error ? err.message : "Unable to download data file");
        });
    };

    const uploadFile = (event: ChangeEvent<HTMLInputElement>) => {
        const file = event.target.files?.[0];
        if (!file) {
            return;
        }

        file.text()
            .then((content) => {
                setSelectedFile({
                    ...emptyFile,
                    path: file.name,
                    name: file.name,
                    content
                });
                setNewPath(file.name);
            })
            .catch((err: unknown) => {
                setError(err instanceof Error ? err.message : "Unable to read selected file");
            });
    };

    const columns: DataTableColumn<DataFileInfo>[] = [
        { key: "path", header: "Path", render: (file) => <button className="font-medium text-[#1f6fb2]" type="button" onClick={() => openFile(file.path)}>{file.path}</button> },
        { key: "kind", header: "Kind", render: (file) => <StatusBadge label={file.kind} tone={file.kind === "other" ? "neutral" : "info"} /> },
        { key: "protected", header: "Access", render: (file) => <StatusBadge label={file.protected ? "Protected" : "Editable"} tone={file.protected ? "neutral" : "success"} /> },
        { key: "size", header: "Size", render: (file) => formatBytes(file.size) },
        { key: "modified", header: "Modified", render: (file) => formatDateTime(file.modified_at) },
        {
            key: "download",
            header: "",
            render: (file) => (
                <button className="text-sm font-medium text-[#1f6fb2]" type="button" onClick={() => downloadFile(file.path)}>
                    Download
                </button>
            )
        }
    ];

    return (
        <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_minmax(420px,0.8fr)]">
            <section className="min-w-0 space-y-3">
                <div className="flex items-center justify-between">
                    <h2 className="text-base font-semibold text-[#1f2933]">Data Files</h2>
                    <div className="flex items-center gap-2">
                        <label className="rounded-sm border border-[#1f6fb2] px-3 py-1.5 text-sm font-medium text-[#1f6fb2]">
                            Upload
                            <input className="hidden" disabled={!canEdit} type="file" onChange={uploadFile} />
                        </label>
                        <button
                            className="rounded-sm bg-[#1f6fb2] px-3 py-1.5 text-sm font-medium text-white hover:bg-[#155a96] disabled:cursor-not-allowed disabled:bg-[#9ca3af]"
                            disabled={!canEdit}
                            type="button"
                            onClick={newFile}
                        >
                            New File
                        </button>
                    </div>
                </div>

                {error ? <div className="rounded border border-red-300 bg-red-50 px-3 py-2 text-sm text-red-800">{error}</div> : null}
                <DataTable columns={columns} rows={files} getRowKey={(file) => file.path} emptyLabel="No data files found" />
            </section>

            <section className="min-w-0 rounded-sm border border-[#d1d5db] bg-white">
                <div className="border-b border-[#d1d5db] bg-[#eef0f2] px-3 py-2 text-sm font-semibold">
                    {selectedFile.path || "New File"}
                </div>
                <div className="space-y-3 p-3">
                    <label className="block text-sm font-medium">
                        Path
                        <input
                            className="mt-1 w-full rounded-sm border border-[#d1d5db] px-2 py-1.5 text-sm"
                            disabled={!canEdit || selectedFile.protected}
                            value={newPath}
                            onChange={(event) => setNewPath(event.target.value)}
                        />
                    </label>
                    <div className="overflow-hidden rounded-sm border border-[#d1d5db] text-sm">
                        <CodeMirror
                            editable={canEdit && !selectedFile.protected}
                            extensions={editorExtensions}
                            height="420px"
                            basicSetup={{
                                foldGutter: true,
                                highlightActiveLine: true,
                                lineNumbers: true
                            }}
                            value={selectedFile.content}
                            onChange={(value) => setSelectedFile({ ...selectedFile, content: value })}
                        />
                    </div>
                    <div className="flex justify-between gap-2">
                        <button
                            className="rounded-sm border border-red-300 px-3 py-1.5 text-sm font-medium text-red-700 disabled:cursor-not-allowed disabled:border-[#d1d5db] disabled:text-[#9ca3af]"
                            disabled={!canEdit || !selectedFile.path || selectedFile.protected}
                            type="button"
                            onClick={removeFile}
                        >
                            Delete
                        </button>
                        <div className="flex gap-2">
                            <button
                                className="rounded-sm border border-[#d1d5db] px-3 py-1.5 text-sm"
                                disabled={!selectedFile.path}
                                type="button"
                                onClick={() => downloadFile(selectedFile.path)}
                            >
                                Download
                            </button>
                            <button
                                className="rounded-sm bg-[#1f6fb2] px-3 py-1.5 text-sm font-medium text-white disabled:cursor-not-allowed disabled:bg-[#9ca3af]"
                                disabled={!canEdit || selectedFile.protected || saving}
                                type="button"
                                onClick={saveFile}
                            >
                                {saving ? "Saving" : "Save"}
                            </button>
                        </div>
                    </div>
                </div>
            </section>
        </div>
    );
}

function formatBytes(value: number): string {
    if (value < 1024) {
        return `${value} B`;
    }

    if (value < 1024 * 1024) {
        return `${Math.round(value / 1024)} KB`;
    }

    return `${Math.round(value / 1024 / 1024)} MB`;
}
