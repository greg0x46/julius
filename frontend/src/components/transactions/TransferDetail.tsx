import { useTranslation } from "../../i18n/context";
import { translate, type Language } from "../../i18n/messages";
import { Descriptions, Typography } from "../ui";
import type { AccountTransfer, TransferMovement } from "../../api/transfers";
import { formatMoney } from "../../presentation/money";
import { PanelSection } from "../shared/PanelStack";

function movementLabel(movement: TransferMovement, language: Language) {
  const amount = movement.amount === null ? translate(language, "transferAmountUnavailable") : formatMoney(movement.amount.replace(/^-/, ""), movement.currency_code);
  return `${movement.account_name || translate(language, "account")} · ${amount}`;
}
export function TransferDetail({ transfer }: { transfer: AccountTransfer }) {
  const { t, language } = useTranslation();
  return (
    <PanelSection title={t("transferCategory")}>
      <Descriptions column={1} size="small" colon={false}>
        <Descriptions.Item label={t("origin")}>{movementLabel(transfer.source, language)}</Descriptions.Item>
        <Descriptions.Item label={t("destination")}>{movementLabel(transfer.destination, language)}</Descriptions.Item>
        <Descriptions.Item label={t("effectiveRate")}>
          {transfer.effective_rate
            ? `1 ${transfer.destination.currency_code} = ${language === "pt-BR" ? transfer.effective_rate.replace(".", ",") : transfer.effective_rate} ${transfer.source.currency_code}`
            : t("rateUnavailable")}
        </Descriptions.Item>
        <Descriptions.Item label={t("recordedFee")}>{transfer.fee ? movementLabel(transfer.fee, language) : t("noneFeminine")}</Descriptions.Item>
      </Descriptions>
      <Typography.Paragraph type="secondary">
        {t("transferDetailHint")}
      </Typography.Paragraph>
    </PanelSection>
  );
}
