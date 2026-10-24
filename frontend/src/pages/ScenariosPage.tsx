import { Alert, Button, Select } from "../components/ui";
import { useState } from "react";

import type { Scenario, ScenarioKind, ScenarioTransactionWrite } from "../api/contracts";
import { scenarioKindLabel } from "../presentation/scenarioLabels";
import { StandaloneScenarioList } from "../components/scenarios/StandaloneScenarioCard";
import { StandaloneScenarioForm } from "../components/scenarios/StandaloneScenarioForm";
import { ListToolbar, PageAction, SearchField } from "../components/layout";
import { SettingsPageContainer as PageContainer } from "../components/SettingsPageContainer";
import { useFeedback } from "../components/shared/useFeedback";
import { useScenarios } from "../hooks/useScenarios";
import { errorMessage } from "../presentation/errors";

/** Search happens here: the list is small and already fully loaded (the kind
 *  filter, unlike search, changes what the server returns). */
function arrange(scenarios: Scenario[], search: string): Scenario[] {
  const needle = search.trim().toLocaleLowerCase("pt-BR");
  if (!needle) return scenarios;
  return scenarios.filter((scenario) => scenario.name.toLocaleLowerCase("pt-BR").includes(needle));
}

export function ScenariosPage() {
  const [kindFilter, setKindFilter] = useState<ScenarioKind | "all">("all");
  const feedback = useFeedback();
  const scenarios = useScenarios({ kind: kindFilter === "all" ? undefined : kindFilter });
  const [search, setSearch] = useState("");
  const [formOpen, setFormOpen] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);

  const submit = async (name: string) => {
    setSaveError(null);
    try {
      await scenarios.createScenario({ name });
      setFormOpen(false);
      feedback.success("Cenário criado");
    } catch (error) {
      setSaveError(errorMessage(error, "Não foi possível salvar o cenário."));
    }
  };

  /** Where a failure raised from the table shows: the page, plus a toast in case it is scrolled out of view. */
  const reportActionError = (message: string) => {
    setActionError(message);
    feedback.error(message);
  };

  // These two resolve with the error message (null on success) rather than
  // reporting it themselves: from the phone sheet the failure belongs inside
  // the sheet, from the table on the page.
  const removeScenario = async (scenario: Scenario): Promise<string | null> => {
    setActionError(null);
    try {
      await scenarios.deleteScenario(scenario.id);
      feedback.success("Cenário excluído");
      return null;
    } catch (error) {
      return errorMessage(error, "Não foi possível excluir o cenário.");
    }
  };

  const toggleScenario = async (scenario: Scenario, isActive: boolean): Promise<string | null> => {
    setActionError(null);
    try {
      await scenarios.toggleScenario({ scenarioId: scenario.id, isActive });
      feedback.success(isActive ? "Cenário incluído na projeção" : "Cenário fora da projeção");
      return null;
    } catch (error) {
      return errorMessage(error, "Não foi possível atualizar o cenário.");
    }
  };

  const addTransaction = (scenarioId: string, write: ScenarioTransactionWrite) =>
    scenarios.createTransaction({ scenarioId, write });

  const deleteTransaction = (scenarioId: string, transactionId: string) =>
    scenarios.deleteTransaction({ scenarioId, transactionId });

  const isFiltered = search.trim() !== "" || kindFilter !== "all";
  const showToolbar = !scenarios.error && (isFiltered || scenarios.isLoading || scenarios.scenarios.length > 0);

  return (
    <PageContainer
      title="Cenários"
      subTitle="Simule decisões hipotéticas, como uma viagem ou uma troca de emprego"
      compactMobileHeader
      extra={[
        <PageAction key="new-scenario" label="Novo cenário" shortLabel="Novo" onClick={() => setFormOpen(true)} />,
      ]}
    >
      {actionError && (
        <Alert
          type="error"
          showIcon
          closable
          onClose={() => setActionError(null)}
          message={actionError}
          style={{ marginBottom: 16 }}
        />
      )}
      {scenarios.error && (
        <Alert
          type="error"
          showIcon
          message="Não foi possível carregar os cenários"
          description={<Button onClick={() => scenarios.refetch()}>Tentar novamente</Button>}
          style={{ marginBottom: 16 }}
        />
      )}

      {/* Nothing to search or filter while there are no scenarios at all (or the list failed to load). */}
      {showToolbar && (
        <ListToolbar
          label="Controles dos cenários"
          start={
            <SearchField
              id="scenarios-search"
              label="Buscar cenários"
              placeholder="Buscar por nome…"
              value={search}
              onChange={setSearch}
            />
          }
          end={
            <label className="scenario-kind-filter">
              <span className="scenario-kind-filter-label">Tipo</span>
              <Select
                aria-label="Filtrar cenários por tipo"
                value={kindFilter}
                onChange={setKindFilter}
                options={[
                  { value: "all", label: "Todos os tipos" },
                  ...Object.entries(scenarioKindLabel).map(([value, label]) => ({ value, label })),
                ]}
                popupMatchSelectWidth={false}
              />
            </label>
          }
        />
      )}

      {/* A failed load shows only the retry: an empty list beside it would say "nothing yet". */}
      {!scenarios.error && (
        <StandaloneScenarioList
          scenarios={arrange(scenarios.scenarios, search)}
          isLoading={scenarios.isLoading}
          isFiltered={isFiltered}
          onDeleteScenario={removeScenario}
          onAddTransaction={addTransaction}
          onDeleteTransaction={deleteTransaction}
          onToggleScenario={toggleScenario}
          onActionError={reportActionError}
        />
      )}
      <StandaloneScenarioForm
        open={formOpen}
        submitting={scenarios.isCreating}
        submitError={saveError}
        onSubmit={submit}
        onCancel={() => setFormOpen(false)}
      />
    </PageContainer>
  );
}
