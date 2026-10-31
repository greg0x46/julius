import { useTranslation } from "../../i18n/context";
import type { Payable } from "../../api/contracts";
import { payableStatusLabel, getPayableVocabulary, settledPercent } from "../../presentation/payableLabels";
import { SummaryStrip } from "../layout";
import { Money } from "../shared/Money";
import { StatusTag } from "../shared/StatusTag";
import { PayableMeter } from "./PayableMeter";

/**
 * The top of a payable's detail: what is still to be settled as the page's
 * one hero figure — alone, so it can wrap but never truncate — with the
 * settled and total amounts beneath. The record's name is the page title and
 * the record's actions sit in the title row, so none of that repeats here.
 * A status shows only once the payable is settled (a tag aligned to the
 * start, not stretched).
 */
export function PayableHeader({ payable }: { payable: Payable }) {
  const { t, language } = useTranslation();
  const vocab = getPayableVocabulary(payable.kind, language);
  const percent = settledPercent(payable.total_amount, payable.settled_amount);

  return (
    <SummaryStrip
      className="payable-header"
      label={vocab.heroLabel}
      value={<Money value={payable.remaining_amount} tone="neutral" size="hero" />}
      items={[
        { label: vocab.settledColumnLabel, value: <Money value={payable.settled_amount} tone="neutral" /> },
        { label: t("totalLabel"), value: <Money value={payable.total_amount} tone="neutral" /> },
      ]}
    >
      {payable.status === "settled" && <StatusTag tone="success">{payableStatusLabel(payable.kind, "settled", language)}</StatusTag>}
      <PayableMeter percent={percent} />
      <span>
        {vocab.settledShareLabel} {percent}%
      </span>
    </SummaryStrip>
  );
}
