import { Input } from 'antd';
import type { ComponentProps, ReactNode, SelectHTMLAttributes } from 'react';

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

type TextFieldProps = ComponentProps<typeof Input> & { label: string; error?: string; hint?: string };
type TextAreaFieldProps = ComponentProps<typeof Input.TextArea> & { label: string; error?: string; hint?: string };

export function TextField({ label, error, hint, ...props }: TextFieldProps) {
  return (
    <FieldWrapper label={label} error={error} hint={hint}>
      <Input className="input" status={error ? 'error' : undefined} {...props} />
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
      <select className="input native-select" aria-invalid={Boolean(error) || undefined} {...props}>
        {children}
      </select>
    </FieldWrapper>
  );
}

export function TextAreaField({ label, error, hint, ...props }: TextAreaFieldProps) {
  return (
    <FieldWrapper label={label} error={error} hint={hint}>
      <Input.TextArea className="input textarea" status={error ? 'error' : undefined} {...props} />
    </FieldWrapper>
  );
}
