import { Menu, Shield, X } from "lucide-react";
import { useMemo, useState } from "react";
import { Link, Outlet, useLocation, useNavigate } from "react-router-dom";
import { clearPanelToken } from "../auth";
import { ConfirmDialog } from "../components/ConfirmDialog";

const nav = [
  { to: "/", label: "Overview", shortLabel: "Home" },
  { to: "/mint-pool", label: "Mint Pool", shortLabel: "Mint" },
  { to: "/miners", label: "Miners", shortLabel: "Mine" },
  { to: "/users", label: "Users", shortLabel: "Users" },
  { to: "/nodes", label: "Nodes", shortLabel: "Nodes" },
  { to: "/settings", label: "Settings", shortLabel: "Prefs" }
];

export function AppLayout() {
  const location = useLocation();
  const navigate = useNavigate();
  const [logoutDialogOpen, setLogoutDialogOpen] = useState(false);
  const [mobileNavOpen, setMobileNavOpen] = useState(false);

  const currentNav = useMemo(
    () => nav.find((item) => location.pathname === item.to) ?? nav[0],
    [location.pathname]
  );

  const confirmLogout = () => {
    clearPanelToken();
    navigate("/login", { replace: true });
  };

  return (
    <div className="h-screen overflow-hidden px-3 py-3 sm:px-5 sm:py-5 lg:px-6 lg:py-6">
      <div className="mx-auto grid h-full min-h-0 max-w-[1600px] gap-4 lg:grid-cols-[260px,minmax(0,1fr)]">
        <aside className="panel-surface hidden h-full min-h-0 flex-col justify-between overflow-y-auto p-4 lg:flex">
          <div>
            <div className="flex items-center gap-3 rounded-3xl border border-white/10 bg-white/[0.05] p-3">
              <div className="flex h-12 w-12 items-center justify-center rounded-2xl bg-sky-400/15 text-sky-300">
                <Shield className="h-6 w-6" />
              </div>
              <div>
                <p className="metric-kicker">VPN Control</p>
                <h1 className="mt-1 font-display text-xl font-bold text-white">MeiMei Panel</h1>
              </div>
            </div>
            <div className="mt-8">
              <p className="metric-kicker">Workspace</p>
            </div>
            <nav className="mt-8 space-y-2">
              {nav.map((item) => {
                const active = location.pathname === item.to;
                return (
                  <Link
                    key={item.to}
                    to={item.to}
                    className={`flex items-center justify-between rounded-2xl px-4 py-3 text-sm font-semibold transition ${
                      active
                        ? "bg-white text-slate-950 shadow-glow"
                        : "border border-transparent text-slate-300 hover:border-white/10 hover:bg-white/[0.04]"
                    }`}
                  >
                    <span>{item.label}</span>
                  </Link>
                );
              })}
            </nav>
          </div>
          <div className="space-y-4">
            <button type="button" onClick={() => setLogoutDialogOpen(true)} className="btn-secondary w-full">
              Log Out
            </button>
          </div>
        </aside>

        <div className="min-w-0 min-h-0 overflow-x-hidden">
          <div className="flex h-full min-h-0 flex-col">
            <header className="panel-surface z-40 mb-4 shrink-0 p-3 sm:p-4">
            <div className="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
              <div className="flex items-start justify-between gap-3">
                <div>
                  <p className="metric-kicker">Sing-box VPN Panel</p>
                  <h2 className="mt-2 font-display text-2xl font-bold text-white sm:text-3xl">{currentNav.label}</h2>
                </div>
                <button
                  type="button"
                  onClick={() => setMobileNavOpen((current) => !current)}
                  className="btn-secondary shrink-0 lg:hidden"
                  aria-label={mobileNavOpen ? "Close navigation" : "Open navigation"}
                >
                  {mobileNavOpen ? <X className="h-4 w-4" /> : <Menu className="h-4 w-4" />}
                </button>
              </div>

              <div className="hidden flex-wrap items-center gap-3 sm:flex">
              </div>
            </div>

            {mobileNavOpen ? (
              <div className="mt-4 grid gap-2 border-t border-white/10 pt-4 lg:hidden">
                {nav.map((item) => {
                  const active = location.pathname === item.to;
                  return (
                    <Link
                      key={item.to}
                      to={item.to}
                      onClick={() => setMobileNavOpen(false)}
                      className={`rounded-2xl px-4 py-3 text-sm font-semibold transition ${
                        active ? "bg-white text-slate-950" : "bg-white/[0.04] text-slate-200"
                      }`}
                    >
                      {item.label}
                    </Link>
                  );
                })}
                <button type="button" onClick={() => setLogoutDialogOpen(true)} className="btn-secondary mt-2">
                  Log Out
                </button>
              </div>
            ) : null}
            </header>

            <main className="min-h-0 flex-1 overflow-x-hidden overflow-y-auto">
              <div className="min-w-0 space-y-4 pb-1 pr-1">
                <Outlet />
              </div>
            </main>
          </div>
        </div>
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
