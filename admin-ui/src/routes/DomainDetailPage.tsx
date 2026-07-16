import { useMemo, useState } from 'react';
import { LinkOutlined, PlusOutlined, ReloadOutlined } from '@ant-design/icons';
import { Button, Switch } from 'antd';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { useQueryClient } from '@tanstack/react-query';
import { CertificateSelectField } from '../components/CertificateSelectField';
import { ConfirmButton } from '../components/ConfirmButton';
import { Dialog } from '../components/Dialog';
import { SelectField, TextField } from '../components/FormField';
import { ErrorState, NotFoundState, PageLoading, ValidationBanner } from '../components/PageStates';
import { useAuthedQuery } from '../hooks/useAuthedQuery';
import { useMutationWithAuth } from '../hooks/useMutationWithAuth';
import {
  mutateBindDomainCertificate,
  mutateCreateDomainEntry,
  mutateCreateProxy,
  mutateDeleteDomain,
  mutateDeleteDomainEntry,
  mutateDisableDomain,
  mutateEnableDomain,
  mutateUnbindDomainCertificate,
  mutateUpdateDomain,
  mutateUpdateDomainEntry,
  queryClients,
  queryDomain,
  queryProxyEntryOptions,
} from '../lib/admin-graphql';
import { isApiError, isNotFoundError, type DomainEntry, type ProxyRecord } from '../lib/contracts';
import { useSession } from '../session';
import { DetailBackLink, PageHeader, StatusBadge, Timestamp } from './shared';

type EntryForm = {
  protocol: string;
  bindHost: string;
  port: string;
};

type ProxyForm = {
  name: string;
  clientId: string;
  pathPrefix: string;
  stripPrefix: boolean;
  upstreamPathPrefix: string;
  targetHost: string;
  targetPort: string;
};

const defaultEntryForm = (): EntryForm => ({ protocol: 'https', bindHost: '', port: '' });
const defaultProxyForm = (): ProxyForm => ({
  name: '',
  clientId: '',
  pathPrefix: '/',
  stripPrefix: false,
  upstreamPathPrefix: '/',
  targetHost: '',
  targetPort: '',
});

export function DomainDetailPage() {
  const { id = '' } = useParams();
  const session = useSession();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [entryDialog, setEntryDialog] = useState(false);
  const [proxyDialog, setProxyDialog] = useState(false);
  const [editHost, setEditHost] = useState('');
  const [editOpen, setEditOpen] = useState(false);
  const [entryForm, setEntryForm] = useState<EntryForm>(defaultEntryForm());
  const [proxyForm, setProxyForm] = useState<ProxyForm>(defaultProxyForm());
  const [certificateId, setCertificateId] = useState('');
  const [fieldErrors, setFieldErrors] = useState<Record<string, string> | undefined>();

  const domainQuery = useAuthedQuery({
    queryKey: ['domain', id],
    queryFn: () => queryDomain(id),
    enabled: Boolean(id),
    refetchInterval: (session.pollIntervalSeconds ?? 0) * 1000 || false,
  });
  const clientsQuery = useAuthedQuery({
    queryKey: ['clients', 'domain-detail', domainQuery.data?.userId],
    queryFn: () =>
      queryClients({
        page: { page: 1, pageSize: 100 },
        filter: { userId: domainQuery.data?.userId },
        sort: { field: 'name', direction: 'asc' },
      }),
    enabled: Boolean(domainQuery.data?.userId),
  });
  const entryOptionsQuery = useAuthedQuery({
    queryKey: ['proxy-entry-options'],
    queryFn: () => queryProxyEntryOptions(),
  });

  const invalidate = async () => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ['domain', id] }),
      queryClient.invalidateQueries({ queryKey: ['domains'] }),
      queryClient.invalidateQueries({ queryKey: ['proxies'] }),
      queryClient.invalidateQueries({ queryKey: ['certificates'] }),
    ]);
  };

  const updateDomainMutation = useMutationWithAuth({
    mutationFn: () => mutateUpdateDomain(session.csrfToken ?? '', { id, host: editHost.trim().toLowerCase() }),
    onSuccess: async () => {
      setEditOpen(false);
      await invalidate();
    },
    onError: (error) => {
      if (isApiError(error)) setFieldErrors(error.fields);
    },
  });

  const enableMutation = useMutationWithAuth({
    mutationFn: () => mutateEnableDomain(session.csrfToken ?? '', id),
    onSuccess: invalidate,
  });
  const disableMutation = useMutationWithAuth({
    mutationFn: () => mutateDisableDomain(session.csrfToken ?? '', id),
    onSuccess: invalidate,
  });
  const deleteMutation = useMutationWithAuth({
    mutationFn: () => mutateDeleteDomain(session.csrfToken ?? '', id),
    onSuccess: () => navigate('/domains'),
  });

  const createEntryMutation = useMutationWithAuth({
    mutationFn: () =>
      mutateCreateDomainEntry(session.csrfToken ?? '', {
        domainId: id,
        protocol: entryForm.protocol,
        bindHost: entryForm.bindHost || undefined,
        port: Number(entryForm.port),
      }),
    onSuccess: async () => {
      setEntryDialog(false);
      setEntryForm(defaultEntryForm());
      await invalidate();
    },
    onError: (error) => {
      if (isApiError(error)) setFieldErrors(error.fields);
    },
  });

  const toggleEntryMutation = useMutationWithAuth({
    mutationFn: (entry: DomainEntry) =>
      mutateUpdateDomainEntry(session.csrfToken ?? '', {
        id: entry.id,
        status: entry.status === 'enabled' ? 'disabled' : 'enabled',
      }),
    onSuccess: invalidate,
  });

  const deleteEntryMutation = useMutationWithAuth({
    mutationFn: (entryId: string) => mutateDeleteDomainEntry(session.csrfToken ?? '', entryId),
    onSuccess: invalidate,
  });

  const bindCertMutation = useMutationWithAuth({
    mutationFn: () => mutateBindDomainCertificate(session.csrfToken ?? '', { domainId: id, certificateId }),
    onSuccess: async () => {
      setCertificateId('');
      await invalidate();
    },
  });
  const unbindCertMutation = useMutationWithAuth({
    mutationFn: () => mutateUnbindDomainCertificate(session.csrfToken ?? '', id),
    onSuccess: invalidate,
  });

  const createProxyMutation = useMutationWithAuth({
    mutationFn: () =>
      mutateCreateProxy(session.csrfToken ?? '', {
        userId: domainQuery.data!.userId,
        clientId: proxyForm.clientId,
        name: proxyForm.name,
        type: 'web',
        config: {
          domainId: id,
          pathPrefix: proxyForm.pathPrefix || '/',
          stripPrefix: proxyForm.stripPrefix,
          upstreamPathPrefix: proxyForm.upstreamPathPrefix || '/',
          targetHost: proxyForm.targetHost,
          targetPort: Number(proxyForm.targetPort),
        },
      }),
    onSuccess: async (result: any) => {
      setProxyDialog(false);
      setProxyForm(defaultProxyForm());
      await invalidate();
      const proxyId = result?.createProxy?.proxyId ?? result?.proxyId;
      if (proxyId) navigate(`/proxies/${proxyId}`);
    },
    onError: (error) => {
      if (isApiError(error)) setFieldErrors(error.fields);
    },
  });

  const domain = domainQuery.data;
  const clients = (clientsQuery.data?.items ?? []).filter((client) => client.userId === domain?.userId);
  const hostOptions = useMemo(() => {
    const hosts = entryOptionsQuery.data?.hosts ?? [];
    return [{ value: '', label: 'Default bind' }, ...hosts.map((host) => ({ value: host.value, label: host.label }))];
  }, [entryOptionsQuery.data]);

  if (domainQuery.isLoading) return <PageLoading label="Loading domain..." />;
  if (domainQuery.isError) {
    if (isNotFoundError(domainQuery.error)) return <NotFoundState resource="Domain" />;
    return <ErrorState title="Failed to load domain" message={domainQuery.error instanceof Error ? domainQuery.error.message : 'Request failed'} retry={() => domainQuery.refetch()} />;
  }
  if (!domain) return <NotFoundState resource="Domain" />;

  const entries = domain.entries ?? [];
  const proxies = domain.proxies ?? [];

  return (
    <section className="page-section">
      <DetailBackLink to="/domains" label="Back to domains" />
      <PageHeader
        title={domain.host}
        description="One host identity for HTTP and HTTPS. Attach listeners, bind a certificate, then add path proxies."
        actions={
          <>
            <Button icon={<ReloadOutlined />} onClick={() => domainQuery.refetch()}>
              Refresh
            </Button>
            <Button
              onClick={() => {
                setEditHost(domain.host);
                setFieldErrors(undefined);
                setEditOpen(true);
              }}
            >
              Rename host
            </Button>
            {domain.status === 'enabled' ? (
              <ConfirmButton label="Disable" confirmLabel="Disable domain? All entries stop accepting traffic." onConfirm={() => disableMutation.mutate(undefined)} tone="secondary" />
            ) : (
              <Button onClick={() => enableMutation.mutate(undefined)}>Enable</Button>
            )}
            <ConfirmButton
              label="Delete"
              confirmLabel="Delete domain? Domain must be disabled and have no enabled proxies."
              onConfirm={() => deleteMutation.mutate(undefined)}
            />
          </>
        }
      />

      <div className="detail-grid detail-grid--wide">
        <div className="panel">
          <h2>Overview</h2>
          <dl className="detail-list">
            <div>
              <dt>Status</dt>
              <dd>
                <StatusBadge value={domain.status} />
              </dd>
            </div>
            <div>
              <dt>Owner</dt>
              <dd className="mono">{domain.userId}</dd>
            </div>
            <div>
              <dt>Path proxies</dt>
              <dd>{domain.proxyCount}</dd>
            </div>
            <div>
              <dt>Updated</dt>
              <dd>
                <Timestamp value={domain.updatedAt} />
              </dd>
            </div>
          </dl>
        </div>

        <div className="panel">
          <div className="panel__header">
            <h2>Certificate</h2>
            {domain.certificateId ? (
              <ConfirmButton label="Unbind" confirmLabel="Unbind certificate? HTTPS entries will fail closed until another certificate is bound." onConfirm={() => unbindCertMutation.mutate(undefined)} />
            ) : null}
          </div>
          {domain.certificateId ? (
            <dl className="detail-list">
              <div>
                <dt>Bound certificate</dt>
                <dd className="mono">{domain.certificateId}</dd>
              </div>
              <div>
                <dt>Serving</dt>
                <dd>
                  <StatusBadge value={domain.certificate?.servingStatus || domain.certificate?.status || 'unknown'} />
                </dd>
              </div>
              <div>
                <dt>Hostnames</dt>
                <dd>{(domain.certificate?.hostnames ?? [domain.certificate?.host]).filter(Boolean).join(', ') || '—'}</dd>
              </div>
            </dl>
          ) : (
            <div className="stack">
              <p className="muted">HTTPS entries require a certificate that covers this host. Bind one below or create a new certificate.</p>
              <CertificateSelectField
                entryHost={domain.host ?? ''}
                domainId={domain.id}
                value={certificateId}
                onChange={setCertificateId}
              />
              <Button type="primary" disabled={!certificateId} loading={bindCertMutation.isPending} onClick={() => bindCertMutation.mutate(undefined)}>
                Bind certificate
              </Button>
            </div>
          )}
        </div>

        <div className="panel">
          <div className="panel__header">
            <h2>Listeners</h2>
            <Button size="small" icon={<PlusOutlined />} onClick={() => { setFieldErrors(undefined); setEntryDialog(true); }}>
              Add entry
            </Button>
          </div>
          {entries.length === 0 ? (
            <p className="muted">No listeners yet. Add an HTTP and/or HTTPS entry so traffic can reach this host.</p>
          ) : (
            <div className="table-wrap">
              <table className="table">
                <thead>
                  <tr>
                    <th>Protocol</th>
                    <th>Bind</th>
                    <th>Port</th>
                    <th>Status</th>
                    <th>Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {entries.map((entry) => (
                    <tr key={entry.id}>
                      <td>
                        <StatusBadge value={entry.protocol} />
                      </td>
                      <td className="mono">{entry.bindHost || 'default'}</td>
                      <td>{entry.port}</td>
                      <td>
                        <StatusBadge value={entry.status} />
                      </td>
                      <td>
                        <div className="inline-actions">
                          <Button size="small" onClick={() => toggleEntryMutation.mutate(entry)}>
                            {entry.status === 'enabled' ? 'Disable' : 'Enable'}
                          </Button>
                          <ConfirmButton label="Delete" confirmLabel="Delete entry?" onConfirm={() => deleteEntryMutation.mutate(entry.id)} />
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>

        <div className="panel">
          <div className="panel__header">
            <h2>Path proxies</h2>
            <Button
              type="primary"
              size="small"
              icon={<PlusOutlined />}
              onClick={() => {
                setFieldErrors(undefined);
                setProxyForm(defaultProxyForm());
                setProxyDialog(true);
              }}
            >
              Add path proxy
            </Button>
          </div>
          <p className="muted">Each path is an independent proxy under this domain. Longest path prefix wins (`/api` does not match `/apix`).</p>
          {proxies.length === 0 ? (
            <p className="muted">No path proxies yet. Add `/` for the site root, then more specific paths as needed.</p>
          ) : (
            <div className="table-wrap">
              <table className="table">
                <thead>
                  <tr>
                    <th>Path</th>
                    <th>Name</th>
                    <th>Client</th>
                    <th>Target</th>
                    <th>Status</th>
                    <th></th>
                  </tr>
                </thead>
                <tbody>
                  {proxies.map((proxy: ProxyRecord) => (
                    <tr key={proxy.id}>
                      <td className="mono">{proxy.config.pathPrefix || '/'}</td>
                      <td>
                        <Link to={`/proxies/${proxy.id}`}>{proxy.name}</Link>
                      </td>
                      <td className="mono muted">{proxy.clientId}</td>
                      <td className="mono">
                        {proxy.config.targetHost}:{proxy.config.targetPort}
                      </td>
                      <td>
                        <StatusBadge value={proxy.runtimeStatus || proxy.status} />
                      </td>
                      <td>
                        <Link to={`/proxies/${proxy.id}`}>
                          <LinkOutlined /> Open
                        </Link>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      </div>

      <Dialog
        open={editOpen}
        title="Rename domain host"
        onClose={() => setEditOpen(false)}
        footer={
          <>
            <Button onClick={() => setEditOpen(false)}>Cancel</Button>
            <Button type="primary" loading={updateDomainMutation.isPending} onClick={() => updateDomainMutation.mutate(undefined)}>
              Save host
            </Button>
          </>
        }
      >
        {updateDomainMutation.isError && isApiError(updateDomainMutation.error) ? (
          <ValidationBanner title={updateDomainMutation.error.message} fields={fieldErrors} />
        ) : null}
        <TextField label="Hostname" value={editHost} error={fieldErrors?.host} onChange={(event) => setEditHost(event.target.value)} />
      </Dialog>

      <Dialog
        open={entryDialog}
        title="Add domain entry"
        onClose={() => setEntryDialog(false)}
        footer={
          <>
            <Button onClick={() => setEntryDialog(false)}>Cancel</Button>
            <Button type="primary" loading={createEntryMutation.isPending} onClick={() => createEntryMutation.mutate(undefined)}>
              Add entry
            </Button>
          </>
        }
      >
        <div className="stack">
          <p className="muted">Entries define which bind address and port expose this host. HTTP and HTTPS share the same path map.</p>
          {createEntryMutation.isError && isApiError(createEntryMutation.error) ? (
            <ValidationBanner title={createEntryMutation.error.message} fields={fieldErrors} />
          ) : null}
          <div className="toolbar-grid">
            <SelectField
              label="Protocol"
              value={entryForm.protocol}
              onChange={(event) => setEntryForm((current) => ({ ...current, protocol: event.target.value }))}
            >
              <option value="http">HTTP</option>
              <option value="https">HTTPS</option>
            </SelectField>
            <SelectField
              label="Bind host"
              value={entryForm.bindHost}
              onChange={(event) => setEntryForm((current) => ({ ...current, bindHost: event.target.value }))}
            >
              {hostOptions.map((option) => (
                <option key={option.value || '__default'} value={option.value}>
                  {option.label}
                </option>
              ))}
            </SelectField>
            <TextField
              label="Port"
              value={entryForm.port}
              error={fieldErrors?.port || fieldErrors?.entry}
              placeholder={entryForm.protocol === 'https' ? '443' : '80'}
              onChange={(event) => setEntryForm((current) => ({ ...current, port: event.target.value }))}
            />
          </div>
          {entryForm.protocol === 'https' && !domain.certificateId ? (
            <p className="banner banner--warning">HTTPS entry requires a bound certificate on this domain first.</p>
          ) : null}
        </div>
      </Dialog>

      <Dialog
        open={proxyDialog}
        title="Add path proxy"
        onClose={() => setProxyDialog(false)}
        footer={
          <>
            <Button onClick={() => setProxyDialog(false)}>Cancel</Button>
            <Button type="primary" loading={createProxyMutation.isPending} onClick={() => createProxyMutation.mutate(undefined)}>
              Create path proxy
            </Button>
          </>
        }
      >
        <div className="stack">
          <p className="muted">
            This proxy serves <strong>{domain.host}</strong> for one path prefix. Prefer `/` for the site root and more specific paths for APIs.
          </p>
          {createProxyMutation.isError && isApiError(createProxyMutation.error) ? (
            <ValidationBanner title={createProxyMutation.error.message} fields={fieldErrors} />
          ) : null}
          <div className="toolbar-grid">
            <TextField label="Name" value={proxyForm.name} error={fieldErrors?.name} onChange={(event) => setProxyForm((current) => ({ ...current, name: event.target.value }))} />
            <SelectField
              label="Provider client"
              value={proxyForm.clientId}
              error={fieldErrors?.clientId}
              onChange={(event) => setProxyForm((current) => ({ ...current, clientId: event.target.value }))}
            >
              <option value="">Select client</option>
              {clients.map((client) => (
                <option key={client.id} value={client.id}>{client.name}</option>
              ))}
            </SelectField>
            <TextField
              label="Path prefix"
              value={proxyForm.pathPrefix}
              error={fieldErrors?.pathPrefix}
              placeholder="/"
              onChange={(event) => setProxyForm((current) => ({ ...current, pathPrefix: event.target.value }))}
              hint="Must start with /. Longest prefix wins."
            />
            <TextField
              label="Upstream path prefix"
              value={proxyForm.upstreamPathPrefix}
              error={fieldErrors?.upstreamPathPrefix}
              onChange={(event) => setProxyForm((current) => ({ ...current, upstreamPathPrefix: event.target.value }))}
            />
            <TextField label="Target host" value={proxyForm.targetHost} error={fieldErrors?.targetHost} onChange={(event) => setProxyForm((current) => ({ ...current, targetHost: event.target.value }))} />
            <TextField label="Target port" value={proxyForm.targetPort} error={fieldErrors?.targetPort} onChange={(event) => setProxyForm((current) => ({ ...current, targetPort: event.target.value }))} />
          </div>
          <label className="field">
            <span className="field__label">Strip path prefix before upstream</span>
            <Switch checked={proxyForm.stripPrefix} onChange={(checked) => setProxyForm((current) => ({ ...current, stripPrefix: checked }))} />
          </label>
        </div>
      </Dialog>
    </section>
  );
}
