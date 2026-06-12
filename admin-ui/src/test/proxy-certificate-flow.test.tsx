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

describe('proxy form ⇄ certificate creation flow', () => {
  it('saves a draft and navigates to the certificate create link when clicking 创建证书 on the HTTPS create form', async () => {
    vi.stubGlobal('fetch', createProxyFetchMock([]));
    renderProxies(['/proxies']);

    await screen.findByRole('heading', { name: 'Proxies' });
    await userEvent.click(screen.getByRole('button', { name: 'Create proxy' }));

    const dialog = await screen.findByRole('dialog', { name: 'Create proxy' });
    // 切换到 HTTPS 以暴露证书选择控件与「创建证书」入口。
    await userEvent.selectOptions(within(dialog).getByLabelText('Type'), 'https');
    await userEvent.selectOptions(within(dialog).getByLabelText('User'), 'user-1');
    await userEvent.selectOptions(within(dialog).getByLabelText('Client'), 'client-1');
    await userEvent.type(within(dialog).getByLabelText('Name'), 'secure-web');
    await userEvent.type(within(dialog).getByLabelText('SNI domain'), 'app.example.com');

    expect(storedDraftIds()).toHaveLength(0);
    await userEvent.click(within(dialog).getByRole('button', { name: '创建证书' }));

    // 草稿已写入 sessionStorage。
    const draftIds = storedDraftIds();
    expect(draftIds).toHaveLength(1);

    // 导航目标是证书创建链接，携带 create=1 / draftId / host / returnTo。
    await waitFor(() => expect(screen.getByText('Certificates route stub')).toBeInTheDocument());
    const url = locationUrl();
    expect(url.startsWith('/certificates?')).toBe(true);
    const params = new URLSearchParams(url.slice(url.indexOf('?')));
    expect(params.get('create')).toBe('1');
    expect(params.get('draftId')).toBe(draftIds[0]);
    expect(params.get('host')).toBe('app.example.com');
    // returnTo 指回 proxies 创建流程，便于成功后返回。
    expect(params.get('returnTo')).toContain('/proxies');
    expect(params.get('returnTo')).toContain('create=1');
  });

  it('restores the saved draft and auto-selects the new certificate when returning with createdCertificateId + draftId', async () => {
    const newCert = makeCert({
      certificateId: 'cert-new',
      proxyId: 'proxy-pending',
      host: 'app.example.com',
      hostnames: ['app.example.com'],
      providerName: 'ACME DNS-01',
    });
    vi.stubGlobal('fetch', createProxyFetchMock([newCert]));

    // 预置一份与点击「创建证书」时一致的草稿快照。
    const draftId = saveProxyDraft({
      userId: 'user-1',
      clientId: 'client-1',
      name: 'secure-web',
      type: 'https',
      description: 'restored draft',
      entryBindHost: '',
      entryHost: 'app.example.com',
      entryPort: '8443',
      targetHost: '127.0.0.1',
      targetPort: '9000',
      certificateId: '',
    });

    renderProxies([`/proxies?create=1&createdCertificateId=cert-new&draftId=${draftId}`]);

    // 返回时自动打开创建对话框并完成草稿还原。
    const dialog = await screen.findByRole('dialog', { name: 'Create proxy' });

    // 先前字段被还原。
    expect(within(dialog).getByLabelText('Name')).toHaveValue('secure-web');
    expect(within(dialog).getByLabelText('Entry port')).toHaveValue('8443');
    expect(within(dialog).getByLabelText('SNI domain')).toHaveValue('app.example.com');
    expect(within(dialog).getByLabelText('Target host')).toHaveValue('127.0.0.1');
    expect(within(dialog).getByLabelText('Target port')).toHaveValue('9000');

    // 新证书被自动选中（证书选择控件显示新证书 id 为当前值）。
    await waitFor(() => expect(within(dialog).getByLabelText('证书')).toHaveValue('cert-new'));
    expect(within(dialog).getByRole('option', { name: /app\.example\.com/ })).toBeInTheDocument();

    // 没有降级提示横幅（草稿成功还原）。
    expect(within(dialog).queryByText(/表单草稿已失效/)).not.toBeInTheDocument();

    // 消费后清理：草稿与 query param 都被移除（避免刷新重复触发）。
    expect(storedDraftIds()).toHaveLength(0);
    await waitFor(() => {
      const params = new URLSearchParams(locationUrl().slice(locationUrl().indexOf('?')));
      expect(params.get('createdCertificateId')).toBeNull();
      expect(params.get('draftId')).toBeNull();
    });

    // 回归保护：清理 query param 触发的 effect 二次运行 + 后续用户交互都不得清空已还原表单。
    await userEvent.type(within(dialog).getByLabelText('Description'), '!');
    expect(within(dialog).getByLabelText('Name')).toHaveValue('secure-web');
    expect(within(dialog).getByLabelText('SNI domain')).toHaveValue('app.example.com');
    expect(within(dialog).getByLabelText('证书')).toHaveValue('cert-new');
    expect(within(dialog).getByLabelText('Type')).toHaveValue('https');
  });

  it('degrades safely with a banner when the draft is missing but still preselects the new certificate', async () => {
    const newCert = makeCert({
      certificateId: 'cert-orphan',
      proxyId: 'proxy-orphan',
      host: 'late.example.com',
      hostnames: ['late.example.com'],
    });
    vi.stubGlobal('fetch', createProxyFetchMock([newCert]));

    // 不预置草稿，draftId 指向一个不存在的草稿（模拟过期/丢失）。
    renderProxies(['/proxies?create=1&createdCertificateId=cert-orphan&draftId=missing-draft-id']);

    const dialog = await screen.findByRole('dialog', { name: 'Create proxy' });

    // 安全降级横幅出现（草稿丢失，但不丢失整个流程）。
    expect(within(dialog).getByText(/表单草稿已失效/)).toBeInTheDocument();

    // 类型回退为 https 以展示证书选择控件；草稿丢失后域名为空，
    // 证书选择控件因缺少 SNI 域名而禁用（提示先填域名），等待用户补填。
    expect(within(dialog).getByLabelText('Type')).toHaveValue('https');
    // 证书选择控件因缺少 SNI 域名而禁用（hint 提示先填域名）。
    const certSelect = dialog.querySelector('.field-stack select') as HTMLSelectElement;
    expect(certSelect).toBeTruthy();
    expect(certSelect).toBeDisabled();
    expect(within(dialog).getByText(/请先填写 SNI 域名以匹配可用证书/)).toBeInTheDocument();

    // 新建证书 ID 已被表单预选（写入 config.certificateId）。由于域名为空、选择控件禁用，
    // 它体现为「当前选择，已不可用」的占位项，待证书列表加载后出现，且 select 选中该证书。
    await waitFor(() =>
      expect(within(certSelect).getByRole('option', { name: /late\.example\.com.*已不可用/ })).toBeInTheDocument(),
    );
    expect(certSelect).toHaveValue('cert-orphan');

    // 草稿缺失时也不残留任何草稿。
    expect(storedDraftIds()).toHaveLength(0);
  });
});

// --- CertificateSelectField 兼容性过滤单测 ---
describe('CertificateSelectField compatibility filtering', () => {
  function renderSelect(props: { entryHost: string; proxyId?: string; value?: string; certificates: ManagedCertificate[] }) {
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
            <CertificateSelectField entryHost={props.entryHost} proxyId={props.proxyId} value={props.value ?? ''} onChange={() => {}} />
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
    expect(screen.getByText(/已绑定到其他 proxy/)).toBeInTheDocument();
    expect(screen.getByText(/当前不可服务/)).toBeInTheDocument();
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
});
