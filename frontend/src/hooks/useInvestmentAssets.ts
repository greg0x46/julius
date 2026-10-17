import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import {
  createInvestmentAsset,
  deleteInvestmentAsset,
  listInvestmentAssets,
  listInvestmentAssetClassification,
  updateInvestmentAsset,
} from "../api/investmentPortfolio";
import type { InvestmentAssetWrite, InvestmentAssetClassDefinition } from "../api/contracts";
import { invalidateAfterInvestmentAssetChange, queryKeys } from "../api/queryKeys";

export const investmentAssetsQueryKey = queryKeys.investmentAssets;
const emptyClassification: InvestmentAssetClassDefinition[] = [];

export function useInvestmentAssets({ enabled = true }: { enabled?: boolean } = {}) {
  const queryClient = useQueryClient();
  const assetsQuery = useQuery({
    queryKey: investmentAssetsQueryKey,
    queryFn: ({ signal }) => listInvestmentAssets(signal),
    enabled,
  });
  const classificationQuery = useQuery({
    queryKey: queryKeys.investmentAssetClassification,
    queryFn: ({ signal }) => listInvestmentAssetClassification(signal),
    staleTime: Infinity,
    enabled,
  });

  const refresh = async () => {
    await invalidateAfterInvestmentAssetChange(queryClient);
  };

  const createMutation = useMutation({
    mutationFn: (write: InvestmentAssetWrite) => createInvestmentAsset(write),
    onSuccess: refresh,
  });
  const updateMutation = useMutation({
    mutationFn: ({ assetId, write }: { assetId: string; write: InvestmentAssetWrite }) =>
      updateInvestmentAsset(assetId, write),
    onSuccess: refresh,
  });
  const deleteMutation = useMutation({
    mutationFn: (assetId: string) => deleteInvestmentAsset(assetId),
    onSuccess: refresh,
  });

  return {
    assets: assetsQuery.data ?? [],
    classification: classificationQuery.data ?? emptyClassification,
    isLoading: assetsQuery.isLoading || classificationQuery.isLoading,
    error: assetsQuery.error ?? classificationQuery.error,
    refetch: () => Promise.all([assetsQuery.refetch(), classificationQuery.refetch()]),
    createAsset: createMutation.mutateAsync,
    updateAsset: updateMutation.mutateAsync,
    deleteAsset: deleteMutation.mutateAsync,
    isSaving: createMutation.isPending || updateMutation.isPending,
    isDeleting: deleteMutation.isPending,
  };
}
