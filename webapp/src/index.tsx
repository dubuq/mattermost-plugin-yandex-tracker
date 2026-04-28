// Entry point for the Yandex Tracker plugin webapp.

import WebhookSettings from './components/settings/WebhookSettings';
import { AddCommentModal } from './components/AddCommentModal';
import { AddCommentMenuItem } from './components/AddCommentMenuItem';
import { setLocale } from './i18n';
import { PLUGIN_ID, STYLE_ID } from './constants';
import { css } from './styles';

const YandexTrackerPlugin = {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  initialize(registry: any, store: any) {
    // Read locale from the MM Redux store — more reliable than document.documentElement.lang
    // (which isn't set yet at plugin init time in the desktop app).
    const applyLocale = () => {
      try {
        const s = store.getState();
        const uid: string = s?.entities?.users?.currentUserId ?? '';
        const locale: string = s?.entities?.users?.profiles?.[uid]?.locale ?? '';
        if (locale) setLocale(locale);
      } catch { /* ignore — best-effort */ }
    };
    applyLocale();
    store.subscribe(applyLocale);

    const style = document.createElement('style');
    style.id = STYLE_ID;
    style.textContent = css;
    document.head.appendChild(style);

    // Render the webhook URL + test connection panel in System Console.
    registry.registerAdminConsoleCustomSetting('WebhookInfo', WebhookSettings, {
      showTitle: false,
    });

    registry.registerPostDropdownMenuComponent(AddCommentMenuItem);
    registry.registerRootComponent(AddCommentModal);

    void store;
  },

  uninitialize() {
    document.getElementById(STYLE_ID)?.remove();
  },
};

// @ts-ignore — window.registerPlugin is injected by Mattermost webapp
window.registerPlugin(PLUGIN_ID, YandexTrackerPlugin);
