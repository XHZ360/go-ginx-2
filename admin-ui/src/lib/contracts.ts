export const CONTRACT_ERROR_CODES = [
  'UNAUTHENTICATED',
  'FORBIDDEN',
  'VALIDATION_FAILED',
  'NOT_FOUND',
  'CONFLICT',
  'ENTRY_CONFLICT',
  'UNSUPPORTED',
  'INTERNAL',
  'INVALID_CSRF',
  'NETWORK',
  'UNKNOWN',
] as const;

export type ContractErrorCode = (typeof CONTRACT_ERROR_CODES)[number];

export type SessionBootstrap = {
  authenticated: boolean;
  username?: string;
  csrfToken?: string;
  pollIntervalSeconds?: number;
};

export type PageInfo = {
  page: number;
  pageSize: number;
  totalCount: number;
  totalPages: number;
  hasNext: boolean;
  hasPrev: boolean;
};

export type User = {
  id: string;
  username: string;
  role: string;
  status: string;
  clientCount: number;
  proxyCount: number;
  uploadBytes: number;
  downloadBytes: number;
  lastActivityAt?: string | null;
  hasPasswordHash: boolean;
  createdAt: string;
  updatedAt: string;
};

export type ClientRuntime = {
  online: boolean;
  protocol?: string | null;
  connectedAt?: string | null;
  lastHeartbeat?: string | null;
  configVersion?: number | null;
  activeProxies?: number | null;
  activeStreams?: number | null;
  uploadBytes?: number | null;
  downloadBytes?: number | null;
  errorSummary?: string | null;
};

export type ProxySummary = {
  id: string;
  name: string;
  type: string;
  status: string;
  runtimeStatus: string;
  entryHost?: string | null;
  entryPort?: number | null;
  targetHost?: string | null;
  targetPort?: number | null;
  activeTCPConnections?: number | null;
};

export type Client = {
  id: string;
  userId: string;
  name: string;
  status: string;
  version: number;
  runtime: ClientRuntime;
  lastOnlineAt?: string | null;
  lastOfflineAt?: string | null;
  createdAt: string;
  updatedAt: string;
};

export type ClientDetail = Client & {
  managedProxies: ProxySummary[];
};

export type ManagedCertificate = {
  proxyId: string;
  certificateId?: string | null;
  host?: string | null;
  status?: string | null;
  notAfter?: string | null;
  lastIssuedAt?: string | null;
  lastRenewedAt?: string | null;
  lastError?: string | null;
};

export type ProxyConfig = {
  entryHost?: string | null;
  entryPort?: number | null;
  targetHost?: string | null;
  targetPort?: number | null;
};

export type ProxyRecord = {
  id: string;
  userId: string;
  clientId: string;
  name: string;
  type: string;
  status: string;
  description?: string | null;
  runtimeStatus: string;
  activeTCPConnections: number;
  uploadBytes: number;
  downloadBytes: number;
  tcpErrorCount: number;
  udpErrorCount: number;
  httpErrorCount: number;
  config: ProxyConfig;
  certificate?: ManagedCertificate | null;
  createdAt: string;
  updatedAt: string;
};

export type AuditEvent = {
  id: string;
  actorType: string;
  actorId: string;
  resourceType: string;
  resourceId: string;
  action: string;
  result: string;
  createdAt: string;
};

export type DashboardSummary = {
  onlineClientCount: number;
  enabledProxyCount: number;
  activeTCPConnectionCount: number;
  cumulativeUploadBytes: number;
  cumulativeDownloadBytes: number;
  cumulativeTCPErrorCount: number;
  cumulativeUDPErrorCount: number;
  cumulativeHTTPErrorCount: number;
};

export type PageResult<T> = {
  items: T[];
  totalCount: number;
  pageInfo: PageInfo;
};

export type ApiErrorCategory =
  | 'auth'
  | 'forbidden'
  | 'validation'
  | 'not-found'
  | 'conflict'
  | 'unsupported'
  | 'internal'
  | 'network';

export class ApiError extends Error {
  code: ContractErrorCode;
  category: ApiErrorCategory;
  fields?: Record<string, string>;
  status?: number;

  constructor(
    code: ContractErrorCode,
    message: string,
    options?: {
      category?: ApiErrorCategory;
      fields?: Record<string, string>;
      status?: number;
    },
  ) {
    super(message);
    this.name = 'ApiError';
    this.code = code;
    this.category = options?.category ?? categorizeError(code);
    this.fields = options?.fields;
    this.status = options?.status;
  }
}

export function categorizeError(code: ContractErrorCode): ApiErrorCategory {
  switch (code) {
    case 'UNAUTHENTICATED':
      return 'auth';
    case 'FORBIDDEN':
    case 'INVALID_CSRF':
      return 'forbidden';
    case 'VALIDATION_FAILED':
      return 'validation';
    case 'NOT_FOUND':
      return 'not-found';
    case 'CONFLICT':
    case 'ENTRY_CONFLICT':
      return 'conflict';
    case 'UNSUPPORTED':
      return 'unsupported';
    case 'NETWORK':
      return 'network';
    default:
      return 'internal';
  }
}

export function isApiError(error: unknown): error is ApiError {
  return error instanceof ApiError;
}

export function isAuthError(error: unknown): boolean {
  return isApiError(error) && error.category === 'auth';
}

export function isValidationError(error: unknown): error is ApiError {
  return isApiError(error) && error.category === 'validation';
}

export function isNotFoundError(error: unknown): boolean {
  return isApiError(error) && error.category === 'not-found';
}

export function isConflictError(error: unknown): boolean {
  return isApiError(error) && error.category === 'conflict';
}
