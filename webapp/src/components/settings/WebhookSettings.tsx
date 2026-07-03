import { useState, useEffect } from 'react';

import { PLUGIN_ID } from '../../constants';
import type { TestResult, ClearResult } from '../../types';

// WebhookSettings renders inside the System Console plugin settings page.
// It displays the webhook URL (for copy-pasting into Yandex Tracker triggers)
// and a Test Connection button that verifies the configured credentials.
const WebhookSettings = () => {
    const [url, setUrl] = useState('');
    const [copied, setCopied] = useState(false);
    const [testing, setTesting] = useState(false);
    const [result, setResult] = useState<TestResult | null>(null);
    const [clearing, setClearing] = useState(false);
    const [clearResult, setClearResult] = useState<ClearResult | null>(null);

    useEffect(() => {
        fetch(`/plugins/${PLUGIN_ID}/webhook-url`, { headers: { 'X-Requested-With': 'XMLHttpRequest' } })
            .then((r) => r.json())
            .then((data) => setUrl(data.url ?? ''))
            .catch(() => setUrl('Failed to load URL'));
    }, []);

    const copy = () => {
        navigator.clipboard.writeText(url);
        setCopied(true);
        setTimeout(() => setCopied(false), 2000);
    };

    const test = async () => {
        setTesting(true);
        setResult(null);
        try {
            const r = await fetch(`/plugins/${PLUGIN_ID}/verify`, { method: 'POST', headers: { 'X-Requested-With': 'XMLHttpRequest' } });
            setResult(await r.json());
        } catch {
            setResult({ ok: false, error: 'Network error — could not reach the plugin.' });
        } finally {
            setTesting(false);
        }
    };

    const clearCache = async () => {
        setClearing(true);
        setClearResult(null);
        try {
            const r = await fetch(`/plugins/${PLUGIN_ID}/clear-cache`, { method: 'POST', headers: { 'X-Requested-With': 'XMLHttpRequest' } });
            if (!r.ok) {
                setClearResult({ ok: false, error: `Server returned ${r.status}` });
                return;
            }
            setClearResult(await r.json());
        } catch {
            setClearResult({ ok: false, error: 'Network error — could not reach the plugin.' });
        } finally {
            setClearing(false);
        }
    };

    return (
        <div style={{ maxWidth: '600px' }}>
            <label
                className='control-label'
                style={{ display: 'block', marginBottom: '6px' }}
            >
                {'Webhook URL'}
            </label>
            <div style={{ display: 'flex', gap: '8px', marginBottom: '12px' }}>
                <input
                    readOnly
                    value={url}
                    className='form-control'
                    style={{ fontFamily: 'monospace', fontSize: '13px' }}
                />
                <button
                    className='btn btn-default'
                    onClick={copy}
                    style={{ whiteSpace: 'nowrap' }}
                >
                    {copied ? 'Copied!' : 'Copy'}
                </button>
            </div>
            <p className='help-text' style={{ marginBottom: '12px' }}>
                {'Paste this URL into your Yandex Tracker trigger settings. Set the same webhook secret in both places.'}
            </p>
            <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
                <button
                    className='btn btn-default'
                    onClick={test}
                    disabled={testing}
                >
                    {testing ? 'Testing…' : 'Test Connection'}
                </button>
                {result && (
                    <span style={{ color: result.ok ? '#3c763d' : '#a94442' }}>
                        {result.ok ? '✓ Connection successful' : `✗ ${result.error}`}
                    </span>
                )}
            </div>
            <hr style={{ margin: '20px 0', borderColor: '#ddd' }} />
            <label className='control-label' style={{ display: 'block', marginBottom: '4px' }}>
                {'Cache Management'}
            </label>
            <p className='help-text' style={{ marginBottom: '12px' }}>
                {'Clears cached issue cards and post mappings. Queue subscriptions and user logins are preserved. Use this if cards stop updating or the plugin is showing stale data.'}
            </p>
            <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
                <button
                    className='btn btn-default'
                    onClick={clearCache}
                    disabled={clearing}
                >
                    {clearing ? 'Clearing…' : 'Clear Cache'}
                </button>
                {clearResult && (
                    <span style={{ color: clearResult.ok ? '#3c763d' : '#a94442' }}>
                        {clearResult.ok
                            ? `✓ Cleared ${clearResult.deleted} entries`
                            : `✗ ${clearResult.error}`}
                    </span>
                )}
            </div>
        </div>
    );
};

export default WebhookSettings;
