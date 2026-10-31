import { translate, type Language, type MessageKey } from "../../i18n/messages";
// Accessible names for the category form's icon and colour pickers. The
// registry keys (`ellipsis`, `line-chart`) are what the API stores, not
// something to show a person: the pickers display the glyph or the swatch
// and name it with these.

export const categoryIconLabel: Record<string, MessageKey> = {
  "shopping-cart": "categoryIconShoppingCart",
  coffee: "categoryIconCoffee",
  car: "categoryIconCar",
  shopping: "categoryIconShoppingBag",
  bank: "categoryIconBank",
  home: "categoryIconHome",
  smile: "categoryIconSmile",
  percentage: "categoryIconPercentage",
  medicine: "categoryIconHealth",
  scissor: "categoryIconScissors",
  team: "categoryIconPeople",
  tool: "categoryIconTool",
  wifi: "categoryIconInternet",
  book: "categoryIconBook",
  "safety-certificate": "categoryIconSecurity",
  heart: "categoryIconHeart",
  global: "categoryIconWorld",
  gift: "categoryIconGift",
  "line-chart": "categoryIconLineChart",
  ellipsis: "categoryIconEllipsis",
  "money-collect": "categoryIconMoney",
  laptop: "categoryIconLaptop",
  trophy: "categoryIconTrophy",
  rise: "categoryIconGrowth",
  rollback: "categoryIconRefund",
  wallet: "categoryIconWallet",
  "plus-circle": "categoryIconPlus",
  swap: "categoryIconExchange",
  phone: "categoryIconPhone",
  fund: "categoryIconFund",
  dollar: "categoryIconDollar",
  cloud: "categoryIconCloud",
  hourglass: "categoryIconHourglass",
  thunderbolt: "categoryIconLightning",
  "credit-card": "categoryIconCreditCard",
  "customer-service": "categoryIconCustomerService",
  fire: "categoryIconFire",
  bulb: "categoryIconLightBulb",
};

export const categoryColorLabel: Record<string, MessageKey> = {
  "#2a78d6": "categoryColorBlue",
  "#eb6834": "categoryColorBurntOrange",
  "#17a2b8": "categoryColorTurquoise",
  "#e64980": "categoryColorDarkPink",
  "#d64545": "categoryColorRed",
  "#b8860b": "categoryColorOchre",
  "#eda100": "categoryColorAmber",
  "#495057": "categoryColorGraphite",
  "#37b24d": "categoryColorGreen",
  "#e87ba4": "categoryColorPink",
  "#4c6ef5": "categoryColorIndigo",
  "#6c757d": "categoryColorGray",
  "#099268": "categoryColorSeaGreen",
  "#7c5cbf": "categoryColorPurple",
  "#1baf7a": "categoryColorJuliusGreen",
  "#f76707": "categoryColorOrange",
};

/** The accessible name of an icon key, falling back to the key for a legacy one. */
export function iconLabel(key: string, language: Language = "pt-BR"): string {
  return categoryIconLabel[key] ? translate(language, categoryIconLabel[key]) : key;
}

export function colorLabel(hex: string, language: Language = "pt-BR"): string {
  return categoryColorLabel[hex] ? translate(language, categoryColorLabel[hex]) : hex;
}
