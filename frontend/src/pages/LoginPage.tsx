import { Alert, Button, Input } from "../components/ui";
import { useState } from "react";

import { login } from "../api/auth";
import julius from "../assets/julius.png";
import { FormField } from "../components/forms/FormField";

/**
 * The signed-out screen: the brand lockup (logo, name, tagline — the same
 * trio as the app bar) over one column of fields on the page background.
 * No card around it; the form is the whole screen. Fields are 16px so iOS
 * does not zoom on focus, and the height is `100dvh` so the browser's
 * collapsing toolbar does not push the button below the fold.
 */
export function LoginPage({ onDone }: { onDone: () => void }) {
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const submit = async () => {
    setBusy(true);
    setError(null);
    try {
      await login(email, password);
      setPassword("");
      onDone();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Não foi possível entrar.");
    } finally {
      setBusy(false);
    }
  };
  return (
    <main className="login-page">
      <div className="login-panel">
        <div className="login-brand">
          <img src={julius} alt="" className="login-brand-logo" />
          <h1 className="login-brand-name">Julius</h1>
          <p className="login-brand-tagline">De graça !?</p>
        </div>
        <form
          className="login-form"
          onSubmit={(event) => {
            event.preventDefault();
            void submit();
          }}
        >
          {error && <Alert type="error" showIcon message={error} />}
          <FormField label="E-mail" htmlFor="login-email">
            <Input
              id="login-email"
              size="large"
              type="email"
              autoComplete="username"
              required
              value={email}
              onChange={(e) => setEmail(e.target.value)}
            />
          </FormField>
          <FormField label="Senha" htmlFor="login-password">
            <Input.Password
              id="login-password"
              size="large"
              autoComplete="current-password"
              required
              value={password}
              onChange={(e) => setPassword(e.target.value)}
            />
          </FormField>
          <Button type="primary" htmlType="submit" size="large" block loading={busy}>
            Entrar
          </Button>
        </form>
      </div>
    </main>
  );
}
