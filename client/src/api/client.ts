import type {
    AuthenticatedUser,
    DashboardSummary,
    DataFileContent,
    DataFileInfo,
    DataFileInput,
    Host,
    Job,
    JobInput,
    JobsResponse,
    LoginRequest
} from "./types";

const jsonHeaders = {
    Accept: "application/json",
    "Content-Type": "application/json"
};

interface APIErrorResponse {
    error?: {
        code?: string;
        message?: string;
    };
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
    const response = await fetch(path, {
        credentials: "same-origin",
        ...init,
        headers: {
            ...jsonHeaders,
            ...init.headers
        }
    });
    if (!response.ok) {
        const message = await readErrorMessage(response);
        throw new Error(message || `API request failed with status ${response.status}`);
    }

    if (response.status === 204) {
        return undefined as T;
    }
    return (await response.json()) as T;
}

async function readErrorMessage(response: Response): Promise<string> {
    try {
        const body = (await response.json()) as APIErrorResponse;
        return body.error?.message ?? "";
    } catch {
        return "";
    }
}

export function login(requestBody: LoginRequest): Promise<AuthenticatedUser> {
    return request<AuthenticatedUser>("/api/auth/login", {
        method: "POST",
        body: JSON.stringify(requestBody)
    });
}

export function logout(): Promise<void> {
    return request<void>("/api/auth/logout", { method: "POST" });
}

export function getCurrentUser(): Promise<AuthenticatedUser> {
    return request<AuthenticatedUser>("/api/auth/me");
}

export function getDashboardSummary(): Promise<DashboardSummary> {
    return request<DashboardSummary>("/api/dashboard/summary");
}

export function getHosts(): Promise<Host[]> {
    return request<Host[]>("/api/hosts");
}

export function getHostGroups(): Promise<string[]> {
    return request<string[]>("/api/hostgroups");
}

export function getJobs(): Promise<JobsResponse> {
    return request<JobsResponse>("/api/jobs");
}

export function createJob(requestBody: JobInput): Promise<Job> {
    return request<Job>("/api/jobs", {
        method: "POST",
        body: JSON.stringify(requestBody)
    });
}

export function updateJob(id: number, requestBody: JobInput): Promise<Job> {
    return request<Job>(`/api/jobs/${id}`, {
        method: "PUT",
        body: JSON.stringify(requestBody)
    });
}

export function getDataFiles(): Promise<DataFileInfo[]> {
    return request<DataFileInfo[]>("/api/data/files");
}

export function getDataFile(path: string): Promise<DataFileContent> {
    return request<DataFileContent>(`/api/data/file?path=${encodeURIComponent(path)}`);
}

export function saveDataFile(requestBody: DataFileInput): Promise<DataFileContent> {
    return request<DataFileContent>("/api/data/file", {
        method: "PUT",
        body: JSON.stringify(requestBody)
    });
}

export function deleteDataFile(path: string): Promise<void> {
    return request<void>("/api/data/file", {
        method: "DELETE",
        body: JSON.stringify({ path })
    });
}

export async function downloadDataFile(path: string): Promise<void> {
    const response = await fetch(`/api/data/download?path=${encodeURIComponent(path)}`, {
        credentials: "same-origin"
    });
    if (!response.ok) {
        const message = await readErrorMessage(response);
        throw new Error(message || `API request failed with status ${response.status}`);
    }

    const blob = await response.blob();
    const url = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = url;
    link.download = path.split("/").pop() || "download";
    document.body.appendChild(link);
    link.click();
    link.remove();
    URL.revokeObjectURL(url);
}
