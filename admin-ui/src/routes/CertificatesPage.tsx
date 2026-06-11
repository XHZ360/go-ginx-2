import { type FormEvent, useState } from 'react';
import { CloudUploadOutlined, ReloadOutlined, SyncOutlined } from '@ant-design/icons';
import { Button } from 'antd';
import { useQueryClient } from '@tanstack/react-query';
import { ConfirmButton } from '../components/ConfirmButton';
import { EmptyState, ErrorState, FilteredEmptyState, PageLoading } from '../components/PageStates';
import { useAuthedQuery } from '../hooks/useAuthedQuery';
import { useMutationWithAuth } from '../hooks/useMutationWithAuth';
import {
  mutateCreateProviderCredential,
  mutateDeleteProviderCredential,
  mutateDisableProviderCredential,
  mutateIssueCertificate,
  mutateRenewCertificate,
  mutateRevokeOriginCertificate,
  mutateRotateOriginCertificate,
  mutateSyncOriginCertificate,
  mutateUpdateProviderCredential,
  mutateVerifyProviderCredential,
  queryCertificates,
  queryProviderCredentials,
  type CertificateFilter,
  type ProviderCredentialInput,
} from '../lib/admin-graphql';
import type { ManagedCertificate, ProviderCredential } from '../lib/contracts';
import { formatTitle } from '../lib/format';
import { useSession } from '../session';
import { PageHeader, Pagination, StatusBadge, Timestamp } from './shared';

const defaultFilter: CertificateFilter = { query: '', status: '' };
const defaultCredentialForm: ProviderCredentialInput = { id: '', name: '', scope: '', token: '' };

export function CertificatesPage() {
  const session = useSession();
  const queryClient = useQueryClient();
  const [page, setPage] = useState(1);
  const [filter, setFilter] = useState<CertificateFilter>(defaultFilter);
  const [credentialForm, setCredentialForm] = useState<ProviderCredentialInput>(defaultCredentialForm);
  const [editingCredentialId, setEditingCredentialId] = useState('');
  const [selectedCredentialId, setSelectedCredentialId] = useState('');

  const query = useAuthedQuery({
    queryKey: ['certificates', page, filter],
    queryFn: () => queryCertificates({ page: { page, pageSize: 10 }, sort: { field: 'host', direction: 'asc' }, filter }),
    refetchInterval: session.pollIntervalSeconds * 3000,
  });

  const credentialsQuery = useAuthedQuery({
    queryKey: ['providerCredentials'],
    queryFn: () => queryProviderCredentials({ page: { page: 1, pageSize: 100 } }),
    refetchInterval: session.pollIntervalSeconds * 3000,
  });

  const credentials = credentialsQuery.data?.items ?? [];
  const hasEnabledCredential = credentials.some((item) => item.status !== 'disabled');

  const invalidateCertificateViews = async (proxyId?: string) => {
    await queryClient.invalidateQueries({ queryKey: ['certificates'] });
    await queryClient.invalidateQueries({ queryKey: ['providerCredentials'] });
    if (proxyId) {
      await queryClient.invalidateQueries({ queryKey: ['proxy', proxyId] });
    }
  };

  const issueMutation = useMutationWithAuth({
    mutationFn: (proxyId: string) => mutateIssueCertificate(session.csrfToken ?? '', proxyId),
    onSuccess: async (_, proxyId) => invalidateCertificateViews(proxyId),
  });

  const issueOriginMutation = useMutationWithAuth({
    mutationFn: (input: { proxyId: string; credentialId?: string }) =>
      mutateIssueCertificate(session.csrfToken ?? '', {
        proxyId: input.proxyId,
        providerType: 'cloudflare_origin_ca',
        credentialId: input.credentialId,
        requestType: 'origin-ecc',
        requestedValidity: 5475,
      }),
    onSuccess: async (_, input) => invalidateCertificateViews(input.proxyId),
  });

  const renewMutation = useMutationWithAuth({
    mutationFn: (proxyId: string) => mutateRenewCertificate(session.csrfToken ?? '', proxyId),
    onSuccess: async (_, proxyId) => invalidateCertificateViews(proxyId),
  });

  const rotateOriginMutation = useMutationWithAuth({
    mutationFn: (proxyId: string) => mutateRotateOriginCertificate(session.csrfToken ?? '', proxyId),
    onSuccess: async (_, proxyId) => invalidateCertificateViews(proxyId),
  });

  const syncOriginMutation = useMutationWithAuth({
    mutationFn: (proxyId: string) => mutateSyncOriginCertificate(session.csrfToken ?? '', proxyId),
    onSuccess: async (_, proxyId) => invalidateCertificateViews(proxyId),
  });

  const revokeOriginMutation = useMutationWithAuth({
    mutationFn: (certificate: ManagedCertificate) =>
      mutateRevokeOriginCertificate(session.csrfToken ?? '', {
        proxyId: certificate.proxyId,
        host: certificate.host ?? '',
        cloudflareCertificateId: certificate.cloudflareCertificateId ?? '',
      }),
    onSuccess: async (_, certificate) => invalidateCertificateViews(certificate.proxyId),
  });

  const createCredentialMutation = useMutationWithAuth({
    mutationFn: (input: ProviderCredentialInput) => mutateCreateProviderCredential(session.csrfToken ?? '', input),
    onSuccess: async () => {
      setCredentialForm(defaultCredentialForm);
      setEditingCredentialId('');
      await queryClient.invalidateQueries({ queryKey: ['providerCredentials'] });
    },
  });

  const updateCredentialMutation = useMutationWithAuth({
    mutationFn: (input: ProviderCredentialInput & { id: string }) => mutateUpdateProviderCredential(session.csrfToken ?? '', input),
    onSuccess: async () => {
      setCredentialForm(defaultCredentialForm);
      setEditingCredentialId('');
      await queryClient.invalidateQueries({ queryKey: ['providerCredentials'] });
    },
  });

  const verifyCredentialMutation = useMutationWithAuth({
    mutationFn: (id: string) => mutateVerifyProviderCredential(session.csrfToken ?? '', id),
    onSuccess: async () => queryClient.invalidateQueries({ queryKey: ['providerCredentials'] }),
  });

  const disableCredentialMutation = useMutationWithAuth({
    mutationFn: (id: string) => mutateDisableProviderCredential(session.csrfToken ?? '', id),
    onSuccess: async () => queryClient.invalidateQueries({ queryKey: ['providerCredentials'] }),
  });

  const deleteCredentialMutation = useMutationWithAuth({
    mutationFn: (id: string) => mutateDeleteProviderCredential(session.csrfToken ?? '', id),
    onSuccess: async () => queryClient.invalidateQueries({ queryKey: ['providerCredentials'] }),
  });

  if (query.isLoading) {
    return <PageLoading label="Loading certificates..." />;
  }
  if (query.error) {
    return <ErrorState title="Certificates failed" message={query.error.message} retry={() => query.refetch()} />;
  }
  if (!query.data) {
    return <PageLoading label="Loading certificates..." />;
  }

  const data = query.data;
  const hasFilter = Boolean(filter.query || filter.status);
  const actionError = [
    issueMutation.error,
    issueOriginMutation.error,
    renewMutation.error,
    rotateOriginMutation.error,
    syncOriginMutation.error,
    revokeOriginMutation.error,
    createCredentialMutation.error,
    updateCredentialMutation.error,
    verifyCredentialMutation.error,
    disableCredentialMutation.error,
    deleteCredentialMutation.error,
  ].find(Boolean);

  const submitCredential = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const input = { ...credentialForm, name: credentialForm.name ?? '', scope: credentialForm.scope ?? '', token: credentialForm.token ?? '' };
    if (editingCredentialId) {
      updateCredentialMutation.mutate({ ...input, id: editingCredentialId });
      return;
    }
    createCredentialMutation.mutate(input);
  };

  return (
    <section className="page-section">
      <PageHeader
        title="Certificates"
        description="Managed certificate status for HTTPS proxies."
        actions={
          <Button type="default" icon={<ReloadOutlined aria-hidden="true" />} onClick={() => query.refetch()}>
            Refresh
          </Button>
        }
      />

      <section className="page-section__band">
        <div className="section-heading">
          <h2>Cloudflare Credentials</h2>
        </div>
        <form className="toolbar-grid" onSubmit={submitCredential}>
          <label className="field">
            <span className="field__label">Name</span>
            <input className="input" value={credentialForm.name ?? ''} onChange={(event) => setCredentialForm((current) => ({ ...current, name: event.target.value }))} />
          </label>
          <label className="field">
            <span className="field__label">Scope</span>
            <input className="input" value={credentialForm.scope ?? ''} onChange={(event) => setCredentialForm((current) => ({ ...current, scope: event.target.value }))} />
          </label>
          <label className="field">
            <span className="field__label">API token</span>
            <input className="input" type="password" value={credentialForm.token ?? ''} onChange={(event) => setCredentialForm((current) => ({ ...current, token: event.target.value }))} />
          </label>
          <div className="field field--actions">
            <Button htmlType="submit" type="primary" icon={<CloudUploadOutlined aria-hidden="true" />} loading={createCredentialMutation.isPending || updateCredentialMutation.isPending}>
              {editingCredentialId ? 'Update' : 'Create'}
            </Button>
            {editingCredentialId ? (
              <Button type="default" onClick={() => { setCredentialForm(defaultCredentialForm); setEditingCredentialId(''); }}>
                Cancel
              </Button>
            ) : null}
          </div>
        </form>

        {credentialsQuery.error ? <div className="banner banner--danger">{formatTitle(credentialsQuery.error.message)}</div> : null}
        {credentials.length > 0 ? (
          <div className="table-wrap">
            <table className="table">
              <thead>
                <tr>
                  <th>Name</th>
                  <th>Status</th>
                  <th>Scope</th>
                  <th>Fingerprint</th>
                  <th>Verified</th>
                  <th>Last error</th>
                  <th>Actions</th>
                </tr>
              </thead>
              <tbody>
                {credentials.map((credential) => (
                  <CredentialRow
                    key={credential.id}
                    credential={credential}
                    onEdit={() => {
                      setEditingCredentialId(credential.id);
                      setCredentialForm({ id: credential.id, name: credential.name, scope: credential.scope ?? '', token: '' });
                    }}
                    onVerify={() => verifyCredentialMutation.mutate(credential.id)}
                    onDisable={() => disableCredentialMutation.mutate(credential.id)}
                    onDelete={() => deleteCredentialMutation.mutate(credential.id)}
                  />
                ))}
              </tbody>
            </table>
          </div>
        ) : null}
      </section>

      <div className="toolbar-grid">
        <label className="field">
          <span className="field__label">Search</span>
          <input className="input" value={filter.query ?? ''} onChange={(event) => setFilter((current) => ({ ...current, query: event.target.value }))} />
        </label>
        <label className="field">
          <span className="field__label">Status</span>
          <select className="input" value={filter.status ?? ''} onChange={(event) => setFilter((current) => ({ ...current, status: event.target.value }))}>
            <option value="">All</option>
            <option value="usable">Usable</option>
            <option value="expiring_soon">Expiring soon</option>
            <option value="expired">Expired</option>
            <option value="missing">Missing</option>
            <option value="invalid">Invalid</option>
            <option value="active">Provider active</option>
            <option value="revoked">Provider revoked</option>
            <option value="missing_remote">Remote missing</option>
            <option value="unknown">Unknown</option>
            <option value="issue_failed">Issue failed</option>
            <option value="renewal_failed">Renewal failed</option>
          </select>
        </label>
        <label className="field">
          <span className="field__label">Origin credential</span>
          <select className="input" value={selectedCredentialId} onChange={(event) => setSelectedCredentialId(event.target.value)}>
            <option value="">Default</option>
            {credentials.map((credential) => (
              <option key={credential.id} value={credential.id} disabled={credential.status === 'disabled'}>{credential.name}</option>
            ))}
          </select>
        </label>
      </div>

      {data.items.length === 0 ? (
        hasFilter ? <FilteredEmptyState onClear={() => setFilter(defaultFilter)} /> : <EmptyState title="No certificates" message="HTTPS proxies will appear here for lifecycle actions." />
      ) : (
        <>
          <div className="table-wrap">
            <table className="table">
              <thead>
                <tr>
                  <th>Proxy</th>
                  <th>Host</th>
                  <th>Provider</th>
                  <th>Serving</th>
                  <th>Operation</th>
                  <th>Provider status</th>
                  <th>Expires</th>
                  <th>Last sync</th>
                  <th>Failures</th>
                  <th>Fingerprint</th>
                  <th>Last error</th>
                  <th>Actions</th>
                </tr>
              </thead>
              <tbody>
                {data.items.map((certificate) => (
                  <CertificateRow
                    key={certificate.proxyId}
                    certificate={certificate}
                    canIssueOrigin={hasEnabledCredential}
                    issueACME={() => issueMutation.mutate(certificate.proxyId)}
                    issueOrigin={() => issueOriginMutation.mutate({ proxyId: certificate.proxyId, credentialId: selectedCredentialId || undefined })}
                    renew={() => renewMutation.mutate(certificate.proxyId)}
                    rotate={() => rotateOriginMutation.mutate(certificate.proxyId)}
                    sync={() => syncOriginMutation.mutate(certificate.proxyId)}
                    revoke={() => revokeOriginMutation.mutate(certificate)}
                  />
                ))}
              </tbody>
            </table>
          </div>
          <Pagination page={data.pageInfo.page} totalPages={data.pageInfo.totalPages} onPageChange={setPage} />
        </>
      )}

      {actionError ? <div className="banner banner--danger">{formatTitle(actionError.message)}</div> : null}
    </section>
  );
}

function CredentialRow({
  credential,
  onEdit,
  onVerify,
  onDisable,
  onDelete,
}: {
  credential: ProviderCredential;
  onEdit: () => void;
  onVerify: () => void;
  onDisable: () => void;
  onDelete: () => void;
}) {
  return (
    <tr>
      <td>{credential.name}</td>
      <td><StatusBadge value={credential.status} /></td>
      <td>{credential.scope || 'Default'}</td>
      <td>{formatFingerprint(credential.tokenFingerprint)}</td>
      <td><Timestamp value={credential.lastVerifiedAt} /></td>
      <td>{credential.lastError || 'None'}</td>
      <td>
        <div className="inline-actions">
          <Button type="default" onClick={onEdit}>Edit</Button>
          <Button type="default" icon={<SyncOutlined aria-hidden="true" />} onClick={onVerify} disabled={credential.status === 'disabled'}>Verify</Button>
          <ConfirmButton label="Disable" confirmLabel={`Disable ${credential.name}?`} onConfirm={onDisable} tone="secondary" disabled={credential.status === 'disabled'} />
          <ConfirmButton label="Delete" confirmLabel={`Delete ${credential.name}?`} onConfirm={onDelete} />
        </div>
      </td>
    </tr>
  );
}

function CertificateRow({
  certificate,
  canIssueOrigin,
  issueACME,
  issueOrigin,
  renew,
  rotate,
  sync,
  revoke,
}: {
  certificate: ManagedCertificate;
  canIssueOrigin: boolean;
  issueACME: () => void;
  issueOrigin: () => void;
  renew: () => void;
  rotate: () => void;
  sync: () => void;
  revoke: () => void;
}) {
  const isOrigin = certificate.providerType === 'cloudflare_origin_ca';
  const hasRemoteID = Boolean(certificate.cloudflareCertificateId);
  return (
    <tr>
      <td>{certificate.proxyId}</td>
      <td>{certificate.host ?? 'N/A'}</td>
      <td>
        <div className="cell-stack">
          <span>{formatTitle(certificate.providerType || 'acme_dns01')}</span>
          {certificate.credentialId ? <span className="muted-text">{certificate.credentialId}</span> : null}
          {certificate.cloudflareCertificateId ? <span className="muted-text">{formatFingerprint(certificate.cloudflareCertificateId)}</span> : null}
        </div>
      </td>
      <td><StatusBadge value={certificate.servingStatus ?? certificate.status ?? 'unknown'} /></td>
      <td><StatusBadge value={certificate.operationStatus ?? 'unknown'} /></td>
      <td><StatusBadge value={certificate.providerStatus ?? 'unknown'} /></td>
      <td><Timestamp value={certificate.notAfter} /></td>
      <td><Timestamp value={certificate.lastSyncedAt} /></td>
      <td>{certificate.failureCount ?? 0}</td>
      <td>{formatFingerprint(certificate.fingerprint)}</td>
      <td>{certificate.lastError || 'None'}</td>
      <td>
        <div className="inline-actions">
          {isOrigin ? (
            <>
              <ConfirmButton label="Rotate" confirmLabel={`Rotate ${certificate.host ?? certificate.proxyId}?`} onConfirm={rotate} tone="secondary" />
              <Button type="default" icon={<SyncOutlined aria-hidden="true" />} onClick={sync}>Sync</Button>
              <ConfirmButton label="Revoke" confirmLabel={`Revoke active Origin CA certificate ${certificate.cloudflareCertificateId ?? certificate.proxyId}? Cloudflare to origin TLS will stop until a replacement is issued.`} onConfirm={revoke} disabled={!hasRemoteID} />
            </>
          ) : (
            <>
              <ConfirmButton label="Issue" confirmLabel={`Issue ACME certificate for ${certificate.host ?? certificate.proxyId}?`} onConfirm={issueACME} tone="secondary" />
              <ConfirmButton label="Renew" confirmLabel={`Renew ${certificate.host ?? certificate.proxyId}?`} onConfirm={renew} tone="secondary" />
              <ConfirmButton label="Issue Origin" confirmLabel={`Issue Origin CA certificate for ${certificate.host ?? certificate.proxyId}?`} onConfirm={issueOrigin} tone="secondary" disabled={!canIssueOrigin} />
            </>
          )}
        </div>
      </td>
    </tr>
  );
}

function formatFingerprint(value?: string | null) {
  if (!value) {
    return 'None';
  }
  return value.length > 16 ? `${value.slice(0, 16)}...` : value;
}
