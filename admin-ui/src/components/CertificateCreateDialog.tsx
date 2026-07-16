import { useEffect, useMemo, useState } from 'react';
import { Button } from 'antd';
import { Dialog } from './Dialog';
import { TextField, SelectField } from './FormField';
import { ValidationBanner } from './PageStates';
import type { CreateCertificateInput } from '../lib/admin-graphql';
import type { CertificateProviderReadiness, ProviderCredential } from '../lib/contracts';

// 创建证书对话框：集中入口，按 provider 类型展示不同字段。
// providerType 取值：
//  - acme_dns01           ACME DNS-01（自动签发）
//  - cloudflare_origin_ca Cloudflare Origin CA（仅 Cloudflare 到源站 TLS）
//  - file                 已有文件路径登记（仅录入服务器可访问的证书/私钥文件路径，绝不接受粘贴私钥文本）

export type CertificateProviderType = 'acme_dns01' | 'cloudflare_origin_ca' | 'file';

const PROVIDER_OPTIONS: { value: CertificateProviderType; label: string }[] = [
  { value: 'acme_dns01', label: 'ACME DNS-01（自动签发）' },
  { value: 'cloudflare_origin_ca', label: 'Cloudflare Origin CA（源站 TLS）' },
  { value: 'file', label: '已有文件路径登记' },
];

// Origin CA 部署提示：与代理证书选择保持一致的安全说明。
const ORIGIN_CA_HINTS = [
  'Origin CA 证书仅用于 Cloudflare 到源站之间的 TLS，浏览器不会直接信任。',
  '需要在 Cloudflare 侧使用 Full (strict) 等合适的 SSL 模式，否则源站连接可能不被校验。',
];

type FormState = {
  providerType: CertificateProviderType;
  host: string;
  credentialId: string;
  requestType: string;
  requestedValidity: string;
  certFile: string;
  keyFile: string;
};

function emptyForm(providerType: CertificateProviderType, host: string): FormState {
  return {
    providerType,
    host,
    credentialId: '',
    requestType: providerType === 'cloudflare_origin_ca' ? 'origin-ecc' : '',
    requestedValidity: providerType === 'cloudflare_origin_ca' ? '5475' : '',
    certFile: '',
    keyFile: '',
  };
}

function normalizeProviderType(value?: string | null): CertificateProviderType {
  if (value === 'cloudflare_origin_ca' || value === 'file') {
    return value;
  }
  return 'acme_dns01';
}

export function CertificateCreateDialog({
  open,
  hostHint,
  providerHint,
  credentials,
  providerReadiness,
  pending,
  errorMessage,
  fieldErrors,
  returnToProxy,
  onSubmit,
  onClose,
}: {
  open: boolean;
  hostHint?: string;
  providerHint?: string;
  credentials: ProviderCredential[];
  providerReadiness: CertificateProviderReadiness[];
  pending: boolean;
  errorMessage?: string;
  fieldErrors?: Record<string, string>;
  returnToProxy: boolean;
  onSubmit: (input: CreateCertificateInput) => void;
  onClose: () => void;
}) {
  const [form, setForm] = useState<FormState>(() => emptyForm(normalizeProviderType(providerHint), hostHint ?? ''));

  // 对话框打开时（含从 proxy 表单跳转预填）重置表单。
  useEffect(() => {
    if (open) {
      setForm(emptyForm(normalizeProviderType(providerHint), hostHint ?? ''));
    }
  }, [open, hostHint, providerHint]);

  const enabledCredentials = useMemo(
    () => credentials.filter((credential) => credential.status !== 'disabled'),
    [credentials],
  );

  const update = <K extends keyof FormState>(key: K, value: FormState[K]) => {
    setForm((current) => ({ ...current, [key]: value }));
  };

  const changeProvider = (value: CertificateProviderType) => {
    setForm((current) => ({ ...emptyForm(value, current.host), credentialId: current.credentialId }));
  };

  const isOrigin = form.providerType === 'cloudflare_origin_ca';
  const isFile = form.providerType === 'file';
  const readiness = providerReadiness.find((item) => item.providerType === form.providerType);
  const providerNotReady = readiness != null && !readiness.ready;
  const hostMissing = form.host.trim().length === 0;

  const submit = () => {
    const input: CreateCertificateInput = { host: form.host.trim(), providerType: form.providerType };
    if (isOrigin) {
      if (form.credentialId) {
        input.credentialId = form.credentialId;
      }
      if (form.requestType) {
        input.requestType = form.requestType;
      }
      if (form.requestedValidity) {
        input.requestedValidity = Number(form.requestedValidity);
      }
    }
    if (isFile) {
      input.certFile = form.certFile.trim();
      input.keyFile = form.keyFile.trim();
    }
    onSubmit(input);
  };

  return (
    <Dialog
      open={open}
      title="创建证书"
      onClose={onClose}
      footer={
        <>
          <Button type="default" onClick={onClose}>
            取消
          </Button>
          <Button type="primary" onClick={submit} disabled={pending || hostMissing || providerNotReady}>
            {pending ? '创建中...' : returnToProxy ? '创建并返回代理' : '创建证书'}
          </Button>
        </>
      }
    >
      {errorMessage ? <div className="banner banner--danger">{errorMessage}</div> : null}
      <ValidationBanner fields={fieldErrors} />
      {returnToProxy ? (
        <div className="banner">创建成功后将自动返回代理表单并选中该证书。</div>
      ) : null}
      <div className="stack">
        <SelectField
          label="证书来源"
          value={form.providerType}
          onChange={(event) => changeProvider(event.target.value as CertificateProviderType)}
        >
          {PROVIDER_OPTIONS.map((option) => (
            <option key={option.value} value={option.value}>
              {option.label}
            </option>
          ))}
        </SelectField>

        <TextField
          label="主机名"
          value={form.host}
          error={fieldErrors?.host}
          placeholder="例如 app.example.com"
          onChange={(event) => update('host', event.target.value)}
        />

        {providerNotReady ? (
          <div className="banner banner--danger">
            <div>当前证书来源尚未就绪：{readiness.missingRequirements.join('、')}</div>
            <div>{readiness.guidance}</div>
          </div>
        ) : null}

        {isOrigin ? (
          <>
            <SelectField
              label="Origin CA 凭据"
              value={form.credentialId}
              error={fieldErrors?.credentialId}
              hint={enabledCredentials.length === 0 ? '暂无可用凭据，将使用默认凭据。' : undefined}
              onChange={(event) => update('credentialId', event.target.value)}
            >
              <option value="">默认凭据</option>
              {enabledCredentials.map((credential) => (
                <option key={credential.id} value={credential.id}>
                  {credential.name}
                </option>
              ))}
            </SelectField>
            <SelectField
              label="请求类型"
              value={form.requestType}
              onChange={(event) => update('requestType', event.target.value)}
            >
              <option value="origin-ecc">origin-ecc</option>
              <option value="origin-rsa">origin-rsa</option>
            </SelectField>
            <TextField
              label="有效期（天）"
              type="number"
              value={form.requestedValidity}
              error={fieldErrors?.requestedValidity}
              onChange={(event) => update('requestedValidity', event.target.value)}
            />
            <div className="banner">
              <ul className="field-errors-list">
                {ORIGIN_CA_HINTS.map((hint) => (
                  <li key={hint}>{hint}</li>
                ))}
              </ul>
            </div>
          </>
        ) : null}

        {isFile ? (
          <>
            <TextField
              label="证书文件路径"
              value={form.certFile}
              error={fieldErrors?.certFile}
              placeholder="服务器可访问的证书文件路径"
              hint="登记服务器上已存在的证书文件路径，不要粘贴私钥文本。"
              onChange={(event) => update('certFile', event.target.value)}
            />
            <TextField
              label="私钥文件路径"
              value={form.keyFile}
              error={fieldErrors?.keyFile}
              placeholder="服务器可访问的私钥文件路径"
              hint="仅登记文件路径；系统不会回显私钥内容。"
              onChange={(event) => update('keyFile', event.target.value)}
            />
          </>
        ) : null}
      </div>
    </Dialog>
  );
}
