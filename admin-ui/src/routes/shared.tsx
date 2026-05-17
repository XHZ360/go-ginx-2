import type { ReactNode } from 'react';
import { Link } from 'react-router-dom';
import { formatDateTime, formatTitle } from '../lib/format';

export function PageHeader({
  title,
  description,
  actions,
}: {
  title: string;
  description?: string;
  actions?: ReactNode;
}) {
  return (
    <div className="page-header">
      <div>
        <h1>{title}</h1>
        {description ? <p className="muted">{description}</p> : null}
      </div>
      {actions ? <div className="toolbar-actions">{actions}</div> : null}
    </div>
  );
}

export function Pagination({
  page,
  totalPages,
  onPageChange,
}: {
  page: number;
  totalPages: number;
  onPageChange: (page: number) => void;
}) {
  return (
    <div className="pagination">
      <button type="button" className="button button--secondary" disabled={page <= 1} onClick={() => onPageChange(page - 1)}>
        Previous
      </button>
      <span>
        Page {page} / {totalPages}
      </span>
      <button type="button" className="button button--secondary" disabled={page >= totalPages} onClick={() => onPageChange(page + 1)}>
        Next
      </button>
    </div>
  );
}

export function StatusBadge({ value }: { value?: string | null }) {
  return <span className={`badge badge--${(value ?? 'unknown').toLowerCase()}`}>{formatTitle(value ?? 'unknown')}</span>;
}

export function DetailBackLink({ to, label }: { to: string; label: string }) {
  return (
    <Link className="back-link" to={to}>
      {label}
    </Link>
  );
}

export function Timestamp({ value }: { value?: string | null }) {
  return <span>{formatDateTime(value)}</span>;
}
