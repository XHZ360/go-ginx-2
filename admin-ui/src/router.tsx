import { createBrowserRouter } from 'react-router-dom';
import { ProtectedLayout, RootRedirect } from './components/Layout';
import { LoginPage } from './routes/LoginPage';
import { DashboardPage } from './routes/DashboardPage';
import { UsersPage } from './routes/UsersPage';
import { UserDetailPage } from './routes/UserDetailPage';
import { ClientsPage } from './routes/ClientsPage';
import { ClientDetailPage } from './routes/ClientDetailPage';
import { DomainsPage } from './routes/DomainsPage';
import { DomainDetailPage } from './routes/DomainDetailPage';
import { ProxiesPage } from './routes/ProxiesPage';
import { ProxyDetailPage } from './routes/ProxyDetailPage';
import { CertificatesPage } from './routes/CertificatesPage';
import { AuditPage } from './routes/AuditPage';

export function createAdminBrowserRouter() {
  return createBrowserRouter([
    {
      path: '/',
      element: <RootRedirect />,
    },
    {
      path: '/login',
      element: <LoginPage />,
    },
    {
      path: '/',
      element: <ProtectedLayout />,
      children: [
        { path: 'dashboard', element: <DashboardPage /> },
        { path: 'users', element: <UsersPage /> },
        { path: 'users/:id', element: <UserDetailPage /> },
        { path: 'clients', element: <ClientsPage /> },
        { path: 'clients/:id', element: <ClientDetailPage /> },
        { path: 'domains', element: <DomainsPage /> },
        { path: 'domains/:id', element: <DomainDetailPage /> },
        { path: 'proxies', element: <ProxiesPage /> },
        { path: 'proxies/:id', element: <ProxyDetailPage /> },
        { path: 'certificates', element: <CertificatesPage /> },
        { path: 'audit', element: <AuditPage /> },
      ],
    },
  ]);
}
