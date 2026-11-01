import { useTranslation } from "../../i18n/context";
import type { Column } from "../ui/Table";
import Table from "../ui/Table";
import { Skeleton, Switch } from "../ui";

import { readableTextOn } from "../../presentation/readableText";
import type { Category, CategoryKind } from "../../api/contracts";
import { categoryKindName, renderCategoryIcon } from "../../presentation/categoryLabels";
import { DataCard, EmptyState, ListRow, ResponsiveList, Section } from "../layout";
import { RecordMenu } from "../shared/RecordMenu";
import { StatusTag } from "../shared/StatusTag";
import { useCompactScreen } from "../shared/useCompactScreen";

const kindOrder: CategoryKind[] = ["expense", "income", "transfer"];

/** Section headings on a phone, where the list is grouped by kind. */
const kindGroupTitle = { expense: "expenseCategories", income: "incomeCategories", transfer: "transferCategories" } as const;

function CategoryBadge({ category }: { category: Category }) {
  return (
    <span className="category-badge" style={{ backgroundColor: category.color, color: readableTextOn(category.color) }} aria-hidden="true">
      {renderCategoryIcon(category.icon)}
    </span>
  );
}

function CategoryName({ category }: { category: Category }) {
  return (
    <span className="category-row-name">
      <CategoryBadge category={category} />
      <span>{category.name}</span>
    </span>
  );
}

// No action: the page's own "Nova categoria" is the way out.


/**
 * Categories as a table from `md` up and, on a phone, as rows grouped under
 * Despesas / Receitas / Transferências: colour badge and name, "Inativa" only
 * when it is, and a tap anywhere on the row opens the edit sheet. The active
 * switch and the `···` of the table are not repeated on the row — the sheet
 * already has both.
 */
export function CategoryList({
  categories,
  isLoading,
  togglingCategoryId,
  onRename,
  onToggle,
}: {
  categories: Category[];
  isLoading: boolean;
  togglingCategoryId: string | null;
  onRename: (category: Category) => void;
  onToggle: (category: Category, isActive: boolean) => void;
}) {
  const { t, language } = useTranslation();
  const emptyState = <DataCard><EmptyState title={t("noCategoriesYet")} hint={t("noCategoriesHint")} /></DataCard>;
  const compact = useCompactScreen();

  const columns: Column<Category>[] = [
    {
      title: t("name"),
      dataIndex: "name",
      render: (_, category) => <CategoryName category={category} />,
    },
    {
      title: t("type"),
      dataIndex: "kind",
      render: (_, category) => categoryKindName(category.kind, language),
      filters: kindOrder.map((kind) => ({ text: categoryKindName(kind, language), value: kind })),
      onFilter: (value, category) => category.kind === value,
    },
    {
      title: t("activeFeminine"),
      dataIndex: "is_active",
      render: (_, category) => (
        <span onClick={(event) => event.stopPropagation()}>
          <Switch
            aria-label={t(category.is_active ? "deactivateCategoryNamed" : "activateCategoryNamed", { name: category.name })}
            checked={category.is_active}
            loading={togglingCategoryId === category.id}
            onChange={(checked) => onToggle(category, checked)}
          />
        </span>
      ),
    },
    {
      title: <span className="visually-hidden">{t("categoryActions")}</span>,
      key: "options",
      width: 56,
      render: (_, category) => (
        <span onClick={(event) => event.stopPropagation()}>
          <RecordMenu
            label={t("actionsFor", { name: category.name })}
            items={[{ key: "edit", label: t("edit"), onClick: () => onRename(category) }]}
          />
        </span>
      ),
    },
  ];

  const table = (
    <DataCard flush>
      <Table<Category>
        aria-label={t("categories")}
        columns={columns}
        dataSource={categories}
        rowKey="id"
        pagination={false}
        onRow={(category) => ({
          onClick: () => onRename(category),
          style: { cursor: "pointer" },
        })}
      />
    </DataCard>
  );

  const row = (category: Category) => ({
    title: <CategoryName category={category} />,
    status: category.is_active ? undefined : <StatusTag tone="neutral">{t("inactiveFeminine")}</StatusTag>,
    onClick: () => onRename(category),
    ariaLabel: t("editCategoryNamed", { name: category.name }),
  });

  if (compact && !isLoading && categories.length > 0) {
    return (
      <>
        {kindOrder.map((kind) => {
          const group = categories.filter((category) => category.kind === kind);
          if (group.length === 0) return null;
          return (
            <Section key={kind} title={t(kindGroupTitle[kind])} className="category-group">
              <ul className="list-rows" aria-label={t(kindGroupTitle[kind])}>
                {group.map((category) => (
                  <ListRow key={category.id} {...row(category)} />
                ))}
              </ul>
            </Section>
          );
        })}
      </>
    );
  }

  return (
    <ResponsiveList
      label={t("categories")}
      items={categories}
      getKey={(category) => category.id}
      row={row}
      wide={table}
      isLoading={isLoading}
      loading={<Skeleton active paragraph={{ rows: 6 }} />}
      empty={emptyState}
    />
  );
}
