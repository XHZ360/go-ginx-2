import { graphqlClient } from './api';
import type {
  AuditEvent,
  Client,
  ClientDetail,
  DashboardSummary,
  ManagedCertificate,
  PageInfo,
  PageResult,
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

type UsersPayload = { users: PageResult<User> };
type ClientsPayload = { clients: PageResult<Client> };
type ProxiesPayload = { proxies: PageResult<ProxyRecord> };
type CertificatesPayload = { certificates: PageResult<ManagedCertificate> };
type AuditPayload = { audit: PageResult<AuditEvent> };
type ClientMutationPayload = { clientId: string; credential?: string | null; token?: string | null; client: ClientDetail };
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

const PAGE_INFO_FRAGMENT = `pageInfo { page pageSize totalCount totalPages hasNext hasPrev }`;

function cleanObject<T extends Record<string, unknown>>(value: T): T {
  return Object.fromEntries(
    Object.entries(value).filter(([, current]) => current !== '' && current !== undefined && current !== null),
  ) as T;
}

function normalizePageResult<T>(result: { items: T[]; totalCount: number; pageInfo: PageInfo }): PageResult<T> {
  return result;
}

export async function queryDashboard() {
  const data = await graphqlClient.request<{ dashboard: DashboardSummary }>({
    query: `query Dashboard {
      dashboard {
        onlineClientCount
        enabledProxyCount
        activeTCPConnectionCount
        cumulativeUploadBytes
        cumulativeDownloadBytes
        cumulativeTCPErrorCount
        cumulativeUDPErrorCount
        cumulativeHTTPErrorCount
      }
    }`,
  });
  return data.dashboard;
}

export async function queryUsers(input: ListInput<UserFilter>) {
  const data = await graphqlClient.request<UsersPayload>({
    query: `query Users($input: AdminUsersInput) {
      users(input: $input) {
        totalCount
        ${PAGE_INFO_FRAGMENT}
        items {
          id
          username
          role
          status
          clientCount
          proxyCount
          uploadBytes
          downloadBytes
          lastActivityAt
          hasPasswordHash
          createdAt
          updatedAt
        }
      }
    }`,
    variables: { input: cleanObject(input) },
  });
  return normalizePageResult(data.users);
}

export async function queryUser(id: string) {
  const data = await graphqlClient.request<{ user: User }>({
    query: `query User($id: String!) {
      user(id: $id) {
        id
        username
        role
        status
        clientCount
        proxyCount
        uploadBytes
        downloadBytes
        lastActivityAt
        hasPasswordHash
        createdAt
        updatedAt
      }
    }`,
    variables: { id },
  });
  return data.user;
}

export async function queryClients(input: ListInput<ClientFilter>) {
  const data = await graphqlClient.request<ClientsPayload>({
    query: `query Clients($input: AdminClientsInput) {
      clients(input: $input) {
        totalCount
        ${PAGE_INFO_FRAGMENT}
        items {
          id
          userId
          name
          status
          version
          runtime {
            online
            protocol
            connectedAt
            lastHeartbeat
            configVersion
            activeProxies
            activeStreams
            uploadBytes
            downloadBytes
            errorSummary
          }
          lastOnlineAt
          lastOfflineAt
          createdAt
          updatedAt
        }
      }
    }`,
    variables: { input: cleanObject(input) },
  });
  return normalizePageResult(data.clients);
}

export async function queryClient(id: string) {
  const data = await graphqlClient.request<{ client: ClientDetail }>({
    query: `query Client($id: String!) {
      client(id: $id) {
        id
        userId
        name
        status
        version
        runtime {
          online
          protocol
          connectedAt
          lastHeartbeat
          configVersion
          activeProxies
          activeStreams
          uploadBytes
          downloadBytes
          errorSummary
        }
        lastOnlineAt
        lastOfflineAt
        managedProxies {
          id
          name
          type
          status
          runtimeStatus
          entryHost
          entryPort
          targetHost
          targetPort
          activeTCPConnections
        }
        createdAt
        updatedAt
      }
    }`,
    variables: { id },
  });
  return data.client;
}

export async function queryProxies(input: ListInput<ProxyFilter>) {
  const data = await graphqlClient.request<ProxiesPayload>({
    query: `query Proxies($input: AdminProxiesInput) {
      proxies(input: $input) {
        totalCount
        ${PAGE_INFO_FRAGMENT}
        items {
          id
          userId
          clientId
          name
          type
          status
          description
          runtimeStatus
          activeTCPConnections
          uploadBytes
          downloadBytes
          tcpErrorCount
          udpErrorCount
          httpErrorCount
          config {
            entryHost
            entryPort
            targetHost
            targetPort
          }
          certificate {
            proxyId
            certificateId
            host
            status
            notAfter
            lastIssuedAt
            lastRenewedAt
            lastError
          }
          createdAt
          updatedAt
        }
      }
    }`,
    variables: { input: cleanObject(input) },
  });
  return normalizePageResult(data.proxies);
}

export async function queryProxy(id: string) {
  const data = await graphqlClient.request<{ proxy: ProxyRecord }>({
    query: `query Proxy($id: String!) {
      proxy(id: $id) {
        id
        userId
        clientId
        name
        type
        status
        description
        runtimeStatus
        activeTCPConnections
        uploadBytes
        downloadBytes
        tcpErrorCount
        udpErrorCount
        httpErrorCount
        config {
          entryHost
          entryPort
          targetHost
          targetPort
        }
        certificate {
          proxyId
          certificateId
          host
          status
          notAfter
          lastIssuedAt
          lastRenewedAt
          lastError
        }
        createdAt
        updatedAt
      }
    }`,
    variables: { id },
  });
  return data.proxy;
}

export async function queryCertificates(input: ListInput<CertificateFilter>) {
  const data = await graphqlClient.request<CertificatesPayload>({
    query: `query Certificates($input: AdminCertificatesInput) {
      certificates(input: $input) {
        totalCount
        ${PAGE_INFO_FRAGMENT}
        items {
          proxyId
          certificateId
          host
          status
          notAfter
          lastIssuedAt
          lastRenewedAt
          lastError
        }
      }
    }`,
    variables: { input: cleanObject(input) },
  });
  return normalizePageResult(data.certificates);
}

export async function queryAudit(input: ListInput<AuditFilter>) {
  const data = await graphqlClient.request<AuditPayload>({
    query: `query Audit($input: AdminAuditInput) {
      audit(input: $input) {
        totalCount
        ${PAGE_INFO_FRAGMENT}
        items {
          id
          actorType
          actorId
          resourceType
          resourceId
          action
          result
          createdAt
        }
      }
    }`,
    variables: { input: cleanObject(input) },
  });
  return normalizePageResult(data.audit);
}

export function mutateCreateUser(csrfToken: string, input: { username: string; password: string; role: string }) {
  return graphqlClient.request<{ createUser: { userId: string; status: string; user: User } }>({
    query: `mutation CreateUser($input: AdminCreateUserInput!) {
      createUser(input: $input) {
        userId
        status
        user {
          id
          username
          role
          status
          clientCount
          proxyCount
          uploadBytes
          downloadBytes
          lastActivityAt
          hasPasswordHash
          createdAt
          updatedAt
        }
      }
    }`,
    variables: { input },
    mutation: true,
    csrfToken,
  });
}

export function mutateDisableUser(csrfToken: string, id: string) {
  return graphqlClient.request<{ disableUser: { userId: string; status: string; user: User } }>({
    query: `mutation DisableUser($input: AdminUserIDInput!) {
      disableUser(input: $input) {
        userId
        status
        user { id status updatedAt username role clientCount proxyCount uploadBytes downloadBytes lastActivityAt hasPasswordHash createdAt updatedAt }
      }
    }`,
    variables: { input: { id } },
    mutation: true,
    csrfToken,
  });
}

export function mutateSetUserPassword(csrfToken: string, input: { id: string; password: string }) {
  return graphqlClient.request<{ setUserPassword: { userId: string; status: string; user: User } }>({
    query: `mutation SetUserPassword($input: AdminSetUserPasswordInput!) {
      setUserPassword(input: $input) {
        userId
        status
        user { id status updatedAt username role clientCount proxyCount uploadBytes downloadBytes lastActivityAt hasPasswordHash createdAt updatedAt }
      }
    }`,
    variables: { input },
    mutation: true,
    csrfToken,
  });
}

export function mutateCreateClient(csrfToken: string, input: { userId: string; name: string; credential?: string }) {
  return graphqlClient.request<{ createClient: ClientMutationPayload }>({
    query: `mutation CreateClient($input: AdminCreateClientInput!) {
      createClient(input: $input) {
        clientId
        credential
        client {
          id
          userId
          name
          status
          version
          runtime {
            online
            protocol
            connectedAt
            lastHeartbeat
            configVersion
            activeProxies
            activeStreams
            uploadBytes
            downloadBytes
            errorSummary
          }
          lastOnlineAt
          lastOfflineAt
          managedProxies {
            id
            name
            type
            status
            runtimeStatus
            entryHost
            entryPort
            targetHost
            targetPort
            activeTCPConnections
          }
          createdAt
          updatedAt
        }
      }
    }`,
    variables: { input: cleanObject(input) },
    mutation: true,
    csrfToken,
  });
}

export function mutateCreateClientJoin(csrfToken: string, input: ClientJoinInput) {
  return graphqlClient.request<{ createClientJoin: ClientMutationPayload }>({
    query: `mutation CreateClientJoin($input: AdminCreateClientJoinInput!) {
      createClientJoin(input: $input) {
        clientId
        token
        client {
          id
          userId
          name
          status
          version
          runtime {
            online
            protocol
            connectedAt
            lastHeartbeat
            configVersion
            activeProxies
            activeStreams
            uploadBytes
            downloadBytes
            errorSummary
          }
          lastOnlineAt
          lastOfflineAt
          managedProxies {
            id
            name
            type
            status
            runtimeStatus
            entryHost
            entryPort
            targetHost
            targetPort
            activeTCPConnections
          }
          createdAt
          updatedAt
        }
      }
    }`,
    variables: { input: cleanObject(input) },
    mutation: true,
    csrfToken,
  });
}

export function mutateRotateClientCredential(csrfToken: string, id: string) {
  return graphqlClient.request<{ rotateClientCredential: ClientMutationPayload }>({
    query: `mutation RotateClientCredential($input: AdminUserIDInput!) {
      rotateClientCredential(input: $input) {
        clientId
        credential
        client {
          id
          userId
          name
          status
          version
          runtime {
            online
            protocol
            connectedAt
            lastHeartbeat
            configVersion
            activeProxies
            activeStreams
            uploadBytes
            downloadBytes
            errorSummary
          }
          lastOnlineAt
          lastOfflineAt
          managedProxies {
            id
            name
            type
            status
            runtimeStatus
            entryHost
            entryPort
            targetHost
            targetPort
            activeTCPConnections
          }
          createdAt
          updatedAt
        }
      }
    }`,
    variables: { input: { id } },
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
    config: {
      entryHost?: string;
      entryPort?: number;
      targetHost?: string;
      targetPort?: number;
    };
  },
) {
  return graphqlClient.request<{ createProxy: { proxyId: string; status: string; proxy: ProxyRecord } }>({
    query: `mutation CreateProxy($input: AdminCreateProxyInput!) {
      createProxy(input: $input) {
        proxyId
        status
        proxy { id name type status runtimeStatus userId clientId description activeTCPConnections uploadBytes downloadBytes tcpErrorCount udpErrorCount httpErrorCount config { entryHost entryPort targetHost targetPort } certificate { proxyId certificateId host status notAfter lastIssuedAt lastRenewedAt lastError } createdAt updatedAt }
      }
    }`,
    variables: { input },
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
    config: {
      entryHost?: string;
      entryPort?: number;
      targetHost?: string;
      targetPort?: number;
    };
  },
) {
  return graphqlClient.request<{ updateProxy: { proxyId: string; status: string; proxy: ProxyRecord } }>({
    query: `mutation UpdateProxy($input: AdminUpdateProxyInput!) {
      updateProxy(input: $input) {
        proxyId
        status
        proxy { id name type status runtimeStatus userId clientId description activeTCPConnections uploadBytes downloadBytes tcpErrorCount udpErrorCount httpErrorCount config { entryHost entryPort targetHost targetPort } certificate { proxyId certificateId host status notAfter lastIssuedAt lastRenewedAt lastError } createdAt updatedAt }
      }
    }`,
    variables: { input },
    mutation: true,
    csrfToken,
  });
}

export function mutateEnableProxy(csrfToken: string, id: string) {
  return graphqlClient.request<{ enableProxy: { proxyId: string; status: string; proxy: ProxyRecord } }>({
    query: `mutation EnableProxy($input: AdminUserIDInput!) {
      enableProxy(input: $input) {
        proxyId
        status
        proxy { id status runtimeStatus name type userId clientId description activeTCPConnections uploadBytes downloadBytes tcpErrorCount udpErrorCount httpErrorCount config { entryHost entryPort targetHost targetPort } certificate { proxyId certificateId host status notAfter lastIssuedAt lastRenewedAt lastError } createdAt updatedAt }
      }
    }`,
    variables: { input: { id } },
    mutation: true,
    csrfToken,
  });
}

export function mutateDisableProxy(csrfToken: string, id: string) {
  return graphqlClient.request<{ disableProxy: { proxyId: string; status: string; proxy: ProxyRecord } }>({
    query: `mutation DisableProxy($input: AdminUserIDInput!) {
      disableProxy(input: $input) {
        proxyId
        status
        proxy { id status runtimeStatus name type userId clientId description activeTCPConnections uploadBytes downloadBytes tcpErrorCount udpErrorCount httpErrorCount config { entryHost entryPort targetHost targetPort } certificate { proxyId certificateId host status notAfter lastIssuedAt lastRenewedAt lastError } createdAt updatedAt }
      }
    }`,
    variables: { input: { id } },
    mutation: true,
    csrfToken,
  });
}

export function mutateDeleteProxy(csrfToken: string, id: string) {
  return graphqlClient.request<{ deleteProxy: { proxyId: string; status: string } }>({
    query: `mutation DeleteProxy($input: AdminUserIDInput!) {
      deleteProxy(input: $input) {
        proxyId
        status
      }
    }`,
    variables: { input: { id } },
    mutation: true,
    csrfToken,
  });
}

export function mutateIssueCertificate(csrfToken: string, proxyId: string) {
  return graphqlClient.request<{ issueManagedCertificate: { proxyId: string; status: string; certificate: ManagedCertificate } }>({
    query: `mutation IssueManagedCertificate($input: AdminCertificateMutationInput!) {
      issueManagedCertificate(input: $input) {
        proxyId
        status
        certificate {
          proxyId
          certificateId
          host
          status
          notAfter
          lastIssuedAt
          lastRenewedAt
          lastError
        }
      }
    }`,
    variables: { input: { proxyId } },
    mutation: true,
    csrfToken,
  });
}

export function mutateRenewCertificate(csrfToken: string, proxyId: string) {
  return graphqlClient.request<{ renewManagedCertificate: { proxyId: string; status: string; certificate: ManagedCertificate } }>({
    query: `mutation RenewManagedCertificate($input: AdminCertificateMutationInput!) {
      renewManagedCertificate(input: $input) {
        proxyId
        status
        certificate {
          proxyId
          certificateId
          host
          status
          notAfter
          lastIssuedAt
          lastRenewedAt
          lastError
        }
      }
    }`,
    variables: { input: { proxyId } },
    mutation: true,
    csrfToken,
  });
}
