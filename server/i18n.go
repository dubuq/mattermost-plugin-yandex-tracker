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

	// Interactive dialog strings for Assign and Change Status actions.
	// AssignDialogTitle args: (issueKey)
	AssignDialogTitle  string
	AssignDialogSubmit string
	AssignLoginLabel   string
	AssignLoginHelp    string
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
		AssignDialogTitle:         "Assign %s to me",
		AssignDialogSubmit:        "Assign",
		AssignLoginLabel:          "Yandex Tracker Login",
		AssignLoginHelp:           "Your Yandex Tracker username (login). Saved after first use.",
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
		AssignDialogTitle:         "Назначить %s на меня",
		AssignDialogSubmit:        "Назначить",
		AssignLoginLabel:          "Логин в Яндекс Трекере",
		AssignLoginHelp:           "Ваш логин в Яндекс Трекере. Сохраняется после первого использования.",
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
		AssignDialogTitle:         "%s mir zuweisen",
		AssignDialogSubmit:        "Zuweisen",
		AssignLoginLabel:          "Yandex Tracker Login",
		AssignLoginHelp:           "Ihr Yandex Tracker Benutzername (Login). Wird nach der ersten Verwendung gespeichert.",
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
		AssignDialogTitle:         "M'assigner %s",
		AssignDialogSubmit:        "Assigner",
		AssignLoginLabel:          "Identifiant Yandex Tracker",
		AssignLoginHelp:           "Votre identifiant Yandex Tracker. Enregistré après la première utilisation.",
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
		AssignDialogTitle:         "Asignarme %s",
		AssignDialogSubmit:        "Asignar",
		AssignLoginLabel:          "Login de Yandex Tracker",
		AssignLoginHelp:           "Tu nombre de usuario en Yandex Tracker. Se guarda tras el primer uso.",
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
		AssignDialogTitle:         "Przypisz %s do mnie",
		AssignDialogSubmit:        "Przypisz",
		AssignLoginLabel:          "Login w Yandex Tracker",
		AssignLoginHelp:           "Twoja nazwa użytkownika w Yandex Tracker. Zapisywana po pierwszym użyciu.",
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
		AssignDialogTitle:         "Призначити %s на мене",
		AssignDialogSubmit:        "Призначити",
		AssignLoginLabel:          "Логін у Яндекс Трекері",
		AssignLoginHelp:           "Ваш логін у Яндекс Трекері. Зберігається після першого використування.",
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
		AssignDialogTitle:         "%s маған тағайындау",
		AssignDialogSubmit:        "Тағайындау",
		AssignLoginLabel:          "Яндекс Трекер логині",
		AssignLoginHelp:           "Яндекс Трекердегі пайдаланушы атыңыз. Бірінші пайдаланудан кейін сақталады.",
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
		AssignDialogTitle:         "%s ni menga belgilash",
		AssignDialogSubmit:        "Belgilash",
		AssignLoginLabel:          "Yandex Tracker logini",
		AssignLoginHelp:           "Yandex Treker foydalanuvchi nomingiz. Birinchi foydalanishdan keyin saqlanadi.",
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
		AssignDialogTitle:         "%s bana ata",
		AssignDialogSubmit:        "Ata",
		AssignLoginLabel:          "Yandex Tracker Girişi",
		AssignLoginHelp:           "Yandex Tracker kullanıcı adınız. İlk kullanımdan sonra kaydedilir.",
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
