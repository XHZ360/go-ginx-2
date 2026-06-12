import { describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { SessionProvider } from '../session';
import { ProtectedLayout } from '../components/Layout';
import { CertificatesPage } from '../routes/CertificatesPage';

// Phase 4 重写后的 Certificates 页测试。
// 页面以「证书」为中心：每行展示 certificateId / host / provider / hostnames / 绑定状态 /
// serving·operation·provider 三个维度状态，并按 provider 类型给出不同的生命周期动作。

const pageInfo = { page: 1, pageSize: 10, totalCount: 0, totalPages: 1, hasNext: false, hasPrev: false };

// ManagedCertificateFields 的完整形状（与 admin.graphql 片段一致）。缺省值便于各用例只覆盖关心的字段。
type CertSeed = {
  proxyId: string;
  certificateId?: string | null;
  boundProxyId?: string | null;
  referenced?: boolean | null;
  servable?: boolean | null;
  deletionRisk?: string | null;
  host?: string | null;
  status?: string | null;
  servingStatus?: string | null;
  operationStatus?: string | null;
  providerType?: string | null;
  providerName?: string | null;
  credentialId?: string | null;
  providerStatus?: string | null;
  cloudflareCertificateId?: string | null;
  hostnames?: string[];
  requestType?: string | null;
  requestedValidity?: number | null;
  lastSyncedAt?: string | null;
  deploymentHints?: string[];
  notAfter?: string | null;
  lastIssuedAt?: string | null;
  lastRenewedAt?: string | null;
  lastCheckedAt?: string | null;
  lastAttemptedAt?: string | null;
  nextAttemptAt?: string | null;
  failureCount?: number | null;
  fingerprint?: string | null;
  lastError?: string | null;
};

function makeCertificate(overrides: Partial<CertSeed> & { certificateId: string; proxyId: string }): CertSeed {
  return {
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
    requestType: null,
    requestedValidity: null,
    lastSyncedAt: null,
    deploymentHints: [],
    notAfter: '2026-09-01T00:00:00Z',
    lastIssuedAt: '2026-05-01T00:00:00Z',
    lastRenewedAt: null,
    lastCheckedAt: '2026-06-01T00:00:00Z',
    lastAttemptedAt: null,
    nextAttemptAt: null,
    failureCount: 0,
    fingerprint: null,
    lastError: null,
    ...overrides,
  };
}

function graphQL(data: unknown, status = 200) {
  return new Response(JSON.stringify(data), { status, headers: { 'Content-Type': 'application/json' } });
}

type ParsedBody = {
  operationName?: string;
  variables?: {
    input?: {
      filter?: { status?: string };
      certificateId?: string;
      confirmHost?: string;
      confirmCertificateId?: string;
      proxyId?: string;
      providerType?: string;
      credentialId?: string;
      requestType?: string;
      requestedValidity?: number;
      host?: string;
      cloudflareCertificateId?: string;
    };
  };
};

type FetchOptions = {
  certificates: CertSeed[];
  credentials?: unknown[];
  // 后端 status 过滤器只覆盖 serving 维度（与生产实现一致）。
  serverStatusDimension?: 'servingStatus' | 'operationStatus' | 'providerStatus';
  // DeleteCertificate 的固定响应（默认成功，无受影响代理）。
  deleteResponse?: unknown;
  requests?: ParsedBody[];
};

function createFetchMock(options: FetchOptions) {
  const dimension = options.serverStatusDimension ?? 'servingStatus';
  return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input);
    if (url.endsWith('/api/admin/session')) {
      return graphQL({ authenticated: true, username: 'admin', csrfToken: 'csrf', pollIntervalSeconds: 5 });
    }
    if (!url.endsWith('/api/admin/graphql')) {
      return graphQL({ data: {} });
    }
    const body = JSON.parse(String(init?.body ?? '{}')) as ParsedBody;
    options.requests?.push(body);

    if (body.operationName === 'Certificates') {
      const status = body.variables?.input?.filter?.status ?? '';
      const items = status
        ? options.certificates.filter((cert) => (cert[dimension] ?? '') === status)
        : options.certificates;
      return graphQL({ data: { certificates: { items, totalCount: items.length, pageInfo: { ...pageInfo, totalCount: items.length } } } });
    }
    if (body.operationName === 'ProviderCredentials') {
      const items = options.credentials ?? [];
      return graphQL({ data: { providerCredentials: { items, totalCount: items.length, pageInfo: { ...pageInfo, totalCount: items.length } } } });
    }
    if (body.operationName === 'DeleteCertificate') {
      return graphQL(options.deleteResponse ?? { data: { deleteCertificate: { certificateId: body.variables?.input?.certificateId ?? '', affectedProxyIds: [], requiredConfirm: false } } });
    }
    if (
      body.operationName === 'IssueManagedCertificate' ||
      body.operationName === 'RenewManagedCertificate' ||
      body.operationName === 'RotateCloudflareOriginCertificate' ||
      body.operationName === 'SyncCloudflareOriginCertificate' ||
      body.operationName === 'RevokeCloudflareOriginCertificate' ||
      body.operationName === 'CreateCertificate'
    ) {
      const key = body.operationName.charAt(0).toLowerCase() + body.operationName.slice(1);
      return graphQL({ data: { [key]: { proxyId: body.variables?.input?.proxyId ?? '', status: 'valid', certificate: null } } });
    }
    return graphQL({ data: {} });
  });
}

const credential = {
  id: 'cred-1',
  name: 'Production Origin CA',
  providerType: 'cloudflare_origin_ca',
  scope: 'Zone SSL:Edit',
  tokenFingerprint: 'abcdef',
  status: 'verified',
  lastVerifiedAt: null,
  lastError: '',
  createdAt: '2026-06-01T00:00:00Z',
  updatedAt: '2026-06-01T00:00:00Z',
};

function renderCertificates(initialEntry = '/certificates') {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  render(
    <QueryClientProvider client={queryClient}>
      <SessionProvider>
        <MemoryRouter initialEntries={[initialEntry]}>
          <Routes>
            <Route path="/login" element={<div>Login route</div>} />
            <Route path="/" element={<ProtectedLayout />}>
              <Route path="certificates" element={<CertificatesPage />} />
            </Route>
          </Routes>
        </MemoryRouter>
      </SessionProvider>
    </QueryClientProvider>,
  );
}

describe('certificates page', () => {
  it('renders certificate rows with the new identity, binding, and dimension columns', async () => {
    const certificates = [
      makeCertificate({
        certificateId: 'cert-acme',
        proxyId: 'proxy-acme',
        host: 'app.example.com',
        boundProxyId: 'proxy-acme',
        referenced: true,
        hostnames: ['app.example.com', 'www.example.com'],
        operationStatus: 'renewal_failed',
        failureCount: 2,
        fingerprint: 'abcdef0123456789abcdef0123456789',
        lastError: 'dns failed',
      }),
    ];
    vi.stubGlobal('fetch', createFetchMock({ certificates }));

    renderCertificates();

    const row = await screen.findByRole('row', { name: /cert-acme/i });
    // 证书身份 + 绑定代理（host 既出现在身份单元的副标题，也出现在 hostnames 列）。
    expect(within(row).getByText('cert-acme')).toBeInTheDocument();
    expect(within(row).getAllByText('app.example.com').length).toBeGreaterThan(0);
    expect(within(row).getByText('www.example.com')).toBeInTheDocument();
    expect(within(row).getByText('proxy-acme')).toBeInTheDocument();
    // 三个状态维度互不冲突地展示
    expect(within(row).getByText('Usable')).toBeInTheDocument();
    expect(within(row).getByText('Renewal Failed')).toBeInTheDocument();
    expect(within(row).getByText('2')).toBeInTheDocument();
    expect(within(row).getByText('abcdef0123456789...')).toBeInTheDocument();
  });

  it('keeps the default origin credential selectable when credentials exist', async () => {
    vi.stubGlobal('fetch', createFetchMock({ certificates: [], credentials: [credential] }));

    renderCertificates();

    const select = await screen.findByLabelText('Origin credential');
    await screen.findByRole('option', { name: 'Production Origin CA' });
    expect(select).toHaveValue('');

    await userEvent.selectOptions(select, 'cred-1');
    expect(select).toHaveValue('cred-1');
    await userEvent.selectOptions(select, '');
    expect(select).toHaveValue('');
  });

  it('issues an origin CA certificate with the default credential (no credentialId) from an ACME row', async () => {
    const requests: ParsedBody[] = [];
    const certificates = [
      makeCertificate({ certificateId: 'cert-acme', proxyId: 'proxy-acme', host: 'acme.example.com', providerType: 'acme_dns01' }),
    ];
    vi.stubGlobal('fetch', createFetchMock({ certificates, credentials: [credential], requests }));
    renderCertificates();

    // 保持默认 Origin credential（空值）。
    const select = await screen.findByLabelText('Origin credential');
    expect(select).toHaveValue('');

    const row = await screen.findByRole('row', { name: /cert-acme/i });
    const issueOrigin = within(row).getByRole('button', { name: 'Issue Origin' });
    expect(issueOrigin).toBeEnabled();
    await userEvent.click(issueOrigin);
    await userEvent.click(await screen.findByRole('button', { name: 'Confirm' }));

    await waitFor(() => expect(requests.some((r) => r.operationName === 'IssueManagedCertificate')).toBe(true));
    const issueRequest = requests.find((r) => r.operationName === 'IssueManagedCertificate') as
      | { variables?: { input?: Record<string, unknown> } }
      | undefined;
    expect(issueRequest?.variables?.input).toMatchObject({ proxyId: 'proxy-acme', providerType: 'cloudflare_origin_ca', requestType: 'origin-ecc', requestedValidity: 5475 });
    expect(issueRequest?.variables?.input).not.toHaveProperty('credentialId');
  });

  // --- 任务 6.5：动作可用性 ---
  it('exposes ACME lifecycle actions for ACME certificates only', async () => {
    const certificates = [
      makeCertificate({ certificateId: 'cert-acme', proxyId: 'proxy-acme', host: 'acme.example.com', providerType: 'acme_dns01' }),
    ];
    vi.stubGlobal('fetch', createFetchMock({ certificates }));
    renderCertificates();

    const row = await screen.findByRole('row', { name: /cert-acme/i });
    expect(within(row).getByRole('button', { name: 'Issue' })).toBeEnabled();
    expect(within(row).getByRole('button', { name: 'Renew' })).toBeEnabled();
    // Origin CA 专属动作不出现在 ACME 行。
    expect(within(row).queryByRole('button', { name: 'Rotate' })).not.toBeInTheDocument();
    expect(within(row).queryByRole('button', { name: 'Sync' })).not.toBeInTheDocument();
    expect(within(row).queryByRole('button', { name: 'Revoke' })).not.toBeInTheDocument();
  });

  it('exposes Rotate/Sync/Revoke for origin CA certificates and disables Revoke without a cloudflare certificate id', async () => {
    const certificates = [
      makeCertificate({
        certificateId: 'cert-origin-ok',
        proxyId: 'proxy-origin-ok',
        host: 'edge.example.com',
        providerType: 'cloudflare_origin_ca',
        cloudflareCertificateId: 'cf-remote-1',
      }),
      makeCertificate({
        certificateId: 'cert-origin-noid',
        proxyId: 'proxy-origin-noid',
        host: 'edge2.example.com',
        providerType: 'cloudflare_origin_ca',
        cloudflareCertificateId: null,
      }),
    ];
    vi.stubGlobal('fetch', createFetchMock({ certificates }));
    renderCertificates();

    const okRow = await screen.findByRole('row', { name: /cert-origin-ok/i });
    expect(within(okRow).getByRole('button', { name: 'Rotate' })).toBeEnabled();
    expect(within(okRow).getByRole('button', { name: 'Sync' })).toBeEnabled();
    expect(within(okRow).getByRole('button', { name: 'Revoke' })).toBeEnabled();
    // ACME 动作不出现在 Origin CA 行。
    expect(within(okRow).queryByRole('button', { name: 'Issue' })).not.toBeInTheDocument();
    expect(within(okRow).queryByRole('button', { name: 'Renew' })).not.toBeInTheDocument();

    // 缺少 cloudflareCertificateId 时 Revoke 禁用，Rotate/Sync 仍可用。
    const noIdRow = await screen.findByRole('row', { name: /cert-origin-noid/i });
    expect(within(noIdRow).getByRole('button', { name: 'Revoke' })).toBeDisabled();
    expect(within(noIdRow).getByRole('button', { name: 'Rotate' })).toBeEnabled();
    expect(within(noIdRow).getByRole('button', { name: 'Sync' })).toBeEnabled();
  });

  it('sends certificateId for lifecycle actions on unbound certificates', async () => {
    const requests: ParsedBody[] = [];
    const certificates = [
      makeCertificate({
        certificateId: 'cert-unbound-acme',
        proxyId: '',
        host: 'free.example.com',
        providerType: 'acme_dns01',
      }),
      makeCertificate({
        certificateId: 'cert-unbound-origin',
        proxyId: '',
        host: 'origin-free.example.com',
        providerType: 'cloudflare_origin_ca',
        cloudflareCertificateId: 'cf-unbound-1',
      }),
    ];
    vi.stubGlobal('fetch', createFetchMock({ certificates, requests }));
    renderCertificates();

    const acmeRow = await screen.findByRole('row', { name: /cert-unbound-acme/i });
    await userEvent.click(within(acmeRow).getByRole('button', { name: 'Renew' }));
    await userEvent.click(await screen.findByRole('button', { name: 'Confirm' }));

    await waitFor(() => expect(requests.some((r) => r.operationName === 'RenewManagedCertificate')).toBe(true));
    const renewRequest = requests.find((r) => r.operationName === 'RenewManagedCertificate');
    expect(renewRequest?.variables?.input).toMatchObject({ certificateId: 'cert-unbound-acme' });
    expect(renewRequest?.variables?.input).not.toHaveProperty('proxyId');

    let originRow = await screen.findByRole('row', { name: /cert-unbound-origin/i });
    await userEvent.click(within(originRow).getByRole('button', { name: 'Rotate' }));
    await userEvent.click(await screen.findByRole('button', { name: 'Confirm' }));
    await waitFor(() => expect(requests.some((r) => r.operationName === 'RotateCloudflareOriginCertificate')).toBe(true));
    const rotateRequest = requests.find((r) => r.operationName === 'RotateCloudflareOriginCertificate');
    expect(rotateRequest?.variables?.input).toMatchObject({ certificateId: 'cert-unbound-origin' });
    expect(rotateRequest?.variables?.input).not.toHaveProperty('proxyId');

    originRow = await screen.findByRole('row', { name: /cert-unbound-origin/i });
    await userEvent.click(within(originRow).getByRole('button', { name: 'Sync' }));
    await waitFor(() => expect(requests.some((r) => r.operationName === 'SyncCloudflareOriginCertificate')).toBe(true));
    const syncRequest = requests.find((r) => r.operationName === 'SyncCloudflareOriginCertificate');
    expect(syncRequest?.variables?.input).toMatchObject({ certificateId: 'cert-unbound-origin' });
    expect(syncRequest?.variables?.input).not.toHaveProperty('proxyId');

    originRow = await screen.findByRole('row', { name: /cert-unbound-origin/i });
    await userEvent.click(within(originRow).getByRole('button', { name: 'Revoke' }));
    await userEvent.click(await screen.findByRole('button', { name: 'Confirm' }));
    await waitFor(() => expect(requests.some((r) => r.operationName === 'RevokeCloudflareOriginCertificate')).toBe(true));
    const revokeRequest = requests.find((r) => r.operationName === 'RevokeCloudflareOriginCertificate');
    expect(revokeRequest?.variables?.input).toMatchObject({
      certificateId: 'cert-unbound-origin',
      host: 'origin-free.example.com',
      cloudflareCertificateId: 'cf-unbound-1',
    });
    expect(revokeRequest?.variables?.input).not.toHaveProperty('proxyId');
  });

  it('exposes no lifecycle actions for file-backed certificates', async () => {
    const certificates = [
      makeCertificate({ certificateId: 'cert-file', proxyId: 'proxy-file', host: 'file.example.com', providerType: 'file' }),
    ];
    vi.stubGlobal('fetch', createFetchMock({ certificates }));
    renderCertificates();

    const row = await screen.findByRole('row', { name: /cert-file/i });
    expect(within(row).queryByRole('button', { name: 'Issue' })).not.toBeInTheDocument();
    expect(within(row).queryByRole('button', { name: 'Renew' })).not.toBeInTheDocument();
    expect(within(row).queryByRole('button', { name: 'Rotate' })).not.toBeInTheDocument();
    expect(within(row).queryByRole('button', { name: 'Sync' })).not.toBeInTheDocument();
    expect(within(row).queryByRole('button', { name: 'Revoke' })).not.toBeInTheDocument();
    // 显式占位的「无生命周期动作」按钮，禁用态。
    expect(within(row).getByRole('button', { name: 'No lifecycle actions' })).toBeDisabled();
  });

  // --- 任务 6.5：高风险删除（强确认） ---
  it('requires typing host or certificate id before a high-risk delete is submitted', async () => {
    const requests: ParsedBody[] = [];
    const certificates = [
      makeCertificate({
        certificateId: 'cert-strong',
        proxyId: 'proxy-strong',
        host: 'serving.example.com',
        boundProxyId: 'proxy-strong',
        referenced: true,
        deletionRisk: 'requires_strong_confirmation',
      }),
    ];
    vi.stubGlobal('fetch', createFetchMock({ certificates, requests }));
    renderCertificates();

    const row = await screen.findByRole('row', { name: /cert-strong/i });
    await userEvent.click(within(row).getByRole('button', { name: 'Delete' }));

    const dialog = await screen.findByRole('dialog', { name: '确认删除证书' });
    const confirm = within(dialog).getByRole('button', { name: '确认删除' });
    // 未输入时确认按钮禁用，提交被阻断。
    expect(confirm).toBeDisabled();

    // 输入错误值仍不放行。
    const field = within(dialog).getByLabelText('键入主机名或证书 ID 以确认');
    await userEvent.type(field, 'wrong-value');
    expect(confirm).toBeDisabled();
    expect(requests.some((r) => r.operationName === 'DeleteCertificate')).toBe(false);

    // 输入正确 host 后放行，删除请求携带 confirmHost。
    await userEvent.clear(field);
    await userEvent.type(field, 'serving.example.com');
    expect(confirm).toBeEnabled();
    await userEvent.click(confirm);

    await waitFor(() => expect(requests.some((r) => r.operationName === 'DeleteCertificate')).toBe(true));
    const deleteRequest = requests.find((r) => r.operationName === 'DeleteCertificate');
    expect(deleteRequest?.variables?.input).toMatchObject({ certificateId: 'cert-strong', confirmHost: 'serving.example.com' });
    expect(deleteRequest?.variables?.input).not.toHaveProperty('confirmCertificateId');
  });

  it('accepts the certificate id as the strong-confirmation token', async () => {
    const requests: ParsedBody[] = [];
    const certificates = [
      makeCertificate({
        certificateId: 'cert-strong-id',
        proxyId: 'proxy-strong-id',
        host: 'serving2.example.com',
        boundProxyId: 'proxy-strong-id',
        referenced: true,
        deletionRisk: 'requires_strong_confirmation',
      }),
    ];
    vi.stubGlobal('fetch', createFetchMock({ certificates, requests }));
    renderCertificates();

    const row = await screen.findByRole('row', { name: /cert-strong-id/i });
    await userEvent.click(within(row).getByRole('button', { name: 'Delete' }));

    const dialog = await screen.findByRole('dialog', { name: '确认删除证书' });
    await userEvent.type(within(dialog).getByLabelText('键入主机名或证书 ID 以确认'), 'cert-strong-id');
    await userEvent.click(within(dialog).getByRole('button', { name: '确认删除' }));

    await waitFor(() => expect(requests.some((r) => r.operationName === 'DeleteCertificate')).toBe(true));
    const deleteRequest = requests.find((r) => r.operationName === 'DeleteCertificate');
    expect(deleteRequest?.variables?.input).toMatchObject({ certificateId: 'cert-strong-id', confirmCertificateId: 'cert-strong-id' });
    expect(deleteRequest?.variables?.input).not.toHaveProperty('confirmHost');
  });

  // --- 任务 6.5：低风险删除（无二次确认、无强确认输入） ---
  it('deletes a low-risk certificate with one plain delete click', async () => {
    const requests: ParsedBody[] = [];
    const certificates = [
      makeCertificate({ certificateId: 'cert-low', proxyId: 'proxy-low', host: 'spare.example.com', deletionRisk: 'low', boundProxyId: '', referenced: false }),
    ];
    vi.stubGlobal('fetch', createFetchMock({ certificates, requests }));
    renderCertificates();

    const row = await screen.findByRole('row', { name: /cert-low/i });
    await userEvent.click(within(row).getByRole('button', { name: 'Delete' }));
    // 低风险不弹强确认对话框，也不弹 Popconfirm 二次确认。
    expect(screen.queryByRole('dialog', { name: '确认删除证书' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Confirm' })).not.toBeInTheDocument();

    await waitFor(() => expect(requests.some((r) => r.operationName === 'DeleteCertificate')).toBe(true));
    const deleteRequest = requests.find((r) => r.operationName === 'DeleteCertificate');
    expect(deleteRequest?.variables?.input).toMatchObject({ certificateId: 'cert-low' });
    expect(deleteRequest?.variables?.input).not.toHaveProperty('confirmHost');
    expect(deleteRequest?.variables?.input).not.toHaveProperty('confirmCertificateId');
  });

  // --- 任务 6.5：状态维度筛选互不串扰 ---
  it('filters serving and operation dimensions independently without cross-dimension matches', async () => {
    // 构造一个跨维度同名风险：A 的 serving=usable 且 operation=idle；
    // B 的 serving=missing 但 operation=usable（故意制造冲突值，验证不串扰）。
    const certificates = [
      makeCertificate({ certificateId: 'cert-A', proxyId: 'proxy-A', host: 'a.example.com', servingStatus: 'usable', operationStatus: 'idle' }),
      makeCertificate({ certificateId: 'cert-B', proxyId: 'proxy-B', host: 'b.example.com', servingStatus: 'missing', operationStatus: 'renewal_failed' }),
    ];
    vi.stubGlobal('fetch', createFetchMock({ certificates, serverStatusDimension: 'servingStatus' }));
    renderCertificates();

    await screen.findByRole('row', { name: /cert-A/i });
    await screen.findByRole('row', { name: /cert-B/i });

    // 服务端「Status」维度按 serving 过滤：missing 仅匹配 B。
    await userEvent.selectOptions(screen.getByLabelText('Status'), 'missing');
    await waitFor(() => expect(screen.queryByRole('row', { name: /cert-A/i })).not.toBeInTheDocument());
    expect(screen.getByRole('row', { name: /cert-B/i })).toBeInTheDocument();

    // 复位 Status，再按客户端「Operation status」维度过滤：renewal_failed 仅匹配 B（A 的 operation=idle 不被误匹配）。
    await userEvent.selectOptions(screen.getByLabelText('Status'), '');
    await screen.findByRole('row', { name: /cert-A/i });
    await userEvent.selectOptions(screen.getByLabelText('Operation status'), 'Renewal failed');
    await waitFor(() => expect(screen.queryByRole('row', { name: /cert-A/i })).not.toBeInTheDocument());
    expect(screen.getByRole('row', { name: /cert-B/i })).toBeInTheDocument();
  });

  it('filters by provider type without matching other provider dimensions', async () => {
    const certificates = [
      makeCertificate({ certificateId: 'cert-acme', proxyId: 'proxy-acme', host: 'acme.example.com', providerType: 'acme_dns01' }),
      makeCertificate({ certificateId: 'cert-origin', proxyId: 'proxy-origin', host: 'origin.example.com', providerType: 'cloudflare_origin_ca', cloudflareCertificateId: 'cf-1' }),
      makeCertificate({ certificateId: 'cert-file', proxyId: 'proxy-file', host: 'file.example.com', providerType: 'file' }),
    ];
    vi.stubGlobal('fetch', createFetchMock({ certificates }));
    renderCertificates();

    await screen.findByRole('row', { name: /cert-acme/i });
    await userEvent.selectOptions(screen.getByLabelText('Provider type'), 'cloudflare_origin_ca');

    await waitFor(() => expect(screen.queryByRole('row', { name: /cert-acme/i })).not.toBeInTheDocument());
    expect(screen.queryByRole('row', { name: /cert-file/i })).not.toBeInTheDocument();
    expect(screen.getByRole('row', { name: /cert-origin/i })).toBeInTheDocument();
  });

  // --- 任务 6.5：Origin CA 部署提示 ---
  it('renders deployment hints for an origin CA certificate', async () => {
    const certificates = [
      makeCertificate({
        certificateId: 'cert-hints',
        proxyId: 'proxy-hints',
        host: 'hints.example.com',
        providerType: 'cloudflare_origin_ca',
        cloudflareCertificateId: 'cf-hints',
        deploymentHints: ['请在 Cloudflare 启用 Full (strict) SSL 模式'],
      }),
      // 对照：ACME 证书即使带 hints 也不展示。
      makeCertificate({
        certificateId: 'cert-acme-nohint',
        proxyId: 'proxy-acme-nohint',
        host: 'plain.example.com',
        providerType: 'acme_dns01',
        deploymentHints: ['这条提示不应展示'],
      }),
    ];
    vi.stubGlobal('fetch', createFetchMock({ certificates }));
    renderCertificates();

    await screen.findByRole('row', { name: /cert-hints/i });
    expect(screen.getByText('请在 Cloudflare 启用 Full (strict) SSL 模式')).toBeInTheDocument();
    expect(screen.queryByText('这条提示不应展示')).not.toBeInTheDocument();
  });
});
