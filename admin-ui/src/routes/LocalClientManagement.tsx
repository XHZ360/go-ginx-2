import { useEffect, useState } from 'react';
import { DeleteOutlined, EditOutlined, PlusOutlined, SaveOutlined, ThunderboltOutlined } from '@ant-design/icons';
import { Button } from 'antd';
import { useQueryClient } from '@tanstack/react-query';
import { ConfirmButton } from '../components/ConfirmButton';
import { Dialog } from '../components/Dialog';
import { SelectField, TextAreaField, TextField } from '../components/FormField';
import { ValidationBanner } from '../components/PageStates';
import { useAuthedQuery } from '../hooks/useAuthedQuery';
import { useMutationWithAuth } from '../hooks/useMutationWithAuth';
import {
  mutateCreateLocalProxy,
  mutateDeleteLocalProxy,
  mutateDisableLocalProxy,
  mutateEnableLocalProxy,
  mutateReplaceLocalTargetAllowlist,
  mutateUpdateLocalProxy,
  queryLocalTargetAllowlist,
} from '../lib/admin-graphql';
import { isApiError, type ClientDetail, type LocalProxyInput, type LocalTargetAllowlistEntry, type ProxySummary } from '../lib/contracts';
import { useSession } from '../session';
import { StatusBadge } from './shared';

type AllowlistDraft = { id: string; cidr: string; portStart: string; portEnd: string };
type LocalProxyDraft = { name: string; type: string; entryBindHost: string; entryPort: string; targetHost: string; targetPort: string; description: string };

const emptyProxyDraft: LocalProxyDraft = { name: '', type: 'tcp', entryBindHost: '', entryPort: '', targetHost: '127.0.0.1', targetPort: '', description: '' };
let nextAllowlistDraftID = 0;

function newAllowlistDraft(entry: Omit<AllowlistDraft, 'id'>): AllowlistDraft {
  nextAllowlistDraftID += 1;
  return { id: `local-allowlist-${nextAllowlistDraftID}`, ...entry };
}

function allowlistDraft(entries: LocalTargetAllowlistEntry[]): AllowlistDraft[] {
  return entries.map((entry) => newAllowlistDraft({ cidr: entry.cidr, portStart: entry.portStart ? String(entry.portStart) : '', portEnd: entry.portEnd ? String(entry.portEnd) : '' }));
}

function proxyDraft(proxy?: ProxySummary): LocalProxyDraft {
  if (!proxy) return { ...emptyProxyDraft };
  return {
    name: proxy.name,
    type: proxy.type,
    entryBindHost: proxy.entryBindHost ?? '',
    entryPort: proxy.entryPort ? String(proxy.entryPort) : '',
    targetHost: proxy.targetHost ?? '',
    targetPort: proxy.targetPort ? String(proxy.targetPort) : '',
    description: proxy.description ?? '',
  };
}

function localProxyInput(form: LocalProxyDraft): LocalProxyInput {
  return {
    name: form.name,
    type: form.type,
    entryBindHost: form.entryBindHost || undefined,
    entryPort: Number(form.entryPort),
    targetHost: form.targetHost,
    targetPort: Number(form.targetPort),
    description: form.description || undefined,
  };
}

export function LocalClientManagement({ client }: { client: ClientDetail }) {
  const session = useSession();
  const queryClient = useQueryClient();
  const [allowlist, setAllowlist] = useState<AllowlistDraft[]>([]);
  const [allowlistError, setAllowlistError] = useState<string>();
  const [editingProxy, setEditingProxy] = useState<ProxySummary>();
  const [proxyDialogOpen, setProxyDialogOpen] = useState(false);
  const [proxyForm, setProxyForm] = useState<LocalProxyDraft>({ ...emptyProxyDraft });
  const [proxyError, setProxyError] = useState<string>();
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>();
  const [actionError, setActionError] = useState<string>();

  const allowlistQuery = useAuthedQuery({ queryKey: ['local-target-allowlist'], queryFn: queryLocalTargetAllowlist });
  useEffect(() => {
    if (allowlistQuery.data) setAllowlist(allowlistDraft(allowlistQuery.data.entries));
  }, [allowlistQuery.data]);

  const invalidate = async () => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ['client', client.id] }),
      queryClient.invalidateQueries({ queryKey: ['clients'] }),
      queryClient.invalidateQueries({ queryKey: ['proxies'] }),
    ]);
  };

  const allowlistMutation = useMutationWithAuth({
    mutationFn: () => mutateReplaceLocalTargetAllowlist(session.csrfToken ?? '', allowlist.map((entry) => ({ cidr: entry.cidr, portStart: Number(entry.portStart) || 0, portEnd: Number(entry.portEnd) || 0 }))),
    onSuccess: (result) => {
      setAllowlistError(undefined);
      setAllowlist(allowlistDraft(result.replaceLocalTargetAllowlist.entries));
      queryClient.invalidateQueries({ queryKey: ['local-target-allowlist'] });
    },
    onError: (error) => setAllowlistError(error.message),
  });

  const saveProxyMutation = useMutationWithAuth({
    mutationFn: async () => {
      const input = localProxyInput(proxyForm);
      if (editingProxy) {
        await mutateUpdateLocalProxy(session.csrfToken ?? '', { ...input, id: editingProxy.id });
      } else {
        await mutateCreateLocalProxy(session.csrfToken ?? '', input);
      }
    },
    onSuccess: async () => {
      setProxyDialogOpen(false);
      setProxyError(undefined);
      setFieldErrors(undefined);
      await invalidate();
    },
    onError: (error) => {
      setProxyError(error.message);
      if (isApiError(error)) setFieldErrors(error.fields);
    },
  });
  const enableMutation = useMutationWithAuth({ mutationFn: (id: string) => mutateEnableLocalProxy(session.csrfToken ?? '', id), onSuccess: invalidate, onError: (error) => setActionError(error.message) });
  const disableMutation = useMutationWithAuth({ mutationFn: (id: string) => mutateDisableLocalProxy(session.csrfToken ?? '', id), onSuccess: invalidate, onError: (error) => setActionError(error.message) });
  const deleteMutation = useMutationWithAuth({ mutationFn: (id: string) => mutateDeleteLocalProxy(session.csrfToken ?? '', id), onSuccess: invalidate, onError: (error) => setActionError(error.message) });

  const openProxyDialog = (proxy?: ProxySummary) => {
    setEditingProxy(proxy);
    setProxyForm(proxyDraft(proxy));
    setProxyError(undefined);
    setFieldErrors(undefined);
    setProxyDialogOpen(true);
  };

  return (
    <>
      <article className="panel">
        <div className="panel__header">
          <h2>Local target allowlist</h2>
          <div className="inline-actions">
            <Button icon={<PlusOutlined aria-hidden="true" />} onClick={() => setAllowlist((current) => [...current, newAllowlistDraft({ cidr: '', portStart: '', portEnd: '' })])}>Add entry</Button>
            <Button type="primary" icon={<SaveOutlined aria-hidden="true" />} loading={allowlistMutation.isPending} onClick={() => allowlistMutation.mutate(undefined)}>Save</Button>
          </div>
        </div>
        {allowlistQuery.error ? <div className="banner banner--danger">{allowlistQuery.error.message}</div> : null}
        {allowlistError ? <div className="banner banner--danger">{allowlistError}</div> : null}
        <div className="local-allowlist">
          {allowlist.map((entry, index) => (
            <div className="local-allowlist__row" key={entry.id}>
              <TextField label="IP or CIDR" value={entry.cidr} onChange={(event) => setAllowlist((current) => current.map((item, itemIndex) => itemIndex === index ? { ...item, cidr: event.target.value } : item))} />
              <TextField label="Port start" type="number" min="0" max="65535" value={entry.portStart} onChange={(event) => setAllowlist((current) => current.map((item, itemIndex) => itemIndex === index ? { ...item, portStart: event.target.value } : item))} />
              <TextField label="Port end" type="number" min="0" max="65535" value={entry.portEnd} onChange={(event) => setAllowlist((current) => current.map((item, itemIndex) => itemIndex === index ? { ...item, portEnd: event.target.value } : item))} />
              <Button className="local-allowlist__remove" danger icon={<DeleteOutlined aria-hidden="true" />} aria-label={`Remove allowlist entry ${index + 1}`} onClick={() => setAllowlist((current) => current.filter((_, itemIndex) => itemIndex !== index))} />
            </div>
          ))}
        </div>
      </article>

      <article className="panel">
        <div className="panel__header">
          <h2>Local proxies</h2>
          <Button type="primary" icon={<PlusOutlined aria-hidden="true" />} onClick={() => openProxyDialog()}>Create local proxy</Button>
        </div>
        {actionError ? <div className="banner banner--danger">{actionError}</div> : null}
        {client.managedProxies.length === 0 ? <p className="muted">No local proxies.</p> : (
          <div className="table-wrap">
            <table className="table">
              <thead><tr><th>Name</th><th>Type</th><th>Status</th><th>Entry</th><th>Target</th><th>Actions</th></tr></thead>
              <tbody>
                {client.managedProxies.map((proxy) => (
                  <tr key={proxy.id}>
                    <td>{proxy.name}</td><td>{proxy.type}</td><td><StatusBadge value={proxy.status} /></td>
                    <td>{proxy.entryBindHost || 'default'}:{proxy.entryPort}</td><td>{proxy.targetHost}:{proxy.targetPort}</td>
                    <td><div className="inline-actions">
                      <Button icon={<EditOutlined aria-hidden="true" />} aria-label={`Edit ${proxy.name}`} onClick={() => openProxyDialog(proxy)} />
                      {proxy.status === 'disabled' ? <Button icon={<ThunderboltOutlined aria-hidden="true" />} onClick={() => enableMutation.mutate(proxy.id)}>Enable</Button> : <ConfirmButton label="Disable" confirmLabel="Disable this local proxy?" onConfirm={() => disableMutation.mutate(proxy.id)} tone="secondary" />}
                      <ConfirmButton label="Delete" confirmLabel="Delete this disabled local proxy?" onConfirm={() => deleteMutation.mutate(proxy.id)} disabled={proxy.status !== 'disabled'} />
                    </div></td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </article>

      <Dialog open={proxyDialogOpen} title={editingProxy ? 'Edit local proxy' : 'Create local proxy'} onClose={() => setProxyDialogOpen(false)} footer={<><Button onClick={() => setProxyDialogOpen(false)}>Cancel</Button><Button type="primary" loading={saveProxyMutation.isPending} onClick={() => saveProxyMutation.mutate(undefined)}>Save proxy</Button></>}>
        {proxyError ? <div className="banner banner--danger">{proxyError}</div> : null}
        <ValidationBanner fields={fieldErrors} />
        <div className="toolbar-grid toolbar-grid--wide">
          <TextField label="Name" value={proxyForm.name} error={fieldErrors?.name} onChange={(event) => setProxyForm((current) => ({ ...current, name: event.target.value }))} />
          <SelectField label="Type" value={proxyForm.type} disabled={Boolean(editingProxy)} onChange={(event) => setProxyForm((current) => ({ ...current, type: event.target.value }))}><option value="tcp">TCP</option><option value="udp">UDP</option></SelectField>
          <TextField label="Bind host" value={proxyForm.entryBindHost} error={fieldErrors?.entryBindHost} onChange={(event) => setProxyForm((current) => ({ ...current, entryBindHost: event.target.value }))} />
          <TextField label="Entry port" type="number" min="1" max="65535" value={proxyForm.entryPort} error={fieldErrors?.entryPort} onChange={(event) => setProxyForm((current) => ({ ...current, entryPort: event.target.value }))} />
          <TextField label="Target IP" value={proxyForm.targetHost} error={fieldErrors?.targetHost ?? fieldErrors?.target} onChange={(event) => setProxyForm((current) => ({ ...current, targetHost: event.target.value }))} />
          <TextField label="Target port" type="number" min="1" max="65535" value={proxyForm.targetPort} error={fieldErrors?.targetPort ?? fieldErrors?.target} onChange={(event) => setProxyForm((current) => ({ ...current, targetPort: event.target.value }))} />
          <TextAreaField label="Description" value={proxyForm.description} onChange={(event) => setProxyForm((current) => ({ ...current, description: event.target.value }))} />
        </div>
      </Dialog>
    </>
  );
}
