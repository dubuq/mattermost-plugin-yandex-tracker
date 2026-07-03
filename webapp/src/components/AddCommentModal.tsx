import { useState, useEffect } from 'react';

import { getStrings } from '../i18n';
import { PLUGIN_ID } from '../constants';
import type { Status } from '../types';

// Module-level bridge: the action callback sets this, the root modal reads it.
// Needed because the dropdown unmounts when closed, killing any local state.
let _open: ((postId: string) => void) | null = null;

// Called by the MM post dropdown action callback (registered in index.tsx).
export const openAddCommentModal = (postId: string) => _open?.(postId);

// Registered as a root component — stays mounted for the lifetime of the app.
export const AddCommentModal = () => {
    const [postId, setPostId] = useState<string | null>(null);
    const [issueKey, setIssueKey] = useState('');
    const [status, setStatus] = useState<Status>('idle');
    const [errorMsg, setErrorMsg] = useState('');

    useEffect(() => {
        _open = (id: string) => {
            setPostId(id);
            setIssueKey('');
            setStatus('idle');
            setErrorMsg('');
        };
        return () => {
            _open = null;
        };
    }, []);

    if (!postId) {
        return null;
    }

    const t = getStrings();
    const close = () => setPostId(null);

    const submit = async () => {
        const key = issueKey.trim().toUpperCase();
        if (!key) {
            return;
        }
        setStatus('loading');
        try {
            const resp = await fetch(`/plugins/${PLUGIN_ID}/add-comment`, {
                method: 'POST',
                // X-Requested-With is required for MM to pass the session to
                // the plugin (Mattermost-User-Id header) on cookie-auth requests.
                headers: { 'Content-Type': 'application/json', 'X-Requested-With': 'XMLHttpRequest' },
                body: JSON.stringify({ post_id: postId, issue_key: key }),
            });
            const data = await resp.json() as { ok: boolean; error?: string };
            if (data.ok) {
                setStatus('done');
                setTimeout(close, 1500);
            } else {
                setStatus('error');
                setErrorMsg(data.error ?? 'Unknown error');
            }
        } catch {
            setStatus('error');
            setErrorMsg(t.networkError);
        }
    };

    const busy = status === 'loading' || status === 'done';

    return (
        <div
            style={{
                position: 'fixed',
                inset: 0,
                background: 'rgba(0,0,0,0.5)',
                zIndex: 9999,
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
            }}
            onClick={(e) => { if (e.target === e.currentTarget) close(); }}
        >
            <div style={{
                background: 'var(--center-channel-bg)',
                color: 'var(--center-channel-color)',
                borderRadius: 8,
                padding: 24,
                width: 360,
                boxShadow: '0 8px 32px rgba(0,0,0,0.4)',
            }}>
                <h3 style={{ margin: '0 0 16px', fontSize: 16, fontWeight: 600, color: 'var(--center-channel-color)' }}>
                    {t.modalTitle}
                </h3>
                <input
                    type="text"
                    placeholder={t.placeholder}
                    value={issueKey}
                    onChange={(e) => setIssueKey(e.target.value)}
                    onKeyDown={(e) => {
                        if (e.key === 'Enter') { void submit(); }
                        if (e.key === 'Escape') { close(); }
                    }}
                    // eslint-disable-next-line jsx-a11y/no-autofocus
                    autoFocus
                    disabled={busy}
                    style={{
                        width: '100%',
                        padding: '8px 10px',
                        fontSize: 14,
                        border: '1px solid var(--center-channel-color-24)',
                        borderRadius: 4,
                        boxSizing: 'border-box',
                        marginBottom: 12,
                        background: 'var(--center-channel-bg)',
                        color: 'var(--center-channel-color)',
                    }}
                />
                {status === 'error' && (
                    <p style={{ color: 'var(--error-text)', fontSize: 13, margin: '0 0 12px' }}>{errorMsg}</p>
                )}
                {status === 'done' && (
                    <p style={{ color: 'var(--online-indicator)', fontSize: 13, margin: '0 0 12px' }}>
                        {t.successMsg(issueKey.trim().toUpperCase())}
                    </p>
                )}
                <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8 }}>
                    <button
                        onClick={close}
                        disabled={status === 'loading'}
                        style={{
                            padding: '6px 14px',
                            cursor: 'pointer',
                            borderRadius: 4,
                            border: '1px solid var(--center-channel-color-24)',
                            background: 'transparent',
                            color: 'var(--center-channel-color)',
                            fontSize: 14,
                        }}
                    >
                        {t.cancelButton}
                    </button>
                    <button
                        onClick={() => { void submit(); }}
                        disabled={busy || !issueKey.trim()}
                        style={{
                            padding: '6px 14px',
                            cursor: 'pointer',
                            borderRadius: 4,
                            border: 'none',
                            background: 'var(--button-bg)',
                            color: 'var(--button-color)',
                            fontSize: 14,
                            opacity: (busy || !issueKey.trim()) ? 0.6 : 1,
                        }}
                    >
                        {status === 'loading' ? t.addingButton : t.addButton}
                    </button>
                </div>
            </div>
        </div>
    );
};
