import { useEffect, useMemo, useRef, useState } from 'react';
import { LinkOutlined, PlusOutlined, ReloadOutlined, ThunderboltOutlined } from '@ant-design/icons';
import { Button } from 'antd';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { useQueryClient } from '@tanstack/react-query';
import { Dialog } from '../components/Dialog';
import { CertificateSelectField } from '../components/CertificateSelectField';
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
  queryProxyEntryOptions,
  queryProxies,
  queryUsers,
  type ProxyFilter,
} from '../lib/admin-graphql';
import { isApiError, type Client, type ProxyEntryHostOption, type ProxyRecord, type User } from '../lib/contracts';
import {
  buildCertificateCreateLink,
  clearProxyDraft,
  loadProxyDraft,
  saveProxyDraft,
  CREATED_CERT_PARAM,
  DRAFT_ID_PARAM,
} from '../lib/proxy-draft';
import { formatBytes } from '../lib/format';
import { useSession } from '../session';
import { PageHeader, Pagination, StatusBadge, Timestamp } from './shared';

// ProxyFormDraft：proxy 创建/编辑表单跳转「创建证书」时的草稿快照。
// 必须不含任何 secret material（私钥/token），proxy 表单本身也没有这类字段。
export type ProxyFormDraft = {
  userId: string;
  clientId: string;
  name: string;
  type: string;
  description: string;
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
    type: 'http',
    description: '',
    entryBindHost: '',
    entryHost: '',
    entryPort: '',
    targetHost: '',
    targetPort: '',
    certificateId: '',
  };
}

function isRouteProxy(type: string) {
  return type === 'http' || type === 'https';
}

function isHTTPSProxy(type: string) {
  return type === 'https';
}

// ProxyConfigSubmit 与 admin-graphql 的 ProxyConfigInput 结构一致，外加 certificateId 绑定字段。
// （ProxyConfigInput 未导出；此处的形状与之兼容，可直接作为 config 传入 mutate*Proxy。）
type ProxyConfigSubmit = {
  entryBindHost?: string;
  entryHost?: string;
  entryPort?: number;
  targetHost?: string;
  targetPort?: number;
  certificateId?: string;
};

// buildProxyConfig 由表单组装提交用的 config（含 certificateId 绑定）。
// 注意：不再从主表单提交 certFile/keyFile（证书改由 certificateId 绑定）。
function buildProxyConfig(form: ProxyFormDraft): ProxyConfigSubmit {
  return {
    entryBindHost: form.entryBindHost || undefined,
    entryHost: form.entryHost || undefined,
    entryPort: form.entryPort ? Number(form.entryPort) : undefined,
    targetHost: form.targetHost || undefined,
    targetPort: form.targetPort ? Number(form.targetPort) : undefined,
    certificateId: isHTTPSProxy(form.type) ? form.certificateId : undefined,
  };
}

function proxyEntryLabel(proxy: ProxyRecord) {
  const bindHost = proxy.config.entryBindHost || 'default';
  const entryPort = proxy.config.entryPort ?? 'default';
  const routeHost = isRouteProxy(proxy.type) ? proxy.config.entryHost || 'domain pending' : '';
  return routeHost ? `${bindHost}:${entryPort} / ${routeHost}` : `${bindHost}:${entryPort}`;
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
  const [showDialog, setShowDialog] = useState(false);
  const [formError, setFormError] = useState<string>();
  const [actionError, setActionError] = useState<string>();
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>();
  const [draftNotice, setDraftNotice] = useState<string>();
  const [form, setForm] = useState(defaultProxyForm());
  // 标记「还原草稿后清理 query param 触发的二次运行」，避免把已还原表单重置为默认值。
  const justRestoredRef = useRef(false);

  useEffect(() => {
    if (searchParams.get('create') !== '1') {
      // 对话框关闭（create 参数移除）后复位，确保下次重新打开按全新创建处理。
      justRestoredRef.current = false;
      return;
    }
    // 从「创建证书」流程返回：还原草稿并自动选中新建证书（任务 5.5 / 5.6）。
    const createdCertificateId = searchParams.get(CREATED_CERT_PARAM);
    const draftId = searchParams.get(DRAFT_ID_PARAM);
    if (createdCertificateId) {
      const draft = loadProxyDraft<ProxyFormDraft>(draftId);
      if (draft) {
        setForm({ ...draft, type: 'https', certificateId: createdCertificateId });
        setDraftNotice(undefined);
      } else {
        // 草稿缺失/过期/解析失败：安全降级——仍打开对话框并预选新证书，避免丢失全部工作。
        setForm({
          ...defaultProxyForm(searchParams.get('userId') ?? '', searchParams.get('clientId') ?? ''),
          type: 'https',
          certificateId: createdCertificateId,
        });
        setDraftNotice('表单草稿已失效，请重新填写；已为你选中新创建的证书。');
      }
      clearProxyDraft(draftId);
      setFieldErrors(undefined);
      setFormError(undefined);
      setShowDialog(true);
      // 标记本次为草稿还原；清理已消费的 query param 会再次触发本 effect。
      justRestoredRef.current = true;
      // 清理已消费的 query param，避免刷新重复触发还原。
      setSearchParams((current) => {
        const next = new URLSearchParams(current);
        next.delete(CREATED_CERT_PARAM);
        next.delete(DRAFT_ID_PARAM);
        return next;
      }, { replace: true });
      return;
    }
    if (justRestoredRef.current) {
      // 这是上一步清理 param 触发的二次运行：保留已还原的表单，不做默认重置。
      justRestoredRef.current = false;
      return;
    }
    setForm(defaultProxyForm(searchParams.get('userId') ?? '', searchParams.get('clientId') ?? ''));
    setDraftNotice(undefined);
    setFieldErrors(undefined);
    setFormError(undefined);
    setShowDialog(true);
  }, [searchParams, setSearchParams]);

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

  const usesRouteHost = useMemo(() => isRouteProxy(form.type), [form.type]);
  const usesCertificates = useMemo(() => isHTTPSProxy(form.type), [form.type]);
  const users = usersQuery.data?.items ?? [];
  const clients = clientsQuery.data?.items ?? [];
  const hostOptions = hostOptionsWithCurrent(entryOptionsQuery.data?.hosts ?? [{ value: '', label: 'Default listener host', isDefault: true }], form.entryBindHost);
  const clientSelectionMismatch = Boolean(form.clientId && clientsQuery.data && !clients.some((client) => client.id === form.clientId));

  const openCreateDialog = () => {
    setForm(defaultProxyForm());
    setFieldErrors(undefined);
    setFormError(undefined);
    setDraftNotice(undefined);
    setShowDialog(true);
  };

  const closeCreateDialog = () => {
    setShowDialog(false);
    setDraftNotice(undefined);
    setSearchParams((current) => {
      const next = new URLSearchParams(current);
      next.delete('create');
      next.delete('userId');
      next.delete('clientId');
      return next;
    }, { replace: true });
  };

  const updateFormUser = (userId: string) => {
    setForm((current) => ({ ...current, userId, clientId: '' }));
  };

  // handleCreateCertificate：保存当前表单草稿（不含 secret material）并跳转到证书创建流程。
  // 返回路径携带 user/client 上下文，确保返回时重新打开创建对话框（其余字段由草稿还原）。
  const handleCreateCertificate = () => {
    const draftId = saveProxyDraft<ProxyFormDraft>({ ...form });
    const returnParams = new URLSearchParams({ create: '1' });
    if (form.userId) {
      returnParams.set('userId', form.userId);
    }
    if (form.clientId) {
      returnParams.set('clientId', form.clientId);
    }
    const returnTo = `/proxies?${returnParams.toString()}`;
    navigate(buildCertificateCreateLink({ returnTo, draftId, host: form.entryHost || undefined }));
  };

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
    <section className="page-section">
      <PageHeader
        title="Proxies"
        description="Manage TCP, UDP, HTTP, and HTTPS proxy resources."
        actions={
          <>
            <Button type="default" icon={<ReloadOutlined aria-hidden="true" />} onClick={() => query.refetch()}>
              Refresh
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
          <option value="tcp">TCP</option>
          <option value="udp">UDP</option>
          <option value="http">HTTP</option>
          <option value="https">HTTPS</option>
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
        <>
          <div className="table-wrap">
            <table className="table">
              <thead>
                <tr>
                  <th>Name</th>
                  <th>Type</th>
                  <th>User</th>
                  <th>Client</th>
                  <th>Status</th>
                  <th>Runtime</th>
                  <th>Entry</th>
                  <th>Target</th>
                  <th>Upload</th>
                  <th>Download</th>
                  <th>Actions</th>
                </tr>
              </thead>
              <tbody>
                {data.items.map((proxy) => (
                  <tr key={proxy.id}>
                    <td><Button type="link" icon={<LinkOutlined aria-hidden="true" />} onClick={() => navigate(`/proxies/${proxy.id}`)}>{proxy.name}</Button></td>
                    <td>{proxy.type}</td>
                    <td>{proxy.userId}</td>
                    <td>{proxy.clientId}</td>
                    <td><StatusBadge value={proxy.status} /></td>
                    <td><StatusBadge value={proxy.runtimeStatus} /></td>
                    <td>{proxyEntryLabel(proxy)}</td>
                    <td>{proxy.config.targetHost ?? '-'}:{proxy.config.targetPort ?? '-'}</td>
                    <td>{formatBytes(proxy.uploadBytes)}</td>
                    <td>{formatBytes(proxy.downloadBytes)}</td>
                    <td>
                      <div className="inline-actions">
                        {proxy.status === 'disabled' ? (
                          <Button type="default" icon={<ThunderboltOutlined aria-hidden="true" />} onClick={() => enableMutation.mutate(proxy.id)}>
                            Enable
                          </Button>
                        ) : (
                          <ConfirmButton label="Disable" confirmLabel="Disable this proxy?" onConfirm={() => disableMutation.mutate(proxy.id)} tone="secondary" />
                        )}
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <Pagination page={data.pageInfo.page} totalPages={data.pageInfo.totalPages} onPageChange={setPage} />
        </>
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
        {draftNotice ? <div className="banner banner--warning">{draftNotice}</div> : null}
        {usersQuery.error ? <div className="banner banner--danger">User options failed to load: {usersQuery.error.message}</div> : null}
        {clientsQuery.error ? <div className="banner banner--danger">Client options failed to load: {clientsQuery.error.message}</div> : null}
        {entryOptionsQuery.error ? <div className="banner banner--danger">Entry options failed to load: {entryOptionsQuery.error.message}</div> : null}
        <ValidationBanner fields={fieldErrors} />
        <div className="toolbar-grid toolbar-grid--wide">
          <UserSelectField value={form.userId} users={users} error={fieldErrors?.userId} onChange={updateFormUser} />
          <ClientSelectField value={form.clientId} clients={clients} error={clientSelectionMismatch ? 'Selected client does not belong to the selected user.' : fieldErrors?.clientId} onChange={(clientId) => setForm((current) => ({ ...current, clientId }))} />
          <TextField label="Name" value={form.name} error={fieldErrors?.name} onChange={(event) => setForm((current) => ({ ...current, name: event.target.value }))} />
          <SelectField label="Type" value={form.type} onChange={(event) => setForm((current) => {
            const type = event.target.value;
            return {
              ...current,
              type,
              entryHost: isRouteProxy(type) ? current.entryHost : '',
              certificateId: isHTTPSProxy(type) ? current.certificateId : '',
            };
          })}>
            <option value="tcp">TCP</option>
            <option value="udp">UDP</option>
            <option value="http">HTTP</option>
            <option value="https">HTTPS</option>
          </SelectField>
          <SelectField label="Bind host" value={form.entryBindHost} error={fieldErrors?.entryBindHost} onChange={(event) => setForm((current) => ({ ...current, entryBindHost: event.target.value }))}>
            {hostOptions.map((option) => (
              <option key={option.value || '__default'} value={option.value}>
                {option.label}
              </option>
            ))}
          </SelectField>
          <TextField label="Entry port" value={form.entryPort} error={fieldErrors?.entryPort} onChange={(event) => setForm((current) => ({ ...current, entryPort: event.target.value }))} />
          {usesRouteHost ? (
            <TextField label={form.type === 'https' ? 'SNI domain' : 'HTTP domain'} value={form.entryHost} error={fieldErrors?.entryHost} onChange={(event) => setForm((current) => ({ ...current, entryHost: event.target.value }))} />
          ) : null}
          <TextField label="Target host" value={form.targetHost} error={fieldErrors?.targetHost} onChange={(event) => setForm((current) => ({ ...current, targetHost: event.target.value }))} />
          <TextField label="Target port" value={form.targetPort} error={fieldErrors?.targetPort} onChange={(event) => setForm((current) => ({ ...current, targetPort: event.target.value }))} />
          {usesCertificates ? (
            <CertificateSelectField
              entryHost={form.entryHost}
              value={form.certificateId}
              onChange={(certificateId) => setForm((current) => ({ ...current, certificateId }))}
              onCreateCertificate={handleCreateCertificate}
              error={fieldErrors?.certificateId}
            />
          ) : null}
          <TextAreaField label="Description" value={form.description} onChange={(event) => setForm((current) => ({ ...current, description: event.target.value }))} />
        </div>
      </Dialog>
    </section>
  );
}
