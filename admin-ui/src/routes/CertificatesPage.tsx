import { type FormEvent, type ReactElement, useEffect, useMemo, useState } from 'react';
import { CloudUploadOutlined, PlusOutlined, ReloadOutlined, SyncOutlined } from '@ant-design/icons';
import { Button, Tooltip } from 'antd';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { useQueryClient } from '@tanstack/react-query';
import { ConfirmButton } from '../components/ConfirmButton';
import { CertificateCreateDialog } from '../components/CertificateCreateDialog';
import { CertificateDeleteDialog } from '../components/CertificateDeleteDialog';
import { EmptyState, ErrorState, FilteredEmptyState, PageLoading } from '../components/PageStates';
import { useAuthedQuery } from '../hooks/useAuthedQuery';
import { useMutationWithAuth } from '../hooks/useMutationWithAuth';
import {
  mutateCreateCertificate,
  mutateCreateProviderCredential,
  mutateDeleteCertificate,
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
  type CertificateMutationInput,
  type CertificateFilter,
  type CreateCertificateInput,
  type DeleteCertificateInput,
  type ProviderCredentialInput,
} from '../lib/admin-graphql';
import { isApiError, type ManagedCertificate, type ProviderCredential } from '../lib/contracts';
import { formatTitle } from '../lib/format';
import {
  appendCreatedCertificate,
  CREATE_PARAM,
  DRAFT_ID_PARAM,
  HOST_HINT_PARAM,
  PROVIDER_HINT_PARAM,
  RETURN_TO_PARAM,
} from '../lib/proxy-draft';
import { useSession } from '../session';
import { PageHeader, Pagination, StatusBadge, Timestamp } from './shared';

const defaultFilter: CertificateFilter = { query: '', status: '' };
const defaultCredentialForm: ProviderCredentialInput = { id: '', name: '', scope: '', token: '' };

// 客户端附加的状态维度筛选（后端单一 status 过滤器只覆盖 serving 维度，这里把各维度拆开，避免歧义）。
type DimensionFilters = {
  operation: string;
  provider: string;
  providerType: string;
};

const defaultDimensionFilters: DimensionFilters = { operation: '', provider: '', providerType: '' };

const ORIGIN_PROVIDER = 'cloudflare_origin_ca';
const FILE_PROVIDER = 'file';

function certificateMutationInput(certificate: ManagedCertificate): CertificateMutationInput {
  return {
    proxyId: certificate.boundProxyId || certificate.proxyId || undefined,
    certificateId: certificate.certificateId || undefined,
  };
}

export function CertificatesPage() {
  const session = useSession();
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  const [page, setPage] = useState(1);
  const [filter, setFilter] = useState<CertificateFilter>(defaultFilter);
  const [dimensionFilters, setDimensionFilters] = useState<DimensionFilters>(defaultDimensionFilters);
  const [credentialForm, setCredentialForm] = useState<ProviderCredentialInput>(defaultCredentialForm);
  const [editingCredentialId, setEditingCredentialId] = useState('');
  const [selectedCredentialId, setSelectedCredentialId] = useState('');

  // 创建证书对话框状态。
  const [showCreate, setShowCreate] = useState(false);
  const [createError, setCreateError] = useState<string>();
  const [createFieldErrors, setCreateFieldErrors] = useState<Record<string, string>>();

  // 删除证书状态：低风险直接删除；高风险（或后端要求确认）走强确认对话框。
  const [deleteTarget, setDeleteTarget] = useState<ManagedCertificate | null>(null);
  const [deleteError, setDeleteError] = useState<string>();

  // 从 proxy 表单跳转过来的导航上下文。
  const createParam = searchParams.get(CREATE_PARAM);
  const returnTo = searchParams.get(RETURN_TO_PARAM) ?? '';
  const draftId = searchParams.get(DRAFT_ID_PARAM) ?? '';
  const hostHint = searchParams.get(HOST_HINT_PARAM) ?? '';
  const providerHint = searchParams.get(PROVIDER_HINT_PARAM) ?? '';
  const returnToProxy = Boolean(returnTo && draftId);

  // 挂载/参数变化时：?create=1 自动打开创建对话框并预填。
  useEffect(() => {
    if (createParam === '1') {
      setShowCreate(true);
      setCreateError(undefined);
      setCreateFieldErrors(undefined);
    }
  }, [createParam]);

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

  // 删除可能影响多个代理，逐一失效其详情视图。
  const invalidateAffectedProxies = async (proxyIds: string[]) => {
    await queryClient.invalidateQueries({ queryKey: ['certificates'] });
    await queryClient.invalidateQueries({ queryKey: ['proxies'] });
    for (const proxyId of proxyIds) {
      if (proxyId) {
        await queryClient.invalidateQueries({ queryKey: ['proxy', proxyId] });
      }
    }
  };

  // 清理创建相关的 query param（关闭对话框时调用）。
  const clearCreateParams = () => {
    setSearchParams(
      (current) => {
        const next = new URLSearchParams(current);
        next.delete(CREATE_PARAM);
        next.delete(RETURN_TO_PARAM);
        next.delete(DRAFT_ID_PARAM);
        next.delete(HOST_HINT_PARAM);
        next.delete(PROVIDER_HINT_PARAM);
        return next;
      },
      { replace: true },
    );
  };

  const closeCreate = () => {
    setShowCreate(false);
    setCreateError(undefined);
    setCreateFieldErrors(undefined);
    clearCreateParams();
  };

  const issueMutation = useMutationWithAuth({
    mutationFn: (input: CertificateMutationInput) => mutateIssueCertificate(session.csrfToken ?? '', input),
    onSuccess: async (_, input) => invalidateCertificateViews(input.proxyId),
  });

  const issueOriginMutation = useMutationWithAuth({
    mutationFn: (input: CertificateMutationInput) =>
      mutateIssueCertificate(session.csrfToken ?? '', {
        ...input,
        providerType: ORIGIN_PROVIDER,
        credentialId: input.credentialId,
        requestType: 'origin-ecc',
        requestedValidity: 5475,
      }),
    onSuccess: async (_, input) => invalidateCertificateViews(input.proxyId),
  });

  const renewMutation = useMutationWithAuth({
    mutationFn: (input: CertificateMutationInput) => mutateRenewCertificate(session.csrfToken ?? '', input),
    onSuccess: async (_, input) => invalidateCertificateViews(input.proxyId),
  });

  const rotateOriginMutation = useMutationWithAuth({
    mutationFn: (input: CertificateMutationInput) => mutateRotateOriginCertificate(session.csrfToken ?? '', input),
    onSuccess: async (_, input) => invalidateCertificateViews(input.proxyId),
  });

  const syncOriginMutation = useMutationWithAuth({
    mutationFn: (input: CertificateMutationInput) => mutateSyncOriginCertificate(session.csrfToken ?? '', input),
    onSuccess: async (_, input) => invalidateCertificateViews(input.proxyId),
  });

  const revokeOriginMutation = useMutationWithAuth({
    mutationFn: (certificate: ManagedCertificate) => {
      const input = certificateMutationInput(certificate);
      return mutateRevokeOriginCertificate(session.csrfToken ?? '', {
        ...input,
        host: certificate.host ?? '',
        cloudflareCertificateId: certificate.cloudflareCertificateId ?? '',
      });
    },
    onSuccess: async (_, certificate) => invalidateCertificateViews(certificateMutationInput(certificate).proxyId),
  });

  const createCertificateMutation = useMutationWithAuth({
    mutationFn: (input: CreateCertificateInput) => mutateCreateCertificate(session.csrfToken ?? '', input),
    onSuccess: async (result) => {
      const createdId = result.createCertificate?.certificate?.certificateId ?? '';
      await invalidateCertificateViews();
      // 来自 proxy 表单：携带新证书 ID 返回，选中后继续编辑代理。
      if (returnToProxy && createdId) {
        const target = appendCreatedCertificate(returnTo, draftId, createdId);
        setShowCreate(false);
        setCreateError(undefined);
        setCreateFieldErrors(undefined);
        navigate(target);
        return;
      }
      closeCreate();
    },
    onError: (error) => {
      if (isApiError(error)) {
        setCreateFieldErrors(error.fields);
        setCreateError(error.message);
        return;
      }
      setCreateError(error.message);
    },
  });

  const deleteCertificateMutation = useMutationWithAuth({
    mutationFn: (input: DeleteCertificateInput) => mutateDeleteCertificate(session.csrfToken ?? '', input),
    onSuccess: async (result) => {
      const affected = result.deleteCertificate?.affectedProxyIds ?? [];
      await invalidateAffectedProxies(affected);
      setDeleteTarget(null);
      setDeleteError(undefined);
      if (affected.length > 0) {
        // 提示受影响代理（已被标记为 needs-config）。
        window.alert(`证书已删除。受影响的代理：${affected.join(', ')}（已标记为需要重新配置）。`);
      }
    },
    onError: (error, variables) => {
      // 防御：原以为低风险，但后端要求强确认 → 切换到强确认对话框。
      if (isApiError(error) && error.code === 'CONFIRMATION_REQUIRED') {
        const target = certificatesById.get(variables.certificateId) ?? deleteTarget;
        if (target) {
          setDeleteTarget(target);
        }
        setDeleteError('该证书仍被代理引用，请键入主机名或证书 ID 以确认删除。');
        return;
      }
      setDeleteError(error.message);
    },
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

  const allItems = query.data?.items ?? [];

  // 按证书 ID 建索引，供删除错误回退时定位目标。
  const certificatesById = useMemo(() => {
    const map = new Map<string, ManagedCertificate>();
    for (const item of allItems) {
      if (item.certificateId) {
        map.set(item.certificateId, item);
      }
    }
    return map;
  }, [allItems]);

  // 客户端按维度筛选（serving 由后端 filter.status 负责）。
  const visibleItems = useMemo(() => {
    return allItems.filter((item) => {
      if (dimensionFilters.operation && (item.operationStatus ?? '') !== dimensionFilters.operation) {
        return false;
      }
      if (dimensionFilters.provider && (item.providerStatus ?? '') !== dimensionFilters.provider) {
        return false;
      }
      if (dimensionFilters.providerType) {
        const type = item.providerType || 'acme_dns01';
        if (type !== dimensionFilters.providerType) {
          return false;
        }
      }
      return true;
    });
  }, [allItems, dimensionFilters]);

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
  const hasServerFilter = Boolean(filter.query || filter.status);
  const hasDimensionFilter = Boolean(dimensionFilters.operation || dimensionFilters.provider || dimensionFilters.providerType);
  const hasFilter = hasServerFilter || hasDimensionFilter;
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

  const clearAllFilters = () => {
    setFilter(defaultFilter);
    setDimensionFilters(defaultDimensionFilters);
  };

  const submitCredential = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const input = { ...credentialForm, name: credentialForm.name ?? '', scope: credentialForm.scope ?? '', token: credentialForm.token ?? '' };
    if (editingCredentialId) {
      updateCredentialMutation.mutate({ ...input, id: editingCredentialId });
      return;
    }
    createCredentialMutation.mutate(input);
  };

  // 删除分流：低风险直接删；高风险打开强确认对话框。
  const requestDelete = (certificate: ManagedCertificate) => {
    setDeleteError(undefined);
    if (certificate.deletionRisk === 'requires_strong_confirmation') {
      setDeleteTarget(certificate);
      return;
    }
    deleteCertificateMutation.mutate({ certificateId: certificate.certificateId ?? '' });
  };

  return (
    <section className="page-section">
      <PageHeader
        title="Certificates"
        description="Managed certificate status for HTTPS proxies."
        actions={
          <>
            <Button type="default" icon={<ReloadOutlined aria-hidden="true" />} onClick={() => query.refetch()}>
              Refresh
            </Button>
            <Button
              type="primary"
              icon={<PlusOutlined aria-hidden="true" />}
              onClick={() => {
                setCreateError(undefined);
                setCreateFieldErrors(undefined);
                setShowCreate(true);
              }}
            >
              Create certificate
            </Button>
          </>
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
          </select>
        </label>
        <label className="field">
          <span className="field__label">Operation status</span>
          <select className="input" value={dimensionFilters.operation} onChange={(event) => setDimensionFilters((current) => ({ ...current, operation: event.target.value }))}>
            <option value="">All</option>
            <option value="idle">Idle</option>
            <option value="pending">Pending</option>
            <option value="issue_failed">Issue failed</option>
            <option value="renewal_failed">Renewal failed</option>
          </select>
        </label>
        <label className="field">
          <span className="field__label">Provider status</span>
          <select className="input" value={dimensionFilters.provider} onChange={(event) => setDimensionFilters((current) => ({ ...current, provider: event.target.value }))}>
            <option value="">All</option>
            <option value="active">Active</option>
            <option value="revoked">Revoked</option>
            <option value="missing_remote">Remote missing</option>
            <option value="unknown">Unknown</option>
          </select>
        </label>
        <label className="field">
          <span className="field__label">Provider type</span>
          <select className="input" value={dimensionFilters.providerType} onChange={(event) => setDimensionFilters((current) => ({ ...current, providerType: event.target.value }))}>
            <option value="">All</option>
            <option value="acme_dns01">ACME DNS-01</option>
            <option value={ORIGIN_PROVIDER}>Cloudflare Origin CA</option>
            <option value={FILE_PROVIDER}>File-backed</option>
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

      {visibleItems.length === 0 ? (
        hasFilter ? <FilteredEmptyState onClear={clearAllFilters} /> : <EmptyState title="No certificates" message="HTTPS proxies will appear here for lifecycle actions." />
      ) : (
        <>
          <div className="table-wrap">
            <table className="table">
              <thead>
                <tr>
                  <th>Certificate</th>
                  <th>Provider</th>
                  <th>Hostnames</th>
                  <th>Bound proxy</th>
                  <th>Use</th>
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
                {visibleItems.map((certificate) => (
                  <CertificateRow
                    key={certificate.certificateId || certificate.proxyId}
                    certificate={certificate}
                    canIssueOrigin={hasEnabledCredential}
                    issueACME={() => issueMutation.mutate(certificateMutationInput(certificate))}
                    issueOrigin={() => issueOriginMutation.mutate({ ...certificateMutationInput(certificate), credentialId: selectedCredentialId || undefined })}
                    renew={() => renewMutation.mutate(certificateMutationInput(certificate))}
                    rotate={() => rotateOriginMutation.mutate(certificateMutationInput(certificate))}
                    sync={() => syncOriginMutation.mutate(certificateMutationInput(certificate))}
                    revoke={() => revokeOriginMutation.mutate(certificate)}
                    onDelete={() => requestDelete(certificate)}
                  />
                ))}
              </tbody>
            </table>
          </div>
          <Pagination page={data.pageInfo.page} totalPages={data.pageInfo.totalPages} onPageChange={setPage} />
        </>
      )}

      {actionError ? <div className="banner banner--danger">{formatTitle(actionError.message)}</div> : null}
      {deleteError && !deleteTarget ? <div className="banner banner--danger">{formatTitle(deleteError)}</div> : null}

      <CertificateCreateDialog
        open={showCreate}
        hostHint={hostHint}
        providerHint={providerHint}
        credentials={credentials}
        pending={createCertificateMutation.isPending}
        errorMessage={createError}
        fieldErrors={createFieldErrors}
        returnToProxy={returnToProxy}
        onSubmit={(input) => createCertificateMutation.mutate(input)}
        onClose={closeCreate}
      />

      <CertificateDeleteDialog
        open={Boolean(deleteTarget)}
        certificate={deleteTarget}
        pending={deleteCertificateMutation.isPending}
        errorMessage={deleteError}
        onConfirm={(input) => deleteCertificateMutation.mutate(input)}
        onClose={() => {
          setDeleteTarget(null);
          setDeleteError(undefined);
        }}
      />
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

// DisabledHint：动作不可用时给出可理解的原因（tooltip 包裹）。
function ActionGate({ reason, children }: { reason?: string; children: ReactElement }) {
  if (!reason) {
    return children;
  }
  return <Tooltip title={reason}>{children}</Tooltip>;
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
  onDelete,
}: {
  certificate: ManagedCertificate;
  canIssueOrigin: boolean;
  issueACME: () => void;
  issueOrigin: () => void;
  renew: () => void;
  rotate: () => void;
  sync: () => void;
  revoke: () => void;
  onDelete: () => void;
}) {
  const providerType = certificate.providerType || 'acme_dns01';
  const isOrigin = providerType === ORIGIN_PROVIDER;
  const isFile = providerType === FILE_PROVIDER;
  const isAcme = !isOrigin && !isFile;
  const hasRemoteID = Boolean(certificate.cloudflareCertificateId);
  const hostLabel = certificate.host ?? certificate.proxyId;
  const hostnames = certificate.hostnames ?? [];
  const boundProxyId = certificate.boundProxyId ?? '';
  const referenced = Boolean(certificate.referenced);
  const strongDelete = certificate.deletionRisk === 'requires_strong_confirmation';

  // 动作可用性原因（用于禁用 tooltip）。
  const originActionReason = isFile
    ? '文件登记证书不支持签发/轮换/同步/吊销操作。'
    : isAcme
      ? '该操作仅适用于 Cloudflare Origin CA 证书。'
      : undefined;
  const acmeActionReason = isFile
    ? '文件登记证书不支持签发/续期操作。'
    : isOrigin
      ? '该操作仅适用于 ACME 证书。'
      : undefined;
  const revokeReason = !hasRemoteID ? '缺少 Cloudflare 证书 ID，无法吊销。' : undefined;
  const issueOriginReason = !canIssueOrigin ? '没有可用的 Origin CA 凭据。' : undefined;

  return (
    <tr>
      <td>
        <div className="cell-stack">
          <span>{certificate.certificateId || 'N/A'}</span>
          <span className="muted-text">{hostLabel}</span>
        </div>
      </td>
      <td>
        <div className="cell-stack">
          <span>{formatTitle(providerType)}</span>
          {certificate.providerName ? <span className="muted-text">{certificate.providerName}</span> : null}
          {certificate.credentialId ? <span className="muted-text">{certificate.credentialId}</span> : null}
          {certificate.cloudflareCertificateId ? <span className="muted-text">{formatFingerprint(certificate.cloudflareCertificateId)}</span> : null}
          {isOrigin && (certificate.deploymentHints?.length ?? 0) > 0 ? (
            <span className="muted-text" title={certificate.deploymentHints?.join('\n')}>
              {certificate.deploymentHints?.[0]}
            </span>
          ) : null}
        </div>
      </td>
      <td>
        <div className="cell-stack">
          {hostnames.length > 0 ? hostnames.map((name) => <span key={name}>{name}</span>) : <span className="muted-text">{certificate.host ?? 'N/A'}</span>}
        </div>
      </td>
      <td>{boundProxyId ? boundProxyId : <span className="muted-text">未绑定</span>}</td>
      <td><StatusBadge value={referenced ? 'active' : 'idle'} /></td>
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
              <ConfirmButton label="Rotate" confirmLabel={`Rotate ${hostLabel}?`} onConfirm={rotate} tone="secondary" />
              <Button type="default" icon={<SyncOutlined aria-hidden="true" />} onClick={sync}>Sync</Button>
              <ActionGate reason={revokeReason}>
                <ConfirmButton label="Revoke" confirmLabel={`Revoke active Origin CA certificate ${certificate.cloudflareCertificateId ?? certificate.proxyId}? Cloudflare to origin TLS will stop until a replacement is issued.`} onConfirm={revoke} disabled={!hasRemoteID} />
              </ActionGate>
            </>
          ) : null}

          {isAcme ? (
            <>
              <ConfirmButton label="Issue" confirmLabel={`Issue ACME certificate for ${hostLabel}?`} onConfirm={issueACME} tone="secondary" />
              <ConfirmButton label="Renew" confirmLabel={`Renew ${hostLabel}?`} onConfirm={renew} tone="secondary" />
              <ActionGate reason={issueOriginReason}>
                <ConfirmButton label="Issue Origin" confirmLabel={`Issue Origin CA certificate for ${hostLabel}?`} onConfirm={issueOrigin} tone="secondary" disabled={!canIssueOrigin} />
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
            <ActionGate reason={`该证书正在为代理 ${boundProxyId || certificate.proxyId} 提供服务，删除需要强确认。`}>
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
