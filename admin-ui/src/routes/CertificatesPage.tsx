import { useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { ConfirmButton } from '../components/ConfirmButton';
import { EmptyState, ErrorState, FilteredEmptyState, PageLoading } from '../components/PageStates';
import { useAuthedQuery } from '../hooks/useAuthedQuery';
import { useMutationWithAuth } from '../hooks/useMutationWithAuth';
import {
  mutateIssueCertificate,
  mutateRenewCertificate,
  queryCertificates,
  type CertificateFilter,
} from '../lib/admin-graphql';
import { formatTitle } from '../lib/format';
import { useSession } from '../session';
import { PageHeader, Pagination, StatusBadge, Timestamp } from './shared';

const defaultFilter: CertificateFilter = { query: '', status: '' };

export function CertificatesPage() {
  const session = useSession();
  const queryClient = useQueryClient();
  const [page, setPage] = useState(1);
  const [filter, setFilter] = useState<CertificateFilter>(defaultFilter);

  const query = useAuthedQuery({
    queryKey: ['certificates', page, filter],
    queryFn: () => queryCertificates({ page: { page, pageSize: 10 }, sort: { field: 'host', direction: 'asc' }, filter }),
    refetchInterval: session.pollIntervalSeconds * 3000,
  });

  const issueMutation = useMutationWithAuth({
    mutationFn: (proxyId: string) => mutateIssueCertificate(session.csrfToken ?? '', proxyId),
    onSuccess: async (_, proxyId) => {
      await queryClient.invalidateQueries({ queryKey: ['certificates'] });
      await queryClient.invalidateQueries({ queryKey: ['proxy', proxyId] });
    },
  });

  const renewMutation = useMutationWithAuth({
    mutationFn: (proxyId: string) => mutateRenewCertificate(session.csrfToken ?? '', proxyId),
    onSuccess: async (_, proxyId) => {
      await queryClient.invalidateQueries({ queryKey: ['certificates'] });
      await queryClient.invalidateQueries({ queryKey: ['proxy', proxyId] });
    },
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

  return (
    <section className="page-section">
      <PageHeader
        title="Certificates"
        description="Managed certificate status for HTTPS proxies."
        actions={
          <button type="button" className="button button--secondary" onClick={() => query.refetch()}>
            Refresh
          </button>
        }
      />

      <div className="toolbar-grid">
        <label className="field">
          <span className="field__label">Search</span>
          <input className="input" value={filter.query ?? ''} onChange={(event) => setFilter((current) => ({ ...current, query: event.target.value }))} />
        </label>
        <label className="field">
          <span className="field__label">Status</span>
          <select className="input" value={filter.status ?? ''} onChange={(event) => setFilter((current) => ({ ...current, status: event.target.value }))}>
            <option value="">All</option>
            <option value="pending">Pending</option>
            <option value="valid">Valid</option>
            <option value="failed">Failed</option>
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
                  <th>Status</th>
                  <th>Expires</th>
                  <th>Issued</th>
                  <th>Renewed</th>
                  <th>Last error</th>
                  <th>Actions</th>
                </tr>
              </thead>
              <tbody>
                {data.items.map((certificate) => (
                  <tr key={certificate.proxyId}>
                    <td>{certificate.proxyId}</td>
                    <td>{certificate.host ?? 'N/A'}</td>
                    <td><StatusBadge value={certificate.status ?? 'unknown'} /></td>
                    <td><Timestamp value={certificate.notAfter} /></td>
                    <td><Timestamp value={certificate.lastIssuedAt} /></td>
                    <td><Timestamp value={certificate.lastRenewedAt} /></td>
                    <td>{certificate.lastError || 'None'}</td>
                    <td>
                      <div className="inline-actions">
                        <ConfirmButton label="Issue" confirmLabel={`Issue certificate for ${certificate.host ?? certificate.proxyId}?`} onConfirm={() => issueMutation.mutate(certificate.proxyId)} tone="secondary" />
                        <ConfirmButton label="Renew" confirmLabel={`Renew ${certificate.host ?? certificate.proxyId}?`} onConfirm={() => renewMutation.mutate(certificate.proxyId)} tone="secondary" />
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

      {(issueMutation.error || renewMutation.error) && (
        <div className="banner banner--danger">{formatTitle((issueMutation.error || renewMutation.error)?.message ?? 'Certificate action failed')}</div>
      )}
    </section>
  );
}
