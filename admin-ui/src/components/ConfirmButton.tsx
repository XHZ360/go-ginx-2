import { useState } from 'react';

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
  const [confirming, setConfirming] = useState(false);
  const className = tone === 'danger' ? 'button button--danger' : 'button button--secondary';

  if (!confirming) {
    return (
      <button type="button" className={className} disabled={disabled} onClick={() => setConfirming(true)}>
        {label}
      </button>
    );
  }

  return (
    <div className="confirm-inline">
      <span>{confirmLabel}</span>
      <button type="button" className={className} disabled={disabled} onClick={onConfirm}>
        Confirm
      </button>
      <button type="button" className="button button--ghost" onClick={() => setConfirming(false)}>
        Cancel
      </button>
    </div>
  );
}
