import adminGraphQLDocument from '../graphql/admin.graphql?raw';
import type {
  AuditQuery,
  AuditQueryVariables,
  CertificatesQuery,
  CertificatesQueryVariables,
  ClientQuery,
  ClientQueryVariables,
  ClientsQuery,
  ClientsQueryVariables,
  CreateClientJoinMutation,
  CreateClientJoinMutationVariables,
  CreateClientMutation,
  CreateClientMutationVariables,
  CreateProxyMutation,
  CreateProxyMutationVariables,
  CreateUserMutation,
  CreateUserMutationVariables,
  DashboardQuery,
  DeleteClientMutation,
  DeleteClientMutationVariables,
  DeleteProxyMutation,
  DeleteProxyMutationVariables,
  DisableProxyMutation,
  DisableProxyMutationVariables,
  DisableUserMutation,
  DisableUserMutationVariables,
  EnableProxyMutation,
  EnableProxyMutationVariables,
  IssueManagedCertificateMutation,
  IssueManagedCertificateMutationVariables,
  ProxiesQuery,
  ProxiesQueryVariables,
  ProxyEntryOptionsQuery,
  ProxyQuery,
  ProxyQueryVariables,
  RenewManagedCertificateMutation,
  RenewManagedCertificateMutationVariables,
  RotateClientCredentialMutation,
  RotateClientCredentialMutationVariables,
  SetUserPasswordMutation,
  SetUserPasswordMutationVariables,
  UpdateProxyMutation,
  UpdateProxyMutationVariables,
  UserQuery,
  UserQueryVariables,
  UsersQuery,
  UsersQueryVariables,
} from '../graphql/generated';
import { graphqlClient } from './api';
import type {
  AuditEvent,
  Client,
  DashboardSummary,
  ManagedCertificate,
  PageInfo,
  PageResult,
  ProxyEntryOptions,
  ProxyRecord,
  User,
} from './contracts';

export type ListInput<TFilter> = {
  page?: { page: number; pageSize: number };
  sort?: { field?: string; direction?: 'asc' | 'desc' };
  filter?: TFilter;
};

export type UserFilter = {
  query?: string;
  role?: string;
  status?: string;
};

export type ClientFilter = {
  query?: string;
  userId?: string;
  status?: string;
  online?: boolean;
};

export type ProxyFilter = {
  query?: string;
  userId?: string;
  clientId?: string;
  type?: string;
  status?: string;
};

export type CertificateFilter = {
  query?: string;
  status?: string;
};

export type AuditFilter = {
  query?: string;
  actorType?: string;
  actorId?: string;
  resourceType?: string;
  action?: string;
  result?: string;
};

type ProxyConfigInput = {
  entryBindHost?: string;
  entryHost?: string;
  entryPort?: number;
  targetHost?: string;
  targetPort?: number;
  certFile?: string;
  keyFile?: string;
};

export type ClientJoinInput = {
  userId: string;
  name: string;
  enrollmentUrl: string;
  serverAddress: string;
  serverTLSAddress?: string;
  serverName: string;
  serverCAFile?: string;
  ttlSeconds?: number;
};

type RequestOptions<TVariables> = {
  operationName: string;
  variables?: TVariables;
  mutation?: boolean;
  csrfToken?: string;
};

function request<TData, TVariables>({
  operationName,
  variables,
  mutation,
  csrfToken,
}: RequestOptions<TVariables>) {
  return graphqlClient.request<TData>({
    query: adminGraphQLDocument,
    operationName,
    variables,
    mutation,
    csrfToken,
  });
}

function cleanObject<T extends Record<string, unknown>>(value: T): T {
  return Object.fromEntries(
    Object.entries(value).filter(([, current]) => current !== '' && current !== undefined && current !== null),
  ) as T;
}

function normalizePageResult<T>(result: { items: T[]; totalCount: number; pageInfo: PageInfo }): PageResult<T> {
  return result;
}

export async function queryDashboard() {
  const data = await request<DashboardQuery, undefined>({ operationName: 'Dashboard' });
  return data.dashboard satisfies DashboardSummary;
}

export async function queryUsers(input: ListInput<UserFilter>) {
  const variables = { input: cleanObject(input) } satisfies UsersQueryVariables;
  const data = await request<UsersQuery, UsersQueryVariables>({ operationName: 'Users', variables });
  return normalizePageResult<User>(data.users);
}

export async function queryUser(id: string) {
  const variables = { id } satisfies UserQueryVariables;
  const data = await request<UserQuery, UserQueryVariables>({ operationName: 'User', variables });
  return data.user satisfies User;
}

export async function queryClients(input: ListInput<ClientFilter>) {
  const variables = { input: cleanObject(input) } satisfies ClientsQueryVariables;
  const data = await request<ClientsQuery, ClientsQueryVariables>({ operationName: 'Clients', variables });
  return normalizePageResult<Client>(data.clients);
}

export async function queryClient(id: string) {
  const variables = { id } satisfies ClientQueryVariables;
  const data = await request<ClientQuery, ClientQueryVariables>({ operationName: 'Client', variables });
  return data.client;
}

export async function queryProxies(input: ListInput<ProxyFilter>) {
  const variables = { input: cleanObject(input) } satisfies ProxiesQueryVariables;
  const data = await request<ProxiesQuery, ProxiesQueryVariables>({ operationName: 'Proxies', variables });
  return normalizePageResult<ProxyRecord>(data.proxies);
}

export async function queryProxy(id: string) {
  const variables = { id } satisfies ProxyQueryVariables;
  const data = await request<ProxyQuery, ProxyQueryVariables>({ operationName: 'Proxy', variables });
  return data.proxy satisfies ProxyRecord;
}

export async function queryProxyEntryOptions() {
  const data = await request<ProxyEntryOptionsQuery, undefined>({ operationName: 'ProxyEntryOptions' });
  return data.proxyEntryOptions satisfies ProxyEntryOptions;
}

export async function queryCertificates(input: ListInput<CertificateFilter>) {
  const variables = { input: cleanObject(input) } satisfies CertificatesQueryVariables;
  const data = await request<CertificatesQuery, CertificatesQueryVariables>({ operationName: 'Certificates', variables });
  return normalizePageResult<ManagedCertificate>(data.certificates);
}

export async function queryAudit(input: ListInput<AuditFilter>) {
  const variables = { input: cleanObject(input) } satisfies AuditQueryVariables;
  const data = await request<AuditQuery, AuditQueryVariables>({ operationName: 'Audit', variables });
  return normalizePageResult<AuditEvent>(data.audit);
}

export function mutateCreateUser(csrfToken: string, input: { username: string; password: string; role: string }) {
  const variables = { input } satisfies CreateUserMutationVariables;
  return request<CreateUserMutation, CreateUserMutationVariables>({
    operationName: 'CreateUser',
    variables,
    mutation: true,
    csrfToken,
  });
}

export function mutateDisableUser(csrfToken: string, id: string) {
  const variables = { input: { id } } satisfies DisableUserMutationVariables;
  return request<DisableUserMutation, DisableUserMutationVariables>({
    operationName: 'DisableUser',
    variables,
    mutation: true,
    csrfToken,
  });
}

export function mutateSetUserPassword(csrfToken: string, input: { id: string; password: string }) {
  const variables = { input } satisfies SetUserPasswordMutationVariables;
  return request<SetUserPasswordMutation, SetUserPasswordMutationVariables>({
    operationName: 'SetUserPassword',
    variables,
    mutation: true,
    csrfToken,
  });
}

export function mutateCreateClient(csrfToken: string, input: { userId: string; name: string; credential?: string }) {
  const variables = { input: cleanObject(input) } satisfies CreateClientMutationVariables;
  return request<CreateClientMutation, CreateClientMutationVariables>({
    operationName: 'CreateClient',
    variables,
    mutation: true,
    csrfToken,
  });
}

export function mutateCreateClientJoin(csrfToken: string, input: ClientJoinInput) {
  const variables = { input: cleanObject(input) } satisfies CreateClientJoinMutationVariables;
  return request<CreateClientJoinMutation, CreateClientJoinMutationVariables>({
    operationName: 'CreateClientJoin',
    variables,
    mutation: true,
    csrfToken,
  });
}

export function mutateRotateClientCredential(csrfToken: string, id: string) {
  const variables = { input: { id } } satisfies RotateClientCredentialMutationVariables;
  return request<RotateClientCredentialMutation, RotateClientCredentialMutationVariables>({
    operationName: 'RotateClientCredential',
    variables,
    mutation: true,
    csrfToken,
  });
}

export function mutateDeleteClient(csrfToken: string, id: string) {
  const variables = { input: { id } } satisfies DeleteClientMutationVariables;
  return request<DeleteClientMutation, DeleteClientMutationVariables>({
    operationName: 'DeleteClient',
    variables,
    mutation: true,
    csrfToken,
  });
}

export function mutateCreateProxy(
  csrfToken: string,
  input: {
    userId: string;
    clientId: string;
    name: string;
    type: string;
    description?: string;
    config: ProxyConfigInput;
  },
) {
  const variables = { input } satisfies CreateProxyMutationVariables;
  return request<CreateProxyMutation, CreateProxyMutationVariables>({
    operationName: 'CreateProxy',
    variables,
    mutation: true,
    csrfToken,
  });
}

export function mutateUpdateProxy(
  csrfToken: string,
  input: {
    id: string;
    type?: string;
    name: string;
    description?: string;
    config: ProxyConfigInput;
  },
) {
  const variables = { input } satisfies UpdateProxyMutationVariables;
  return request<UpdateProxyMutation, UpdateProxyMutationVariables>({
    operationName: 'UpdateProxy',
    variables,
    mutation: true,
    csrfToken,
  });
}

export function mutateEnableProxy(csrfToken: string, id: string) {
  const variables = { input: { id } } satisfies EnableProxyMutationVariables;
  return request<EnableProxyMutation, EnableProxyMutationVariables>({
    operationName: 'EnableProxy',
    variables,
    mutation: true,
    csrfToken,
  });
}

export function mutateDisableProxy(csrfToken: string, id: string) {
  const variables = { input: { id } } satisfies DisableProxyMutationVariables;
  return request<DisableProxyMutation, DisableProxyMutationVariables>({
    operationName: 'DisableProxy',
    variables,
    mutation: true,
    csrfToken,
  });
}

export function mutateDeleteProxy(csrfToken: string, id: string) {
  const variables = { input: { id } } satisfies DeleteProxyMutationVariables;
  return request<DeleteProxyMutation, DeleteProxyMutationVariables>({
    operationName: 'DeleteProxy',
    variables,
    mutation: true,
    csrfToken,
  });
}

export function mutateIssueCertificate(csrfToken: string, proxyId: string) {
  const variables = { input: { proxyId } } satisfies IssueManagedCertificateMutationVariables;
  return request<IssueManagedCertificateMutation, IssueManagedCertificateMutationVariables>({
    operationName: 'IssueManagedCertificate',
    variables,
    mutation: true,
    csrfToken,
  });
}

export function mutateRenewCertificate(csrfToken: string, proxyId: string) {
  const variables = { input: { proxyId } } satisfies RenewManagedCertificateMutationVariables;
  return request<RenewManagedCertificateMutation, RenewManagedCertificateMutationVariables>({
    operationName: 'RenewManagedCertificate',
    variables,
    mutation: true,
    csrfToken,
  });
}
