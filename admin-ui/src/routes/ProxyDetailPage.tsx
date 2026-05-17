import { useEffect, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { useQueryClient } from '@tanstack/react-query';
import { ConfirmButton } from '../components/ConfirmButton';
import { Dialog } from '../components/Dialog';
import { TextAreaField, TextField } from '../components/FormField';
import { ErrorState, NotFoundState, PageLoading, ValidationBanner } from '../components/PageStates';
import { useAuthedQuery } from '../hooks/useAuthedQuery';
import { useMutationWithAuth } from '../hooks/useMutationWithAuth';
import {
  mutateDeleteProxy,
  mutateDisableProxy,
  mutateEnableProxy,
  mutateUpdateProxy,
  queryProxy,
} from '../lib/admin-graphql';
import { isApiError, isNotFoundError } from '../lib/contracts';
import { formatBytes } from '../lib/format';
import { useSession } from '../session';
import { DetailBackLink, PageHeader, StatusBadge, Timestamp } from './shared';

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
    entryHost: '',
    entryPort: '',
    targetHost: '',
    targetPort: '',
  });

  const query = useAuthedQuery({
    queryKey: ['proxy', id],
    queryFn: () => queryProxy(id),
    refetchInterval: session.pollIntervalSeconds * 1000,
  });

  useEffect(() => {
    if (!query.data) {
      return;
    }
    setLocalForm({
      name: query.data.name,
      description: query.data.description ?? '',
      entryHost: query.data.config.entryHost ?? '',
      entryPort: query.data.config.entryPort?.toString() ?? '',
      targetHost: query.data.config.targetHost ?? '',
      targetPort: query.data.config.targetPort?.toString() ?? '',
    });
  }, [query.data]);

  const updateMutation = useMutationWithAuth({
    mutationFn: (input: { name: string; description: string; entryHost: string; entryPort: string; targetHost: string; targetPort: string }) =>
      mutateUpdateProxy(session.csrfToken ?? '', {
        id,
        type: query.data?.type,
        name: input.name,
        description: input.description,
        config: {
          entryHost: input.entryHost || undefined,
          entryPort: input.entryPort ? Number(input.entryPort) : undefined,
          targetHost: input.targetHost || undefined,
          targetPort: input.targetPort ? Number(input.targetPort) : undefined,
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
      await queryClient.invalidateQueries({ queryKey: ['proxy', id] });
      await queryClient.invalidateQueries({ queryKey: ['proxies'] });
    },
  });

  const disableMutation = useMutationWithAuth({
    mutationFn: () => mutateDisableProxy(session.csrfToken ?? '', id),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['proxy', id] });
      await queryClient.invalidateQueries({ queryKey: ['proxies'] });
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

  return (
    <section className="page-section">
      <DetailBackLink to="/proxies" label="Back to proxies" />
      <PageHeader
        title={proxy.name}
        description={`Proxy ID: ${proxy.id}`}
        actions={
          <>
            <button type="button" className="button button--secondary" onClick={() => setEditing(true)}>
              Edit proxy
            </button>
            {proxy.status === 'disabled' ? (
                <button type="button" className="button button--secondary" onClick={() => enableMutation.mutate(undefined)}>
                Enable
              </button>
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
            <div><dt>Entry</dt><dd>{proxy.config.entryHost ?? '0.0.0.0'}:{proxy.config.entryPort ?? '-'}</dd></div>
            <div><dt>Target</dt><dd>{proxy.config.targetHost ?? '-'}:{proxy.config.targetPort ?? '-'}</dd></div>
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
              <div><dt>Status</dt><dd><StatusBadge value={proxy.certificate.status} /></dd></div>
              <div><dt>Host</dt><dd>{proxy.certificate.host ?? 'N/A'}</dd></div>
              <div><dt>Expires</dt><dd><Timestamp value={proxy.certificate.notAfter} /></dd></div>
              <div><dt>Issued</dt><dd><Timestamp value={proxy.certificate.lastIssuedAt} /></dd></div>
              <div><dt>Renewed</dt><dd><Timestamp value={proxy.certificate.lastRenewedAt} /></dd></div>
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
            <button type="button" className="button button--secondary" onClick={() => setEditing(false)}>
              Cancel
            </button>
            <button type="button" className="button" onClick={() => updateMutation.mutate(localForm)} disabled={updateMutation.isPending}>
              {updateMutation.isPending ? 'Saving...' : 'Save changes'}
            </button>
          </>
        }
      >
        <ValidationBanner fields={fieldErrors} />
        <div className="stack">
          <TextField label="Name" value={localForm.name} error={fieldErrors?.name} onChange={(event) => setLocalForm((current) => ({ ...current, name: event.target.value }))} />
          <TextField label="Entry host" value={localForm.entryHost} error={fieldErrors?.entryHost} onChange={(event) => setLocalForm((current) => ({ ...current, entryHost: event.target.value }))} />
          <TextField label="Entry port" value={localForm.entryPort} error={fieldErrors?.entryPort} onChange={(event) => setLocalForm((current) => ({ ...current, entryPort: event.target.value }))} />
          <TextField label="Target host" value={localForm.targetHost} error={fieldErrors?.targetHost} onChange={(event) => setLocalForm((current) => ({ ...current, targetHost: event.target.value }))} />
          <TextField label="Target port" value={localForm.targetPort} error={fieldErrors?.targetPort} onChange={(event) => setLocalForm((current) => ({ ...current, targetPort: event.target.value }))} />
          <TextAreaField label="Description" value={localForm.description} onChange={(event) => setLocalForm((current) => ({ ...current, description: event.target.value }))} />
        </div>
      </Dialog>
    </section>
  );
}
