import { useTranslation } from "../i18n/context";
import { Alert, Button, Input } from "./ui";
import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "../api/transport";
import { FormField } from "./forms/FormField";
import { invalidateDataSources } from "../api/queryKeys";

export function PluggySettings() {
  const { t } = useTranslation();
  const client = useQueryClient();
  const [id, setID] = useState("");
  const [secret, setSecret] = useState("");
  const [item, setItem] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [saved, setSaved] = useState(false);
  const save = async () => {
    setBusy(true);
    setError(null);
    setSaved(false);
    try {
      const response = await apiFetch("/api/settings/pluggy", {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ pluggy_client_id: id, pluggy_client_secret: secret, pluggy_item_id: item }),
      });
      if (!response.ok) {
        // Surface the server's own sentence (e.g. the demo-mode 403).
        const problem = (await response.json().catch(() => null)) as { detail?: string } | null;
        throw new Error(problem?.detail || t("credentialsSaveError"));
      }
      setID("");
      setSecret("");
      setItem("");
      setSaved(true);
      void invalidateDataSources(client);
    } catch (err) {
      setError(err instanceof Error ? err.message : t("saveFailedGeneric"));
    } finally {
      setBusy(false);
    }
  };
  return (
    <form
      className="settings-form"
      onSubmit={(event) => {
        event.preventDefault();
        void save();
      }}
    >
      <p className="settings-radio-hint">
        {t("credentialsHint")}
      </p>
      {error && <Alert type="error" message={error} />}
      {saved && <Alert type="success" message={t("credentialsSaved")} />}
      <FormField label="Client ID" htmlFor="pluggy-client-id">
        <Input id="pluggy-client-id" required value={id} onChange={(e) => setID(e.target.value)} />
      </FormField>
      <FormField label="Client secret" htmlFor="pluggy-client-secret">
        <Input.Password
          id="pluggy-client-secret"
          required
          autoComplete="off"
          value={secret}
          onChange={(e) => setSecret(e.target.value)}
        />
      </FormField>
      <FormField label={t("newConnectionItemId")} htmlFor="pluggy-item">
        <Input id="pluggy-item" value={item} onChange={(e) => setItem(e.target.value)} />
      </FormField>
      <Button htmlType="submit" loading={busy}>
        {t("saveCredentials")}
      </Button>
    </form>
  );
}
