import { useMemo, useState } from 'react';
import { GlobalOutlined, PlusOutlined, ReloadOutlined } from '@ant-design/icons';
import { Button, type TableColumnsType } from 'antd';
import { Link, useNavigate, useSearchParams } from 'react-router-dom';
import { useQueryClient } from '@tanstack/react-query';
import { Dialog } from '../components/Dialog';
import { SelectField, TextField } from '../components/FormField';
import { EmptyState, ErrorState, FilteredEmptyState, PageLoading, ValidationBanner } from '../components/PageStates';
import { ConfirmButton } from '../components/ConfirmButton';
import { useAuthedQuery } from '../hooks/useAuthedQuery';
import { useMutationWithAuth } from '../hooks/useMutationWithAuth';
import {
  mutateCreateDomain,
  mutateDisableDomain,
  mutateEnableDomain,
  queryDomains,
  queryUsers,
  type DomainFilter,
} from '../lib/admin-graphql';
import { isApiError, type DomainRecord, type User } from '../lib/contracts';
import { useSession } from '../session';
import { DataTable, PageHeader, StatusBadge, Timestamp, pageTablePagination } from './shared';

const defaultFilter: DomainFilter = { query: '', userId: '', status: '' };

type DomainForm = {
  userId: string;
  host: string;
};

function defaultForm(userId = ''): DomainForm {
  return { userId, host: '' };
}

export function DomainsPage() {
  const session = useSession();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [searchParams, setSearchParams] = useSearchParams();
  const [page, setPage] = useState(1);
  const [filter, setFilter] = useState<DomainFilter>(defaultFilter);
  const [form, setForm] = useState<DomainForm>(defaultForm(searchParams.get('userId') ?? ''));
  const [fieldErrors, setFieldErrors] = useState<Record<string, string> | undefined>();
  const showDialog = searchParams.get('create') === '1';

  const domainsQuery = useAuthedQuery({
    queryKey: ['domains', page, filter],
    queryFn: () => queryDomains({ page: { page, pageSize: 10 }, filter, sort: { field: 'host', direction: 'asc' } }),
    refetchInterval: (session.pollIntervalSeconds ?? 0) * 1000 || false,
  });
  const usersQuery = useAuthedQuery({
    queryKey: ['users', 'domain-create'],
    queryFn: () => queryUsers({ page: { page: 1, pageSize: 100 }, sort: { field: 'username', direction: 'asc' } }),
  });

  const createMutation = useMutationWithAuth({
    mutationFn: async () => {
      setFieldErrors(undefined);
      return mutateCreateDomain(session.csrfToken ?? '', {
        userId: form.userId,
        host: form.host.trim().toLowerCase(),
      });
    },
    onSuccess: async (domain) => {
      await queryClient.invalidateQueries({ queryKey: ['domains'] });
      setSearchParams({});
      setForm(defaultForm());
      navigate(`/domains/${domain.id}`);
    },
    onError: (error) => {
      if (isApiError(error) && error.fields) {
        setFieldErrors(error.fields);
      }
    },
  });

  const enableMutation = useMutationWithAuth({
    mutationFn: (id: string) => mutateEnableDomain(session.csrfToken ?? '', id),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['domains'] }),
  });
  const disableMutation = useMutationWithAuth({
    mutationFn: (id: string) => mutateDisableDomain(session.csrfToken ?? '', id),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['domains'] }),
  });

  const items = domainsQuery.data?.items ?? [];
  const users = usersQuery.data?.items ?? [];
  const hasFilters = Boolean(filter.query || filter.userId || filter.status);
  const userLabel = useMemo(() => {
    const map = new Map(users.map((user: User) => [user.id, user.username]));
    return (id: string) => map.get(id) ?? id;
  }, [users]);

  const columns = useMemo<TableColumnsType<DomainRecord>>(
    () => [
      {
        title: 'Host',
        dataIndex: 'host',
        key: 'host',
        render: (_, domain) => (
          <div className="cell-stack">
            <Link to={`/domains/${domain.id}`}>
              <GlobalOutlined aria-hidden="true" /> {domain.host}
            </Link>
            <span className="muted mono">{domain.id}</span>
          </div>
        ),
      },
      {
        title: 'Owner',
        dataIndex: 'userId',
        key: 'userId',
        width: 140,
        render: (userId: string) => userLabel(userId),
      },
      {
        title: 'Status',
        dataIndex: 'status',
        key: 'status',
        width: 110,
        render: (value: string) => <StatusBadge value={value} />,
      },
      {
        title: 'Entries',
        key: 'entries',
        width: 160,
        render: (_, domain) => (
          <span className="muted">
            HTTP {domain.httpEntryCount} · HTTPS {domain.httpsEntryCount}
          </span>
        ),
      },
      { title: 'Proxies', dataIndex: 'proxyCount', key: 'proxyCount', width: 90, align: 'right' },
      {
        title: 'Certificate',
        key: 'certificate',
        width: 140,
        render: (_, domain) =>
          domain.certificateId ? (
            <span className="mono muted">{domain.certificateId.slice(0, 12)}…</span>
          ) : (
            <span className="muted">Unbound</span>
          ),
      },
      {
        title: 'Updated',
        dataIndex: 'updatedAt',
        key: 'updatedAt',
        width: 160,
        render: (value: string) => <Timestamp value={value} />,
      },
      {
        title: 'Actions',
        key: 'actions',
        fixed: 'right',
        width: 120,
        render: (_, domain) => (
          <div className="inline-actions">
            {domain.status === 'enabled' ? (
              <ConfirmButton
                label="Disable"
                confirmLabel="Disable this domain? HTTPS/HTTP listeners for this host stop accepting traffic."
                onConfirm={() => disableMutation.mutate(domain.id)}
                tone="secondary"
              />
            ) : (
              <Button size="small" onClick={() => enableMutation.mutate(domain.id)}>
                Enable
              </Button>
            )}
          </div>
        ),
      },
    ],
    [disableMutation, enableMutation, userLabel],
  );

  return (
    <section className="page-section page-section--fill">
      <PageHeader
        title="Domains"
        description="Public host identities shared by HTTP and HTTPS. Create a domain first, then attach path proxies and certificates."
        actions={
          <>
            <Button icon={<ReloadOutlined />} onClick={() => domainsQuery.refetch()}>
              Refresh
            </Button>
            <Button type="primary" icon={<PlusOutlined />} onClick={() => setSearchParams({ create: '1' })}>
              Create domain
            </Button>
          </>
        }
      />

      <div className="toolbar-grid toolbar-grid--wide">
        <TextField
          label="Search host"
          value={filter.query ?? ''}
          onChange={(event) => {
            setPage(1);
            setFilter((current) => ({ ...current, query: event.target.value }));
          }}
          placeholder="app.example.com"
        />
        <SelectField
          label="Owner"
          value={filter.userId ?? ''}
          onChange={(event) => {
            setPage(1);
            setFilter((current) => ({ ...current, userId: event.target.value }));
          }}
        >
          <option value="">All users</option>
          {users.map((user) => (
            <option key={user.id} value={user.id}>{user.username}</option>
          ))}
        </SelectField>
        <SelectField
          label="Status"
          value={filter.status ?? ''}
          onChange={(event) => {
            setPage(1);
            setFilter((current) => ({ ...current, status: event.target.value }));
          }}
        >
          <option value="">All statuses</option>
          <option value="enabled">Enabled</option>
          <option value="disabled">Disabled</option>
        </SelectField>
      </div>

      {domainsQuery.isLoading ? <PageLoading label="Loading domains..." /> : null}
      {domainsQuery.isError ? <ErrorState title="Failed to load domains" message={domainsQuery.error instanceof Error ? domainsQuery.error.message : 'Request failed'} retry={() => domainsQuery.refetch()} /> : null}

      {!domainsQuery.isLoading && !domainsQuery.isError && items.length === 0 ? (
        hasFilters ? (
          <FilteredEmptyState onClear={() => setFilter(defaultFilter)} />
        ) : (
          <EmptyState
            title="No domains yet"
            message="Domains hold the public hostname, listeners, and certificate. Path proxies hang under a domain."
            action={
              <Button type="primary" icon={<PlusOutlined />} onClick={() => setSearchParams({ create: '1' })}>
                Create first domain
              </Button>
            }
          />
        )
      ) : null}

      {items.length > 0 && domainsQuery.data ? (
        <DataTable<DomainRecord>
          rowKey="id"
          columns={columns}
          dataSource={items}
          scroll={{ x: 1100 }}
          pagination={pageTablePagination(domainsQuery.data.pageInfo, setPage, { itemLabel: 'domains' })}
        />
      ) : null}

      <Dialog
        open={showDialog}
        title="Create domain"
        onClose={() => {
          setSearchParams({});
          setFieldErrors(undefined);
        }}
        footer={
          <>
            <Button onClick={() => setSearchParams({})}>Cancel</Button>
            <Button type="primary" loading={createMutation.isPending} onClick={() => createMutation.mutate(undefined)}>
              Create domain
            </Button>
          </>
        }
      >
        <div className="stack">
          <p className="muted">
            A domain is the public hostname visitors use. After creating it, add HTTP/HTTPS entries and path proxies on the detail page.
          </p>
          {createMutation.isError && isApiError(createMutation.error) ? (
            <ValidationBanner title={createMutation.error.message} fields={fieldErrors} />
          ) : null}
          <div className="toolbar-grid">
            <SelectField
              label="Owner"
              value={form.userId}
              error={fieldErrors?.userId}
              onChange={(event) => setForm((current) => ({ ...current, userId: event.target.value }))}
            >
              <option value="">Select user</option>
              {users.map((user) => (
                <option key={user.id} value={user.id}>{user.username}</option>
              ))}
            </SelectField>
            <TextField
              label="Hostname"
              value={form.host}
              error={fieldErrors?.host}
              placeholder="app.example.com"
              onChange={(event) => setForm((current) => ({ ...current, host: event.target.value }))}
              hint="Globally unique. HTTP and HTTPS share this host and path map."
            />
          </div>
        </div>
      </Dialog>
    </section>
  );
}
