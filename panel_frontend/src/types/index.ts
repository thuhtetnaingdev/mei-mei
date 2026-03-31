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

export interface UserStats {
  totalUsers: number;
  enabledUsers: number;
  disabledUsers: number;
  testingUsers: number;
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
  directPackages: string[];
  directDomains: string[];
  proxyDomains: string[];
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

// Pagination types
export interface PaginationMeta {
  total: number;
  page: number;
  pageSize: number;
  totalPages: number;
}

export interface UserListResult {
  users: User[];
  pagination: PaginationMeta;
}

export interface UserListOptions {
  // Search filters
  search?: string;

  // Boolean filters
  enabled?: boolean;
  isTesting?: boolean;

  // String filters
  userType?: UserType | "";

  // Numeric range filters (bandwidth in GB)
  minBandwidthGb?: number | null;
  maxBandwidthGb?: number | null;

  // Usage range filters (bytes)
  minUsageBytes?: number | null;
  maxUsageBytes?: number | null;

  // Token balance range filters
  minTokenBalance?: number | null;
  maxTokenBalance?: number | null;

  // Date range filters (RFC3339 format)
  createdAfter?: string | null;
  createdBefore?: string | null;

  // Sorting
  sortBy?: string;
  sortOrder?: "asc" | "desc";

  // Pagination
  page?: number;
  pageSize?: number;
}

// Public user response - sanitized user data for public access
export interface PublicUserResponse {
  id: number;
  uuid: string;
  email: string;
  enabled: boolean;
  isTesting: boolean;
  expiresAt?: string | null;
  bandwidthLimitGb: number;
  bandwidthUsedBytes: number;
  userType: string;
  createdAt: string;
  updatedAt: string;
  // Computed fields for user convenience
  bandwidthRemainingGb: number;
  usagePercentage: number;
  subscriptionUrl: string;
  singboxImportUrl: string;
  hiddifyImportUrl: string;
  singboxProfileUrl: string;
  clashProfileUrl: string;
}

// Key Verification types for VLESS REALITY Key Verification & Auto-Fix System
export interface KeyVerificationResult {
  nodeId: number;
  nodeName: string;
  status: "verified" | "mismatch" | "node_unreachable" | "error";
  publicKeyMatch: boolean;
  shortIDMatch: boolean;
  panelPublicKey: string;
  nodePublicKey: string;
  panelShortId: string;
  nodeShortId: string;
  verifiedAt?: string | null;
  autoFixTriggered?: boolean;
  autoFixSuccess?: boolean;
  error?: string;
}

export interface NodeKeyStatus {
  nodeId: number;
  nodeName: string;
  realityPublicKey: string;
  realityShortId: string;
  realityPrivateKeyHash?: string | null;
  nodePublicKey?: string | null;
  nodeShortId?: string | null;
  publicKeyMatch?: boolean;
  shortIdMatch?: boolean;
  lastKeyVerificationAt?: string | null;
  keyMismatchDetectedAt?: string | null;
  keyMismatchAutoFixedAt?: string | null;
}
