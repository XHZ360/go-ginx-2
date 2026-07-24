import { describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { ProtectedLayout } from '../components/Layout';
import { ClientDetailPage } from '../routes/ClientDetailPage';
import { ProxyDetailPage } from '../routes/ProxyDetailPage';
import { SessionProvider } from '../session';

function json(payload: unknown) {
  return new Response(JSON.stringify(payload), { status: 200, headers: { 'Content-Type': 'application/json' } });
}

function systemClient() {
  return {
    id: 'server-local',
    userId: 'server-local-system',
    name: 'Server Local',
    isSystem: true,
    status: 'offline',
    version: 0,
    runtime: { online: true, protocol: 'tcp_tls', connectedAt: '2026-07-24T00:00:00Z', lastHeartbeat: '2026-07-24T00:00:00Z', configVersion: 0, activeProxies: 0, activeStreams: 0, uploadBytes: 0, downloadBytes: 0, errorSummary: null },
    lastOnlineAt: null,
    lastOfflineAt: null,
    managedProxies: [],
    createdAt: '2026-07-24T00:00:00Z',
    updatedAt: '2026-07-24T00:00:00Z',
  };
}

function renderSystemClient(fetchMock: ReturnType<typeof vi.fn>) {
  vi.stubGlobal('fetch', fetchMock);
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  render(
    <QueryClientProvider client={queryClient}>
      <SessionProvider>
        <MemoryRouter initialEntries={['/clients/server-local']}>
          <Routes>
            <Route path="/login" element={<div>Login</div>} />
            <Route path="/" element={<ProtectedLayout />}>
              <Route path="clients/:id" element={<ClientDetailPage />} />
            </Route>
          </Routes>
        </MemoryRouter>
      </SessionProvider>
    </QueryClientProvider>,
  );
}

function renderSystemProxy(fetchMock: ReturnType<typeof vi.fn>) {
  vi.stubGlobal('fetch', fetchMock);
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  render(
    <QueryClientProvider client={queryClient}>
      <SessionProvider>
        <MemoryRouter initialEntries={['/proxies/local-1']}>
          <Routes>
            <Route path="/login" element={<div>Login</div>} />
            <Route path="/" element={<ProtectedLayout />}>
              <Route path="proxies/:id" element={<ProxyDetailPage />} />
            </Route>
          </Routes>
        </MemoryRouter>
      </SessionProvider>
    </QueryClientProvider>,
  );
}

describe('server-local client management', () => {
  it('uses only dedicated local management operations', async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith('/api/admin/session')) {
        return json({ authenticated: true, username: 'admin', csrfToken: 'csrf', pollIntervalSeconds: 60 });
      }
      const body = JSON.parse(String(init?.body ?? '{}')) as { operationName?: string; variables?: { input?: Record<string, unknown> } };
      switch (body.operationName) {
        case 'Client':
          return json({ data: { client: systemClient() } });
        case 'LocalTargetAllowlist':
          return json({ data: { localTargetAllowlist: { entries: [{ cidr: '127.0.0.1/32', portStart: 0, portEnd: 0 }, { cidr: '::1/128', portStart: 0, portEnd: 0 }] } } });
        case 'ReplaceLocalTargetAllowlist':
          return json({ data: { replaceLocalTargetAllowlist: { entries: body.variables?.input?.entries } } });
        case 'CreateLocalProxy':
          return json({ data: { createLocalProxy: { proxyId: 'local-1', status: 'enabled', proxy: null } } });
        default:
          return json({ data: {} });
      }
    });
    renderSystemClient(fetchMock);

    await screen.findByRole('heading', { name: 'Server Local' });
    expect(screen.getByText('System client')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Create proxy' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Rotate credential' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Delete client' })).not.toBeInTheDocument();

    await screen.findByDisplayValue('127.0.0.1/32');
    await userEvent.click(screen.getByRole('button', { name: 'Save' }));
    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith('/api/admin/graphql', expect.objectContaining({ body: expect.stringContaining('ReplaceLocalTargetAllowlist') })));

    await userEvent.click(screen.getByRole('button', { name: 'Create local proxy' }));
    const dialog = screen.getByRole('dialog', { name: 'Create local proxy' });
    await userEvent.type(within(dialog).getByLabelText('Name'), 'loopback echo');
    await userEvent.type(within(dialog).getByLabelText('Entry port'), '18080');
    await userEvent.type(within(dialog).getByLabelText('Target port'), '8080');
    await userEvent.click(within(dialog).getByRole('button', { name: 'Save proxy' }));
    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith('/api/admin/graphql', expect.objectContaining({
      body: expect.stringMatching(/CreateLocalProxy[\s\S]*"targetHost":"127\.0\.0\.1"[\s\S]*"targetPort":8080/),
    })));
  }, 10000);

  it('updates a system proxy only through the local mutation', async () => {
    const operationNames: string[] = [];
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      if (String(input).endsWith('/api/admin/session')) {
        return json({ authenticated: true, username: 'admin', csrfToken: 'csrf', pollIntervalSeconds: 60 });
      }
      const body = JSON.parse(String(init?.body ?? '{}')) as { operationName?: string };
      if (body.operationName) operationNames.push(body.operationName);
      switch (body.operationName) {
        case 'Proxy':
          return json({ data: { proxy: {
            id: 'local-1', userId: 'server-local-system', clientId: 'server-local', name: 'Local echo', isSystem: true,
            type: 'tcp', status: 'enabled', description: 'echo', runtimeStatus: 'online', activeTCPConnections: 0,
            uploadBytes: 0, downloadBytes: 0, tcpErrorCount: 0, udpErrorCount: 0, httpErrorCount: 0,
            accessAuthEnabled: false, accessAuthVersion: 0,
            config: { entryBindHost: '127.0.0.1', entryPort: 18080, targetHost: '127.0.0.1', targetPort: 8080 },
            certificate: null, createdAt: '2026-07-24T00:00:00Z', updatedAt: '2026-07-24T00:00:00Z',
          } } });
        case 'Domains':
          return json({ data: { domains: { totalCount: 0, pageInfo: { page: 1, pageSize: 100, totalPages: 0, hasNextPage: false, hasPreviousPage: false }, items: [] } } });
        case 'ProxyEntryOptions':
          return json({ data: { proxyEntryOptions: { tcpDefaultBindHost: '127.0.0.1', hosts: [] } } });
        case 'UpdateLocalProxy':
          return json({ data: { updateLocalProxy: { proxyId: 'local-1', status: 'enabled', proxy: null } } });
        default:
          return json({ data: {} });
      }
    });
    renderSystemProxy(fetchMock);

    await screen.findByRole('heading', { name: 'Local echo' });
    await userEvent.click(screen.getByRole('button', { name: 'Edit proxy' }));
    await userEvent.click(within(screen.getByRole('dialog', { name: 'Edit proxy' })).getByRole('button', { name: 'Save changes' }));

    await waitFor(() => expect(operationNames).toContain('UpdateLocalProxy'));
    expect(operationNames).not.toContain('UpdateProxy');
  });
});
