import { Grid, Modal } from 'antd';
import type { PropsWithChildren, ReactNode } from 'react';

export function Dialog({
  open,
  title,
  onClose,
  footer,
  width = 720,
  children,
}: PropsWithChildren<{
  open: boolean;
  title: string;
  onClose: () => void;
  footer?: ReactNode;
  width?: number | string;
}>) {
  const screens = Grid.useBreakpoint();
  const modalWidth = screens.md ? width : '100%';

  return (
    <Modal
      open={open}
      title={title}
      onCancel={onClose}
      footer={footer ?? null}
      width={modalWidth}
      centered
      mask={{ closable: true }}
      className="admin-modal"
      styles={screens.md ? undefined : { body: { maxHeight: '70vh', overflowY: 'auto' } }}
    >
      <div className="dialog__body">{children}</div>
    </Modal>
  );
}
