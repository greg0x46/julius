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
      feedback.success("Categoria salva");
    } catch (error) {
      setSaveError(errorMessage(error, "Não foi possível salvar a categoria."));
    }
  };

  const toggle = async (category: Category, isActive: boolean) => {
    setActionError(null);
    setTogglingCategoryId(category.id);
    try {
      await categories.updateCategory({ categoryId: category.id, write: { is_active: isActive } });
      feedback.success(isActive ? "Categoria ativada" : "Categoria desativada");
    } catch (error) {
      const message = errorMessage(error, "Não foi possível atualizar a categoria.");
      setActionError(message);
      feedback.error(message);
    } finally {
      setTogglingCategoryId(null);
    }
  };

  return (
    <PageContainer
      title="Categorias"
      subTitle="Classifique despesas, receitas e transferências"
      compactMobileHeader
      extra={[
        <PageAction key="new-category" label="Nova categoria" shortLabel="Nova" onClick={openCreate} />,
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
          message="Não foi possível carregar as categorias"
          description={<Button onClick={() => categories.refetch()}>Tentar novamente</Button>}
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
