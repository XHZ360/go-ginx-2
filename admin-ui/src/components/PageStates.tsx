import type { ReactNode } from 'react';

type StateCardProps = {
  title: string;
  message: string;
  action?: ReactNode;
};

function StateCard({ title, message, action }: StateCardProps) {
  return (
    <div className="state-card" role="status">
      <h2>{title}</h2>
      <p>{message}</p>
      {action ? <div className="state-card__action">{action}</div> : null}
    </div>
  );
}

export function PageLoading({ label = 'Loading page...' }: { label?: string }) {
  return (
    <div className="state-card state-card--loading" aria-busy="true">
      <div className="spinner" />
      <span>{label}</span>
    </div>
  );
}

export function EmptyState({ title, message, action }: StateCardProps) {
  return <StateCard title={title} message={message} action={action} />;
}

export function FilteredEmptyState({ onClear }: { onClear: () => void }) {
  return (
    <StateCard
      title="No matching results"
      message="The current filters returned no records."
      action={
        <button type="button" className="button button--secondary" onClick={onClear}>
          Clear filters
        </button>
      }
    />
  );
}

export function NotFoundState({ resource }: { resource: string }) {
  return <StateCard title="Not found" message={`${resource} does not exist or is no longer available.`} />;
}

export function ErrorState({
  title,
  message,
  retry,
}: {
  title?: string;
  message: string;
  retry?: () => void;
}) {
  return (
    <StateCard
      title={title ?? 'Request failed'}
      message={message}
      action={
        retry ? (
          <button type="button" className="button button--secondary" onClick={retry}>
            Retry
          </button>
        ) : undefined
      }
    />
  );
}

export function ValidationBanner({
  title = 'Validation failed',
  fields,
}: {
  title?: string;
  fields?: Record<string, string>;
}) {
  if (!fields || Object.keys(fields).length === 0) {
    return null;
  }
  return (
    <div className="banner banner--danger" role="alert">
      <strong>{title}</strong>
      <ul className="field-errors-list">
        {Object.entries(fields).map(([field, message]) => (
          <li key={field}>
            <span>{field}</span>
            <span>{message}</span>
          </li>
        ))}
      </ul>
    </div>
  );
}
