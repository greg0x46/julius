import { useTranslation } from "../../i18n/context";
import { CalendarOutlined, CaretDownFilled, LeftOutlined, RightOutlined } from "../ui/icons";
import { Button, Input, Popover } from "../ui";
import { useState } from "react";

import { periodNavigation } from "./periodNavigation";

export interface DateRangePreset {
  value: string;
  label: string;
  /** A pair of nulls means "the whole period" — no bounds at all. */
  range: () => [string | null, string | null];
}

type Props = {
  value: [string | null, string | null];
  presets: DateRangePreset[];
  onChange: (from: string | null, to: string | null) => void;
  /** The shortcut back to the screen's own default window. */
  reset?: { preset: string; label: string };
  /** Drops the button chrome, for a context bar or a card header. */
  bare?: boolean;
  id?: string;
};


/**
 * The period control shared by the transactions filter bar and the Home
 * balance widget: previous/next arrows over the current window, a popover with
 * the shared shortcuts and a custom from/to panel, and a link back to the
 * screen's default window.
 *
 * It owns no period of its own — the caller keeps it in the URL, in state or
 * in localStorage, whichever fits the screen.
 */
export function PeriodNavigator({ value, presets, onChange, reset, bare, id = "filter-period" }: Props) {
  const { t, language } = useTranslation();
  const [open, setOpen] = useState(false);
  const [customDates, setCustomDates] = useState<[string, string]>(["", ""]);
  const [error, setError] = useState<string | null>(null);
  const [from, to] = value;

  const navigation = periodNavigation(from, to, language);
  const selectedPreset = presets.find((preset) => {
    const [presetFrom, presetTo] = preset.range();
    return from === presetFrom && to === presetTo;
  });
  const resetTo = reset ?? { preset: "this-month", label: t("currentMonth") };
  const resetRange = presets.find((preset) => preset.value === resetTo.preset)?.range();

  const commit = (nextFrom: string | null, nextTo: string | null) => {
    onChange(nextFrom, nextTo);
    setOpen(false);
  };

  const confirmCustom = () => {
    const [customFrom, customTo] = customDates;
    if (customFrom === "" !== (customTo === "")) {
      setError(t("periodBothDates"));
      return;
    }
    if (customFrom !== "" && customTo !== "" && customFrom > customTo) {
      setError(t("periodInvalidDates"));
      return;
    }
    setError(null);
    commit(customFrom || null, customTo || null);
  };

  return (
    <div
      className={`filter-period-navigation${bare ? " filter-period-navigation-bare" : ""}`}
      role="group"
      aria-label={t("periodNavigation")}
    >
      <Button
        className="filter-period-arrow"
        aria-label={navigation?.previousLabel ?? t("previousPeriod")}
        title={navigation?.previousLabel ?? t("previousPeriod")}
        icon={<LeftOutlined aria-hidden="true" />}
        disabled={!navigation?.previous}
        onClick={() => {
          if (navigation?.previous) commit(...navigation.previous);
        }}
      />
      <Popover
        title={t("selectPeriod")}
        trigger="click"
        open={open}
        onOpenChange={(next) => {
          setError(null);
          if (next) setCustomDates([from ?? "", to ?? ""]);
          setOpen(next);
        }}
        content={
          <>
            <div className="filter-period-presets">
              {presets.map((preset) => (
                <Button
                  key={preset.value}
                  type={selectedPreset?.value === preset.value ? "primary" : "default"}
                  onClick={() => commit(...preset.range())}
                >
                  {preset.label}
                </Button>
              ))}
            </div>
            <div className="filter-date-panel">
              <label htmlFor={`${id}-from`}>{t("startDate")}</label>
              <Input
                id={`${id}-from`}
                type="date"
                value={customDates[0]}
                onChange={(event) => setCustomDates((current) => [event.target.value, current[1]])}
              />
              <label htmlFor={`${id}-to`}>{t("endDate")}</label>
              <Input
                id={`${id}-to`}
                type="date"
                value={customDates[1]}
                onChange={(event) => setCustomDates((current) => [current[0], event.target.value])}
              />
              {error && <p role="alert">{error}</p>}
              <Button type="primary" onClick={confirmCustom}>
                {t("confirmPeriod")}
              </Button>
            </div>
          </>
        }
      >
        <Button className="filter-period-heading" aria-label={t("selectPeriod")} aria-expanded={open}>
          <CalendarOutlined className="filter-period-heading-icon" aria-hidden="true" />
          <span aria-live="polite" aria-atomic="true">
            {navigation?.label ?? (from === null && to === null ? t("allTime") : t("choosePeriod"))}
          </span>
          <CaretDownFilled className="filter-period-heading-caret" aria-hidden="true" />
        </Button>
      </Popover>
      <Button
        className="filter-period-arrow"
        aria-label={navigation?.nextLabel ?? t("nextPeriod")}
        title={navigation?.nextLabel ?? t("nextPeriod")}
        icon={<RightOutlined aria-hidden="true" />}
        disabled={!navigation?.next}
        onClick={() => {
          if (navigation?.next) commit(...navigation.next);
        }}
      />
      {resetRange && (from !== resetRange[0] || to !== resetRange[1]) && (
        <Button type="link" onClick={() => commit(...resetRange)}>
          {resetTo.label}
        </Button>
      )}
    </div>
  );
}
