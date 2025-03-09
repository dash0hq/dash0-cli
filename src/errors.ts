import colors from 'colors/safe.js';
import { format } from 'node:util';

export interface AbortExecutionOptions {
    colorize?: boolean;
}

export function abortExecution(msg: string, ...args: unknown[]): Error {
    return abortExecutionWithOpts(undefined, msg, ...args);
}

export function abortExecutionWithOpts(
        { colorize = true }: AbortExecutionOptions = {},
        msg: string,
        ...args: unknown[]
): Error {
    // Make unit-testing easier by only aborting the process outside of Jest
    // https://jestjs.io/docs/environment-variables
    if (process.env.NODE_ENV !== 'test') {
        if (colorize) {
            msg = colors.red(msg);
        }
        console.error(msg, ...args);
        process.exit(1);
    }

    return new Error(format(msg, ...args));
}

export async function abortExecutionWithError(error: any, msg: string, ...args: unknown[]): Promise<Error> {
    let message = (error as Error)?.message ?? 'Unknown error';

    const errorBody = await getExecutionErrorBody(error);
    if (errorBody) {
        message = `${message}: ${JSON.stringify(errorBody, undefined, 2)}`;
    }

    return abortExecution(`${msg}: %s`, ...args, message);
}

export async function getExecutionErrorBody<T>(error: any): Promise<T | undefined> {
    if (typeof error?.response?.json === 'function') {
        try {
            return await (error.response as Response).json();
        } catch {
            // Ignore errors
        }
    }
}
