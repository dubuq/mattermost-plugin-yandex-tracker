import type { UIStrings } from './types';

const translations: Record<string, UIStrings> = {
    en: {
        menuAction:   'Add to Tracker issue',
        modalTitle:   'Add to Tracker issue',
        placeholder:  'Issue key — e.g. DEV-123',
        addButton:    'Add',
        addingButton: 'Adding…',
        cancelButton: 'Cancel',
        successMsg:   (key) => `Comment added to ${key}`,
        networkError: 'Network error — could not reach the plugin.',
    },
    ru: {
        menuAction:   'Добавить к задаче',
        modalTitle:   'Добавить к задаче в Tracker',
        placeholder:  'Ключ задачи — напр. DEV-123',
        addButton:    'Добавить',
        addingButton: 'Добавление…',
        cancelButton: 'Отмена',
        successMsg:   (key) => `Комментарий добавлен к ${key}`,
        networkError: 'Ошибка сети — плагин недоступен.',
    },
    de: {
        menuAction:   'Zu Tracker-Aufgabe hinzufügen',
        modalTitle:   'Zu Tracker-Aufgabe hinzufügen',
        placeholder:  'Aufgabenschlüssel — z.B. DEV-123',
        addButton:    'Hinzufügen',
        addingButton: 'Wird hinzugefügt…',
        cancelButton: 'Abbrechen',
        successMsg:   (key) => `Kommentar zu ${key} hinzugefügt`,
        networkError: 'Netzwerkfehler — Plugin nicht erreichbar.',
    },
    fr: {
        menuAction:   "Ajouter à l'issue Tracker",
        modalTitle:   "Ajouter à l'issue Tracker",
        placeholder:  "Clé de l'issue — ex. DEV-123",
        addButton:    'Ajouter',
        addingButton: 'Ajout…',
        cancelButton: 'Annuler',
        successMsg:   (key) => `Commentaire ajouté à ${key}`,
        networkError: "Erreur réseau — impossible d'atteindre le plugin.",
    },
    es: {
        menuAction:   'Añadir a issue de Tracker',
        modalTitle:   'Añadir a issue de Tracker',
        placeholder:  'Clave del issue — ej. DEV-123',
        addButton:    'Añadir',
        addingButton: 'Añadiendo…',
        cancelButton: 'Cancelar',
        successMsg:   (key) => `Comentario añadido a ${key}`,
        networkError: 'Error de red — no se puede conectar al plugin.',
    },
    pl: {
        menuAction:   'Dodaj do zadania w Tracker',
        modalTitle:   'Dodaj do zadania w Tracker',
        placeholder:  'Klucz zadania — np. DEV-123',
        addButton:    'Dodaj',
        addingButton: 'Dodawanie…',
        cancelButton: 'Anuluj',
        successMsg:   (key) => `Komentarz dodany do ${key}`,
        networkError: 'Błąd sieci — nie można połączyć się z wtyczką.',
    },
    uk: {
        menuAction:   'Додати до задачі',
        modalTitle:   'Додати до задачі в Tracker',
        placeholder:  'Ключ задачі — напр. DEV-123',
        addButton:    'Додати',
        addingButton: 'Додавання…',
        cancelButton: 'Скасувати',
        successMsg:   (key) => `Коментар додано до ${key}`,
        networkError: 'Помилка мережі — плагін недоступний.',
    },
    kz: {
        menuAction:   'Тапсырмаға қосу',
        modalTitle:   'Tracker тапсырмасына қосу',
        placeholder:  'Тапсырма кілті — мыс. DEV-123',
        addButton:    'Қосу',
        addingButton: 'Қосылуда…',
        cancelButton: 'Болдырмау',
        successMsg:   (key) => `${key} тапсырмасына пікір қосылды`,
        networkError: 'Желі қатесі — плагинге қосылу мүмкін емес.',
    },
    uz: {
        menuAction:   "Tracker vazifasiga qo'shish",
        modalTitle:   "Tracker vazifasiga qo'shish",
        placeholder:  'Vazifa kaliti — mas. DEV-123',
        addButton:    "Qo'shish",
        addingButton: "Qo'shilmoqda…",
        cancelButton: 'Bekor qilish',
        successMsg:   (key) => `${key} ga izoh qo'shildi`,
        networkError: "Tarmoq xatosi — plaginга ulanib bo'lmadi.",
    },
    tr: {
        menuAction:   'Tracker görevine ekle',
        modalTitle:   'Tracker görevine ekle',
        placeholder:  'Görev anahtarı — örn. DEV-123',
        addButton:    'Ekle',
        addingButton: 'Ekleniyor…',
        cancelButton: 'İptal',
        successMsg:   (key) => `${key} görevine yorum eklendi`,
        networkError: 'Ağ hatası — eklentiye ulaşılamıyor.',
    },
};

// Locale override set from the MM Redux store in index.tsx — more reliable than
// document.documentElement.lang which may not be set yet in the desktop app.
let _locale = '';
export const setLocale = (locale: string) => { _locale = locale.slice(0, 2).toLowerCase(); };

export const getLang = (): string => {
    const raw = _locale || document.documentElement.lang || navigator.language || 'en';
    const lang = raw.slice(0, 2).toLowerCase();
    return lang in translations ? lang : 'en';
};

export const getStrings = (): UIStrings => translations[getLang()];

export const getMenuActionLabel = (): string => getStrings().menuAction;
