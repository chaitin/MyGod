import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"

import { login } from "@/data/auth-api"
import {
  appInfoQueryOptions,
  queryKeys,
  type ServerTarget,
} from "@/data/query"

export function useCachedAppInfo(server: ServerTarget) {
  return useQuery({
    ...appInfoQueryOptions(server),
    enabled: false,
  })
}

export function useLoginMutation(server: ServerTarget) {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (input: { account: string; password: string }) =>
      login(server.url, input),
    onSuccess: () =>
      queryClient.invalidateQueries({
        exact: true,
        queryKey: queryKeys.appInfo(server),
        refetchType: "none",
      }),
  })
}
