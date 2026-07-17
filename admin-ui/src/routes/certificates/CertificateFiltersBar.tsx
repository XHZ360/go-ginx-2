import { SelectField, TextField } from '../../components/FormField';
import type { CertificateFilter } from '../../lib/admin-graphql';
import type { ProviderCredential } from '../../lib/contracts';
import type { DimensionFilters } from './constants';
import { FILE_PROVIDER, ORIGIN_PROVIDER } from './constants';

export function CertificateFiltersBar({
  filter,
  dimensionFilters,
  credentials,
  selectedCredentialId,
  onFilterChange,
  onDimensionChange,
  onCredentialChange,
}: {
  filter: CertificateFilter;
  dimensionFilters: DimensionFilters;
  credentials: ProviderCredential[];
  selectedCredentialId: string;
  onFilterChange: (next: CertificateFilter) => void;
  onDimensionChange: (next: DimensionFilters) => void;
  onCredentialChange: (value: string) => void;
}) {
  return (
    <div className="toolbar-grid toolbar-grid--wide">
      <TextField
        label="Search"
        value={filter.query ?? ''}
        onChange={(event) => onFilterChange({ ...filter, query: event.target.value })}
        placeholder="host, certificate id..."
      />
      <SelectField label="Status" value={filter.status ?? ''} onChange={(event) => onFilterChange({ ...filter, status: event.target.value })}>
        <option value="">All</option>
        <option value="usable">Usable</option>
        <option value="expiring_soon">Expiring soon</option>
        <option value="expired">Expired</option>
        <option value="missing">Missing</option>
        <option value="invalid">Invalid</option>
      </SelectField>
      <SelectField
        label="Operation status"
        value={dimensionFilters.operation}
        onChange={(event) => onDimensionChange({ ...dimensionFilters, operation: event.target.value })}
      >
        <option value="">All</option>
        <option value="idle">Idle</option>
        <option value="pending">Pending</option>
        <option value="issue_failed">Issue failed</option>
        <option value="renewal_failed">Renewal failed</option>
      </SelectField>
      <SelectField
        label="Provider status"
        value={dimensionFilters.provider}
        onChange={(event) => onDimensionChange({ ...dimensionFilters, provider: event.target.value })}
      >
        <option value="">All</option>
        <option value="active">Active</option>
        <option value="revoked">Revoked</option>
        <option value="missing_remote">Remote missing</option>
        <option value="unknown">Unknown</option>
      </SelectField>
      <SelectField
        label="Provider type"
        value={dimensionFilters.providerType}
        onChange={(event) => onDimensionChange({ ...dimensionFilters, providerType: event.target.value })}
      >
        <option value="">All</option>
        <option value="acme_dns01">ACME DNS-01</option>
        <option value={ORIGIN_PROVIDER}>Cloudflare Origin CA</option>
        <option value={FILE_PROVIDER}>File-backed</option>
      </SelectField>
      <SelectField label="Origin credential" value={selectedCredentialId} onChange={(event) => onCredentialChange(event.target.value)}>
        <option value="">Default</option>
        {credentials.map((credential) => (
          <option key={credential.id} value={credential.id} disabled={credential.status === 'disabled'}>
            {credential.name}
          </option>
        ))}
      </SelectField>
    </div>
  );
}
