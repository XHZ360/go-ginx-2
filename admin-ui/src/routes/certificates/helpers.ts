import type { CertificateMutationInput } from '../../lib/admin-graphql';
import type { ManagedCertificate } from '../../lib/contracts';

export function certificateMutationInput(certificate: ManagedCertificate): CertificateMutationInput {
  return {
    proxyId: certificate.boundProxyId || certificate.proxyId || undefined,
    certificateId: certificate.certificateId || undefined,
  };
}

export function formatFingerprint(value?: string | null) {
  if (!value) {
    return 'None';
  }
  return value.length > 16 ? `${value.slice(0, 16)}...` : value;
}

export function certificateProviderType(certificate: ManagedCertificate) {
  return certificate.providerType || 'acme_dns01';
}
