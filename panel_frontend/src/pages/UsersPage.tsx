import { FormEvent, useEffect, useState } from "react";
import QRCode from "qrcode";
import api from "../api/client";
import { BandwidthUsage } from "../components/BandwidthUsage";
import { ConfirmDialog } from "../components/ConfirmDialog";
import { SectionCard } from "../components/SectionCard";
import type { User } from "../types";

interface SubscriptionResponse {
  userId: number;
  uuid: string;
  subscription: string;
  url: string;
  remoteProfileUrl: string;
  singboxImportUrl: string;
  nodeLinks: Array<{
    nodeName: string;
    protocol: string;
    url: string;
  }>;
}

type UserFormState = {
  email: string;
  enabled: boolean;
  bandwidthLimitGb: number;
  notes: string;
};

const defaultFormState: UserFormState = {
  email: "",
  enabled: true,
  bandwidthLimitGb: 100,
  notes: ""
};

export function UsersPage() {
  const [users, setUsers] = useState<User[]>([]);
  const [form, setForm] = useState<UserFormState>(defaultFormState);
  const [editingUserId, setEditingUserId] = useState<number | null>(null);
  const [userDialogOpen, setUserDialogOpen] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<User | null>(null);
  const [accessDialogOpen, setAccessDialogOpen] = useState(false);
  const [accessLoading, setAccessLoading] = useState(false);
  const [accessError, setAccessError] = useState("");
  const [selectedAccess, setSelectedAccess] = useState<SubscriptionResponse | null>(null);
  const [selectedUser, setSelectedUser] = useState<User | null>(null);
  const [qrCodeUrl, setQrCodeURL] = useState("");
  const [copiedKey, setCopiedKey] = useState("");
  const [formStatus, setFormStatus] = useState("");

  const loadUsers = () => api.get<User[]>("/users").then((res) => setUsers(res.data));
  const syncNodes = () => api.post("/nodes/sync").catch(() => undefined);

  useEffect(() => {
    void loadUsers().catch(() => undefined);
  }, []);

  const resetForm = () => {
    setForm(defaultFormState);
    setEditingUserId(null);
    setFormStatus("");
    setUserDialogOpen(false);
  };

  const submitUser = async (event: FormEvent) => {
    event.preventDefault();
    const payload = {
      email: form.email,
      enabled: form.enabled,
      bandwidthLimitGb: form.bandwidthLimitGb,
      notes: form.notes
    };

    if (editingUserId) {
      await api.patch(`/users/${editingUserId}`, payload);
      setFormStatus("User updated.");
    } else {
      await api.post("/users", payload);
      setFormStatus("User created.");
    }

    await syncNodes();
    await loadUsers();
    resetForm();
  };

  const openAccess = async (user: User) => {
    setSelectedUser(user);
    setSelectedAccess(null);
    setAccessDialogOpen(true);
    setAccessLoading(true);
    setAccessError("");
    setCopiedKey("");
    setQrCodeURL("");

    try {
      const response = await api.get<SubscriptionResponse>(`/subscription/${user.id}`);
      setSelectedAccess(response.data);

      const qrPayload = response.data.singboxImportUrl || response.data.remoteProfileUrl || response.data.url;
      if (qrPayload) {
        const qr = await QRCode.toDataURL(qrPayload, {
          width: 280,
          margin: 1
        });
        setQrCodeURL(qr);
      }
    } catch {
      setAccessError("We could not load this user access bundle. Please try again.");
      setQrCodeURL("");
    } finally {
      setAccessLoading(false);
    }
  };

  const closeAccessDialog = () => {
    setAccessDialogOpen(false);
    setSelectedAccess(null);
    setSelectedUser(null);
    setAccessLoading(false);
    setAccessError("");
    setQrCodeURL("");
    setCopiedKey("");
  };

  const startEdit = (user: User) => {
    setEditingUserId(user.id);
    setForm({
      email: user.email,
      enabled: user.enabled,
      bandwidthLimitGb: user.bandwidthLimitGb,
      notes: user.notes ?? ""
    });
    setFormStatus("");
    setUserDialogOpen(true);
  };

  const openCreateDialog = () => {
    setEditingUserId(null);
    setForm(defaultFormState);
    setFormStatus("");
    setUserDialogOpen(true);
  };

  const deleteUser = async () => {
    if (!deleteTarget) {
      return;
    }

    await api.delete(`/users/${deleteTarget.id}`);
    await syncNodes();
    if (editingUserId === deleteTarget.id) {
      resetForm();
    }
    if (selectedAccess?.userId === deleteTarget.id) {
      closeAccessDialog();
    }
    setFormStatus("User deleted.");
    setDeleteTarget(null);
    await loadUsers();
  };

  const copyText = async (value: string, key: string) => {
    if (!value) {
      return;
    }

    try {
      if (navigator.clipboard?.writeText) {
        await navigator.clipboard.writeText(value);
      } else {
        const textarea = document.createElement("textarea");
        textarea.value = value;
        textarea.style.position = "fixed";
        textarea.style.opacity = "0";
        document.body.appendChild(textarea);
        textarea.focus();
        textarea.select();
        document.execCommand("copy");
        document.body.removeChild(textarea);
      }
      setCopiedKey(key);
      window.setTimeout(() => {
        setCopiedKey((current) => (current === key ? "" : current));
      }, 1600);
    } catch {
      setCopiedKey("");
    }
  };

  const accessCards = selectedAccess
    ? [
        {
          key: "json",
          label: "JSON Profile URL",
          value: selectedAccess.remoteProfileUrl,
          emptyMessage: "This backend is not returning a JSON profile URL yet.",
          copyLabel: "Copy JSON URL"
        },
        {
          key: "import",
          label: "Sing-box Import Link",
          value: selectedAccess.singboxImportUrl,
          emptyMessage: "This backend is not returning a sing-box import link yet.",
          copyLabel: "Copy Import Link",
          openLabel: "Open Import Link"
        },
        {
          key: "legacy",
          label: "Legacy Subscription URL",
          value: selectedAccess.url,
          emptyMessage: "Legacy subscription URL is not available.",
          copyLabel: "Copy Subscription URL"
        }
      ]
    : [];

  return (
    <div className="space-y-8">
      <SectionCard
        title="Users"
        description="Manage users from the table, then open access links and QR codes for mobile import."
        action={
          <button
            type="button"
            onClick={openCreateDialog}
            className="rounded-full bg-ink px-4 py-2 text-sm font-semibold text-white transition hover:bg-slate-800"
          >
            Add User
          </button>
        }
      >
        <div className="space-y-4">
          {formStatus ? <p className="text-sm text-slate-500">{formStatus}</p> : null}
          <div className="overflow-hidden rounded-2xl border border-slate-100">
            <table className="min-w-full divide-y divide-slate-100 text-left text-sm">
              <thead className="bg-slate-50 text-slate-500">
                <tr>
                  <th className="px-4 py-3">Email</th>
                  <th className="px-4 py-3">UUID</th>
                  <th className="px-4 py-3">Status</th>
                  <th className="px-4 py-3">Bandwidth Usage</th>
                  <th className="px-4 py-3">Manage</th>
                  <th className="px-4 py-3">Access</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100 bg-white">
                {users.map((user) => (
                  <tr key={user.id}>
                    <td className="px-4 py-3">{user.email}</td>
                    <td className="px-4 py-3 font-mono text-xs">{user.uuid}</td>
                    <td className="px-4 py-3">{user.enabled ? "Enabled" : "Disabled"}</td>
                    <td className="px-4 py-3">
                      <div className="min-w-[200px]">
                        <BandwidthUsage usedBytes={user.bandwidthUsedBytes} limitGb={user.bandwidthLimitGb} showDetails={false} />
                      </div>
                    </td>
                    <td className="px-4 py-3">
                      <div className="flex flex-wrap gap-2">
                        <button
                          onClick={() => startEdit(user)}
                          className="rounded-full border border-slate-200 px-3 py-1.5 text-xs font-semibold text-slate-700"
                        >
                          Edit
                        </button>
                        <button
                          onClick={() => setDeleteTarget(user)}
                          className="rounded-full bg-rose-500 px-3 py-1.5 text-xs font-semibold text-white"
                        >
                          Delete
                        </button>
                      </div>
                    </td>
                    <td className="px-4 py-3">
                      <button
                        onClick={() => void openAccess(user)}
                        className="rounded-full bg-tide px-3 py-1.5 text-xs font-semibold text-white"
                      >
                        QR / Link
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      </SectionCard>

      <ConfirmDialog
        open={userDialogOpen}
        title={editingUserId ? "Update User" : "Add User"}
        description="Create a new user identity or update an existing one without leaving the users table."
        hideActions
        onCancel={resetForm}
        onConfirm={() => undefined}
      >
        <form className="space-y-4" onSubmit={(event) => void submitUser(event)}>
          <label className="block">
            <span className="mb-2 block text-sm font-medium text-slate-600">Email</span>
            <input
              value={form.email}
              onChange={(event) => setForm((current) => ({ ...current, email: event.target.value }))}
              placeholder="user@example.com"
              className="w-full rounded-2xl border border-slate-200 px-4 py-3 text-sm outline-none ring-0 transition focus:border-tide"
            />
          </label>

          <label className="block">
            <span className="mb-2 block text-sm font-medium text-slate-600">Bandwidth Limit (GB)</span>
            <input
              type="number"
              min={0}
              value={form.bandwidthLimitGb}
              onChange={(event) => setForm((current) => ({ ...current, bandwidthLimitGb: Number(event.target.value) || 0 }))}
              className="w-full rounded-2xl border border-slate-200 px-4 py-3 text-sm outline-none ring-0 transition focus:border-tide"
            />
          </label>

          <label className="block">
            <span className="mb-2 block text-sm font-medium text-slate-600">Notes</span>
            <textarea
              value={form.notes}
              onChange={(event) => setForm((current) => ({ ...current, notes: event.target.value }))}
              rows={4}
              placeholder="Optional note for this user"
              className="w-full rounded-2xl border border-slate-200 px-4 py-3 text-sm outline-none ring-0 transition focus:border-tide"
            />
          </label>

          <label className="flex items-center gap-3 rounded-2xl border border-slate-200 px-4 py-3 text-sm text-slate-700">
            <input
              type="checkbox"
              checked={form.enabled}
              onChange={(event) => setForm((current) => ({ ...current, enabled: event.target.checked }))}
              className="h-4 w-4 rounded border-slate-300"
            />
            Enabled
          </label>

          <div className="flex justify-end gap-3">
            <button
              type="button"
              onClick={resetForm}
              className="rounded-2xl border border-slate-200 px-4 py-2.5 text-sm font-semibold text-slate-700 transition hover:bg-slate-50"
            >
              Cancel
            </button>
            <button type="submit" className="rounded-2xl bg-ink px-4 py-2.5 text-sm font-semibold text-white transition hover:bg-slate-800">
              {editingUserId ? "Save Changes" : "Create User"}
            </button>
          </div>
        </form>
      </ConfirmDialog>

      <ConfirmDialog
        open={Boolean(deleteTarget)}
        title="Delete user?"
        description={deleteTarget ? `This will remove ${deleteTarget.email} from the panel and sync the change to nodes.` : ""}
        confirmLabel="Delete User"
        tone="danger"
        onCancel={() => setDeleteTarget(null)}
        onConfirm={() => void deleteUser()}
      />
      <ConfirmDialog
        open={accessDialogOpen}
        title="User Access"
        description="Use the JSON profile URL directly in sing-box mobile, or scan the QR code to import via the client URL scheme."
        hideActions
        panelClassName="max-w-5xl"
        onCancel={closeAccessDialog}
        onConfirm={() => undefined}
      >
        <div className="max-h-[calc(90vh-8rem)] overflow-y-auto pr-1">
          {accessLoading ? (
            <div className="space-y-5">
              <div className="rounded-3xl bg-slate-50 p-5">
                <div className="h-4 w-24 animate-pulse rounded-full bg-slate-200" />
                <div className="mt-4 h-6 w-2/3 animate-pulse rounded-full bg-slate-200" />
                <div className="mt-6 h-4 w-20 animate-pulse rounded-full bg-slate-200" />
                <div className="mt-3 h-4 w-full animate-pulse rounded-full bg-slate-200" />
                <div className="mt-2 h-4 w-4/5 animate-pulse rounded-full bg-slate-200" />
              </div>
              <div className="grid gap-5 xl:grid-cols-[320px,minmax(0,1fr)]">
                <div className="rounded-3xl bg-slate-50 p-6">
                  <div className="aspect-square animate-pulse rounded-[2rem] bg-slate-200" />
                </div>
                <div className="space-y-4">
                  {[0, 1, 2].map((index) => (
                    <div key={index} className="rounded-3xl border border-slate-100 p-5">
                      <div className="h-4 w-32 animate-pulse rounded-full bg-slate-200" />
                      <div className="mt-4 h-20 animate-pulse rounded-2xl bg-slate-100" />
                    </div>
                  ))}
                </div>
              </div>
            </div>
          ) : accessError ? (
            <div className="space-y-4">
              <div className="rounded-3xl border border-rose-200 bg-rose-50 p-5 text-sm text-rose-700">{accessError}</div>
              <div className="flex flex-wrap justify-end gap-3">
                <button
                  type="button"
                  onClick={() => selectedUser && void openAccess(selectedUser)}
                  className="rounded-2xl bg-ink px-4 py-2.5 text-sm font-semibold text-white transition hover:bg-slate-800"
                >
                  Retry
                </button>
                <button
                  type="button"
                  onClick={closeAccessDialog}
                  className="rounded-2xl border border-slate-200 px-4 py-2.5 text-sm font-semibold text-slate-700 transition hover:bg-slate-50"
                >
                  Close
                </button>
              </div>
            </div>
          ) : selectedAccess ? (
            <div className="space-y-5">
            <div className="rounded-2xl bg-slate-50 p-4">
              <p className="text-xs font-semibold uppercase tracking-[0.2em] text-slate-500">User Email</p>
              <p className="mt-2 text-sm font-medium text-slate-900">{selectedUser?.email ?? "Unknown user"}</p>
              <p className="mt-4 text-xs font-semibold uppercase tracking-[0.2em] text-slate-500">UUID</p>
              <p className="mt-2 break-all font-mono text-xs text-slate-700">{selectedUser?.uuid ?? selectedAccess.uuid}</p>
              <p className="mt-4 text-xs font-semibold uppercase tracking-[0.2em] text-slate-500">Bandwidth Usage</p>
              <div className="mt-3">
                <BandwidthUsage
                  usedBytes={selectedUser?.bandwidthUsedBytes ?? 0}
                  limitGb={selectedUser?.bandwidthLimitGb ?? 0}
                  showDetails={true}
                />
              </div>
            </div>

            <div className="grid gap-5 xl:grid-cols-[320px,minmax(0,1fr)]">
              <div className="space-y-4 rounded-3xl bg-slate-50 p-6">
                <div className="flex items-start justify-center">
                {qrCodeUrl ? (
                  <img
                    src={qrCodeUrl}
                    alt="Sing-box import QR code"
                    className="h-auto w-full max-w-[260px] rounded-2xl bg-white p-4 shadow-sm"
                  />
                ) : (
                    <div className="flex h-[260px] w-full max-w-[260px] items-center justify-center rounded-2xl bg-slate-100">
                    <p className="text-center text-sm text-slate-400">QR code not available</p>
                  </div>
                )}
                </div>
                <div className="rounded-2xl border border-slate-200 bg-white p-4">
                  <p className="text-xs font-semibold uppercase tracking-[0.2em] text-slate-500">Quick Actions</p>
                  <div className="mt-3 flex flex-wrap gap-2">
                    <button
                      type="button"
                      onClick={() => void copyText(selectedAccess.singboxImportUrl || selectedAccess.remoteProfileUrl || selectedAccess.url, "best-link")}
                      className="rounded-full bg-ink px-3 py-1.5 text-xs font-semibold text-white"
                    >
                      {copiedKey === "best-link" ? "Copied" : "Copy Best Link"}
                    </button>
                    {selectedAccess.singboxImportUrl ? (
                      <a
                        href={selectedAccess.singboxImportUrl}
                        className="rounded-full bg-tide px-3 py-1.5 text-xs font-semibold text-white"
                      >
                        Open in sing-box
                      </a>
                    ) : null}
                  </div>
                </div>
              </div>

              <div className="min-w-0 space-y-4">
                {accessCards.map((card) => {
                  const hasValue = Boolean(card.value);

                  return (
                    <div key={card.key} className="rounded-3xl border border-slate-100 p-5">
                      <div className="flex flex-wrap items-start justify-between gap-3">
                        <div>
                          <p className="text-xs font-semibold uppercase tracking-[0.2em] text-slate-500">{card.label}</p>
                        </div>
                        <div className="flex flex-wrap gap-2">
                          <button
                            type="button"
                            disabled={!hasValue}
                            onClick={() => void copyText(card.value, card.key)}
                            className="rounded-full bg-ink px-3 py-1.5 text-xs font-semibold text-white disabled:cursor-not-allowed disabled:bg-slate-300"
                          >
                            {copiedKey === card.key ? "Copied" : card.copyLabel}
                          </button>
                          {card.openLabel && hasValue ? (
                            <a href={card.value} className="rounded-full bg-tide px-3 py-1.5 text-xs font-semibold text-white">
                              {card.openLabel}
                            </a>
                          ) : null}
                        </div>
                      </div>
                      <div className="mt-4 max-h-40 overflow-auto rounded-2xl bg-slate-50 p-3">
                        <p className="break-all font-mono text-xs leading-6 text-slate-700">{card.value || card.emptyMessage}</p>
                      </div>
                    </div>
                  );
                })}

                <div className="rounded-2xl border border-slate-100 p-4">
                  <p className="text-xs font-semibold uppercase tracking-[0.2em] text-slate-500">Node URLs</p>
                  <div className="mt-3 space-y-3">
                    {selectedAccess.nodeLinks?.length ? selectedAccess.nodeLinks.map((link) => (
                      <div key={`${link.nodeName}-${link.protocol}`} className="min-w-0 rounded-2xl bg-slate-50 p-3">
                        <p className="text-xs font-semibold uppercase tracking-[0.16em] text-slate-500">
                          {link.nodeName} / {link.protocol}
                        </p>
                        <p className="mt-2 break-all font-mono text-xs text-slate-700">{link.url}</p>
                        <button
                          onClick={() => void copyText(link.url, `${link.nodeName}-${link.protocol}`)}
                          className="mt-3 rounded-full bg-ink px-3 py-1.5 text-xs font-semibold text-white"
                        >
                          {copiedKey === `${link.nodeName}-${link.protocol}` ? "Copied" : `Copy ${link.protocol} URL`}
                        </button>
                      </div>
                    )) : (
                      <div className="rounded-2xl bg-slate-50 p-4 text-sm text-slate-500">
                        No per-node URLs were returned for this user yet.
                      </div>
                    )}
                  </div>
                </div>
              </div>
            </div>

            <div className="flex justify-end">
              <button
                type="button"
                onClick={closeAccessDialog}
                className="rounded-2xl border border-slate-200 px-4 py-2.5 text-sm font-semibold text-slate-700 transition hover:bg-slate-50"
              >
                Close
              </button>
            </div>
          </div>
          ) : null}
        </div>
      </ConfirmDialog>
    </div>
  );
}
