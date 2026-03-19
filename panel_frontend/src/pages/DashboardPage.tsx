import { Activity, Server, ShieldCheck, Users } from "lucide-react";
import { useEffect, useState } from "react";
import api from "../api/client";
import { SectionCard } from "../components/SectionCard";
import { StatCard } from "../components/StatCard";
import type { DashboardStats, Node, User } from "../types";

export function DashboardPage() {
  const [stats, setStats] = useState<DashboardStats>({ users: 0, activeUsers: 0, nodes: 0, onlineNodes: 0 });

  useEffect(() => {
    void Promise.all([api.get<User[]>("/users"), api.get<Node[]>("/nodes")])
      .then(([usersRes, nodesRes]) => {
        const users = usersRes.data;
        const nodes = nodesRes.data;
        setStats({
          users: users.length,
          activeUsers: users.filter((user) => user.enabled).length,
          nodes: nodes.length,
          onlineNodes: nodes.filter((node) => node.healthStatus === "online").length
        });
      })
      .catch(() => undefined);
  }, []);

  return (
    <div className="space-y-8">
      <section className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
        <StatCard label="Total Users" value={stats.users} icon={<Users className="h-6 w-6 text-white" />} accent="bg-tide" />
        <StatCard label="Active Users" value={stats.activeUsers} icon={<ShieldCheck className="h-6 w-6 text-white" />} accent="bg-pine" />
        <StatCard label="Registered Nodes" value={stats.nodes} icon={<Server className="h-6 w-6 text-white" />} accent="bg-ember" />
        <StatCard label="Online Nodes" value={stats.onlineNodes} icon={<Activity className="h-6 w-6 text-white" />} accent="bg-slate-700" />
      </section>

      <SectionCard
        title="Operational Flow"
        description="The control plane creates identities, syncs nodes, and keeps user traffic on the data plane."
      >
        <div className="grid gap-4 md:grid-cols-3">
          {[
            "Create or update a user from the admin panel.",
            "Push active users to all node agents using signed API calls.",
            "Deliver a multi-node subscription covering VLESS, TUIC, and Hysteria2."
          ].map((step, index) => (
            <div key={step} className="rounded-2xl bg-slate-50 p-5">
              <p className="text-sm font-semibold text-tide">Step {index + 1}</p>
              <p className="mt-2 text-sm text-slate-600">{step}</p>
            </div>
          ))}
        </div>
      </SectionCard>
    </div>
  );
}
