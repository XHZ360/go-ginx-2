import {
  createContext,
  useContext,
  useEffect,
  useMemo,
  useState,
  type PropsWithChildren,
} from 'react';
import { sessionClient } from './lib/api';
import { ApiError, type SessionBootstrap } from './lib/contracts';
import { clearDestination } from './lib/navigation';

export type SessionStatus = 'unknown' | 'checking' | 'authenticated' | 'unauthenticated';

type SessionState = {
  status: SessionStatus;
  username?: string;
  csrfToken?: string;
  expiresAt?: string;
  pollIntervalSeconds: number;
};

type SessionContextValue = SessionState & {
  bootstrap: () => Promise<SessionBootstrap>;
  login: (input: { username: string; password: string }) => Promise<SessionBootstrap>;
  logout: () => Promise<void>;
  markUnauthenticated: () => void;
};

const SessionContext = createContext<SessionContextValue | null>(null);

const DEFAULT_POLL_INTERVAL = 5;

function fromBootstrap(bootstrap: SessionBootstrap): SessionState {
  if (!bootstrap.authenticated) {
    return {
      status: 'unauthenticated',
      pollIntervalSeconds: DEFAULT_POLL_INTERVAL,
    };
  }
  return {
    status: 'authenticated',
    username: bootstrap.username,
    csrfToken: bootstrap.csrfToken,
    expiresAt: bootstrap.expiresAt,
    pollIntervalSeconds: bootstrap.pollIntervalSeconds ?? DEFAULT_POLL_INTERVAL,
  };
}

export function SessionProvider({ children }: PropsWithChildren) {
  const [state, setState] = useState<SessionState>({
    status: 'unknown',
    pollIntervalSeconds: DEFAULT_POLL_INTERVAL,
  });

  const bootstrap = async () => {
    setState((current) => ({ ...current, status: 'checking' }));
    const result = await sessionClient.bootstrap();
    setState(fromBootstrap(result));
    return result;
  };

  const login = async (input: { username: string; password: string }) => {
    const result = await sessionClient.login(input);
    setState(fromBootstrap(result));
    return result;
  };

  const logout = async () => {
    try {
      await sessionClient.logout(state.csrfToken);
    } finally {
      clearDestination();
      setState({ status: 'unauthenticated', pollIntervalSeconds: DEFAULT_POLL_INTERVAL });
    }
  };

  const markUnauthenticated = () => {
    clearDestination();
    setState({ status: 'unauthenticated', pollIntervalSeconds: DEFAULT_POLL_INTERVAL });
  };

  useEffect(() => {
    if (state.status !== 'unknown') {
      return;
    }
    bootstrap().catch((error: unknown) => {
      if (error instanceof ApiError) {
        setState({ status: 'unauthenticated', pollIntervalSeconds: DEFAULT_POLL_INTERVAL });
        return;
      }
      setState({ status: 'unauthenticated', pollIntervalSeconds: DEFAULT_POLL_INTERVAL });
    });
  }, [state.status]);

  useEffect(() => {
    if (state.status !== 'authenticated' || !state.expiresAt) {
      return;
    }
    const expiresAt = Date.parse(state.expiresAt);
    if (Number.isNaN(expiresAt)) {
      return;
    }
    const timer = window.setTimeout(() => {
      markUnauthenticated();
    }, Math.max(0, expiresAt - Date.now()));
    return () => window.clearTimeout(timer);
  }, [state.status, state.expiresAt]);

  const value = useMemo<SessionContextValue>(
    () => ({
      ...state,
      bootstrap,
      login,
      logout,
      markUnauthenticated,
    }),
    [state],
  );

  return <SessionContext.Provider value={value}>{children}</SessionContext.Provider>;
}

export function useSession(): SessionContextValue {
  const value = useContext(SessionContext);
  if (!value) {
    throw new Error('useSession must be used within SessionProvider');
  }
  return value;
}
