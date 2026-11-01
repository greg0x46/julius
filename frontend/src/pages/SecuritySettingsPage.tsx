import { useTranslation } from "../i18n/context";
import { MCPSettings } from "../components/MCPSettings";
import { AuthenticationSettings } from "../components/AuthenticationSettings";
import { SettingsPageContainer } from "../components/SettingsPageContainer";

export function SecuritySettingsPage() {
  const { t } = useTranslation();
  return (
    <SettingsPageContainer title={t("security")} subTitle={t("securitySubtitle")} compactMobileHeader>
      <AuthenticationSettings />
      <MCPSettings />
    </SettingsPageContainer>
  );
}
