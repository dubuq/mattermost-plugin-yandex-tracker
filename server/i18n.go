package main

import "strings"

// Translations holds all user-visible strings rendered by the plugin.
// Add a new entry to supportedLanguages to support an additional language.
type Translations struct {
	StatusLabel    string
	PriorityLabel  string
	TypeLabel      string
	AssigneeLabel  string
	RefreshButton  string
	DismissButton  string
	CollapseButton string
	ExpandButton   string
	// Notification message templates — posted by the bot in the thread of the issue card.
	// CommentNotification args: (issueKey, author)
	// AssignmentNotification args: (issueKey, assignee)
	CommentNotification    string
	AssignmentNotification string

	// Write action buttons — shown in expanded cards only.
	AssignToMeButton   string
	ChangeStatusButton string

	// Per-user connection strings. Write actions are only performed with the
	// user's own Tracker account, never the service account.
	ConnectPrompt   string // shown when a non-connected user clicks a write action
	ReconnectPrompt string // shown when the stored token is rejected (expired/revoked)
	// AssignedToYou args: (issueKey); AssignFailed args: (issueKey, error)
	AssignedToYou string
	AssignFailed  string

	// Interactive dialog strings for the Change Status action.
	// StatusDialogTitle args: (issueKey)
	StatusDialogTitle  string
	StatusDialogSubmit string
	StatusFieldLabel   string

	// Fill-fields flow — shown when a transition requires additional fields.
	// FillFieldsPrompt args: (issueKey, transitionName)
	FillFieldsPrompt  string
	FillFieldsButton  string // button label on the ephemeral post
	// FillFieldsDialogTitle args: (issueKey, transitionName)
	FillFieldsDialogTitle  string
	FillFieldsDialogSubmit string
}

var supportedLanguages = map[string]Translations{
	"en": {
		StatusLabel:               "Status",
		PriorityLabel:             "Priority",
		TypeLabel:                 "Type",
		AssigneeLabel:             "Assignee",
		RefreshButton:             "Refresh",
		DismissButton:             "✕",
		CollapseButton:            "Collapse",
		ExpandButton:              "Expand",
		CommentNotification:       "New comment on %s by %s",
		AssignmentNotification:    "%s assigned to %s",
		AssignToMeButton:          "Assign to me",
		ChangeStatusButton:        "Change Status",
		ConnectPrompt:             "This action requires your personal Yandex Tracker account. Connect it by typing `/tracker connect`.",
		ReconnectPrompt:           "Your Yandex Tracker authorization has expired or was revoked. Reconnect by typing `/tracker connect`.",
		AssignedToYou:             "%s is now assigned to you.",
		AssignFailed:              "Failed to assign %s: %s",
		StatusDialogTitle:         "Change Status: %s",
		StatusDialogSubmit:        "Change",
		StatusFieldLabel:          "New Status",
		FillFieldsPrompt:          "Changing %s to \"%s\" requires additional fields.",
		FillFieldsButton:          "Fill Fields",
		FillFieldsDialogTitle:     "Fill Fields: %s → %s",
		FillFieldsDialogSubmit:    "Apply",
	},
	"ru": {
		StatusLabel:               "Статус",
		PriorityLabel:             "Приоритет",
		TypeLabel:                 "Тип",
		AssigneeLabel:             "Исполнитель",
		RefreshButton:             "Обновить",
		DismissButton:             "✕",
		CollapseButton:            "Свернуть",
		ExpandButton:              "Развернуть",
		CommentNotification:       "Новый комментарий к %s от %s",
		AssignmentNotification:    "%s назначен: %s",
		AssignToMeButton:          "Назначить мне",
		ChangeStatusButton:        "Изменить статус",
		ConnectPrompt:             "Для этого действия нужен ваш личный аккаунт Яндекс Трекера. Подключите его командой `/tracker connect`.",
		ReconnectPrompt:           "Авторизация в Яндекс Трекере истекла или была отозвана. Подключитесь заново командой `/tracker connect`.",
		AssignedToYou:             "%s теперь назначена на вас.",
		AssignFailed:              "Не удалось назначить %s: %s",
		StatusDialogTitle:         "Изменить статус: %s",
		StatusDialogSubmit:        "Изменить",
		StatusFieldLabel:          "Новый статус",
		FillFieldsPrompt:          "Смена статуса %s на \"%s\" требует дополнительных полей.",
		FillFieldsButton:          "Заполнить поля",
		FillFieldsDialogTitle:     "Дополнительные поля: %s → %s",
		FillFieldsDialogSubmit:    "Применить",
	},
	"de": {
		StatusLabel:               "Status",
		PriorityLabel:             "Priorität",
		TypeLabel:                 "Typ",
		AssigneeLabel:             "Bearbeiter",
		RefreshButton:             "Aktualisieren",
		DismissButton:             "✕",
		CollapseButton:            "Einklappen",
		ExpandButton:              "Ausklappen",
		CommentNotification:       "Neuer Kommentar zu %s von %s",
		AssignmentNotification:    "%s zugewiesen an %s",
		AssignToMeButton:          "Mir zuweisen",
		ChangeStatusButton:        "Status ändern",
		ConnectPrompt:             "Diese Aktion erfordert Ihr persönliches Yandex-Tracker-Konto. Verbinden Sie es mit `/tracker connect`.",
		ReconnectPrompt:           "Ihre Yandex-Tracker-Autorisierung ist abgelaufen oder wurde widerrufen. Verbinden Sie sich erneut mit `/tracker connect`.",
		AssignedToYou:             "%s ist Ihnen jetzt zugewiesen.",
		AssignFailed:              "Zuweisung von %s fehlgeschlagen: %s",
		StatusDialogTitle:         "Status ändern: %s",
		StatusDialogSubmit:        "Ändern",
		StatusFieldLabel:          "Neuer Status",
		FillFieldsPrompt:          "%s auf \"%s\" ändern erfordert zusätzliche Felder.",
		FillFieldsButton:          "Felder ausfüllen",
		FillFieldsDialogTitle:     "Felder ausfüllen: %s → %s",
		FillFieldsDialogSubmit:    "Anwenden",
	},
	"fr": {
		StatusLabel:               "Statut",
		PriorityLabel:             "Priorité",
		TypeLabel:                 "Type",
		AssigneeLabel:             "Responsable",
		RefreshButton:             "Actualiser",
		DismissButton:             "✕",
		CollapseButton:            "Réduire",
		ExpandButton:              "Développer",
		CommentNotification:       "Nouveau commentaire sur %s de %s",
		AssignmentNotification:    "%s assigné à %s",
		AssignToMeButton:          "M'assigner",
		ChangeStatusButton:        "Changer le statut",
		ConnectPrompt:             "Cette action nécessite votre compte Yandex Tracker personnel. Connectez-le avec `/tracker connect`.",
		ReconnectPrompt:           "Votre autorisation Yandex Tracker a expiré ou a été révoquée. Reconnectez-vous avec `/tracker connect`.",
		AssignedToYou:             "%s vous est maintenant assigné.",
		AssignFailed:              "Échec de l'assignation de %s : %s",
		StatusDialogTitle:         "Changer le statut : %s",
		StatusDialogSubmit:        "Changer",
		StatusFieldLabel:          "Nouveau statut",
		FillFieldsPrompt:          "Changer %s en \"%s\" nécessite des champs supplémentaires.",
		FillFieldsButton:          "Remplir les champs",
		FillFieldsDialogTitle:     "Champs requis : %s → %s",
		FillFieldsDialogSubmit:    "Appliquer",
	},
	"es": {
		StatusLabel:               "Estado",
		PriorityLabel:             "Prioridad",
		TypeLabel:                 "Tipo",
		AssigneeLabel:             "Asignado",
		RefreshButton:             "Actualizar",
		DismissButton:             "✕",
		CollapseButton:            "Contraer",
		ExpandButton:              "Expandir",
		CommentNotification:       "Nuevo comentario en %s de %s",
		AssignmentNotification:    "%s asignado a %s",
		AssignToMeButton:          "Asignarme",
		ChangeStatusButton:        "Cambiar estado",
		ConnectPrompt:             "Esta acción requiere tu cuenta personal de Yandex Tracker. Conéctala con `/tracker connect`.",
		ReconnectPrompt:           "Tu autorización de Yandex Tracker ha caducado o ha sido revocada. Vuelve a conectarte con `/tracker connect`.",
		AssignedToYou:             "%s ahora está asignado a ti.",
		AssignFailed:              "No se pudo asignar %s: %s",
		StatusDialogTitle:         "Cambiar estado: %s",
		StatusDialogSubmit:        "Cambiar",
		StatusFieldLabel:          "Nuevo estado",
		FillFieldsPrompt:          "Cambiar %s a \"%s\" requiere campos adicionales.",
		FillFieldsButton:          "Rellenar campos",
		FillFieldsDialogTitle:     "Campos requeridos: %s → %s",
		FillFieldsDialogSubmit:    "Aplicar",
	},
	"pl": {
		StatusLabel:               "Status",
		PriorityLabel:             "Priorytet",
		TypeLabel:                 "Typ",
		AssigneeLabel:             "Przypisany",
		RefreshButton:             "Odśwież",
		DismissButton:             "✕",
		CollapseButton:            "Zwiń",
		ExpandButton:              "Rozwiń",
		CommentNotification:       "Nowy komentarz do %s od %s",
		AssignmentNotification:    "%s przypisano do %s",
		AssignToMeButton:          "Przypisz do mnie",
		ChangeStatusButton:        "Zmień status",
		ConnectPrompt:             "Ta akcja wymaga Twojego osobistego konta Yandex Tracker. Połącz je komendą `/tracker connect`.",
		ReconnectPrompt:           "Twoja autoryzacja Yandex Tracker wygasła lub została cofnięta. Połącz się ponownie komendą `/tracker connect`.",
		AssignedToYou:             "%s jest teraz przypisane do Ciebie.",
		AssignFailed:              "Nie udało się przypisać %s: %s",
		StatusDialogTitle:         "Zmień status: %s",
		StatusDialogSubmit:        "Zmień",
		StatusFieldLabel:          "Nowy status",
		FillFieldsPrompt:          "Zmiana %s na \"%s\" wymaga dodatkowych pól.",
		FillFieldsButton:          "Wypełnij pola",
		FillFieldsDialogTitle:     "Wymagane pola: %s → %s",
		FillFieldsDialogSubmit:    "Zastosuj",
	},
	"uk": {
		StatusLabel:               "Статус",
		PriorityLabel:             "Пріоритет",
		TypeLabel:                 "Тип",
		AssigneeLabel:             "Виконавець",
		RefreshButton:             "Оновити",
		DismissButton:             "✕",
		CollapseButton:            "Згорнути",
		ExpandButton:              "Розгорнути",
		CommentNotification:       "Новий коментар до %s від %s",
		AssignmentNotification:    "%s призначено: %s",
		AssignToMeButton:          "Призначити мені",
		ChangeStatusButton:        "Змінити статус",
		ConnectPrompt:             "Для цієї дії потрібен ваш особистий акаунт Яндекс Трекера. Підключіть його командою `/tracker connect`.",
		ReconnectPrompt:           "Авторизація в Яндекс Трекері закінчилася або була відкликана. Підключіться знову командою `/tracker connect`.",
		AssignedToYou:             "%s тепер призначено на вас.",
		AssignFailed:              "Не вдалося призначити %s: %s",
		StatusDialogTitle:         "Змінити статус: %s",
		StatusDialogSubmit:        "Змінити",
		StatusFieldLabel:          "Новий статус",
		FillFieldsPrompt:          "Зміна %s на \"%s\" потребує додаткових полів.",
		FillFieldsButton:          "Заповнити поля",
		FillFieldsDialogTitle:     "Додаткові поля: %s → %s",
		FillFieldsDialogSubmit:    "Застосувати",
	},
	"kz": {
		StatusLabel:               "Мәртебе",
		PriorityLabel:             "Басымдылық",
		TypeLabel:                 "Түр",
		AssigneeLabel:             "Орындаушы",
		RefreshButton:             "Жаңарту",
		DismissButton:             "✕",
		CollapseButton:            "Жию",
		ExpandButton:              "Жаю",
		CommentNotification:       "%s бойынша %s жаңа пікір қалдырды",
		AssignmentNotification:    "%s орындаушысы: %s",
		AssignToMeButton:          "Маған тағайындау",
		ChangeStatusButton:        "Мәртебені өзгерту",
		ConnectPrompt:             "Бұл әрекет үшін жеке Яндекс Трекер аккаунтыңыз қажет. Оны `/tracker connect` пәрменімен қосыңыз.",
		ReconnectPrompt:           "Яндекс Трекердегі авторизация мерзімі өтті немесе кері қайтарылды. `/tracker connect` пәрменімен қайта қосылыңыз.",
		AssignedToYou:             "%s енді сізге тағайындалды.",
		AssignFailed:              "%s тағайындау сәтсіз аяқталды: %s",
		StatusDialogTitle:         "Мәртебені өзгерту: %s",
		StatusDialogSubmit:        "Өзгерту",
		StatusFieldLabel:          "Жаңа мәртебе",
		FillFieldsPrompt:          "%s мәртебесін \"%s\" ретінде өзгерту үшін қосымша өрістер қажет.",
		FillFieldsButton:          "Өрістерді толтыру",
		FillFieldsDialogTitle:     "Қосымша өрістер: %s → %s",
		FillFieldsDialogSubmit:    "Қолдану",
	},
	"uz": {
		StatusLabel:               "Holat",
		PriorityLabel:             "Ustuvorlik",
		TypeLabel:                 "Tur",
		AssigneeLabel:             "Ijrochi",
		RefreshButton:             "Yangilash",
		DismissButton:             "✕",
		CollapseButton:            "Yig'ish",
		ExpandButton:              "Ochish",
		CommentNotification:       "%s bo'yicha %s yangi izoh qoldirdi",
		AssignmentNotification:    "%s ijrochisi: %s",
		AssignToMeButton:          "Menga belgilash",
		ChangeStatusButton:        "Holat o'zgartirish",
		ConnectPrompt:             "Bu amal uchun shaxsiy Yandex Tracker akkauntingiz kerak. Uni `/tracker connect` buyrug'i bilan ulang.",
		ReconnectPrompt:           "Yandex Tracker avtorizatsiyasi muddati tugagan yoki bekor qilingan. `/tracker connect` buyrug'i bilan qayta ulaning.",
		AssignedToYou:             "%s endi sizga belgilandi.",
		AssignFailed:              "%s ni belgilash muvaffaqiyatsiz tugadi: %s",
		StatusDialogTitle:         "Holatni o'zgartirish: %s",
		StatusDialogSubmit:        "O'zgartirish",
		StatusFieldLabel:          "Yangi holat",
		FillFieldsPrompt:          "%s holatini \"%s\" ga o'zgartirish uchun qo'shimcha maydonlar talab qilinadi.",
		FillFieldsButton:          "Maydonlarni to'ldirish",
		FillFieldsDialogTitle:     "Qo'shimcha maydonlar: %s → %s",
		FillFieldsDialogSubmit:    "Qo'llash",
	},
	"tr": {
		StatusLabel:               "Durum",
		PriorityLabel:             "Öncelik",
		TypeLabel:                 "Tür",
		AssigneeLabel:             "Atanan",
		RefreshButton:             "Yenile",
		DismissButton:             "✕",
		CollapseButton:            "Daralt",
		ExpandButton:              "Genişlet",
		CommentNotification:       "%s için %s yorum yaptı",
		AssignmentNotification:    "%s atandı: %s",
		AssignToMeButton:          "Bana ata",
		ChangeStatusButton:        "Durumu değiştir",
		ConnectPrompt:             "Bu işlem kişisel Yandex Tracker hesabınızı gerektirir. `/tracker connect` komutuyla bağlayın.",
		ReconnectPrompt:           "Yandex Tracker yetkilendirmenizin süresi doldu veya iptal edildi. `/tracker connect` komutuyla yeniden bağlanın.",
		AssignedToYou:             "%s artık size atandı.",
		AssignFailed:              "%s ataması başarısız oldu: %s",
		StatusDialogTitle:         "Durumu değiştir: %s",
		StatusDialogSubmit:        "Değiştir",
		StatusFieldLabel:          "Yeni durum",
		FillFieldsPrompt:          "%s durumunu \"%s\" olarak değiştirmek ek alanlar gerektiriyor.",
		FillFieldsButton:          "Alanları Doldur",
		FillFieldsDialogTitle:     "Gerekli Alanlar: %s → %s",
		FillFieldsDialogSubmit:    "Uygula",
	},
}

// translations returns Translations based on the Mattermost server's default client locale.
// This is set by the admin in System Console → Site Configuration → Localization.
func (p *Plugin) translations() Translations {
	cfg := p.API.GetConfig()
	if cfg != nil && cfg.LocalizationSettings.DefaultClientLocale != nil {
		return translationsForLocale(*cfg.LocalizationSettings.DefaultClientLocale)
	}
	return supportedLanguages["en"]
}

// translationsForUser returns Translations in the given MM user's locale,
// falling back to English when the user cannot be loaded.
func (p *Plugin) translationsForUser(userID string) Translations {
	var locale string
	if user, appErr := p.API.GetUser(userID); appErr == nil {
		locale = user.Locale
	}
	return translationsForLocale(locale)
}

// translationsForLocale returns Translations for a raw locale string (e.g. "ru", "ru-RU").
// Falls back to English for unknown locales.
func translationsForLocale(locale string) Translations {
	lang := locale
	if len(lang) > 2 {
		lang = lang[:2]
	}
	if t, ok := supportedLanguages[strings.ToLower(lang)]; ok {
		return t
	}
	return supportedLanguages["en"]
}
