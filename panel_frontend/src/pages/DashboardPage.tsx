import { Activity, ArrowUpRight, Globe, Server, ShieldCheck, Users } from "lucide-react";
import { useEffect, useState } from "react";
import api from "../api/client";
import { SectionCard } from "../components/SectionCard";
import { StatCard } from "../components/StatCard";
import type { DashboardStats, Node, UserStats } from "../types";

export function DashboardPage() {
  const [stats, setStats] = useState<DashboardStats>({ users: 0, activeUsers: 0, nodes: 0, onlineNodes: 0 });
  const [nodes, setNodes] = useState<Node[]>([]);

  useEffect(() => {
    void Promise.all([
      // Use lightweight stats endpoint instead of fetching all users
      api.get<UserStats>("/users/stats"),
      api.get<Node[]>("/nodes")
    ])
      .then(([statsRes, nodesRes]) => {
        const userStats = statsRes.data;
        const nextNodes = nodesRes.data;
        setNodes(nextNodes);
        setStats({
          users: userStats.totalUsers,
          activeUsers: userStats.enabledUsers,
          nodes: nextNodes.length,
          onlineNodes: nextNodes.filter((node) => node.healthStatus === "online").length
        });
      })
      .catch(() => undefined);
  }, []);

  const offlineNodes = stats.nodes - stats.onlineNodes;
  const disabledUsers = stats.users - stats.activeUsers;
  const latestNodes = [...nodes].sort((a, b) => b.id - a.id).slice(0, 4);

  return (
    <div className="space-y-4">
      <section className="grid gap-4 xl:grid-cols-[minmax(0,1.2fr),minmax(320px,0.8fr)]">
        <div className="panel-surface overflow-hidden p-6 sm:p-7">
          <div className="grid gap-8 2xl:grid-cols-[minmax(0,1fr),320px] 2xl:items-end">
            <div className="max-w-2xl">
              <p className="metric-kicker">Sing-box Control Plane</p>
              <h3 className="mt-3 font-display text-3xl font-bold text-white sm:text-4xl">
                Manage multi-node VPN users, nodes, and subscriptions.
              </h3>
              <p className="mt-4 text-sm leading-7 text-slate-400 sm:text-base">
                Create user identities, bootstrap VPS nodes, sync active accounts across the fleet, and deliver import
                links for sing-box clients from one admin panel.
              </p>
            </div>

            <div className="grid gap-3 sm:grid-cols-2 2xl:grid-cols-1">
              <div className="panel-subtle p-4">
                <p className="metric-kicker">Fleet Health</p>
                <p className="mt-3 text-3xl font-bold text-white">{stats.onlineNodes}/{stats.nodes || 0}</p>
                <p className="mt-2 text-sm text-slate-400">nodes online</p>
              </div>
              <div className="panel-subtle p-4">
                <p className="metric-kicker">User Access</p>
                <p className="mt-3 text-3xl font-bold text-white">{stats.activeUsers}/{stats.users || 0}</p>
                <p className="mt-2 text-sm text-slate-400">users enabled</p>
              </div>
            </div>
          </div>
        </div>

        <div className="panel-surface p-6">
          <div className="flex items-center justify-between">
            <div>
              <p className="metric-kicker">Control Signals</p>
              <h3 className="mt-2 font-display text-2xl font-semibold text-white">At a glance</h3>
            </div>
            <div className="rounded-2xl border border-white/10 bg-sky-400/10 p-3 text-sky-300">
              <Globe className="h-5 w-5" />
            </div>
          </div>
          <div className="mt-6 space-y-4">
            {[
              {
                label: "Nodes needing attention",
                value: offlineNodes,
                tone: offlineNodes > 0 ? "text-amber-300" : "text-emerald-300",
                note: offlineNodes > 0 ? "offline or unreachable" : "all registered nodes healthy"
              },
              {
                label: "Users currently disabled",
                value: disabledUsers,
                tone: disabledUsers > 0 ? "text-slate-100" : "text-slate-400",
                note: disabledUsers > 0 ? "not receiving synced access" : "all user accounts active"
              }
            ].map((item) => (
              <div key={item.label} className="panel-subtle flex items-center justify-between p-4">
                <div>
                  <p className="text-sm font-medium text-slate-300">{item.label}</p>
                  <p className="mt-1 text-xs uppercase tracking-[0.18em] text-slate-500">{item.note}</p>
                </div>
                <p className={`font-display text-3xl font-bold ${item.tone}`}>{item.value}</p>
              </div>
            ))}
          </div>
        </div>
      </section>

      <section className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <StatCard label="Total Users" value={stats.users} detail="registered accounts" icon={<Users className="h-6 w-6 text-white" />} accent="bg-sky-500/20 text-sky-300" />
        <StatCard label="Active Users" value={stats.activeUsers} detail="sync-ready identities" icon={<ShieldCheck className="h-6 w-6 text-white" />} accent="bg-emerald-500/20 text-emerald-300" />
        <StatCard label="Registered Nodes" value={stats.nodes} detail="data-plane endpoints" icon={<Server className="h-6 w-6 text-white" />} accent="bg-orange-500/20 text-orange-200" />
        <StatCard label="Online Nodes" value={stats.onlineNodes} detail="passing health checks" icon={<Activity className="h-6 w-6 text-white" />} accent="bg-violet-500/20 text-violet-200" />
      </section>

      <section className="grid gap-4 xl:grid-cols-2">
        <SectionCard
          eyebrow="Recent Users"
          title="Newest identities"
          description="Freshly created VPN users stay visible here so you can quickly confirm provisioning before sharing access links."
        >
          <div className="panel-subtle p-4 text-sm text-slate-400">
            <p>User list available on the</p>
            <a href="/users" className="mt-1 inline-block text-sm text-sky-400 hover:text-sky-300 hover:underline">
              Users page →
            </a>
          </div>
        </SectionCard>

        <SectionCard
          eyebrow="Node Snapshot"
          title="Recently registered nodes"
          description="A quick health view of your latest nodes with region and endpoint details for day-to-day infrastructure checks."
        >
          <div className="space-y-3">
            {latestNodes.length ? latestNodes.map((node) => (
              <div key={node.id} className="panel-subtle flex flex-col gap-3 p-4 sm:flex-row sm:items-center sm:justify-between">
                <div>
                  <p className="text-sm font-semibold text-white">{node.name}</p>
                  <p className="mt-1 text-xs uppercase tracking-[0.18em] text-slate-500">{node.location || "Unknown region"}</p>
                </div>
                <div className="flex items-center gap-3">
                  <span className={`status-pill ${
                    node.healthStatus === "online" ? "text-emerald-300" : node.healthStatus === "offline" ? "text-rose-300" : "text-slate-400"
                  }`}>
                    <span className={`h-2 w-2 rounded-full ${
                      node.healthStatus === "online" ? "bg-emerald-400" : node.healthStatus === "offline" ? "bg-rose-400" : "bg-slate-500"
                    }`} />
                    {node.healthStatus}
                  </span>
                  <p className="text-xs text-slate-500">{node.publicHost || node.baseUrl}</p>
                </div>
              </div>
            )) : (
              <div className="panel-subtle p-4 text-sm text-slate-400">No nodes found yet.</div>
            )}
          </div>
        </SectionCard>
      </section>

    </div>
  );
}
