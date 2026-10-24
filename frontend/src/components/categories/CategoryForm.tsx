import { CheckOutlined } from "../ui/icons";
import { Input, Select, Switch } from "../ui";
import { useEffect, useState } from "react";

import type { Category, CategoryKind } from "../../api/contracts";
import {
  categoryColorPalette,
  categoryIconRegistry,
  categoryKindLabel,
  renderCategoryIcon,
} from "../../presentation/categoryLabels";
import { FormDrawer } from "../forms/FormDrawer";
import { FormField } from "../forms/FormField";
import { colorLabel, iconLabel } from "./categoryPickerLabels";

const kindOptions = (Object.keys(categoryKindLabel) as CategoryKind[]).map((value) => ({
  value,
  label: categoryKindLabel[value],
}));

const defaultIcon = "ellipsis";
const defaultColor = categoryColorPalette[0];

interface CategoryDraft {
  name: string;
  kind: CategoryKind;
  icon: string;
  color: string;
  is_active: boolean;
}

function draftFrom(category: Category | null): CategoryDraft {
  return category
    ? {
        name: category.name,
        kind: category.kind,
        icon: category.icon,
        color: category.color,
        is_active: category.is_active,
      }
    : { name: "", kind: "expense", icon: defaultIcon, color: defaultColor, is_active: true };
}

export function CategoryForm({
  open,
  category,
  submitting,
  submitError,
  onSubmit,
  onCancel,
}: {
  open: boolean;
  category: Category | null;
  submitting: boolean;
  submitError: string | null;
  onSubmit: (draft: CategoryDraft) => void;
  onCancel: () => void;
}) {
  const [draft, setDraft] = useState<CategoryDraft>(() => draftFrom(category));
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (open) {
      setDraft(draftFrom(category));
      setError(null);
    }
  }, [open, category]);

  const submit = () => {
    if (draft.name.trim() === "") {
      setError("Informe um nome para a categoria.");
      return;
    }
    setError(null);
    onSubmit({ ...draft, name: draft.name.trim() });
  };

  return (
    <FormDrawer
      title={category ? "Editar categoria" : "Nova categoria"}
      open={open}
      onClose={onCancel}
      onSubmit={submit}
      submitting={submitting}
      error={error ?? submitError}
    >
      <FormField label="Nome" htmlFor="category-name">
        <Input
          id="category-name"
          value={draft.name}
          onChange={(event) => setDraft((current) => ({ ...current, name: event.target.value }))}
          placeholder="Ex.: Manutenção do Apartamento"
        />
      </FormField>

      <FormField label="Tipo" htmlFor="category-kind">
        <Select
          id="category-kind"
          value={draft.kind}
          options={kindOptions}
          disabled={category !== null}
          onChange={(value: CategoryKind) => setDraft((current) => ({ ...current, kind: value }))}
        />
      </FormField>

      {category && (
        <FormField
          label="Ativa"
          htmlFor="category-active"
          hint="Categorias não são excluídas. Desative para deixar de usar."
        >
          <Switch
            id="category-active"
            aria-label="Categoria ativa"
            checked={draft.is_active}
            onChange={(checked) => setDraft((current) => ({ ...current, is_active: checked }))}
            style={{ width: "fit-content" }}
          />
        </FormField>
      )}

      <FormField label="Ícone" labelId="category-icon-label">
        <div className="category-icon-grid" role="radiogroup" aria-labelledby="category-icon-label">
          {Object.keys(categoryIconRegistry).map((key) => {
            const selected = draft.icon === key;
            return (
              <button
                key={key}
                type="button"
                role="radio"
                aria-checked={selected}
                aria-label={iconLabel(key)}
                title={iconLabel(key)}
                className="category-icon-option"
                onClick={() => setDraft((current) => ({ ...current, icon: key }))}
              >
                {renderCategoryIcon(key)}
              </button>
            );
          })}
        </div>
      </FormField>

      <FormField label="Cor" labelId="category-color-label">
        <div className="category-color-grid" role="radiogroup" aria-labelledby="category-color-label">
          {categoryColorPalette.map((hex) => {
            const selected = draft.color === hex;
            return (
              <button
                key={hex}
                type="button"
                role="radio"
                aria-checked={selected}
                aria-label={`Cor ${colorLabel(hex)}`}
                title={colorLabel(hex)}
                className="category-color-swatch"
                style={{ backgroundColor: hex }}
                onClick={() => setDraft((current) => ({ ...current, color: hex }))}
              >
                {selected && <CheckOutlined style={{ color: "#fff" }} aria-hidden="true" />}
              </button>
            );
          })}
        </div>
      </FormField>
    </FormDrawer>
  );
}
