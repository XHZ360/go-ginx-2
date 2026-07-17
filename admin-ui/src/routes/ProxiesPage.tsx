import { useMemo, useState } from 'react';
import { LinkOutlined, PlusOutlined, ReloadOutlined, ThunderboltOutlined } from '@ant-design/icons';
import { Button, Switch, type TableColumnsType } from 'antd';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { useQueryClient } from '@tanstack/react-query';
import { Dialog } from '../components/Dialog';
import { SelectField, TextAreaField, TextField } from '../components/FormField';
import { EmptyState, ErrorState, FilteredEmptyState, PageLoading, ValidationBanner } from '../components/PageStates';
import { ConfirmButton } from '../components/ConfirmButton';
import { useAuthedQuery } from '../hooks/useAuthedQuery';
import { useMutationWithAuth } from '../hooks/useMutationWithAuth';
import {
  mutateCreateProxy,
  mutateDisableProxy,
  mutateEnableProxy,
  queryClients,
  queryDomains,
  queryProxyEntryOptions,
  queryProxies,
  queryUsers,
  type ProxyFilter,
} from '../lib/admin-graphql';
import { isApiError, type Client, type DomainRecord, type ProxyEntryHostOption, type ProxyRecord, type User } from '../lib/contracts';
import { formatBytes } from '../lib/format';
import { useSession } from '../session';
import { DataTable, PageHeader, StatusBadge, pageTablePagination } from './shared';

export type ProxyFormDraft = {
  userId: string;
  clientId: string;
  name: string;
  type: string;
  description: string;
  domainId: string;
  pathPrefix: string;
  stripPrefix: boolean;
  upstreamPathPrefix: string;
  entryBindHost: string;
  entryHost: string;
  entryPort: string;
  targetHost: string;
  targetPort: string;
  certificateId: string;
};

const defaultFilter: ProxyFilter = { query: '', userId: '', clientId: '', type: '', status: '' };

function defaultProxyForm(userId = '', clientId = ''): ProxyFormDraft {
  return {
    userId,
    clientId,
    name: '',
    type: 'web',
    description: '',
    domainId: '',
    pathPrefix: '/',
    stripPrefix: false,
    upstreamPathPrefix: '/',
    entryBindHost: '',
    entryHost: '',
    entryPort: '',
    targetHost: '',
    targetPort: '',
    certificateId: '',
  };
}

function isWebProxy(type: string) {
  return type === 'web' || type === 'http' || type === 'https';
}

function isTCPUDP(type: string) {
  return type === 'tcp' || type === 'udp';
}

// ProxyConfigSubmit 与 admin-graphql 的 ProxyConfigInput 结构一致。
type ProxyConfigSubmit = {
  domainId?: string;
  pathPrefix?: string;
  stripPrefix?: boolean;
  upstreamPathPrefix?: string;
  entryBindHost?: string;
  entryHost?: string;
  entryPort?: number;
  targetHost?: string;
  targetPort?: number;
  certificateId?: string;
};

function buildProxyConfig(form: ProxyFormDraft): ProxyConfigSubmit {
  if (isWebProxy(form.type)) {
    return {
      domainId: form.domainId || undefined,
      pathPrefix: form.pathPrefix || '/',
      stripPrefix: form.stripPrefix,
      upstreamPathPrefix: form.upstreamPathPrefix || '/',
      targetHost: form.targetHost || undefined,
      targetPort: form.targetPort ? Number(form.targetPort) : undefined,
    };
  }
  return {
    entryBindHost: form.entryBindHost || undefined,
    entryHost: form.entryHost || undefined,
    entryPort: form.entryPort ? Number(form.entryPort) : undefined,
    targetHost: form.targetHost || undefined,
    targetPort: form.targetPort ? Number(form.targetPort) : undefined,
  };
}

function proxyEntryLabel(proxy: ProxyRecord) {
  if (isWebProxy(proxy.type)) {
    const host = proxy.config.entryHost || proxy.config.domainId || 'domain';
    const path = proxy.config.pathPrefix || '/';
    return `${host}${path}`;
  }
  const bindHost = proxy.config.entryBindHost || 'default';
  const entryPort = proxy.config.entryPort ?? 'default';
  return `${bindHost}:${entryPort}`;
}

function hostOptionsWithCurrent(options: ProxyEntryHostOption[], current: string) {
  if (!current || options.some((option) => option.value === current)) {
    return options;
  }
  return [{ value: current, label: current, isDefault: false }, ...options];
}

function userOptionLabel(user: User) {
  return `${user.username} (${user.id})`;
}

function clientOptionLabel(client: Client) {
  return `${client.name} (${client.id})`;
}

function UserSelectField({
  value,
  users,
  onChange,
  error,
}: {
  value: string;
  users: User[];
  onChange: (value: string) => void;
  error?: string;
}) {
  const hasSelectedUser = !value || users.some((user) => user.id === value);
  return (
    <SelectField label="User" value={value} error={error} onChange={(event) => onChange(event.target.value)}>
      <option value="">Select user</option>
      {!hasSelectedUser ? <option value={value}>User ID {value}</option> : null}
      {users.map((user) => (
        <option key={user.id} value={user.id}>
          {userOptionLabel(user)}
        </option>
      ))}
    </SelectField>
  );
}

function ClientSelectField({
  value,
  clients,
  onChange,
  error,
}: {
  value: string;
  clients: Client[];
  onChange: (value: string) => void;
  error?: string;
}) {
  const hasSelectedClient = !value || clients.some((client) => client.id === value);
  return (
    <SelectField label="Client" value={value} error={error} onChange={(event) => onChange(event.target.value)}>
      <option value="">Select client</option>
      {!hasSelectedClient ? <option value={value}>Client ID {value}</option> : null}
      {clients.map((client) => (
        <option key={client.id} value={client.id}>
          {clientOptionLabel(client)}
        </option>
      ))}
    </SelectField>
  );
}

export function ProxiesPage() {
  const session = useSession();
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  const [page, setPage] = useState(1);
  const [filter, setFilter] = useState<ProxyFilter>(defaultFilter);
  const [showDialog, setShowDialog] = useState(searchParams.get('create') === '1');
  const [formError, setFormError] = useState<string>();
  const [actionError, setActionError] = useState<string>();
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>();
  const [form, setForm] = useState(defaultProxyForm(searchParams.get('userId') ?? '', searchParams.get('clientId') ?? ''));

  const query = useAuthedQuery({
    queryKey: ['proxies', page, filter],
    queryFn: () => queryProxies({ page: { page, pageSize: 10 }, sort: { field: 'name', direction: 'asc' }, filter }),
    refetchInterval: session.pollIntervalSeconds * 1000,
  });
  const usersQuery = useAuthedQuery({
    queryKey: ['proxy-user-options'],
    queryFn: () => queryUsers({ page: { page: 1, pageSize: 100 }, sort: { field: 'username', direction: 'asc' }, filter: {} }),
  });
  const clientsQuery = useAuthedQuery({
    queryKey: ['proxy-client-options', form.userId],
    queryFn: () =>
      queryClients({
        page: { page: 1, pageSize: 100 },
        sort: { field: 'name', direction: 'asc' },
        filter: form.userId ? { userId: form.userId } : {},
      }),
  });
  const domainsQuery = useAuthedQuery({
    queryKey: ['proxy-domain-options', form.userId],
    queryFn: () =>
      queryDomains({
        page: { page: 1, pageSize: 100 },
        sort: { field: 'host', direction: 'asc' },
        filter: form.userId ? { userId: form.userId, status: 'enabled' } : { status: 'enabled' },
      }),
  });
  const entryOptionsQuery = useAuthedQuery({
    queryKey: ['proxy-entry-options'],
    queryFn: () => queryProxyEntryOptions(),
  });

  const createMutation = useMutationWithAuth({
    mutationFn: () =>
      mutateCreateProxy(session.csrfToken ?? '', {
        userId: form.userId,
        clientId: form.clientId,
        name: form.name,
        type: form.type,
        description: form.description,
        config: buildProxyConfig(form),
      }),
    onSuccess: async () => {
      setShowDialog(false);
      setFormError(undefined);
      setFieldErrors(undefined);
      setSearchParams((current) => {
        const next = new URLSearchParams(current);
        next.delete('create');
        next.delete('userId');
        next.delete('clientId');
        return next;
      }, { replace: true });
      await queryClient.invalidateQueries({ queryKey: ['proxies'] });
    },
    onError: (error) => {
      if (isApiError(error)) {
        setFieldErrors(error.fields);
        setFormError(error.code === 'ENTRY_CONFLICT' ? 'Requested listener conflicts with an active listener.' : error.message);
      }
    },
  });

  const enableMutation = useMutationWithAuth({
    mutationFn: (id: string) => mutateEnableProxy(session.csrfToken ?? '', id),
    onSuccess: async (_, id) => {
      setActionError(undefined);
      await queryClient.invalidateQueries({ queryKey: ['proxies'] });
      await queryClient.invalidateQueries({ queryKey: ['proxy', id] });
    },
    onError: (error) => {
      setActionError(error instanceof Error ? error.message : 'Enable failed.');
    },
  });

  const disableMutation = useMutationWithAuth({
    mutationFn: (id: string) => mutateDisableProxy(session.csrfToken ?? '', id),
    onSuccess: async (_, id) => {
      setActionError(undefined);
      await queryClient.invalidateQueries({ queryKey: ['proxies'] });
      await queryClient.invalidateQueries({ queryKey: ['proxy', id] });
    },
    onError: (error) => {
      setActionError(error instanceof Error ? error.message : 'Disable failed.');
    },
  });

  const usesWeb = useMemo(() => isWebProxy(form.type), [form.type]);
  const users = usersQuery.data?.items ?? [];
  const clients = clientsQuery.data?.items ?? [];
  const domains = (domainsQuery.data?.items ?? []).filter((domain: DomainRecord) => !form.userId || domain.userId === form.userId);
  const hostOptions = hostOptionsWithCurrent(entryOptionsQuery.data?.hosts ?? [{ value: '', label: 'Default listener host', isDefault: true }], form.entryBindHost);
  const clientSelectionMismatch = Boolean(form.clientId && clientsQuery.data && !clients.some((client) => client.id === form.clientId));

  const openCreateDialog = () => {
    setForm(defaultProxyForm());
    setFieldErrors(undefined);
    setFormError(undefined);
    setShowDialog(true);
  };

  const closeCreateDialog = () => {
    setShowDialog(false);
    setSearchParams((current) => {
      const next = new URLSearchParams(current);
      next.delete('create');
      next.delete('userId');
      next.delete('clientId');
      return next;
    }, { replace: true });
  };

  const updateFormUser = (userId: string) => {
    setForm((current) => ({ ...current, userId, clientId: '', domainId: '' }));
  };

  const columns = useMemo<TableColumnsType<ProxyRecord>>(
    () => [
      {
        title: 'Name',
        dataIndex: 'name',
        key: 'name',
        render: (name: string, proxy) => (
          <Button type="link" icon={<LinkOutlined aria-hidden="true" />} onClick={() => navigate(`/proxies/${proxy.id}`)}>
            {name}
          </Button>
        ),
      },
      { title: 'Type', dataIndex: 'type', key: 'type', width: 90 },
      { title: 'User', dataIndex: 'userId', key: 'userId', ellipsis: true, width: 120 },
      { title: 'Client', dataIndex: 'clientId', key: 'clientId', ellipsis: true, width: 120 },
      {
        title: 'Status',
        dataIndex: 'status',
        key: 'status',
        width: 110,
        render: (value: string) => <StatusBadge value={value} />,
      },
      {
        title: 'Runtime',
        dataIndex: 'runtimeStatus',
        key: 'runtimeStatus',
        width: 110,
        render: (value: string) => <StatusBadge value={value} />,
      },
      {
        title: 'Entry',
        key: 'entry',
        ellipsis: true,
        render: (_, proxy) => proxyEntryLabel(proxy),
      },
      {
        title: 'Target',
        key: 'target',
        width: 160,
        render: (_, proxy) => `${proxy.config.targetHost ?? '-'}:${proxy.config.targetPort ?? '-'}`,
      },
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
        title: 'Actions',
        key: 'actions',
        fixed: 'right',
        width: 120,
        render: (_, proxy) => (
          <div className="inline-actions">
            {proxy.status === 'disabled' ? (
              <Button type="default" icon={<ThunderboltOutlined aria-hidden="true" />} onClick={() => enableMutation.mutate(proxy.id)}>
                Enable
              </Button>
            ) : (
              <ConfirmButton label="Disable" confirmLabel="Disable this proxy?" onConfirm={() => disableMutation.mutate(proxy.id)} tone="secondary" />
            )}
          </div>
        ),
      },
    ],
    [disableMutation, enableMutation, navigate],
  );

  if (query.isLoading) {
    return <PageLoading label="Loading proxies..." />;
  }
  if (query.error) {
    return <ErrorState title="Proxies failed" message={query.error.message} retry={() => query.refetch()} />;
  }
  if (!query.data) {
    return <PageLoading label="Loading proxies..." />;
  }

  const data = query.data;

  const hasFilter = Boolean(filter.query || filter.userId || filter.clientId || filter.type || filter.status);

  return (
    <section className="page-section page-section--fill">
      <PageHeader
        title="Proxies"
        description="TCP/UDP listeners and web path proxies. Prefer creating web paths from a Domain so host, TLS, and paths stay consistent."
        actions={
          <>
            <Button type="default" icon={<ReloadOutlined aria-hidden="true" />} onClick={() => query.refetch()}>
              Refresh
            </Button>
            <Button type="default" onClick={() => navigate('/domains?create=1')}>
              Create domain
            </Button>
            <Button type="primary" icon={<PlusOutlined aria-hidden="true" />} onClick={openCreateDialog}>
              Create proxy
            </Button>
          </>
        }
      />
      {actionError ? <div className="banner banner--danger">{actionError}</div> : null}

      <div className="toolbar-grid toolbar-grid--wide">
        <TextField label="Search" value={filter.query ?? ''} onChange={(event) => setFilter((current) => ({ ...current, query: event.target.value }))} />
        <TextField label="User ID" value={filter.userId ?? ''} onChange={(event) => setFilter((current) => ({ ...current, userId: event.target.value }))} />
        <TextField label="Client ID" value={filter.clientId ?? ''} onChange={(event) => setFilter((current) => ({ ...current, clientId: event.target.value }))} />
        <SelectField label="Type" value={filter.type ?? ''} onChange={(event) => setFilter((current) => ({ ...current, type: event.target.value }))}>
          <option value="">All</option>
          <option value="web">Web path</option>
          <option value="tcp">TCP</option>
          <option value="udp">UDP</option>
        </SelectField>
        <SelectField label="Status" value={filter.status ?? ''} onChange={(event) => setFilter((current) => ({ ...current, status: event.target.value }))}>
          <option value="">All</option>
          <option value="enabled">Enabled</option>
          <option value="disabled">Disabled</option>
          <option value="online">Online</option>
          <option value="offline">Offline</option>
        </SelectField>
      </div>

      {data.items.length === 0 ? (
        hasFilter ? (
          <FilteredEmptyState onClear={() => setFilter(defaultFilter)} />
        ) : (
          <EmptyState title="No proxies" message="Create the first proxy to expose managed traffic." />
        )
      ) : (
        <DataTable<ProxyRecord>
          rowKey="id"
          columns={columns}
          dataSource={data.items}
          scroll={{ x: 1300 }}
          pagination={pageTablePagination(data.pageInfo, setPage, { itemLabel: 'proxies' })}
        />
      )}

      <Dialog
        open={showDialog}
        title="Create proxy"
        onClose={closeCreateDialog}
        footer={
          <>
            <Button type="default" onClick={closeCreateDialog}>
              Cancel
            </Button>
            <Button type="primary" onClick={() => createMutation.mutate(undefined)} disabled={createMutation.isPending || clientSelectionMismatch}>
              {createMutation.isPending ? 'Creating...' : 'Create proxy'}
            </Button>
          </>
        }
      >
        {formError ? <div className="banner banner--danger">{formError}</div> : null}
        {usersQuery.error ? <div className="banner banner--danger">User options failed to load: {usersQuery.error.message}</div> : null}
        {clientsQuery.error ? <div className="banner banner--danger">Client options failed to load: {clientsQuery.error.message}</div> : null}
        {domainsQuery.error ? <div className="banner banner--danger">Domain options failed to load: {domainsQuery.error.message}</div> : null}
        {entryOptionsQuery.error ? <div className="banner banner--danger">Entry options failed to load: {entryOptionsQuery.error.message}</div> : null}
        <ValidationBanner fields={fieldErrors} />
        <p className="muted">Web path proxies attach to a Domain. TCP/UDP still use raw listeners. Certificates and HTTP/HTTPS ports are managed on the Domain page.</p>
        <div className="toolbar-grid toolbar-grid--wide">
          <UserSelectField value={form.userId} users={users} error={fieldErrors?.userId} onChange={updateFormUser} />
          <ClientSelectField value={form.clientId} clients={clients} error={clientSelectionMismatch ? 'Selected client does not belong to the selected user.' : fieldErrors?.clientId} onChange={(clientId) => setForm((current) => ({ ...current, clientId }))} />
          <TextField label="Name" value={form.name} error={fieldErrors?.name} onChange={(event) => setForm((current) => ({ ...current, name: event.target.value }))} />
          <SelectField label="Type" value={form.type} onChange={(event) => setForm((current) => {
            const type = event.target.value;
            return {
              ...current,
              type,
              domainId: isWebProxy(type) ? current.domainId : '',
              pathPrefix: isWebProxy(type) ? current.pathPrefix || '/' : '',
            };
          })}>
            <option value="web">Web path (recommended)</option>
            <option value="tcp">TCP</option>
            <option value="udp">UDP</option>
          </SelectField>
          {usesWeb ? (
            <>
              <SelectField label="Domain" value={form.domainId} error={fieldErrors?.domainId} onChange={(event) => setForm((current) => ({ ...current, domainId: event.target.value }))}>
                <option value="">Select domain</option>
                {domains.map((domain) => (
                  <option key={domain.id} value={domain.id}>
                    {domain.host}
                  </option>
                ))}
              </SelectField>
              <TextField label="Path prefix" value={form.pathPrefix} error={fieldErrors?.pathPrefix} placeholder="/" onChange={(event) => setForm((current) => ({ ...current, pathPrefix: event.target.value }))} hint="Longest path wins. Create domains first if the list is empty." />
              <TextField label="Upstream path prefix" value={form.upstreamPathPrefix} error={fieldErrors?.upstreamPathPrefix} onChange={(event) => setForm((current) => ({ ...current, upstreamPathPrefix: event.target.value }))} />
              <label className="field">
                <span className="field__label">Strip path prefix</span>
                <Switch checked={form.stripPrefix} onChange={(checked) => setForm((current) => ({ ...current, stripPrefix: checked }))} />
              </label>
            </>
          ) : (
            <>
              <SelectField label="Bind host" value={form.entryBindHost} error={fieldErrors?.entryBindHost} onChange={(event) => setForm((current) => ({ ...current, entryBindHost: event.target.value }))}>
                {hostOptions.map((option) => (
                  <option key={option.value || '__default'} value={option.value}>
                    {option.label}
                  </option>
                ))}
              </SelectField>
              <TextField label="Entry port" value={form.entryPort} error={fieldErrors?.entryPort} onChange={(event) => setForm((current) => ({ ...current, entryPort: event.target.value }))} />
            </>
          )}
          <TextField label="Target host" value={form.targetHost} error={fieldErrors?.targetHost} onChange={(event) => setForm((current) => ({ ...current, targetHost: event.target.value }))} />
          <TextField label="Target port" value={form.targetPort} error={fieldErrors?.targetPort} onChange={(event) => setForm((current) => ({ ...current, targetPort: event.target.value }))} />
          <TextAreaField label="Description" value={form.description} onChange={(event) => setForm((current) => ({ ...current, description: event.target.value }))} />
        </div>
        {usesWeb && domains.length === 0 ? (
          <p className="banner banner--warning">
            No enabled domains for this user.{' '}
            <Button type="link" onClick={() => navigate('/domains?create=1')}>
              Create a domain first
            </Button>
          </p>
        ) : null}
      </Dialog>
    </section>
  );
}
