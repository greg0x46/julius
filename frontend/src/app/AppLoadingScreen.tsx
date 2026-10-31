import { useTranslation } from "../i18n/context";
import juliusLoading from "../assets/julius-loading.gif";

export function AppLoadingScreen() {
  const { t } = useTranslation();
  return (
    <div className="app-loading-screen" role="status">
      <img src={juliusLoading} alt="" className="app-loading-screen-gif" />
      <p className="app-loading-screen-caption">{t("loadingCaption")}</p>
    </div>
  );
}
