import { MCPSettings } from "../components/MCPSettings";
import { AuthenticationSettings } from "../components/AuthenticationSettings";
import { SettingsPageContainer } from "../components/SettingsPageContainer";

export function SecuritySettingsPage() {
  return (
    <SettingsPageContainer title="Segurança" subTitle="Gerencie sua senha e o acesso à sua conta" compactMobileHeader>
      <AuthenticationSettings />
      <MCPSettings />
    </SettingsPageContainer>
  );
}
