import { NavLink, Navigate, Outlet, useLocation, useNavigate } from 'react-router-dom';
import { PageLoading } from './PageStates';
import { rememberDestination } from '../lib/navigation';
import { useSession } from '../session';

const navItems = [
  { to: '/dashboard', label: 'Dashboard' },
  { to: '/users', label: 'Users' },
  { to: '/clients', label: 'Clients' },
  { to: '/proxies', label: 'Proxies' },
  { to: '/certificates', label: 'Certificates' },
  { to: '/audit', label: 'Audit' },
];

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

  return (
    <div className="app-shell">
      <aside className="app-sidebar">
        <div>
          <div className="brand">GoGinx Admin</div>
          <p className="muted">Management console</p>
        </div>
        <nav className="nav-list" aria-label="Primary">
          {navItems.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              className={({ isActive }) => (isActive ? 'nav-link nav-link--active' : 'nav-link')}
            >
              {item.label}
            </NavLink>
          ))}
        </nav>
      </aside>
      <main className="app-main">
        <header className="topbar">
          <div>
            <strong>{session.username}</strong>
            <span className="muted"> signed in</span>
          </div>
          <button
            type="button"
            className="button button--secondary"
            onClick={async () => {
              await session.logout();
              navigate('/login', { replace: true });
            }}
          >
            Logout
          </button>
        </header>
        <Outlet />
      </main>
    </div>
  );
}
