import { useEffect, useMemo, useState } from 'react';
import { EditOutlined, PlusOutlined, ThunderboltOutlined } from '@ant-design/icons';
import { Button, QRCode, Switch } from 'antd';
import { useNavigate, useParams, useSearchParams } from 'react-router-dom';
import { useQueryClient } from '@tanstack/react-query';
import { ConfirmButton } from '../components/ConfirmButton';
import { Dialog } from '../components/Dialog';
import { CertificateSelectField } from '../components/CertificateSelectField';
import { SelectField, TextAreaField, TextField } from '../components/FormField';
import { ErrorState, NotFoundState, PageLoading, ValidationBanner } from '../components/PageStates';
import { useAuthedQuery } from '../hooks/useAuthedQuery';
import { useMutationWithAuth } from '../hooks/useMutationWithAuth';
import {
  mutateCreateProxyActivationLink,
  mutateCreateProxyRoute,
  mutateDeleteProxy,
  mutateDeleteProxyRoute,
  mutateDisableProxy,
  mutateDisableProxyAccessAuth,
  mutateEnableProxy,
  mutateEnableProxyAccessAuthAndCreateActivation,
  mutateRevokeAllProxyAccess,
  mutateUpdateProxy,
  mutateUpdateProxyRoute,
  queryClients,
  queryProxyEntryOptions,
  queryProxy,
} from '../lib/admin-graphql';
import { isApiError, isNotFoundError, type ProxyActivation, type ProxyEntryHostOption, type ProxyRecord, type ProxyRoute } from '../lib/contracts';
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
import { DetailBackLink, PageHeader, StatusBadge, Timestamp } from './shared';

type ProxyEditDraft = {
  name: string;
  description: string;
  entryBindHost: string;
  entryHost: string;
  entryPort: string;
  targetHost: string;
  targetPort: string;
  certificateId: string;
};

type ProxyConfigSubmit = {
  entryBindHost?: string;
  entryHost?: string;
  entryPort?: number;
  targetHost?: string;
  targetPort?: number;
  certificateId?: string;
};

type RouteDraft = {
  id?: string;
  clientId: string;
  pathPrefix: string;
  stripPrefix: boolean;
  upstreamPathPrefix: string;
  targetHost: string;
  targetPort: string;
  status: string;
};

function isRouteProxy(type: string) {
  return type === 'http' || type === 'https';
}

function isHTTPSProxy(type: string) {
  return type === 'https';
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

function buildUpdateConfig(input: ProxyEditDraft, type?: string): ProxyConfigSubmit {
  return {
    entryBindHost: input.entryBindHost || undefined,
    entryHost: input.entryHost || undefined,
    entryPort: input.entryPort ? Number(input.entryPort) : undefined,
    targetHost: input.targetHost || undefined,
    targetPort: input.targetPort ? Number(input.targetPort) : undefined,
    certificateId: type && isHTTPSProxy(type) ? input.certificateId : undefined,
  };
}

function emptyRouteDraft(clientId = ''): RouteDraft {
  return {
    clientId,
    pathPrefix: '/',
    stripPrefix: false,
    upstreamPathPrefix: '/',
    targetHost: '',
    targetPort: '',
    status: 'enabled',
  };
}

function routeDraftFromRoute(route: ProxyRoute): RouteDraft {
  return {
    id: route.id,
    clientId: route.clientId,
    pathPrefix: route.pathPrefix,
    stripPrefix: route.stripPrefix,
    upstreamPathPrefix: route.upstreamPathPrefix || '/',
    targetHost: route.targetHost,
    targetPort: String(route.targetPort),
    status: route.status,
  };
}

export function ProxyDetailPage() {
  const { id = '' } = useParams();
  const navigate = useNavigate();
  const session = useSession();
  const queryClient = useQueryClient();
  const [searchParams, setSearchParams] = useSearchParams();
  const [editing, setEditing] = useState(false);
  const [formError, setFormError] = useState<string>();
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>();
  const [draftNotice, setDraftNotice] = useState<string>();
  const [localForm, setLocalForm] = useState<ProxyEditDraft>({
    name: '',
    description: '',
    entryBindHost: '',
    entryHost: '',
    entryPort: '',
    targetHost: '',
    targetPort: '',
    certificateId: '',
  });
  const [routeDialogOpen, setRouteDialogOpen] = useState(false);
  const [routeDraft, setRouteDraft] = useState<RouteDraft>(emptyRouteDraft());
  const [routeError, setRouteError] = useState<string>();
  const [activation, setActivation] = useState<ProxyActivation>();

  const query = useAuthedQuery({
    queryKey: ['proxy', id],
    queryFn: () => queryProxy(id),
    refetchInterval: session.pollIntervalSeconds * 1000,
  });
  const entryOptionsQuery = useAuthedQuery({
    queryKey: ['proxy-entry-options'],
    queryFn: () => queryProxyEntryOptions(),
  });
  const clientsQuery = useAuthedQuery({
    queryKey: ['clients', 'proxy-routes', query.data?.userId],
    queryFn: () => queryClients({ page: { page: 1, pageSize: 200 }, filter: { userId: query.data?.userId } }),
    enabled: Boolean(query.data?.userId),
  });

  useEffect(() => {
    if (!query.data) {
      return;
    }
    if (editing) {
      return;
    }
    setLocalForm({
      name: query.data.name,
      description: query.data.description ?? '',
      entryBindHost: query.data.config.entryBindHost ?? '',
      entryHost: query.data.config.entryHost ?? '',
      entryPort: query.data.config.entryPort?.toString() ?? '',
      targetHost: query.data.config.targetHost ?? '',
      targetPort: query.data.config.targetPort?.toString() ?? '',
      certificateId: query.data.config.certificateId ?? '',
    });
  }, [query.data, editing]);

  useEffect(() => {
    const createdCertificateId = searchParams.get(CREATED_CERT_PARAM);
    if (!createdCertificateId) {
      return;
    }
    const draftId = searchParams.get(DRAFT_ID_PARAM);
    const draft = loadProxyDraft<ProxyEditDraft>(draftId);
    if (draft) {
      setLocalForm({ ...draft, certificateId: createdCertificateId });
      setDraftNotice(undefined);
    } else {
      const fallback = query.data;
      setLocalForm((current) =>
        fallback
          ? {
              name: fallback.name,
              description: fallback.description ?? '',
              entryBindHost: fallback.config.entryBindHost ?? '',
              entryHost: fallback.config.entryHost ?? '',
              entryPort: fallback.config.entryPort?.toString() ?? '',
              targetHost: fallback.config.targetHost ?? '',
              targetPort: fallback.config.targetPort?.toString() ?? '',
              certificateId: createdCertificateId,
            }
          : { ...current, certificateId: createdCertificateId },
      );
      setDraftNotice('表单草稿已失效，请重新填写；已为你选中新创建的证书。');
    }
    clearProxyDraft(draftId);
    setFieldErrors(undefined);
    setFormError(undefined);
    setEditing(true);
    setSearchParams((current) => {
      const next = new URLSearchParams(current);
      next.delete(CREATED_CERT_PARAM);
      next.delete(DRAFT_ID_PARAM);
      return next;
    }, { replace: true });
  }, [searchParams, setSearchParams]);

  const updateMutation = useMutationWithAuth({
    mutationFn: (input: ProxyEditDraft) =>
      mutateUpdateProxy(session.csrfToken ?? '', {
        id,
        type: query.data?.type,
        name: input.name,
        description: input.description,
        config: buildUpdateConfig(input, query.data?.type),
      }),
    onSuccess: async () => {
      setEditing(false);
      setFieldErrors(undefined);
      setFormError(undefined);
      await queryClient.invalidateQueries({ queryKey: ['proxy', id] });
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
    mutationFn: () => mutateEnableProxy(session.csrfToken ?? '', id),
    onSuccess: async () => {
      setFormError(undefined);
      await queryClient.invalidateQueries({ queryKey: ['proxy', id] });
      await queryClient.invalidateQueries({ queryKey: ['proxies'] });
    },
    onError: (error) => {
      if (isApiError(error)) {
        setFormError(error.code === 'ENTRY_CONFLICT' ? 'Requested listener conflicts with an active listener.' : error.message);
      }
    },
  });

  const disableMutation = useMutationWithAuth({
    mutationFn: () => mutateDisableProxy(session.csrfToken ?? '', id),
    onSuccess: async () => {
      setFormError(undefined);
      await queryClient.invalidateQueries({ queryKey: ['proxy', id] });
      await queryClient.invalidateQueries({ queryKey: ['proxies'] });
    },
    onError: (error) => {
      if (isApiError(error)) {
        setFormError(error.message);
      }
    },
  });

  const deleteMutation = useMutationWithAuth({
    mutationFn: () => mutateDeleteProxy(session.csrfToken ?? '', id),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['proxies'] });
      navigate('/proxies', { replace: true });
    },
    onError: (error) => {
      if (isApiError(error)) {
        setFormError(error.code === 'CONFLICT' ? 'Disable the proxy before deleting it.' : error.message);
      }
    },
  });

  const enableAuthMutation = useMutationWithAuth({
    mutationFn: () => mutateEnableProxyAccessAuthAndCreateActivation(session.csrfToken ?? '', id),
    onSuccess: async (result) => {
      setFormError(undefined);
      setActivation(result);
      await queryClient.invalidateQueries({ queryKey: ['proxy', id] });
    },
    onError: (error) => {
      if (isApiError(error)) {
        setFormError(error.message);
      }
    },
  });

  const createActivationMutation = useMutationWithAuth({
    mutationFn: () => mutateCreateProxyActivationLink(session.csrfToken ?? '', id),
    onSuccess: (result) => {
      setFormError(undefined);
      setActivation(result);
    },
    onError: (error) => {
      if (isApiError(error)) {
        setFormError(error.message);
      }
    },
  });

  const revokeAccessMutation = useMutationWithAuth({
    mutationFn: () => mutateRevokeAllProxyAccess(session.csrfToken ?? '', id),
    onSuccess: async () => {
      setFormError(undefined);
      setActivation(undefined);
      await queryClient.invalidateQueries({ queryKey: ['proxy', id] });
    },
    onError: (error) => {
      if (isApiError(error)) {
        setFormError(error.message);
      }
    },
  });

  const disableAuthMutation = useMutationWithAuth({
    mutationFn: () => mutateDisableProxyAccessAuth(session.csrfToken ?? '', id),
    onSuccess: async () => {
      setFormError(undefined);
      setActivation(undefined);
      await queryClient.invalidateQueries({ queryKey: ['proxy', id] });
    },
    onError: (error) => {
      if (isApiError(error)) {
        setFormError(error.message);
      }
    },
  });

  const saveRouteMutation = useMutationWithAuth({
    mutationFn: async (input: RouteDraft) => {
      if (input.id) {
        return mutateUpdateProxyRoute(session.csrfToken ?? '', {
          id: input.id,
          clientId: input.clientId,
          pathPrefix: input.pathPrefix,
          stripPrefix: input.stripPrefix,
          upstreamPathPrefix: input.upstreamPathPrefix || '/',
          targetHost: input.targetHost,
          targetPort: Number(input.targetPort),
          status: input.status,
        });
      }
      return mutateCreateProxyRoute(session.csrfToken ?? '', {
        proxyId: id,
        clientId: input.clientId,
        pathPrefix: input.pathPrefix,
        stripPrefix: input.stripPrefix,
        upstreamPathPrefix: input.upstreamPathPrefix || '/',
        targetHost: input.targetHost,
        targetPort: Number(input.targetPort),
      });
    },
    onSuccess: async () => {
      setRouteDialogOpen(false);
      setRouteError(undefined);
      await queryClient.invalidateQueries({ queryKey: ['proxy', id] });
    },
    onError: (error) => {
      if (isApiError(error)) {
        setRouteError(error.message);
      }
    },
  });

  const deleteRouteMutation = useMutationWithAuth({
    mutationFn: (routeId: string) => mutateDeleteProxyRoute(session.csrfToken ?? '', routeId),
    onSuccess: async () => {
      setFormError(undefined);
      await queryClient.invalidateQueries({ queryKey: ['proxy', id] });
    },
    onError: (error) => {
      if (isApiError(error)) {
        setFormError(error.message);
      }
    },
  });

  const openEdit = () => {
    setDraftNotice(undefined);
    setFieldErrors(undefined);
    setFormError(undefined);
    setEditing(true);
  };

  const closeEdit = () => {
    setEditing(false);
    setDraftNotice(undefined);
  };

  const handleCreateCertificate = () => {
    const draftId = saveProxyDraft<ProxyEditDraft>({ ...localForm });
    navigate(buildCertificateCreateLink({ returnTo: `/proxies/${id}`, draftId, host: localForm.entryHost || undefined }));
  };

  const clientOptions = useMemo(() => clientsQuery.data?.items ?? [], [clientsQuery.data?.items]);

  if (query.isLoading) {
    return <PageLoading label="Loading proxy..." />;
  }
  if (query.error) {
    if (isNotFoundError(query.error)) {
      return <NotFoundState resource="Proxy" />;
    }
    return <ErrorState title="Proxy failed" message={query.error.message} retry={() => query.refetch()} />;
  }
  if (!query.data) {
    return <PageLoading label="Loading proxy..." />;
  }

  const proxy = query.data;
  const usesRouteHost = isRouteProxy(proxy.type);
  const usesCertificates = isHTTPSProxy(proxy.type);
  const hostOptions = hostOptionsWithCurrent(entryOptionsQuery.data?.hosts ?? [{ value: '', label: 'Default listener host', isDefault: true }], localForm.entryBindHost);
  const routes = proxy.routes ?? [];

  return (
    <section className="page-section">
      <DetailBackLink to="/proxies" label="Back to proxies" />
      <PageHeader
        title={proxy.name}
        description={`Proxy ID: ${proxy.id}`}
        actions={
          <>
            <Button type="default" icon={<EditOutlined aria-hidden="true" />} onClick={openEdit}>
              Edit proxy
            </Button>
            {proxy.status === 'disabled' ? (
              <Button type="default" icon={<ThunderboltOutlined aria-hidden="true" />} onClick={() => enableMutation.mutate(undefined)}>
                Enable
              </Button>
            ) : (
              <ConfirmButton label="Disable proxy" confirmLabel="Disable this proxy?" onConfirm={() => disableMutation.mutate(undefined)} />
            )}
            <ConfirmButton label="Delete proxy" confirmLabel="Delete disabled proxy?" onConfirm={() => deleteMutation.mutate(undefined)} disabled={deleteMutation.isPending} />
          </>
        }
      />

      {formError ? <div className="banner banner--danger">{formError}</div> : null}

      <div className="detail-grid detail-grid--wide">
        <article className="panel">
          <h2>Configuration</h2>
          <dl className="detail-list">
            <div><dt>Type</dt><dd>{proxy.type}</dd></div>
            <div><dt>Status</dt><dd><StatusBadge value={proxy.status} /></dd></div>
            <div><dt>Runtime</dt><dd><StatusBadge value={proxy.runtimeStatus} /></dd></div>
            <div><dt>Entry</dt><dd>{proxyEntryLabel(proxy)}</dd></div>
            {usesRouteHost ? <div><dt>Domain</dt><dd>{proxy.config.entryHost || 'Not set'}</dd></div> : null}
            <div><dt>Target</dt><dd>{proxy.config.targetHost ?? '-'}:{proxy.config.targetPort ?? '-'}</dd></div>
            {usesCertificates ? <div><dt>绑定证书</dt><dd>{proxy.config.certificateId || '未绑定'}</dd></div> : null}
            <div><dt>Description</dt><dd>{proxy.description || 'No description'}</dd></div>
          </dl>
        </article>
        <article className="panel">
          <h2>Runtime stats</h2>
          <dl className="detail-list">
            <div><dt>Active TCP</dt><dd>{proxy.activeTCPConnections}</dd></div>
            <div><dt>Upload</dt><dd>{formatBytes(proxy.uploadBytes)}</dd></div>
            <div><dt>Download</dt><dd>{formatBytes(proxy.downloadBytes)}</dd></div>
            <div><dt>TCP errors</dt><dd>{proxy.tcpErrorCount}</dd></div>
            <div><dt>UDP errors</dt><dd>{proxy.udpErrorCount}</dd></div>
            <div><dt>HTTP errors</dt><dd>{proxy.httpErrorCount}</dd></div>
            <div><dt>Updated</dt><dd><Timestamp value={proxy.updatedAt} /></dd></div>
          </dl>
        </article>
        {proxy.certificate ? (
          <article className="panel">
            <h2>Certificate</h2>
            <dl className="detail-list">
              <div><dt>Serving</dt><dd><StatusBadge value={proxy.certificate.servingStatus ?? proxy.certificate.status} /></dd></div>
              <div><dt>Operation</dt><dd><StatusBadge value={proxy.certificate.operationStatus} /></dd></div>
              <div><dt>Host</dt><dd>{proxy.certificate.host ?? 'N/A'}</dd></div>
              <div><dt>Expires</dt><dd><Timestamp value={proxy.certificate.notAfter} /></dd></div>
              <div><dt>Issued</dt><dd><Timestamp value={proxy.certificate.lastIssuedAt} /></dd></div>
              <div><dt>Renewed</dt><dd><Timestamp value={proxy.certificate.lastRenewedAt} /></dd></div>
              <div><dt>Checked</dt><dd><Timestamp value={proxy.certificate.lastCheckedAt} /></dd></div>
              <div><dt>Attempted</dt><dd><Timestamp value={proxy.certificate.lastAttemptedAt} /></dd></div>
              <div><dt>Next attempt</dt><dd><Timestamp value={proxy.certificate.nextAttemptAt} /></dd></div>
              <div><dt>Failures</dt><dd>{proxy.certificate.failureCount ?? 0}</dd></div>
              <div><dt>Fingerprint</dt><dd>{formatFingerprint(proxy.certificate.fingerprint)}</dd></div>
              <div><dt>Last error</dt><dd>{proxy.certificate.lastError || 'None'}</dd></div>
            </dl>
          </article>
        ) : null}
        {usesRouteHost ? (
          <article className="panel">
            <div className="panel__header">
              <h2>Path routes</h2>
              <Button
                type="default"
                icon={<PlusOutlined aria-hidden="true" />}
                onClick={() => {
                  setRouteDraft(emptyRouteDraft(proxy.clientId));
                  setRouteError(undefined);
                  setRouteDialogOpen(true);
                }}
              >
                Add route
              </Button>
            </div>
            {routes.length === 0 ? (
              <p className="muted">No path routes configured. Default backend is used for all paths.</p>
            ) : (
              <table className="data-table">
                <thead>
                  <tr>
                    <th>Path</th>
                    <th>Client</th>
                    <th>Target</th>
                    <th>Rewrite</th>
                    <th>Status</th>
                    <th />
                  </tr>
                </thead>
                <tbody>
                  {routes.map((route) => (
                    <tr key={route.id}>
                      <td>{route.pathPrefix}</td>
                      <td>{route.clientId}</td>
                      <td>
                        {route.targetHost}:{route.targetPort}
                      </td>
                      <td>
                        {route.stripPrefix ? `strip → ${route.upstreamPathPrefix || '/'}` : 'keep path'}
                      </td>
                      <td>
                        <StatusBadge value={route.status} />
                      </td>
                      <td className="table-actions">
                        <Button
                          type="link"
                          onClick={() => {
                            setRouteDraft(routeDraftFromRoute(route));
                            setRouteError(undefined);
                            setRouteDialogOpen(true);
                          }}
                        >
                          Edit
                        </Button>
                        <ConfirmButton
                          label="Delete"
                          confirmLabel="Delete this path route?"
                          onConfirm={() => deleteRouteMutation.mutate(route.id)}
                          disabled={deleteRouteMutation.isPending}
                        />
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </article>
        ) : null}
        {usesCertificates ? (
          <article className="panel">
            <h2>Access activation</h2>
            <dl className="detail-list">
              <div>
                <dt>Status</dt>
                <dd>{proxy.accessAuthEnabled ? 'Enabled' : 'Disabled'}</dd>
              </div>
            </dl>
            <div className="stack">
              {!proxy.accessAuthEnabled ? (
                <Button type="primary" onClick={() => enableAuthMutation.mutate(undefined)} disabled={enableAuthMutation.isPending}>
                  {enableAuthMutation.isPending ? 'Enabling...' : 'Enable auth and create link'}
                </Button>
              ) : (
                <>
                  <Button type="default" onClick={() => createActivationMutation.mutate(undefined)} disabled={createActivationMutation.isPending}>
                    {createActivationMutation.isPending ? 'Creating...' : 'Create activation link'}
                  </Button>
                  <ConfirmButton
                    label="Revoke all access"
                    confirmLabel="Revoke all activation tokens and cookies?"
                    onConfirm={() => revokeAccessMutation.mutate(undefined)}
                    disabled={revokeAccessMutation.isPending}
                  />
                  <ConfirmButton
                    label="Disable access auth"
                    confirmLabel="Disable access authentication and revoke all credentials?"
                    onConfirm={() => disableAuthMutation.mutate(undefined)}
                    disabled={disableAuthMutation.isPending}
                  />
                </>
              )}
            </div>
          </article>
        ) : null}
      </div>

      <Dialog
        open={editing}
        title="Edit proxy"
        onClose={closeEdit}
        footer={
          <>
            <Button type="default" onClick={closeEdit}>
              Cancel
            </Button>
            <Button type="primary" onClick={() => updateMutation.mutate(localForm)} disabled={updateMutation.isPending}>
              {updateMutation.isPending ? 'Saving...' : 'Save changes'}
            </Button>
          </>
        }
      >
        {formError ? <div className="banner banner--danger">{formError}</div> : null}
        {draftNotice ? <div className="banner banner--warning">{draftNotice}</div> : null}
        {entryOptionsQuery.error ? <div className="banner banner--danger">Entry options failed to load: {entryOptionsQuery.error.message}</div> : null}
        <ValidationBanner fields={fieldErrors} />
        <div className="stack">
          <TextField label="Name" value={localForm.name} error={fieldErrors?.name} onChange={(event) => setLocalForm((current) => ({ ...current, name: event.target.value }))} />
          <SelectField label="Bind host" value={localForm.entryBindHost} error={fieldErrors?.entryBindHost} onChange={(event) => setLocalForm((current) => ({ ...current, entryBindHost: event.target.value }))}>
            {hostOptions.map((option) => (
              <option key={option.value || '__default'} value={option.value}>
                {option.label}
              </option>
            ))}
          </SelectField>
          <TextField label="Entry port" value={localForm.entryPort} error={fieldErrors?.entryPort} onChange={(event) => setLocalForm((current) => ({ ...current, entryPort: event.target.value }))} />
          {usesRouteHost ? (
            <TextField label={proxy.type === 'https' ? 'SNI domain' : 'HTTP domain'} value={localForm.entryHost} error={fieldErrors?.entryHost} onChange={(event) => setLocalForm((current) => ({ ...current, entryHost: event.target.value }))} />
          ) : null}
          <TextField label="Target host" value={localForm.targetHost} error={fieldErrors?.targetHost} onChange={(event) => setLocalForm((current) => ({ ...current, targetHost: event.target.value }))} />
          <TextField label="Target port" value={localForm.targetPort} error={fieldErrors?.targetPort} onChange={(event) => setLocalForm((current) => ({ ...current, targetPort: event.target.value }))} />
          {usesCertificates ? (
            <CertificateSelectField
              entryHost={localForm.entryHost}
              proxyId={proxy.id}
              value={localForm.certificateId}
              onChange={(certificateId) => setLocalForm((current) => ({ ...current, certificateId }))}
              onCreateCertificate={handleCreateCertificate}
              error={fieldErrors?.certificateId}
            />
          ) : null}
          <TextAreaField label="Description" value={localForm.description} onChange={(event) => setLocalForm((current) => ({ ...current, description: event.target.value }))} />
        </div>
      </Dialog>

      <Dialog
        open={routeDialogOpen}
        title={routeDraft.id ? 'Edit path route' : 'Add path route'}
        onClose={() => {
          setRouteDialogOpen(false);
          setRouteError(undefined);
        }}
        footer={
          <>
            <Button
              type="default"
              onClick={() => {
                setRouteDialogOpen(false);
                setRouteError(undefined);
              }}
            >
              Cancel
            </Button>
            <Button type="primary" onClick={() => saveRouteMutation.mutate(routeDraft)} disabled={saveRouteMutation.isPending}>
              {saveRouteMutation.isPending ? 'Saving...' : 'Save route'}
            </Button>
          </>
        }
      >
        {routeError ? <div className="banner banner--danger">{routeError}</div> : null}
        <div className="stack">
          <TextField label="Path prefix" value={routeDraft.pathPrefix} onChange={(event) => setRouteDraft((current) => ({ ...current, pathPrefix: event.target.value }))} />
          <SelectField label="Client" value={routeDraft.clientId} onChange={(event) => setRouteDraft((current) => ({ ...current, clientId: event.target.value }))}>
            <option value="">Select client</option>
            {clientOptions.map((client) => (
              <option key={client.id} value={client.id}>
                {client.name} ({client.id})
              </option>
            ))}
          </SelectField>
          <TextField label="Target host" value={routeDraft.targetHost} onChange={(event) => setRouteDraft((current) => ({ ...current, targetHost: event.target.value }))} />
          <TextField label="Target port" value={routeDraft.targetPort} onChange={(event) => setRouteDraft((current) => ({ ...current, targetPort: event.target.value }))} />
          <label className="field">
            <span className="field__label">Strip path prefix</span>
            <Switch checked={routeDraft.stripPrefix} onChange={(checked) => setRouteDraft((current) => ({ ...current, stripPrefix: checked }))} />
          </label>
          <TextField
            label="Upstream path prefix"
            value={routeDraft.upstreamPathPrefix}
            onChange={(event) => setRouteDraft((current) => ({ ...current, upstreamPathPrefix: event.target.value }))}
            hint="Used when strip prefix is enabled. Default is /."
          />
          {routeDraft.id ? (
            <SelectField label="Status" value={routeDraft.status} onChange={(event) => setRouteDraft((current) => ({ ...current, status: event.target.value }))}>
              <option value="enabled">enabled</option>
              <option value="disabled">disabled</option>
            </SelectField>
          ) : null}
        </div>
      </Dialog>

      <Dialog
        open={Boolean(activation)}
        title="Activation link"
        onClose={() => setActivation(undefined)}
        footer={
          <Button type="primary" onClick={() => setActivation(undefined)}>
            Close
          </Button>
        }
      >
        {activation ? (
          <div className="stack">
            <div className="banner banner--warning">This activation URL is shown only once. Copy or scan it before closing.</div>
            <TextField label="Activation URL" value={activation.url} readOnly />
            <div className="stack">
              <Button
                type="default"
                onClick={async () => {
                  try {
                    await navigator.clipboard.writeText(activation.url);
                  } catch {
                    setFormError('Copy to clipboard failed');
                  }
                }}
              >
                Copy URL
              </Button>
              <div className="qr-wrap">
                <QRCode value={activation.url} size={180} />
              </div>
              <p className="muted">Expires: {activation.expiresAt ? new Date(activation.expiresAt).toLocaleString() : 'Unknown'}</p>
            </div>
          </div>
        ) : null}
      </Dialog>
    </section>
  );
}

function formatFingerprint(value?: string | null) {
  if (!value) {
    return 'None';
  }
  return value.length > 32 ? `${value.slice(0, 32)}...` : value;
}
