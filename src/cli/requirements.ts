import {
    ensureApiAccessConfigurationIsAvailable,
} from '../config/require-api-access.js';

export type ActionFn = (...args: any[]) => void | Promise<void>;

export function requireApiAccess(fn: ActionFn): ActionFn {
    return async (...args: any[]) => {
        await ensureApiAccessConfigurationIsAvailable();
        return fn(...args);
    };
}
