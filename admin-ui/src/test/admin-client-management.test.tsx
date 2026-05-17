import { describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { SessionProvider } from '../session';
import { ProtectedLayout } from '../components/Layout';
import { ClientsPage } from '../routes/ClientsPage';
import { ClientDetailPage } from '../routes/ClientDetailPage';
import { UsersPage } from '../routes/UsersPage';
import { UserDetailPage } from '../routes/UserDetailPage';

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
  {
    id: 'user-2',
    username: 'bob',
    role: 'user',
    status: 'enabled',
    clientCount: 0,
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

function graphQL(data: unknown, status = 200) {
  return new Response(JSON.stringify(data), { status, headers: { 'Content-Type': 'application/json' } });
}

function renderAdmin(initialEntries: string[]) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  render(
    <QueryClientProvider client={queryClient}>
      <SessionProvider>
        <MemoryRouter initialEntries={initialEntries}>
          <Routes>
            <Route path="/login" element={<div>Login route</div>} />
            <Route path="/" element={<ProtectedLayout />}>
              <Route path="users" element={<UsersPage />} />
              <Route path="users/:id" element={<UserDetailPage />} />
              <Route path="clients" element={<ClientsPage />} />
              <Route path="clients/:id" element={<ClientDetailPage />} />
            </Route>
          </Routes>
        </MemoryRouter>
      </SessionProvider>
    </QueryClientProvider>,
  );
}

function sessionResponse() {
  return graphQL({ authenticated: true, username: 'admin', csrfToken: 'csrf', pollIntervalSeconds: 5 });
}

function createFetchMock(options?: { createValidationFailure?: boolean; rotateFailure?: boolean }) {
  const clientsByUser: Record<string, ReturnType<typeof client>[]> = {
    '': [client('client-all', 'user-2', 'all-node')],
    'user-1': [client('client-1', 'user-1', 'home-node')],
    'user-missing': [],
  };

  return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input);
    if (url.endsWith('/api/admin/session')) {
      return sessionResponse();
    }
    if (!url.endsWith('/api/admin/graphql')) {
      return graphQL({ data: {} });
    }

    const body = JSON.parse(String(init?.body ?? '{}')) as {
      query?: string;
      variables?: { input?: { filter?: { userId?: string }; userId?: string; name?: string; serverAddress?: string } };
    };
    const query = body.query ?? '';
    const variables = body.variables ?? {};

    if (query.includes('query Users')) {
      return graphQL({ data: { users: { items: users, totalCount: users.length, pageInfo: { ...pageInfo, totalCount: users.length } } } });
    }
    if (query.includes('query User(')) {
      return graphQL({ data: { user: users[0] } });
    }
    if (query.includes('query Clients')) {
      const userId = variables.input?.filter?.userId ?? '';
      const items = clientsByUser[userId] ?? [];
      return graphQL({ data: { clients: { items, totalCount: items.length, pageInfo: { ...pageInfo, totalCount: items.length } } } });
    }
    if (query.includes('query Client(')) {
      return graphQL({ data: { client: client('client-1', 'user-1', 'home-node') } });
    }
    if (query.includes('mutation CreateClientJoin')) {
      return graphQL({
        data: {
          createClientJoin: {
            clientId: 'client-joined',
            token: 'goginx_join_generated-token',
            client: client('client-joined', variables.input?.userId ?? '', variables.input?.name ?? ''),
          },
        },
      });
    }
    if (query.includes('mutation CreateClient')) {
      if (options?.createValidationFailure) {
        return graphQL({
          errors: [{
            message: 'validation failed',
            extensions: { code: 'VALIDATION_FAILED', fields: { userId: 'user is required', name: 'name is required' } },
          }],
        });
      }
      return graphQL({
        data: {
          createClient: {
            clientId: 'client-created',
            credential: 'generated-secret',
            client: client('client-created', variables.input?.userId ?? '', variables.input?.name ?? ''),
          },
        },
      });
    }
    if (query.includes('mutation RotateClientCredential')) {
      if (options?.rotateFailure) {
        return graphQL({
          errors: [{ message: 'client not found', extensions: { code: 'NOT_FOUND' } }],
        });
      }
      return graphQL({
        data: {
          rotateClientCredential: {
            clientId: 'client-1',
            credential: 'rotated-secret',
            client: client('client-1', 'user-1', 'home-node'),
          },
        },
      });
    }

    return graphQL({ data: {} });
  });
}

describe('admin client management', () => {
  it('filters clients by user selector and creates a client with one-time credential', async () => {
    const fetchMock = createFetchMock();
    vi.stubGlobal('fetch', fetchMock);

    renderAdmin(['/clients?userId=user-missing']);

    await screen.findByRole('heading', { name: 'Clients' });
    expect(screen.getByLabelText('User')).toHaveValue('user-missing');
    expect(screen.getByRole('option', { name: 'User ID user-missing' })).toBeInTheDocument();

    await userEvent.selectOptions(screen.getByLabelText('User'), 'user-1');
    await screen.findByText('home-node');

    await userEvent.click(screen.getByRole('button', { name: 'Create client' }));
    expect(screen.getByLabelText('Owner user')).toHaveValue('user-1');
    await userEvent.type(screen.getByLabelText('Name'), 'branch-node');
    await userEvent.click(within(screen.getByRole('dialog', { name: 'Create client' })).getByRole('button', { name: 'Create client' }));

    await screen.findByText('generated-secret');
    expect(fetchMock).toHaveBeenCalledWith('/api/admin/graphql', expect.objectContaining({
      body: expect.stringContaining('"userId":"user-1"'),
    }));
  });

  it('generates a client join token from the clients page', async () => {
    const fetchMock = createFetchMock();
    vi.stubGlobal('fetch', fetchMock);

    renderAdmin(['/clients?userId=user-1']);

    await screen.findByText('home-node');
    await userEvent.click(screen.getByRole('button', { name: 'Create join token' }));

    const dialog = screen.getByRole('dialog', { name: 'Create join token' });
    expect(within(dialog).getByLabelText('Owner user')).toHaveValue('user-1');
    await userEvent.type(within(dialog).getByLabelText('Name'), 'garage-node');
    await userEvent.clear(within(dialog).getByLabelText('Server address'));
    await userEvent.type(within(dialog).getByLabelText('Server address'), 'edge.example.com:8443');
    await userEvent.click(within(dialog).getByRole('button', { name: 'Create join token' }));

    await screen.findByText('goginx_join_generated-token');
    expect(fetchMock).toHaveBeenCalledWith('/api/admin/graphql', expect.objectContaining({
      body: expect.stringContaining('"serverAddress":"edge.example.com:8443"'),
    }));
  });

  it('keeps create validation errors inside the client form and clears user filtering through the selector', async () => {
    vi.stubGlobal('fetch', createFetchMock({ createValidationFailure: true }));

    renderAdmin(['/clients?userId=user-1']);

    await screen.findByText('home-node');
    await userEvent.selectOptions(screen.getByLabelText('User'), '');
    await screen.findByText('all-node');

    await userEvent.click(screen.getByRole('button', { name: 'Create client' }));
    expect(screen.getByLabelText('Owner user')).toHaveValue('');
    await userEvent.click(within(screen.getByRole('dialog', { name: 'Create client' })).getByRole('button', { name: 'Create client' }));

    await screen.findByText('validation failed');
    expect(screen.getAllByText('name is required').length).toBeGreaterThan(0);
    expect(screen.getByRole('heading', { name: 'Clients' })).toBeInTheDocument();
  });

  it('navigates from users surfaces to scoped clients', async () => {
    vi.stubGlobal('fetch', createFetchMock());

    renderAdmin(['/users']);

    await screen.findByText('alice');
    await userEvent.click(screen.getAllByRole('button', { name: 'Clients' })[0]);

    await screen.findByRole('heading', { name: 'Clients' });
    expect(screen.getByLabelText('User')).toHaveValue('user-1');
    await screen.findByText('home-node');
  });

  it('navigates from user detail to scoped clients', async () => {
    vi.stubGlobal('fetch', createFetchMock());

    renderAdmin(['/users/user-1']);

    await screen.findByRole('heading', { name: 'alice' });
    await userEvent.click(screen.getByRole('button', { name: 'View clients' }));

    await screen.findByRole('heading', { name: 'Clients' });
    expect(screen.getByLabelText('User')).toHaveValue('user-1');
    await screen.findByText('home-node');
  });

  it('rotates client credentials and keeps detail content visible on rotation errors', async () => {
    vi.stubGlobal('fetch', createFetchMock());

    renderAdmin(['/clients/client-1']);

    await screen.findByRole('heading', { name: 'home-node' });
    await userEvent.click(screen.getByRole('button', { name: 'Rotate credential' }));
    await userEvent.click(screen.getByRole('button', { name: 'Confirm' }));

    await screen.findByText('rotated-secret');
    expect(screen.getByText('Client ID: client-1')).toBeInTheDocument();
  });

  it('shows rotation failures without discarding client detail', async () => {
    vi.stubGlobal('fetch', createFetchMock({ rotateFailure: true }));

    renderAdmin(['/clients/client-1']);

    await screen.findByRole('heading', { name: 'home-node' });
    await userEvent.click(screen.getByRole('button', { name: 'Rotate credential' }));
    await userEvent.click(screen.getByRole('button', { name: 'Confirm' }));

    await screen.findByText('client not found');
    expect(screen.getByText('Client ID: client-1')).toBeInTheDocument();
  });
});
