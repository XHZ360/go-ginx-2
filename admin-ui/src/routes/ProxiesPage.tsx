import { useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';
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
  queryProxies,
  type ProxyFilter,
} from '../lib/admin-graphql';
import { isApiError } from '../lib/contracts';
import { formatBytes } from '../lib/format';
import { useSession } from '../session';
import { PageHeader, Pagination, StatusBadge, Timestamp } from './shared';

const defaultFilter: ProxyFilter = { query: '', userId: '', clientId: '', type: '', status: '' };

export function ProxiesPage() {
  const session = useSession();
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const [page, setPage] = useState(1);
  const [filter, setFilter] = useState<ProxyFilter>(defaultFilter);
  const [showDialog, setShowDialog] = useState(false);
  const [formError, setFormError] = useState<string>();
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>();
  const [form, setForm] = useState({
    userId: '',
    clientId: '',
    name: '',
    type: 'http',
    description: '',
    entryHost: '',
    entryPort: '',
    targetHost: '',
    targetPort: '',
  });

  const query = useAuthedQuery({
    queryKey: ['proxies', page, filter],
    queryFn: () => queryProxies({ page: { page, pageSize: 10 }, sort: { field: 'name', direction: 'asc' }, filter }),
    refetchInterval: session.pollIntervalSeconds * 1000,
  });

  const createMutation = useMutationWithAuth({
    mutationFn: () =>
      mutateCreateProxy(session.csrfToken ?? '', {
        userId: form.userId,
        clientId: form.clientId,
        name: form.name,
        type: form.type,
        description: form.description,
        config: {
          entryHost: form.entryHost || undefined,
          entryPort: form.entryPort ? Number(form.entryPort) : undefined,
          targetHost: form.targetHost || undefined,
          targetPort: form.targetPort ? Number(form.targetPort) : undefined,
        },
      }),
    onSuccess: async () => {
      setShowDialog(false);
      setFormError(undefined);
      setFieldErrors(undefined);
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
      await queryClient.invalidateQueries({ queryKey: ['proxies'] });
      await queryClient.invalidateQueries({ queryKey: ['proxy', id] });
    },
  });

  const disableMutation = useMutationWithAuth({
    mutationFn: (id: string) => mutateDisableProxy(session.csrfToken ?? '', id),
    onSuccess: async (_, id) => {
      await queryClient.invalidateQueries({ queryKey: ['proxies'] });
      await queryClient.invalidateQueries({ queryKey: ['proxy', id] });
    },
  });

  const usesPort = useMemo(() => form.type === 'tcp' || form.type === 'udp', [form.type]);

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
            <button type="button" className="button button--secondary" onClick={() => query.refetch()}>
              Refresh
            </button>
            <button type="button" className="button" onClick={() => setShowDialog(true)}>
              Create proxy
            </button>
          </>
        }
      />

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
                    <td><button type="button" className="link-button" onClick={() => navigate(`/proxies/${proxy.id}`)}>{proxy.name}</button></td>
                    <td>{proxy.type}</td>
                    <td>{proxy.userId}</td>
                    <td>{proxy.clientId}</td>
                    <td><StatusBadge value={proxy.status} /></td>
                    <td><StatusBadge value={proxy.runtimeStatus} /></td>
                    <td>{proxy.config.entryHost ?? '0.0.0.0'}:{proxy.config.entryPort ?? '-'}</td>
                    <td>{proxy.config.targetHost ?? '-'}:{proxy.config.targetPort ?? '-'}</td>
                    <td>{formatBytes(proxy.uploadBytes)}</td>
                    <td>{formatBytes(proxy.downloadBytes)}</td>
                    <td>
                      <div className="inline-actions">
                        {proxy.status === 'disabled' ? (
                          <button type="button" className="button button--secondary" onClick={() => enableMutation.mutate(proxy.id)}>
                            Enable
                          </button>
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
        onClose={() => setShowDialog(false)}
        footer={
          <>
            <button type="button" className="button button--secondary" onClick={() => setShowDialog(false)}>
              Cancel
            </button>
            <button type="button" className="button" onClick={() => createMutation.mutate(undefined)} disabled={createMutation.isPending}>
              {createMutation.isPending ? 'Creating...' : 'Create proxy'}
            </button>
          </>
        }
      >
        {formError ? <div className="banner banner--danger">{formError}</div> : null}
        <ValidationBanner fields={fieldErrors} />
        <div className="toolbar-grid toolbar-grid--wide">
          <TextField label="User ID" value={form.userId} error={fieldErrors?.userId} onChange={(event) => setForm((current) => ({ ...current, userId: event.target.value }))} />
          <TextField label="Client ID" value={form.clientId} error={fieldErrors?.clientId} onChange={(event) => setForm((current) => ({ ...current, clientId: event.target.value }))} />
          <TextField label="Name" value={form.name} error={fieldErrors?.name} onChange={(event) => setForm((current) => ({ ...current, name: event.target.value }))} />
          <SelectField label="Type" value={form.type} onChange={(event) => setForm((current) => ({ ...current, type: event.target.value }))}>
            <option value="tcp">TCP</option>
            <option value="udp">UDP</option>
            <option value="http">HTTP</option>
            <option value="https">HTTPS</option>
          </SelectField>
          <TextField label={usesPort ? 'Entry port' : 'Entry host'} value={usesPort ? form.entryPort : form.entryHost} error={usesPort ? fieldErrors?.entryPort : fieldErrors?.entryHost} onChange={(event) => setForm((current) => usesPort ? { ...current, entryPort: event.target.value } : { ...current, entryHost: event.target.value })} />
          <TextField label="Target host" value={form.targetHost} error={fieldErrors?.targetHost} onChange={(event) => setForm((current) => ({ ...current, targetHost: event.target.value }))} />
          <TextField label="Target port" value={form.targetPort} error={fieldErrors?.targetPort} onChange={(event) => setForm((current) => ({ ...current, targetPort: event.target.value }))} />
          <TextAreaField label="Description" value={form.description} onChange={(event) => setForm((current) => ({ ...current, description: event.target.value }))} />
        </div>
      </Dialog>
    </section>
  );
}
