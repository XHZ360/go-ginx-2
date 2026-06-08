import { Button } from 'antd';
import { ReloadOutlined } from '@ant-design/icons';
import { useAuthedQuery } from '../hooks/useAuthedQuery';
import { queryDashboard } from '../lib/admin-graphql';
import { formatBytes, formatCount } from '../lib/format';
import { ErrorState, PageLoading } from '../components/PageStates';
import { PageHeader } from './shared';
import { useSession } from '../session';

const cards = [
  { key: 'onlineClientCount', label: 'Online clients', kind: 'count' },
  { key: 'enabledProxyCount', label: 'Enabled proxies', kind: 'count' },
  { key: 'activeTCPConnectionCount', label: 'Active TCP connections', kind: 'count' },
  { key: 'cumulativeUploadBytes', label: 'Upload', kind: 'bytes' },
  { key: 'cumulativeDownloadBytes', label: 'Download', kind: 'bytes' },
  { key: 'cumulativeTCPErrorCount', label: 'TCP errors', kind: 'count' },
  { key: 'cumulativeUDPErrorCount', label: 'UDP errors', kind: 'count' },
  { key: 'cumulativeHTTPErrorCount', label: 'HTTP errors', kind: 'count' },
] as const;

export function DashboardPage() {
  const session = useSession();
  const query = useAuthedQuery({
    queryKey: ['dashboard'],
    queryFn: queryDashboard,
    refetchInterval: session.pollIntervalSeconds * 1000,
  });

  if (query.isLoading) {
    return <PageLoading label="Loading dashboard..." />;
  }

  if (query.error) {
    return <ErrorState title="Dashboard failed" message={query.error.message} retry={() => query.refetch()} />;
  }

  return (
    <section className="page-section">
      <PageHeader
        title="Dashboard"
        description="Runtime summary for the current admin surface."
        actions={
          <Button type="default" icon={<ReloadOutlined aria-hidden="true" />} onClick={() => query.refetch()}>
            Refresh
          </Button>
        }
      />
      <div className="metric-grid">
        {cards.map((card) => {
          const value = query.data?.[card.key] ?? 0;
          return (
            <article key={card.key} className="metric-card">
              <span className="metric-card__label">{card.label}</span>
              <strong className="metric-card__value">
                {card.kind === 'bytes' ? formatBytes(Number(value)) : formatCount(Number(value))}
              </strong>
            </article>
          );
        })}
      </div>
    </section>
  );
}
