import { useMemo } from 'react';
import { Button } from 'antd';
import { SelectField } from './FormField';
import { useAuthedQuery } from '../hooks/useAuthedQuery';
import { queryCertificates } from '../lib/admin-graphql';
import type { ManagedCertificate } from '../lib/contracts';
import { StatusBadge, Timestamp } from '../routes/shared';

// hostnameCovers 判断单个证书 hostname 是否覆盖目标 SNI 域名。
// 支持通配 `*.` 前缀：`*.example.com` 覆盖 `a.example.com`（仅单级子域）。
function hostnameCovers(pattern: string, host: string): boolean {
  const p = pattern.trim().toLowerCase();
  const h = host.trim().toLowerCase();
  if (!p || !h) {
    return false;
  }
  if (p === h) {
    return true;
  }
  if (p.startsWith('*.')) {
    const suffix = p.slice(1); // 含前导点，例如 ".example.com"
    if (!h.endsWith(suffix)) {
      return false;
    }
    // 通配只匹配单级标签，剩余部分不应再含点。
    const label = h.slice(0, h.length - suffix.length);
    return label.length > 0 && !label.includes('.');
  }
  return false;
}

// certificateHostnames 汇总证书声明的所有 hostname（host + hostnames[]）。
function certificateHostnames(cert: ManagedCertificate): string[] {
  const names = new Set<string>();
  if (cert.host) {
    names.add(cert.host);
  }
  for (const name of cert.hostnames ?? []) {
    if (name) {
      names.add(name);
    }
  }
  return Array.from(names);
}

// certificateCovers 判断证书是否覆盖给定 SNI 域名。
export function certificateCovers(cert: ManagedCertificate, host: string): boolean {
  if (!host) {
    return false;
  }
  return certificateHostnames(cert).some((name) => hostnameCovers(name, host));
}

// certificateBindable 判断证书是否可被当前 proxy 绑定：
// 未绑定（boundProxyId === ''）或已绑定到当前 proxy（编辑场景）。
function certificateBindable(cert: ManagedCertificate, proxyId?: string): boolean {
  const bound = cert.boundProxyId ?? '';
  if (bound === '') {
    return true;
  }
  return Boolean(proxyId) && bound === proxyId;
}

// 不可用原因，用于在 select 之外向管理员解释为何某证书被过滤或不可服务。
type Unusable = { kind: 'incompatible' | 'bound-elsewhere' | 'not-servable'; reason: string };

function describeUnusable(cert: ManagedCertificate, entryHost: string, proxyId?: string): Unusable | null {
  if (!certificateCovers(cert, entryHost)) {
    return { kind: 'incompatible', reason: '证书 hostnames 不覆盖该 SNI 域名' };
  }
  if (!certificateBindable(cert, proxyId)) {
    return { kind: 'bound-elsewhere', reason: '证书已绑定到其他 proxy' };
  }
  if (cert.servable === false) {
    return { kind: 'not-servable', reason: '证书当前不可服务' };
  }
  return null;
}

function certificateKey(cert: ManagedCertificate): string {
  return cert.certificateId ?? cert.proxyId;
}

function certificateLabel(cert: ManagedCertificate): string {
  const names = certificateHostnames(cert);
  const head = names[0] ?? cert.host ?? '(未命名证书)';
  const extra = names.length > 1 ? ` 等 ${names.length} 个域名` : '';
  const provider = cert.providerName || cert.providerType || '';
  const providerSuffix = provider ? ` · ${provider}` : '';
  return `${head}${extra}${providerSuffix}`;
}

export type CertificateSelectFieldProps = {
  // 当前 proxy 的 SNI 域名，用于兼容性匹配。
  entryHost: string;
  // 编辑场景下当前 proxy 的 id（用于放行已绑定到本 proxy 的证书）。
  proxyId?: string;
  // 当前选中的证书 id（绑定到 config.certificateId）。
  value: string;
  onChange: (certificateId: string) => void;
  // 「创建证书」跳转回调（保存草稿 + 导航由父表单负责）。
  onCreateCertificate?: () => void;
  error?: string;
};

// CertificateSelectField：HTTPS proxy 的证书选择控件。
// 仅列出「兼容」（hostnames 覆盖 SNI 域名）且「可绑定」（未绑定或已绑定到本 proxy）的证书。
export function CertificateSelectField({
  entryHost,
  proxyId,
  value,
  onChange,
  onCreateCertificate,
  error,
}: CertificateSelectFieldProps) {
  const certificatesQuery = useAuthedQuery({
    queryKey: ['proxy-certificate-options'],
    queryFn: () => queryCertificates({ page: { page: 1, pageSize: 200 }, sort: { field: 'host', direction: 'asc' } }),
  });

  const allCertificates = useMemo(() => certificatesQuery.data?.items ?? [], [certificatesQuery.data]);

  const usableCertificates = useMemo(
    () =>
      allCertificates.filter(
        (cert) =>
          certificateCovers(cert, entryHost) &&
          certificateBindable(cert, proxyId) &&
          cert.servable !== false,
      ),
    [allCertificates, entryHost, proxyId],
  );

  // 当前选中的证书（可能已不在可用列表中，例如被其他 proxy 抢绑或失去可服务能力）。
  const selectedCert = useMemo(
    () => allCertificates.find((cert) => certificateKey(cert) === value),
    [allCertificates, value],
  );
  const selectedUnusable = useMemo(
    () => (selectedCert ? describeUnusable(selectedCert, entryHost, proxyId) : null),
    [selectedCert, entryHost, proxyId],
  );

  // 被过滤掉的兼容证书（域名覆盖但绑定/可服务受限），用于解释“为什么没出现在列表里”。
  const filteredReasons = useMemo(() => {
    const reasons: string[] = [];
    let boundElsewhere = 0;
    let notServable = 0;
    for (const cert of allCertificates) {
      if (!certificateCovers(cert, entryHost)) {
        continue;
      }
      const unusable = describeUnusable(cert, entryHost, proxyId);
      if (!unusable) {
        continue;
      }
      if (unusable.kind === 'bound-elsewhere') {
        boundElsewhere += 1;
      } else if (unusable.kind === 'not-servable') {
        notServable += 1;
      }
    }
    if (boundElsewhere > 0) {
      reasons.push(`${boundElsewhere} 个匹配域名的证书已绑定到其他 proxy`);
    }
    if (notServable > 0) {
      reasons.push(`${notServable} 个匹配域名的证书当前不可服务`);
    }
    return reasons;
  }, [allCertificates, entryHost, proxyId]);

  const needsDomain = !entryHost.trim();
  const noUsable = !needsDomain && usableCertificates.length === 0;

  // 即使当前选中证书已不在可用列表，也要保留一个 option，避免 select 丢失展示。
  const selectedMissingFromList =
    Boolean(value) && !usableCertificates.some((cert) => certificateKey(cert) === value);

  return (
    <div className="field-stack">
      <SelectField
        label="证书"
        value={value}
        error={error}
        hint={needsDomain ? '请先填写 SNI 域名以匹配可用证书' : undefined}
        onChange={(event) => onChange(event.target.value)}
        disabled={needsDomain}
      >
        <option value="">不绑定证书（稍后配置）</option>
        {selectedMissingFromList && selectedCert ? (
          <option value={value}>{certificateLabel(selectedCert)}（当前选择，已不可用）</option>
        ) : null}
        {usableCertificates.map((cert) => (
          <option key={certificateKey(cert)} value={certificateKey(cert)}>
            {certificateLabel(cert)}
          </option>
        ))}
      </SelectField>

      <div className="inline-actions">
        {onCreateCertificate ? (
          <Button type="link" onClick={onCreateCertificate} disabled={certificatesQuery.isLoading}>
            创建证书
          </Button>
        ) : null}
      </div>

      {certificatesQuery.error ? (
        <div className="banner banner--danger">证书列表加载失败：{certificatesQuery.error.message}</div>
      ) : null}

      {selectedUnusable ? (
        <div className="banner banner--warning">当前选中的证书不可用：{selectedUnusable.reason}</div>
      ) : null}

      {noUsable ? (
        <div className="banner banner--warning">
          没有覆盖该 SNI 域名且可绑定的证书。
          {filteredReasons.length > 0 ? `（${filteredReasons.join('；')}）` : null}
          {onCreateCertificate ? ' 可点击上方「创建证书」新建一张。' : null}
        </div>
      ) : filteredReasons.length > 0 ? (
        <div className="field__hint">部分匹配域名的证书未列出：{filteredReasons.join('；')}。</div>
      ) : null}

      {selectedCert ? <CertificateSummary cert={selectedCert} /> : null}
    </div>
  );
}

// CertificateSummary：展示所选证书的关键信息（provider、hostnames、有效期、服务状态、部署提示）。
function CertificateSummary({ cert }: { cert: ManagedCertificate }) {
  const hostnames = certificateHostnames(cert);
  const isOriginCa = (cert.providerType ?? '').toLowerCase().includes('origin');
  const hints = isOriginCa ? cert.deploymentHints ?? [] : [];
  return (
    <article className="panel panel--inset">
      <dl className="detail-list">
        <div>
          <dt>Provider</dt>
          <dd>{cert.providerName || cert.providerType || 'N/A'}</dd>
        </div>
        <div>
          <dt>Hostnames</dt>
          <dd>{hostnames.length > 0 ? hostnames.join(', ') : 'N/A'}</dd>
        </div>
        <div>
          <dt>有效期至</dt>
          <dd><Timestamp value={cert.notAfter} /></dd>
        </div>
        <div>
          <dt>服务状态</dt>
          <dd><StatusBadge value={cert.servingStatus ?? cert.status} /></dd>
        </div>
        {hints.length > 0 ? (
          <div>
            <dt>部署提示</dt>
            <dd>
              <ul className="hint-list">
                {hints.map((hint, index) => (
                  <li key={index}>{hint}</li>
                ))}
              </ul>
            </dd>
          </div>
        ) : null}
      </dl>
    </article>
  );
}
