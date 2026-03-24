export type UserType = "light" | "medium" | "moderate" | "unknown";

export interface User {
  id: number;
  uuid: string;
  email: string;
  enabled: boolean;
  isTesting: boolean;
  expiresAt?: string | null;
  bandwidthLimitGb: number;
  bandwidthUsedBytes: number;
  lastWeekMeteredBytes: number;
  tokenBalance: number;
  notes: string;
  bandwidthAllocations: UserBandwidthAllocation[];
  createdAt: string;
  userType?: UserType;
}

export interface UserClassificationStatus {
  schedulerActive: boolean;
  lastRunAt?: string | null;
  nextRunAt?: string | null;
}

export interface UserClassificationStats {
  lightUsers: number;
  mediumUsers: number;
  moderateUsers: number;
  unknownUsers: number;
  totalUsers: number;
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
  nodeUsages: UserBandwidthNodeUsage[];
  createdAt: string;
  updatedAt: string;
}

export interface UserBandwidthNodeUsage {
  id: number;
  allocationId: number;
  userId: number;
  nodeId: number;
  minerId?: number | null;
  bandwidthBytes: number;
  rewardedTokens: number;
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
  enabled: boolean;
  isTestable: boolean;
  location: string;
  publicHost: string;
  expiresAt?: string | null;
  bandwidthLimitGb: number;
  bandwidthUsedBytes: number;
  rewardedTokens: number;
  healthStatus: string;
  syncVerificationStatus: string;
  syncVerificationError: string;
  syncVerifiedAt?: string | null;
  lastAppliedConfigHash: string;
  appliedUserCount: number;
  lastConfigAppliedAt?: string | null;
  singboxVersion: string;
  lastHeartbeat?: string | null;
  lastSyncAt?: string | null;
}

export interface NodePortDiagnostic {
  label: string;
  port: number;
  protocol: string;
  checked: boolean;
  reachable: boolean;
  latencyMs: number;
  errorMessage?: string;
}

export interface NodeDiagnosticResult {
  nodeId: number;
  nodeName: string;
  publicHost: string;
  baseUrl: string;
  apiReachable: boolean;
  apiLatencyMs: number;
  apiErrorMessage?: string;
  downloadMbps: number;
  uploadMbps: number;
  downloadBytes: number;
  uploadBytes: number;
  downloadError?: string;
  uploadError?: string;
  ports: NodePortDiagnostic[];
  qualityStatus: string;
  testedAt: string;
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

export interface ProtocolSettings {
  realitySnis: string[];
  hysteria2Masquerades: string[];
}

export interface ProtocolSettingsUpdateResponse extends ProtocolSettings {
  syncedNodes: number;
  syncError?: string;
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
