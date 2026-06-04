/** Internal type. DO NOT USE DIRECTLY. */
type Exact<T extends { [key: string]: unknown }> = { [K in keyof T]: T[K] };
/** Internal type. DO NOT USE DIRECTLY. */
export type Incremental<T> = T | { [P in keyof T]?: P extends ' $fragmentName' | '__typename' ? T[P] : never };
export type AdminAuditFilterInput = {
  action?: string | null | undefined;
  actorId?: string | null | undefined;
  actorType?: string | null | undefined;
  query?: string | null | undefined;
  resourceType?: string | null | undefined;
  result?: string | null | undefined;
};

export type AdminAuditInput = {
  filter?: AdminAuditFilterInput | null | undefined;
  page?: AdminPaginationInput | null | undefined;
  sort?: AdminSortInput | null | undefined;
};

export type AdminCertificateFilterInput = {
  query?: string | null | undefined;
  status?: string | null | undefined;
};

export type AdminCertificateMutationInput = {
  proxyId: string;
};

export type AdminCertificatesInput = {
  filter?: AdminCertificateFilterInput | null | undefined;
  page?: AdminPaginationInput | null | undefined;
  sort?: AdminSortInput | null | undefined;
};

export type AdminClientFilterInput = {
  online?: boolean | null | undefined;
  query?: string | null | undefined;
  status?: string | null | undefined;
  userId?: string | null | undefined;
};

export type AdminClientsInput = {
  filter?: AdminClientFilterInput | null | undefined;
  page?: AdminPaginationInput | null | undefined;
  sort?: AdminSortInput | null | undefined;
};

export type AdminCreateClientInput = {
  credential?: string | null | undefined;
  name: string;
  userId: string;
};

export type AdminCreateClientJoinInput = {
  enrollmentUrl?: string | null | undefined;
  id?: string | null | undefined;
  name: string;
  serverAddress?: string | null | undefined;
  serverCAFile?: string | null | undefined;
  serverName?: string | null | undefined;
  serverTLSAddress?: string | null | undefined;
  ttlSeconds?: number | null | undefined;
  userId: string;
};

export type AdminCreateProxyInput = {
  clientId: string;
  config: AdminProxyConfigInput;
  description?: string | null | undefined;
  name: string;
  type: string;
  userId: string;
};

export type AdminCreateUserInput = {
  password?: string | null | undefined;
  role?: string | null | undefined;
  username: string;
};

export type AdminPaginationInput = {
  page?: number | null | undefined;
  pageSize?: number | null | undefined;
};

export type AdminProxiesInput = {
  filter?: AdminProxyFilterInput | null | undefined;
  page?: AdminPaginationInput | null | undefined;
  sort?: AdminSortInput | null | undefined;
};

export type AdminProxyConfigInput = {
  certFile?: string | null | undefined;
  entryBindHost?: string | null | undefined;
  entryHost?: string | null | undefined;
  entryPort?: number | null | undefined;
  keyFile?: string | null | undefined;
  targetHost?: string | null | undefined;
  targetPort?: number | null | undefined;
};

export type AdminProxyFilterInput = {
  clientId?: string | null | undefined;
  query?: string | null | undefined;
  status?: string | null | undefined;
  type?: string | null | undefined;
  userId?: string | null | undefined;
};

export type AdminSetUserPasswordInput = {
  id: string;
  password: string;
};

export type AdminSortInput = {
  direction?: string | null | undefined;
  field?: string | null | undefined;
};

export type AdminUpdateProxyInput = {
  config: AdminProxyConfigInput;
  description?: string | null | undefined;
  id: string;
  name: string;
  type?: string | null | undefined;
};

export type AdminUserFilterInput = {
  query?: string | null | undefined;
  role?: string | null | undefined;
  status?: string | null | undefined;
};

export type AdminUserIdInput = {
  id: string;
};

export type AdminUsersInput = {
  filter?: AdminUserFilterInput | null | undefined;
  page?: AdminPaginationInput | null | undefined;
  sort?: AdminSortInput | null | undefined;
};

export type PageInfoFieldsFragment = { page: number, pageSize: number, totalCount: number, totalPages: number, hasNext: boolean, hasPrev: boolean };

export type UserFieldsFragment = { id: string, username: string, role: string, status: string, clientCount: number, proxyCount: number, uploadBytes: number, downloadBytes: number, lastActivityAt: string, hasPasswordHash: boolean, createdAt: string, updatedAt: string };

export type ClientRuntimeFieldsFragment = { online: boolean, protocol: string, connectedAt: string, lastHeartbeat: string, configVersion: number, activeProxies: number, activeStreams: number, uploadBytes: number, downloadBytes: number, errorSummary: string };

export type ProxySummaryFieldsFragment = { id: string, name: string, type: string, status: string, runtimeStatus: string, entryBindHost: string, entryHost: string, entryPort: number, targetHost: string, targetPort: number, activeTCPConnections: number };

export type ClientFieldsFragment = { id: string, userId: string, name: string, status: string, version: number, lastOnlineAt: string, lastOfflineAt: string, createdAt: string, updatedAt: string, runtime: { online: boolean, protocol: string, connectedAt: string, lastHeartbeat: string, configVersion: number, activeProxies: number, activeStreams: number, uploadBytes: number, downloadBytes: number, errorSummary: string } };

export type ClientDetailFieldsFragment = { id: string, userId: string, name: string, status: string, version: number, lastOnlineAt: string, lastOfflineAt: string, createdAt: string, updatedAt: string, runtime: { online: boolean, protocol: string, connectedAt: string, lastHeartbeat: string, configVersion: number, activeProxies: number, activeStreams: number, uploadBytes: number, downloadBytes: number, errorSummary: string }, managedProxies: Array<{ id: string, name: string, type: string, status: string, runtimeStatus: string, entryBindHost: string, entryHost: string, entryPort: number, targetHost: string, targetPort: number, activeTCPConnections: number }> };

export type ProxyConfigFieldsFragment = { entryBindHost: string, entryHost: string, entryPort: number, targetHost: string, targetPort: number, certFile: string, keyFile: string };

export type ManagedCertificateFieldsFragment = { proxyId: string, certificateId: string, host: string, status: string, notAfter: string, lastIssuedAt: string, lastRenewedAt: string, lastError: string };

export type ProxyFieldsFragment = { id: string, userId: string, clientId: string, name: string, type: string, status: string, description: string, runtimeStatus: string, activeTCPConnections: number, uploadBytes: number, downloadBytes: number, tcpErrorCount: number, udpErrorCount: number, httpErrorCount: number, createdAt: string, updatedAt: string, config: { entryBindHost: string, entryHost: string, entryPort: number, targetHost: string, targetPort: number, certFile: string, keyFile: string }, certificate: { proxyId: string, certificateId: string, host: string, status: string, notAfter: string, lastIssuedAt: string, lastRenewedAt: string, lastError: string } };

export type AuditFieldsFragment = { id: string, actorType: string, actorId: string, resourceType: string, resourceId: string, action: string, result: string, createdAt: string };

export type UserPayloadFieldsFragment = { userId: string, status: string, user: { id: string, username: string, role: string, status: string, clientCount: number, proxyCount: number, uploadBytes: number, downloadBytes: number, lastActivityAt: string, hasPasswordHash: boolean, createdAt: string, updatedAt: string } };

export type ClientPayloadFieldsFragment = { clientId: string, credential: string, token: string, client: { id: string, userId: string, name: string, status: string, version: number, lastOnlineAt: string, lastOfflineAt: string, createdAt: string, updatedAt: string, runtime: { online: boolean, protocol: string, connectedAt: string, lastHeartbeat: string, configVersion: number, activeProxies: number, activeStreams: number, uploadBytes: number, downloadBytes: number, errorSummary: string }, managedProxies: Array<{ id: string, name: string, type: string, status: string, runtimeStatus: string, entryBindHost: string, entryHost: string, entryPort: number, targetHost: string, targetPort: number, activeTCPConnections: number }> } };

export type ProxyPayloadFieldsFragment = { proxyId: string, status: string, proxy: { id: string, userId: string, clientId: string, name: string, type: string, status: string, description: string, runtimeStatus: string, activeTCPConnections: number, uploadBytes: number, downloadBytes: number, tcpErrorCount: number, udpErrorCount: number, httpErrorCount: number, createdAt: string, updatedAt: string, config: { entryBindHost: string, entryHost: string, entryPort: number, targetHost: string, targetPort: number, certFile: string, keyFile: string }, certificate: { proxyId: string, certificateId: string, host: string, status: string, notAfter: string, lastIssuedAt: string, lastRenewedAt: string, lastError: string } } };

export type CertificatePayloadFieldsFragment = { proxyId: string, status: string, certificate: { proxyId: string, certificateId: string, host: string, status: string, notAfter: string, lastIssuedAt: string, lastRenewedAt: string, lastError: string } };

export type DashboardQueryVariables = Exact<{ [key: string]: never; }>;


export type DashboardQuery = { dashboard: { onlineClientCount: number, enabledProxyCount: number, activeTCPConnectionCount: number, cumulativeUploadBytes: number, cumulativeDownloadBytes: number, cumulativeTCPErrorCount: number, cumulativeUDPErrorCount: number, cumulativeHTTPErrorCount: number } };

export type UsersQueryVariables = Exact<{
  input: AdminUsersInput | null | undefined;
}>;


export type UsersQuery = { users: { totalCount: number, pageInfo: { page: number, pageSize: number, totalCount: number, totalPages: number, hasNext: boolean, hasPrev: boolean }, items: Array<{ id: string, username: string, role: string, status: string, clientCount: number, proxyCount: number, uploadBytes: number, downloadBytes: number, lastActivityAt: string, hasPasswordHash: boolean, createdAt: string, updatedAt: string }> } };

export type UserQueryVariables = Exact<{
  id: string;
}>;


export type UserQuery = { user: { id: string, username: string, role: string, status: string, clientCount: number, proxyCount: number, uploadBytes: number, downloadBytes: number, lastActivityAt: string, hasPasswordHash: boolean, createdAt: string, updatedAt: string } };

export type ClientsQueryVariables = Exact<{
  input: AdminClientsInput | null | undefined;
}>;


export type ClientsQuery = { clients: { totalCount: number, pageInfo: { page: number, pageSize: number, totalCount: number, totalPages: number, hasNext: boolean, hasPrev: boolean }, items: Array<{ id: string, userId: string, name: string, status: string, version: number, lastOnlineAt: string, lastOfflineAt: string, createdAt: string, updatedAt: string, runtime: { online: boolean, protocol: string, connectedAt: string, lastHeartbeat: string, configVersion: number, activeProxies: number, activeStreams: number, uploadBytes: number, downloadBytes: number, errorSummary: string } }> } };

export type ClientQueryVariables = Exact<{
  id: string;
}>;


export type ClientQuery = { client: { id: string, userId: string, name: string, status: string, version: number, lastOnlineAt: string, lastOfflineAt: string, createdAt: string, updatedAt: string, runtime: { online: boolean, protocol: string, connectedAt: string, lastHeartbeat: string, configVersion: number, activeProxies: number, activeStreams: number, uploadBytes: number, downloadBytes: number, errorSummary: string }, managedProxies: Array<{ id: string, name: string, type: string, status: string, runtimeStatus: string, entryBindHost: string, entryHost: string, entryPort: number, targetHost: string, targetPort: number, activeTCPConnections: number }> } };

export type ProxiesQueryVariables = Exact<{
  input: AdminProxiesInput | null | undefined;
}>;


export type ProxiesQuery = { proxies: { totalCount: number, pageInfo: { page: number, pageSize: number, totalCount: number, totalPages: number, hasNext: boolean, hasPrev: boolean }, items: Array<{ id: string, userId: string, clientId: string, name: string, type: string, status: string, description: string, runtimeStatus: string, activeTCPConnections: number, uploadBytes: number, downloadBytes: number, tcpErrorCount: number, udpErrorCount: number, httpErrorCount: number, createdAt: string, updatedAt: string, config: { entryBindHost: string, entryHost: string, entryPort: number, targetHost: string, targetPort: number, certFile: string, keyFile: string }, certificate: { proxyId: string, certificateId: string, host: string, status: string, notAfter: string, lastIssuedAt: string, lastRenewedAt: string, lastError: string } }> } };

export type ProxyQueryVariables = Exact<{
  id: string;
}>;


export type ProxyQuery = { proxy: { id: string, userId: string, clientId: string, name: string, type: string, status: string, description: string, runtimeStatus: string, activeTCPConnections: number, uploadBytes: number, downloadBytes: number, tcpErrorCount: number, udpErrorCount: number, httpErrorCount: number, createdAt: string, updatedAt: string, config: { entryBindHost: string, entryHost: string, entryPort: number, targetHost: string, targetPort: number, certFile: string, keyFile: string }, certificate: { proxyId: string, certificateId: string, host: string, status: string, notAfter: string, lastIssuedAt: string, lastRenewedAt: string, lastError: string } } };

export type ProxyEntryOptionsQueryVariables = Exact<{ [key: string]: never; }>;


export type ProxyEntryOptionsQuery = { proxyEntryOptions: { tcpDefaultBindHost: string, httpDefaultBindHost: string, httpDefaultPort: number, httpsDefaultBindHost: string, httpsDefaultPort: number, hosts: Array<{ value: string, label: string, isDefault: boolean }> } };

export type CertificatesQueryVariables = Exact<{
  input: AdminCertificatesInput | null | undefined;
}>;


export type CertificatesQuery = { certificates: { totalCount: number, pageInfo: { page: number, pageSize: number, totalCount: number, totalPages: number, hasNext: boolean, hasPrev: boolean }, items: Array<{ proxyId: string, certificateId: string, host: string, status: string, notAfter: string, lastIssuedAt: string, lastRenewedAt: string, lastError: string }> } };

export type AuditQueryVariables = Exact<{
  input: AdminAuditInput | null | undefined;
}>;


export type AuditQuery = { audit: { totalCount: number, pageInfo: { page: number, pageSize: number, totalCount: number, totalPages: number, hasNext: boolean, hasPrev: boolean }, items: Array<{ id: string, actorType: string, actorId: string, resourceType: string, resourceId: string, action: string, result: string, createdAt: string }> } };

export type CreateUserMutationVariables = Exact<{
  input: AdminCreateUserInput;
}>;


export type CreateUserMutation = { createUser: { userId: string, status: string, user: { id: string, username: string, role: string, status: string, clientCount: number, proxyCount: number, uploadBytes: number, downloadBytes: number, lastActivityAt: string, hasPasswordHash: boolean, createdAt: string, updatedAt: string } } };

export type DisableUserMutationVariables = Exact<{
  input: AdminUserIdInput;
}>;


export type DisableUserMutation = { disableUser: { userId: string, status: string, user: { id: string, username: string, role: string, status: string, clientCount: number, proxyCount: number, uploadBytes: number, downloadBytes: number, lastActivityAt: string, hasPasswordHash: boolean, createdAt: string, updatedAt: string } } };

export type SetUserPasswordMutationVariables = Exact<{
  input: AdminSetUserPasswordInput;
}>;


export type SetUserPasswordMutation = { setUserPassword: { userId: string, status: string, user: { id: string, username: string, role: string, status: string, clientCount: number, proxyCount: number, uploadBytes: number, downloadBytes: number, lastActivityAt: string, hasPasswordHash: boolean, createdAt: string, updatedAt: string } } };

export type CreateClientMutationVariables = Exact<{
  input: AdminCreateClientInput;
}>;


export type CreateClientMutation = { createClient: { clientId: string, credential: string, token: string, client: { id: string, userId: string, name: string, status: string, version: number, lastOnlineAt: string, lastOfflineAt: string, createdAt: string, updatedAt: string, runtime: { online: boolean, protocol: string, connectedAt: string, lastHeartbeat: string, configVersion: number, activeProxies: number, activeStreams: number, uploadBytes: number, downloadBytes: number, errorSummary: string }, managedProxies: Array<{ id: string, name: string, type: string, status: string, runtimeStatus: string, entryBindHost: string, entryHost: string, entryPort: number, targetHost: string, targetPort: number, activeTCPConnections: number }> } } };

export type CreateClientJoinMutationVariables = Exact<{
  input: AdminCreateClientJoinInput;
}>;


export type CreateClientJoinMutation = { createClientJoin: { clientId: string, credential: string, token: string, client: { id: string, userId: string, name: string, status: string, version: number, lastOnlineAt: string, lastOfflineAt: string, createdAt: string, updatedAt: string, runtime: { online: boolean, protocol: string, connectedAt: string, lastHeartbeat: string, configVersion: number, activeProxies: number, activeStreams: number, uploadBytes: number, downloadBytes: number, errorSummary: string }, managedProxies: Array<{ id: string, name: string, type: string, status: string, runtimeStatus: string, entryBindHost: string, entryHost: string, entryPort: number, targetHost: string, targetPort: number, activeTCPConnections: number }> } } };

export type RotateClientCredentialMutationVariables = Exact<{
  input: AdminUserIdInput;
}>;


export type RotateClientCredentialMutation = { rotateClientCredential: { clientId: string, credential: string, token: string, client: { id: string, userId: string, name: string, status: string, version: number, lastOnlineAt: string, lastOfflineAt: string, createdAt: string, updatedAt: string, runtime: { online: boolean, protocol: string, connectedAt: string, lastHeartbeat: string, configVersion: number, activeProxies: number, activeStreams: number, uploadBytes: number, downloadBytes: number, errorSummary: string }, managedProxies: Array<{ id: string, name: string, type: string, status: string, runtimeStatus: string, entryBindHost: string, entryHost: string, entryPort: number, targetHost: string, targetPort: number, activeTCPConnections: number }> } } };

export type DeleteClientMutationVariables = Exact<{
  input: AdminUserIdInput;
}>;


export type DeleteClientMutation = { deleteClient: { clientId: string } };

export type CreateProxyMutationVariables = Exact<{
  input: AdminCreateProxyInput;
}>;


export type CreateProxyMutation = { createProxy: { proxyId: string, status: string, proxy: { id: string, userId: string, clientId: string, name: string, type: string, status: string, description: string, runtimeStatus: string, activeTCPConnections: number, uploadBytes: number, downloadBytes: number, tcpErrorCount: number, udpErrorCount: number, httpErrorCount: number, createdAt: string, updatedAt: string, config: { entryBindHost: string, entryHost: string, entryPort: number, targetHost: string, targetPort: number, certFile: string, keyFile: string }, certificate: { proxyId: string, certificateId: string, host: string, status: string, notAfter: string, lastIssuedAt: string, lastRenewedAt: string, lastError: string } } } };

export type UpdateProxyMutationVariables = Exact<{
  input: AdminUpdateProxyInput;
}>;


export type UpdateProxyMutation = { updateProxy: { proxyId: string, status: string, proxy: { id: string, userId: string, clientId: string, name: string, type: string, status: string, description: string, runtimeStatus: string, activeTCPConnections: number, uploadBytes: number, downloadBytes: number, tcpErrorCount: number, udpErrorCount: number, httpErrorCount: number, createdAt: string, updatedAt: string, config: { entryBindHost: string, entryHost: string, entryPort: number, targetHost: string, targetPort: number, certFile: string, keyFile: string }, certificate: { proxyId: string, certificateId: string, host: string, status: string, notAfter: string, lastIssuedAt: string, lastRenewedAt: string, lastError: string } } } };

export type EnableProxyMutationVariables = Exact<{
  input: AdminUserIdInput;
}>;


export type EnableProxyMutation = { enableProxy: { proxyId: string, status: string, proxy: { id: string, userId: string, clientId: string, name: string, type: string, status: string, description: string, runtimeStatus: string, activeTCPConnections: number, uploadBytes: number, downloadBytes: number, tcpErrorCount: number, udpErrorCount: number, httpErrorCount: number, createdAt: string, updatedAt: string, config: { entryBindHost: string, entryHost: string, entryPort: number, targetHost: string, targetPort: number, certFile: string, keyFile: string }, certificate: { proxyId: string, certificateId: string, host: string, status: string, notAfter: string, lastIssuedAt: string, lastRenewedAt: string, lastError: string } } } };

export type DisableProxyMutationVariables = Exact<{
  input: AdminUserIdInput;
}>;


export type DisableProxyMutation = { disableProxy: { proxyId: string, status: string, proxy: { id: string, userId: string, clientId: string, name: string, type: string, status: string, description: string, runtimeStatus: string, activeTCPConnections: number, uploadBytes: number, downloadBytes: number, tcpErrorCount: number, udpErrorCount: number, httpErrorCount: number, createdAt: string, updatedAt: string, config: { entryBindHost: string, entryHost: string, entryPort: number, targetHost: string, targetPort: number, certFile: string, keyFile: string }, certificate: { proxyId: string, certificateId: string, host: string, status: string, notAfter: string, lastIssuedAt: string, lastRenewedAt: string, lastError: string } } } };

export type DeleteProxyMutationVariables = Exact<{
  input: AdminUserIdInput;
}>;


export type DeleteProxyMutation = { deleteProxy: { proxyId: string, status: string } };

export type IssueManagedCertificateMutationVariables = Exact<{
  input: AdminCertificateMutationInput;
}>;


export type IssueManagedCertificateMutation = { issueManagedCertificate: { proxyId: string, status: string, certificate: { proxyId: string, certificateId: string, host: string, status: string, notAfter: string, lastIssuedAt: string, lastRenewedAt: string, lastError: string } } };

export type RenewManagedCertificateMutationVariables = Exact<{
  input: AdminCertificateMutationInput;
}>;


export type RenewManagedCertificateMutation = { renewManagedCertificate: { proxyId: string, status: string, certificate: { proxyId: string, certificateId: string, host: string, status: string, notAfter: string, lastIssuedAt: string, lastRenewedAt: string, lastError: string } } };
