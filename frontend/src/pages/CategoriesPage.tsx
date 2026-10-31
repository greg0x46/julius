import { useTranslation } from "../i18n/context";
import { Alert, Button } from "../components/ui";
import { useState } from "react";

import type { Category, CategoryKind } from "../api/contracts";
import { CategoryForm } from "../components/categories/CategoryForm";
import { CategoryList } from "../components/categories/CategoryList";
import { PageAction } from "../components/layout";
import { SettingsPageContainer as PageContainer } from "../components/SettingsPageContainer";
import { useFeedback } from "../components/shared/useFeedback";
import { useCategories } from "../hooks/useCategories";
import { errorMessage } from "../presentation/errors";

export function CategoriesPage() {
  const { t } = useTranslation();
  const categories = useCategories();
  const feedback = useFeedback();
  const [formOpen, setFormOpen] = useState(false);
  const [editingCategory, setEditingCategory] = useState<Category | null>(null);
  const [togglingCategoryId, setTogglingCategoryId] = useState<string | null>(null);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);

  const openCreate = () => {
    setEditingCategory(null);
    setSaveError(null);
    setFormOpen(true);
  };

  const openEdit = (category: Category) => {
    setEditingCategory(category);
    setSaveError(null);
    setFormOpen(true);
  };

  const closeForm = () => setFormOpen(false);

  const submit = async (draft: {
    name: string;
    kind: CategoryKind;
    icon: string;
    color: string;
    is_active: boolean;
  }) => {
    setSaveError(null);
    try {
      if (editingCategory) {
        await categories.updateCategory({
          categoryId: editingCategory.id,
          write: { name: draft.name, icon: draft.icon, color: draft.color, is_active: draft.is_active },
        });
      } else {
        await categories.createCategory({
          name: draft.name,
          kind: draft.kind,
          icon: draft.icon,
          color: draft.color,
        });
      }
      setFormOpen(false);
      feedback.success(t("categorySaved"));
    } catch (error) {
      setSaveError(errorMessage(error, t("categorySaveErrorFull")));
    }
  };

  const toggle = async (category: Category, isActive: boolean) => {
    setActionError(null);
    setTogglingCategoryId(category.id);
    try {
      await categories.updateCategory({ categoryId: category.id, write: { is_active: isActive } });
      feedback.success(isActive ? t("categoryActivated") : t("categoryDeactivated"));
    } catch (error) {
      const message = errorMessage(error, t("categoryUpdateError"));
      setActionError(message);
      feedback.error(message);
    } finally {
      setTogglingCategoryId(null);
    }
  };

  return (
    <PageContainer
      title={t("categories")}
      subTitle={t("categoriesSubtitle")}
      compactMobileHeader
      extra={[
        <PageAction key="new-category" label={t("newCategory")} shortLabel={t("newFeminine")} onClick={openCreate} />,
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
      {categories.error && (
        <Alert
          type="error"
          showIcon
          message={t("categoriesLoadErrorShort")}
          description={<Button onClick={() => categories.refetch()}>{t("retry")}</Button>}
          style={{ marginBottom: 16 }}
        />
      )}
      {/* A failed load shows only the retry: an empty list beside it would say "nothing yet". */}
      {!categories.error && (
        <CategoryList
          categories={categories.categories}
          isLoading={categories.isLoading}
          togglingCategoryId={togglingCategoryId}
          onRename={openEdit}
          onToggle={toggle}
        />
      )}
      <CategoryForm
        open={formOpen}
        category={editingCategory}
        submitting={categories.isSaving}
        submitError={saveError}
        onSubmit={submit}
        onCancel={closeForm}
      />
    </PageContainer>
  );
}
