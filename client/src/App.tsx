import { useEffect, useState } from "react";
import { getCurrentUser, logout } from "./api/client";
import type { AuthenticatedUser } from "./api/types";
import { Data } from "./pages/Data";
import { Dashboard } from "./pages/Dashboard";
import { Hosts } from "./pages/Hosts";
import { Jobs } from "./pages/Jobs";

type Page = "dashboard" | "hosts" | "jobs" | "data";

interface AppRoute {
    page: Page;
    jobID: number | null;
    shouldReplace: boolean;
}

const navItems: Array<{ id: Page; label: string; path: string }> = [
    { id: "dashboard", label: "Dashboard", path: "/dashboard" },
    { id: "hosts", label: "Hosts", path: "/hosts" },
    { id: "jobs", label: "Jobs", path: "/jobs" },
    { id: "data", label: "Data", path: "/data" }
];

export function App() {
    const initialRoute = routeFromLocation();
    const [route, setRoute] = useState<AppRoute>(initialRoute);
    const [user, setUser] = useState<AuthenticatedUser | null>(null);
    const [checkingUser, setCheckingUser] = useState(true);
    const [jobToOpen, setJobToOpen] = useState<number | null>(initialRoute.jobID);

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
                    setCheckingUser(false);
                }
            });

        return () => {
            mounted = false;
        };
    }, []);

    useEffect(() => {
        const handlePopState = () => {
            const nextRoute = routeFromLocation();
            setRoute(nextRoute);
            setJobToOpen(nextRoute.jobID);
        };

        window.addEventListener("popstate", handlePopState);
        return () => {
            window.removeEventListener("popstate", handlePopState);
        };
    }, []);

    useEffect(() => {
        if (checkingUser || !user) {
            return;
        }

        if (route.shouldReplace) {
            navigate(route, true);
            return;
        }

        if (route.page === "data" && !user.can_edit) {
            navigate({ page: "dashboard", jobID: null, shouldReplace: false }, true);
        }
    }, [checkingUser, user, route]);

    const navigate = (nextRoute: AppRoute, replace = false) => {
        const path = pathForRoute(nextRoute);
        const normalizedRoute: AppRoute = { ...nextRoute, shouldReplace: false };

        setRoute(normalizedRoute);
        setJobToOpen(nextRoute.jobID);

        if (replace) {
            window.history.replaceState(null, "", path);
            return;
        }

        if (window.location.pathname !== path) {
            window.history.pushState(null, "", path);
        }
    };

    const handleLogout = () => {
        logout().finally(() => {
            setUser(null);
            navigate({ page: "dashboard", jobID: null, shouldReplace: false }, true);
            setJobToOpen(null);
        });
    };

    const openJob = (jobID: number) => {
        navigate({ page: "jobs", jobID, shouldReplace: false });
    };

    if (checkingUser) {
        return null;
    }

    if (!user) {
        window.location.replace("/login");
        return null;
    }

    const page = route.page;
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

            <nav className="border-b border-[#b9c0c8] bg-white px-5">
                <div className="flex h-10 items-end gap-1">
                    {visibleNavItems.map((item) => (
                        <button
                            key={item.id}
                            className={`border border-b-0 px-4 py-2 text-sm font-medium ${
                                page === item.id
                                    ? "border-[#b9c0c8] bg-[#f5f5f5] text-[#1f2933]"
                                    : "border-transparent text-[#1f2933] hover:border-[#d1d5db] hover:bg-[#eef0f2]"
                            }`}
                            type="button"
                            onClick={() => navigate({ page: item.id, jobID: null, shouldReplace: false })}
                        >
                            {item.label}
                        </button>
                    ))}
                </div>
            </nav>

            <main className="min-w-0">
                <div className="border-b border-[#d1d5db] bg-[#f5f5f5] px-5 py-3">
                    <h1 className="text-lg font-semibold">{visibleNavItems.find((item) => item.id === page)?.label}</h1>
                </div>
                <div className="p-5">
                    {page === "dashboard" ? <Dashboard onOpenJob={openJob} /> : null}
                    {page === "hosts" ? <Hosts canEdit={user.can_edit} /> : null}
                    {page === "jobs" ? (
                        <Jobs
                            canEdit={user.can_edit}
                            openJobID={jobToOpen}
                            onJobClosed={() => {
                                if (route.jobID !== null) {
                                    navigate({ page: "jobs", jobID: null, shouldReplace: false }, true);
                                }
                            }}
                            onJobSelected={(jobID) => navigate({ page: "jobs", jobID, shouldReplace: false })}
                            onOpenJobHandled={() => setJobToOpen(null)}
                        />
                    ) : null}
                    {page === "data" ? <Data canEdit={user.can_edit} /> : null}
                </div>
            </main>
        </div>
    );
}

function routeFromLocation(): AppRoute {
    const path = window.location.pathname.replace(/\/+$/, "") || "/";
    const parts = path.split("/").filter(Boolean);

    if (parts.length === 0) {
        return { page: "dashboard", jobID: null, shouldReplace: true };
    }

    if (parts.length === 1) {
        switch (parts[0]) {
            case "dashboard":
                return { page: "dashboard", jobID: null, shouldReplace: false };
            case "hosts":
                return { page: "hosts", jobID: null, shouldReplace: false };
            case "jobs":
                return { page: "jobs", jobID: null, shouldReplace: false };
            case "data":
                return { page: "data", jobID: null, shouldReplace: false };
            default:
                return { page: "dashboard", jobID: null, shouldReplace: true };
        }
    }

    if (parts.length === 2 && parts[0] === "jobs") {
        const jobID = Number(parts[1]);
        if (Number.isInteger(jobID) && jobID > 0) {
            return { page: "jobs", jobID, shouldReplace: false };
        }
    }

    return { page: "dashboard", jobID: null, shouldReplace: true };
}

function pathForRoute(route: AppRoute): string {
    if (route.page === "jobs" && route.jobID !== null) {
        return `/jobs/${route.jobID}`;
    }

    return navItems.find((item) => item.id === route.page)?.path ?? "/dashboard";
}
