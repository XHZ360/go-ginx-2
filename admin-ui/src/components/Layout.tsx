import {
  AuditOutlined,
  DashboardOutlined,
  DeploymentUnitOutlined,
  DesktopOutlined,
  LogoutOutlined,
  SafetyCertificateOutlined,
  TeamOutlined,
  UserOutlined,
} from '@ant-design/icons';
import { Button, Layout as AntLayout, Menu, Typography } from 'antd';
import { Navigate, Outlet, useLocation, useNavigate } from 'react-router-dom';
import { PageLoading } from './PageStates';
import { rememberDestination } from '../lib/navigation';
import { useSession } from '../session';

const navItems = [
  { to: '/dashboard', label: 'Dashboard', icon: <DashboardOutlined aria-hidden="true" /> },
  { to: '/users', label: 'Users', icon: <TeamOutlined aria-hidden="true" /> },
  { to: '/clients', label: 'Clients', icon: <DesktopOutlined aria-hidden="true" /> },
  { to: '/proxies', label: 'Proxies', icon: <DeploymentUnitOutlined aria-hidden="true" /> },
  { to: '/certificates', label: 'Certificates', icon: <SafetyCertificateOutlined aria-hidden="true" /> },
  { to: '/audit', label: 'Audit', icon: <AuditOutlined aria-hidden="true" /> },
];

const menuItems = navItems.map((item) => ({
  key: item.to,
  icon: item.icon,
  label: item.label,
}));

export function RootRedirect() {
  const session = useSession();

  if (session.status === 'unknown' || session.status === 'checking') {
    return <PageLoading label="Checking session..." />;
  }

  return <Navigate to={session.status === 'authenticated' ? '/dashboard' : '/login'} replace />;
}

export function ProtectedLayout() {
  const session = useSession();
  const location = useLocation();
  const navigate = useNavigate();

  if (session.status === 'unknown' || session.status === 'checking') {
    return <PageLoading label="Loading admin session..." />;
  }

  if (session.status !== 'authenticated') {
    rememberDestination(`${location.pathname}${location.search}${location.hash}`);
    return <Navigate to="/login" replace />;
  }

  const selectedKey = navItems.find((item) => location.pathname === item.to || location.pathname.startsWith(`${item.to}/`))?.to ?? '/dashboard';

  return (
    <AntLayout className="app-shell">
      <AntLayout.Sider className="app-sidebar" width={248}>
        <div className="brand-block">
          <div className="brand">GoGinx Admin</div>
          <Typography.Text className="sidebar-caption">Management console</Typography.Text>
        </div>
        <Menu
          aria-label="Primary"
          className="nav-list"
          mode="inline"
          selectedKeys={[selectedKey]}
          items={menuItems}
          onClick={({ key }) => navigate(String(key))}
          theme="dark"
        />
      </AntLayout.Sider>
      <AntLayout className="app-content-shell">
        <AntLayout.Header className="topbar">
          <div className="topbar__user">
            <UserOutlined aria-hidden="true" />
            <strong>{session.username}</strong>
            <span className="muted"> signed in</span>
          </div>
          <Button
            type="default"
            icon={<LogoutOutlined aria-hidden="true" />}
            onClick={async () => {
              await session.logout();
              navigate('/login', { replace: true });
            }}
          >
            Logout
          </Button>
        </AntLayout.Header>
        <AntLayout.Content className="app-main">
          <Outlet />
        </AntLayout.Content>
      </AntLayout>
    </AntLayout>
  );
}
