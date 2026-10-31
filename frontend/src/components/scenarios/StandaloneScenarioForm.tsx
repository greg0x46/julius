import { useTranslation } from "../../i18n/context";
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
  const { t } = useTranslation();
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
      setError(t("scenarioNameRequired"));
      return;
    }
    setError(null);
    onSubmit(name.trim());
  };

  return (
    <FormDrawer
      title={t("newScenario")}
      open={open}
      onClose={onCancel}
      onSubmit={submit}
      submitting={submitting}
      error={error ?? submitError}
    >
      <FormField
        label={t("name")}
        htmlFor="standalone-scenario-name"
        hint={t("scenarioNameHint")}
      >
        <Input
          id="standalone-scenario-name"
          value={name}
          onChange={(event) => setName(event.target.value)}
          placeholder={t("scenarioNameExample")}
        />
      </FormField>
    </FormDrawer>
  );
}
