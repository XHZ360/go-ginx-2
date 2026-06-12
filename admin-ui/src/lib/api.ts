import { ApiError, type ContractErrorCode, type SessionBootstrap } from './contracts';

const JSON_HEADERS = { 'Content-Type': 'application/json' };

type JSONErrorEnvelope = {
  error?: {
    code?: string;
    message?: string;
    fields?: Record<string, string>;
  };
};

type GraphQLErrorEnvelope = {
  message?: string;
  extensions?: {
    code?: string;
    fields?: Record<string, string>;
  };
};

type GraphQLResponse<TData> = {
  data?: TData;
  errors?: GraphQLErrorEnvelope[];
};

function normalizeCode(code?: string): ContractErrorCode {
  switch (code) {
    case 'UNAUTHENTICATED':
    case 'FORBIDDEN':
    case 'VALIDATION_FAILED':
    case 'NOT_FOUND':
    case 'CONFLICT':
    case 'ENTRY_CONFLICT':
    case 'CONFIRMATION_REQUIRED':
    case 'CERTIFICATE_INCOMPATIBLE':
    case 'UNSUPPORTED':
    case 'INTERNAL':
    case 'INVALID_CSRF':
      return code;
    default:
      return 'UNKNOWN';
  }
}

async function readJson(response: Response): Promise<unknown> {
  const text = await response.text();
  if (!text) {
    return null;
  }
  try {
    return JSON.parse(text) as unknown;
  } catch {
    return null;
  }
}

async function fetchJSON<T>(input: string, init?: RequestInit): Promise<T> {
  let response: Response;
  try {
    response = await fetch(input, {
      credentials: 'include',
      ...init,
    });
  } catch (error) {
    throw new ApiError('NETWORK', error instanceof Error ? error.message : 'network request failed');
  }
  const body = (await readJson(response)) as JSONErrorEnvelope | null;
  if (!response.ok) {
    const code = normalizeCode(body?.error?.code);
    throw new ApiError(code, body?.error?.message ?? response.statusText, {
      status: response.status,
      fields: body?.error?.fields,
    });
  }
  return body as T;
}

export const sessionClient = {
  bootstrap(): Promise<SessionBootstrap> {
    return fetchJSON<SessionBootstrap>('/api/admin/session', {
      method: 'GET',
    });
  },
  login(input: { username: string; password: string }): Promise<SessionBootstrap> {
    return fetchJSON<SessionBootstrap>('/api/admin/login', {
      method: 'POST',
      headers: JSON_HEADERS,
      body: JSON.stringify(input),
    });
  },
  logout(csrfToken?: string): Promise<SessionBootstrap> {
    return fetchJSON<SessionBootstrap>('/api/admin/logout', {
      method: 'POST',
      headers: {
        ...JSON_HEADERS,
        ...(csrfToken ? { 'X-GoGinx-CSRF-Token': csrfToken } : {}),
      },
      body: JSON.stringify({}),
    });
  },
};

export const graphqlClient = {
  async request<TData>(options: {
    query: string;
    variables?: unknown;
    operationName?: string;
    mutation?: boolean;
    csrfToken?: string;
  }): Promise<TData> {
    let response: Response;
    try {
      response = await fetch('/api/admin/graphql', {
        method: 'POST',
        credentials: 'include',
        headers: {
          ...JSON_HEADERS,
          ...(options.mutation && options.csrfToken
            ? { 'X-GoGinx-CSRF-Token': options.csrfToken }
            : {}),
        },
        body: JSON.stringify({
          query: options.query,
          variables: options.variables,
          operationName: options.operationName,
        }),
      });
    } catch (error) {
      throw new ApiError('NETWORK', error instanceof Error ? error.message : 'network request failed');
    }

    const body = (await readJson(response)) as GraphQLResponse<TData> | JSONErrorEnvelope | null;

    if (!response.ok) {
      const envelope = body as JSONErrorEnvelope | null;
      throw new ApiError(normalizeCode(envelope?.error?.code), envelope?.error?.message ?? response.statusText, {
        status: response.status,
        fields: envelope?.error?.fields,
      });
    }

    const result = body as GraphQLResponse<TData> | null;
    const firstError = result?.errors?.[0];
    if (firstError) {
      throw new ApiError(normalizeCode(firstError.extensions?.code), firstError.message ?? 'request failed', {
        status: response.status,
        fields: firstError.extensions?.fields,
      });
    }

    if (!result?.data) {
      throw new ApiError('UNKNOWN', 'missing graphql response payload', { status: response.status });
    }

    return result.data;
  },
};
