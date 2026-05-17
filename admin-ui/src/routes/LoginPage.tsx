import { useMemo, useState } from 'react';
import { Navigate, useNavigate } from 'react-router-dom';
import { TextField } from '../components/FormField';
import { ValidationBanner } from '../components/PageStates';
import { defaultDestination, consumeDestination } from '../lib/navigation';
import { isApiError } from '../lib/contracts';
import { useSession } from '../session';

export function LoginPage() {
  const session = useSession();
  const navigate = useNavigate();
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [errorMessage, setErrorMessage] = useState<string>();
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>();
  const [submitting, setSubmitting] = useState(false);

  const checking = session.status === 'unknown' || session.status === 'checking';
  const destination = useMemo(() => consumeDestination(), []);

  if (session.status === 'authenticated') {
    return <Navigate to={destination || defaultDestination()} replace />;
  }

  return (
    <div className="login-screen">
      <div className="login-card">
        <div className="login-card__intro">
          <h1>GoGinx Admin</h1>
          <p>Sign in to manage users, clients, proxies, certificates, and audit activity.</p>
        </div>

        {errorMessage ? (
          <div className="banner banner--danger" role="alert">
            {errorMessage}
          </div>
        ) : null}
        <ValidationBanner fields={fieldErrors} />

        <form
          className="stack"
          onSubmit={async (event) => {
            event.preventDefault();
            setSubmitting(true);
            setErrorMessage(undefined);
            setFieldErrors(undefined);
            try {
              await session.login({ username, password });
              navigate(destination || defaultDestination(), { replace: true });
            } catch (error) {
              if (isApiError(error)) {
                setFieldErrors(error.fields);
                setErrorMessage(
                  error.code === 'UNAUTHENTICATED'
                    ? 'Invalid administrator credentials.'
                    : error.code === 'NETWORK'
                      ? 'Network error, please retry.'
                      : error.message,
                );
              } else {
                setErrorMessage('Login failed.');
              }
            } finally {
              setSubmitting(false);
            }
          }}
        >
          <TextField
            label="Username"
            name="username"
            autoComplete="username"
            value={username}
            error={fieldErrors?.username}
            disabled={checking || submitting}
            onChange={(event) => setUsername(event.target.value)}
          />
          <TextField
            label="Password"
            name="password"
            type="password"
            autoComplete="current-password"
            value={password}
            error={fieldErrors?.password}
            disabled={checking || submitting}
            onChange={(event) => setPassword(event.target.value)}
          />
          <button type="submit" className="button" disabled={checking || submitting}>
            {submitting ? 'Signing in...' : 'Sign in'}
          </button>
        </form>
      </div>
    </div>
  );
}
