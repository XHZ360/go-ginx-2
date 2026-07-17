import { Button, Space, Tag, Typography } from 'antd';
import type { CertificateProviderReadiness } from '../../lib/contracts';
import { ORIGIN_PROVIDER } from './constants';

function providerLabel(providerType: string) {
  return providerType === ORIGIN_PROVIDER ? 'Cloudflare Origin CA' : 'ACME DNS-01';
}

function ReadinessItem({ readiness }: { readiness: CertificateProviderReadiness }) {
  const copyDiagnostic = () => {
    const diagnostic = [
      `provider=${readiness.providerType}`,
      `ready=${readiness.ready}`,
      `missingRequirements=${readiness.missingRequirements.join(',')}`,
      `guidance=${readiness.guidance ?? ''}`,
    ].join('\n');
    void navigator.clipboard?.writeText(diagnostic);
  };

  return (
    <div className={`banner ${readiness.ready ? 'banner--success' : 'banner--danger'} readiness-item`}>
      <div className="panel__header">
        <Space wrap>
          <Typography.Text strong>{providerLabel(readiness.providerType)}</Typography.Text>
          <Tag color={readiness.ready ? 'success' : 'error'}>{readiness.ready ? 'Ready' : 'Not ready'}</Tag>
        </Space>
        {!readiness.ready ? (
          <Button type="link" size="small" onClick={copyDiagnostic}>
            复制诊断信息
          </Button>
        ) : null}
      </div>
      <div>
        {readiness.ready
          ? '已就绪'
          : `缺少 ${readiness.missingRequirements.join('、') || 'unknown requirements'}`}
        {readiness.tokenEnvName ? `；环境变量 ${readiness.tokenEnvName}` : ''}
      </div>
      {readiness.guidance ? <div>{readiness.guidance}</div> : null}
    </div>
  );
}

export function ProviderReadinessSection({
  items,
  error,
}: {
  items: CertificateProviderReadiness[];
  error?: Error | null;
}) {
  return (
    <div className="stack">
      {error ? <div className="banner banner--danger">无法检查服务端就绪状态；创建请求仍由服务端校验。</div> : null}
      {items.length === 0 && !error ? <Typography.Text type="secondary">No provider readiness data yet.</Typography.Text> : null}
      {items.map((readiness) => (
        <ReadinessItem key={readiness.providerType} readiness={readiness} />
      ))}
    </div>
  );
}
