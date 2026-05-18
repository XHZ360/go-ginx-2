import { useRef } from 'react';
import type { MouseEvent, PropsWithChildren, ReactNode } from 'react';

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
  const pointerStartedOnBackdrop = useRef(false);

  if (!open) {
    return null;
  }
  const handleBackdropMouseDown = (event: MouseEvent<HTMLDivElement>) => {
    pointerStartedOnBackdrop.current = event.target === event.currentTarget;
  };
  const handleBackdropClick = (event: MouseEvent<HTMLDivElement>) => {
    if (event.target === event.currentTarget && pointerStartedOnBackdrop.current) {
      onClose();
    }
    pointerStartedOnBackdrop.current = false;
  };

  return (
    <div className="dialog-backdrop" role="presentation" onMouseDown={handleBackdropMouseDown} onClick={handleBackdropClick}>
      <div className="dialog" role="dialog" aria-modal="true" aria-label={title}>
        <div className="dialog__header">
          <h2>{title}</h2>
          <button type="button" className="button button--ghost" onClick={onClose}>
            Close
          </button>
        </div>
        <div className="dialog__body">{children}</div>
        {footer ? <div className="dialog__footer">{footer}</div> : null}
      </div>
    </div>
  );
}
