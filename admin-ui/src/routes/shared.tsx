import { Button, Space, Table, Tag, Typography } from 'antd';
import type { TablePaginationConfig, TableProps } from 'antd';
import { LeftOutlined, RightOutlined } from '@ant-design/icons';
import type { ReactNode } from 'react';
import { Link } from 'react-router-dom';
import type { PageInfo } from '../lib/contracts';
import { formatDateTime, formatTitle } from '../lib/format';

const statusColors: Record<string, string> = {
  enabled: 'success',
  online: 'success',
  valid: 'success',
  usable: 'success',
  idle: 'success',
  active: 'success',
  verified: 'success',
  success: 'success',
  disabled: 'error',
  revoked: 'error',
  offline: 'error',
  invalid: 'error',
  expired: 'error',
  issue_failed: 'error',
  renewal_failed: 'error',
  failed: 'error',
  danger: 'error',
  pending: 'warning',
  verification_failed: 'warning',
  missing: 'warning',
  missing_remote: 'warning',
  expiring_soon: 'warning',
  needs_config: 'warning',
  unknown: 'default',
};

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
        <Typography.Title level={1}>{title}</Typography.Title>
        {description ? <Typography.Text type="secondary">{description}</Typography.Text> : null}
      </div>
      {actions ? <Space className="toolbar-actions" wrap>{actions}</Space> : null}
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
      <Button type="default" icon={<LeftOutlined aria-hidden="true" />} disabled={page <= 1} onClick={() => onPageChange(page - 1)}>
        Previous
      </Button>
      <span>
        Page {page} / {totalPages}
      </span>
      <Button type="default" disabled={page >= totalPages} onClick={() => onPageChange(page + 1)}>
        Next
        <RightOutlined aria-hidden="true" />
      </Button>
    </div>
  );
}

export function pageTablePagination(
  pageInfo: PageInfo,
  onPageChange: (page: number) => void,
  options?: { itemLabel?: string },
): TablePaginationConfig {
  return {
    current: pageInfo.page,
    pageSize: pageInfo.pageSize,
    total: pageInfo.totalCount,
    showSizeChanger: false,
    showTotal: options?.itemLabel ? (total) => `${total} ${options.itemLabel}` : undefined,
    onChange: (nextPage) => onPageChange(nextPage),
  };
}

const defaultTableBodyScrollY = 'max(200px, calc(100dvh - 280px))';

export function DataTable<T extends object>(props: TableProps<T> & { compact?: boolean }) {
  const { className, size = 'middle', scroll, pagination, compact = false, ...rest } = props;
  const nextScroll = compact
    ? {
        x: scroll?.x ?? 'max-content',
        ...(scroll?.y !== undefined ? { y: scroll.y } : {}),
      }
    : {
        ...(scroll ?? {}),
        y: scroll?.y ?? defaultTableBodyScrollY,
      };

  return (
    <div className={['data-table-host', compact ? 'data-table-host--compact' : ''].filter(Boolean).join(' ')}>
      <Table<T>
        className={['data-table', compact ? 'data-table--compact' : '', className].filter(Boolean).join(' ')}
        size={size}
        scroll={nextScroll}
        tableLayout={compact ? 'auto' : undefined}
        pagination={
          pagination === false
            ? false
            : {
                ...(typeof pagination === 'object' ? pagination : {}),
                placement: ['bottomEnd'],
              }
        }
        {...rest}
      />
    </div>
  );
}

export function StatusBadge({ value }: { value?: string | null }) {
  const status = (value ?? 'unknown').toLowerCase();
  return <Tag className="status-tag" color={statusColors[status] ?? 'processing'}>{formatTitle(value ?? 'unknown')}</Tag>;
}

export function DetailBackLink({ to, label }: { to: string; label: string }) {
  return (
    <Link className="back-link" to={to}>
      <LeftOutlined aria-hidden="true" />
      {label}
    </Link>
  );
}

export function Timestamp({ value }: { value?: string | null }) {
  return <span>{formatDateTime(value)}</span>;
}
