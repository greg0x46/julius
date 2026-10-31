import { useTranslation } from "../i18n/context";
import { useState } from "react";

import { InvestmentAssetSettings } from "../components/InvestmentAssetSettings";
import { PageAction } from "../components/layout";
import { QuotesSettings } from "../components/QuotesSettings";
import { SettingsPageContainer } from "../components/SettingsPageContainer";
import { useInvestmentAssets } from "../hooks/useInvestmentAssets";

export function InvestmentAssetsSettingsPage() {
  const { t } = useTranslation();
  const [creating, setCreating] = useState(false);
  // A new asset is picked from the class catalog, so the action waits for it.
  const assets = useInvestmentAssets();
  return (
    <SettingsPageContainer
      title={t("investmentAssets")}
      subTitle={t("assetSettingsSubtitle")}
      compactMobileHeader
      extra={
        <PageAction
          label={t("newAsset")}
          shortLabel={t("new")}
          disabled={assets.isLoading || assets.classification.length === 0}
          onClick={() => setCreating(true)}
        />
      }
    >
      <InvestmentAssetSettings creating={creating} onCreatingClose={() => setCreating(false)} />
      <QuotesSettings />
    </SettingsPageContainer>
  );
}
