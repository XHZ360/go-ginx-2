import { type ReactElement, useMemo } from 'react';
import { SyncOutlined } from '@ant-design/icons';
import { Button, Tooltip, type TableColumnsType } from 'antd';
import { ConfirmButton } from '../../components/ConfirmButton';
import type { ManagedCertificate, PageInfo } from '../../lib/contracts';
import { formatTitle } from '../../lib/format';
import { DataTable, StatusBadge, Timestamp, pageTablePagination } from '../shared';
import { FILE_PROVIDER, ORIGIN_PROVIDER } from './constants';
import { certificateProviderType, formatFingerprint } from './helpers';

function ActionGate({ reason, children }: { reason?: string; children: ReactElement }) {
  if (!reason) {
    return children;
  }
  return <Tooltip title={reason}>{children}</Tooltip>;
}

function CertificateActions({
  certificate,
  canIssueOrigin,
  onIssueACME,
  onIssueOrigin,
  onRenew,
  onRotate,
  onSync,
  onRevoke,
  onDelete,
}: {
  certificate: ManagedCertificate;
  canIssueOrigin: boolean;
  onIssueACME: () => void;
  onIssueOrigin: () => void;
  onRenew: () => void;
  onRotate: () => void;
  onSync: () => void;
  onRevoke: () => void;
  onDelete: () => void;
}) {
  const providerType = certificateProviderType(certificate);
  const isOrigin = providerType === ORIGIN_PROVIDER;
  const isFile = providerType === FILE_PROVIDER;
  const isAcme = !isOrigin && !isFile;
  const hasRemoteID = Boolean(certificate.cloudflareCertificateId);
  const hostLabel = certificate.host ?? certificate.proxyId;
  const boundProxyId = certificate.boundProxyId ?? '';
  const boundDomainId = certificate.boundDomainId ?? '';
  const strongDelete = certificate.deletionRisk === 'requires_strong_confirmation';
  const revokeReason = !hasRemoteID ? '缺少 Cloudflare 证书 ID，无法吊销。' : undefined;
  const issueOriginReason = !canIssueOrigin ? '没有可用的 Origin CA 凭据。' : undefined;

  return (
    <div className="inline-actions">
      {isOrigin ? (
        <>
          <ConfirmButton label="Rotate" confirmLabel={`Rotate ${hostLabel}?`} onConfirm={onRotate} tone="secondary" />
          <Button type="default" icon={<SyncOutlined aria-hidden="true" />} onClick={onSync}>
            Sync
          </Button>
          <ActionGate reason={revokeReason}>
            <ConfirmButton
              label="Revoke"
              confirmLabel={`Revoke active Origin CA certificate ${certificate.cloudflareCertificateId ?? certificate.proxyId}? Cloudflare to origin TLS will stop until a replacement is issued.`}
              onConfirm={onRevoke}
              disabled={!hasRemoteID}
            />
          </ActionGate>
        </>
      ) : null}

      {isAcme ? (
        <>
          <ConfirmButton label="Issue" confirmLabel={`Issue ACME certificate for ${hostLabel}?`} onConfirm={onIssueACME} tone="secondary" />
          <ConfirmButton label="Renew" confirmLabel={`Renew ${hostLabel}?`} onConfirm={onRenew} tone="secondary" />
          <ActionGate reason={issueOriginReason}>
            <ConfirmButton label="Issue Origin" confirmLabel={`Issue Origin CA certificate for ${hostLabel}?`} onConfirm={onIssueOrigin} tone="secondary" disabled={!canIssueOrigin} />
          </ActionGate>
        </>
      ) : null}

      {isFile ? (
        <ActionGate reason="文件登记证书不支持签发/续期/轮换/同步/吊销操作。">
          <Button type="default" disabled>
            No lifecycle actions
          </Button>
        </ActionGate>
      ) : null}

      {strongDelete ? (
        <ActionGate reason={`该证书正在为 Domain ${boundDomainId || boundProxyId || certificate.proxyId} 提供服务，删除需要强确认。`}>
          <Button type="primary" danger onClick={onDelete}>
            Delete
          </Button>
        </ActionGate>
      ) : (
        <Button type="primary" danger onClick={onDelete}>
          Delete
        </Button>
      )}
    </div>
  );
}

export function CertificateTable({
  items,
  pageInfo,
  canIssueOrigin,
  onPageChange,
  onIssueACME,
  onIssueOrigin,
  onRenew,
  onRotate,
  onSync,
  onRevoke,
  onDelete,
}: {
  items: ManagedCertificate[];
  pageInfo: PageInfo;
  canIssueOrigin: boolean;
  onPageChange: (page: number) => void;
  onIssueACME: (certificate: ManagedCertificate) => void;
  onIssueOrigin: (certificate: ManagedCertificate) => void;
  onRenew: (certificate: ManagedCertificate) => void;
  onRotate: (certificate: ManagedCertificate) => void;
  onSync: (certificate: ManagedCertificate) => void;
  onRevoke: (certificate: ManagedCertificate) => void;
  onDelete: (certificate: ManagedCertificate) => void;
}) {
  const columns = useMemo<TableColumnsType<ManagedCertificate>>(
    () => [
      {
        title: 'Certificate',
        key: 'certificate',
        width: 180,
        render: (_, certificate) => (
          <div className="cell-stack">
            <span>{certificate.certificateId || 'N/A'}</span>
            <span className="muted-text">{certificate.host ?? certificate.proxyId}</span>
          </div>
        ),
      },
      {
        title: 'Provider',
        key: 'provider',
        width: 180,
        render: (_, certificate) => {
          const providerType = certificateProviderType(certificate);
          return (
            <div className="cell-stack">
              <span>{formatTitle(providerType)}</span>
              {certificate.providerName ? <span className="muted-text">{certificate.providerName}</span> : null}
              {certificate.credentialId ? <span className="muted-text">{certificate.credentialId}</span> : null}
              {certificate.cloudflareCertificateId ? <span className="muted-text">{formatFingerprint(certificate.cloudflareCertificateId)}</span> : null}
              {providerType === ORIGIN_PROVIDER && (certificate.deploymentHints?.length ?? 0) > 0 ? (
                <span className="muted-text" title={certificate.deploymentHints?.join('\n')}>
                  {certificate.deploymentHints?.[0]}
                </span>
              ) : null}
            </div>
          );
        },
      },
      {
        title: 'Hostnames',
        key: 'hostnames',
        width: 160,
        render: (_, certificate) => {
          const hostnames = certificate.hostnames ?? [];
          return (
            <div className="cell-stack">
              {hostnames.length > 0 ? hostnames.map((name) => <span key={name}>{name}</span>) : <span className="muted-text">{certificate.host ?? 'N/A'}</span>}
            </div>
          );
        },
      },
      {
        title: 'Bound domain',
        key: 'boundDomain',
        width: 140,
        render: (_, certificate) => {
          const boundDomainId = certificate.boundDomainId ?? '';
          const boundProxyId = certificate.boundProxyId ?? '';
          if (boundDomainId) {
            return <span className="mono">{boundDomainId}</span>;
          }
          if (boundProxyId) {
            return (
              <span className="mono muted-text" title="legacy proxy binding">
                {boundProxyId}
              </span>
            );
          }
          return <span className="muted-text">未绑定</span>;
        },
      },
      {
        title: 'Use',
        key: 'use',
        width: 90,
        render: (_, certificate) => <StatusBadge value={certificate.referenced ? 'active' : 'idle'} />,
      },
      {
        title: 'Serving',
        key: 'serving',
        width: 110,
        render: (_, certificate) => <StatusBadge value={certificate.servingStatus ?? certificate.status ?? 'unknown'} />,
      },
      {
        title: 'Operation',
        key: 'operation',
        width: 120,
        render: (_, certificate) => <StatusBadge value={certificate.operationStatus ?? 'unknown'} />,
      },
      {
        title: 'Provider status',
        key: 'providerStatus',
        width: 130,
        render: (_, certificate) => <StatusBadge value={certificate.providerStatus ?? 'unknown'} />,
      },
      {
        title: 'Expires',
        dataIndex: 'notAfter',
        key: 'notAfter',
        width: 150,
        render: (value?: string | null) => <Timestamp value={value} />,
      },
      {
        title: 'Last sync',
        dataIndex: 'lastSyncedAt',
        key: 'lastSyncedAt',
        width: 150,
        render: (value?: string | null) => <Timestamp value={value} />,
      },
      {
        title: 'Failures',
        dataIndex: 'failureCount',
        key: 'failureCount',
        width: 90,
        align: 'right',
        render: (value?: number | null) => value ?? 0,
      },
      {
        title: 'Fingerprint',
        dataIndex: 'fingerprint',
        key: 'fingerprint',
        width: 150,
        render: (value?: string | null) => formatFingerprint(value),
      },
      {
        title: 'Last error',
        dataIndex: 'lastError',
        key: 'lastError',
        ellipsis: true,
        render: (value?: string | null) => value || 'None',
      },
      {
        title: 'Actions',
        key: 'actions',
        fixed: 'right',
        width: 320,
        render: (_, certificate) => (
          <CertificateActions
            certificate={certificate}
            canIssueOrigin={canIssueOrigin}
            onIssueACME={() => onIssueACME(certificate)}
            onIssueOrigin={() => onIssueOrigin(certificate)}
            onRenew={() => onRenew(certificate)}
            onRotate={() => onRotate(certificate)}
            onSync={() => onSync(certificate)}
            onRevoke={() => onRevoke(certificate)}
            onDelete={() => onDelete(certificate)}
          />
        ),
      },
    ],
    [canIssueOrigin, onDelete, onIssueACME, onIssueOrigin, onRenew, onRevoke, onRotate, onSync],
  );

  return (
    <div className="certificate-panel certificate-panel--table">
      <DataTable<ManagedCertificate>
        rowKey={(row) => row.certificateId || row.proxyId || `${row.host ?? 'cert'}`}
        columns={columns}
        dataSource={items}
        scroll={{ x: 1800 }}
        pagination={pageTablePagination(pageInfo, onPageChange, { itemLabel: 'certificates' })}
      />
    </div>
  );
}
