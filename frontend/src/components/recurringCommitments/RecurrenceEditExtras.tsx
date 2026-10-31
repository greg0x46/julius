import { useTranslation } from "../../i18n/context";
import { Button } from "../ui";

import type { RecurringCommitment } from "../../api/contracts";
import { useConfirm } from "../shared/useConfirm";
import { RecurrenceOccurrenceList } from "./RecurrenceOccurrenceList";

/**
 * What the table's row offers on a wide screen, for a phone's edit drawer
 * where a stacked row has no room for it: the occurrences (and with them
 * Conciliar) and the delete. Only an active recurrence has occurrences, same
 * as the table's expandable row.
 */
export function RecurrenceEditExtras({
  commitment,
  onDelete,
}: {
  commitment: RecurringCommitment;
  onDelete: (commitment: RecurringCommitment) => void;
}) {
  const { t } = useTranslation();
  const confirm = useConfirm();

  return (
    <>
      {commitment.is_active && (
        <section className="recurrence-extra" aria-labelledby="recurrence-extra-occurrences">
          <h3 id="recurrence-extra-occurrences" className="recurrence-extra-title">
            {t("occurrencesTitle")}
          </h3>
          <RecurrenceOccurrenceList commitment={commitment} />
        </section>
      )}
      <Button danger type="text" className="recurrence-extra-delete" onClick={() =>
          confirm({
            title: t("deleteRecurrence"),
            description: t("irreversibleAction"),
            onConfirm: () => onDelete(commitment),
          })
        }>
        {t("deleteRecurrence")}
      </Button>
    </>
  );
}
