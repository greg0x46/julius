import { useTranslation } from "../i18n/context";

export function SkipLink() {
  const { t } = useTranslation();
  return (
    <a className="skip-link" href="#conteudo-principal">
      {t("skipToContent")}
    </a>
  );
}
