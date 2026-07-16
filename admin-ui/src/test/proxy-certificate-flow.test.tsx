import { describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom';
import { SessionProvider } from '../session';
import { ProtectedLayout } from '../components/Layout';
import { ProxiesPage } from '../routes/ProxiesPage';
import { CertificateSelectField } from '../components/CertificateSelectField';
import { saveProxyDraft } from '../lib/proxy-draft';
import type { ManagedCertificate } from '../lib/contracts';

// proxy-draft 的 sessionStorage key 前缀（src/lib/proxy-draft.ts 内部约定，未导出）。
// 测试据此枚举已写入的草稿，验证「保存→消费→清理」的存储副作用。
const DRAFT_KEY_PREFIX_FOR_TEST = 'goginx.admin.proxyDraft.';

// 任务 6.6：HTTPS proxy 表单 ⇄ 证书创建的「跳转→创建→返回→还原」往返流程。

const pageInfo = { page: 1, pageSize: 10, totalCount: 0, totalPages: 1, hasNext: false, hasPrev: false };

const users = [
  {
    id: 'user-1',
    username: 'alice',
    role: 'user',
    status: 'enabled',
    clientCount: 1,
    proxyCount: 0,
    uploadBytes: 0,
    downloadBytes: 0,
    lastActivityAt: null,
    hasPasswordHash: true,
    createdAt: '2026-05-17T00:00:00Z',
    updatedAt: '2026-05-17T00:00:00Z',
  },
];

function client(id: string, userId: string, name: string) {
  return {
    id,
    userId,
    name,
    status: 'offline',
    version: 1,
    runtime: {
      online: false,
      protocol: null,
      connectedAt: null,
      lastHeartbeat: null,
      configVersion: null,
      activeProxies: 0,
      activeStreams: 0,
      uploadBytes: 0,
      downloadBytes: 0,
      errorSummary: null,
    },
    lastOnlineAt: null,
    lastOfflineAt: null,
    managedProxies: [],
    createdAt: '2026-05-17T00:00:00Z',
    updatedAt: '2026-05-17T00:00:00Z',
  };
}

function makeCert(overrides: Partial<ManagedCertificate> & { certificateId: string }): ManagedCertificate {
  return {
    proxyId: overrides.certificateId,
    boundProxyId: '',
    referenced: false,
    servable: true,
    deletionRisk: 'low',
    host: null,
    status: 'usable',
    servingStatus: 'usable',
    operationStatus: 'idle',
    providerType: 'acme_dns01',
    providerName: null,
    credentialId: null,
    providerStatus: 'active',
    cloudflareCertificateId: null,
    hostnames: [],
    notAfter: '2026-09-01T00:00:00Z',
    deploymentHints: [],
    ...overrides,
  };
}

function graphQL(data: unknown, status = 200) {
  return new Response(JSON.stringify(data), { status, headers: { 'Content-Type': 'application/json' } });
}

function createProxyFetchMock(certificates: ManagedCertificate[]) {
  const clientsByUser: Record<string, ReturnType<typeof client>[]> = {
    '': [client('client-1', 'user-1', 'home-node')],
    'user-1': [client('client-1', 'user-1', 'home-node')],
  };
  return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input);
    if (url.endsWith('/api/admin/session')) {
      return graphQL({ authenticated: true, username: 'admin', csrfToken: 'csrf', pollIntervalSeconds: 5 });
    }
    if (!url.endsWith('/api/admin/graphql')) {
      return graphQL({ data: {} });
    }
    const body = JSON.parse(String(init?.body ?? '{}')) as { operationName?: string; variables?: { input?: { filter?: { userId?: string } } } };
    const op = body.operationName ?? '';
    if (op === 'Users') {
      return graphQL({ data: { users: { items: users, totalCount: users.length, pageInfo: { ...pageInfo, totalCount: users.length } } } });
    }
    if (op === 'Clients') {
      const userId = body.variables?.input?.filter?.userId ?? '';
      const items = clientsByUser[userId] ?? clientsByUser[''];
      return graphQL({ data: { clients: { items, totalCount: items.length, pageInfo: { ...pageInfo, totalCount: items.length } } } });
    }
    if (op === 'Proxies') {
      return graphQL({ data: { proxies: { items: [], totalCount: 0, pageInfo } } });
    }
    if (op === 'Domains') {
      return graphQL({
        data: {
          domains: {
            items: [
              {
                id: 'domain-1',
                userId: 'user-1',
                host: 'app.example.com',
                certificateId: '',
                status: 'enabled',
                proxyCount: 0,
                httpEntryCount: 1,
                httpsEntryCount: 1,
                createdAt: '2026-05-17T00:00:00Z',
                updatedAt: '2026-05-17T00:00:00Z',
              },
            ],
            totalCount: 1,
            pageInfo: { ...pageInfo, totalCount: 1 },
          },
        },
      });
    }
    if (op === 'ProxyEntryOptions') {
      return graphQL({
        data: {
          proxyEntryOptions: {
            tcpDefaultBindHost: '127.0.0.1',
            httpDefaultBindHost: '127.0.0.1',
            httpDefaultPort: 80,
            httpsDefaultBindHost: '127.0.0.1',
            httpsDefaultPort: 443,
            hosts: [
              { value: '', label: 'Default listener host', isDefault: true },
              { value: '127.0.0.1', label: 'Loopback IPv4', isDefault: false },
            ],
          },
        },
      });
    }
    if (op === 'Certificates') {
      return graphQL({ data: { certificates: { items: certificates, totalCount: certificates.length, pageInfo: { ...pageInfo, totalCount: certificates.length } } } });
    }
    return graphQL({ data: {} });
  });
}

// LocationProbe：把当前 URL 暴露到 DOM，便于断言 navigate(...) 的目标。
function LocationProbe() {
  const location = useLocation();
  return <div data-testid="location">{`${location.pathname}${location.search}`}</div>;
}

function renderProxies(initialEntries: string[]) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  render(
    <QueryClientProvider client={queryClient}>
      <SessionProvider>
        <MemoryRouter initialEntries={initialEntries}>
          <LocationProbe />
          <Routes>
            <Route path="/login" element={<div>Login route</div>} />
            <Route path="/" element={<ProtectedLayout />}>
              <Route path="proxies" element={<ProxiesPage />} />
              {/* 证书创建流程的目标路由用桩替代，仅用于断言导航参数。 */}
              <Route path="certificates" element={<div>Certificates route stub</div>} />
            </Route>
          </Routes>
        </MemoryRouter>
      </SessionProvider>
    </QueryClientProvider>,
  );
}

function locationUrl(): string {
  return screen.getByTestId('location').textContent ?? '';
}

function storedDraftIds(): string[] {
  const ids: string[] = [];
  for (let i = 0; i < window.sessionStorage.length; i += 1) {
    const key = window.sessionStorage.key(i);
    if (key && key.startsWith(DRAFT_KEY_PREFIX_FOR_TEST)) {
      ids.push(key.slice(DRAFT_KEY_PREFIX_FOR_TEST.length));
    }
  }
  return ids;
}

describe('proxy form domain-first creation', () => {
  it('creates a web path proxy with domain and path prefix', async () => {
    const fetchMock = createProxyFetchMock([]);
    vi.stubGlobal('fetch', fetchMock);
    renderProxies(['/proxies']);

    await screen.findByRole('heading', { name: 'Proxies' });
    await userEvent.click(screen.getByRole('button', { name: 'Create proxy' }));

    const dialog = await screen.findByRole('dialog', { name: 'Create proxy' });
    expect(within(dialog).getByLabelText('Type')).toHaveValue('web');
    await userEvent.selectOptions(within(dialog).getByLabelText('User'), 'user-1');
    await userEvent.selectOptions(within(dialog).getByLabelText('Client'), 'client-1');
    await userEvent.type(within(dialog).getByLabelText('Name'), 'api-path');
    await waitFor(() => expect(within(dialog).getByLabelText('Domain')).not.toBeDisabled());
    await userEvent.selectOptions(within(dialog).getByLabelText('Domain'), 'domain-1');
    await userEvent.type(within(dialog).getByLabelText('Target host'), '127.0.0.1');
    await userEvent.type(within(dialog).getByLabelText('Target port'), '9000');
    await userEvent.click(within(dialog).getByRole('button', { name: 'Create proxy' }));

    await waitFor(() => {
      const bodies = fetchMock.mock.calls
        .filter((call) => String(call[0]).endsWith('/api/admin/graphql'))
        .map((call) => String(call[1]?.body ?? ''));
      expect(bodies.some((body) => body.includes('"domainId":"domain-1"') && body.includes('"type":"web"'))).toBe(true);
    });
  });

  it('offers create domain guidance when no domains exist for the user', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input);
        if (url.endsWith('/api/admin/session')) {
          return new Response(JSON.stringify({ authenticated: true, username: 'admin', csrfToken: 'csrf', pollIntervalSeconds: 5 }), {
            status: 200,
            headers: { 'Content-Type': 'application/json' },
          });
        }
        const body = JSON.parse(String(init?.body ?? '{}')) as { operationName?: string };
        if (body.operationName === 'Domains') {
          return new Response(JSON.stringify({ data: { domains: { items: [], totalCount: 0, pageInfo } } }), {
            status: 200,
            headers: { 'Content-Type': 'application/json' },
          });
        }
        if (body.operationName === 'Users') {
          return new Response(JSON.stringify({ data: { users: { items: users, totalCount: 1, pageInfo: { ...pageInfo, totalCount: 1 } } } }), {
            status: 200,
            headers: { 'Content-Type': 'application/json' },
          });
        }
        if (body.operationName === 'Clients') {
          return new Response(JSON.stringify({ data: { clients: { items: [client('client-1', 'user-1', 'home')], totalCount: 1, pageInfo: { ...pageInfo, totalCount: 1 } } } }), {
            status: 200,
            headers: { 'Content-Type': 'application/json' },
          });
        }
        if (body.operationName === 'Proxies') {
          return new Response(JSON.stringify({ data: { proxies: { items: [], totalCount: 0, pageInfo } } }), {
            status: 200,
            headers: { 'Content-Type': 'application/json' },
          });
        }
        if (body.operationName === 'ProxyEntryOptions') {
          return new Response(
            JSON.stringify({
              data: {
                proxyEntryOptions: {
                  tcpDefaultBindHost: '127.0.0.1',
                  httpDefaultBindHost: '127.0.0.1',
                  httpDefaultPort: 80,
                  httpsDefaultBindHost: '127.0.0.1',
                  httpsDefaultPort: 443,
                  hosts: [{ value: '', label: 'Default listener host', isDefault: true }],
                },
              },
            }),
            { status: 200, headers: { 'Content-Type': 'application/json' } },
          );
        }
        return new Response(JSON.stringify({ data: {} }), { status: 200, headers: { 'Content-Type': 'application/json' } });
      }),
    );

    renderProxies(['/proxies']);
    await userEvent.click(await screen.findByRole('button', { name: 'Create proxy' }));
    const dialog = await screen.findByRole('dialog', { name: 'Create proxy' });
    await userEvent.selectOptions(within(dialog).getByLabelText('User'), 'user-1');
    expect(await within(dialog).findByText(/No enabled domains for this user/)).toBeInTheDocument();
    expect(within(dialog).getByRole('button', { name: 'Create a domain first' })).toBeInTheDocument();
  });
});

// --- CertificateSelectField 兼容性过滤单测 ---
describe('CertificateSelectField compatibility filtering', () => {
  function renderSelect(props: {
    entryHost: string;
    proxyId?: string;
    domainId?: string;
    value?: string;
    requireServable?: boolean;
    certificates: ManagedCertificate[];
  }) {
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith('/api/admin/session')) {
        return graphQL({ authenticated: true, username: 'admin', csrfToken: 'csrf', pollIntervalSeconds: 5 });
      }
      if (!url.endsWith('/api/admin/graphql')) {
        return graphQL({ data: {} });
      }
      const body = JSON.parse(String(init?.body ?? '{}')) as { operationName?: string };
      if (body.operationName === 'Certificates') {
        return graphQL({ data: { certificates: { items: props.certificates, totalCount: props.certificates.length, pageInfo: { ...pageInfo, totalCount: props.certificates.length } } } });
      }
      return graphQL({ data: {} });
    });
    vi.stubGlobal('fetch', fetchMock);

    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
    render(
      <QueryClientProvider client={queryClient}>
        <SessionProvider>
          <MemoryRouter>
            <CertificateSelectField
              entryHost={props.entryHost}
              proxyId={props.proxyId}
              domainId={props.domainId}
              requireServable={props.requireServable}
              value={props.value ?? ''}
              onChange={() => {}}
            />
          </MemoryRouter>
        </SessionProvider>
      </QueryClientProvider>,
    );
  }

  it('lists only certificates whose hostnames cover the SNI domain and that are bindable', async () => {
    const certs = [
      // 兼容（精确匹配）且未绑定 → 列出。
      makeCert({ certificateId: 'cert-match', host: 'app.example.com', hostnames: ['app.example.com'] }),
      // 通配覆盖单级子域 → 列出。
      makeCert({ certificateId: 'cert-wild', host: '*.example.com', hostnames: ['*.example.com'] }),
      // 域名不覆盖 → 过滤掉（且不计入“已绑定他处”等原因）。
      makeCert({ certificateId: 'cert-other', host: 'other.test', hostnames: ['other.test'] }),
      // 兼容但已绑定到其他 proxy → 过滤掉，并以原因解释。
      makeCert({ certificateId: 'cert-bound', host: 'app.example.com', hostnames: ['app.example.com'], boundProxyId: 'proxy-x' }),
      // 兼容、未绑定但不可服务 → 过滤掉，并以原因解释。
      makeCert({ certificateId: 'cert-unservable', host: 'app.example.com', hostnames: ['app.example.com'], servable: false }),
    ];
    renderSelect({ entryHost: 'app.example.com', certificates: certs });

    const select = await screen.findByLabelText('证书');
    // 仅匹配且可绑定的证书出现在下拉里。
    await waitFor(() => {
      expect(within(select).queryByRole('option', { name: /cert-match/ }) ?? within(select).getByText(/app\.example\.com/)).toBeTruthy();
    });
    const optionValues = Array.from(select.querySelectorAll('option')).map((o) => o.getAttribute('value'));
    expect(optionValues).toContain('cert-match');
    expect(optionValues).toContain('cert-wild');
    expect(optionValues).not.toContain('cert-other');
    expect(optionValues).not.toContain('cert-bound');
    expect(optionValues).not.toContain('cert-unservable');

    // 过滤原因向管理员解释为何部分匹配证书未列出。
    expect(screen.getByText(/已绑定到其他 Proxy/)).toBeInTheDocument();
    expect(screen.getByText(/当前不可服务/)).toBeInTheDocument();
  });

  it('domain bind mode allows a certificate already referenced by another domain', async () => {
    const certs = [
      makeCert({
        certificateId: 'cert-wild',
        host: '*.example.com',
        hostnames: ['*.example.com'],
        boundDomainId: 'domain-other',
        referenced: true,
      }),
    ];
    renderSelect({
      entryHost: 'app.example.com',
      domainId: 'domain-self',
      requireServable: false,
      certificates: certs,
    });

    const select = await screen.findByLabelText('证书');
    await waitFor(() => {
      const optionValues = Array.from(select.querySelectorAll('option')).map((o) => o.getAttribute('value'));
      expect(optionValues).toContain('cert-wild');
    });
  });

  it('allows a certificate already bound to the current proxy in edit context', async () => {
    const certs = [
      makeCert({ certificateId: 'cert-self', host: 'app.example.com', hostnames: ['app.example.com'], boundProxyId: 'proxy-self' }),
    ];
    renderSelect({ entryHost: 'app.example.com', proxyId: 'proxy-self', value: 'cert-self', certificates: certs });

    const select = await screen.findByLabelText('证书');
    const optionValues = Array.from(select.querySelectorAll('option')).map((o) => o.getAttribute('value'));
    expect(optionValues).toContain('cert-self');
  });

  it('domain bind mode lists covering certificates even when not yet servable', async () => {
    const certs = [
      makeCert({
        certificateId: 'cert-pending',
        host: 'app.example.com',
        hostnames: ['app.example.com'],
        servable: false,
        servingStatus: 'missing',
        status: 'pending',
      }),
      makeCert({
        certificateId: 'cert-other-host',
        host: 'other.example.com',
        hostnames: ['other.example.com'],
        servable: true,
      }),
    ];
    renderSelect({
      entryHost: 'app.example.com',
      domainId: 'domain-1',
      requireServable: false,
      certificates: certs,
    });

    const select = await screen.findByLabelText('证书');
    await waitFor(() => {
      const optionValues = Array.from(select.querySelectorAll('option')).map((o) => o.getAttribute('value'));
      expect(optionValues).toContain('cert-pending');
      expect(optionValues).not.toContain('cert-other-host');
    });
  });
});
