import { type FormEvent, useEffect, useMemo, useState } from 'react';
import { PlusOutlined, ReloadOutlined } from '@ant-design/icons';
import { Button, Collapse } from 'antd';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { useQueryClient } from '@tanstack/react-query';
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
  queryCertificateProviderReadiness,
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
import { PageHeader } from './shared';
import {
  defaultCredentialForm,
  defaultDimensionFilters,
  defaultFilter,
  ORIGIN_PROVIDER,
  type DimensionFilters,
} from './certificates/constants';
import { certificateMutationInput } from './certificates/helpers';
import { CertificateFiltersBar } from './certificates/CertificateFiltersBar';
import { CertificateTable } from './certificates/CertificateTable';
import { CredentialsPanel } from './certificates/CredentialsPanel';
import { ProviderReadinessSection } from './certificates/ProviderReadinessSection';

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
  const [showCreate, setShowCreate] = useState(false);
  const [createError, setCreateError] = useState<string>();
  const [createFieldErrors, setCreateFieldErrors] = useState<Record<string, string>>();
  const [deleteTarget, setDeleteTarget] = useState<ManagedCertificate | null>(null);
  const [deleteError, setDeleteError] = useState<string>();

  const createParam = searchParams.get(CREATE_PARAM);
  const returnTo = searchParams.get(RETURN_TO_PARAM) ?? '';
  const draftId = searchParams.get(DRAFT_ID_PARAM) ?? '';
  const hostHint = searchParams.get(HOST_HINT_PARAM) ?? '';
  const providerHint = searchParams.get(PROVIDER_HINT_PARAM) ?? '';
  const returnToProxy = Boolean(returnTo && draftId);

  useEffect(() => {
    if (createParam === '1') {
      setShowCreate(true);
      setCreateError(undefined);
      setCreateFieldErrors(undefined);
    }
  }, [createParam]);

  const pollMs = session.pollIntervalSeconds * 1000;
  const query = useAuthedQuery({
    queryKey: ['certificates', page, filter],
    queryFn: () => queryCertificates({ page: { page, pageSize: 10 }, sort: { field: 'host', direction: 'asc' }, filter }),
    refetchInterval: pollMs,
  });
  const credentialsQuery = useAuthedQuery({
    queryKey: ['providerCredentials'],
    queryFn: () => queryProviderCredentials({ page: { page: 1, pageSize: 100 } }),
    refetchInterval: pollMs,
  });
  const readinessQuery = useAuthedQuery({
    queryKey: ['certificateProviderReadiness'],
    queryFn: queryCertificateProviderReadiness,
    refetchInterval: pollMs,
  });

  const credentials = credentialsQuery.data?.items ?? [];
  const hasEnabledCredential = credentials.some((item) => item.status !== 'disabled');

  const invalidateCertificateViews = async (proxyId?: string) => {
    await queryClient.invalidateQueries({ queryKey: ['certificates'] });
    await queryClient.invalidateQueries({ queryKey: ['providerCredentials'] });
    await queryClient.invalidateQueries({ queryKey: ['certificateProviderReadiness'] });
    if (proxyId) {
      await queryClient.invalidateQueries({ queryKey: ['proxy', proxyId] });
    }
  };

  const invalidateAffectedProxies = async (proxyIds: string[]) => {
    await queryClient.invalidateQueries({ queryKey: ['certificates'] });
    await queryClient.invalidateQueries({ queryKey: ['proxies'] });
    for (const proxyId of proxyIds) {
      if (proxyId) {
        await queryClient.invalidateQueries({ queryKey: ['proxy', proxyId] });
      }
    }
  };

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
        window.alert(`证书已删除。受影响的代理：${affected.join(', ')}（已标记为需要重新配置）。`);
      }
    },
    onError: (error, variables) => {
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
  const certificatesById = useMemo(() => {
    const map = new Map<string, ManagedCertificate>();
    for (const item of allItems) {
      if (item.certificateId) {
        map.set(item.certificateId, item);
      }
    }
    return map;
  }, [allItems]);

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
    const input = {
      ...credentialForm,
      name: credentialForm.name ?? '',
      scope: credentialForm.scope ?? '',
      token: credentialForm.token ?? '',
    };
    if (editingCredentialId) {
      updateCredentialMutation.mutate({ ...input, id: editingCredentialId });
      return;
    }
    createCredentialMutation.mutate(input);
  };

  const requestDelete = (certificate: ManagedCertificate) => {
    setDeleteError(undefined);
    if (certificate.deletionRisk === 'requires_strong_confirmation') {
      setDeleteTarget(certificate);
      return;
    }
    deleteCertificateMutation.mutate({ certificateId: certificate.certificateId ?? '' });
  };

  const editCredential = (credential: ProviderCredential) => {
    setEditingCredentialId(credential.id);
    setCredentialForm({
      id: credential.id,
      name: credential.name,
      scope: credential.scope ?? '',
      token: '',
    });
  };

  return (
    <section className="page-section page-section--fill certificates-page">
      <PageHeader
        title="Certificates"
        description="Create certificates, manage Cloudflare credentials, and run lifecycle actions for HTTPS domains."
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

      <Collapse
        className="certificate-setup"
        defaultActiveKey={['readiness', 'credentials']}
        items={[
          {
            key: 'readiness',
            label: 'Provider readiness',
            children: (
              <ProviderReadinessSection
                items={readinessQuery.data ?? []}
                error={readinessQuery.error instanceof Error ? readinessQuery.error : null}
              />
            ),
          },
          {
            key: 'credentials',
            label: 'Cloudflare Credentials',
            children: (
              <CredentialsPanel
                credentials={credentials}
                form={credentialForm}
                editingId={editingCredentialId}
                loading={credentialsQuery.isLoading}
                error={credentialsQuery.error instanceof Error ? credentialsQuery.error : null}
                pending={createCredentialMutation.isPending || updateCredentialMutation.isPending}
                onFormChange={setCredentialForm}
                onSubmit={submitCredential}
                onCancelEdit={() => {
                  setCredentialForm(defaultCredentialForm);
                  setEditingCredentialId('');
                }}
                onEdit={editCredential}
                onVerify={(id) => verifyCredentialMutation.mutate(id)}
                onDisable={(id) => disableCredentialMutation.mutate(id)}
                onDelete={(id) => deleteCredentialMutation.mutate(id)}
              />
            ),
          },
        ]}
      />

      <CertificateFiltersBar
        filter={filter}
        dimensionFilters={dimensionFilters}
        credentials={credentials}
        selectedCredentialId={selectedCredentialId}
        onFilterChange={(next) => {
          setPage(1);
          setFilter(next);
        }}
        onDimensionChange={(next) => {
          setPage(1);
          setDimensionFilters(next);
        }}
        onCredentialChange={setSelectedCredentialId}
      />

      {actionError ? <div className="banner banner--danger">{formatTitle(actionError.message)}</div> : null}
      {deleteError && !deleteTarget ? <div className="banner banner--danger">{formatTitle(deleteError)}</div> : null}

      {visibleItems.length === 0 ? (
        hasFilter ? (
          <FilteredEmptyState onClear={clearAllFilters} />
        ) : (
          <EmptyState title="No certificates" message="Create a certificate or issue one for an HTTPS domain." />
        )
      ) : (
        <CertificateTable
          items={visibleItems}
          pageInfo={data.pageInfo}
          canIssueOrigin={hasEnabledCredential}
          onPageChange={setPage}
          onIssueACME={(certificate) => issueMutation.mutate(certificateMutationInput(certificate))}
          onIssueOrigin={(certificate) =>
            issueOriginMutation.mutate({
              ...certificateMutationInput(certificate),
              credentialId: selectedCredentialId || undefined,
            })
          }
          onRenew={(certificate) => renewMutation.mutate(certificateMutationInput(certificate))}
          onRotate={(certificate) => rotateOriginMutation.mutate(certificateMutationInput(certificate))}
          onSync={(certificate) => syncOriginMutation.mutate(certificateMutationInput(certificate))}
          onRevoke={(certificate) => revokeOriginMutation.mutate(certificate)}
          onDelete={requestDelete}
        />
      )}

      <CertificateCreateDialog
        open={showCreate}
        hostHint={hostHint}
        providerHint={providerHint}
        credentials={credentials}
        providerReadiness={readinessQuery.data ?? []}
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
