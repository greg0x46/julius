const STROKE = { fill: "none", stroke: "currentColor", strokeLinecap: "round", strokeLinejoin: "round" } as const;

/** Check once the goal is reached, a target while it is still being chased. */
export function GoalIcon({ reached }: { reached: boolean }) {
  return reached ? (
    <svg width="16" height="16" viewBox="0 0 16 16">
      <path d="M3 8.5L6.5 12L13 4.5" strokeWidth="1.6" {...STROKE} />
    </svg>
  ) : (
    <svg width="16" height="16" viewBox="0 0 16 16">
      <circle cx="8" cy="8" r="7" strokeWidth="1.3" {...STROKE} />
      <circle cx="8" cy="8" r="4" strokeWidth="1.3" {...STROKE} />
      <circle cx="8" cy="8" r="1.3" fill="currentColor" />
    </svg>
  );
}

export function TargetDateIcon() {
  return (
    <svg width="13" height="13" viewBox="0 0 16 16" aria-hidden="true">
      <rect x="2" y="3" width="12" height="11" rx="1.5" strokeWidth="1.2" {...STROKE} />
      <path d="M2 6.5h12M5 1.5V4M11 1.5V4" strokeWidth="1.2" {...STROKE} />
    </svg>
  );
}
