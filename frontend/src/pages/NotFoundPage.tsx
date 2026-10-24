import { Button } from "../components/ui";
import { useNavigate } from "react-router-dom";

import { EmptyState, Page } from "../components/layout";


export function NotFoundPage() {
  const navigate = useNavigate();
  return (
    <Page title="Página não encontrada" width="narrow">
      <EmptyState
        title="O endereço acessado não corresponde a uma página do Julius."
        action={<Button onClick={() => navigate("/")}>Voltar para o Início</Button>}
      />
    </Page>
  );
}
