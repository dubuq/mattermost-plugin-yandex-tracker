export const css = `
  /* Limit issue card width to match default MM link preview cards. */
  .post .attachment {
    max-width: 600px;
  }

  /* Adjust attachment fields labels */
  .post .attachment .attachment-fields .attachment-field__caption {
    padding-top: 6px;
    font-size: 12px;
  }

  /* Adjust action buttons placement */
  .post .attachment .attachment-actions {
    flex-wrap: wrap;
    gap: 8px;
    margin-top: 8px;
    padding: 0;
  }

  /* Compact action buttons — 24px height instead of the default 32px. */
  .post .attachment .attachment-actions button {
    height: 24px;
    padding: 0 8px;
    margin: 0;
    font-size: 12px;
  }

  /* Post dropdown menu item registered by this plugin. */
  .yt-plugin-menu-item {
    display: flex;
    align-items: center;
    gap: 10px;
    width: 100%;
    padding: 6px 17px;
    background: none;
    border: none;
    cursor: pointer;
    font-size: 14px;
    color: var(--center-channel-color);
    text-align: left;
    white-space: nowrap;
  }
  .yt-plugin-menu-item:hover {
    background: var(--center-channel-color-08);
  }
  .yt-plugin-menu-item__icon {
    display: flex;
    align-items: center;
    flex-shrink: 0;
  }
`;
