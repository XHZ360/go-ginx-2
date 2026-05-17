import { useEffect } from 'react';
import { useMutation, type UseMutationOptions } from '@tanstack/react-query';
import { isAuthError } from '../lib/contracts';
import { useSession } from '../session';

export function useMutationWithAuth<TData, TVariables>(
  options: UseMutationOptions<TData, Error, TVariables>,
) {
  const session = useSession();
  const mutation = useMutation(options);

  useEffect(() => {
    if (mutation.error && isAuthError(mutation.error)) {
      session.markUnauthenticated();
    }
  }, [mutation.error, session]);

  return mutation;
}
