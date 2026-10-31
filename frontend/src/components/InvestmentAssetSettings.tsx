import { useTranslation } from "../i18n/context";
import { Alert, Button, Skeleton, Table } from "./ui";
import type { ColumnsType } from "./ui/Table";
import { useEffect, useState } from "react";

import type { InvestmentAsset, InvestmentAssetClass, InvestmentAssetClassDefinition } from "../api/contracts";
import { useInvestmentAssets } from "../hooks/useInvestmentAssets";
import { errorMessage } from "../presentation/errors";
import { FormDrawer } from "./forms/FormDrawer";
import {
  assetWriteFrom,
  catalogQuoteMarket,
  customAssetType,
  emptyAssetDraft,
  type AssetDraft,
} from "./investments/assetDraft";
import { InvestmentAssetFields } from "./investments/InvestmentAssetFields";
import { RecordMenu } from "./shared/RecordMenu";
import { useConfirm } from "./shared/useConfirm";
import { DataCard, EmptyState, ResponsiveList } from "./layout";
import { useFeedback } from "./shared/useFeedback";

function draftFrom(asset: InvestmentAsset | null, classification: InvestmentAssetClassDefinition[]): AssetDraft {
  if (!asset) return emptyAssetDraft;
  const known = classification
    .find((item) => item.asset_class === asset.asset_class)
    ?.types.some((item) => item.name === asset.asset_type);
  return {
    name: asset.name,
    ticker: asset.ticker ?? "",
    assetClass: asset.asset_class,
    assetType: known ? asset.asset_type : customAssetType,
    customType: known ? "" : asset.asset_type,
    currencyCode: asset.currency_code,
    quoteEnabled: asset.quote_source !== null,
    quoteSymbol: asset.quote_symbol ?? "",
  };
}

/**
 * The asset registry: a list (rows on a phone, a flat table from `md` up)
 * and the form drawer. The "Novo ativo" action lives in the page's title row,
 * so it reaches this component as `creating` / `onCreatingClose`; editing
 * stays local because it starts from a row.
 */
export function InvestmentAssetSettings({
  creating,
  onCreatingClose,
}: {
  creating: boolean;
  onCreatingClose: () => void;
}) {
  const { t, language } = useTranslation();
  const assets = useInvestmentAssets();
  const feedback = useFeedback();
  const confirm = useConfirm();
  const [editing, setEditing] = useState<InvestmentAsset | null>(null);
  const [draft, setDraft] = useState<AssetDraft>(emptyAssetDraft);
  const [formError, setFormError] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [deletingId, setDeletingId] = useState<string | null>(null);
  const open = creating || editing !== null;
  // The form needs the class catalog; until it loads there is nothing to pick.
  const catalogReady = assets.classification.length > 0;

  useEffect(() => {
    if (open) {
      setDraft(draftFrom(editing, assets.classification));
      setFormError(null);
    }
  }, [open, editing, assets.classification]);

  const close = () => {
    setEditing(null);
    onCreatingClose();
  };

  const legacyMarket =
    editing &&
    draft.assetClass === editing.asset_class &&
    draft.assetType === customAssetType &&
    draft.customType === editing.asset_type
      ? editing.quote_source
      : null;
  const quoteMarket = catalogQuoteMarket(draft, assets.classification) ?? legacyMarket;
  const quoteSource = draft.quoteEnabled ? quoteMarket : null;
  const classOptions = assets.classification.map((item) => ({ value: item.asset_class, label: item.label }));
  const classLabel = (value: InvestmentAssetClass) =>
    assets.classification.find((item) => item.asset_class === value)?.label ?? value;
  // Grouped by class (in the catalog's order), then by name.
  const sortedAssets = [...assets.assets].sort((left, right) => {
    const order = (value: InvestmentAssetClass) =>
      assets.classification.findIndex((item) => item.asset_class === value);
    return order(left.asset_class) - order(right.asset_class) || left.name.localeCompare(right.name, language);
  });

  const submit = async () => {
    // O código para cotação vem pré-preenchido com o que o sistema derivou do
    // código antigo. Se o código mudou e o campo não foi tocado, ele deixa de
    // valer: enviar vazio faz o sistema derivar de novo do código novo.
    const tickerChanged =
      editing !== null && (editing.ticker ?? "").trim().toLowerCase() !== draft.ticker.trim().toLowerCase();
    const symbolUntouched = editing !== null && draft.quoteSymbol.trim() === (editing.quote_symbol ?? "");
    const quoteSymbol = !quoteSource || (tickerChanged && symbolUntouched) ? "" : draft.quoteSymbol.trim();
    const write = assetWriteFrom(draft, quoteSource, quoteSymbol, t);
    if (typeof write === "string") {
      setFormError(write);
      return;
    }
    setFormError(null);
    try {
      if (editing) {
        await assets.updateAsset({ assetId: editing.id, write });
      } else {
        await assets.createAsset(write);
      }
      close();
      feedback.success(t("saved"));
    } catch (error) {
      setFormError(errorMessage(error, t("assetSaveError")));
    }
  };

  const remove = async (asset: InvestmentAsset) => {
    setActionError(null);
    setDeletingId(asset.id);
    try {
      await assets.deleteAsset(asset.id);
      feedback.success(t("deleted"));
    } catch (error) {
      const message = errorMessage(error, t("assetDeleteError"));
      setActionError(message);
      feedback.error(message);
    } finally {
      setDeletingId(null);
    }
  };

  const menu = (asset: InvestmentAsset) => (
    <RecordMenu
      label={t("actionsFor", { name: asset.name })}
      items={[
        { key: "edit", label: t("edit"), disabled: !catalogReady, onClick: () => setEditing(asset) },
        {
          key: "delete",
          label: t("delete"),
          danger: true,
          disabled: deletingId !== null,
          onClick: () =>
            confirm({
              title: t("deleteAsset"),
              description: t("deleteAssetHint"),
              onConfirm: () => void remove(asset),
            }),
        },
      ]}
    />
  );

  const columns: ColumnsType<InvestmentAsset> = [
    {
      title: t("name"),
      dataIndex: "name",
      sorter: (left, right) => left.name.localeCompare(right.name, language),
    },
    {
      title: t("codeLabel"),
      dataIndex: "ticker",
      render: (_, asset) => asset.ticker ?? <span className="investment-quiet">—</span>,
    },
    {
      title: t("classLabel"),
      dataIndex: "asset_class",
      render: (_, asset) => classLabel(asset.asset_class),
      filters: classOptions.map((item) => ({ text: item.label, value: item.value })),
      onFilter: (value, asset) => asset.asset_class === value,
    },
    { title: t("type"), dataIndex: "asset_type" },
    { title: t("currency"), dataIndex: "currency_code", width: 100 },
    {
      title: <span className="investment-visually-hidden">{t("actions")}</span>,
      key: "actions",
      width: 48,
      render: (_, asset) => <span onClick={(event) => event.stopPropagation()}>{menu(asset)}</span>,
    },
  ];

  return (
    <>
      {actionError && (
        <Alert
          type="error"
          showIcon
          closable
          message={actionError}
          onClose={() => setActionError(null)}
          style={{ marginBottom: 16 }}
        />
      )}
      {assets.error && (
        <Alert
          type="error"
          showIcon
          message={t("assetsLoadError")}
          action={<Button onClick={() => assets.refetch()}>{t("retry")}</Button>}
          style={{ marginBottom: 16 }}
        />
      )}

      {/* A failed load shows only the retry: an empty list beside it would say "nothing yet". */}
      {!assets.error && (
        <DataCard flush className="settings-list-card">
          <ResponsiveList
            label={t("investmentAssets")}
            items={sortedAssets}
            getKey={(asset) => asset.id}
            isLoading={assets.isLoading}
            loading={
              <div role="status" aria-label={t("loadingAssets")}>
                <Skeleton active paragraph={{ rows: 3 }} title={false} />
              </div>
            }
            // No action: the page's own "Novo ativo" is the way out.
            empty={
              <EmptyState
                title={t("noAssetsRegistered")}
                hint={t("noAssetsRegisteredHint")}
              />
            }
            row={(asset) => ({
              title: asset.name,
              meta: [asset.ticker, classLabel(asset.asset_class), asset.asset_type, asset.currency_code]
                .filter(Boolean)
                .join(" · "),
              trailing: menu(asset),
              ariaLabel: asset.name,
            })}
            wide={
              <Table<InvestmentAsset>
                aria-label={t("investmentAssets")}
                className="investment-table"
                rowKey="id"
                columns={columns}
                dataSource={sortedAssets}
                pagination={false}
                onRow={(asset) => ({
                  onClick: () => {
                    if (catalogReady) setEditing(asset);
                  },
                  style: { cursor: catalogReady ? "pointer" : "default" },
                })}
              />
            }
          />
        </DataCard>
      )}

      <FormDrawer
        title={editing ? t("editAsset") : t("newAsset")}
        open={open}
        onClose={close}
        onSubmit={() => void submit()}
        submitting={assets.isSaving}
        error={formError}
      >
        <InvestmentAssetFields
          draft={draft}
          onChange={setDraft}
          classification={assets.classification}
          quoteMarket={quoteMarket}
        />
      </FormDrawer>
    </>
  );
}
