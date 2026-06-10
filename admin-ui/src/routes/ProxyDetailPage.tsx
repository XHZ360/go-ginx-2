import { useEffect, useState } from 'react';
import { EditOutlined, ThunderboltOutlined } from '@ant-design/icons';
import { Button } from 'antd';
import { useNavigate, useParams } from 'react-router-dom';
import { useQueryClient } from '@tanstack/react-query';
import { ConfirmButton } from '../components/ConfirmButton';
import { Dialog } from '../components/Dialog';
import { SelectField, TextAreaField, TextField } from '../components/FormField';
import { ErrorState, NotFoundState, PageLoading, ValidationBanner } from '../components/PageStates';
import { useAuthedQuery } from '../hooks/useAuthedQuery';
import { useMutationWithAuth } from '../hooks/useMutationWithAuth';
import {
  mutateDeleteProxy,
  mutateDisableProxy,
  mutateEnableProxy,
  mutateUpdateProxy,
  queryProxyEntryOptions,
  queryProxy,
} from '../lib/admin-graphql';
import { isApiError, isNotFoundError, type ProxyEntryHostOption, type ProxyRecord } from '../lib/contracts';
import { formatBytes } from '../lib/format';
import { useSession } from '../session';
import { DetailBackLink, PageHeader, StatusBadge, Timestamp } from './shared';

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

export function ProxyDetailPage() {
  const { id = '' } = useParams();
  const navigate = useNavigate();
  const session = useSession();
  const queryClient = useQueryClient();
  const [editing, setEditing] = useState(false);
  const [formError, setFormError] = useState<string>();
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>();
  const [localForm, setLocalForm] = useState({
    name: '',
    description: '',
    entryBindHost: '',
    entryHost: '',
    entryPort: '',
    targetHost: '',
    targetPort: '',
    certFile: '',
    keyFile: '',
  });

  const query = useAuthedQuery({
    queryKey: ['proxy', id],
    queryFn: () => queryProxy(id),
    refetchInterval: session.pollIntervalSeconds * 1000,
  });
  const entryOptionsQuery = useAuthedQuery({
    queryKey: ['proxy-entry-options'],
    queryFn: () => queryProxyEntryOptions(),
  });

  useEffect(() => {
    if (!query.data) {
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
      certFile: query.data.config.certFile ?? '',
      keyFile: query.data.config.keyFile ?? '',
    });
  }, [query.data]);

  const updateMutation = useMutationWithAuth({
    mutationFn: (input: { name: string; description: string; entryBindHost: string; entryHost: string; entryPort: string; targetHost: string; targetPort: string; certFile: string; keyFile: string }) =>
      mutateUpdateProxy(session.csrfToken ?? '', {
        id,
        type: query.data?.type,
        name: input.name,
        description: input.description,
        config: {
          entryBindHost: input.entryBindHost || undefined,
          entryHost: input.entryHost || undefined,
          entryPort: input.entryPort ? Number(input.entryPort) : undefined,
          targetHost: input.targetHost || undefined,
          targetPort: input.targetPort ? Number(input.targetPort) : undefined,
          certFile: input.certFile || undefined,
          keyFile: input.keyFile || undefined,
        },
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

  return (
    <section className="page-section">
      <DetailBackLink to="/proxies" label="Back to proxies" />
      <PageHeader
        title={proxy.name}
        description={`Proxy ID: ${proxy.id}`}
        actions={
          <>
            <Button type="default" icon={<EditOutlined aria-hidden="true" />} onClick={() => setEditing(true)}>
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
            {usesCertificates ? <div><dt>Certificate files</dt><dd>{proxy.config.certFile || 'Not set'} / {proxy.config.keyFile || 'Not set'}</dd></div> : null}
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
      </div>

      <Dialog
        open={editing}
        title="Edit proxy"
        onClose={() => setEditing(false)}
        footer={
          <>
            <Button type="default" onClick={() => setEditing(false)}>
              Cancel
            </Button>
            <Button type="primary" onClick={() => updateMutation.mutate(localForm)} disabled={updateMutation.isPending}>
              {updateMutation.isPending ? 'Saving...' : 'Save changes'}
            </Button>
          </>
        }
      >
        {formError ? <div className="banner banner--danger">{formError}</div> : null}
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
            <>
              <TextField label="Certificate file" value={localForm.certFile} error={fieldErrors?.certFile} onChange={(event) => setLocalForm((current) => ({ ...current, certFile: event.target.value }))} />
              <TextField label="Private key file" value={localForm.keyFile} error={fieldErrors?.keyFile} onChange={(event) => setLocalForm((current) => ({ ...current, keyFile: event.target.value }))} />
            </>
          ) : null}
          <TextAreaField label="Description" value={localForm.description} onChange={(event) => setLocalForm((current) => ({ ...current, description: event.target.value }))} />
        </div>
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
