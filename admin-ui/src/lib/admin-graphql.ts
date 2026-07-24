import adminGraphQLDocument from '../graphql/admin.graphql?raw';
import type {
  AdminBindCertificateInput,
  AdminCreateCertificateInput,
  AdminDeleteCertificateInput,
  AdminUnbindCertificateInput,
  AuditQuery,
  AuditQueryVariables,
  BindCertificateMutation,
  BindCertificateMutationVariables,
  CertificatesQuery,
	CertificatesQueryVariables,
	CertificateProviderReadinessQuery,
  ClientQuery,
  ClientQueryVariables,
  ClientsQuery,
  ClientsQueryVariables,
  CreateCertificateMutation,
  CreateCertificateMutationVariables,
  CreateLocalProxyMutation,
  CreateLocalProxyMutationVariables,
  CreateClientJoinMutation,
  CreateClientJoinMutationVariables,
  CreateClientMutation,
  CreateClientMutationVariables,
  CreateProxyActivationLinkMutation,
  CreateProxyActivationLinkMutationVariables,
  CreateProxyMutation,
  CreateProxyMutationVariables,
  CreateProviderCredentialMutation,
  CreateProviderCredentialMutationVariables,
  CreateUserMutation,
  CreateUserMutationVariables,
  DashboardQuery,
  DeleteCertificateMutation,
  DeleteCertificateMutationVariables,
  DeleteClientMutation,
  DeleteClientMutationVariables,
  DeleteLocalProxyMutation,
  DeleteLocalProxyMutationVariables,
  DeleteProxyMutation,
  DeleteProxyMutationVariables,
  DeleteProviderCredentialMutation,
  DeleteProviderCredentialMutationVariables,
  DisableProxyAccessAuthMutation,
  DisableProxyAccessAuthMutationVariables,
  DisableProxyMutation,
  DisableProxyMutationVariables,
  DisableLocalProxyMutation,
  DisableLocalProxyMutationVariables,
  DisableProviderCredentialMutation,
  DisableProviderCredentialMutationVariables,
  DisableUserMutation,
  DisableUserMutationVariables,
  EnableProxyAccessAuthAndCreateActivationMutation,
  EnableProxyAccessAuthAndCreateActivationMutationVariables,
  EnableProxyMutation,
  EnableProxyMutationVariables,
  EnableLocalProxyMutation,
  EnableLocalProxyMutationVariables,
  IssueManagedCertificateMutation,
  IssueManagedCertificateMutationVariables,
  LocalTargetAllowlistQuery,
  ProviderCredentialsQuery,
  ProviderCredentialsQueryVariables,
  ProxiesQuery,
  ProxiesQueryVariables,
  ProxyEntryOptionsQuery,
  ProxyQuery,
  ProxyQueryVariables,
  RenewManagedCertificateMutation,
  RenewManagedCertificateMutationVariables,
  ReplaceLocalTargetAllowlistMutation,
  ReplaceLocalTargetAllowlistMutationVariables,
  RevokeAllProxyAccessMutation,
  RevokeAllProxyAccessMutationVariables,
  RevokeCloudflareOriginCertificateMutation,
  RevokeCloudflareOriginCertificateMutationVariables,
  RotateCloudflareOriginCertificateMutation,
  RotateCloudflareOriginCertificateMutationVariables,
  RotateClientCredentialMutation,
  RotateClientCredentialMutationVariables,
  SetUserPasswordMutation,
  SetUserPasswordMutationVariables,
  SyncCloudflareOriginCertificateMutation,
  SyncCloudflareOriginCertificateMutationVariables,
  UnbindCertificateMutation,
  UnbindCertificateMutationVariables,
  UpdateProviderCredentialMutation,
  UpdateProviderCredentialMutationVariables,
  UpdateLocalProxyMutation,
  UpdateLocalProxyMutationVariables,
  UpdateProxyMutation,
  UpdateProxyMutationVariables,
  UserQuery,
  UserQueryVariables,
  UsersQuery,
  UsersQueryVariables,
  VerifyProviderCredentialMutation,
  VerifyProviderCredentialMutationVariables,
} from '../graphql/generated';
import { graphqlClient } from './api';
import type {
  AuditEvent,
  Client,
  DashboardSummary,
  DomainEntry,
  DomainRecord,
	LocalProxyInput,
	LocalTargetAllowlist,
	LocalTargetAllowlistEntry,
	ManagedCertificate,
	CertificateProviderReadiness,
  PageInfo,
  PageResult,
  ProviderCredential,
  ProxyActivation,
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

export type CertificateMutationInput = {
  proxyId?: string;
  certificateId?: string;
  providerType?: string;
  credentialId?: string;
  requestType?: string;
  requestedValidity?: number;
};

export type CreateCertificateInput = AdminCreateCertificateInput;
export type DeleteCertificateInput = AdminDeleteCertificateInput;
export type BindCertificateInput = AdminBindCertificateInput;
export type UnbindCertificateInput = AdminUnbindCertificateInput;

export type ProviderCredentialInput = {
  id?: string;
  name: string;
  scope?: string;
  token?: string;
};

type ProxyConfigInput = {
  domainId?: string;
  pathPrefix?: string;
  stripPrefix?: boolean;
  upstreamPathPrefix?: string;
  entryBindHost?: string;
  entryHost?: string;
  entryPort?: number;
  targetHost?: string;
  targetPort?: number;
  certFile?: string;
  keyFile?: string;
  certificateId?: string;
};

export type DomainFilter = {
  query?: string;
  userId?: string;
  status?: string;
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

export async function queryDomains(input: ListInput<DomainFilter>) {
  const variables = { input: cleanObject(input) };
  const data = await request<{ domains: PageResult<DomainRecord> }, { input: ListInput<DomainFilter> }>({
    operationName: 'Domains',
    variables,
  });
  return normalizePageResult<DomainRecord>(data.domains);
}

export async function queryDomain(id: string) {
  const data = await request<{ domain: DomainRecord }, { id: string }>({
    operationName: 'Domain',
    variables: { id },
  });
  return data.domain;
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

export async function queryLocalTargetAllowlist() {
  const data = await request<LocalTargetAllowlistQuery, undefined>({ operationName: 'LocalTargetAllowlist' });
  return data.localTargetAllowlist satisfies LocalTargetAllowlist;
}

export function mutateReplaceLocalTargetAllowlist(csrfToken: string, entries: LocalTargetAllowlistEntry[]) {
  const variables = { input: { entries } } satisfies ReplaceLocalTargetAllowlistMutationVariables;
  return request<ReplaceLocalTargetAllowlistMutation, ReplaceLocalTargetAllowlistMutationVariables>({
    operationName: 'ReplaceLocalTargetAllowlist',
    variables,
    mutation: true,
    csrfToken,
  });
}

export async function queryCertificates(input: ListInput<CertificateFilter>) {
  const variables = { input: cleanObject(input) } satisfies CertificatesQueryVariables;
  const data = await request<CertificatesQuery, CertificatesQueryVariables>({ operationName: 'Certificates', variables });
  return normalizePageResult<ManagedCertificate>(data.certificates);
}

export async function queryCertificateProviderReadiness() {
  const data = await request<CertificateProviderReadinessQuery, undefined>({ operationName: 'CertificateProviderReadiness' });
  return data.certificateProviderReadiness satisfies CertificateProviderReadiness[];
}

export async function queryProviderCredentials(input: { page?: { page: number; pageSize: number } }) {
  const variables = { input: cleanObject(input) } satisfies ProviderCredentialsQueryVariables;
  const data = await request<ProviderCredentialsQuery, ProviderCredentialsQueryVariables>({ operationName: 'ProviderCredentials', variables });
  return normalizePageResult<ProviderCredential>(data.providerCredentials);
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

export function mutateCreateLocalProxy(csrfToken: string, input: LocalProxyInput) {
  const variables = { input: cleanObject(input) } satisfies CreateLocalProxyMutationVariables;
  return request<CreateLocalProxyMutation, CreateLocalProxyMutationVariables>({ operationName: 'CreateLocalProxy', variables, mutation: true, csrfToken });
}

export function mutateUpdateLocalProxy(csrfToken: string, input: LocalProxyInput & { id: string }) {
  const variables = { input: cleanObject(input) } satisfies UpdateLocalProxyMutationVariables;
  return request<UpdateLocalProxyMutation, UpdateLocalProxyMutationVariables>({ operationName: 'UpdateLocalProxy', variables, mutation: true, csrfToken });
}

export function mutateEnableLocalProxy(csrfToken: string, id: string) {
  const variables = { input: { id } } satisfies EnableLocalProxyMutationVariables;
  return request<EnableLocalProxyMutation, EnableLocalProxyMutationVariables>({ operationName: 'EnableLocalProxy', variables, mutation: true, csrfToken });
}

export function mutateDisableLocalProxy(csrfToken: string, id: string) {
  const variables = { input: { id } } satisfies DisableLocalProxyMutationVariables;
  return request<DisableLocalProxyMutation, DisableLocalProxyMutationVariables>({ operationName: 'DisableLocalProxy', variables, mutation: true, csrfToken });
}

export function mutateDeleteLocalProxy(csrfToken: string, id: string) {
  const variables = { input: { id } } satisfies DeleteLocalProxyMutationVariables;
  return request<DeleteLocalProxyMutation, DeleteLocalProxyMutationVariables>({ operationName: 'DeleteLocalProxy', variables, mutation: true, csrfToken });
}

export async function mutateCreateDomain(
  csrfToken: string,
  input: { userId: string; host: string; certificateId?: string },
) {
  const data = await request<{ createDomain: { domainId: string; status: string; domain: DomainRecord } }, { input: typeof input }>({
    operationName: 'CreateDomain',
    variables: { input: cleanObject(input) },
    mutation: true,
    csrfToken,
  });
  return data.createDomain.domain;
}

export async function mutateUpdateDomain(
  csrfToken: string,
  input: { id: string; host?: string; certificateId?: string },
) {
  const data = await request<{ updateDomain: { domain: DomainRecord } }, { input: typeof input }>({
    operationName: 'UpdateDomain',
    variables: { input: cleanObject(input) },
    mutation: true,
    csrfToken,
  });
  return data.updateDomain.domain;
}

export function mutateEnableDomain(csrfToken: string, id: string) {
  return request({ operationName: 'EnableDomain', variables: { input: { id } }, mutation: true, csrfToken });
}

export function mutateDisableDomain(csrfToken: string, id: string) {
  return request({ operationName: 'DisableDomain', variables: { input: { id } }, mutation: true, csrfToken });
}

export function mutateDeleteDomain(csrfToken: string, id: string) {
  return request({ operationName: 'DeleteDomain', variables: { input: { id } }, mutation: true, csrfToken });
}

export async function mutateCreateDomainEntry(
  csrfToken: string,
  input: { domainId: string; protocol: string; bindHost?: string; port: number },
) {
  const data = await request<{ createDomainEntry: { entry: DomainEntry } }, { input: typeof input }>({
    operationName: 'CreateDomainEntry',
    variables: { input: cleanObject(input) },
    mutation: true,
    csrfToken,
  });
  return data.createDomainEntry.entry;
}

export async function mutateUpdateDomainEntry(
  csrfToken: string,
  input: { id: string; bindHost?: string; port?: number; status?: string },
) {
  const data = await request<{ updateDomainEntry: { entry: DomainEntry } }, { input: typeof input }>({
    operationName: 'UpdateDomainEntry',
    variables: { input: cleanObject(input) },
    mutation: true,
    csrfToken,
  });
  return data.updateDomainEntry.entry;
}

export function mutateDeleteDomainEntry(csrfToken: string, id: string) {
  return request({ operationName: 'DeleteDomainEntry', variables: { input: { id } }, mutation: true, csrfToken });
}

export async function mutateBindDomainCertificate(
  csrfToken: string,
  input: { domainId: string; certificateId: string },
) {
  const data = await request<{ bindDomainCertificate: { domain: DomainRecord } }, { input: typeof input }>({
    operationName: 'BindDomainCertificate',
    variables: { input },
    mutation: true,
    csrfToken,
  });
  return data.bindDomainCertificate.domain;
}

export async function mutateUnbindDomainCertificate(csrfToken: string, id: string) {
  const data = await request<{ unbindDomainCertificate: { domain: DomainRecord } }, { input: { id: string } }>({
    operationName: 'UnbindDomainCertificate',
    variables: { input: { id } },
    mutation: true,
    csrfToken,
  });
  return data.unbindDomainCertificate.domain;
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

export async function mutateEnableProxyAccessAuthAndCreateActivation(csrfToken: string, id: string) {
  const variables = { input: { id } } satisfies EnableProxyAccessAuthAndCreateActivationMutationVariables;
  const data = await request<EnableProxyAccessAuthAndCreateActivationMutation, EnableProxyAccessAuthAndCreateActivationMutationVariables>({
    operationName: 'EnableProxyAccessAuthAndCreateActivation',
    variables,
    mutation: true,
    csrfToken,
  });
  return data.enableProxyAccessAuthAndCreateActivation satisfies ProxyActivation;
}

export async function mutateCreateProxyActivationLink(csrfToken: string, id: string) {
  const variables = { input: { id } } satisfies CreateProxyActivationLinkMutationVariables;
  const data = await request<CreateProxyActivationLinkMutation, CreateProxyActivationLinkMutationVariables>({
    operationName: 'CreateProxyActivationLink',
    variables,
    mutation: true,
    csrfToken,
  });
  return data.createProxyActivationLink satisfies ProxyActivation;
}

export function mutateRevokeAllProxyAccess(csrfToken: string, id: string) {
  const variables = { input: { id } } satisfies RevokeAllProxyAccessMutationVariables;
  return request<RevokeAllProxyAccessMutation, RevokeAllProxyAccessMutationVariables>({
    operationName: 'RevokeAllProxyAccess',
    variables,
    mutation: true,
    csrfToken,
  });
}

export function mutateDisableProxyAccessAuth(csrfToken: string, id: string) {
  const variables = { input: { id } } satisfies DisableProxyAccessAuthMutationVariables;
  return request<DisableProxyAccessAuthMutation, DisableProxyAccessAuthMutationVariables>({
    operationName: 'DisableProxyAccessAuth',
    variables,
    mutation: true,
    csrfToken,
  });
}

export function mutateIssueCertificate(csrfToken: string, input: CertificateMutationInput | string) {
  const variables = { input: typeof input === 'string' ? { proxyId: input } : cleanObject(input) } satisfies IssueManagedCertificateMutationVariables;
  return request<IssueManagedCertificateMutation, IssueManagedCertificateMutationVariables>({
    operationName: 'IssueManagedCertificate',
    variables,
    mutation: true,
    csrfToken,
  });
}

export function mutateRenewCertificate(csrfToken: string, input: CertificateMutationInput | string) {
  const variables = { input: typeof input === 'string' ? { proxyId: input } : cleanObject(input) } satisfies RenewManagedCertificateMutationVariables;
  return request<RenewManagedCertificateMutation, RenewManagedCertificateMutationVariables>({
    operationName: 'RenewManagedCertificate',
    variables,
    mutation: true,
    csrfToken,
  });
}

export function mutateRotateOriginCertificate(csrfToken: string, input: CertificateMutationInput | string) {
  const variables = { input: typeof input === 'string' ? { proxyId: input } : cleanObject(input) } satisfies RotateCloudflareOriginCertificateMutationVariables;
  return request<RotateCloudflareOriginCertificateMutation, RotateCloudflareOriginCertificateMutationVariables>({
    operationName: 'RotateCloudflareOriginCertificate',
    variables,
    mutation: true,
    csrfToken,
  });
}

export function mutateSyncOriginCertificate(csrfToken: string, input: CertificateMutationInput | string) {
  const variables = { input: typeof input === 'string' ? { proxyId: input } : cleanObject(input) } satisfies SyncCloudflareOriginCertificateMutationVariables;
  return request<SyncCloudflareOriginCertificateMutation, SyncCloudflareOriginCertificateMutationVariables>({
    operationName: 'SyncCloudflareOriginCertificate',
    variables,
    mutation: true,
    csrfToken,
  });
}

export function mutateRevokeOriginCertificate(
  csrfToken: string,
  input: { proxyId?: string; certificateId?: string; host: string; cloudflareCertificateId: string },
) {
  const variables = { input: cleanObject(input) } satisfies RevokeCloudflareOriginCertificateMutationVariables;
  return request<RevokeCloudflareOriginCertificateMutation, RevokeCloudflareOriginCertificateMutationVariables>({
    operationName: 'RevokeCloudflareOriginCertificate',
    variables,
    mutation: true,
    csrfToken,
  });
}

export function mutateCreateCertificate(csrfToken: string, input: CreateCertificateInput) {
  const variables = { input: cleanObject(input) } satisfies CreateCertificateMutationVariables;
  return request<CreateCertificateMutation, CreateCertificateMutationVariables>({
    operationName: 'CreateCertificate',
    variables,
    mutation: true,
    csrfToken,
  });
}

export function mutateDeleteCertificate(csrfToken: string, input: DeleteCertificateInput) {
  const variables = { input: cleanObject(input) } satisfies DeleteCertificateMutationVariables;
  return request<DeleteCertificateMutation, DeleteCertificateMutationVariables>({
    operationName: 'DeleteCertificate',
    variables,
    mutation: true,
    csrfToken,
  });
}

export function mutateBindCertificate(csrfToken: string, input: BindCertificateInput) {
  const variables = { input } satisfies BindCertificateMutationVariables;
  return request<BindCertificateMutation, BindCertificateMutationVariables>({
    operationName: 'BindCertificate',
    variables,
    mutation: true,
    csrfToken,
  });
}

export function mutateUnbindCertificate(csrfToken: string, input: UnbindCertificateInput) {
  const variables = { input } satisfies UnbindCertificateMutationVariables;
  return request<UnbindCertificateMutation, UnbindCertificateMutationVariables>({
    operationName: 'UnbindCertificate',
    variables,
    mutation: true,
    csrfToken,
  });
}

export function mutateCreateProviderCredential(csrfToken: string, input: ProviderCredentialInput) {
  const variables = { input: cleanObject(input) } satisfies CreateProviderCredentialMutationVariables;
  return request<CreateProviderCredentialMutation, CreateProviderCredentialMutationVariables>({
    operationName: 'CreateProviderCredential',
    variables,
    mutation: true,
    csrfToken,
  });
}

export function mutateUpdateProviderCredential(csrfToken: string, input: ProviderCredentialInput & { id: string }) {
  const variables = { input: cleanObject(input) } satisfies UpdateProviderCredentialMutationVariables;
  return request<UpdateProviderCredentialMutation, UpdateProviderCredentialMutationVariables>({
    operationName: 'UpdateProviderCredential',
    variables,
    mutation: true,
    csrfToken,
  });
}

export function mutateVerifyProviderCredential(csrfToken: string, id: string) {
  const variables = { input: { id } } satisfies VerifyProviderCredentialMutationVariables;
  return request<VerifyProviderCredentialMutation, VerifyProviderCredentialMutationVariables>({
    operationName: 'VerifyProviderCredential',
    variables,
    mutation: true,
    csrfToken,
  });
}

export function mutateDisableProviderCredential(csrfToken: string, id: string) {
  const variables = { input: { id } } satisfies DisableProviderCredentialMutationVariables;
  return request<DisableProviderCredentialMutation, DisableProviderCredentialMutationVariables>({
    operationName: 'DisableProviderCredential',
    variables,
    mutation: true,
    csrfToken,
  });
}

export function mutateDeleteProviderCredential(csrfToken: string, id: string) {
  const variables = { input: { id } } satisfies DeleteProviderCredentialMutationVariables;
  return request<DeleteProviderCredentialMutation, DeleteProviderCredentialMutationVariables>({
    operationName: 'DeleteProviderCredential',
    variables,
    mutation: true,
    csrfToken,
  });
}
