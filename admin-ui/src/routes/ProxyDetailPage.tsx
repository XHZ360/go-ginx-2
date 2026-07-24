import { useMemo, useState } from 'react';
import { EditOutlined, ThunderboltOutlined } from '@ant-design/icons';
import { Button, QRCode, Switch, Tag } from 'antd';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { useQueryClient } from '@tanstack/react-query';
import { ConfirmButton } from '../components/ConfirmButton';
import { Dialog } from '../components/Dialog';
import { SelectField, TextAreaField, TextField } from '../components/FormField';
import { ErrorState, NotFoundState, PageLoading, ValidationBanner } from '../components/PageStates';
import { useAuthedQuery } from '../hooks/useAuthedQuery';
import { useMutationWithAuth } from '../hooks/useMutationWithAuth';
import {
  mutateCreateProxyActivationLink,
  mutateDeleteLocalProxy,
  mutateDeleteProxy,
  mutateDisableLocalProxy,
  mutateDisableProxy,
  mutateDisableProxyAccessAuth,
  mutateEnableLocalProxy,
  mutateEnableProxy,
  mutateEnableProxyAccessAuthAndCreateActivation,
  mutateRevokeAllProxyAccess,
  mutateUpdateLocalProxy,
  mutateUpdateProxy,
  queryDomains,
  queryProxy,
  queryProxyEntryOptions,
} from '../lib/admin-graphql';
import { isApiError, isNotFoundError, type ProxyActivation, type ProxyEntryHostOption, type ProxyRecord } from '../lib/contracts';
import { formatBytes } from '../lib/format';
import { useSession } from '../session';
import { DetailBackLink, PageHeader, StatusBadge, Timestamp } from './shared';

type EditForm = {
  name: string;
  description: string;
  domainId: string;
  pathPrefix: string;
  stripPrefix: boolean;
  upstreamPathPrefix: string;
  entryBindHost: string;
  entryPort: string;
  targetHost: string;
  targetPort: string;
};

function isWebProxy(type: string) {
  return type === 'web' || type === 'http' || type === 'https';
}

function proxyRouteLabel(proxy: ProxyRecord) {
  if (isWebProxy(proxy.type)) {
    const host = proxy.config.entryHost || proxy.config.domainId || 'domain';
    return `${host}${proxy.config.pathPrefix || '/'}`;
  }
  return `${proxy.config.entryBindHost || 'default'}:${proxy.config.entryPort ?? 'default'}`;
}

function hostOptionsWithCurrent(options: ProxyEntryHostOption[], current: string) {
  if (!current || options.some((option) => option.value === current)) {
    return options;
  }
  return [{ value: current, label: current, isDefault: false }, ...options];
}

function formFromProxy(proxy: ProxyRecord): EditForm {
  return {
    name: proxy.name,
    description: proxy.description ?? '',
    domainId: proxy.config.domainId ?? '',
    pathPrefix: proxy.config.pathPrefix ?? '/',
    stripPrefix: Boolean(proxy.config.stripPrefix),
    upstreamPathPrefix: proxy.config.upstreamPathPrefix ?? '/',
    entryBindHost: proxy.config.entryBindHost ?? '',
    entryPort: proxy.config.entryPort != null ? String(proxy.config.entryPort) : '',
    targetHost: proxy.config.targetHost ?? '',
    targetPort: proxy.config.targetPort != null ? String(proxy.config.targetPort) : '',
  };
}

export function ProxyDetailPage() {
  const { id = '' } = useParams();
  const session = useSession();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [editing, setEditing] = useState(false);
  const [form, setForm] = useState<EditForm | null>(null);
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>();
  const [formError, setFormError] = useState<string>();
  const [activation, setActivation] = useState<ProxyActivation | null>(null);

  const query = useAuthedQuery({
    queryKey: ['proxy', id],
    queryFn: () => queryProxy(id),
    enabled: Boolean(id),
    refetchInterval: (session.pollIntervalSeconds ?? 0) * 1000 || false,
  });
  const domainsQuery = useAuthedQuery({
    queryKey: ['proxy-detail-domains', query.data?.userId],
    queryFn: () =>
      queryDomains({
        page: { page: 1, pageSize: 100 },
        filter: query.data?.userId ? { userId: query.data.userId } : {},
        sort: { field: 'host', direction: 'asc' },
      }),
    enabled: Boolean(query.data?.userId),
  });
  const entryOptionsQuery = useAuthedQuery({
    queryKey: ['proxy-entry-options'],
    queryFn: () => queryProxyEntryOptions(),
  });

  const invalidate = async () => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ['proxy', id] }),
      queryClient.invalidateQueries({ queryKey: ['proxies'] }),
      queryClient.invalidateQueries({ queryKey: ['domains'] }),
    ]);
  };

  const updateMutation = useMutationWithAuth({
    mutationFn: async () => {
      if (!form || !query.data) throw new Error('missing form');
      const web = isWebProxy(query.data.type);
      if (query.data.isSystem) {
        await mutateUpdateLocalProxy(session.csrfToken ?? '', {
          id,
          name: form.name,
          type: query.data.type,
          description: form.description,
          entryBindHost: form.entryBindHost || undefined,
          entryPort: Number(form.entryPort),
          targetHost: form.targetHost,
          targetPort: Number(form.targetPort),
        });
        return;
      }
      await mutateUpdateProxy(session.csrfToken ?? '', {
        id,
        type: query.data.type,
        name: form.name,
        description: form.description,
        config: web
          ? {
              domainId: form.domainId,
              pathPrefix: form.pathPrefix || '/',
              stripPrefix: form.stripPrefix,
              upstreamPathPrefix: form.upstreamPathPrefix || '/',
              targetHost: form.targetHost,
              targetPort: Number(form.targetPort),
            }
          : {
              entryBindHost: form.entryBindHost,
              entryPort: form.entryPort ? Number(form.entryPort) : undefined,
              targetHost: form.targetHost,
              targetPort: Number(form.targetPort),
            },
      });
    },
    onSuccess: async () => {
      setEditing(false);
      setFormError(undefined);
      setFieldErrors(undefined);
      await invalidate();
    },
    onError: (error) => {
      if (isApiError(error)) {
        setFieldErrors(error.fields);
        setFormError(error.message);
      }
    },
  });

  const enableMutation = useMutationWithAuth({
    mutationFn: async () => {
      if (query.data?.isSystem) await mutateEnableLocalProxy(session.csrfToken ?? '', id);
      else await mutateEnableProxy(session.csrfToken ?? '', id);
    },
    onSuccess: invalidate,
  });
  const disableMutation = useMutationWithAuth({
    mutationFn: async () => {
      if (query.data?.isSystem) await mutateDisableLocalProxy(session.csrfToken ?? '', id);
      else await mutateDisableProxy(session.csrfToken ?? '', id);
    },
    onSuccess: invalidate,
  });
  const deleteMutation = useMutationWithAuth({
    mutationFn: async () => {
      if (query.data?.isSystem) await mutateDeleteLocalProxy(session.csrfToken ?? '', id);
      else await mutateDeleteProxy(session.csrfToken ?? '', id);
    },
    onSuccess: () => navigate('/proxies'),
  });

  const enableAuthMutation = useMutationWithAuth({
    mutationFn: () => mutateEnableProxyAccessAuthAndCreateActivation(session.csrfToken ?? '', id),
    onSuccess: async (result: any) => {
      setActivation(result.enableProxyAccessAuthAndCreateActivation ?? result);
      await invalidate();
    },
  });
  const createLinkMutation = useMutationWithAuth({
    mutationFn: () => mutateCreateProxyActivationLink(session.csrfToken ?? '', id),
    onSuccess: (result: any) => {
      setActivation(result.createProxyActivationLink ?? result);
    },
  });
  const revokeAuthMutation = useMutationWithAuth({
    mutationFn: () => mutateRevokeAllProxyAccess(session.csrfToken ?? '', id),
    onSuccess: invalidate,
  });
  const disableAuthMutation = useMutationWithAuth({
    mutationFn: () => mutateDisableProxyAccessAuth(session.csrfToken ?? '', id),
    onSuccess: async () => {
      setActivation(null);
      await invalidate();
    },
  });

  const domains = domainsQuery.data?.items ?? [];
  const hostOptions = useMemo(
    () => hostOptionsWithCurrent(entryOptionsQuery.data?.hosts ?? [{ value: '', label: 'Default listener host', isDefault: true }], form?.entryBindHost ?? ''),
    [entryOptionsQuery.data, form?.entryBindHost],
  );

  if (query.isLoading) return <PageLoading label="Loading proxy..." />;
  if (query.isError) {
    if (isNotFoundError(query.error)) return <NotFoundState resource="Proxy" />;
    return <ErrorState title="Failed to load proxy" message={query.error instanceof Error ? query.error.message : 'Request failed'} retry={() => query.refetch()} />;
  }
  const proxy = query.data;
  if (!proxy) return <NotFoundState resource="Proxy" />;

  const web = isWebProxy(proxy.type);
  const domainId = proxy.config.domainId;

  return (
    <section className="page-section">
      <DetailBackLink to="/proxies" label="Back to proxies" />
      <PageHeader
        title={proxy.name}
        description={web ? `Web path proxy · ${proxyRouteLabel(proxy)}` : `Proxy ID: ${proxy.id}`}
        actions={
          <>
            {proxy.isSystem ? <Tag color="blue">System proxy</Tag> : null}
            <Button
              type="default"
              icon={<EditOutlined aria-hidden="true" />}
              onClick={() => {
                setForm(formFromProxy(proxy));
                setFieldErrors(undefined);
                setFormError(undefined);
                setEditing(true);
              }}
            >
              Edit proxy
            </Button>
            {proxy.status === 'disabled' ? (
              <Button type="default" icon={<ThunderboltOutlined aria-hidden="true" />} onClick={() => enableMutation.mutate(undefined)}>
                Enable
              </Button>
            ) : (
              <ConfirmButton label="Disable" confirmLabel="Disable this proxy?" onConfirm={() => disableMutation.mutate(undefined)} tone="secondary" />
            )}
            <ConfirmButton label="Delete" confirmLabel="Delete disabled proxy?" onConfirm={() => deleteMutation.mutate(undefined)} />
          </>
        }
      />

      <div className="detail-grid detail-grid--wide">
        <article className="panel">
          <h2>Configuration</h2>
          <dl className="detail-list">
            <div>
              <dt>Type</dt>
              <dd>{proxy.type}</dd>
            </div>
            <div>
              <dt>Status</dt>
              <dd>
                <StatusBadge value={proxy.status} />
              </dd>
            </div>
            <div>
              <dt>Runtime</dt>
              <dd>
                <StatusBadge value={proxy.runtimeStatus} />
              </dd>
            </div>
            {web ? (
              <>
                <div>
                  <dt>Domain</dt>
                  <dd>
                    {domainId ? <Link to={`/domains/${domainId}`}>{proxy.config.entryHost || domainId}</Link> : proxy.config.entryHost || '—'}
                  </dd>
                </div>
                <div>
                  <dt>Path prefix</dt>
                  <dd className="mono">{proxy.config.pathPrefix || '/'}</dd>
                </div>
                <div>
                  <dt>Path rewrite</dt>
                  <dd>
                    {proxy.config.stripPrefix ? 'strip prefix' : 'keep path'} → {proxy.config.upstreamPathPrefix || '/'}
                  </dd>
                </div>
              </>
            ) : (
              <div>
                <dt>Entry</dt>
                <dd>{proxyRouteLabel(proxy)}</dd>
              </div>
            )}
            <div>
              <dt>Client</dt>
              <dd className="mono">{proxy.clientId}</dd>
            </div>
            <div>
              <dt>Target</dt>
              <dd className="mono">
                {proxy.config.targetHost}:{proxy.config.targetPort}
              </dd>
            </div>
            <div>
              <dt>Description</dt>
              <dd>{proxy.description || 'No description'}</dd>
            </div>
          </dl>
          {web ? (
            <p className="muted">
              Certificates and HTTP/HTTPS listeners live on the{' '}
              {domainId ? <Link to={`/domains/${domainId}`}>domain page</Link> : 'domain page'}.
            </p>
          ) : null}
        </article>

        <article className="panel">
          <h2>Runtime stats</h2>
          <dl className="detail-list">
            <div>
              <dt>Active TCP</dt>
              <dd>{proxy.activeTCPConnections}</dd>
            </div>
            <div>
              <dt>Upload</dt>
              <dd>{formatBytes(proxy.uploadBytes)}</dd>
            </div>
            <div>
              <dt>Download</dt>
              <dd>{formatBytes(proxy.downloadBytes)}</dd>
            </div>
            <div>
              <dt>HTTP errors</dt>
              <dd>{proxy.httpErrorCount}</dd>
            </div>
            <div>
              <dt>Updated</dt>
              <dd>
                <Timestamp value={proxy.updatedAt} />
              </dd>
            </div>
          </dl>
          {(proxy as any).statsLegacyAggregate || proxy.description?.includes('legacy') ? (
            <p className="banner banner--warning">Historical totals may include pre-migration traffic from all paths on this host.</p>
          ) : null}
        </article>

        {web ? (
          <article className="panel">
            <h2>Access activation</h2>
            <dl className="detail-list">
              <div>
                <dt>Status</dt>
                <dd>{proxy.accessAuthEnabled ? 'Enabled' : 'Disabled'}</dd>
              </div>
              {proxy.accessAuthEnabled ? (
                <div>
                  <dt>Auth version</dt>
                  <dd className="mono">{proxy.accessAuthVersion ?? 0}</dd>
                </div>
              ) : null}
            </dl>
            {proxy.accessAuthEnabled && (proxy.accessAuthVersion ?? 0) > 0 ? (
              <p className="banner banner--warning">
                Domain/Path changes revoke existing cookies and activation links. Create a new activation link after identity changes.
              </p>
            ) : null}
            <div className="inline-actions">
              {!proxy.accessAuthEnabled ? (
                <Button type="primary" loading={enableAuthMutation.isPending} onClick={() => enableAuthMutation.mutate(undefined)}>
                  Enable and create activation link
                </Button>
              ) : (
                <>
                  <Button loading={createLinkMutation.isPending} onClick={() => createLinkMutation.mutate(undefined)}>
                    New activation link
                  </Button>
                  <ConfirmButton label="Revoke all access" confirmLabel="Revoke all access cookies?" onConfirm={() => revokeAuthMutation.mutate(undefined)} />
                  <ConfirmButton label="Disable auth" confirmLabel="Disable access authentication?" onConfirm={() => disableAuthMutation.mutate(undefined)} tone="secondary" />
                </>
              )}
            </div>
            <p className="muted">Requires an HTTPS domain entry and bound certificate. HTTP visitors are redirected with 308 when HTTPS is available.</p>
          </article>
        ) : null}
      </div>

      <Dialog
        open={editing && Boolean(form)}
        title="Edit proxy"
        onClose={() => setEditing(false)}
        footer={
          <>
            <Button onClick={() => setEditing(false)}>Cancel</Button>
            <Button type="primary" loading={updateMutation.isPending} onClick={() => updateMutation.mutate(undefined)}>
              Save changes
            </Button>
          </>
        }
      >
        {formError ? <div className="banner banner--danger">{formError}</div> : null}
        <ValidationBanner fields={fieldErrors} />
        {form ? (
          <div className="toolbar-grid toolbar-grid--wide">
            <TextField label="Name" value={form.name} error={fieldErrors?.name} onChange={(event) => setForm((current) => current && { ...current, name: event.target.value })} />
            {web ? (
              <>
                <SelectField
                  label="Domain"
                  value={form.domainId}
                  error={fieldErrors?.domainId}
                  onChange={(event) => setForm((current) => current && { ...current, domainId: event.target.value })}
                >
                  <option value="">Select domain</option>
                  {domains.map((domain) => (
                    <option key={domain.id} value={domain.id}>{domain.host}</option>
                  ))}
                </SelectField>
                <TextField label="Path prefix" value={form.pathPrefix} error={fieldErrors?.pathPrefix} onChange={(event) => setForm((current) => current && { ...current, pathPrefix: event.target.value })} />
                <TextField label="Upstream path prefix" value={form.upstreamPathPrefix} error={fieldErrors?.upstreamPathPrefix} onChange={(event) => setForm((current) => current && { ...current, upstreamPathPrefix: event.target.value })} />
                <label className="field">
                  <span className="field__label">Strip path prefix</span>
                  <Switch checked={form.stripPrefix} onChange={(checked) => setForm((current) => current && { ...current, stripPrefix: checked })} />
                </label>
              </>
            ) : (
              <>
                <SelectField
                  label="Bind host"
                  value={form.entryBindHost}
                  error={fieldErrors?.entryBindHost}
                  onChange={(event) => setForm((current) => current && { ...current, entryBindHost: event.target.value })}
                >
                  {hostOptions.map((option) => (
                    <option key={option.value || '__default'} value={option.value}>
                      {option.label}
                    </option>
                  ))}
                </SelectField>
                <TextField label="Entry port" value={form.entryPort} error={fieldErrors?.entryPort} onChange={(event) => setForm((current) => current && { ...current, entryPort: event.target.value })} />
              </>
            )}
            <TextField label="Target host" value={form.targetHost} error={fieldErrors?.targetHost} onChange={(event) => setForm((current) => current && { ...current, targetHost: event.target.value })} />
            <TextField label="Target port" value={form.targetPort} error={fieldErrors?.targetPort} onChange={(event) => setForm((current) => current && { ...current, targetPort: event.target.value })} />
            <TextAreaField label="Description" value={form.description} onChange={(event) => setForm((current) => current && { ...current, description: event.target.value })} />
          </div>
        ) : null}
      </Dialog>

      <Dialog
        open={Boolean(activation?.url)}
        title="Activation link"
        onClose={() => setActivation(null)}
        footer={
          <Button type="primary" onClick={() => setActivation(null)}>
            Done
          </Button>
        }
      >
        {activation?.url ? (
          <div className="stack">
            <p className="muted">Share once. The full URL is not stored again after you close this dialog.</p>
            <code className="mono">{activation.url}</code>
            <div className="qr-wrap">
              <QRCode value={activation.url} />
            </div>
            {activation.expiresAt ? (
              <p className="muted">
                Expires <Timestamp value={activation.expiresAt} />
              </p>
            ) : null}
          </div>
        ) : null}
      </Dialog>
    </section>
  );
}
