const RADIUS = 40;
const CIRCUMFERENCE = 2 * Math.PI * RADIUS;
const SIZE = 96;
const CENTER = SIZE / 2;

export type GoalProgressTone = "accent" | "warning" | "success";

/**
 * Red → amber → green as the goal nears. Amber sits in the middle because a
 * direct red/green mix goes muddy; the full green is left for a reached goal
 * (the `success` tone), so an unreached ring stops a little short of it.
 */
function progressColor(percent: number): string {
  const position = percent * 0.9;
  return position < 45
    ? `color-mix(in oklab, var(--color-warning) ${Math.round((position / 45) * 100)}%, var(--color-danger))`
    : `color-mix(in oklab, var(--color-success) ${Math.round(((position - 45) / 45) * 100)}%, var(--color-warning))`;
}

interface GoalProgressRingProps {
  /** The real percent (may exceed 100 once the goal is reached); drives the number shown. */
  percent: number;
  tone: GoalProgressTone;
  label: string;
}

/**
 * A goal's progress as a ring instead of a bar: the fill (and the
 * `aria-valuenow` it reports) is capped at 100, but the number in the
 * middle shows the real, uncapped percent — same split as the old linear
 * bar (`Progress percent={capped} format={() => realPercent}`). The stroke
 * colour moves from red to green with the progress.
 */
export function GoalProgressRing({ percent, tone, label }: GoalProgressRingProps) {
  const clamped = Math.min(100, Math.max(0, percent));
  const offset = CIRCUMFERENCE * (1 - clamped / 100);
  const text = `${percent}%`;

  return (
    <svg
      width={SIZE}
      height={SIZE}
      viewBox={`0 0 ${SIZE} ${SIZE}`}
      className={`goal-ring goal-ring-${tone}`}
      role="progressbar"
      aria-label={label}
      aria-valuenow={clamped}
      aria-valuemin={0}
      aria-valuemax={100}
    >
      <circle cx={CENTER} cy={CENTER} r={RADIUS} className="goal-ring-track" />
      <circle
        cx={CENTER}
        cy={CENTER}
        r={RADIUS}
        className="goal-ring-fill"
        style={tone === "success" ? undefined : { stroke: progressColor(clamped) }}
        strokeDasharray={CIRCUMFERENCE}
        strokeDashoffset={offset}
        transform={`rotate(-90 ${CENTER} ${CENTER})`}
      />
      <text
        x={CENTER}
        y={CENTER - 4}
        textAnchor="middle"
        className="goal-ring-text"
        // Squeeze a long percent (e.g. 12345%) to fit inside the ring.
        textLength={text.length > 5 ? text.length * 8.5 : undefined}
        lengthAdjust="spacingAndGlyphs"
      >
        {text}
      </text>
      <text x={CENTER} y={CENTER + 16} textAnchor="middle" className="goal-ring-subtext">
        da meta
      </text>
    </svg>
  );
}
