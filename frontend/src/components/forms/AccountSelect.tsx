import { useTranslation } from "../../i18n/context";
import { Select } from "../ui";
import { useMemo } from "react";

import type { Account } from "../../api/contracts";
import { useAccounts } from "../../hooks/useAccounts";
import { accountDisplayName, maskedAccountNumber } from "../../presentation/accountLabels";

interface AccountSelectProps {
  id?: string;
  /** The selected account id; empty/undefined/null means none. */
  value?: string | null;
  /** The chosen account id, or null when the selection is cleared (optional only). */
  onChange: (accountId: string | null) => void;
  /** An optional field can be cleared; a required one cannot. */
  optional?: boolean;
  placeholder?: string;
  disabled?: boolean;
  /** Marks the field invalid (red outline, aria-invalid) after a failed submit. */
  invalid?: boolean;
}

/** "Itaú · •••• 0966": what tells two accounts with the same name apart. */
function secondaryLine(account: Account): string {
  const institution =
    account.name !== null && account.institution_name !== null ? account.institution_name : null;
  return [institution, maskedAccountNumber(account.number)].filter(Boolean).join(" · ");
}

/**
 * Picks one of the user's accounts by name (searchable by name, institution
 * and number) instead of asking for a raw account id. The institution and
 * masked number show as a second, quieter line in the dropdown.
 */
export function AccountSelect({
  id,
  value,
  onChange,
  optional = false,
  placeholder,
  disabled,
  invalid,
}: AccountSelectProps) {
  const { t, language } = useTranslation();
  const { accounts, isLoading } = useAccounts();

  const options = useMemo(
    () =>
      accounts.filter((account) => !account.archived_at).map((account) => ({
        value: account.id,
        label: accountDisplayName(account, language),
        secondary: secondaryLine(account),
        searchText: [accountDisplayName(account, language), account.institution_name, account.number]
          .filter(Boolean)
          .join(" "),
      })),
    [accounts, language],
  );

  // While accounts load, a preselected id has no label yet; show the
  // placeholder rather than the raw id.
  const known = options.some((option) => option.value === value);
  const shownValue = value && (known || !isLoading) ? value : undefined;

  return (
    <Select
      id={id}
      value={shownValue}
      options={options}
      placeholder={placeholder ?? t("selectAccount")}
      allowClear={optional}
      loading={isLoading}
      disabled={disabled}
      status={invalid ? "error" : undefined}
      aria-invalid={invalid || undefined}
      showSearch
      optionFilterProp="searchText"
      onChange={(next: string | undefined) => onChange(next ?? null)}
      optionRender={(option) => (
        <div className="account-select-option">
          <span>{option.data.label}</span>
          {option.data.secondary ? (
            <span className="account-select-option-meta">{option.data.secondary}</span>
          ) : null}
        </div>
      )}
    />
  );
}
