import { describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { SessionProvider } from '../session';
import { ProtectedLayout } from '../components/Layout';
import { CertificatesPage } from '../routes/CertificatesPage';

const pageInfo = { page: 1, pageSize: 10, totalCount: 0, totalPages: 1, hasNext: false, hasPrev: false };

const certificates = [
  {
    proxyId: 'proxy-serving',
    certificateId: 'cert-1',
    host: 'app.example.com',
    status: 'renewal_failed',
    servingStatus: 'usable',
    operationStatus: 'renewal_failed',
    notAfter: '2026-07-01T00:00:00Z',
    lastIssuedAt: '2026-05-01T00:00:00Z',
    lastRenewedAt: null,
    lastCheckedAt: '2026-06-01T00:00:00Z',
    lastAttemptedAt: '2026-06-01T00:05:00Z',
    nextAttemptAt: '2026-06-01T00:10:00Z',
    failureCount: 2,
    fingerprint: 'abcdef0123456789abcdef0123456789',
    lastError: 'dns failed',
  },
  {
    proxyId: 'proxy-missing',
    certificateId: null,
    host: 'missing.example.com',
    status: 'pending',
    servingStatus: 'missing',
    operationStatus: 'idle',
    notAfter: null,
    lastIssuedAt: null,
    lastRenewedAt: null,
    lastCheckedAt: '2026-06-01T00:00:00Z',
    lastAttemptedAt: null,
    nextAttemptAt: null,
    failureCount: 0,
    fingerprint: null,
    lastError: 'certificate active material is missing',
  },
];

function graphQL(data: unknown) {
  return new Response(JSON.stringify(data), { status: 200, headers: { 'Content-Type': 'application/json' } });
}

function renderCertificates() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  render(
    <QueryClientProvider client={queryClient}>
      <SessionProvider>
        <MemoryRouter initialEntries={['/certificates']}>
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
  it('shows serving and operation state and filters by certificate status', async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith('/api/admin/session')) {
        return graphQL({ authenticated: true, username: 'admin', csrfToken: 'csrf', pollIntervalSeconds: 5 });
      }
      if (!url.endsWith('/api/admin/graphql')) {
        return graphQL({ data: {} });
      }
      const body = JSON.parse(String(init?.body ?? '{}')) as { operationName?: string; variables?: { input?: { filter?: { status?: string } } } };
      if (body.operationName === 'Certificates') {
        const status = body.variables?.input?.filter?.status ?? '';
        const items = status ? certificates.filter((certificate) => certificate.servingStatus === status || certificate.operationStatus === status || certificate.status === status) : certificates;
        return graphQL({ data: { certificates: { items, totalCount: items.length, pageInfo: { ...pageInfo, totalCount: items.length } } } });
      }
      if (body.operationName === 'ProviderCredentials') {
        return graphQL({ data: { providerCredentials: { items: [], totalCount: 0, pageInfo } } });
      }
      return graphQL({ data: {} });
    });
    vi.stubGlobal('fetch', fetchMock);

    renderCertificates();

    const servingRow = await screen.findByRole('row', { name: /proxy-serving/i });
    expect(within(servingRow).getByText('Usable')).toBeInTheDocument();
    expect(within(servingRow).getByText('Renewal Failed')).toBeInTheDocument();
    expect(within(servingRow).getByText('2')).toBeInTheDocument();
    expect(within(servingRow).getByText('abcdef0123456789...')).toBeInTheDocument();
    expect(await screen.findByRole('row', { name: /proxy-missing/i })).toBeInTheDocument();

    await userEvent.selectOptions(screen.getByLabelText('Status'), 'missing');
    await waitFor(() => expect(screen.queryByRole('row', { name: /proxy-serving/i })).not.toBeInTheDocument());
    expect(screen.getByRole('row', { name: /proxy-missing/i })).toBeInTheDocument();
  });

  it('keeps default origin credential selectable when credentials exist', async () => {
    const requests: Array<{ operationName?: string; variables?: { input?: Record<string, unknown> } }> = [];
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith('/api/admin/session')) {
        return graphQL({ authenticated: true, username: 'admin', csrfToken: 'csrf', pollIntervalSeconds: 5 });
      }
      if (!url.endsWith('/api/admin/graphql')) {
        return graphQL({ data: {} });
      }
      const body = JSON.parse(String(init?.body ?? '{}')) as { operationName?: string; variables?: { input?: Record<string, unknown> } };
      requests.push(body);
      if (body.operationName === 'Certificates') {
        return graphQL({ data: { certificates: { items: certificates, totalCount: certificates.length, pageInfo: { ...pageInfo, totalCount: certificates.length } } } });
      }
      if (body.operationName === 'ProviderCredentials') {
        return graphQL({
          data: {
            providerCredentials: {
              items: [{ id: 'cred-1', name: 'Production Origin CA', providerType: 'cloudflare_origin_ca', scope: 'Zone SSL:Edit', tokenFingerprint: 'abcdef', status: 'verified', lastVerifiedAt: null, lastError: '', createdAt: '2026-06-01T00:00:00Z', updatedAt: '2026-06-01T00:00:00Z' }],
              totalCount: 1,
              pageInfo: { ...pageInfo, totalCount: 1 },
            },
          },
        });
      }
      if (body.operationName === 'IssueManagedCertificate') {
        return graphQL({ data: { issueManagedCertificate: { proxyId: body.variables?.input?.proxyId, status: 'valid' } } });
      }
      return graphQL({ data: {} });
    });
    vi.stubGlobal('fetch', fetchMock);

    renderCertificates();

    const select = await screen.findByLabelText('Origin credential');
    await screen.findByRole('option', { name: 'Production Origin CA' });
    expect(select).toHaveValue('');

    await userEvent.selectOptions(select, 'cred-1');
    expect(select).toHaveValue('cred-1');
    await userEvent.selectOptions(select, '');
    expect(select).toHaveValue('');

    const missingRow = await screen.findByRole('row', { name: /proxy-missing/i });
    await userEvent.click(within(missingRow).getByRole('button', { name: /Issue Origin/i }));
    await userEvent.click(await screen.findByRole('button', { name: /Confirm/i }));

    await waitFor(() => expect(requests.some((request) => request.operationName === 'IssueManagedCertificate')).toBe(true));
    const issueRequest = requests.find((request) => request.operationName === 'IssueManagedCertificate');
    expect(issueRequest?.variables?.input).toMatchObject({ proxyId: 'proxy-missing', providerType: 'cloudflare_origin_ca', requestType: 'origin-ecc', requestedValidity: 5475 });
    expect(issueRequest?.variables?.input).not.toHaveProperty('credentialId');
  });
});
