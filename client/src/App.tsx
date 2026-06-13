import { FormEvent, useEffect, useState } from "react";
import { getCurrentUser, login, logout } from "./api/client";
import type { AuthenticatedUser } from "./api/types";
import { Data } from "./pages/Data";
import { Dashboard } from "./pages/Dashboard";
import { Hosts } from "./pages/Hosts";
import { Jobs } from "./pages/Jobs";

type Page = "dashboard" | "hosts" | "jobs" | "data";

const navItems: Array<{ id: Page; label: string }> = [
    { id: "dashboard", label: "Dashboard" },
    { id: "hosts", label: "Hosts" },
    { id: "jobs", label: "Jobs" },
    { id: "data", label: "Data" }
];

export function App() {
    const [page, setPage] = useState<Page>("dashboard");
    const [user, setUser] = useState<AuthenticatedUser | null>(null);
    const [loadingUser, setLoadingUser] = useState(true);

    useEffect(() => {
        let mounted = true;

        getCurrentUser()
            .then((response) => {
                if (mounted) {
                    setUser(response);
                }
            })
            .catch(() => {
                if (mounted) {
                    setUser(null);
                }
            })
            .finally(() => {
                if (mounted) {
                    setLoadingUser(false);
                }
            });

        return () => {
            mounted = false;
        };
    }, []);

    const handleLogout = () => {
        logout().finally(() => {
            setUser(null);
            setPage("dashboard");
        });
    };

    if (loadingUser) {
        return (
            <div className="flex min-h-screen items-center justify-center bg-[#f5f5f5] text-sm text-[#6b7280]">
                Loading
            </div>
        );
    }

    if (!user) {
        return <LoginScreen onLogin={setUser} />;
    }

    const visibleNavItems = user.can_edit ? navItems : navItems.filter((item) => item.id !== "data");

    return (
        <div className="min-h-screen bg-[#f5f5f5] text-[#1f2933]">
            <header className="border-b border-[#1f2327] bg-[#2b2f33] text-white">
                <div className="flex h-12 items-center px-5">
                    <div className="text-sm font-semibold">Overlord IPA</div>
                    <div className="ml-6 text-xs text-gray-300">Infrastructure automation</div>
                    <div className="ml-auto flex items-center gap-3 text-xs">
                        <span className="text-gray-300">
                            {user.display_name || user.username} · {user.can_edit ? "Editor" : "Viewer"}
                        </span>
                        <button className="rounded-sm border border-gray-500 px-2 py-1 hover:bg-[#3a4046]" type="button" onClick={handleLogout}>
                            Log out
                        </button>
                    </div>
                </div>
            </header>

            <div className="flex min-h-[calc(100vh-3rem)]">
                <aside className="w-56 shrink-0 border-r border-[#d1d5db] bg-white">
                    <nav className="p-2">
                        {visibleNavItems.map((item) => (
                            <button
                                key={item.id}
                                className={`block w-full rounded-sm px-3 py-2 text-left text-sm font-medium ${
                                    page === item.id ? "bg-[#1f6fb2] text-white" : "text-[#1f2933] hover:bg-[#eef0f2]"
                                }`}
                                type="button"
                                onClick={() => setPage(item.id)}
                            >
                                {item.label}
                            </button>
                        ))}
                    </nav>
                </aside>

                <main className="min-w-0 flex-1">
                    <div className="border-b border-[#d1d5db] bg-white px-5 py-3">
                        <h1 className="text-lg font-semibold">{visibleNavItems.find((item) => item.id === page)?.label}</h1>
                    </div>
                    <div className="p-5">
                        {page === "dashboard" ? <Dashboard /> : null}
                        {page === "hosts" ? <Hosts canEdit={user.can_edit} /> : null}
                        {page === "jobs" ? <Jobs canEdit={user.can_edit} /> : null}
                        {page === "data" ? <Data canEdit={user.can_edit} /> : null}
                    </div>
                </main>
            </div>
        </div>
    );
}

function LoginScreen({ onLogin }: { onLogin: (user: AuthenticatedUser) => void }) {
    const [username, setUsername] = useState("");
    const [password, setPassword] = useState("");
    const [error, setError] = useState("");
    const [submitting, setSubmitting] = useState(false);

    const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
        event.preventDefault();
        setError("");
        setSubmitting(true);

        login({ username, password })
            .then(onLogin)
            .catch((err: unknown) => {
                setError(err instanceof Error ? err.message : "Login failed");
            })
            .finally(() => {
                setSubmitting(false);
            });
    };

    return (
        <div className="min-h-screen bg-[#f5f5f5] text-[#1f2933]">
            <header className="border-b border-[#1f2327] bg-[#2b2f33] text-white">
                <div className="flex h-12 items-center px-5">
                    <div className="text-sm font-semibold">Overlord IPA</div>
                    <div className="ml-6 text-xs text-gray-300">Infrastructure automation</div>
                </div>
            </header>

            <main className="mx-auto mt-16 w-full max-w-md rounded border border-[#d1d5db] bg-white">
                <div className="border-b border-[#d1d5db] px-4 py-3">
                    <h1 className="text-base font-semibold">Log in with FreeIPA</h1>
                </div>
                <form className="space-y-4 p-4" autoComplete="on" onSubmit={handleSubmit}>
                    {error ? <div className="rounded border border-red-300 bg-red-50 px-3 py-2 text-sm text-red-800">{error}</div> : null}
                    <label className="block text-sm font-medium" htmlFor="username">
                        Username
                        <input
                            id="username"
                            name="username"
                            className="mt-1 w-full rounded-sm border border-[#d1d5db] px-3 py-2 text-sm outline-none focus:border-[#1f6fb2]"
                            autoComplete="username"
                            type="text"
                            value={username}
                            onChange={(event) => setUsername(event.target.value)}
                            onInput={(event) => setUsername(event.currentTarget.value)}
                        />
                    </label>
                    <label className="block text-sm font-medium" htmlFor="password">
                        Password
                        <input
                            id="password"
                            name="password"
                            className="mt-1 w-full rounded-sm border border-[#d1d5db] px-3 py-2 text-sm outline-none focus:border-[#1f6fb2]"
                            autoComplete="current-password"
                            type="password"
                            value={password}
                            onChange={(event) => setPassword(event.target.value)}
                            onInput={(event) => setPassword(event.currentTarget.value)}
                        />
                    </label>
                    <button
                        className="w-full rounded-sm bg-[#1f6fb2] px-3 py-2 text-sm font-medium text-white hover:bg-[#155a96] disabled:cursor-not-allowed disabled:bg-[#9ca3af]"
                        disabled={submitting}
                        type="submit"
                    >
                        {submitting ? "Signing in" : "Sign in"}
                    </button>
                </form>
            </main>
        </div>
    );
}
