import { type FormEvent, useMemo } from 'react';
import { CloudUploadOutlined, SyncOutlined } from '@ant-design/icons';
import { Button, Spin, type TableColumnsType } from 'antd';
import { ConfirmButton } from '../../components/ConfirmButton';
import { TextField } from '../../components/FormField';
import type { ProviderCredentialInput } from '../../lib/admin-graphql';
import type { ProviderCredential } from '../../lib/contracts';
import { formatTitle } from '../../lib/format';
import { DataTable, StatusBadge, Timestamp } from '../shared';
import { formatFingerprint } from './helpers';

export function CredentialsPanel({
  credentials,
  form,
  editingId,
  loading,
  error,
  pending,
  onFormChange,
  onSubmit,
  onCancelEdit,
  onEdit,
  onVerify,
  onDisable,
  onDelete,
}: {
  credentials: ProviderCredential[];
  form: ProviderCredentialInput;
  editingId: string;
  loading?: boolean;
  error?: Error | null;
  pending: boolean;
  onFormChange: (next: ProviderCredentialInput) => void;
  onSubmit: (event: FormEvent<HTMLFormElement>) => void;
  onCancelEdit: () => void;
  onEdit: (credential: ProviderCredential) => void;
  onVerify: (id: string) => void;
  onDisable: (id: string) => void;
  onDelete: (id: string) => void;
}) {
  const columns = useMemo<TableColumnsType<ProviderCredential>>(
    () => [
      { title: 'Name', dataIndex: 'name', key: 'name', ellipsis: true },
      {
        title: 'Status',
        dataIndex: 'status',
        key: 'status',
        width: 120,
        render: (value: string) => <StatusBadge value={value} />,
      },
      {
        title: 'Scope',
        dataIndex: 'scope',
        key: 'scope',
        ellipsis: true,
        render: (value?: string | null) => value || 'Default',
      },
      {
        title: 'Fingerprint',
        dataIndex: 'tokenFingerprint',
        key: 'tokenFingerprint',
        width: 160,
        render: (value?: string | null) => formatFingerprint(value),
      },
      {
        title: 'Verified',
        dataIndex: 'lastVerifiedAt',
        key: 'lastVerifiedAt',
        width: 160,
        render: (value?: string | null) => <Timestamp value={value} />,
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
        width: 280,
        render: (_, credential) => (
          <div className="inline-actions">
            <Button type="default" size="small" onClick={() => onEdit(credential)}>
              Edit
            </Button>
            <Button type="default" size="small" icon={<SyncOutlined aria-hidden="true" />} onClick={() => onVerify(credential.id)} disabled={credential.status === 'disabled'}>
              Verify
            </Button>
            <ConfirmButton
              label="Disable"
              confirmLabel={`Disable ${credential.name}?`}
              onConfirm={() => onDisable(credential.id)}
              tone="secondary"
              disabled={credential.status === 'disabled'}
            />
            <ConfirmButton label="Delete" confirmLabel={`Delete ${credential.name}?`} onConfirm={() => onDelete(credential.id)} />
          </div>
        ),
      },
    ],
    [onDelete, onDisable, onEdit, onVerify],
  );

  return (
    <Spin spinning={Boolean(loading)}>
      <div className="stack">
        <form className="toolbar-grid" onSubmit={onSubmit}>
          <TextField label="Name" value={form.name ?? ''} onChange={(event) => onFormChange({ ...form, name: event.target.value })} />
          <TextField label="Scope" value={form.scope ?? ''} onChange={(event) => onFormChange({ ...form, scope: event.target.value })} />
          <TextField
            label="API token"
            type="password"
            value={form.token ?? ''}
            onChange={(event) => onFormChange({ ...form, token: event.target.value })}
            hint={editingId ? 'Leave blank to keep the existing token.' : undefined}
          />
          <div className="field field--actions">
            <Button htmlType="submit" type="primary" icon={<CloudUploadOutlined aria-hidden="true" />} loading={pending}>
              {editingId ? 'Update' : 'Create'}
            </Button>
            {editingId ? (
              <Button type="default" onClick={onCancelEdit}>
                Cancel
              </Button>
            ) : null}
          </div>
        </form>

        {error ? <div className="banner banner--danger">{formatTitle(error.message)}</div> : null}

        {credentials.length > 0 ? (
          <div className="certificate-panel__table">
            <DataTable<ProviderCredential>
              compact
              rowKey="id"
              columns={columns}
              dataSource={credentials}
              pagination={false}
              scroll={{ x: 'max-content' }}
            />
          </div>
        ) : (
          <p className="muted">No Cloudflare credentials yet. Create one before issuing Origin CA certificates.</p>
        )}
      </div>
    </Spin>
  );
}
