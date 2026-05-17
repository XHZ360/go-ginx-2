import type { InputHTMLAttributes, ReactNode, SelectHTMLAttributes, TextareaHTMLAttributes } from 'react';

type FieldWrapperProps = {
  label: string;
  error?: string;
  hint?: string;
  children: ReactNode;
};

function FieldWrapper({ label, error, hint, children }: FieldWrapperProps) {
  return (
    <label className="field">
      <span className="field__label">{label}</span>
      {children}
      {hint ? <span className="field__hint">{hint}</span> : null}
      {error ? <span className="field__error">{error}</span> : null}
    </label>
  );
}

export function TextField({
  label,
  error,
  hint,
  ...props
}: InputHTMLAttributes<HTMLInputElement> & { label: string; error?: string; hint?: string }) {
  return (
    <FieldWrapper label={label} error={error} hint={hint}>
      <input className="input" {...props} />
    </FieldWrapper>
  );
}

export function SelectField({
  label,
  error,
  hint,
  children,
  ...props
}: SelectHTMLAttributes<HTMLSelectElement> & { label: string; error?: string; hint?: string; children: ReactNode }) {
  return (
    <FieldWrapper label={label} error={error} hint={hint}>
      <select className="input" {...props}>
        {children}
      </select>
    </FieldWrapper>
  );
}

export function TextAreaField({
  label,
  error,
  hint,
  ...props
}: TextareaHTMLAttributes<HTMLTextAreaElement> & { label: string; error?: string; hint?: string }) {
  return (
    <FieldWrapper label={label} error={error} hint={hint}>
      <textarea className="input textarea" {...props} />
    </FieldWrapper>
  );
}
