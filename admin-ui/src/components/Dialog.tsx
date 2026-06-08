import { Modal } from 'antd';
import type { PropsWithChildren, ReactNode } from 'react';

export function Dialog({
  open,
  title,
  onClose,
  footer,
  children,
}: PropsWithChildren<{
  open: boolean;
  title: string;
  onClose: () => void;
  footer?: ReactNode;
}>) {
  return (
    <Modal
      open={open}
      title={title}
      onCancel={onClose}
      footer={footer ?? null}
      width={720}
      centered
      mask={{ closable: true }}
      className="admin-modal"
    >
      <div className="dialog__body">{children}</div>
    </Modal>
  );
}
