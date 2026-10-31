import { useTranslation } from "../i18n/context";
import { translate, type Language } from "../i18n/messages";
import { DownloadOutlined, InboxOutlined } from "../components/ui/icons";
import { Alert, Button, Checkbox, Input, Radio, Select, Skeleton, Space, Typography } from "../components/ui";
import { useEffect, useRef, useState } from "react";
import { Link } from "react-router-dom";

import { LoadingState, UnavailableState } from "../components/AsyncState";
import { BottomActionBar, EmptyState, Page, Section } from "../components/layout";
import { useCompactScreen } from "../components/shared/useCompactScreen";
import { useFeedback } from "../components/shared/useFeedback";
import { StatementImportHistory } from "../components/statementImport/StatementImportHistory";
import { StatementPreviewRows } from "../components/statementImport/StatementPreviewRows";
import { StatementPreviewSummary } from "../components/statementImport/StatementPreviewSummary";
import { useStatementImports } from "../hooks/useStatementImports";

const accountsPath = "/contas-e-cartoes";

function message(error: unknown, language: Language): string {
  return error instanceof Error ? error.message : translate(language, "operationFailedGeneric");
}

export function StatementImportPage() {
  const { t, language } = useTranslation();
  const compact = useCompactScreen();
  const feedback = useFeedback();
  const { accounts, history, preview: previewing, confirm: confirming, template, discard } = useStatementImports();
  const [file, setFile] = useState<File | null>(null);
  const [targetKind, setTargetKind] = useState<"new" | "existing">("new");
  const [accountId, setAccountId] = useState<string>();
  const [newAccountName, setNewAccountName] = useState("");
  const [allowPartial, setAllowPartial] = useState(false);
  const [ambiguousDecisions, setAmbiguousDecisions] = useState<Record<number, "import" | "ignore">>({});
  const resultRef = useRef<HTMLDivElement>(null);

  const preview = previewing.data;
  const result = confirming.result;
  const busy = previewing.isPending || confirming.isPending;
  const failure = confirming.error ?? previewing.error;

  // The confirm action sits at the bottom on a phone, so its outcome has to
  // come to the user instead of appearing out of sight above the list.
  useEffect(() => {
    if (result) resultRef.current?.scrollIntoView?.({ block: "start" });
  }, [result]);

  // The confirm button sits in the bottom bar, far from where a failure is
  // written (above the preview): say it where the user is looking too.
  useEffect(() => {
    if (confirming.error) feedback.error(message(confirming.error, language));
  }, [confirming.error, feedback, language]);

  function loadPreview(selectedFile: File, selectedAccountId?: string) {
    setAllowPartial(false);
    setAmbiguousDecisions({});
    previewing.request(selectedFile, selectedAccountId);
  }

  function changeFile(selectedFile: File | null) {
    setFile(selectedFile);
    discard();
    if (selectedFile && (targetKind === "new" || accountId)) loadPreview(selectedFile, targetKind === "existing" ? accountId : undefined);
  }

  function changeTarget(kind: "new" | "existing", id?: string) {
    setTargetKind(kind);
    setAccountId(id);
    discard();
    if (file && (kind === "new" || id)) loadPreview(file, kind === "existing" ? id : undefined);
  }

  function confirm() {
    if (!file || !preview) return;
    confirming.submit({
      file, preview, allowPartial, ambiguousDecisions,
      target: targetKind === "existing" ? { accountId } : { newAccountName: newAccountName.trim() },
    });
  }

  const targetReady = targetKind === "new" ? newAccountName.trim().length > 0 : Boolean(accountId);
  const ambiguousRows = preview?.rows.filter((row) => row.status === "ambiguous") ?? [];
  const selectedNew = (preview?.counts.new ?? 0) + ambiguousRows.filter((row) => ambiguousDecisions[row.line_number] === "import").length;
  const canConfirm = file !== null && preview !== null && targetReady && !busy && !result &&
    ambiguousRows.every((row) => Boolean(ambiguousDecisions[row.line_number])) &&
    (preview.counts.invalid === 0 || allowPartial) && !(targetKind === "new" && selectedNew === 0);

  // Only a preview still waiting for its confirmation has an action to offer.
  const confirmAction = preview && !result ? (
    <Button type="primary" loading={busy} disabled={!canConfirm} onClick={confirm}>
      {selectedNew === 0 ? t("registerResendNoNew") : t("importNTransactions", { count: selectedNew, unit: selectedNew === 1 ? t("transactionSingularLower") : t("transactionPluralLower") })}
    </Button>
  ) : undefined;

  return (
    <Page
      title={t("importStatement")}
      backTo={accountsPath}
      backLabel={t("backToAccounts")}
      description={t("statementImportDescription")}
      actions={confirmAction}
      className="statement-import-page"
      width="narrow"
      compactMobileHeader
      hasBottomActionBar={confirmAction !== undefined}
    >
      <div className="statement-import-sections">
        {result && <div ref={resultRef} className="statement-import-result">
          <Alert type="success" showIcon message={result.counts.new > 0 ? t("statementImported") : t("statementCheckedNoNew")}
            description={<Space direction="vertical"><span>{t("importCountsSummary", { new: result.counts.new, duplicate: result.counts.duplicate, invalid: result.counts.invalid })}</span>
              <Link to={`${accountsPath}/${result.account_id}`}>{t("viewAccount")}</Link></Space>} />
        </div>}

        <Section title={t("selectFileSection")}>
          <div className="statement-import-panel">
            <label className="statement-file-picker">
              <InboxOutlined aria-hidden="true" />
              <span>{file ? file.name : t("selectCsvFile")}</span>
              <input type="file" accept=".csv,text/csv" aria-label={t("csvFileLabel")} onChange={(event) => changeFile(event.target.files?.[0] ?? null)} />
            </label>
            <Typography.Text type="secondary">{t("periodReadFromMovements")}</Typography.Text>
            <div className="statement-template">
              <Typography.Text type="secondary">{t("templateHint")}</Typography.Text>
              <Button type="link" className="statement-template-action" icon={<DownloadOutlined />} loading={template.isPending} onClick={template.download}>
                {t("downloadTemplate")}
              </Button>
            </div>
            {template.error && <Alert type="error" showIcon message={message(template.error, language)} />}
          </div>
        </Section>

        <Section title={t("chooseAccountSection")}>
          <div className="statement-import-panel">
            <Typography.Paragraph>{t("fileAccountHint")}</Typography.Paragraph>
            <Radio.Group value={targetKind} onChange={(event) => changeTarget(event.target.value as "new" | "existing")}>
              <Radio value="new">{t("createFileAccount")}</Radio>
              <Radio value="existing" disabled={accounts.state.kind !== "ready"}>{t("useExistingAccount")}</Radio>
            </Radio.Group>
            {accounts.state.kind === "unavailable" &&
              <UnavailableState onRetry={accounts.retry}>{t("fileAccountsListError")}</UnavailableState>}
            <div className="statement-account-field">
              {targetKind === "new" ? <Input value={newAccountName} maxLength={100} placeholder={t("newAccountNamePlaceholder")}
                aria-label={t("newAccountNameLabel")} onChange={(event) => setNewAccountName(event.target.value)} /> :
                <Select value={accountId} placeholder={t("selectFileAccount")} aria-label={t("fileAccountLabel")} className="statement-account-select"
                  options={accounts.state.kind === "ready" ? accounts.state.items.map((account) => ({ value: account.id, label: `${account.name} (${account.currency_code})` })) : []}
                  onChange={(value) => changeTarget("existing", value)} />}
            </div>
          </div>
        </Section>

        {busy && <LoadingState>{t("processingStatement")}</LoadingState>}
        {failure && <Alert type="error" showIcon message={message(failure, language)} action={file && !preview && !result ?
          <Button size="small" onClick={() => loadPreview(file, targetKind === "existing" ? accountId : undefined)}>{t("generatePreviewAgain")}</Button> : undefined} />}

        {preview && (
          <Section title={t("reviewPreview")} flush>
            <StatementPreviewSummary preview={preview} />
            <div className="statement-import-review-notes">
              {!targetReady && <Typography.Text type="secondary">{t("informAccountToConfirm")}</Typography.Text>}
              {targetKind === "new" && preview.counts.new === 0 && <Alert type="info" showIcon message={t("noValidTransactionsForAccount")} />}
              {preview.counts.new === 0 && preview.counts.duplicate > 0 && ambiguousRows.length === 0 &&
                <Alert type="info" showIcon message={t("statementAlreadyImported")} />}
              {ambiguousRows.length > 0 && <Alert type="warning" showIcon message={t("differentBalancesWarning")} />}
              {preview.warnings.map((warning, index) => <Alert key={index} type="warning" showIcon message={warning} />)}
              {preview.counts.invalid > 0 && <Alert type="warning" showIcon
                message={t("invalidRowsWarning", { count: preview.counts.invalid })} />}
              {preview.counts.invalid > 0 && <Checkbox checked={allowPartial} onChange={(event) => setAllowPartial(event.target.checked)}>
                {t("importOnlyValidRows")}
              </Checkbox>}
            </div>
            <StatementPreviewRows rows={preview.rows} conflicts={preview.conflicts} filename={preview.filename} ambiguousDecisions={ambiguousDecisions}
              onAmbiguousDecision={(line, decision) => setAmbiguousDecisions((current) => ({ ...current, [line]: decision }))} />
          </Section>
        )}

        <Section title={t("recentImports")} flush={history.state.kind === "ready"}>
          {history.state.kind === "loading" && (
            <div role="status" aria-label={t("loadingHistoryShort")}>
              <Skeleton active paragraph={{ rows: 3 }} />
            </div>
          )}
          {history.state.kind === "empty" && <EmptyState title={t("noStatementsImported")} />}
          {history.state.kind === "unavailable" &&
            <UnavailableState onRetry={history.retry}>{t("historyLoadError")}</UnavailableState>}
          {history.state.kind === "ready" && <StatementImportHistory items={history.state.items} />}
        </Section>
      </div>
      {compact && confirmAction && <BottomActionBar>{confirmAction}</BottomActionBar>}
    </Page>
  );
}
