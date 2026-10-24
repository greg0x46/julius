import { useQuery } from "@tanstack/react-query";

import { getSession } from "../api/auth";
import { queryKeys } from "../api/queryKeys";

/** Shown only when the server runs as the public demo (same query as AuthGate). */
export function DemoNotice() {
  const { data } = useQuery({ queryKey: queryKeys.authSession, queryFn: ({ signal }) => getSession(signal) });
  if (!data?.demo) return null;
  return (
    <div className="demo-notice" role="note">
      <strong>Ambiente de demonstração.</strong> Os dados são fictícios e redefinidos todos os dias; acesso e integrações
      ficam desativados.
    </div>
  );
}
