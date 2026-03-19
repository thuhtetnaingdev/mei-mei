import { useState } from "react";
import { Link, Outlet, useLocation, useNavigate } from "react-router-dom";
import { clearPanelToken } from "../auth";
import { ConfirmDialog } from "../components/ConfirmDialog";

const nav = [
  { to: "/", label: "Dashboard" },
  { to: "/users", label: "Users" },
  { to: "/nodes", label: "Nodes" },
  { to: "/settings", label: "Settings" }
];

export function AppLayout() {
  const location = useLocation();
  const navigate = useNavigate();
  const [logoutDialogOpen, setLogoutDialogOpen] = useState(false);

  const confirmLogout = () => {
    clearPanelToken();
    navigate("/login", { replace: true });
  };

  return (
    <div className="min-h-screen px-4 py-6 md:px-8">
      <div className="mx-auto max-w-7xl">
        <header className="mb-8 rounded-[2rem] border border-white/70 bg-ink px-6 py-5 text-white shadow-panel">
          <div className="flex flex-col gap-6 md:flex-row md:items-center md:justify-between">
            <div>
              <p className="text-sm uppercase tracking-[0.25em] text-sky-200">Sing-box Control Plane</p>
              <h1 className="mt-2 font-display text-3xl font-bold">Multi-node Proxy Management</h1>
            </div>
            <div className="flex flex-wrap items-center gap-2">
              <nav className="flex flex-wrap gap-2">
                {nav.map((item) => {
                  const active = location.pathname === item.to;
                  return (
                    <Link
                      key={item.to}
                      to={item.to}
                      className={`rounded-full px-4 py-2 text-sm font-medium transition ${
                        active ? "bg-white text-ink" : "bg-white/10 text-slate-200 hover:bg-white/20"
                      }`}
                    >
                      {item.label}
                    </Link>
                  );
                })}
              </nav>
              <button
                type="button"
                onClick={() => setLogoutDialogOpen(true)}
                className="rounded-full border border-white/20 px-4 py-2 text-sm font-medium text-slate-100 transition hover:bg-white/10"
              >
                Logout
              </button>
            </div>
          </div>
        </header>
        <Outlet />
      </div>
      <ConfirmDialog
        open={logoutDialogOpen}
        title="Log out?"
        description="You will be returned to the login screen and will need to sign in again to manage the panel."
        confirmLabel="Log Out"
        onCancel={() => setLogoutDialogOpen(false)}
        onConfirm={confirmLogout}
      />
    </div>
  );
}
