import { Link } from "react-router-dom";
import { useGoals } from "../../hooks/useGoals";
import { useToday } from "../../hooks/useToday";
import { isZeroDecimal } from "../../presentation/decimal";
import { Money } from "../shared/Money";

export function MonthlyContributionIndicator() {
  const { goals, isLoading, error } = useGoals();
  const month = useToday().format("YYYY-MM");
  const goal = goals.find((item) => item.monthly_contribution?.month === month);
  if (isLoading || error || !goal) return null;
  const contribution = goal.monthly_contribution!;
  const date = new Intl.DateTimeFormat("pt-BR", { month: "short", year: "numeric", timeZone: "UTC" }).format(new Date(`${goal.target_date}T00:00:00Z`));
  return (
    <Link className="monthly-contribution" to="/metas">
      <span className="monthly-contribution-label">Falta aportar este mês</span>
      {contribution.remaining === null ? (
        <span>Estimativa indisponível</span>
      ) : (
        <Money value={contribution.remaining} currencyCode="BRL" tone="neutral" size="row" report />
      )}
      {contribution.remaining !== null && isZeroDecimal(contribution.remaining) && (
        <span className="monthly-contribution-context">Aporte do mês em dia</span>
      )}
      <span className="monthly-contribution-context monthly-contribution-goal"><span>{goal.name}</span><span> · {date}</span></span>
      {contribution.remaining === null ? (
        <span className="monthly-contribution-context">Histórico de aportes incompleto ou ambíguo</span>
      ) : contribution.assumes_zero_return && (
        <span className="monthly-contribution-context">Estimativa sem rendimento</span>
      )}
    </Link>
  );
}
