import { describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { SessionProvider } from '../session';
import { LoginPage } from '../routes/LoginPage';
import { ProtectedLayout, RootRedirect } from '../components/Layout';
import { DashboardPage } from '../routes/DashboardPage';

function renderWithProviders(initialEntries: string[], routes: React.ReactNode) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  render(
    <QueryClientProvider client={queryClient}>
      <SessionProvider>
        <MemoryRouter initialEntries={initialEntries}>{routes}</MemoryRouter>
      </SessionProvider>
    </QueryClientProvider>,
  );
}

describe('session bootstrap and navigation', () => {
  it('redirects root to login when bootstrap is unauthenticated', async () => {
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL) => {
      if (String(input).endsWith('/api/admin/session')) {
        return new Response(JSON.stringify({ authenticated: false }), { status: 200 });
      }
      return new Response(JSON.stringify({ data: { dashboard: { onlineClientCount: 0, enabledProxyCount: 0, activeTCPConnectionCount: 0, cumulativeUploadBytes: 0, cumulativeDownloadBytes: 0, cumulativeTCPErrorCount: 0, cumulativeUDPErrorCount: 0, cumulativeHTTPErrorCount: 0 } } }), { status: 200 });
    }));

    renderWithProviders(
      ['/'],
      <Routes>
        <Route path="/" element={<RootRedirect />} />
        <Route path="/login" element={<div>Login route</div>} />
      </Routes>,
    );
    await screen.findByText('Login route');
  });

  it('redirects to login when the authenticated session has expired', async () => {
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL) => {
      if (String(input).endsWith('/api/admin/session')) {
        return new Response(JSON.stringify({ authenticated: true, username: 'admin', csrfToken: 'csrf', expiresAt: new Date(0).toISOString() }), { status: 200 });
      }
      return new Response('{}', { status: 200 });
    }));

    renderWithProviders(
      ['/dashboard'],
      <Routes>
        <Route path="/login" element={<div>Login route</div>} />
        <Route path="/" element={<ProtectedLayout />}>
          <Route path="dashboard" element={<div>Dashboard route</div>} />
        </Route>
      </Routes>,
    );

    await screen.findByText('Login route');
  });

  it('restores intended destination after login', async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith('/api/admin/session')) {
        return new Response(JSON.stringify({ authenticated: false }), { status: 200 });
      }
      if (url.endsWith('/api/admin/login')) {
        return new Response(JSON.stringify({ authenticated: true, username: 'admin', csrfToken: 'csrf', pollIntervalSeconds: 5 }), { status: 200 });
      }
      if (url.endsWith('/api/admin/graphql')) {
        return new Response(JSON.stringify({ data: { dashboard: { onlineClientCount: 1, enabledProxyCount: 2, activeTCPConnectionCount: 3, cumulativeUploadBytes: 4, cumulativeDownloadBytes: 5, cumulativeTCPErrorCount: 6, cumulativeUDPErrorCount: 7, cumulativeHTTPErrorCount: 8 } } }), { status: 200 });
      }
      return new Response('{}', { status: 200 });
    });
    vi.stubGlobal('fetch', fetchMock);

    renderWithProviders(
      ['/users/user-1'],
      <Routes>
        <Route path="/login" element={<LoginPage />} />
        <Route path="/" element={<ProtectedLayout />}>
          <Route path="users/:id" element={<div>User detail route</div>} />
          <Route path="dashboard" element={<DashboardPage />} />
        </Route>
      </Routes>,
    );

    await screen.findByRole('button', { name: 'Sign in' });
    await userEvent.type(screen.getByLabelText('Username'), 'admin');
    await userEvent.type(screen.getByLabelText('Password'), 'secret');
    await userEvent.click(screen.getByRole('button', { name: 'Sign in' }));

    await waitFor(() => expect(screen.getByText('User detail route')).toBeInTheDocument());
  });

  it('restores scoped clients destination after login', async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.endsWith('/api/admin/session')) {
        return new Response(JSON.stringify({ authenticated: false }), { status: 200 });
      }
      if (url.endsWith('/api/admin/login')) {
        return new Response(JSON.stringify({ authenticated: true, username: 'admin', csrfToken: 'csrf', pollIntervalSeconds: 5 }), { status: 200 });
      }
      return new Response('{}', { status: 200 });
    });
    vi.stubGlobal('fetch', fetchMock);

    renderWithProviders(
      ['/clients?userId=user-1'],
      <Routes>
        <Route path="/login" element={<LoginPage />} />
        <Route path="/" element={<ProtectedLayout />}>
          <Route path="clients" element={<div>Scoped clients route</div>} />
          <Route path="dashboard" element={<DashboardPage />} />
        </Route>
      </Routes>,
    );

    await screen.findByRole('button', { name: 'Sign in' });
    await userEvent.type(screen.getByLabelText('Username'), 'admin');
    await userEvent.type(screen.getByLabelText('Password'), 'secret');
    await userEvent.click(screen.getByRole('button', { name: 'Sign in' }));

    await waitFor(() => expect(screen.getByText('Scoped clients route')).toBeInTheDocument());
  });
});
