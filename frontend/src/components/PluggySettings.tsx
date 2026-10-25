import { Alert, Button, Input } from "./ui";
import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "../api/transport";
import { FormField } from "./forms/FormField";
import { invalidateDataSources } from "../api/queryKeys";

export function PluggySettings() {
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
      if (!response.ok) throw new Error("Não foi possível salvar as credenciais.");
      setID("");
      setSecret("");
      setItem("");
      setSaved(true);
      void invalidateDataSources(client);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Não foi possível salvar.");
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
        Preencha para configurar ou substituir as credenciais. Os valores salvos não são exibidos.
      </p>
      {error && <Alert type="error" message={error} />}
      {saved && <Alert type="success" message="Credenciais salvas." />}
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
      <FormField label="Item ID de uma nova conexão (opcional)" htmlFor="pluggy-item">
        <Input id="pluggy-item" value={item} onChange={(e) => setItem(e.target.value)} />
      </FormField>
      <Button htmlType="submit" loading={busy}>
        Salvar credenciais
      </Button>
    </form>
  );
}
