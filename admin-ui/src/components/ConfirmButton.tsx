import { Button, Popconfirm } from 'antd';
import { DeleteOutlined, ExclamationCircleOutlined } from '@ant-design/icons';

export function ConfirmButton({
  label,
  confirmLabel,
  onConfirm,
  disabled,
  tone = 'danger',
}: {
  label: string;
  confirmLabel: string;
  onConfirm: () => void;
  disabled?: boolean;
  tone?: 'danger' | 'secondary';
}) {
  return (
    <Popconfirm
      title={confirmLabel}
      okText="Confirm"
      cancelText="Cancel"
      onConfirm={onConfirm}
      disabled={disabled}
      icon={<ExclamationCircleOutlined aria-hidden="true" />}
    >
      <Button
        type={tone === 'danger' ? 'primary' : 'default'}
        danger={tone === 'danger'}
        disabled={disabled}
        icon={tone === 'danger' ? <DeleteOutlined aria-hidden="true" /> : undefined}
      >
        {label}
      </Button>
    </Popconfirm>
  );
}
