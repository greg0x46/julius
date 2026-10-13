import { Descriptions, Typography } from "antd";
import type { AccountTransfer, TransferMovement } from "../../api/transfers";
import { formatMoney } from "../../presentation/money";
import { PanelSection } from "../shared/PanelStack";

function movementLabel(movement: TransferMovement) {
  return `${movement.account_name || "Conta"} · ${formatMoney(movement.amount.replace(/^-/, ""), movement.currency_code)}`;
}
export function TransferDetail({ transfer }: { transfer: AccountTransfer }) {
  return (
    <PanelSection title="Transferência">
      <Descriptions column={1} size="small" colon={false}>
        <Descriptions.Item label="Origem">{movementLabel(transfer.source)}</Descriptions.Item>
        <Descriptions.Item label="Destino">{movementLabel(transfer.destination)}</Descriptions.Item>
        <Descriptions.Item label="Taxa efetiva">
          {transfer.effective_rate
            ? `1 ${transfer.destination.currency_code} = ${transfer.effective_rate.replace(".", ",")} ${transfer.source.currency_code}`
            : "Indisponível após alteração dos valores"}
        </Descriptions.Item>
        <Descriptions.Item label="Tarifa registrada">{transfer.fee ? movementLabel(transfer.fee) : "Nenhuma"}</Descriptions.Item>
      </Descriptions>
      <Typography.Paragraph type="secondary">
        Os valores transferidos ficam fora das receitas e despesas. A tarifa registrada conta como despesa.
      </Typography.Paragraph>
    </PanelSection>
  );
}
