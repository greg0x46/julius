import { Progress } from "antd";

import type { Goal } from "../../api/contracts";
import { formatDateOnly } from "../../presentation/dates";
import { compareDecimals } from "../../presentation/decimal";
import { goalMetricLabel, goalPercent } from "../../presentation/goals";
import { Section } from "../layout";
import { Money } from "../shared/Money";
import { RecordMenu } from "../shared/RecordMenu";
import { useConfirm } from "../shared/useConfirm";

/**
 * One goal as a Section: where it stands against its target. Editar and
 * Excluir live in one "···" menu on the title line.
 */
export function GoalCard({
  goal,
  busy,
  onEdit,
  onRemove,
}: {
  goal: Goal;
  busy: boolean;
  onEdit: (goal: Goal) => void;
  onRemove: (goal: Goal) => void;
}) {
  const confirm = useConfirm();
  const percent = goalPercent(goal.progress);
  const reached = compareDecimals(goal.progress, "1") >= 0;

  return (
    <Section
      title={goal.name}
      className="goal-section"
      trailing={
        <RecordMenu
          label={`Ações de ${goal.name}`}
          items={[
            { key: "edit", label: "Editar meta", onClick: () => onEdit(goal) },
            {
              key: "delete",
              label: "Excluir meta",
              danger: true,
              disabled: busy,
              onClick: () =>
                confirm({
                  title: "Excluir meta",
                  description: "Seus investimentos e saldos não são afetados; só o acompanhamento da meta é removido.",
                  onConfirm: () => onRemove(goal),
                }),
            },
          ]}
        />
      }
    >
      <p className="goal-metric">{goalMetricLabel[goal.metric]}</p>
      <div className="goal-figures">
        <Money currencyCode={goal.currency_code} value={goal.current_value} tone="balance" size="hero" />
        <span className="goal-target">
          de <Money currencyCode={goal.currency_code} value={goal.target_amount} tone="neutral" />
        </span>
      </div>
      <Progress
        percent={Math.min(100, percent)}
        format={() => `${percent}%`}
        status={reached ? "success" : "normal"}
        aria-label={`Progresso de ${goal.name}`}
      />
      {reached && <p className="goal-note">Meta atingida</p>}
      {goal.target_date !== null && <p className="goal-note">Data-alvo: {formatDateOnly(goal.target_date)}</p>}
      {goal.notes !== null && <p className="goal-note">{goal.notes}</p>}
    </Section>
  );
}
