import { useMemo, useState } from 'react';
import { PlusOutlined, ReloadOutlined } from '@ant-design/icons';
import { Button, type TableColumnsType } from 'antd';
import { useNavigate } from 'react-router-dom';
import { useQueryClient } from '@tanstack/react-query';
import { Dialog } from '../components/Dialog';
import { TextField, SelectField } from '../components/FormField';
import { useAuthedQuery } from '../hooks/useAuthedQuery';
import { useMutationWithAuth } from '../hooks/useMutationWithAuth';
import { mutateCreateUser, queryUsers, type UserFilter } from '../lib/admin-graphql';
import { isApiError, type User } from '../lib/contracts';
import { formatBytes } from '../lib/format';
import { EmptyState, ErrorState, FilteredEmptyState, PageLoading, ValidationBanner } from '../components/PageStates';
import { useSession } from '../session';
import { DataTable, PageHeader, StatusBadge, Timestamp, pageTablePagination } from './shared';

const defaultFilter: UserFilter = { query: '', role: '', status: '' };

export function UsersPage() {
  const session = useSession();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [page, setPage] = useState(1);
  const [filter, setFilter] = useState<UserFilter>(defaultFilter);
  const [showDialog, setShowDialog] = useState(false);
  const [form, setForm] = useState({ username: '', password: '', role: 'user' });
  const [formFields, setFormFields] = useState<Record<string, string>>();
  const [formError, setFormError] = useState<string>();

  const query = useAuthedQuery({
    queryKey: ['users', page, filter],
    queryFn: () => queryUsers({ page: { page, pageSize: 10 }, sort: { field: 'username', direction: 'asc' }, filter }),
  });

  const createMutation = useMutationWithAuth({
    mutationFn: () => mutateCreateUser(session.csrfToken ?? '', form),
    onSuccess: async () => {
      setShowDialog(false);
      setForm({ username: '', password: '', role: 'user' });
      setFormFields(undefined);
      setFormError(undefined);
      await queryClient.invalidateQueries({ queryKey: ['users'] });
    },
    onError: (error) => {
      if (isApiError(error)) {
        setFormFields(error.fields);
        setFormError(error.message);
      }
    },
  });

  const columns = useMemo<TableColumnsType<User>>(
    () => [
      { title: 'ID', dataIndex: 'id', key: 'id', ellipsis: true, width: 120 },
      { title: 'Username', dataIndex: 'username', key: 'username', ellipsis: true },
      { title: 'Role', dataIndex: 'role', key: 'role', width: 100 },
      {
        title: 'Status',
        dataIndex: 'status',
        key: 'status',
        width: 120,
        render: (value: string) => <StatusBadge value={value} />,
      },
      { title: 'Clients', dataIndex: 'clientCount', key: 'clientCount', width: 90, align: 'right' },
      { title: 'Proxies', dataIndex: 'proxyCount', key: 'proxyCount', width: 90, align: 'right' },
      {
        title: 'Upload',
        dataIndex: 'uploadBytes',
        key: 'uploadBytes',
        width: 110,
        render: (value: number) => formatBytes(value),
      },
      {
        title: 'Download',
        dataIndex: 'downloadBytes',
        key: 'downloadBytes',
        width: 110,
        render: (value: number) => formatBytes(value),
      },
      {
        title: 'Last activity',
        dataIndex: 'lastActivityAt',
        key: 'lastActivityAt',
        width: 160,
        render: (value: string | null | undefined) => <Timestamp value={value} />,
      },
      {
        title: 'Updated',
        dataIndex: 'updatedAt',
        key: 'updatedAt',
        width: 160,
        render: (value: string | null | undefined) => <Timestamp value={value} />,
      },
      {
        title: 'Actions',
        key: 'actions',
        fixed: 'right',
        width: 100,
        render: (_, user) => (
          <Button
            type="link"
            onClick={(event) => {
              event.stopPropagation();
              navigate(`/clients?userId=${encodeURIComponent(user.id)}`);
            }}
          >
            Clients
          </Button>
        ),
      },
    ],
    [navigate],
  );

  if (query.isLoading) {
    return <PageLoading label="Loading users..." />;
  }
  if (query.error) {
    return <ErrorState title="Users failed" message={query.error.message} retry={() => query.refetch()} />;
  }
  if (!query.data) {
    return <PageLoading label="Loading users..." />;
  }

  const data = query.data;
  const hasFilter = Boolean(filter.query || filter.role || filter.status);

  return (
    <section className="page-section page-section--fill">
      <PageHeader
        title="Users"
        description="Manage administrator-visible user accounts."
        actions={
          <>
            <Button type="default" icon={<ReloadOutlined aria-hidden="true" />} onClick={() => query.refetch()}>
              Refresh
            </Button>
            <Button type="primary" icon={<PlusOutlined aria-hidden="true" />} onClick={() => setShowDialog(true)}>
              Create user
            </Button>
          </>
        }
      />

      <div className="toolbar-grid">
        <TextField label="Search" value={filter.query ?? ''} onChange={(event) => setFilter((current) => ({ ...current, query: event.target.value }))} />
        <SelectField label="Role" value={filter.role ?? ''} onChange={(event) => setFilter((current) => ({ ...current, role: event.target.value }))}>
          <option value="">All</option>
          <option value="user">User</option>
          <option value="admin">Admin</option>
        </SelectField>
        <SelectField label="Status" value={filter.status ?? ''} onChange={(event) => setFilter((current) => ({ ...current, status: event.target.value }))}>
          <option value="">All</option>
          <option value="enabled">Enabled</option>
          <option value="disabled">Disabled</option>
        </SelectField>
      </div>

      {data.items.length === 0 ? (
        hasFilter ? (
          <FilteredEmptyState onClear={() => setFilter(defaultFilter)} />
        ) : (
          <EmptyState title="No users" message="Create the first user to start assigning clients and proxies." />
        )
      ) : (
        <DataTable<User>
          rowKey="id"
          columns={columns}
          dataSource={data.items}
          scroll={{ x: 1100 }}
          pagination={pageTablePagination(data.pageInfo, setPage, { itemLabel: 'users' })}
          onRow={(user) => ({
            onClick: () => navigate(`/users/${user.id}`),
            className: 'table-row-link',
          })}
        />
      )}

      <Dialog
        open={showDialog}
        title="Create user"
        onClose={() => setShowDialog(false)}
        footer={
          <>
            <Button type="default" onClick={() => setShowDialog(false)}>
              Cancel
            </Button>
            <Button type="primary" onClick={() => createMutation.mutate(undefined)} disabled={createMutation.isPending}>
              {createMutation.isPending ? 'Creating...' : 'Create user'}
            </Button>
          </>
        }
      >
        {formError ? <div className="banner banner--danger">{formError}</div> : null}
        <ValidationBanner fields={formFields} />
        <div className="stack">
          <TextField label="Username" value={form.username} error={formFields?.username} onChange={(event) => setForm((current) => ({ ...current, username: event.target.value }))} />
          <TextField label="Initial password" type="password" value={form.password} error={formFields?.password} onChange={(event) => setForm((current) => ({ ...current, password: event.target.value }))} />
          <SelectField label="Role" value={form.role} onChange={(event) => setForm((current) => ({ ...current, role: event.target.value }))}>
            <option value="user">User</option>
            <option value="admin">Admin</option>
          </SelectField>
        </div>
      </Dialog>
    </section>
  );
}
