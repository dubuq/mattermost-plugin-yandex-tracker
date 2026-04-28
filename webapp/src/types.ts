export type Status = 'idle' | 'loading' | 'done' | 'error';

export type UIStrings = {
    menuAction: string;
    modalTitle: string;
    placeholder: string;
    addButton: string;
    addingButton: string;
    cancelButton: string;
    successMsg: (key: string) => string;
    networkError: string;
};

export type TestResult = { ok: boolean; error?: string };
export type ClearResult = { ok: boolean; deleted?: number; error?: string };
