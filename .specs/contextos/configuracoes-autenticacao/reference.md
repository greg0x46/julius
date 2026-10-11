# Configurações / Autenticação

> Referência viva do comportamento implementado.

## Modelo

Uma conta proprietária, sem cadastro público ou dependência de provedor.
`internal/auth` persiste a conta e sessões em SQLite/Postgres; Argon2id
(19 MiB, duas iterações, paralelismo 1, parâmetros versionados) protege a senha.
E-mail é normalizado; senhas aceitam 15–128 caracteres sem composição obrigatória.

Sessões usam tokens aleatórios de 32 bytes, somente SHA-256 no banco e cookie
HttpOnly/SameSite=Lax/Path=/, sem Domain. HTTPS usa Secure e prefixo __Host-;
HTTP local usa nome sem prefixo. Prazo absoluto de sete dias e inatividade de
24 horas, atualizada em cada requisição autenticada. Consultas periódicas da
interface também contam como atividade. Expiração é validada pelo servidor.
Logout revoga uma sessão; troca/reset revogam todas. A revisão da conta também
invalida sessões criadas concorrentemente com troca/reset. Login limpa sessões
expiradas. Sessões sobrevivem a reinícios.

`internal/settings.Secrets` guarda a chave imutável usada para AES-256-GCM;
não é uma sessão de usuário. A chave externa é validada contra o verificador
mestre e todos os valores criptografados antes de iniciar HTTP e worker.
O worker não depende de login; espera quando a Pluggy ainda não foi configurada.
As funções de senha antiga em settings existem apenas para migração e testes.

## Configuração e comandos

- `JULIUS_PUBLIC_URL`: origem exata sem caminho, HTTPS em produção; HTTP
  somente para localhost/127.0.0.1/::1. Não deriva segurança de headers de proxy.
- Exatamente uma de `JULIUS_MASTER_KEY` e `JULIUS_MASTER_KEY_FILE`:
  chave aleatória de 32 bytes em Base64. Não há integração com nuvem.
- `julius auth init [-db ...]`: cria conta em banco sem configuração legada.
- `julius auth migrate [-db ...]`: com processo parado, pede senha antiga,
  e-mail e nova senha; recriptografa todos os segredos, cria conta e remove
  verificador legado numa transação. Exige backup prévio; recusa repetição.
- `julius auth reset-password [-db ...]`: redefine senha e revoga sessões;
  não precisa da chave nem a modifica. Senhas são lidas sem eco em terminal.

O servidor recusa configuração pendente/chave inválida. Chave perdida não é
recuperável pela senha; rotação da chave não está implementada. Instruções no README.

## HTTP

| Método / rota | Comportamento |
|---|---|
| POST /api/auth/login | Recebe email/password, cria cookie e retorna authenticated/email |
| GET /api/auth/session | Retorna authenticated e email quando autenticado; false sem sessão |
| POST /api/auth/logout | Revoga cookie atual, retorna authenticated=false |
| PUT /api/auth/password | Recebe current_password/password, revoga todas, retorna authenticated=false |
| PUT /api/settings/pluggy | Grava pluggy_client_id/pluggy_client_secret criptografados; pluggy_item_id opcional cria conexão na mesma transação; retorna saved=true |
| GET /api/settings/quotes | Retorna apenas enabled, time e timezone; nunca retorna o token |
| PUT /api/settings/quotes | Atualiza parcialmente enabled/time/timezone e brapi_token opcional, atomicamente; retorna saved=true |
| GET/PUT /api/preferences | Preferências existentes, agora autenticadas |

Todas as rotas /api são privadas por padrão, exceto os dois endpoints de
login/status. Ausência de sessão: 401; falha de banco: 503. Arquivos estáticos
e /health são públicos. Setup/unlock antigos não são registrados.
Escritas, inclusive login, exigem Origin configurada e X-Julius-Request: 1
(403 se inválidos), sem CORS. Respostas /api recebem Cache-Control: no-store.
Login e troca de senha compartilham limites de cinco tentativas/minuto por
e-mail e trinta globais, em memória limitada, e uma operação KDF simultânea.
Excesso/contenção: 429 e Retry-After. Mensagens de credenciais são genéricas;
KDF é executado também para e-mail incorreto. Corpo de autenticação limitado a 4 KiB.

## Frontend

AuthGate consulta a sessão, inclusive a cada minuto/foco, e mostra LoginPage
no mesmo caminho quando necessário. Transporte HTTP centralizado injeta cabeçalho
CSRF, usa cookies same-origin e combina cancelamento de chamada com o da sessão.
401 privado limpa sessão/cache e desmonta conteúdo; respostas/corpos pendentes
são descartados após logout. Login e senha incorretos mostram erro no formulário.
Configurações contém troca de senha, Sair e formulário write-only da Pluggy.

## Validação e limites

Testes cobrem migração/rollback nos dois dialetos, expiração, revogação,
reinício, cookies, CSRF, rate limiting, falhas de banco e cancelamento/cache.
Postgres usa JULIUS_TEST_POSTGRES_DSN com schema isolado por teste de auth.
A reivindicação de syncs pelo worker é coordenada entre instâncias via
Postgres (ver sincronização open banking); o rate limit continua por
instância. Sem múltiplos usuários, MFA ou recuperação por e-mail. Hospedagem e provisionamento ficam externos.

## Configuração de cotações

`enabled` é booleano, `time` exige `HH:MM` canônico (00:00–23:59) e
`timezone` exige zona carregável, como `America/Sao_Paulo`. Campos omitidos
preservam seus valores; `enabled: false` desabilita e `brapi_token: ""`
limpa o token. O token permanece criptografado e somente para escrita.
Uma atualização só de horário não exige nem altera a chave/token. Dados
inválidos recebem 422 e falhas de banco/chave recebem 503, sem gravação parcial.
GET e PUT exigem sessão; PUT mantém a proteção CSRF das demais escritas.
O padrão é habilitado, 19:00, America/Sao_Paulo. As alterações são aplicadas
pelo agendador em execução, inclusive a suspensão do backfill; a inicialização
legada e persistência estão descritas em Investimentos. GET também devolve
`last_refresh` (data, origem agendada/manual, status success/partial/deferred/
failed e contagens) ou null. POST /api/settings/quotes/refresh roda
RefreshAll na hora e devolve esse registro; outra atualização em andamento
recebe 409. Falhas de provedor não apagam cotações já gravadas. A tela de
ativos de investimento mostra agendamento, última atualização, "Atualizar
agora" e o token da brapi como configuração avançada.
