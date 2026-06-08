import { Alert, Button, Card, Spin } from 'antd';
import { ClearOutlined, ReloadOutlined } from '@ant-design/icons';
import type { ReactNode } from 'react';

type StateCardProps = {
  title: string;
  message: string;
  action?: ReactNode;
};

function StateCard({ title, message, action }: StateCardProps) {
  return (
    <Card className="state-card" role="status">
      <h2>{title}</h2>
      <p>{message}</p>
      {action ? <div className="state-card__action">{action}</div> : null}
    </Card>
  );
}

export function PageLoading({ label = 'Loading page...' }: { label?: string }) {
  return (
    <Card className="state-card state-card--loading" aria-busy="true">
      <Spin size="small" />
      <span>{label}</span>
    </Card>
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
        <Button type="default" icon={<ClearOutlined aria-hidden="true" />} onClick={onClear}>
          Clear filters
        </Button>
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
          <Button type="default" icon={<ReloadOutlined aria-hidden="true" />} onClick={retry}>
            Retry
          </Button>
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
    <Alert
      className="banner"
      type="error"
      showIcon
      role="alert"
      title={title}
      description={
        <ul className="field-errors-list">
          {Object.entries(fields).map(([field, message]) => (
            <li key={field}>
              <span>{field}</span>
              <span>{message}</span>
            </li>
          ))}
        </ul>
      }
    />
  );
}
