import { useEffect, useState } from 'react';
import {
  AuditOutlined,
  DashboardOutlined,
  DeploymentUnitOutlined,
  DesktopOutlined,
  GlobalOutlined,
  LogoutOutlined,
  MenuOutlined,
  SafetyCertificateOutlined,
  TeamOutlined,
  UserOutlined,
} from '@ant-design/icons';
import { Button, Drawer, Grid, Layout as AntLayout, Menu, Typography } from 'antd';
import { Navigate, Outlet, useLocation, useNavigate } from 'react-router-dom';
import { PageLoading } from './PageStates';
import { rememberDestination } from '../lib/navigation';
import { useSession } from '../session';

const SIDER_WIDTH = 248;

const navItems = [
  { to: '/dashboard', label: 'Dashboard', icon: <DashboardOutlined aria-hidden="true" /> },
  { to: '/users', label: 'Users', icon: <TeamOutlined aria-hidden="true" /> },
  { to: '/clients', label: 'Clients', icon: <DesktopOutlined aria-hidden="true" /> },
  { to: '/domains', label: 'Domains', icon: <GlobalOutlined aria-hidden="true" /> },
  { to: '/proxies', label: 'Proxies', icon: <DeploymentUnitOutlined aria-hidden="true" /> },
  { to: '/certificates', label: 'Certificates', icon: <SafetyCertificateOutlined aria-hidden="true" /> },
  { to: '/audit', label: 'Audit', icon: <AuditOutlined aria-hidden="true" /> },
];

const menuItems = navItems.map((item) => ({
  key: item.to,
  icon: item.icon,
  label: item.label,
}));

function BrandBlock() {
  return (
    <div className="brand-block">
      <div className="brand">GoGinx Admin</div>
      <Typography.Text className="sidebar-caption">Management console</Typography.Text>
    </div>
  );
}

function SideNav({
  selectedKey,
  onNavigate,
}: {
  selectedKey: string;
  onNavigate: (key: string) => void;
}) {
  return (
    <>
      <BrandBlock />
      <Menu
        aria-label="Primary"
        className="nav-list"
        mode="inline"
        selectedKeys={[selectedKey]}
        items={menuItems}
        onClick={({ key }) => onNavigate(String(key))}
        theme="dark"
      />
    </>
  );
}

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
  const screens = Grid.useBreakpoint();
  const isMobile = !screens.lg;
  const [mobileOpen, setMobileOpen] = useState(false);
  const [collapsed, setCollapsed] = useState(false);

  useEffect(() => {
    setMobileOpen(false);
  }, [location.pathname]);

  if (session.status === 'unknown' || session.status === 'checking') {
    return <PageLoading label="Loading admin session..." />;
  }

  if (session.status !== 'authenticated') {
    rememberDestination(`${location.pathname}${location.search}${location.hash}`);
    return <Navigate to="/login" replace />;
  }

  const selectedKey = navItems.find((item) => location.pathname === item.to || location.pathname.startsWith(`${item.to}/`))?.to ?? '/dashboard';

  const go = (key: string) => {
    navigate(key);
    setMobileOpen(false);
  };

  return (
    <AntLayout className="app-shell">
      {!isMobile ? (
        <AntLayout.Sider
          className="app-sidebar"
          width={SIDER_WIDTH}
          collapsedWidth={80}
          collapsible
          collapsed={collapsed}
          onCollapse={setCollapsed}
          theme="dark"
          trigger={null}
        >
          <SideNav selectedKey={selectedKey} onNavigate={go} />
          <button
            type="button"
            className="app-sidebar__collapse"
            aria-label={collapsed ? 'Expand navigation' : 'Collapse navigation'}
            onClick={() => setCollapsed((current) => !current)}
          >
            {collapsed ? '»' : '«'}
          </button>
        </AntLayout.Sider>
      ) : null}

      <AntLayout className="app-content-shell">
        <AntLayout.Header className="topbar">
          <div className="topbar__leading">
            {isMobile ? (
              <Button
                type="text"
                className="topbar__menu-trigger"
                icon={<MenuOutlined aria-hidden="true" />}
                aria-label="Open navigation"
                onClick={() => setMobileOpen(true)}
              />
            ) : null}
            <div className="topbar__user">
              <UserOutlined aria-hidden="true" />
              <strong>{session.username}</strong>
              <span className="muted"> signed in</span>
            </div>
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

      <Drawer
        open={isMobile && mobileOpen}
        onClose={() => setMobileOpen(false)}
        placement="left"
        width={SIDER_WIDTH}
        className="app-nav-drawer"
        styles={{ body: { padding: 0, background: '#0f172a' }, header: { display: 'none' } }}
        destroyOnHidden
      >
        <div className="app-sidebar app-sidebar--drawer">
          <SideNav selectedKey={selectedKey} onNavigate={go} />
        </div>
      </Drawer>
    </AntLayout>
  );
}
