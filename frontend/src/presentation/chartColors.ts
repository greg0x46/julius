// Chart colours, by the job each one does (dataviz skill: identity vs state).
//
// Balance-over-time charts (Home's "Evolução do saldo", Patrimônio)
// use the brand pair: ink for what already happened, brand green for what is
// still a forecast, and a third, cooler hue only for a simulation. They never
// borrow the income/expense green and red of the money tones — a line going
// down is not an "expense" — except the single red marker for the lowest
// balance, which is a warning and so is allowed the danger red.
export const balanceChartColor = {
  realized: "#172b4d",
  projected: "#1baf7a",
  simulation: "#3b5b8c",
  lowest: "#c4302b",
  grid: "#e5e9f0",
  axis: "#5d697c",
} as const;

// Categorical slots for "Por categoria". Deliberately free of the green and the
// red that mean income/expense everywhere else, so a slice never reads as
// "good" or "bad". Order validated with the dataviz skill's validator
// (adjacent-pair CVD and normal-vision separation PASS on the light surface);
// contrast against white is below 3:1 for two slots, which is why every slice
// also has a named, amount-labelled row next to the donut.
export const categoryPalette = [
  "#2a78d6", // blue
  "#eb6834", // orange
  "#17a2b8", // teal
  "#4a3aa7", // violet
  "#eda100", // yellow
  "#e87ba4", // magenta
] as const;

/** "Sem categoria": a neutral, not a hue — it is the absence of a category. */
export const uncategorizedColor = "#898781";
/** "Outras": the folded tail of the list, lighter than any single category. */
export const otherCategoriesColor = "#c3c7ce";
