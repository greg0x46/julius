import { Alert } from "../components/ui";
import { useState } from "react";
import { useNavigate, useParams, useSearchParams } from "react-router-dom";

import { isUuid, type Payable, type PayableKind } from "../api/contracts";
import { DetailPage, InvalidDetailPage } from "../components/layout";
import { PayableForm } from "../components/payables/PayableForm";
import { PayableHeader } from "../components/payables/PayableHeader";
import { PayableTimeline } from "../components/payables/PayableTimeline";
import { RecordMenu } from "../components/shared/RecordMenu";
import { useConfirm } from "../components/shared/useConfirm";
import { useFeedback } from "../components/shared/useFeedback";
import { usePayableDetail } from "../hooks/usePayableDetail";
import { usePayables } from "../hooks/usePayables";
import { errorMessage } from "../presentation/errors";
import { payableVocabulary } from "../presentation/payableLabels";

const pageTitle = "Pendência";
const backTo = "/pendencias";
const backLabel = "Voltar para pendências";

function deleteDescription(payable: Payable): string {
  if (payable.link_count === 0) return "Esta ação não pode ser desfeita.";
  const plural = payable.link_count > 1;
  return `${payable.link_count} transação${plural ? "ões" : ""} vinculada${plural ? "s" : ""} ${
    plural ? "serão desfeitas" : "será desfeita"
  }; as transações em si permanecem inalteradas.`;
}

function ValidPayableDetail({ id, requestedKind }: { id: string; requestedKind: PayableKind }) {
  const payable = usePayableDetail(id, requestedKind);
  const { kind } = payable;
  const payables = usePayables(kind);
  const navigate = useNavigate();
  const feedback = useFeedback();
  const [formOpen, setFormOpen] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [deleteError, setDeleteError] = useState<string | null>(null);
  const confirm = useConfirm();
  const vocab = payableVocabulary[kind];

  const submitEdit = async (write: { name: string; total_amount: string }) => {
    setSaveError(null);
    try {
      await payables.updatePayable({
        payableId: id,
        write: { name: write.name, total_amount: write.total_amount },
      });
      setFormOpen(false);
      feedback.success("Salvo");
    } catch (error) {
      setSaveError(errorMessage(error, "Não foi possível salvar a pendência."));
    }
  };

  const submitDelete = async () => {
    setDeleteError(null);
    try {
      await payables.deletePayable(id);
      feedback.success("Excluído");
      navigate(backTo);
    } catch (error) {
      // The Alert sits at the top of the page; the confirm that started this
      // is long gone, so a toast tells the user where they are.
      const message = errorMessage(error, "Não foi possível excluir a pendência.");
      setDeleteError(message);
      feedback.error(message);
    }
  };

  const snapshot = payable.state.snapshot;
  const actions = (
    <RecordMenu
      label="Mais ações"
      items={[
        {
          key: "edit",
          label: "Editar",
          onClick: () => {
            setSaveError(null);
            setFormOpen(true);
          },
        },
        {
          key: "delete",
          label: "Excluir",
          danger: true,
          onClick: () => {
            if (snapshot === null) return;
            confirm({ title: vocab.deleteTitle, description: deleteDescription(snapshot), onConfirm: submitDelete });
          },
        },
      ]}
    />
  );

  return (
    <DetailPage
      title={pageTitle}
      backTo={backTo}
      backLabel={backLabel}
      width="narrow"
      state={payable.state}
      retry={payable.retry}
      recordTitle={(record) => record.name}
      loadingLabel="Carregando…"
      notFoundMessage="Não encontrada"
      notFoundDescription="Não existe uma dívida ou conta a receber com este identificador."
      unavailableMessage="Não foi possível consultar esta pendência agora."
      actions={actions}
    >
      {(record) => (
        <>
          {deleteError && (
            <Alert type="error" showIcon closable onClose={() => setDeleteError(null)} message={deleteError} />
          )}

          <PayableHeader payable={record} />

          <PayableTimeline
            payableId={id}
            kind={kind}
            links={record.links}
            search={payable.search}
            onSearchChange={payable.setSearch}
            candidates={payable.eligibleTransactions}
            isSearching={payable.isSearching}
            isLinking={payable.isLinking}
            onLinkTransaction={payable.linkTransaction}
            onUnlinkTransaction={payable.unlinkTransaction}
          />

          <PayableForm
            kind={kind}
            open={formOpen}
            payable={record}
            submitting={payables.isUpdating}
            submitError={saveError}
            onSubmit={submitEdit}
            onCancel={() => setFormOpen(false)}
          />
        </>
      )}
    </DetailPage>
  );
}

export function PayableDetailPage() {
  const { id = "" } = useParams();
  const [searchParams] = useSearchParams();
  const kindParam = searchParams.get("kind");
  const requestedKind: PayableKind = kindParam === "receivable" ? "receivable" : "debt";
  return isUuid(id) ? (
    <ValidPayableDetail id={id} requestedKind={requestedKind} />
  ) : (
    <InvalidDetailPage title={pageTitle} backTo={backTo} backLabel={backLabel} invalidTitle="Endereço inválido" />
  );
}
