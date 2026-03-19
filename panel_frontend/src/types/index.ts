export interface User {
  id: number;
  uuid: string;
  email: string;
  enabled: boolean;
  expiresAt?: string | null;
  bandwidthLimitGb: number;
  bandwidthUsedBytes: number;
  notes: string;
  createdAt: string;
}

export interface Node {
  id: number;
  name: string;
  baseUrl: string;
  location: string;
  publicHost: string;
  vlessPort: number;
  tuicPort: number;
  hysteria2Port: number;
  expiresAt?: string | null;
  bandwidthLimitGb: number;
  bandwidthUsedBytes: number;
  healthStatus: string;
  singboxVersion: string;
  lastHeartbeat?: string | null;
  lastSyncAt?: string | null;
}

export interface DashboardStats {
  users: number;
  activeUsers: number;
  nodes: number;
  onlineNodes: number;
}
