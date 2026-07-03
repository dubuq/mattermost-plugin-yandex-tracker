# Mattermost Yandex Tracker Plugin

[![Release](https://img.shields.io/github/v/release/dubuq/mattermost-plugin-yandex-tracker)](https://github.com/dubuq/mattermost-plugin-yandex-tracker/releases/latest)

Brings Yandex Tracker issues into Mattermost — inline preview cards that update in place, webhook-driven notifications, and write actions without leaving the conversation.

Tested with Mattermost Server 9.x and Yandex Tracker Cloud and legacy 360 organizations.

> 🇷🇺 Пошаговая инструкция по установке на русском - в разделе [Инструкция по установке (RU)](#инструкция-по-установке-ru).

## Feature summary

### Inline issue preview cards

Paste any Yandex Tracker issue key (e.g. `DEV-123`) or a full tracker URL into a message and a preview card appears inline, showing status, priority, type, and assignee. Cards are collapsed by default and can be expanded for full detail. Action buttons are always visible regardless of the card state.

![Collapsed issue card](assets/card-collapsed.png)

![Expanded issue card](assets/card-expanded.png)

### Real-time card updates

When an issue status changes in Yandex Tracker, the card on the original Mattermost message updates in place — no new bot message, no noise. Comment and assignment events post a notification reply in the issue's thread.

![Bot notification reply in thread](assets/notification-thread.png)

### Write actions

- **Assign to me** — assigns the issue to your Yandex Tracker account directly from Mattermost
- **Change Status** — opens a transition dialog; if the transition requires additional fields (e.g. resolution), a second dialog collects them
- **Add to Tracker issue** — post any Mattermost message as a comment on an issue via the `...` post menu

Write actions are always performed with the **user's own Yandex Tracker account**, never the plugin's service account. Each user connects once with `/tracker connect` (per-user OAuth); until then, clicking a write action shows a prompt to connect, and ephemeral cards from `/tracker ISSUE-1` hide the write buttons entirely. This requires the **Yandex OAuth Client ID/Secret** plugin settings (see setup below).

![Change Status transition dialog](assets/change-status-dialog.png)

![Fill Fields dialog for required transition fields](assets/fill-fields-dialog.png)

![Add to Tracker issue post button](assets/add-to-tracker-button.png)

![Add to Tracker issue post dialog](assets/add-to-tracker-dialog.png)

### Queue subscriptions

Subscribe a channel to a Yandex Tracker queue — new issues created in that queue automatically post a preview card in the channel.

```
/tracker subscribe DEV
/tracker unsubscribe DEV
/tracker subscriptions
```

![Auto-posted card from queue subscription](assets/subscription-card.png)

### Slash command

```
/tracker DEV-123
```

Posts an ephemeral preview card for any issue (only visible to you).

---

## Getting started

### Prerequisites

- Mattermost Server **9.0** or later (self-hosted)
- Yandex Tracker access — Cloud org or legacy Yandex 360 org both supported
- A Yandex **OAuth token** and your **Organization ID** for read access (see below)
- A Yandex **OAuth application** (Client ID + Secret) for per-user write actions — each user connects their own account via `/tracker connect` (see [Per-user OAuth app](#per-user-oauth-app-for-write-actions))

### 1. Get your Yandex credentials

#### OAuth token

1. Go to [oauth.yandex.ru](https://oauth.yandex.ru) and create an application
2. Under **Platforms**, tick **Web services** and set the callback URL to `https://oauth.yandex.ru/verification_code`
3. Under **Access**, enable `tracker:read` and `tracker:write` under **Yandex.Tracker**
4. Save the app and copy the **Client ID**
5. Open the following URL in your browser and authorize:
   ```
   https://oauth.yandex.ru/authorize?response_type=token&client_id=YOUR_CLIENT_ID
   ```
6. Copy the token from the URL fragment (`access_token=...`). It looks like `y0_AgAA...` and is valid for 1 year.

> **IAM token alternative:** Run `yc iam create-token` with the Yandex Cloud CLI. Valid for 12 hours.

#### Organization ID

1. Go to [console.cloud.yandex.ru](https://console.cloud.yandex.ru)
2. Open **Organization settings** and copy the **Organization ID** (looks like `bpf...`)

> **Legacy organizations:** If your Tracker predates Yandex Cloud, you may have a numeric org ID sent via `X-Org-Id` instead of `X-Cloud-Org-Id`. The plugin detects which to use based on the ID format.

Verify your credentials before setting up the plugin:

```bash
curl -s \
  -H "Authorization: OAuth YOUR_TOKEN" \
  -H "X-Cloud-Org-Id: YOUR_ORG_ID" \
  "https://api.tracker.yandex.net/v2/issues/YOUR_ISSUE_KEY" | jq .
```

### 2. Build and install

```bash
git clone https://github.com/dubuq/mattermost-plugin-yandex-tracker
cd mattermost-plugin-yandex-tracker
make bundle
# creates: com.yandex-tracker-mattermost-<version>.tar.gz
```

In Mattermost: **System Console → Plugin Management → Upload Plugin** → upload the `.tar.gz` → enable the plugin.

Make sure **Enable Plugin Uploads** is `true` in **System Console → Plugin Management**.

### 3. Configure the plugin

Go to **System Console → Plugins → Yandex Tracker** and fill in:

| Field                    | Value                                                                         |
| ------------------------ | ----------------------------------------------------------------------------- |
| **Yandex Tracker Token** | OAuth token (`y0_AgAA...`)                                                    |
| **Organization ID**      | Org ID (`bpf...`)                                                             |
| **Yandex OAuth Client ID / Secret** | Credentials of a Yandex OAuth app for per-user connections (see below) |
| **Webhook Secret**       | Any strong random string — `openssl rand -hex 32`                             |
| **Bot Display Name**     | `Tracker Bot` (or any name you prefer)                                        |
| **Monitor All Channels** | Disabled (default) — add the bot to channels you want monitored               |
| **Background Refresh**   | Every 6 hours (default) — re-fetches all tracked issues as a webhook fallback |

Click **Test Connection** to verify credentials, then **Save**.

#### Per-user OAuth app (for write actions)

Write actions (assign, status changes, comments) require each user to connect their own Yandex account. Set up one OAuth application for this:

1. Go to [oauth.yandex.ru](https://oauth.yandex.ru) and create an application (you can reuse the app from step 1, but the Redirect URI must be changed)
2. Under **Platforms**, tick **Web services** and set the Redirect URI to:
   ```
   https://YOUR-MATTERMOST-SERVER/plugins/com.yandex-tracker-mattermost/oauth/complete
   ```
3. Under **Access**, enable `tracker:read` and `tracker:write`
4. Copy the **ClientID** and **Client secret** into the plugin settings (**Yandex OAuth Client ID** / **Yandex OAuth Client Secret**) and save

Users then run `/tracker connect` in any channel, approve access on the Yandex consent page, and get a confirmation DM from the bot. `/tracker disconnect` removes the connection. Tokens are stored encrypted (AES-256-GCM) in the plugin's KV store and refreshed automatically when they expire — users only need to reconnect if they revoke the app's access in Yandex.

### 4. Add the bot to channels

With **Monitor All Channels** disabled (recommended), the plugin only monitors channels where `@tracker-bot` is a member. Add the bot the same way you'd add any user:

1. Open the channel → **Members** → **Add Members**
2. Search for `tracker-bot` → Add

> The bot must be a member of the team before it can be added to individual channels. If you don't see it, go to **System Console → User Management → Teams** → add tracker-bot to the team first.

### 5. Set up webhooks

The webhook URL is shown in **System Console → Plugins → Yandex Tracker**. In Yandex Tracker, create a trigger for each event type you want:

**Status change** (updates the card in place):

```json
{ "key": "{{issue.key}}", "type": "issueUpdated" }
```

**New comment** (bot replies in the issue thread):

```json
{
  "key": "{{issue.key}}",
  "type": "commentCreated",
  "author": "{{comment.author.display}}"
}
```

**Assignee changed** (bot replies in the issue thread):

```json
{
  "key": "{{issue.key}}",
  "type": "issueAssigned",
  "assignee": "{{issue.assignee.display}}"
}
```

**Issue created** (posts card in subscribed channels):

```json
{ "key": "{{issue.key}}", "type": "issueCreated" }
```

Set `Content-Type: application/json` and `X-Webhook-Secret: <your secret>` as headers on each trigger.

---

## Configuration reference

### Status colors

Cards display a colored left border based on the issue's status. Configure which statuses map to which color under **Card Color** settings. Each color slot has two fields:

| Field                      | Example value                             |
| -------------------------- | ----------------------------------------- |
| **Card Color — Active**    | `#1E88E5`                                 |
| **Statuses — Active**      | `In Progress, In Review, В работе`        |
| **Card Color — Done**      | `#43A047`                                 |
| **Statuses — Done**        | `Closed, Resolved, Закрыт, Решён`         |
| **Card Color — Cancelled** | `#E53935`                                 |
| **Statuses — Cancelled**   | `Cancelled, Won't fix, Отменён, Дубликат` |
| **Card Color — Default**   | `#AAAAAA`                                 |
| **Card Color — Custom**    | `#9C27B0` (optional 5th slot)             |
| **Statuses — Custom**      | `On Hold, На паузе`                       |

Status names are matched **case-insensitively**. The Default color applies to any status not matched by the other groups. Leave a color field blank to use the Mattermost theme default.

To find the exact status names for your Tracker, open any issue in Yandex Tracker and note the status label as shown in the UI — use those exact strings in the comma-separated lists.

### Required fields for transitions

Some status transitions in Yandex Tracker require additional fields before they can execute (e.g. closing an issue requires choosing a resolution). Configure these under **Required Fields by Transition** as a JSON object.

#### Format

```json
{
  "Transition Name": {
    "fieldApiKey": {
      "Display Label": "apiValue",
      "Display Label 2": "apiValue2"
    },
    "anotherField": {}
  }
}
```

- The top-level key is the **transition name** as shown in the Change Status dialog (matched case-insensitively)
- Each field entry is either:
  - A **non-empty object** `{"Label": "apiKey", ...}` — renders as a dropdown; the label is shown to the user, the API key is sent to Tracker
  - An **empty object** `{}` — renders as a free-text input; the value is sent as-is

#### Finding field API keys and values

1. In Yandex Tracker, open an issue and manually execute the transition you want to configure
2. In your browser DevTools → Network tab, find the `POST /v2/issues/<KEY>/transitions/<id>/_execute` request
3. The request body shows the field keys and values Tracker expects (e.g. `{"resolution": {"key": "fixed"}}`)

#### Example — closing with resolution

Yandex Tracker's "Close" transition requires a `resolution` field. The available resolutions for your queue can be found in **Queue settings → Resolutions**.

```json
{
  "Закрыть": {
    "resolution": {
      "Решено": "fixed",
      "Не будет исправлено": "wontFix",
      "Дубликат": "duplicate"
    }
  },
  "Close": {
    "resolution": {
      "Fixed": "fixed",
      "Won't fix": "wontFix",
      "Duplicate": "duplicate"
    }
  }
}
```

When a user clicks **Change Status** and selects "Закрыть", the plugin detects the required `resolution` field, shows an intermediate **Fill Fields** dialog with a dropdown of the three options, and then executes the transition with `{"resolution": {"key": "fixed"}}` (or whichever value was selected).

Transitions without an entry in this config execute immediately with no intermediate dialog.

---

## Usage

### Assign to me

Expand a card and click **Assign to me**. The issue is assigned to your connected Yandex Tracker account (run `/tracker connect` once beforehand). The assignment is made with your own token, so Tracker records you as the author of the change.

### Change Status

Expand a card and click **Change Status** to see available transitions for the current issue state. If a transition requires additional fields (e.g. a resolution when closing), a second **Fill Fields** dialog appears automatically. After submitting, the card updates to reflect the new status.

### Add to Tracker issue

Hover over any Mattermost message → click `...` → **Add to Tracker issue**. A dialog asks for the issue key (`DEV-123`). The message text is posted as a comment on that Tracker issue, authored by your connected account. Useful for pushing AI-generated thread summaries or decisions from a Mattermost discussion directly into the relevant issue.

### Slash commands

| Command                      | Description                                                            |
| ---------------------------- | ---------------------------------------------------------------------- |
| `/tracker KEY`               | Post an ephemeral preview card for any issue (only visible to you)     |
| `/tracker connect`           | Connect your personal Yandex Tracker account (required for write actions) |
| `/tracker disconnect`        | Remove your personal Tracker connection                                |
| `/tracker subscribe QUEUE`   | Subscribe the current channel to a queue — new issues auto-post a card |
| `/tracker unsubscribe QUEUE` | Remove the channel's subscription to a queue                           |
| `/tracker subscriptions`     | List all queues the current channel is subscribed to                   |

### Card lifecycle

Cards that have not received a status update for **7 days** are automatically removed from tracking. If a card stops updating, paste the issue key again to re-track it. This keeps the plugin's storage and background API usage bounded over time.

---

## Development

This plugin contains both a server (Go) and webapp (React/TypeScript) portion. Read the Mattermost documentation about [Developer Workflow](https://developers.mattermost.com/integrate/plugins/developer-workflow/) and [Developer Setup](https://developers.mattermost.com/integrate/plugins/developer-setup/) for more information.

### Local setup

```bash
# Server
go build ./server/...

# Webapp
cd webapp && yarn && yarn build

# Full plugin bundle
make bundle
```

To deploy directly to a local Mattermost instance:

```bash
export MM_SERVICESETTINGS_SITEURL=http://localhost:8065
export MM_ADMIN_USERNAME=admin
export MM_ADMIN_PASSWORD=admin
make deploy
```

For webhook testing without a public server, use [ngrok](https://ngrok.com):

```bash
ngrok http 8065
# Use the ngrok URL as your MM SiteURL and as the webhook URL in Yandex Tracker triggers
```

### Releasing new versions

This project uses [semantic versioning](https://semver.org):

- **Patch** `v0.1.0 → v0.1.1` — bug fixes
- **Minor** `v0.1.0 → v0.2.0` — new features, backwards compatible
- **Major** `v0.1.0 → v1.0.0` — breaking changes or significant milestone

To release:

1. Update `version` in `plugin.json`
2. Add an entry to `CHANGELOG.md`
3. `git tag v<version> && git push origin v<version>`
4. Create a GitHub release from the tag and attach the built `.tar.gz`

---

## Инструкция по установке (RU)

Пошаговая инструкция для администратора. Всюду ниже замените `YOUR-MATTERMOST-SERVER` на адрес вашего сервера Mattermost.

### Часть 1. Сервисный токен (для чтения задач)

Лучше делать всё из-под сервисного (робот-)аккаунта организации, а не личного - токен будет жить, даже если сотрудник уйдёт. У аккаунта должен быть доступ на чтение к очередям, задачи из которых будут показываться в Mattermost.

1. Зайдите на [oauth.yandex.ru](https://oauth.yandex.ru) под этим аккаунтом → "Создать приложение".
2. В разделе "Платформы" выберите "Веб-сервисы", Callback URI: `https://oauth.yandex.ru/verification_code`
3. В разделе "Доступы" → "Яндекс.Трекер" включите `tracker:read` и `tracker:write`.
4. Сохраните и скопируйте ClientID приложения.
5. Откройте в браузере (подставив ClientID из шага 4) и нажмите "Разрешить": `https://oauth.yandex.ru/authorize?response_type=token&client_id=CLIENT_ID_ИЗ_ШАГА_4`
6. После редиректа в адресной строке будет `access_token=y0_AgAA...` - скопируйте это значение. Это токен для чтения, действует 1 год.

Также понадобится ID организации: [console.cloud.yandex.ru](https://console.cloud.yandex.ru) → настройки организации → "Идентификатор организации" (вида `bpf...`). Если организация старая (Яндекс 360 без Cloud) - ID будет числовой, это нормально, подойдёт и он.

### Часть 2. OAuth-приложение для сотрудников (для действий от их имени)

Это второе, отдельное приложение - через него сотрудники будут подключать свои личные аккаунты Трекера к Mattermost командой `/tracker connect`.

1. Там же на [oauth.yandex.ru](https://oauth.yandex.ru) → "Создать приложение" (ещё одно).
2. "Платформы" → "Веб-сервисы", Redirect URI: `https://YOUR-MATTERMOST-SERVER/plugins/com.yandex-tracker-mattermost/oauth/complete`
3. "Доступы" → "Яндекс.Трекер": `tracker:read` и `tracker:write`.
4. Сохраните и скопируйте ClientID и Client secret.

### Часть 3. Установка плагина в Mattermost

1. Скачайте файл плагина `.tar.gz` из [releases](https://github.com/dubuq/mattermost-plugin-yandex-tracker/releases/latest) (или соберите сами: `make bundle`).
2. System Console → Plugin Management → убедитесь, что "Enable Plugin Uploads" включено (если нет - включите в config.json: `PluginSettings.EnableUploads: true` и перезапустите сервер).
3. Там же → "Upload Plugin" → загрузите файл `.tar.gz` → включите плагин (Enable).

### Часть 4. Настройка плагина

System Console → Plugins → Yandex Tracker, заполните поля:

- **Yandex Tracker Token** - токен из части 1 (`y0_AgAA...`)
- **Organization ID** - ID организации из части 1
- **Yandex OAuth Client ID** - ClientID приложения из части 2
- **Yandex OAuth Client Secret** - Client secret приложения из части 2
- **Webhook Secret** - любая случайная строка, например результат `openssl rand -hex 32`. Сохраните её - она понадобится при настройке триггеров в части 6.
- Остальные поля (Bot Display Name, Monitor All Channels, Background Refresh, цвета карточек) - можно оставить по умолчанию.

Нажмите "Test Connection" - должно показать успех. Затем "Save".

### Часть 5. Добавить бота в команду и каналы

1. System Console → User Management → Teams → выберите команду → добавьте пользователя `tracker-bot`.
2. В каждом канале, где нужны карточки: Участники → Добавить участников → `tracker-bot`. Плагин отслеживает только каналы, где бот состоит участником (если не включён Monitor All Channels).

### Часть 6. Триггеры в Яндекс.Трекере

Требуются права администратора очереди. В настройках очереди → "Триггеры" создайте HTTP-триггеры на адрес `https://YOUR-MATTERMOST-SERVER/plugins/com.yandex-tracker-mattermost/webhook` с заголовками `Content-Type: application/json` и `X-Webhook-Secret: <секрет из части 4>` - по одному на каждое событие:

- Изменение задачи: `{ "key": "{{issue.key}}", "type": "issueUpdated" }`
- Новый комментарий: `{ "key": "{{issue.key}}", "type": "commentCreated", "author": "{{comment.author.display}}" }`
- Смена исполнителя: `{ "key": "{{issue.key}}", "type": "issueAssigned", "assignee": "{{issue.assignee.display}}" }`
- Создание задачи (для подписок `/tracker subscribe`): `{ "key": "{{issue.key}}", "type": "issueCreated" }`

### Проверка

1. Вставьте ключ задачи (например `DEV-123`) в канал с ботом - появится карточка.
2. Выполните `/tracker connect` - после подтверждения на странице Яндекса бот пришлёт личное сообщение.
3. Поменяйте статус задачи в Трекере - карточка в Mattermost обновится сама в течение нескольких секунд.

---

## License

This repository is licensed under the [Apache 2.0 License](LICENSE).

---

## Help and support

- To report a bug, please [open an issue](https://github.com/dubuq/mattermost-plugin-yandex-tracker/issues)
- For feature requests, open an issue or submit a pull request
