import type { CertificateFilter, ProviderCredentialInput } from '../../lib/admin-graphql';

export const ORIGIN_PROVIDER = 'cloudflare_origin_ca';
export const FILE_PROVIDER = 'file';

export const defaultFilter: CertificateFilter = { query: '', status: '' };
export const defaultCredentialForm: ProviderCredentialInput = { id: '', name: '', scope: '', token: '' };

export type DimensionFilters = {
  operation: string;
  provider: string;
  providerType: string;
};

export const defaultDimensionFilters: DimensionFilters = {
  operation: '',
  provider: '',
  providerType: '',
};
