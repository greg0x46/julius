import { Input } from "../ui";
import { useEffect, useState } from "react";

import { FormDrawer } from "../forms/FormDrawer";
import { FormField } from "../forms/FormField";

export function StandaloneScenarioForm({
  open,
  submitting,
  submitError,
  onSubmit,
  onCancel,
}: {
  open: boolean;
  submitting: boolean;
  submitError: string | null;
  onSubmit: (name: string) => void;
  onCancel: () => void;
}) {
  const [name, setName] = useState("");
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (open) {
      setName("");
      setError(null);
    }
  }, [open]);

  const submit = () => {
    if (name.trim() === "") {
      setError("Informe um nome para o cenário.");
      return;
    }
    setError(null);
    onSubmit(name.trim());
  };

  return (
    <FormDrawer
      title="Novo cenário"
      open={open}
      onClose={onCancel}
      onSubmit={submit}
      submitting={submitting}
      error={error ?? submitError}
    >
      <FormField
        label="Nome"
        htmlFor="standalone-scenario-name"
        hint="Depois de criar, adicione as transações hipotéticas do cenário."
      >
        <Input
          id="standalone-scenario-name"
          value={name}
          onChange={(event) => setName(event.target.value)}
          placeholder="Ex.: Viagem, Novo emprego"
        />
      </FormField>
    </FormDrawer>
  );
}
