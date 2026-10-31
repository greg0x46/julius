import { useTranslation } from "../i18n/context";
import { Alert } from "../components/ui";
import { useState } from "react";
import { useNavigate, useParams, useSearchParams } from "react-router-dom";

import { isUuid, type PayableKind } from "../api/contracts";
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
import { getPayableVocabulary } from "../presentation/payableLabels";

const backTo = "/pendencias";



function ValidPayableDetail({ id, requestedKind }: { id: string; requestedKind: PayableKind }) {
  const { t, language } = useTranslation();
  const payable = usePayableDetail(id, requestedKind);
  const { kind } = payable;
  const payables = usePayables(kind);
  const navigate = useNavigate();
  const feedback = useFeedback();
  const [formOpen, setFormOpen] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [deleteError, setDeleteError] = useState<string | null>(null);
  const confirm = useConfirm();
  const vocab = getPayableVocabulary(kind, language);

  const submitEdit = async (write: { name: string; total_amount: string }) => {
    setSaveError(null);
    try {
      await payables.updatePayable({
        payableId: id,
        write: { name: write.name, total_amount: write.total_amount },
      });
      setFormOpen(false);
      feedback.success(t("saved"));
    } catch (error) {
      setSaveError(errorMessage(error, t("payableSaveError")));
    }
  };

  const submitDelete = async () => {
    setDeleteError(null);
    try {
      await payables.deletePayable(id);
      feedback.success(t("deleted"));
      navigate(backTo);
    } catch (error) {
      // The Alert sits at the top of the page; the confirm that started this
      // is long gone, so a toast tells the user where they are.
      const message = errorMessage(error, t("payableDeleteError"));
      setDeleteError(message);
      feedback.error(message);
    }
  };

  const snapshot = payable.state.snapshot;
  const actions = (
    <RecordMenu
      label={t("moreActions")}
      items={[
        {
          key: "edit",
          label: t("edit"),
          onClick: () => {
            setSaveError(null);
            setFormOpen(true);
          },
        },
        {
          key: "delete",
          label: t("delete"),
          danger: true,
          onClick: () => {
            if (snapshot === null) return;
            confirm({ title: vocab.deleteTitle, description: snapshot.link_count === 0 ? t("irreversibleAction") : t(snapshot.link_count === 1 ? "deleteLinkedOne" : "deleteLinkedMany", { count: snapshot.link_count }), onConfirm: submitDelete });
          },
        },
      ]}
    />
  );

  return (
    <DetailPage
      title={t("payable")}
      backTo={backTo}
      backLabel={t("backToPayables")}
      width="narrow"
      state={payable.state}
      retry={payable.retry}
      recordTitle={(record) => record.name}
      loadingLabel={t("loading")}
      notFoundMessage={t("notFoundFeminine")}
      notFoundDescription={t("payableNotFoundDescription")}
      unavailableMessage={t("payableUnavailable")}
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
  const { t } = useTranslation();
  const { id = "" } = useParams();
  const [searchParams] = useSearchParams();
  const kindParam = searchParams.get("kind");
  const requestedKind: PayableKind = kindParam === "receivable" ? "receivable" : "debt";
  return isUuid(id) ? (
    <ValidPayableDetail id={id} requestedKind={requestedKind} />
  ) : (
    <InvalidDetailPage title={t("payable")} backTo={backTo} backLabel={t("backToPayables")} invalidTitle={t("invalidAddress")} />
  );
}
