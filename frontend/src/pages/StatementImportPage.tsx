import { DownloadOutlined, InboxOutlined } from "@ant-design/icons";
import { Alert, Button, Checkbox, Input, Radio, Select, Skeleton, Space, Typography } from "antd";
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

function message(error: unknown): string {
  return error instanceof Error ? error.message : "Não foi possível concluir a operação.";
}

export function StatementImportPage() {
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
    if (confirming.error) feedback.error(message(confirming.error));
  }, [confirming.error, feedback]);

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
      {selectedNew === 0 ? "Registrar reenvio sem novas transações" : `Importar ${selectedNew} ${selectedNew === 1 ? "transação" : "transações"}`}
    </Button>
  ) : undefined;

  return (
    <Page
      title="Importar extrato"
      backTo={accountsPath}
      backLabel="Voltar para contas e cartões"
      description="Revise as movimentações de um arquivo antes de adicioná-las à sua conta."
      actions={confirmAction}
      className="statement-import-page"
      width="narrow"
      compactMobileHeader
      hasBottomActionBar={confirmAction !== undefined}
    >
      <div className="statement-import-sections">
        {result && <div ref={resultRef} className="statement-import-result">
          <Alert type="success" showIcon message={result.counts.new > 0 ? "Extrato importado" : "Extrato conferido: nenhuma transação nova"}
            description={<Space direction="vertical"><span>{result.counts.new} novos, {result.counts.duplicate} já importados e {result.counts.invalid} inválidos.</span>
              <Link to={`${accountsPath}/${result.account_id}`}>Ver conta</Link></Space>} />
        </div>}

        <Section title="Selecione o arquivo">
          <div className="statement-import-panel">
            <label className="statement-file-picker">
              <InboxOutlined aria-hidden="true" />
              <span>{file ? file.name : "Selecionar arquivo CSV"}</span>
              <input type="file" accept=".csv,text/csv" aria-label="Arquivo CSV" onChange={(event) => changeFile(event.target.files?.[0] ?? null)} />
            </label>
            <Typography.Text type="secondary">O período é lido das movimentações, mesmo que o nome do arquivo indique outro mês.</Typography.Text>
            <div className="statement-template">
              <Typography.Text type="secondary">Não tem o arquivo? Baixe o modelo no formato Flash, substitua as linhas de exemplo e envie.</Typography.Text>
              <Button type="link" className="statement-template-action" icon={<DownloadOutlined />} loading={template.isPending} onClick={template.download}>
                Baixar modelo (CSV)
              </Button>
            </div>
            {template.error && <Alert type="error" showIcon message={message(template.error)} />}
          </div>
        </Section>

        <Section title="Escolha a conta">
          <div className="statement-import-panel">
            <Typography.Paragraph>Use uma conta de arquivo. Extratos enviados aqui ficam separados de conexões automáticas.</Typography.Paragraph>
            <Radio.Group value={targetKind} onChange={(event) => changeTarget(event.target.value)}>
              <Radio value="new">Criar conta de arquivo</Radio>
              <Radio value="existing" disabled={accounts.state.kind !== "ready"}>Usar conta existente</Radio>
            </Radio.Group>
            {accounts.state.kind === "unavailable" &&
              <UnavailableState onRetry={accounts.retry}>Não foi possível listar contas de arquivo</UnavailableState>}
            <div className="statement-account-field">
              {targetKind === "new" ? <Input value={newAccountName} maxLength={100} placeholder="Nome da conta, por exemplo Flash"
                aria-label="Nome da nova conta" onChange={(event) => setNewAccountName(event.target.value)} /> :
                <Select value={accountId} placeholder="Selecione uma conta de arquivo" aria-label="Conta de arquivo" className="statement-account-select"
                  options={accounts.state.kind === "ready" ? accounts.state.items.map((account) => ({ value: account.id, label: `${account.name} (${account.currency_code})` })) : []}
                  onChange={(value) => changeTarget("existing", value)} />}
            </div>
          </div>
        </Section>

        {busy && <LoadingState>Processando extrato…</LoadingState>}
        {failure && <Alert type="error" showIcon message={message(failure)} action={file && !preview && !result ?
          <Button size="small" onClick={() => loadPreview(file, targetKind === "existing" ? accountId : undefined)}>Gerar prévia novamente</Button> : undefined} />}

        {preview && (
          <Section title="Revise a prévia" flush>
            <StatementPreviewSummary preview={preview} />
            <div className="statement-import-review-notes">
              {!targetReady && <Typography.Text type="secondary">Informe a conta para confirmar.</Typography.Text>}
              {targetKind === "new" && preview.counts.new === 0 && <Alert type="info" showIcon message="Não há transações válidas para criar esta conta." />}
              {preview.counts.new === 0 && preview.counts.duplicate > 0 && ambiguousRows.length === 0 &&
                <Alert type="info" showIcon message="Este extrato já foi importado nesta conta. Nenhuma transação nova será criada." />}
              {ambiguousRows.length > 0 && <Alert type="warning" showIcon message="Revise as movimentações iguais às de outro arquivo e escolha importar ou ignorar cada uma." />}
              {preview.warnings.map((warning, index) => <Alert key={index} type="warning" showIcon message={warning} />)}
              {preview.counts.invalid > 0 && <Alert type="warning" showIcon
                message={`${preview.counts.invalid} linha(s) inválida(s). Corrija o arquivo ou escolha importar apenas as válidas.`} />}
              {preview.counts.invalid > 0 && <Checkbox checked={allowPartial} onChange={(event) => setAllowPartial(event.target.checked)}>
                Importar somente as linhas válidas; as inválidas serão ignoradas
              </Checkbox>}
            </div>
            <StatementPreviewRows rows={preview.rows} ambiguousDecisions={ambiguousDecisions}
              onAmbiguousDecision={(line, decision) => setAmbiguousDecisions((current) => ({ ...current, [line]: decision }))} />
          </Section>
        )}

        <Section title="Importações recentes" flush={history.state.kind === "ready"}>
          {history.state.kind === "loading" && (
            <div role="status" aria-label="Carregando histórico">
              <Skeleton active paragraph={{ rows: 3 }} />
            </div>
          )}
          {history.state.kind === "empty" && <EmptyState title="Nenhum extrato importado ainda" />}
          {history.state.kind === "unavailable" &&
            <UnavailableState onRetry={history.retry}>Não foi possível carregar o histórico</UnavailableState>}
          {history.state.kind === "ready" && <StatementImportHistory items={history.state.items} />}
        </Section>
      </div>
      {compact && confirmAction && <BottomActionBar>{confirmAction}</BottomActionBar>}
    </Page>
  );
}
