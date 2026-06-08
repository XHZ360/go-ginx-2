import { useState } from 'react';
import { ReloadOutlined } from '@ant-design/icons';
import { Button } from 'antd';
import { queryAudit, type AuditFilter } from '../lib/admin-graphql';
import { EmptyState, ErrorState, FilteredEmptyState, PageLoading } from '../components/PageStates';
import { useAuthedQuery } from '../hooks/useAuthedQuery';
import { PageHeader, Pagination, StatusBadge, Timestamp } from './shared';

const defaultFilter: AuditFilter = { query: '', actorType: '', actorId: '', resourceType: '', action: '', result: '' };

export function AuditPage() {
  const [page, setPage] = useState(1);
  const [filter, setFilter] = useState<AuditFilter>(defaultFilter);

  const query = useAuthedQuery({
    queryKey: ['audit', page, filter],
    queryFn: () => queryAudit({ page: { page, pageSize: 20 }, sort: { field: 'createdAt', direction: 'desc' }, filter }),
  });

  if (query.isLoading) {
    return <PageLoading label="Loading audit..." />;
  }
  if (query.error) {
    return <ErrorState title="Audit failed" message={query.error.message} retry={() => query.refetch()} />;
  }
  if (!query.data) {
    return <PageLoading label="Loading audit..." />;
  }

  const data = query.data;

  const hasFilter = Boolean(filter.query || filter.actorType || filter.actorId || filter.resourceType || filter.action || filter.result);

  return (
    <section className="page-section">
      <PageHeader
        title="Audit"
        description="Recent control-plane activity."
        actions={
          <Button type="default" icon={<ReloadOutlined aria-hidden="true" />} onClick={() => query.refetch()}>
            Refresh
          </Button>
        }
      />

      <div className="toolbar-grid toolbar-grid--wide">
        <label className="field"><span className="field__label">Search</span><input className="input" value={filter.query ?? ''} onChange={(event) => setFilter((current) => ({ ...current, query: event.target.value }))} /></label>
        <label className="field"><span className="field__label">Actor type</span><input className="input" value={filter.actorType ?? ''} onChange={(event) => setFilter((current) => ({ ...current, actorType: event.target.value }))} /></label>
        <label className="field"><span className="field__label">Actor ID</span><input className="input" value={filter.actorId ?? ''} onChange={(event) => setFilter((current) => ({ ...current, actorId: event.target.value }))} /></label>
        <label className="field"><span className="field__label">Resource type</span><input className="input" value={filter.resourceType ?? ''} onChange={(event) => setFilter((current) => ({ ...current, resourceType: event.target.value }))} /></label>
        <label className="field"><span className="field__label">Action</span><input className="input" value={filter.action ?? ''} onChange={(event) => setFilter((current) => ({ ...current, action: event.target.value }))} /></label>
        <label className="field"><span className="field__label">Result</span><input className="input" value={filter.result ?? ''} onChange={(event) => setFilter((current) => ({ ...current, result: event.target.value }))} /></label>
      </div>

      {data.items.length === 0 ? (
        hasFilter ? <FilteredEmptyState onClear={() => setFilter(defaultFilter)} /> : <EmptyState title="No audit events" message="Audit activity will appear here once admin actions are recorded." />
      ) : (
        <>
          <div className="table-wrap">
            <table className="table">
              <thead>
                <tr>
                  <th>Timestamp</th>
                  <th>Actor</th>
                  <th>Resource</th>
                  <th>Action</th>
                  <th>Result</th>
                </tr>
              </thead>
              <tbody>
                {data.items.map((event) => (
                  <tr key={event.id}>
                    <td><Timestamp value={event.createdAt} /></td>
                    <td>{event.actorType}:{event.actorId}</td>
                    <td>{event.resourceType}:{event.resourceId}</td>
                    <td>{event.action}</td>
                    <td><StatusBadge value={event.result} /></td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <Pagination page={data.pageInfo.page} totalPages={data.pageInfo.totalPages} onPageChange={setPage} />
        </>
      )}
    </section>
  );
}
