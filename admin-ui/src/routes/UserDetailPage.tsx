import { useState } from 'react';
import { useParams } from 'react-router-dom';
import { useQueryClient } from '@tanstack/react-query';
import { ConfirmButton } from '../components/ConfirmButton';
import { Dialog } from '../components/Dialog';
import { TextField } from '../components/FormField';
import { EmptyState, ErrorState, NotFoundState, PageLoading, ValidationBanner } from '../components/PageStates';
import { useAuthedQuery } from '../hooks/useAuthedQuery';
import { useMutationWithAuth } from '../hooks/useMutationWithAuth';
import { mutateDisableUser, mutateSetUserPassword, queryUser } from '../lib/admin-graphql';
import { isApiError, isNotFoundError } from '../lib/contracts';
import { formatBytes } from '../lib/format';
import { useSession } from '../session';
import { DetailBackLink, PageHeader, StatusBadge, Timestamp } from './shared';

export function UserDetailPage() {
  const { id = '' } = useParams();
  const session = useSession();
  const queryClient = useQueryClient();
  const [passwordDialog, setPasswordDialog] = useState(false);
  const [password, setPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [errorMessage, setErrorMessage] = useState<string>();
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>();

  const query = useAuthedQuery({ queryKey: ['user', id], queryFn: () => queryUser(id) });

  const disableMutation = useMutationWithAuth({
    mutationFn: () => mutateDisableUser(session.csrfToken ?? '', id),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['user', id] });
      await queryClient.invalidateQueries({ queryKey: ['users'] });
    },
  });

  const passwordMutation = useMutationWithAuth({
    mutationFn: () => mutateSetUserPassword(session.csrfToken ?? '', { id, password }),
    onSuccess: async () => {
      setPasswordDialog(false);
      setPassword('');
      setConfirmPassword('');
      setFieldErrors(undefined);
      setErrorMessage(undefined);
      await queryClient.invalidateQueries({ queryKey: ['user', id] });
      await queryClient.invalidateQueries({ queryKey: ['users'] });
    },
    onError: (error) => {
      if (isApiError(error)) {
        setFieldErrors(error.fields);
        setErrorMessage(error.message);
      }
    },
  });

  if (query.isLoading) {
    return <PageLoading label="Loading user..." />;
  }
  if (query.error) {
    if (isNotFoundError(query.error)) {
      return <NotFoundState resource="User" />;
    }
    return <ErrorState title="User failed" message={query.error.message} retry={() => query.refetch()} />;
  }
  if (!query.data) {
    return <EmptyState title="No user" message="User detail is unavailable." />;
  }

  const user = query.data;

  return (
    <section className="page-section">
      <DetailBackLink to="/users" label="Back to users" />
      <PageHeader
        title={user.username}
        description={`User ID: ${user.id}`}
        actions={
          <>
            <button type="button" className="button button--secondary" onClick={() => setPasswordDialog(true)}>
              Set password
            </button>
            <ConfirmButton
              label="Disable user"
              confirmLabel="Disable this user?"
              onConfirm={() => disableMutation.mutate(undefined)}
              disabled={user.status === 'disabled' || disableMutation.isPending}
            />
          </>
        }
      />

      <div className="detail-grid">
        <article className="panel">
          <h2>Identity</h2>
          <dl className="detail-list">
            <div><dt>Role</dt><dd>{user.role}</dd></div>
            <div><dt>Status</dt><dd><StatusBadge value={user.status} /></dd></div>
            <div><dt>Password configured</dt><dd>{user.hasPasswordHash ? 'Yes' : 'No'}</dd></div>
          </dl>
        </article>
        <article className="panel">
          <h2>Activity</h2>
          <dl className="detail-list">
            <div><dt>Clients</dt><dd>{user.clientCount}</dd></div>
            <div><dt>Proxies</dt><dd>{user.proxyCount}</dd></div>
            <div><dt>Upload</dt><dd>{formatBytes(user.uploadBytes)}</dd></div>
            <div><dt>Download</dt><dd>{formatBytes(user.downloadBytes)}</dd></div>
            <div><dt>Last activity</dt><dd><Timestamp value={user.lastActivityAt} /></dd></div>
            <div><dt>Updated</dt><dd><Timestamp value={user.updatedAt} /></dd></div>
          </dl>
        </article>
      </div>

      <Dialog
        open={passwordDialog}
        title="Set password"
        onClose={() => setPasswordDialog(false)}
        footer={
          <>
            <button type="button" className="button button--secondary" onClick={() => setPasswordDialog(false)}>
              Cancel
            </button>
            <button
              type="button"
              className="button"
              disabled={passwordMutation.isPending}
              onClick={() => {
                setFieldErrors(undefined);
                if (password !== confirmPassword) {
                  setErrorMessage('Passwords do not match.');
                  return;
                }
                setErrorMessage(undefined);
                passwordMutation.mutate(undefined);
              }}
            >
              {passwordMutation.isPending ? 'Saving...' : 'Save password'}
            </button>
          </>
        }
      >
        {errorMessage ? <div className="banner banner--danger">{errorMessage}</div> : null}
        <ValidationBanner fields={fieldErrors} />
        <div className="stack">
          <TextField label="New password" type="password" value={password} error={fieldErrors?.password} onChange={(event) => setPassword(event.target.value)} />
          <TextField label="Confirm password" type="password" value={confirmPassword} onChange={(event) => setConfirmPassword(event.target.value)} />
        </div>
      </Dialog>
    </section>
  );
}
