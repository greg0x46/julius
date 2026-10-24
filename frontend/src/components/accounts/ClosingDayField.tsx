import { useEffect, useState } from "react";
import { Button, InputNumber, Tooltip, Typography } from "../ui";

import type { Account } from "../../api/contracts";
import { closingDayLabel, closingDaySourceHint } from "../../presentation/accountLabels";
import { FormDrawer } from "../forms/FormDrawer";
import { FormField } from "../forms/FormField";
import { useFeedback } from "../shared/useFeedback";

/** Shows a card's closing day and lets the user state it. The manual value
 *  wins over anything the provider reports, and clearing it hands control
 *  back to the provider. Renders only the value and its action — the label
 *  ("Fechamento") belongs to the summary strip that hosts it. The editor is the
 *  shared form shell (a full-screen sheet on a phone), not a centered dialog. */
export function ClosingDayField({
  account,
  onSave,
  saving,
}: {
  account: Account;
  onSave: (closingDay: number | null) => Promise<unknown>;
  saving: boolean;
}) {
  const feedback = useFeedback();
  const [open, setOpen] = useState(false);
  const [draft, setDraft] = useState<number | null>(account.closing_day);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (open) {
      setDraft(account.closing_day);
      setError(null);
    }
  }, [open, account.closing_day]);

  const submit = async (value: number | null) => {
    if (value !== null && (!Number.isInteger(value) || value < 1 || value > 31)) {
      setError("Informe um dia entre 1 e 31.");
      return;
    }
    try {
      await onSave(value);
      setOpen(false);
      feedback.success(value === null ? "Voltou ao dado da instituição" : "Dia de fechamento salvo");
    } catch {
      setError("Não foi possível salvar o dia de fechamento.");
    }
  };

  return (
    <>
      <span className="closing-day-field">
        <Tooltip title={closingDaySourceHint(account.closing_day_source)}>
          <span>{closingDayLabel(account.closing_day)}</span>
        </Tooltip>
        <Button size="small" type="link" className="closing-day-field-action" onClick={() => setOpen(true)}>
          {account.closing_day === null ? "Definir" : "Alterar"}
        </Button>
      </span>

      <FormDrawer
        open={open}
        title="Dia de fechamento"
        onClose={() => setOpen(false)}
        onSubmit={() => void submit(draft)}
        submitting={saving}
        // An empty field would submit null, which is what the "usar o dado da
        // instituição" button below already does — and that button only shows
        // when there is something to clear. Saving nothing is never the intent.
        submitDisabled={draft === null}
        error={error}
      >
        <Typography.Paragraph type="secondary">
          Nem toda instituição informa o fechamento do cartão. Defina o dia aqui para que o
          Julius use o seu valor.
        </Typography.Paragraph>
        <FormField label="Dia do mês" htmlFor="closing-day-input">
          <InputNumber
            id="closing-day-input"
            min={1}
            max={31}
            precision={0}
            inputMode="numeric"
            style={{ width: "100%" }}
            value={draft}
            onChange={setDraft}
          />
        </FormField>
        {account.closing_day_source === "manual" && (
          <Button danger type="text" className="closing-day-clear" loading={saving} onClick={() => void submit(null)}>
            Usar o dado da instituição
          </Button>
        )}
      </FormDrawer>
    </>
  );
}
