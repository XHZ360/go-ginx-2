import { useEffect, useMemo, useState } from 'react';
import { CopyOutlined, DeploymentUnitOutlined, PlusOutlined, ReloadOutlined } from '@ant-design/icons';
import { Button, Tag, type TableColumnsType } from 'antd';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { useQueryClient } from '@tanstack/react-query';
import { Dialog } from '../components/Dialog';
import { SelectField, TextField } from '../components/FormField';
import { useAuthedQuery } from '../hooks/useAuthedQuery';
import { useMutationWithAuth } from '../hooks/useMutationWithAuth';
import { mutateCreateClient, mutateCreateClientJoin, mutateDeleteClient, queryClients, queryUsers, type ClientFilter, type ClientJoinInput } from '../lib/admin-graphql';
import { isApiError, type Client, type User } from '../lib/contracts';
import { EmptyState, ErrorState, FilteredEmptyState, PageLoading, ValidationBanner } from '../components/PageStates';
import { useSession } from '../session';
import { DataTable, PageHeader, StatusBadge, Timestamp, pageTablePagination } from './shared';
import { ConfirmButton } from '../components/ConfirmButton';

const defaultFilter: ClientFilter = { query: '', userId: '', status: '' };
const defaultServerName = 'go-ginx-control.local';
const defaultServerCAFile = 'data/certs/control-ca.crt';
const defaultJoinTTLSeconds = '3600';

type ClientDialogMode = 'credential' | 'join';
type ClientForm = {
  userId: string;
  name: string;
  credential: string;
  enrollmentUrl: string;
  serverAddress: string;
  serverTLSAddress: string;
  serverName: string;
  serverCAFile: string;
  ttlSeconds: string;
};

function defaultJoinValues() {
  const currentLocation = typeof window === 'undefined' ? undefined : window.location;
  const host = currentLocation?.hostname || '127.0.0.1';
  const origin = currentLocation?.origin || 'http://127.0.0.1:8080';
  return {
    enrollmentUrl: `${origin}/api/client/enroll`,
    serverAddress: `${host}:8443`,
    serverTLSAddress: `${host}:9443`,
    serverName: defaultServerName,
    serverCAFile: defaultServerCAFile,
    ttlSeconds: defaultJoinTTLSeconds,
  };
}

function defaultForm(userId: string): ClientForm {
  return { userId, name: '', credential: '', ...defaultJoinValues() };
}

function userOptionLabel(user: User) {
  return `${user.username} (${user.id})`;
}

function clientJoinInputFromForm(form: ClientForm): ClientJoinInput {
  const ttlSeconds = Number(form.ttlSeconds);
  return {
    userId: form.userId,
    name: form.name,
    enrollmentUrl: form.enrollmentUrl,
    serverAddress: form.serverAddress,
    serverTLSAddress: form.serverTLSAddress || undefined,
    serverName: form.serverName,
    serverCAFile: form.serverCAFile || undefined,
    ttlSeconds: Number.isFinite(ttlSeconds) && ttlSeconds > 0 ? ttlSeconds : undefined,
  };
}

function UserSelectField({
  label,
  value,
  users,
  onChange,
  error,
  allLabel,
}: {
  label: string;
  value: string;
  users: User[];
  onChange: (value: string) => void;
  error?: string;
  allLabel: string;
}) {
  const hasSelectedUser = !value || users.some((user) => user.id === value);

  return (
    <SelectField label={label} value={value} error={error} onChange={(event) => onChange(event.target.value)}>
      <option value="">{allLabel}</option>
      {!hasSelectedUser ? <option value={value}>User ID {value}</option> : null}
      {users.map((user) => (
        <option key={user.id} value={user.id}>
          {userOptionLabel(user)}
        </option>
      ))}
    </SelectField>
  );
}

export function ClientsPage() {
  const session = useSession();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [searchParams, setSearchParams] = useSearchParams();
  const scopedUserId = searchParams.get('userId') ?? '';
  const [page, setPage] = useState(1);
  const [filter, setFilter] = useState<ClientFilter>({ ...defaultFilter, userId: scopedUserId });
  const [showDialog, setShowDialog] = useState(false);
  const [dialogMode, setDialogMode] = useState<ClientDialogMode>('credential');
  const [form, setForm] = useState<ClientForm>(defaultForm(scopedUserId));
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>();
  const [formError, setFormError] = useState<string>();
  const [createdCredential, setCreatedCredential] = useState<string>();
  const [createdJoinToken, setCreatedJoinToken] = useState<string>();
  const [copyStatus, setCopyStatus] = useState<string>();
  const [actionError, setActionError] = useState<string>();

  useEffect(() => {
    setFilter((current) => (current.userId === scopedUserId ? current : { ...current, userId: scopedUserId }));
    setPage(1);
  }, [scopedUserId]);

  const query = useAuthedQuery({
    queryKey: ['clients', page, filter],
    queryFn: () => queryClients({ page: { page, pageSize: 10 }, sort: { field: 'name', direction: 'asc' }, filter }),
    refetchInterval: session.pollIntervalSeconds * 1000,
  });
  const usersQuery = useAuthedQuery({
    queryKey: ['client-user-options'],
    queryFn: () => queryUsers({ page: { page: 1, pageSize: 100 }, sort: { field: 'username', direction: 'asc' }, filter: {} }),
  });

  const createMutation = useMutationWithAuth({
    mutationFn: () =>
      mutateCreateClient(session.csrfToken ?? '', {
        userId: form.userId,
        name: form.name,
        credential: form.credential || undefined,
      }),
    onSuccess: async (data) => {
      setFieldErrors(undefined);
      setFormError(undefined);
      setCreatedCredential(data.createClient.credential ?? undefined);
      setCreatedJoinToken(undefined);
      setCopyStatus(undefined);
      await queryClient.invalidateQueries({ queryKey: ['clients'] });
    },
    onError: (error) => {
      setCreatedCredential(undefined);
      setCreatedJoinToken(undefined);
      setCopyStatus(undefined);
      if (isApiError(error)) {
        setFieldErrors(error.fields);
        setFormError(error.message);
      }
    },
  });

  const createJoinMutation = useMutationWithAuth({
    mutationFn: () => mutateCreateClientJoin(session.csrfToken ?? '', clientJoinInputFromForm(form)),
    onSuccess: async (data) => {
      setFieldErrors(undefined);
      setFormError(undefined);
      setCreatedCredential(undefined);
      setCreatedJoinToken(data.createClientJoin.token ?? undefined);
      setCopyStatus(undefined);
      await queryClient.invalidateQueries({ queryKey: ['clients'] });
    },
    onError: (error) => {
      setCreatedCredential(undefined);
      setCreatedJoinToken(undefined);
      setCopyStatus(undefined);
      if (isApiError(error)) {
        setFieldErrors(error.fields);
        setFormError(error.message);
      } else {
        setFormError(error.message);
      }
    },
  });

  const deleteMutation = useMutationWithAuth({
    mutationFn: (id: string) => mutateDeleteClient(session.csrfToken ?? '', id),
    onSuccess: async () => {
      setActionError(undefined);
      await queryClient.invalidateQueries({ queryKey: ['clients'] });
    },
    onError: (error) => {
      setActionError(error.message);
    },
  });

  const users = usersQuery.data?.items ?? [];

  const updateUserFilter = (userId: string) => {
    setPage(1);
    setFilter((current) => ({ ...current, userId }));
    setSearchParams((current) => {
      const next = new URLSearchParams(current);
      if (userId) {
        next.set('userId', userId);
      } else {
        next.delete('userId');
      }
      return next;
    }, { replace: true });
  };

  const openCreateDialog = () => {
    setDialogMode('credential');
    setForm(defaultForm(filter.userId ?? ''));
    setFieldErrors(undefined);
    setFormError(undefined);
    setCreatedCredential(undefined);
    setCreatedJoinToken(undefined);
    setCopyStatus(undefined);
    setShowDialog(true);
  };

  const openJoinDialog = () => {
    setDialogMode('join');
    setForm(defaultForm(filter.userId ?? ''));
    setFieldErrors(undefined);
    setFormError(undefined);
    setCreatedCredential(undefined);
    setCreatedJoinToken(undefined);
    setCopyStatus(undefined);
    setShowDialog(true);
  };

  const closeCreateDialog = () => {
    setShowDialog(false);
    setForm(defaultForm(filter.userId ?? ''));
    setFieldErrors(undefined);
    setFormError(undefined);
    setCreatedCredential(undefined);
    setCreatedJoinToken(undefined);
    setCopyStatus(undefined);
  };

  const copyJoinToken = async () => {
    if (!createdJoinToken) {
      return;
    }
    if (typeof navigator === 'undefined' || !navigator.clipboard) {
      setCopyStatus('Clipboard is unavailable.');
      return;
    }
    try {
      await navigator.clipboard.writeText(createdJoinToken);
      setCopyStatus('Copied.');
    } catch {
      setCopyStatus('Copy failed.');
    }
  };

  const clearFilters = () => {
    setPage(1);
    setFilter(defaultFilter);
    setSearchParams((current) => {
      const next = new URLSearchParams(current);
      next.delete('userId');
      return next;
    }, { replace: true });
  };

  const createProxyForClient = (clientId: string, userId: string) => {
    navigate(`/proxies?create=1&userId=${encodeURIComponent(userId)}&clientId=${encodeURIComponent(clientId)}`);
  };

  const columns = useMemo<TableColumnsType<Client>>(
    () => [
      { title: 'ID', dataIndex: 'id', key: 'id', ellipsis: true, width: 120 },
      { title: 'User', dataIndex: 'userId', key: 'userId', ellipsis: true, width: 120 },
      {
        title: 'Name',
        dataIndex: 'name',
        key: 'name',
        ellipsis: true,
        render: (name: string, client) => <div className="inline-actions"><span>{name}</span>{client.isSystem ? <Tag color="blue">System</Tag> : null}</div>,
      },
      {
        title: 'Status',
        dataIndex: 'status',
        key: 'status',
        width: 110,
        render: (value: string) => <StatusBadge value={value} />,
      },
      {
        title: 'Online',
        key: 'online',
        width: 90,
        render: (_, client) => (client.runtime.online ? 'Yes' : 'No'),
      },
      {
        title: 'Protocol',
        key: 'protocol',
        width: 100,
        render: (_, client) => client.runtime.protocol ?? 'N/A',
      },
      {
        title: 'Active proxies',
        key: 'activeProxies',
        width: 120,
        align: 'right',
        render: (_, client) => client.runtime.activeProxies ?? 0,
      },
      {
        title: 'Active streams',
        key: 'activeStreams',
        width: 120,
        align: 'right',
        render: (_, client) => client.runtime.activeStreams ?? 0,
      },
      {
        title: 'Last heartbeat',
        key: 'lastHeartbeat',
        width: 160,
        render: (_, client) => <Timestamp value={client.runtime.lastHeartbeat} />,
      },
      {
        title: 'Actions',
        key: 'actions',
        fixed: 'right',
        width: 220,
        render: (_, client) => (
          <div className="inline-actions" onClick={(event) => event.stopPropagation()}>
            {client.isSystem ? (
              <Button type="default" icon={<DeploymentUnitOutlined aria-hidden="true" />} onClick={() => navigate(`/clients/${client.id}`)}>
                Manage local
              </Button>
            ) : (
              <>
                <Button type="default" icon={<DeploymentUnitOutlined aria-hidden="true" />} onClick={() => createProxyForClient(client.id, client.userId)}>
                  Create proxy
                </Button>
                <ConfirmButton
                  label="Delete"
                  confirmLabel="Delete this client?"
                  onConfirm={() => deleteMutation.mutate(client.id)}
                  disabled={deleteMutation.isPending}
                />
              </>
            )}
          </div>
        ),
      },
    ],
    [deleteMutation.isPending, navigate],
  );

  if (query.isLoading) {
    return <PageLoading label="Loading clients..." />;
  }
  if (query.error) {
    return <ErrorState title="Clients failed" message={query.error.message} retry={() => query.refetch()} />;
  }
  if (!query.data) {
    return <PageLoading label="Loading clients..." />;
  }

  const data = query.data;

  const hasFilter = Boolean(filter.query || filter.userId || filter.status);

  return (
    <section className="page-section page-section--fill">
      <PageHeader
        title="Clients"
        description="Live runtime view for managed clients."
        actions={
          <>
            <Button type="default" icon={<ReloadOutlined aria-hidden="true" />} onClick={() => query.refetch()}>
              Refresh
            </Button>
            <Button type="default" icon={<DeploymentUnitOutlined aria-hidden="true" />} onClick={openJoinDialog}>
              Create join token
            </Button>
            <Button type="primary" icon={<PlusOutlined aria-hidden="true" />} onClick={openCreateDialog}>
              Create client
            </Button>
          </>
        }
      />

      <div className="toolbar-grid">
        <TextField label="Search" value={filter.query ?? ''} onChange={(event) => setFilter((current) => ({ ...current, query: event.target.value }))} />
        <UserSelectField
          label="User"
          value={filter.userId ?? ''}
          users={users}
          onChange={updateUserFilter}
          allLabel="All users"
        />
        <label className="field">
          <span className="field__label">Status</span>
          <select className="input" value={filter.status ?? ''} onChange={(event) => setFilter((current) => ({ ...current, status: event.target.value }))}>
            <option value="">All</option>
            <option value="online">Online</option>
            <option value="offline">Offline</option>
            <option value="disabled">Disabled</option>
          </select>
        </label>
      </div>

      {actionError ? <div className="banner banner--danger">{actionError}</div> : null}

      {data.items.length === 0 ? (
        hasFilter ? (
          <FilteredEmptyState onClear={clearFilters} />
        ) : (
          <EmptyState title="No clients" message="No managed clients are registered yet." />
        )
      ) : (
        <DataTable<Client>
          rowKey="id"
          columns={columns}
          dataSource={data.items}
          scroll={{ x: 1200 }}
          pagination={pageTablePagination(data.pageInfo, setPage, { itemLabel: 'clients' })}
          onRow={(client) => ({
            onClick: () => navigate(`/clients/${client.id}`),
            className: 'table-row-link',
          })}
        />
      )}

      <Dialog
        open={showDialog}
        title={dialogMode === 'join' ? 'Create join token' : 'Create client'}
        onClose={closeCreateDialog}
        footer={
          <>
            <Button type="default" onClick={closeCreateDialog}>
              Close
            </Button>
            {dialogMode === 'join' ? (
              <Button type="primary" onClick={() => createJoinMutation.mutate(undefined)} disabled={createJoinMutation.isPending}>
                {createJoinMutation.isPending ? 'Creating...' : 'Create join token'}
              </Button>
            ) : (
              <Button type="primary" onClick={() => createMutation.mutate(undefined)} disabled={createMutation.isPending}>
                {createMutation.isPending ? 'Creating...' : 'Create client'}
              </Button>
            )}
          </>
        }
      >
        {formError ? <div className="banner banner--danger">{formError}</div> : null}
        {createdCredential ? (
          <div className="banner banner--success" role="status">
            <strong>Client credential</strong>
            <p>This value is shown once. Store it before closing this dialog.</p>
            <code className="secret-value">{createdCredential}</code>
          </div>
        ) : null}
        {createdJoinToken ? (
          <div className="banner banner--success" role="status">
            <strong>Client join token</strong>
            <p>This value is shown once and expires after the configured TTL.</p>
            <code className="secret-value">{createdJoinToken}</code>
            <Button type="default" className="secret-action" icon={<CopyOutlined aria-hidden="true" />} onClick={copyJoinToken}>
              Copy token
            </Button>
            {copyStatus ? <span className="field__hint">{copyStatus}</span> : null}
          </div>
        ) : null}
        {usersQuery.error ? <div className="banner banner--danger">User options failed to load: {usersQuery.error.message}</div> : null}
        <ValidationBanner fields={fieldErrors} />
        <div className="stack">
          <UserSelectField
            label="Owner user"
            value={form.userId}
            users={users}
            onChange={(userId) => setForm((current) => ({ ...current, userId }))}
            error={fieldErrors?.userId}
            allLabel="Select user"
          />
          <TextField label="Name" value={form.name} error={fieldErrors?.name} onChange={(event) => setForm((current) => ({ ...current, name: event.target.value }))} />
          {dialogMode === 'join' ? (
            <>
              <TextField label="Enrollment URL" value={form.enrollmentUrl} error={fieldErrors?.enrollmentUrl} onChange={(event) => setForm((current) => ({ ...current, enrollmentUrl: event.target.value }))} />
              <TextField label="Server address" value={form.serverAddress} error={fieldErrors?.serverAddress} onChange={(event) => setForm((current) => ({ ...current, serverAddress: event.target.value }))} />
              <TextField label="Server TLS address" value={form.serverTLSAddress} onChange={(event) => setForm((current) => ({ ...current, serverTLSAddress: event.target.value }))} />
              <TextField label="Server name" value={form.serverName} error={fieldErrors?.serverName} onChange={(event) => setForm((current) => ({ ...current, serverName: event.target.value }))} />
              <TextField label="Server CA file" value={form.serverCAFile} onChange={(event) => setForm((current) => ({ ...current, serverCAFile: event.target.value }))} />
              <TextField label="TTL seconds" type="number" min="1" value={form.ttlSeconds} onChange={(event) => setForm((current) => ({ ...current, ttlSeconds: event.target.value }))} />
            </>
          ) : (
            <TextField
              label="Initial credential"
              value={form.credential}
              error={fieldErrors?.credential}
              hint="Leave blank to generate one."
              onChange={(event) => setForm((current) => ({ ...current, credential: event.target.value }))}
            />
          )}
        </div>
      </Dialog>
    </section>
  );
}
