import { useEffect, useState } from 'react';
import { Button } from 'antd';
import { Dialog } from './Dialog';
import { TextField } from './FormField';
import type { ManagedCertificate } from '../lib/contracts';
import type { DeleteCertificateInput } from '../lib/admin-graphql';

// 强确认删除对话框：当证书已绑定代理且正在生效（deletionRisk === requires_strong_confirmation），
// 或后端返回 CONFIRMATION_REQUIRED 时使用。要求管理员手动键入证书主机名或证书 ID 才能删除。

export function CertificateDeleteDialog({
  open,
  certificate,
  pending,
  errorMessage,
  onConfirm,
  onClose,
}: {
  open: boolean;
  certificate: ManagedCertificate | null;
  pending: boolean;
  errorMessage?: string;
  onConfirm: (input: DeleteCertificateInput) => void;
  onClose: () => void;
}) {
  const [typed, setTyped] = useState('');

  useEffect(() => {
    if (open) {
      setTyped('');
    }
  }, [open, certificate?.certificateId]);

  if (!certificate) {
    return null;
  }

  const host = certificate.host ?? '';
  const certificateId = certificate.certificateId ?? '';
  const boundProxyId = certificate.boundProxyId ?? '';
  const trimmed = typed.trim();
  const matchesHost = Boolean(host) && trimmed === host;
  const matchesId = Boolean(certificateId) && trimmed === certificateId;
  const matched = matchesHost || matchesId;

  const submit = () => {
    if (!matched) {
      return;
    }
    const input: DeleteCertificateInput = { certificateId };
    if (matchesHost) {
      input.confirmHost = host;
    } else {
      input.confirmCertificateId = certificateId;
    }
    onConfirm(input);
  };

  return (
    <Dialog
      open={open}
      title="确认删除证书"
      onClose={onClose}
      footer={
        <>
          <Button type="default" onClick={onClose}>
            取消
          </Button>
          <Button type="primary" danger onClick={submit} disabled={pending || !matched}>
            {pending ? '删除中...' : '确认删除'}
          </Button>
        </>
      }
    >
      <div className="banner banner--danger">
        该证书已绑定并正在为代理
        {boundProxyId ? ` ${boundProxyId} ` : ' '}
        提供服务。删除后该代理将被标记为「需要重新配置」(needs-config)，在重新绑定可用证书前 HTTPS 将不可用。
      </div>
      <div className="stack">
        <p className="muted-text">
          请输入证书主机名
          {host ? <strong> {host} </strong> : ' '}
          或证书 ID
          {certificateId ? <strong> {certificateId} </strong> : ' '}
          以确认删除：
        </p>
        <TextField
          label="键入主机名或证书 ID 以确认"
          value={typed}
          placeholder={host || certificateId}
          onChange={(event) => setTyped(event.target.value)}
        />
      </div>
      {errorMessage ? <div className="banner banner--danger">{errorMessage}</div> : null}
    </Dialog>
  );
}
