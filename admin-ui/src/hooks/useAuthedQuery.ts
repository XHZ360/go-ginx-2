import { useEffect } from 'react';
import { useQuery, type QueryKey, type UseQueryOptions } from '@tanstack/react-query';
import { isAuthError } from '../lib/contracts';
import { useSession } from '../session';

type QueryOptions<TData> = Omit<
  UseQueryOptions<TData, Error, TData, QueryKey>,
  'queryKey' | 'queryFn'
> & {
  queryKey: QueryKey;
  queryFn: () => Promise<TData>;
};

export function useAuthedQuery<TData>({ queryKey, queryFn, ...options }: QueryOptions<TData>) {
  const session = useSession();
  const query = useQuery({ queryKey, queryFn, ...options });

  useEffect(() => {
    if (query.error && isAuthError(query.error)) {
      session.markUnauthenticated();
    }
  }, [query.error, session]);

  return query;
}
