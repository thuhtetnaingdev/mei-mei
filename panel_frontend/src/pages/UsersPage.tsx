import { Calendar, Copy, Link2, Minus, Pencil, Plus, QrCode, ShieldCheck, Trash2, UserRoundX, Users } from "lucide-react";
import { FormEvent, useEffect, useState } from "react";
import axios from "axios";
import QRCode from "qrcode";
import api from "../api/client";
import { BandwidthUsage } from "../components/BandwidthUsage";
import { ConfirmDialog } from "../components/ConfirmDialog";
import { SectionCard } from "../components/SectionCard";
import type { MintPoolSnapshot, Node, User, UserBandwidthAllocation } from "../types";

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

type UserAccessNodeUsageRow = {
  key: string;
  nodeId?: number;
  nodeName: string;
  publicHost: string;
  protocols: string[];
  bandwidthBytes: number;
  percentage: number;
  hasCurrentAccess: boolean;
};

type NodeUsageRingProps = {
  percentage: number;
  emphasis: "active" | "history" | "idle";
};

type UserFormState = {
  email: string;
  enabled: boolean;
  notes: string;
  initialBandwidthGb: number;
  initialTokenAmount: number;
  initialExpiresAt: string;
};

const defaultFormState: UserFormState = {
  email: "",
  enabled: true,
  notes: "",
  initialBandwidthGb: 100,
  initialTokenAmount: 100,
  initialExpiresAt: ""
};

type AllocationFormState = {
  bandwidthGb: number;
  tokenAmount: number;
  expiresAt: string;
};

const defaultAllocationForm: AllocationFormState = {
  bandwidthGb: 50,
  tokenAmount: 50,
  expiresAt: ""
};

type ReductionFormState = {
  action: "increase" | "reduce";
  bandwidthGb: number;
  note: string;
};

const defaultReductionForm: ReductionFormState = {
  action: "reduce",
  bandwidthGb: 10,
  note: ""
};

type AllocationEditFormState = {
  expiresAt: string;
};

const defaultAllocationEditForm: AllocationEditFormState = {
  expiresAt: ""
};

const formatDate = (value?: string | null) => {
  if (!value) {
    return "No expiry";
  }

  return new Date(value).toLocaleString();
};

const formatBandwidthBytes = (bytes: number) => `${(bytes / (1024 ** 3)).toFixed(bytes >= 1024 ** 3 ? 1 : 2)} GB`;
const formatTokenAmount = (value: number) =>
  new Intl.NumberFormat(undefined, {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2
  }).format(value);
const formatPercentage = (value: number) => `${value >= 10 || Number.isInteger(value) ? value.toFixed(0) : value.toFixed(1)}%`;

const toDateTimeLocalValue = (value?: string | null) => {
  if (!value) {
    return "";
  }

  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return "";
  }

  const offset = date.getTimezoneOffset();
  const normalized = new Date(date.getTime() - offset * 60_000);
  return normalized.toISOString().slice(0, 16);
};

const bytesPerGB = 1024 ** 3;

type DerivedUserSummary = {
  expiresAt: string | null;
  bandwidthLimitGb: number;
  tokenBalance: number;
};

const deriveUserSummary = (user: User): DerivedUserSummary => {
  const allocations = user.bandwidthAllocations ?? [];
  if (!allocations.length) {
    return {
      expiresAt: user.expiresAt ?? null,
      bandwidthLimitGb: user.bandwidthLimitGb ?? 0,
      tokenBalance: user.tokenBalance ?? 0
    };
  }

  const now = Date.now();
  let totalRemainingBytes = 0;
  let totalRemainingTokens = 0;
  let latestExpiryAt: string | null = null;

  allocations.forEach((allocation) => {
    if ((allocation.remainingBandwidthBytes ?? 0) <= 0) {
      return;
    }

    if (allocation.expiresAt) {
      const expiryTime = new Date(allocation.expiresAt).getTime();
      if (!Number.isNaN(expiryTime) && expiryTime <= now) {
        return;
      }

      if (!latestExpiryAt || expiryTime > new Date(latestExpiryAt).getTime()) {
        latestExpiryAt = allocation.expiresAt;
      }
    }

    totalRemainingBytes += allocation.remainingBandwidthBytes ?? 0;
    totalRemainingTokens += allocation.remainingTokens ?? 0;
  });

  if (totalRemainingBytes <= 0 && totalRemainingTokens <= 0 && !latestExpiryAt) {
    return {
      expiresAt: user.expiresAt ?? null,
      bandwidthLimitGb: user.bandwidthLimitGb ?? 0,
      tokenBalance: user.tokenBalance ?? 0
    };
  }

  return {
    expiresAt: latestExpiryAt ?? user.expiresAt ?? null,
    bandwidthLimitGb: totalRemainingBytes > 0 ? Math.ceil(totalRemainingBytes / bytesPerGB) : user.bandwidthLimitGb ?? 0,
    tokenBalance: totalRemainingTokens > 0 ? totalRemainingTokens : user.tokenBalance ?? 0
  };
};

const extractApiError = (error: unknown, fallback: string) => {
  if (axios.isAxiosError(error)) {
    const backendMessage =
      typeof error.response?.data?.error === "string" ? error.response.data.error : "";
    return backendMessage || error.message || fallback;
  }

  return fallback;
};

const buildUserAccessNodeUsage = (
  user: User | null,
  access: SubscriptionResponse | null,
  nodes: Node[]
): { rows: UserAccessNodeUsageRow[]; totalUsageBytes: number } => {
  const nodeById = new Map(nodes.map((node) => [node.id, node]));
  const nodeByName = new Map(nodes.map((node) => [node.name, node]));
  const usageBytesByNodeId = new Map<number, number>();

  (user?.bandwidthAllocations ?? []).forEach((allocation) => {
    (allocation.nodeUsages ?? []).forEach((nodeUsage) => {
      usageBytesByNodeId.set(
        nodeUsage.nodeId,
        (usageBytesByNodeId.get(nodeUsage.nodeId) ?? 0) + (nodeUsage.bandwidthBytes ?? 0)
      );
    });
  });

  const totalUsageBytes = Array.from(usageBytesByNodeId.values()).reduce((sum, value) => sum + value, 0);
  const rows = new Map<string, UserAccessNodeUsageRow>();

  (access?.nodeLinks ?? []).forEach((link) => {
    const matchedNode = nodeByName.get(link.nodeName);
    const key = matchedNode ? `node:${matchedNode.id}` : `access:${link.nodeName}`;
    const existing = rows.get(key);

    if (existing) {
      const normalizedProtocol = link.protocol.toUpperCase();
      if (!existing.protocols.includes(normalizedProtocol)) {
        existing.protocols.push(normalizedProtocol);
      }
      return;
    }

    rows.set(key, {
      key,
      nodeId: matchedNode?.id,
      nodeName: matchedNode?.name ?? link.nodeName,
      publicHost: matchedNode?.publicHost ?? "",
      protocols: [link.protocol.toUpperCase()],
      bandwidthBytes: 0,
      percentage: 0,
      hasCurrentAccess: true
    });
  });

  usageBytesByNodeId.forEach((bandwidthBytes, nodeId) => {
    const key = `node:${nodeId}`;
    const matchedNode = nodeById.get(nodeId);
    const existing = rows.get(key);

    if (existing) {
      existing.bandwidthBytes = bandwidthBytes;
      if (!existing.publicHost && matchedNode?.publicHost) {
        existing.publicHost = matchedNode.publicHost;
      }
      if ((!existing.nodeName || existing.nodeName.startsWith("Node #")) && matchedNode?.name) {
        existing.nodeName = matchedNode.name;
      }
      return;
    }

    rows.set(key, {
      key,
      nodeId,
      nodeName: matchedNode?.name ?? `Node #${nodeId}`,
      publicHost: matchedNode?.publicHost ?? "",
      protocols: [],
      bandwidthBytes,
      percentage: 0,
      hasCurrentAccess: false
    });
  });

  const sortedRows = Array.from(rows.values())
    .map((row) => ({
      ...row,
      percentage: totalUsageBytes > 0 ? (row.bandwidthBytes / totalUsageBytes) * 100 : 0
    }))
    .sort((left, right) => {
      if (right.bandwidthBytes !== left.bandwidthBytes) {
        return right.bandwidthBytes - left.bandwidthBytes;
      }
      if (left.hasCurrentAccess !== right.hasCurrentAccess) {
        return Number(right.hasCurrentAccess) - Number(left.hasCurrentAccess);
      }
      return left.nodeName.localeCompare(right.nodeName);
    });

  return {
    rows: sortedRows,
    totalUsageBytes
  };
};

function AllocationSummary({ allocation }: { allocation: UserBandwidthAllocation }) {
  return (
    <div className="rounded-2xl border border-white/10 bg-slate-950/35 p-3">
      <p className="text-sm font-medium text-white">
        {formatBandwidthBytes(allocation.remainingBandwidthBytes)} left of {formatBandwidthBytes(allocation.totalBandwidthBytes)}
      </p>
      <p className="mt-1 text-xs text-slate-500">
        {allocation.remainingTokens.toFixed(2)} / {allocation.tokenAmount.toFixed(2)} tokens · {formatDate(allocation.expiresAt)}
      </p>
    </div>
  );
}

function NodeUsageRing({ percentage, emphasis }: NodeUsageRingProps) {
  const clampedPercentage = Math.max(0, Math.min(percentage, 100));
  const radius = 22;
  const circumference = 2 * Math.PI * radius;
  const dashOffset = circumference * (1 - clampedPercentage / 100);
  const strokeClass =
    emphasis === "history" ? "stroke-amber-300" : emphasis === "idle" ? "stroke-slate-500" : "stroke-sky-300";
  const textClass =
    emphasis === "history" ? "text-amber-200" : emphasis === "idle" ? "text-slate-300" : "text-sky-300";

  return (
    <div className="relative h-16 w-16 shrink-0">
      <svg viewBox="0 0 56 56" className="h-16 w-16 -rotate-90">
        <circle cx="28" cy="28" r={radius} className="fill-none stroke-white/10" strokeWidth="5" />
        <circle
          cx="28"
          cy="28"
          r={radius}
          className={`fill-none ${strokeClass}`}
          strokeWidth="5"
          strokeLinecap="round"
          strokeDasharray={circumference}
          strokeDashoffset={dashOffset}
        />
      </svg>
      <div className="absolute inset-0 flex items-center justify-center">
        <span className={`text-xs font-semibold ${textClass}`}>{Math.round(clampedPercentage)}%</span>
      </div>
    </div>
  );
}

export function UsersPage() {
  const [users, setUsers] = useState<User[]>([]);
  const [nodes, setNodes] = useState<Node[]>([]);
  const [form, setForm] = useState<UserFormState>(defaultFormState);
  const [allocationForm, setAllocationForm] = useState<AllocationFormState>(defaultAllocationForm);
  const [editingUserId, setEditingUserId] = useState<number | null>(null);
  const [userDialogOpen, setUserDialogOpen] = useState(false);
  const [reductionDialogOpen, setReductionDialogOpen] = useState(false);
  const [reductionTarget, setReductionTarget] = useState<User | null>(null);
  const [reductionAllocationTarget, setReductionAllocationTarget] = useState<UserBandwidthAllocation | null>(null);
  const [reductionForm, setReductionForm] = useState<ReductionFormState>(defaultReductionForm);
  const [allocationEditDialogOpen, setAllocationEditDialogOpen] = useState(false);
  const [allocationEditTargetUser, setAllocationEditTargetUser] = useState<User | null>(null);
  const [allocationEditTarget, setAllocationEditTarget] = useState<UserBandwidthAllocation | null>(null);
  const [allocationEditForm, setAllocationEditForm] = useState<AllocationEditFormState>(defaultAllocationEditForm);
  const [deleteTarget, setDeleteTarget] = useState<User | null>(null);
  const [accessDialogOpen, setAccessDialogOpen] = useState(false);
  const [accessLoading, setAccessLoading] = useState(false);
  const [accessError, setAccessError] = useState("");
  const [selectedAccess, setSelectedAccess] = useState<SubscriptionResponse | null>(null);
  const [selectedUser, setSelectedUser] = useState<User | null>(null);
  const [qrCodeUrl, setQrCodeURL] = useState("");
  const [copiedKey, setCopiedKey] = useState("");
  const [formStatus, setFormStatus] = useState("");
  const [formError, setFormError] = useState("");
  const [mainWalletBalance, setMainWalletBalance] = useState<number | null>(null);

  const loadUsers = () => api.get<User[]>("/users").then((res) => setUsers(res.data));
  const loadNodes = () => api.get<Node[]>("/nodes").then((res) => setNodes(res.data));
  const loadTreasury = () =>
    api.get<MintPoolSnapshot>("/mint-pool").then((res) => setMainWalletBalance(res.data.pool.mainWalletBalance));
  const syncNodes = () => api.post("/nodes/sync").catch(() => undefined);

  useEffect(() => {
    void Promise.all([loadUsers(), loadTreasury(), loadNodes().catch(() => undefined)]).catch(() => undefined);
  }, []);

  const closeUserDialog = () => {
    setForm(defaultFormState);
    setAllocationForm(defaultAllocationForm);
    setEditingUserId(null);
    setFormError("");
    setUserDialogOpen(false);
  };

  const closeReductionDialog = () => {
    setReductionDialogOpen(false);
    setReductionTarget(null);
    setReductionAllocationTarget(null);
    setReductionForm(defaultReductionForm);
    setFormError("");
  };

  const closeAllocationEditDialog = () => {
    setAllocationEditDialogOpen(false);
    setAllocationEditTargetUser(null);
    setAllocationEditTarget(null);
    setAllocationEditForm(defaultAllocationEditForm);
    setFormError("");
  };

  const submitUser = async (event: FormEvent) => {
    event.preventDefault();
    setFormError("");

    try {
      if (editingUserId) {
        await api.patch(`/users/${editingUserId}`, {
          email: form.email,
          enabled: form.enabled,
          notes: form.notes
        });
        setFormStatus("User updated.");
      } else {
        if (form.initialBandwidthGb > 0 && !form.initialExpiresAt) {
          setFormError("Initial expiry is required when assigning bandwidth.");
          return;
        }

        await api.post("/users", {
          email: form.email,
          enabled: form.enabled,
          notes: form.notes,
          bandwidthAllocations: form.initialBandwidthGb > 0 ? [
            {
              bandwidthGb: form.initialBandwidthGb,
              tokenAmount: form.initialTokenAmount,
              expiresAt: form.initialExpiresAt ? new Date(form.initialExpiresAt).toISOString() : null
            }
          ] : []
        });
        setFormStatus("User created.");
      }

      await syncNodes();
      await Promise.all([loadUsers(), loadTreasury()]);
      closeUserDialog();
    } catch (error) {
      setFormError(extractApiError(error, "We could not save this user right now."));
    }
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
      notes: user.notes ?? "",
      initialBandwidthGb: 0,
      initialTokenAmount: 0,
      initialExpiresAt: ""
    });
    setAllocationForm(defaultAllocationForm);
    setFormError("");
    setUserDialogOpen(true);
  };

  const openCreateDialog = () => {
    setEditingUserId(null);
    setForm(defaultFormState);
    setFormError("");
    setUserDialogOpen(true);
  };

  const openAdjustBandwidthEntry = (user: User, allocation: UserBandwidthAllocation, action: "increase" | "reduce") => {
    setReductionTarget(user);
    setReductionAllocationTarget(allocation);
    setReductionForm({
      action,
      bandwidthGb: 10,
      note: ""
    });
    setFormError("");
    setReductionDialogOpen(true);
  };

  const openEditBandwidthEntry = (user: User, allocation: UserBandwidthAllocation) => {
    setAllocationEditTargetUser(user);
    setAllocationEditTarget(allocation);
    setAllocationEditForm({
      expiresAt: toDateTimeLocalValue(allocation.expiresAt)
    });
    setFormError("");
    setAllocationEditDialogOpen(true);
  };

  const submitAllocation = async (event: FormEvent) => {
    event.preventDefault();
    if (!editingUserId) {
      return;
    }

    setFormError("");

    if (!allocationForm.expiresAt) {
      setFormError("Expiry is required when adding bandwidth.");
      return;
    }

    try {
      await api.post(`/users/${editingUserId}/bandwidth-allocations`, {
        bandwidthGb: allocationForm.bandwidthGb,
        tokenAmount: allocationForm.tokenAmount,
        expiresAt: allocationForm.expiresAt ? new Date(allocationForm.expiresAt).toISOString() : null
      });

      const targetUser = users.find((user) => user.id === editingUserId);
      setFormStatus(`Added bandwidth to ${targetUser?.email ?? "user"}.`);
      await syncNodes();
      await Promise.all([loadUsers(), loadTreasury()]);
      setAllocationForm(defaultAllocationForm);
    } catch (error) {
      setFormError(extractApiError(error, "We could not add bandwidth right now."));
    }
  };

  const submitReduction = async (event: FormEvent) => {
    event.preventDefault();
    if (!reductionTarget || !reductionAllocationTarget) {
      return;
    }

    setFormError("");

    try {
      const response = await api.post<User>(`/users/${reductionTarget.id}/bandwidth-allocations/${reductionAllocationTarget.id}/adjust`, {
        action: reductionForm.action,
        bandwidthGb: reductionForm.bandwidthGb,
        note: reductionForm.note
      });

      const updatedUser = response.data;
      setUsers((current) => current.map((user) => (user.id === updatedUser.id ? updatedUser : user)));
      setFormStatus(
        `${reductionForm.action === "increase" ? "Increased" : "Reduced"} bandwidth entry for ${reductionTarget.email}.`
      );
      await syncNodes();
      await loadTreasury();
      closeReductionDialog();
    } catch (error) {
      setFormError(extractApiError(error, "We could not adjust this bandwidth entry right now."));
    }
  };

  const submitAllocationEdit = async (event: FormEvent) => {
    event.preventDefault();
    if (!allocationEditTargetUser || !allocationEditTarget) {
      return;
    }

    setFormError("");
    if (!allocationEditForm.expiresAt) {
      setFormError("Expiry is required.");
      return;
    }

    try {
      const response = await api.patch<User>(
        `/users/${allocationEditTargetUser.id}/bandwidth-allocations/${allocationEditTarget.id}`,
        {
          expiresAt: new Date(allocationEditForm.expiresAt).toISOString()
        }
      );

      const updatedUser = response.data;
      setUsers((current) => current.map((user) => (user.id === updatedUser.id ? updatedUser : user)));
      setFormStatus(`Updated expiry for bandwidth entry on ${allocationEditTargetUser.email}.`);
      await syncNodes();
      closeAllocationEditDialog();
    } catch (error) {
      setFormError(extractApiError(error, "We could not update this entry right now."));
    }
  };

  const deleteUser = async () => {
    if (!deleteTarget) {
      return;
    }

    await api.delete(`/users/${deleteTarget.id}`);
    await syncNodes();
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

  const activeUsers = users.filter((user) => user.enabled).length;
  const disabledUsers = users.length - activeUsers;
  const totalBandwidth = users.reduce((sum, user) => sum + user.bandwidthUsedBytes, 0);
  const totalTokens = users.reduce((sum, user) => sum + (user.tokenBalance ?? 0), 0);
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
  const selectedUserSummary = selectedUser ? deriveUserSummary(selectedUser) : null;
  const selectedNodeUsage = buildUserAccessNodeUsage(selectedUser, selectedAccess, nodes);
  const editingUser = editingUserId ? users.find((user) => user.id === editingUserId) ?? null : null;
  const bandwidthHistoryEntries = [...(editingUser?.bandwidthAllocations ?? [])].sort(
    (left, right) => new Date(right.createdAt).getTime() - new Date(left.createdAt).getTime()
  );
  const activeBandwidthEntries = bandwidthHistoryEntries.filter((allocation) => {
    if ((allocation.remainingBandwidthBytes ?? 0) <= 0) {
      return false;
    }
    if (!allocation.expiresAt) {
      return true;
    }
    return new Date(allocation.expiresAt).getTime() > Date.now();
  });
  const bandwidthHistoryRemainingBytes = bandwidthHistoryEntries.reduce(
    (sum, allocation) => sum + (allocation.remainingBandwidthBytes ?? 0),
    0
  );
  const bandwidthHistoryRemainingTokens = bandwidthHistoryEntries.reduce(
    (sum, allocation) => sum + (allocation.remainingTokens ?? 0),
    0
  );
  const bandwidthHistoryLatestExpiry = bandwidthHistoryEntries.reduce<string | null>((latest, allocation) => {
    if (!allocation.expiresAt) {
      return latest;
    }
    if (!latest) {
      return allocation.expiresAt;
    }
    return new Date(allocation.expiresAt).getTime() > new Date(latest).getTime() ? allocation.expiresAt : latest;
  }, null);

  return (
    <div className="space-y-4">
      <section className="grid gap-3 xl:grid-cols-[minmax(0,1.1fr),minmax(320px,0.9fr)]">
        <SectionCard
          eyebrow="Identity Control"
          title="Users and access delivery"
          description="Manage the full customer access flow from one compact workspace, then open QR and import links without leaving the panel."
          className="!p-4 sm:!p-5"
          action={
            <button type="button" onClick={openCreateDialog} className="btn-primary gap-2">
              <Plus className="h-4 w-4" />
              Add
            </button>
          }
        >
          <div className="grid gap-2.5 sm:grid-cols-3">
            <div className="panel-subtle p-3">
              <p className="metric-kicker">Total Users</p>
              <p className="mt-2 font-display text-2xl font-bold text-white">{users.length}</p>
            </div>
            <div className="panel-subtle p-3">
              <p className="metric-kicker">Enabled</p>
              <p className="mt-2 font-display text-2xl font-bold text-emerald-300">{activeUsers}</p>
            </div>
            <div className="panel-subtle p-3">
              <p className="metric-kicker">Disabled</p>
              <p className="mt-2 font-display text-2xl font-bold text-slate-300">{disabledUsers}</p>
            </div>
          </div>
        </SectionCard>

        <div className="panel-surface p-4 sm:p-5">
          <div className="flex items-center justify-between">
            <div>
              <p className="metric-kicker">Traffic Snapshot</p>
              <h3 className="mt-1.5 font-display text-xl font-semibold text-white">Usage posture</h3>
            </div>
            <div className="rounded-2xl border border-white/10 bg-sky-400/10 p-2.5 text-sky-300">
              <Users className="h-4.5 w-4.5" />
            </div>
          </div>
          <div className="mt-4 grid gap-2.5 sm:grid-cols-2">
            <div className="panel-subtle p-3">
              <p className="text-sm text-slate-300">Total traffic consumed</p>
              <p className="mt-1.5 font-display text-2xl font-bold text-white">{(totalBandwidth / (1024 ** 3)).toFixed(totalBandwidth >= 1024 ** 3 ? 1 : 2)} GB</p>
            </div>
            <div className="panel-subtle p-3">
              <p className="text-sm text-slate-300">Remaining user tokens</p>
              <p className="mt-1.5 font-display text-2xl font-bold text-emerald-300">{totalTokens.toFixed(2)}</p>
            </div>
            <div className="panel-subtle flex items-center justify-between p-3">
              <div className="flex items-center gap-3">
                <div className="rounded-xl bg-emerald-500/10 p-2 text-emerald-300">
                  <ShieldCheck className="h-4 w-4" />
                </div>
                <p className="text-sm text-slate-300">Sync-ready users</p>
              </div>
              <p className="text-lg font-semibold text-white">{activeUsers}</p>
            </div>
            <div className="panel-subtle flex items-center justify-between p-3">
              <div className="flex items-center gap-3">
                <div className="rounded-xl bg-white/5 p-2 text-slate-300">
                  <UserRoundX className="h-4 w-4" />
                </div>
                <p className="text-sm text-slate-300">Disabled accounts</p>
              </div>
              <p className="text-lg font-semibold text-white">{disabledUsers}</p>
            </div>
          </div>
        </div>
      </section>

      <SectionCard
        eyebrow="User Directory"
        title="Manage users"
        description="The desktop table stays dense for operations speed, while mobile automatically switches to compact stacked cards."
      >
        <div className="space-y-4">
          {formStatus ? <p className="text-sm text-slate-400">{formStatus}</p> : null}
          {formError ? <p className="text-sm text-rose-300">{formError}</p> : null}
          {mainWalletBalance !== null ? (
            <div className={`rounded-2xl border px-4 py-3 text-sm ${mainWalletBalance < 0 ? "border-rose-400/20 bg-rose-400/10 text-rose-200" : "border-sky-400/20 bg-sky-400/10 text-sky-200"}`}>
              Main wallet available: {formatTokenAmount(mainWalletBalance)} Mei
            </div>
          ) : null}

          <div className="hidden overflow-hidden rounded-[24px] border border-white/10 lg:block">
            <div className="overflow-x-auto">
              <table className="data-table">
                <thead>
                  <tr>
                    <th>Email</th>
                    <th>UUID</th>
                    <th>Status</th>
                    <th>Bandwidth</th>
                    <th>Tokens</th>
                    <th>Expiry</th>
                    <th>Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {users.map((user) => {
                    const summary = deriveUserSummary(user);

                    return (
                      <tr key={user.id} className="bg-transparent">
                        <td>
                          <div className="min-w-[180px]">
                            <p className="font-semibold text-white">{user.email}</p>
                            <p className="mt-1 text-xs text-slate-500">{user.notes || "No notes"}</p>
                          </div>
                        </td>
                        <td className="font-mono text-xs text-slate-400">{user.uuid}</td>
                        <td>
                          <span className={`status-pill ${user.enabled ? "text-emerald-300" : "text-slate-400"}`}>
                            <span className={`h-2 w-2 rounded-full ${user.enabled ? "bg-emerald-400" : "bg-slate-500"}`} />
                            {user.enabled ? "enabled" : "disabled"}
                          </span>
                        </td>
                        <td className="min-w-[220px]">
                          <BandwidthUsage usedBytes={user.bandwidthUsedBytes} limitGb={summary.bandwidthLimitGb} showDetails={false} />
                          <p className="mt-2 text-xs text-slate-500">{(user.bandwidthAllocations ?? []).length} bandwidth entries</p>
                        </td>
                        <td className="text-sm font-semibold text-emerald-300">{summary.tokenBalance.toFixed(2)}</td>
                        <td className="text-sm text-slate-400">{formatDate(summary.expiresAt)}</td>
                        <td className="min-w-[150px]">
                          <div className="grid grid-cols-3 gap-1.5">
                            <button
                              onClick={() => void openAccess(user)}
                              className="btn-primary justify-center px-0 py-2 text-xs"
                              title="QR"
                              aria-label={`Open QR for ${user.email}`}
                            >
                              <QrCode className="h-3.5 w-3.5" />
                            </button>
                            <button
                              onClick={() => startEdit(user)}
                              className="btn-secondary justify-center px-0 py-2 text-xs"
                              title="Edit"
                              aria-label={`Edit ${user.email}`}
                            >
                              <Pencil className="h-3.5 w-3.5" />
                            </button>
                            <button
                              onClick={() => setDeleteTarget(user)}
                              className="btn-danger justify-center px-0 py-2 text-xs"
                              title="Delete"
                              aria-label={`Delete ${user.email}`}
                            >
                              <Trash2 className="h-3.5 w-3.5" />
                            </button>
                          </div>
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          </div>

          <div className="grid gap-3 lg:hidden">
            {users.map((user) => {
              const summary = deriveUserSummary(user);

              return (
                <article key={user.id} className="panel-subtle p-4">
                  <div className="flex flex-col gap-3">
                    <div className="flex items-start justify-between gap-3">
                      <div className="min-w-0">
                        <p className="truncate text-sm font-semibold text-white">{user.email}</p>
                        <p className="mt-1 truncate font-mono text-xs text-slate-500">{user.uuid}</p>
                      </div>
                      <span className={`status-pill shrink-0 ${user.enabled ? "text-emerald-300" : "text-slate-400"}`}>
                        <span className={`h-2 w-2 rounded-full ${user.enabled ? "bg-emerald-400" : "bg-slate-500"}`} />
                        {user.enabled ? "enabled" : "disabled"}
                      </span>
                    </div>
                    <BandwidthUsage usedBytes={user.bandwidthUsedBytes} limitGb={summary.bandwidthLimitGb} showDetails />
                    <div className="grid gap-2 text-xs text-slate-400 sm:grid-cols-2">
                      <div className="panel-subtle p-3">
                        <p className="metric-kicker">Notes</p>
                        <p className="mt-2 text-sm text-slate-300">{user.notes || "No notes"}</p>
                      </div>
                      <div className="panel-subtle p-3">
                        <p className="metric-kicker">Expiry</p>
                        <p className="mt-2 text-sm text-slate-300">{formatDate(summary.expiresAt)}</p>
                      </div>
                      <div className="panel-subtle p-3">
                        <p className="metric-kicker">Token Balance</p>
                        <p className="mt-2 text-sm text-emerald-300">{summary.tokenBalance.toFixed(2)}</p>
                      </div>
                      <div className="panel-subtle p-3">
                        <p className="metric-kicker">Bandwidth Entries</p>
                        <p className="mt-2 text-sm text-slate-300">{(user.bandwidthAllocations ?? []).length}</p>
                      </div>
                    </div>
                    <div className="panel-subtle p-3">
                      <p className="metric-kicker">Bandwidth List</p>
                      <div className="mt-2 space-y-2">
                        {(user.bandwidthAllocations ?? []).length ? (user.bandwidthAllocations ?? []).map((allocation) => (
                          <AllocationSummary key={allocation.id} allocation={allocation} />
                        )) : (
                          <p className="text-sm text-slate-500">No bandwidth entries yet.</p>
                        )}
                      </div>
                    </div>
                    <div className="flex flex-wrap gap-2">
                      <button onClick={() => startEdit(user)} className="btn-secondary flex-1">
                        Edit
                      </button>
                      <button onClick={() => void openAccess(user)} className="btn-primary flex-1 gap-2">
                        <QrCode className="h-4 w-4" />
                        QR
                      </button>
                      <button onClick={() => setDeleteTarget(user)} className="btn-danger flex-1">
                        Delete
                      </button>
                    </div>
                  </div>
                </article>
              );
            })}
          </div>
        </div>
      </SectionCard>

      <ConfirmDialog
        open={userDialogOpen}
        title={editingUserId ? "Edit User" : "Add User"}
        description={
          editingUserId
            ? "Update the user profile, add a new bandwidth package, and review bandwidth history from one modal."
            : "Create a new identity without leaving the users workspace."
        }
        hideActions
        panelClassName={editingUserId ? "max-w-4xl" : "max-w-2xl"}
        onCancel={closeUserDialog}
        onConfirm={() => undefined}
      >
        <div className="max-h-[calc(85vh-8rem)] overflow-y-auto pr-1">
          {formError ? (
            <div className="rounded-2xl border border-rose-400/20 bg-rose-400/10 px-4 py-3 text-sm text-rose-200">
              {formError}
            </div>
          ) : null}

          {editingUserId ? (
            <div className="mt-4 space-y-4">
              <form className="panel-subtle space-y-4 p-5" onSubmit={(event) => void submitUser(event)}>
                <div>
                  <p className="metric-kicker">Update User</p>
                  <p className="mt-2 text-sm text-slate-400">Edit identity details and sync state for this account.</p>
                </div>

                <label className="block">
                  <span className="mb-2 block text-sm font-medium text-slate-300">Email</span>
                  <input
                    value={form.email}
                    onChange={(event) => setForm((current) => ({ ...current, email: event.target.value }))}
                    placeholder="user@example.com"
                    className="input-shell"
                  />
                </label>

                <label className="block">
                  <span className="mb-2 block text-sm font-medium text-slate-300">Notes</span>
                  <textarea
                    value={form.notes}
                    onChange={(event) => setForm((current) => ({ ...current, notes: event.target.value }))}
                    rows={4}
                    placeholder="Optional note for this user"
                    className="input-shell resize-none"
                  />
                </label>

                <label className="flex items-center gap-3 rounded-2xl border border-white/10 bg-white/[0.03] px-4 py-3 text-sm text-slate-200">
                  <input
                    type="checkbox"
                    checked={form.enabled}
                    onChange={(event) => setForm((current) => ({ ...current, enabled: event.target.checked }))}
                    className="h-4 w-4 rounded border-white/20 bg-transparent"
                  />
                  Enabled for node sync
                </label>

                <div className="flex flex-wrap justify-end gap-3">
                  <button type="button" onClick={closeUserDialog} className="btn-secondary">
                    Close
                  </button>
                  <button type="submit" className="btn-primary">
                    Save Changes
                  </button>
                </div>
              </form>

              <form className="panel-subtle space-y-4 p-5" onSubmit={(event) => void submitAllocation(event)}>
                <div>
                  <p className="metric-kicker">Add Bandwidth</p>
                  <p className="mt-2 text-sm text-slate-400">Create a new bandwidth package without leaving this edit modal.</p>
                </div>

                {mainWalletBalance !== null ? (
                  <div className={`rounded-2xl border px-4 py-3 text-sm ${mainWalletBalance < 0 ? "border-rose-400/20 bg-rose-400/10 text-rose-200" : "border-sky-400/20 bg-sky-400/10 text-sky-200"}`}>
                    Main wallet available: {formatTokenAmount(mainWalletBalance)} Mei
                  </div>
                ) : null}

                <div className="grid gap-4 md:grid-cols-3">
                  <label className="block">
                    <span className="mb-2 block text-sm font-medium text-slate-300">Bandwidth (GB)</span>
                    <input
                      type="number"
                      min={1}
                      value={allocationForm.bandwidthGb}
                      onChange={(event) => setAllocationForm((current) => ({ ...current, bandwidthGb: Number(event.target.value) || 0 }))}
                      className="input-shell"
                    />
                  </label>

                  <label className="block">
                    <span className="mb-2 block text-sm font-medium text-slate-300">Tokens</span>
                    <input
                      type="number"
                      min={0}
                      step="0.01"
                      value={allocationForm.tokenAmount}
                      onChange={(event) => setAllocationForm((current) => ({ ...current, tokenAmount: Number(event.target.value) || 0 }))}
                      className="input-shell"
                    />
                  </label>

                  <label className="block">
                    <span className="mb-2 block text-sm font-medium text-slate-300">Expiry</span>
                    <input
                      type="datetime-local"
                      required
                      value={allocationForm.expiresAt}
                      onChange={(event) => setAllocationForm((current) => ({ ...current, expiresAt: event.target.value }))}
                      className="input-shell"
                    />
                  </label>
                </div>

                <div className="flex flex-wrap justify-end gap-3">
                  <button type="submit" className="btn-primary">
                    Add Bandwidth
                  </button>
                </div>
              </form>

              <div className="panel-subtle p-5">
                <div className="flex flex-wrap items-start justify-between gap-3">
                  <div>
                    <p className="metric-kicker">Bandwidth History</p>
                    <p className="mt-2 text-sm text-slate-400">
                      {editingUser ? `Manage each bandwidth entry for ${editingUser.email}.` : "Manage each bandwidth entry."}
                    </p>
                  </div>
                  {bandwidthHistoryEntries.length ? (
                    <div className="flex flex-wrap gap-2 text-sm">
                      <div className="rounded-full border border-white/10 bg-white/[0.04] px-3 py-2 text-slate-300">
                        <span className="text-slate-500">Entries</span> <span className="font-semibold text-white">{bandwidthHistoryEntries.length}</span>
                      </div>
                      <div className="rounded-full border border-white/10 bg-white/[0.04] px-3 py-2 text-slate-300">
                        <span className="text-slate-500">Active</span> <span className="font-semibold text-emerald-300">{activeBandwidthEntries.length}</span>
                      </div>
                    </div>
                  ) : null}
                </div>

                {bandwidthHistoryEntries.length ? (
                  <div className="mt-4 space-y-3">
                    <div className="flex flex-wrap gap-2 text-sm">
                      <div className="rounded-full border border-white/10 bg-white/[0.04] px-3 py-2 text-slate-300">
                        <span className="text-slate-500">Remaining</span> <span className="font-semibold text-white">{formatBandwidthBytes(bandwidthHistoryRemainingBytes)}</span>
                      </div>
                      <div className="rounded-full border border-white/10 bg-white/[0.04] px-3 py-2 text-slate-300">
                        <span className="text-slate-500">Tokens</span> <span className="font-semibold text-white">{formatTokenAmount(bandwidthHistoryRemainingTokens)}</span>
                      </div>
                      <div className="rounded-full border border-white/10 bg-white/[0.04] px-3 py-2 text-slate-300">
                        <span className="text-slate-500">Next expiry</span> <span className="font-semibold text-white">{formatDate(bandwidthHistoryLatestExpiry)}</span>
                      </div>
                    </div>

                    {bandwidthHistoryEntries.map((allocation) => {
                      const consumedBytes = Math.max(allocation.totalBandwidthBytes - allocation.remainingBandwidthBytes, 0);
                      const consumedTokens = Math.max(allocation.tokenAmount - allocation.remainingTokens, 0);
                      const hasRemainingBalance =
                        (allocation.remainingBandwidthBytes ?? 0) > 0 && (allocation.remainingTokens ?? 0) > 0;
                      const isExpired = allocation.expiresAt ? new Date(allocation.expiresAt).getTime() <= Date.now() : false;
                      const statusLabel = !hasRemainingBalance ? "Depleted" : isExpired ? "Expired" : "Active";
                      const statusClasses =
                        statusLabel === "Depleted"
                          ? "bg-slate-500/10 text-slate-200"
                          : statusLabel === "Expired"
                            ? "bg-rose-500/10 text-rose-200"
                            : "bg-emerald-500/10 text-emerald-200";

                      return (
                        <div key={allocation.id} className="rounded-[22px] border border-white/10 bg-white/[0.03] px-4 py-4">
                          <div className="flex flex-wrap items-center gap-2">
                            <p className="text-sm font-semibold text-white">Entry #{allocation.id}</p>
                            <span className={`rounded-full px-2.5 py-1 text-[11px] font-semibold uppercase tracking-[0.16em] ${statusClasses}`}>
                              {statusLabel}
                            </span>
                            <span className="text-xs text-slate-500">Added {formatDate(allocation.createdAt)}</span>
                          </div>

                          <div className="mt-4 grid gap-3 md:grid-cols-2">
                            <div className="rounded-xl bg-slate-950/35 px-3 py-3">
                              <p className="text-[11px] uppercase tracking-[0.16em] text-slate-500">Remaining</p>
                              <p className="mt-1 text-lg font-semibold text-white">{formatBandwidthBytes(allocation.remainingBandwidthBytes)}</p>
                              <p className="text-sm text-slate-500">{formatTokenAmount(allocation.remainingTokens)} tokens</p>
                            </div>
                            <div className="rounded-xl bg-slate-950/35 px-3 py-3">
                              <p className="text-[11px] uppercase tracking-[0.16em] text-slate-500">Consumed</p>
                              <p className="mt-1 text-lg font-semibold text-white">{formatBandwidthBytes(consumedBytes)}</p>
                              <p className="text-sm text-slate-500">{formatTokenAmount(consumedTokens)} tokens</p>
                            </div>
                            <div className="rounded-xl bg-slate-950/35 px-3 py-3">
                              <p className="text-[11px] uppercase tracking-[0.16em] text-slate-500">Total</p>
                              <p className="mt-1 text-lg font-semibold text-white">{formatBandwidthBytes(allocation.totalBandwidthBytes)}</p>
                              <p className="text-sm text-slate-500">{formatTokenAmount(allocation.tokenAmount)} tokens</p>
                            </div>
                            <div className="rounded-xl bg-slate-950/35 px-3 py-3">
                              <p className="text-[11px] uppercase tracking-[0.16em] text-slate-500">Expiry</p>
                              <p className="mt-1 text-lg font-semibold text-white break-words">{formatDate(allocation.expiresAt)}</p>
                            </div>
                          </div>

                          <div className="mt-3 flex flex-wrap gap-2 text-xs">
                            <span className="rounded-full border border-white/10 bg-white/[0.04] px-2.5 py-1 text-slate-300">
                              Split {allocation.adminPercent?.toFixed(0) ?? "0"}/{allocation.usagePoolPercent?.toFixed(0) ?? "0"}/{allocation.reservePoolPercent?.toFixed(0) ?? "0"}
                            </span>
                            <span className="rounded-full border border-white/10 bg-white/[0.04] px-2.5 py-1 text-slate-300">
                              Admin {formatTokenAmount(allocation.adminAmount ?? 0)}
                            </span>
                            <span className="rounded-full border border-white/10 bg-white/[0.04] px-2.5 py-1 text-slate-300">
                              Usage {formatTokenAmount(allocation.usagePoolDistributed ?? 0)} / {formatTokenAmount(allocation.usagePoolTotal ?? 0)}
                            </span>
                            <span className="rounded-full border border-white/10 bg-white/[0.04] px-2.5 py-1 text-slate-300">
                              Reserve {formatTokenAmount(allocation.reservePoolDistributed ?? 0)} / {formatTokenAmount(allocation.reservePoolTotal ?? 0)}
                            </span>
                            <span className="rounded-full border border-white/10 bg-white/[0.04] px-2.5 py-1 text-slate-300">
                              Settlement {allocation.settlementStatus || "pending"}
                            </span>
                          </div>
                          {allocation.settlementWarning ? (
                            <p className="mt-2 text-xs text-amber-200">{allocation.settlementWarning}</p>
                          ) : null}

                          <div className="mt-4 flex flex-wrap gap-2">
                            <div className="min-w-0 flex-1" />
                            <div className="flex flex-wrap gap-2">
                              <button
                                type="button"
                                onClick={() => editingUser && openEditBandwidthEntry(editingUser, allocation)}
                                className="btn-secondary gap-2 px-3 py-2 text-xs"
                              >
                                <Calendar className="h-3.5 w-3.5" />
                                Update Expiry
                              </button>
                              <button
                                type="button"
                                onClick={() => editingUser && openAdjustBandwidthEntry(editingUser, allocation, "increase")}
                                className="btn-secondary gap-2 px-3 py-2 text-xs"
                              >
                                <Plus className="h-3.5 w-3.5" />
                                Increase
                              </button>
                              <button
                                type="button"
                                onClick={() => editingUser && openAdjustBandwidthEntry(editingUser, allocation, "reduce")}
                                className="btn-secondary gap-2 px-3 py-2 text-xs"
                              >
                                <Minus className="h-3.5 w-3.5" />
                                Reduce
                              </button>
                            </div>
                          </div>
                        </div>
                      );
                    })}
                  </div>
                ) : (
                  <div className="mt-4 rounded-2xl border border-dashed border-white/10 p-4 text-sm text-slate-500">
                    No bandwidth entries yet for this user.
                  </div>
                )}
              </div>
            </div>
          ) : (
            <form className="space-y-4" onSubmit={(event) => void submitUser(event)}>
              {mainWalletBalance !== null ? (
                <div className={`rounded-2xl border px-4 py-3 text-sm ${mainWalletBalance < 0 ? "border-rose-400/20 bg-rose-400/10 text-rose-200" : "border-sky-400/20 bg-sky-400/10 text-sky-200"}`}>
                  Main wallet available: {formatTokenAmount(mainWalletBalance)} Mei
                </div>
              ) : null}

              <label className="block">
                <span className="mb-2 block text-sm font-medium text-slate-300">Email</span>
                <input
                  value={form.email}
                  onChange={(event) => setForm((current) => ({ ...current, email: event.target.value }))}
                  placeholder="user@example.com"
                  className="input-shell"
                />
              </label>

              <label className="block">
                <span className="mb-2 block text-sm font-medium text-slate-300">Notes</span>
                <textarea
                  value={form.notes}
                  onChange={(event) => setForm((current) => ({ ...current, notes: event.target.value }))}
                  rows={4}
                  placeholder="Optional note for this user"
                  className="input-shell resize-none"
                />
              </label>

              <div className="grid gap-4 md:grid-cols-3">
                <label className="block">
                  <span className="mb-2 block text-sm font-medium text-slate-300">Initial Bandwidth (GB)</span>
                  <input
                    type="number"
                    min={0}
                    value={form.initialBandwidthGb}
                    onChange={(event) => setForm((current) => ({ ...current, initialBandwidthGb: Number(event.target.value) || 0 }))}
                    className="input-shell"
                  />
                </label>

                <label className="block">
                  <span className="mb-2 block text-sm font-medium text-slate-300">Initial Tokens</span>
                  <input
                    type="number"
                    min={0}
                    step="0.01"
                    value={form.initialTokenAmount}
                    onChange={(event) => setForm((current) => ({ ...current, initialTokenAmount: Number(event.target.value) || 0 }))}
                    className="input-shell"
                  />
                </label>

                <label className="block">
                  <span className="mb-2 block text-sm font-medium text-slate-300">Initial Expiry</span>
                  <input
                    type="datetime-local"
                    required
                    value={form.initialExpiresAt}
                    onChange={(event) => setForm((current) => ({ ...current, initialExpiresAt: event.target.value }))}
                    className="input-shell"
                  />
                </label>
              </div>

              <label className="flex items-center gap-3 rounded-2xl border border-white/10 bg-white/[0.03] px-4 py-3 text-sm text-slate-200">
                <input
                  type="checkbox"
                  checked={form.enabled}
                  onChange={(event) => setForm((current) => ({ ...current, enabled: event.target.checked }))}
                  className="h-4 w-4 rounded border-white/20 bg-transparent"
                />
                Enabled for node sync
              </label>

              <div className="flex flex-wrap justify-end gap-3">
                <button type="button" onClick={closeUserDialog} className="btn-secondary">
                  Cancel
                </button>
                <button type="submit" className="btn-primary">
                  Create User
                </button>
              </div>
            </form>
          )}
        </div>
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
        open={allocationEditDialogOpen}
        title="Update Entry Expiry"
        description={
          allocationEditTargetUser && allocationEditTarget
            ? `Change the expiry date for entry #${allocationEditTarget.id} on ${allocationEditTargetUser.email}.`
            : ""
        }
        hideActions
        onCancel={closeAllocationEditDialog}
        onConfirm={() => undefined}
      >
        <form className="space-y-4" onSubmit={(event) => void submitAllocationEdit(event)}>
          {formError ? (
            <div className="rounded-2xl border border-rose-400/20 bg-rose-400/10 px-4 py-3 text-sm text-rose-200">
              {formError}
            </div>
          ) : null}

          <div className="rounded-2xl border border-white/10 bg-white/[0.03] px-4 py-3 text-sm text-slate-300">
            Changing expiry does not change bandwidth or token balances for this entry.
          </div>

          <label className="block">
            <span className="mb-2 block text-sm font-medium text-slate-300">Expiry</span>
            <input
              type="datetime-local"
              required
              value={allocationEditForm.expiresAt}
              onChange={(event) => setAllocationEditForm({ expiresAt: event.target.value })}
              className="input-shell"
            />
          </label>

          <div className="flex flex-wrap justify-end gap-3">
            <button type="button" onClick={closeAllocationEditDialog} className="btn-secondary">
              Cancel
            </button>
            <button type="submit" className="btn-primary">
              Save Expiry
            </button>
          </div>
        </form>
      </ConfirmDialog>

      <ConfirmDialog
        open={reductionDialogOpen}
        title={reductionForm.action === "increase" ? "Increase bandwidth entry" : "Reduce bandwidth entry"}
        description={
          reductionTarget && reductionAllocationTarget
            ? `${reductionForm.action === "increase" ? "Increase" : "Reduce"} bandwidth entry #${reductionAllocationTarget.id} for ${reductionTarget.email}.`
            : ""
        }
        hideActions
        onCancel={closeReductionDialog}
        onConfirm={() => undefined}
      >
        <form className="space-y-4" onSubmit={(event) => void submitReduction(event)}>
          {formError ? (
            <div className="rounded-2xl border border-rose-400/20 bg-rose-400/10 px-4 py-3 text-sm text-rose-200">
              {formError}
            </div>
          ) : null}

          {mainWalletBalance !== null ? (
            <div className={`rounded-2xl border px-4 py-3 text-sm ${mainWalletBalance < 0 ? "border-rose-400/20 bg-rose-400/10 text-rose-200" : "border-sky-400/20 bg-sky-400/10 text-sky-200"}`}>
              Main wallet available: {formatTokenAmount(mainWalletBalance)} Mei
            </div>
          ) : null}

          <label className="block">
            <span className="mb-2 block text-sm font-medium text-slate-300">
              {reductionForm.action === "increase" ? "Increase Bandwidth (GB)" : "Reduce Bandwidth (GB)"}
            </span>
            <input
              type="number"
              min={1}
              value={reductionForm.bandwidthGb}
              onChange={(event) => setReductionForm((current) => ({ ...current, bandwidthGb: Number(event.target.value) || 0 }))}
              className="input-shell"
            />
          </label>

          <label className="block">
            <span className="mb-2 block text-sm font-medium text-slate-300">Note</span>
            <textarea
              value={reductionForm.note}
              onChange={(event) => setReductionForm((current) => ({ ...current, note: event.target.value }))}
              rows={3}
              placeholder="Optional admin note for why this bandwidth is being reduced"
              className="input-shell resize-none"
            />
          </label>

          <div className="rounded-2xl border border-white/10 bg-white/[0.03] px-4 py-3 text-sm text-slate-300">
            {reductionForm.action === "increase"
              ? "Tokens are drawn automatically from the main wallet using this entry's token ratio."
              : "Tokens are refunded automatically to the main wallet using this entry's token ratio."}
          </div>

          <div className="flex flex-wrap justify-end gap-3">
            <button type="button" onClick={closeReductionDialog} className="btn-secondary">
              Cancel
            </button>
            <button type="submit" className="btn-primary">
              {reductionForm.action === "increase" ? "Increase Entry" : "Reduce Entry"}
            </button>
          </div>
        </form>
      </ConfirmDialog>

      <ConfirmDialog
        open={accessDialogOpen}
        title="User Access"
        description="Copy JSON, import, or legacy links from one focused workspace, then scan the QR code on mobile."
        hideActions
        panelClassName="max-w-6xl"
        onCancel={closeAccessDialog}
        onConfirm={() => undefined}
      >
        <div className="max-h-[calc(90vh-8rem)] overflow-y-auto pr-1">
          {accessLoading ? (
            <div className="space-y-4">
              <div className="panel-subtle p-5">
                <div className="h-4 w-28 animate-pulse rounded-full bg-white/10" />
                <div className="mt-4 h-7 w-1/2 animate-pulse rounded-full bg-white/10" />
                <div className="mt-6 h-4 w-full animate-pulse rounded-full bg-white/10" />
              </div>
              <div className="grid gap-4 xl:grid-cols-[320px,minmax(0,1fr)]">
                <div className="panel-subtle p-6">
                  <div className="aspect-square animate-pulse rounded-[2rem] bg-white/10" />
                </div>
                <div className="space-y-4">
                  {[0, 1, 2].map((index) => (
                    <div key={index} className="panel-subtle p-5">
                      <div className="h-4 w-32 animate-pulse rounded-full bg-white/10" />
                      <div className="mt-4 h-20 animate-pulse rounded-2xl bg-white/10" />
                    </div>
                  ))}
                </div>
              </div>
            </div>
          ) : accessError ? (
            <div className="space-y-4">
              <div className="rounded-3xl border border-rose-400/20 bg-rose-500/10 p-5 text-sm text-rose-200">{accessError}</div>
              <div className="flex flex-wrap justify-end gap-3">
                <button type="button" onClick={() => selectedUser && void openAccess(selectedUser)} className="btn-primary">
                  Retry
                </button>
                <button type="button" onClick={closeAccessDialog} className="btn-secondary">
                  Close
                </button>
              </div>
            </div>
          ) : selectedAccess ? (
            <div className="space-y-4">
              <div className="grid gap-4 xl:grid-cols-[320px,minmax(0,1fr)]">
                <div className="space-y-4">
                  <div className="panel-subtle p-5">
                    <p className="metric-kicker">User</p>
                    <p className="mt-3 text-sm font-semibold text-white">{selectedUser?.email ?? "Unknown user"}</p>
                    <p className="mt-3 font-mono text-xs text-slate-500">{selectedUser?.uuid ?? selectedAccess.uuid}</p>
                    <div className="mt-5">
                      <BandwidthUsage
                        usedBytes={selectedUser?.bandwidthUsedBytes ?? 0}
                        limitGb={selectedUserSummary?.bandwidthLimitGb ?? selectedUser?.bandwidthLimitGb ?? 0}
                        showDetails
                      />
                    </div>
                  </div>

                  <div className="panel-subtle p-5">
                    <div className="flex items-start justify-center">
                      {qrCodeUrl ? (
                        <img
                          src={qrCodeUrl}
                          alt="Sing-box import QR code"
                          className="h-auto w-full max-w-[260px] rounded-[24px] bg-white p-4 shadow-glow"
                        />
                      ) : (
                        <div className="flex h-[260px] w-full max-w-[260px] items-center justify-center rounded-[24px] bg-white/5">
                          <p className="text-center text-sm text-slate-500">QR code not available</p>
                        </div>
                      )}
                    </div>
                  </div>
                </div>

                <div className="min-w-0 space-y-4">
                  {accessCards.map((card) => {
                    const hasValue = Boolean(card.value);

                    return (
                      <div key={card.key} className="panel-subtle p-5">
                        <div className="flex flex-wrap items-start justify-between gap-3">
                          <div>
                            <p className="metric-kicker">{card.label}</p>
                          </div>
                          <div className="flex flex-wrap gap-2">
                            <button
                              type="button"
                              disabled={!hasValue}
                              onClick={() => void copyText(card.value, card.key)}
                              className="btn-primary gap-2 px-3 py-2 text-xs disabled:bg-slate-500"
                            >
                              <Copy className="h-3.5 w-3.5" />
                              {copiedKey === card.key ? "Copied" : card.copyLabel}
                            </button>
                            {card.openLabel && hasValue ? (
                              <a href={card.value} className="btn-secondary px-3 py-2 text-xs">
                                <Link2 className="mr-2 inline h-3.5 w-3.5" />
                                {card.openLabel}
                              </a>
                            ) : null}
                          </div>
                        </div>
                        <div className="mt-4 max-h-44 overflow-auto rounded-2xl border border-white/10 bg-slate-950/40 p-3">
                          <p className="break-all font-mono text-xs leading-6 text-slate-300">{card.value || card.emptyMessage}</p>
                        </div>
                      </div>
                    );
                  })}

                  <div className="panel-subtle p-5">
                    <div className="flex flex-wrap items-start justify-between gap-3">
                      <div>
                        <p className="metric-kicker">Node Usage Share</p>
                        <p className="mt-2 text-sm text-slate-400">Percentage of this user&apos;s recorded traffic on each node.</p>
                      </div>
                      <div className="rounded-full border border-white/10 bg-white/[0.04] px-3 py-2 text-xs text-slate-300">
                        {selectedNodeUsage.totalUsageBytes > 0
                          ? `${formatBandwidthBytes(selectedNodeUsage.totalUsageBytes)} tracked`
                          : `${selectedNodeUsage.rows.length} nodes`}
                      </div>
                    </div>

                    {selectedNodeUsage.rows.length ? (
                      <div className="mt-4 space-y-3">
                        {selectedNodeUsage.totalUsageBytes <= 0 ? (
                          <div className="rounded-2xl border border-dashed border-white/10 px-4 py-3 text-sm text-slate-500">
                            No node traffic has been recorded for this user yet. The tiles below will fill in after the first usage report.
                          </div>
                        ) : null}

                        <div className="grid gap-3 sm:grid-cols-2 2xl:grid-cols-3">
                          {selectedNodeUsage.rows.map((row) => {
                            const metaParts = [row.publicHost, row.protocols.length ? row.protocols.join(" / ") : ""].filter(Boolean);
                            const emphasis: NodeUsageRingProps["emphasis"] =
                              row.bandwidthBytes <= 0 ? "idle" : row.hasCurrentAccess ? "active" : "history";

                            return (
                              <div key={row.key} className="rounded-2xl border border-white/10 bg-slate-950/35 p-4">
                                <div className="flex items-start gap-3">
                                  <NodeUsageRing percentage={row.percentage} emphasis={emphasis} />
                                  <div className="min-w-0 flex-1">
                                    <div className="flex flex-wrap items-center gap-2">
                                      <p className="truncate text-sm font-semibold text-white">{row.nodeName}</p>
                                      {!row.hasCurrentAccess ? (
                                        <span className="rounded-full border border-amber-400/20 bg-amber-400/10 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-[0.16em] text-amber-200">
                                          History
                                        </span>
                                      ) : null}
                                    </div>
                                    <p className="mt-1 text-xs text-slate-500">{metaParts.join(" · ") || "Usage recorded for this node."}</p>
                                    <div className="mt-3 flex items-end justify-between gap-3">
                                      <div>
                                        <p className="text-xs uppercase tracking-[0.16em] text-slate-500">Traffic</p>
                                        <p className="mt-1 text-sm font-semibold text-white">{formatBandwidthBytes(row.bandwidthBytes)}</p>
                                      </div>
                                      <div className="text-right">
                                        <p className="text-xs uppercase tracking-[0.16em] text-slate-500">Share</p>
                                        <p className={`mt-1 text-sm font-semibold ${emphasis === "history" ? "text-amber-200" : emphasis === "idle" ? "text-slate-300" : "text-sky-300"}`}>
                                          {formatPercentage(row.percentage)}
                                        </p>
                                      </div>
                                    </div>
                                  </div>
                                </div>
                              </div>
                            );
                          })}
                        </div>
                      </div>
                    ) : (
                      <div className="mt-4 rounded-2xl border border-dashed border-white/10 p-4 text-sm text-slate-500">
                        Node usage will appear here once this user starts consuming traffic.
                      </div>
                    )}
                  </div>

                  <div className="panel-subtle p-5">
                    <div className="flex items-center gap-2">
                      <QrCode className="h-4 w-4 text-sky-300" />
                      <p className="metric-kicker">Node URLs</p>
                    </div>
                    <div className="mt-4 space-y-3">
                      {selectedAccess.nodeLinks?.length ? selectedAccess.nodeLinks.map((link) => (
                        <div key={`${link.nodeName}-${link.protocol}`} className="rounded-2xl border border-white/10 bg-slate-950/35 p-4">
                          <p className="text-xs font-semibold uppercase tracking-[0.16em] text-slate-500">
                            {link.nodeName} / {link.protocol}
                          </p>
                          <p className="mt-2 break-all font-mono text-xs leading-6 text-slate-300">{link.url}</p>
                          <button
                            onClick={() => void copyText(link.url, `${link.nodeName}-${link.protocol}`)}
                            className="btn-primary mt-3 gap-2 px-3 py-2 text-xs"
                          >
                            <Copy className="h-3.5 w-3.5" />
                            {copiedKey === `${link.nodeName}-${link.protocol}` ? "Copied" : `Copy ${link.protocol} URL`}
                          </button>
                        </div>
                      )) : (
                        <div className="rounded-2xl border border-dashed border-white/10 p-4 text-sm text-slate-500">
                          No per-node links were returned for this user.
                        </div>
                      )}
                    </div>
                  </div>
                </div>
              </div>
            </div>
          ) : null}
        </div>
      </ConfirmDialog>
    </div>
  );
}
