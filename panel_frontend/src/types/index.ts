export interface User {
  id: number;
  uuid: string;
  email: string;
  enabled: boolean;
  expiresAt?: string | null;
  bandwidthLimitGb: number;
  bandwidthUsedBytes: number;
  tokenBalance: number;
  notes: string;
  bandwidthAllocations: UserBandwidthAllocation[];
  createdAt: string;
}

export interface UserBandwidthAllocation {
  id: number;
  userId: number;
  totalBandwidthBytes: number;
  remainingBandwidthBytes: number;
  tokenAmount: number;
  remainingTokens: number;
  adminPercent: number;
  usagePoolPercent: number;
  reservePoolPercent: number;
  adminAmount: number;
  usagePoolTotal: number;
  usagePoolDistributed: number;
  reservePoolTotal: number;
  reservePoolDistributed: number;
  settlementStatus: string;
  settlementWarning: string;
  settledAt?: string | null;
  expiresAt?: string | null;
  createdAt: string;
  updatedAt: string;
}

export interface UserRecord {
  id: number;
  userId: number;
  action: string;
  title: string;
  details: string;
  createdAt: string;
  updatedAt: string;
}

export interface Node {
  id: number;
  minerId?: number | null;
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
  rewardedTokens: number;
  healthStatus: string;
  singboxVersion: string;
  lastHeartbeat?: string | null;
  lastSyncAt?: string | null;
}

export interface Miner {
  id: number;
  name: string;
  walletAddress: string;
  rewardedTokens: number;
  notes: string;
  nodes: Node[];
  createdAt: string;
  updatedAt: string;
}

export interface DashboardStats {
  users: number;
  activeUsers: number;
  nodes: number;
  onlineNodes: number;
}

export interface MintPoolState {
  id: number;
  totalMmkReserve: number;
  totalMeiMinted: number;
  mainWalletBalance: number;
  adminWalletBalance: number;
  totalTransferredToUsers: number;
  totalRewardedToMiners: number;
  totalAdminCollected: number;
  lastMintAt?: string | null;
  createdAt: string;
  updatedAt: string;
}

export interface DistributionSettings {
  adminPercent: number;
  usagePoolPercent: number;
  reservePoolPercent: number;
}

export interface MintPoolEvent {
  id: number;
  mmkAmount: number;
  meiAmount: number;
  exchangeRate: string;
  note: string;
  createdAt: string;
}

export interface MintPoolTransferEvent {
  id: number;
  transferType: string;
  fromWallet: string;
  toWallet: string;
  amount: number;
  userId?: number | null;
  minerId?: number | null;
  nodeId?: number | null;
  note: string;
  createdAt: string;
}

export interface MintPoolSnapshot {
  pool: MintPoolState;
  history: MintPoolEvent[];
  transfers: MintPoolTransferEvent[];
}
