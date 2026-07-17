import { useMemo, useState } from 'react';
import { ReloadOutlined } from '@ant-design/icons';
import { Button, type TableColumnsType } from 'antd';
import { queryAudit, type AuditFilter } from '../lib/admin-graphql';
import { type AuditEvent } from '../lib/contracts';
import { EmptyState, ErrorState, FilteredEmptyState, PageLoading } from '../components/PageStates';
import { TextField } from '../components/FormField';
import { useAuthedQuery } from '../hooks/useAuthedQuery';
import { DataTable, PageHeader, StatusBadge, Timestamp, pageTablePagination } from './shared';

const defaultFilter: AuditFilter = { query: '', actorType: '', actorId: '', resourceType: '', action: '', result: '' };

export function AuditPage() {
  const [page, setPage] = useState(1);
  const [filter, setFilter] = useState<AuditFilter>(defaultFilter);

  const query = useAuthedQuery({
    queryKey: ['audit', page, filter],
    queryFn: () => queryAudit({ page: { page, pageSize: 20 }, sort: { field: 'createdAt', direction: 'desc' }, filter }),
  });

  const columns = useMemo<TableColumnsType<AuditEvent>>(
    () => [
      {
        title: 'Timestamp',
        dataIndex: 'createdAt',
        key: 'createdAt',
        width: 180,
        render: (value: string) => <Timestamp value={value} />,
      },
      {
        title: 'Actor',
        key: 'actor',
        ellipsis: true,
        render: (_, event) => `${event.actorType}:${event.actorId}`,
      },
      {
        title: 'Resource',
        key: 'resource',
        ellipsis: true,
        render: (_, event) => `${event.resourceType}:${event.resourceId}`,
      },
      { title: 'Action', dataIndex: 'action', key: 'action', width: 140 },
      {
        title: 'Result',
        dataIndex: 'result',
        key: 'result',
        width: 120,
        render: (value: string) => <StatusBadge value={value} />,
      },
    ],
    [],
  );

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
    <section className="page-section page-section--fill">
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
        <TextField label="Search" value={filter.query ?? ''} onChange={(event) => setFilter((current) => ({ ...current, query: event.target.value }))} />
        <TextField label="Actor type" value={filter.actorType ?? ''} onChange={(event) => setFilter((current) => ({ ...current, actorType: event.target.value }))} />
        <TextField label="Actor ID" value={filter.actorId ?? ''} onChange={(event) => setFilter((current) => ({ ...current, actorId: event.target.value }))} />
        <TextField label="Resource type" value={filter.resourceType ?? ''} onChange={(event) => setFilter((current) => ({ ...current, resourceType: event.target.value }))} />
        <TextField label="Action" value={filter.action ?? ''} onChange={(event) => setFilter((current) => ({ ...current, action: event.target.value }))} />
        <TextField label="Result" value={filter.result ?? ''} onChange={(event) => setFilter((current) => ({ ...current, result: event.target.value }))} />
      </div>

      {data.items.length === 0 ? (
        hasFilter ? <FilteredEmptyState onClear={() => setFilter(defaultFilter)} /> : <EmptyState title="No audit events" message="Audit activity will appear here once admin actions are recorded." />
      ) : (
        <DataTable<AuditEvent>
          rowKey="id"
          columns={columns}
          dataSource={data.items}
          scroll={{ x: 900 }}
          pagination={pageTablePagination(data.pageInfo, setPage, { itemLabel: 'events' })}
        />
      )}
    </section>
  );
}
