# Read your own financial data with Meu Pluggy

Use Meu Pluggy to authorize your banks, then the Pluggy developer Application
to give Julius API access. You need a running Julius instance and its owner
login; see [Getting started](../getting-started.md).
Instructions are in English; Julius button names below match its Portuguese UI.

## 1. Create your personal account

Create a free account at [Meu Pluggy](https://meu.pluggy.ai).
This path is for your own financial data. See the official
[Meu Pluggy API access guide](https://meu.pluggy.ai/api-guide).

## 2. Connect your banks in Meu Pluggy

Inside Meu Pluggy, connect each bank and complete its authorization, including
approval in the bank's app when requested. Wait until its data is available.
Bank consent is managed here.

## 3. Select a developer Application

Sign in to the [Pluggy Dashboard](https://dashboard.pluggy.ai) with the same
account. Under **Applications**, create or select the Application Julius will
use. Keep all Julius connections in that Application.

## 4. Connect MeuPluggy to the Application

Open **Applications → your application → Demo → Connect Account**.
Choose **MeuPluggy**, not the bank itself and not **Sandbox**, and sign in with
your Meu Pluggy account. Each bank needs authorization into the Application;
include every bank you want Julius to read, including banks added later.
This grants access to existing connections without repeating bank approval.

If you use custom connector selection, enable **MeuPluggy** under
**Customization → Connectors → Personal → Direct Connectors**, then return to
the Demo. Its Direct badge describes the MeuPluggy connector, not a separate
connection to your bank. See the official
[personal-use tutorial](https://docs.pluggy.ai/en/docs/guides/meu-pluggy-personal-use).

## 5. Save every Item ID and the Application credentials

On the Demo page, open each bank proxy Item under **Connected Items** and copy
its `itemId`. Save one ID per bank with its name. Personal-use accounts cannot
list Items with `GET /v2/items`; keep these IDs for later access.

Copy the Application's **Client ID** from **Applications**. The **Client Secret**
appears only at creation/generation: store it immediately in a password or
secrets manager. If lost, use **Generate new secret**; this invalidates the old
secret, so update Julius and any other integration using it. Never put secrets
in Git, screenshots, logs, or support messages. See
[Dashboard connectors, Item IDs, and credentials](https://docs.pluggy.ai/en/docs/guides/dashboard-connectors).

## 6. Configure and verify Julius

1. Sign in to Julius and open **Configurações → Open Banking**.
2. Expand **Credenciais da Pluggy**, enter **Client ID** and **Client secret**,
   and click **Salvar credenciais**. Leave the optional Item ID blank for now.
   Saved values are encrypted and are not displayed again; cleared fields after
   saving are expected. Saving does not test access to Pluggy.
3. Click **Adicionar conexão**, paste a bank's proxy `itemId` into
   **Item ID (Pluggy)**, optionally set **Apelido (opcional)**, and click
   **Adicionar**. Repeat for each bank, registering each ID once.
4. Keep the connections active. In a connection's **⋯** menu, choose
   **Sincronizar agora** to test it individually. The page action syncs all
   active connections.
5. Follow **Histórico de sincronizações** and open the run to inspect status,
   counts, and failures. Wait for **Concluída**, then check the imported accounts
   and transactions against Meu Pluggy. A saved connection alone does not
   confirm API access or that the bank shared every product.

## Refreshes and consent

Meu Pluggy refreshes the original bank connections approximately every 24 hours.
Proxy Items reflect those updates and cannot be manually refreshed through the
API. Julius's **Sincronizar agora** reads currently available data; it does not
force a new bank refresh. A proxy's `nextAutoSyncAt: null` is expected, as
explained in the [Meu Pluggy API guide](https://meu.pluggy.ai/api-guide).

Bank consent may expire or require action inside Meu Pluggy. Application access
also expires after one year: renew through **Demo → Connect Account → MeuPluggy**
and sign in again. Check the resulting Item IDs before continuing; they may
change. See [personal-use authorization renewal](https://docs.pluggy.ai/en/docs/guides/meu-pluggy-personal-use)
and the [Item lifecycle](https://docs.pluggy.ai/en/docs/connections/item).

## Common mistakes

- **Dashboard asks for CPF and bank approval again:** go back and choose
  **MeuPluggy**. Bank authorization belongs in Meu Pluggy.
- **MeuPluggy is missing:** enable it in the custom connector selection above.
  **Sandbox / Pluggy Bank** contains synthetic data, not your bank data.
- **Sync fails:** inspect the run's failures; check that both credentials and
  the proxy Item belong to the same Application, and that the secret was not
  rotated. Correct credentials under **Credenciais da Pluggy**, then retry.
- **Data is old or empty:** check the bank connection in Meu Pluggy and the
  Application authorization. An expired authorization can return empty data
  before the Item status changes; check `consentExpiresAt` in `GET /items/{id}`
  if needed. Repeated Julius syncs cannot renew consent or force bank updates.
