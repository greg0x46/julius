import type { SVGProps } from "react";
type IconProps = SVGProps<SVGSVGElement> & { spin?: boolean };
function Icon({ path, spin, ...props }: IconProps & { path: string }) {
  return (
    <svg
      {...props}
      viewBox="0 0 24 24"
      width="1em"
      height="1em"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.7"
      strokeLinecap="round"
      strokeLinejoin="round"
      className={`ui-icon ${spin ? "ui-icon-spin" : ""} ${props.className ?? ""}`}
      aria-hidden={props["aria-label"] ? undefined : true}
      role={props["aria-label"] ? "img" : undefined}
    >
      <path d={path} />
    </svg>
  );
}
export function AimOutlined(props: IconProps) {
  return (
    <Icon
      {...props}
      path="M12 2v4m0 12v4M2 12h4m12 0h4M20 12a8 8 0 1 1-16 0 8 8 0 0 1 16 0M15 12a3 3 0 1 1-6 0 3 3 0 0 1 6 0"
    />
  );
}
export function ArrowLeftOutlined(props: IconProps) {
  return <Icon {...props} path="M20 12H4m7-7-7 7 7 7" />;
}
export function BankOutlined(props: IconProps) {
  return (
    <Icon {...props} path="m2 8 10-6 10 6ZM4 11v8m5-8v8m6-8v8m5-8v8M2 22h20" />
  );
}
export function BulbOutlined(props: IconProps) {
  return <Icon {...props} path="M9 18v4h6v-4M8 15a7 7 0 1 1 8 0l-1 3H9Z" />;
}
export function CalendarOutlined(props: IconProps) {
  return <Icon {...props} path="M3 5h18v16H3ZM3 10h18M7 2v6m10-6v6" />;
}
export function CarOutlined(props: IconProps) {
  return (
    <Icon
      {...props}
      path="M3 10l3-7h12l3 7v9H3ZM3 10h18M6 15h2m8 0h2M5 19v3m14-3v3"
    />
  );
}
export function CaretDownFilled(props: IconProps) {
  return <Icon {...props} path="m5 9 7 7 7-7" />;
}
export function CaretDownOutlined(props: IconProps) {
  return <Icon {...props} path="m5 9 7 7 7-7" />;
}
export function CaretRightOutlined(props: IconProps) {
  return <Icon {...props} path="m9 5 7 7-7 7" />;
}
export function CheckOutlined(props: IconProps) {
  return <Icon {...props} path="m4 12 5 5L20 6" />;
}
export function CloseOutlined(props: IconProps) {
  return <Icon {...props} path="m5 5 14 14M5 19 19 5" />;
}
export function CloudOutlined(props: IconProps) {
  return (
    <Icon
      {...props}
      path="M6 20a5 5 0 0 1-1-10 7 7 0 0 1 14 0 5 5 0 0 1-1 10Z"
    />
  );
}
export function CoffeeOutlined(props: IconProps) {
  return (
    <Icon
      {...props}
      path="M3 7h13v10H3Zm13 1h3a3 3 0 0 1 0 6h-3M2 21h16M6 2v2m5-2v2"
    />
  );
}
export function CreditCardOutlined(props: IconProps) {
  return <Icon {...props} path="M2 5h20v14H2ZM2 9h20M5 15h4" />;
}
export function CustomerServiceOutlined(props: IconProps) {
  return (
    <Icon
      {...props}
      path="M3 13v-1a9 9 0 0 1 18 0v6h-5v-7h5M3 11h5v7H3Zm18 7c0 4-6 4-9 4"
    />
  );
}
export function DeleteOutlined(props: IconProps) {
  return (
    <Icon {...props} path="M3 6h18M9 6V3h6v3M5 6l1 15h12l1-15M10 10v7m4-7v7" />
  );
}
export function DollarOutlined(props: IconProps) {
  return (
    <Icon {...props} path="M12 2v20m6-16H9a4 4 0 0 0 0 8h6a4 4 0 0 1 0 8H6" />
  );
}
export function DownOutlined(props: IconProps) {
  return <Icon {...props} path="m5 9 7 7 7-7" />;
}
export function DownloadOutlined(props: IconProps) {
  return <Icon {...props} path="M12 3v14m-5-5 5 5 5-5M3 16v5h18v-5" />;
}
export function EllipsisOutlined(props: IconProps) {
  return <Icon {...props} path="M5 12h.01M12 12h.01M19 12h.01" />;
}
export function ExclamationCircleOutlined(props: IconProps) {
  return (
    <Icon
      {...props}
      path="M12 6v8m0 3v.01M22 12A10 10 0 1 1 2 12a10 10 0 0 1 20 0"
    />
  );
}
export function FilterFilled(props: IconProps) {
  return <Icon {...props} path="M3 4h18l-7 8v7l-4 2V12Z" />;
}
export function FilterOutlined(props: IconProps) {
  return <Icon {...props} path="M3 4h18l-7 8v7l-4 2V12Z" />;
}
export function FireOutlined(props: IconProps) {
  return (
    <Icon
      {...props}
      path="M12 2c3 6-3 6 2 11 2-2 3-4 3-6 9 10 2 15-5 15S0 15 7 8c-1 5 4 5 5-6"
    />
  );
}
export function FundOutlined(props: IconProps) {
  return <Icon {...props} path="M3 3v18h18M7 17v-5m5 5V7m5 10v-7" />;
}
export function GiftOutlined(props: IconProps) {
  return (
    <Icon
      {...props}
      path="M3 8h18v5H3Zm2 5v8h14v-8M12 8v13m0-13H8a3 3 0 1 1 3-3Zm0 0h4a3 3 0 1 0-3-3Z"
    />
  );
}
export function GlobalOutlined(props: IconProps) {
  return (
    <Icon
      {...props}
      path="M22 12A10 10 0 1 1 2 12a10 10 0 0 1 20 0M2 12h20M12 2c-5 6-5 14 0 20 5-6 5-14 0-20"
    />
  );
}
export function HeartOutlined(props: IconProps) {
  return <Icon {...props} path="M12 21 3 12a6 6 0 0 1 9-8 6 6 0 0 1 9 8Z" />;
}
export function HomeOutlined(props: IconProps) {
  return <Icon {...props} path="m3 10 9-7 9 7v11h-6v-8H9v8H3Z" />;
}
export function HourglassOutlined(props: IconProps) {
  return (
    <Icon
      {...props}
      path="M5 2h14M5 22h14M6 2v5l6 5-6 5v5m12-20v5l-6 5 6 5v5"
    />
  );
}
export function InboxOutlined(props: IconProps) {
  return <Icon {...props} path="M3 3h18v18H3ZM3 14h5l2 3h4l2-3h5" />;
}
export function InfoCircleOutlined(props: IconProps) {
  return (
    <Icon
      {...props}
      path="M12 11v6m0-10v.01M22 12A10 10 0 1 1 2 12a10 10 0 0 1 20 0"
    />
  );
}
export function LaptopOutlined(props: IconProps) {
  return <Icon {...props} path="M4 3h16v14H4ZM2 21h20l-2-4H4Z" />;
}
export function LeftOutlined(props: IconProps) {
  return <Icon {...props} path="m15 5-7 7 7 7" />;
}
export function LineChartOutlined(props: IconProps) {
  return <Icon {...props} path="M3 3v18h18M5 16l5-6 4 3 7-8" />;
}
export function LinkOutlined(props: IconProps) {
  return (
    <Icon
      {...props}
      path="m8 15-2 2a4 4 0 0 1-6-6l5-5a4 4 0 0 1 6 0m2 3 2-2a4 4 0 0 1 6 6l-5 5a4 4 0 0 1-6 0m-2-3 7-7"
    />
  );
}
export function MedicineBoxOutlined(props: IconProps) {
  return <Icon {...props} path="M3 7h18v14H3ZM8 7V3h8v4m-4 3v8m-4-4h8" />;
}
export function MoneyCollectOutlined(props: IconProps) {
  return (
    <Icon {...props} path="M2 4h20v12H2Zm10 3v6m-6-3h.01m12 0h.01M6 20h12" />
  );
}
export function MoonOutlined(props: IconProps) {
  return <Icon {...props} path="M21 12.8A9 9 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8Z" />;
}
export function PercentageOutlined(props: IconProps) {
  return (
    <Icon
      {...props}
      path="m4 20 16-16M8 6a2 2 0 1 1-4 0 2 2 0 0 1 4 0m12 12a2 2 0 1 1-4 0 2 2 0 0 1 4 0"
    />
  );
}
export function PhoneOutlined(props: IconProps) {
  return <Icon {...props} path="M7 2h10v20H7Zm4 17h2" />;
}
export function PieChartOutlined(props: IconProps) {
  return (
    <Icon
      {...props}
      path="M11 2v11h11A10 10 0 1 1 11 2Zm4 0v7h7a10 10 0 0 0-7-7"
    />
  );
}
export function PlusCircleOutlined(props: IconProps) {
  return (
    <Icon
      {...props}
      path="M12 6v12M6 12h12M22 12A10 10 0 1 1 2 12a10 10 0 0 1 20 0"
    />
  );
}
export function PlusOutlined(props: IconProps) {
  return <Icon {...props} path="M12 4v16M4 12h16" />;
}
export function QuestionCircleOutlined(props: IconProps) {
  return (
    <Icon
      {...props}
      path="M9 8a3 3 0 1 1 4 3c-1 1-1 2-1 3m0 3v.01M22 12A10 10 0 1 1 2 12a10 10 0 0 1 20 0"
    />
  );
}
export function ReadOutlined(props: IconProps) {
  return (
    <Icon
      {...props}
      path="M12 5C8 2 4 3 2 4v16c4-2 7-2 10 0 3-2 6-2 10 0V4c-4-2-7-2-10 1Zm0 0v15"
    />
  );
}
export function RedoOutlined(props: IconProps) {
  return <Icon {...props} path="M20 8V3l-4 4a8 8 0 1 0 3 10M20 3h-5" />;
}
export function RightOutlined(props: IconProps) {
  return <Icon {...props} path="m9 5 7 7-7 7" />;
}
export function RiseOutlined(props: IconProps) {
  return <Icon {...props} path="m3 17 6-6 4 4 8-11m-6 0h6v6" />;
}
export function RollbackOutlined(props: IconProps) {
  return <Icon {...props} path="M4 4v7h7M4 11l5-5h7a6 6 0 0 1 0 12h-5" />;
}
export function SafetyCertificateOutlined(props: IconProps) {
  return (
    <Icon {...props} path="m12 2 9 4v7c0 5-9 9-9 9s-9-4-9-9V6Zm-5 10 3 3 7-7" />
  );
}
export function ScissorOutlined(props: IconProps) {
  return (
    <Icon
      {...props}
      path="m5 4 15 16M5 20 20 4M8 5a3 3 0 1 1-6 0 3 3 0 0 1 6 0m0 14a3 3 0 1 1-6 0 3 3 0 0 1 6 0"
    />
  );
}
export function SearchOutlined(props: IconProps) {
  return (
    <Icon {...props} path="M21 21l-5-5M18 10a8 8 0 1 1-16 0 8 8 0 0 1 16 0" />
  );
}
export function SettingOutlined(props: IconProps) {
  return (
    <Icon
      {...props}
      path="m9 3-1 3-3 1-2 5 2 5 3 1 1 3h6l1-3 3-1 2-5-2-5-3-1-1-3ZM16 12a4 4 0 1 1-8 0 4 4 0 0 1 8 0"
    />
  );
}
export function ShoppingCartOutlined(props: IconProps) {
  return <Icon {...props} path="M2 3h3l3 12h11l3-8H6m4 13h.01m8 0h.01" />;
}
export function ShoppingOutlined(props: IconProps) {
  return <Icon {...props} path="M4 7h16l1 14H3ZM8 7V5a4 4 0 0 1 8 0v2" />;
}
export function SmileOutlined(props: IconProps) {
  return (
    <Icon
      {...props}
      path="M22 12A10 10 0 1 1 2 12a10 10 0 0 1 20 0M7 9h.01M17 9h.01M7 15q5 6 10 0"
    />
  );
}
export function SunOutlined(props: IconProps) {
  return (
    <Icon
      {...props}
      path="M12 8a4 4 0 1 0 0 8 4 4 0 0 0 0-8ZM12 2v2m0 16v2M4.9 4.9l1.4 1.4m11.4 11.4 1.4 1.4M2 12h2m16 0h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4"
    />
  );
}
export function SwapOutlined(props: IconProps) {
  return <Icon {...props} path="M3 7h17l-4-4m5 14H4l4 4M20 7l-4 4M4 17l4-4" />;
}
export function SyncOutlined(props: IconProps) {
  return (
    <Icon
      {...props}
      path="M3 9a9 9 0 0 1 16-4l2 3m0-6v6h-6M21 15A9 9 0 0 1 5 19l-2-3m0 6v-6h6"
    />
  );
}
export function TeamOutlined(props: IconProps) {
  return (
    <Icon
      {...props}
      path="M10 7a3 3 0 1 1-6 0 3 3 0 0 1 6 0m10 0a3 3 0 1 1-6 0 3 3 0 0 1 6 0M1 21v-4a6 6 0 0 1 12 0v4m0-9a6 6 0 0 1 10 5v4"
    />
  );
}
export function ThunderboltOutlined(props: IconProps) {
  return <Icon {...props} path="m13 2-9 12h7l-1 8 10-13h-7Z" />;
}
export function ToolOutlined(props: IconProps) {
  return (
    <Icon
      {...props}
      path="m14 3 1 5 5 1 2-4a7 7 0 0 1-9 10l-8 8-4-4 8-8A7 7 0 0 1 14 3"
    />
  );
}
export function TransactionOutlined(props: IconProps) {
  return <Icon {...props} path="M3 7h17l-4-4m5 14H4l4 4M20 7l-4 4M4 17l4-4" />;
}
export function TrophyOutlined(props: IconProps) {
  return (
    <Icon
      {...props}
      path="M6 3h12v7a6 6 0 0 1-12 0ZM6 5H2v3a5 5 0 0 0 5 5m11-8h4v3a5 5 0 0 1-5 5m-5 3v5m-5 0h10"
    />
  );
}
export function UploadOutlined(props: IconProps) {
  return <Icon {...props} path="M12 17V3m-5 5 5-5 5 5M3 16v5h18v-5" />;
}
export function WalletOutlined(props: IconProps) {
  return <Icon {...props} path="M3 5h17v15H3ZM3 5V3h14v2m-1 6h6v5h-6Z" />;
}
export function WifiOutlined(props: IconProps) {
  return (
    <Icon
      {...props}
      path="M2 8a16 16 0 0 1 20 0M5 12a11 11 0 0 1 14 0m-10 4a5 5 0 0 1 6 0m-3 4h.01"
    />
  );
}
