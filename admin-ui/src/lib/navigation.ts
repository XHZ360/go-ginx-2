const INTENDED_DESTINATION_KEY = 'goginx.admin.intendedDestination';
const DEFAULT_PATH = '/dashboard';

const safeRoutes = [
  /^\/dashboard$/,
  /^\/users$/,
  /^\/users\/[^/]+$/,
  /^\/clients$/,
  /^\/clients\/[^/]+$/,
  /^\/domains$/,
  /^\/domains\/[^/]+$/,
  /^\/proxies$/,
  /^\/proxies\/[^/]+$/,
  /^\/certificates$/,
  /^\/audit$/,
];

function storage(): Storage | null {
  if (typeof window === 'undefined') {
    return null;
  }
  return window.sessionStorage;
}

export function sanitizeDestination(destination?: string | null): string {
  if (!destination) {
    return DEFAULT_PATH;
  }
  let parsed: URL;
  try {
    parsed = new URL(destination, 'http://admin.local');
  } catch {
    return DEFAULT_PATH;
  }
  if (parsed.origin !== 'http://admin.local') {
    return DEFAULT_PATH;
  }
  if (parsed.pathname === '/' || parsed.pathname === '/login') {
    return DEFAULT_PATH;
  }
  if (!safeRoutes.some((pattern) => pattern.test(parsed.pathname))) {
    return DEFAULT_PATH;
  }
  return `${parsed.pathname}${parsed.search}${parsed.hash}`;
}

export function rememberDestination(destination: string): void {
  storage()?.setItem(INTENDED_DESTINATION_KEY, sanitizeDestination(destination));
}

export function consumeDestination(): string {
  const value = storage()?.getItem(INTENDED_DESTINATION_KEY);
  storage()?.removeItem(INTENDED_DESTINATION_KEY);
  return sanitizeDestination(value);
}

export function clearDestination(): void {
  storage()?.removeItem(INTENDED_DESTINATION_KEY);
}

export function defaultDestination(): string {
  return DEFAULT_PATH;
}
