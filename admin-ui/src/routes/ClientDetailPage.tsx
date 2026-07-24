import { useState } from 'react';
import { DeploymentUnitOutlined } from '@ant-design/icons';
import { Button, Tag } from 'antd';
import { useNavigate, useParams } from 'react-router-dom';
import { useQueryClient } from '@tanstack/react-query';
import { ConfirmButton } from '../components/ConfirmButton';
import { useAuthedQuery } from '../hooks/useAuthedQuery';
import { useMutationWithAuth } from '../hooks/useMutationWithAuth';
import { mutateDeleteClient, mutateRotateClientCredential, queryClient } from '../lib/admin-graphql';
import { formatBytes } from '../lib/format';
import { ErrorState, NotFoundState, PageLoading } from '../components/PageStates';
import { isNotFoundError, type ProxySummary } from '../lib/contracts';
import { useSession } from '../session';
import { DetailBackLink, PageHeader, StatusBadge, Timestamp } from './shared';
import { LocalClientManagement } from './LocalClientManagement';

function isWebProxy(type: string) {
  return type === 'web' || type === 'http' || type === 'https';
}

function formatProxyEntry(proxy: ProxySummary) {
  if (isWebProxy(proxy.type)) {
    const host = proxy.entryHost || 'domain pending';
    return host;
  }
  const bindHost = proxy.entryBindHost || 'default';
  const entryPort = proxy.entryPort ?? 'default';
  return `${bindHost}:${entryPort}`;
}

export function ClientDetailPage() {
  const { id = '' } = useParams();
  const session = useSession();
  const navigate = useNavigate();
  const queryClientInstance = useQueryClient();
  const [rotatedCredential, setRotatedCredential] = useState<string>();
  const [rotationError, setRotationError] = useState<string>();
  const [deleteError, setDeleteError] = useState<string>();
  const query = useAuthedQuery({
    queryKey: ['client', id],
    queryFn: () => queryClient(id),
    refetchInterval: session.pollIntervalSeconds * 1000,
  });
  const rotateMutation = useMutationWithAuth({
    mutationFn: () => mutateRotateClientCredential(session.csrfToken ?? '', id),
    onSuccess: async (data) => {
      setRotationError(undefined);
      setRotatedCredential(data.rotateClientCredential.credential ?? undefined);
      await queryClientInstance.invalidateQueries({ queryKey: ['client', id] });
      await queryClientInstance.invalidateQueries({ queryKey: ['clients'] });
    },
    onError: (error) => {
      setRotatedCredential(undefined);
      setRotationError(error.message);
    },
  });
  const deleteMutation = useMutationWithAuth({
    mutationFn: () => mutateDeleteClient(session.csrfToken ?? '', id),
    onSuccess: async () => {
      setDeleteError(undefined);
      await queryClientInstance.invalidateQueries({ queryKey: ['clients'] });
      navigate('/clients');
    },
    onError: (error) => {
      setDeleteError(error.message);
    },
  });

  if (query.isLoading) {
    return <PageLoading label="Loading client..." />;
  }
  if (query.error) {
    if (isNotFoundError(query.error)) {
      return <NotFoundState resource="Client" />;
    }
    return <ErrorState title="Client failed" message={query.error.message} retry={() => query.refetch()} />;
  }
  if (!query.data) {
    return <PageLoading label="Loading client..." />;
  }

  const client = query.data;

  return (
    <section className="page-section">
      <DetailBackLink to="/clients" label="Back to clients" />
      <PageHeader
        title={client.name}
        description={`Client ID: ${client.id}`}
        actions={
          <>
            <StatusBadge value={client.status} />
            {client.isSystem ? <Tag color="blue">System client</Tag> : (
              <>
                <Button
                  type="default"
                  icon={<DeploymentUnitOutlined aria-hidden="true" />}
                  onClick={() => navigate(`/proxies?create=1&userId=${encodeURIComponent(client.userId)}&clientId=${encodeURIComponent(client.id)}`)}
                >
                  Create proxy
                </Button>
                <ConfirmButton
                  label="Rotate credential"
                  confirmLabel="Rotate this client credential?"
                  onConfirm={() => rotateMutation.mutate(undefined)}
                  disabled={rotateMutation.isPending}
                  tone="secondary"
                />
                <ConfirmButton
                  label="Delete client"
                  confirmLabel="Delete this client?"
                  onConfirm={() => deleteMutation.mutate(undefined)}
                  disabled={deleteMutation.isPending}
                />
              </>
            )}
          </>
        }
      />
      {rotationError ? <div className="banner banner--danger">{rotationError}</div> : null}
      {deleteError ? <div className="banner banner--danger">{deleteError}</div> : null}
      {rotatedCredential ? (
        <div className="banner banner--success" role="status">
          <strong>New client credential</strong>
          <p>This value is shown once. Store it before leaving this page.</p>
          <code className="secret-value">{rotatedCredential}</code>
        </div>
      ) : null}
      <div className="detail-grid">
        <article className="panel">
          <h2>Runtime</h2>
          <dl className="detail-list">
            <div><dt>Online</dt><dd>{client.runtime.online ? 'Yes' : 'No'}</dd></div>
            <div><dt>Protocol</dt><dd>{client.runtime.protocol ?? 'N/A'}</dd></div>
            <div><dt>Connected</dt><dd><Timestamp value={client.runtime.connectedAt} /></dd></div>
            <div><dt>Last heartbeat</dt><dd><Timestamp value={client.runtime.lastHeartbeat} /></dd></div>
            <div><dt>Error summary</dt><dd>{client.runtime.errorSummary || 'None'}</dd></div>
          </dl>
        </article>
        <article className="panel">
          <h2>Stats</h2>
          <dl className="detail-list">
            <div><dt>Active proxies</dt><dd>{client.runtime.activeProxies ?? 0}</dd></div>
            <div><dt>Active streams</dt><dd>{client.runtime.activeStreams ?? 0}</dd></div>
            <div><dt>Upload</dt><dd>{formatBytes(client.runtime.uploadBytes)}</dd></div>
            <div><dt>Download</dt><dd>{formatBytes(client.runtime.downloadBytes)}</dd></div>
            <div><dt>Version</dt><dd>{client.version}</dd></div>
          </dl>
        </article>
      </div>

      {client.isSystem ? <LocalClientManagement client={client} /> : <article className="panel">
        <h2>Managed proxies</h2>
        {client.managedProxies.length === 0 ? (
          <p className="muted">No proxies assigned to this client.</p>
        ) : (
          <div className="table-wrap">
            <table className="table">
              <thead>
                <tr>
                  <th>ID</th>
                  <th>Name</th>
                  <th>Type</th>
                  <th>Status</th>
                  <th>Runtime</th>
                  <th>Entry</th>
                </tr>
              </thead>
              <tbody>
                {client.managedProxies.map((proxy) => (
                  <tr key={proxy.id}>
                    <td>{proxy.id}</td>
                    <td>{proxy.name}</td>
                    <td>{proxy.type}</td>
                    <td><StatusBadge value={proxy.status} /></td>
                    <td><StatusBadge value={proxy.runtimeStatus} /></td>
                    <td>{formatProxyEntry(proxy)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </article>}
    </section>
  );
}
