import { homedir } from 'node:os';
import fs from 'node:fs/promises';
import path from 'node:path';

import { abortExecution } from '../../errors.js';
import { Context } from './types.js';

const configDir = path.join(homedir(), '.dash0');
const contextsFile = path.join(configDir, 'contexts.json');
const activeContextFile = path.join(configDir, 'activeContext');

export async function addContext(context: Context): Promise<void> {
    const contexts = await getContexts();

    const updatedContexts = contexts.filter(p => p.name !== context.name).concat(context);

    await writeContexts(updatedContexts);
}

export async function removeContext(contextName: string): Promise<void> {
    const contexts = await getContexts();

    const updatedContexts = contexts.filter(p => p.name !== contextName);

    await writeContexts(updatedContexts);
}

export async function getContexts(): Promise<Context[]> {
    await ensureConfigDirectoryExists();

    let fileContent: string;
    try {
        fileContent = await fs.readFile(contextsFile, { encoding: 'utf8' });
    } catch (e) {
        if ((e as any)?.code === 'ENOENT') {
            return [];
        }

        throw abortExecution("Failed to read file '%s': %s", contextsFile, (e as Error)?.message ?? 'Unknown error');
    }

    try {
        return JSON.parse(fileContent);
    } catch (e) {
        throw abortExecution(
                "Failed to parse file '%s' as JSON: %s",
                contextsFile,
                (e as Error)?.message ?? 'Unknown error'
        );
    }
}

async function writeContexts(contexts: Context[]): Promise<void> {
    await ensureConfigDirectoryExists();

    try {
        await fs.writeFile(contextsFile, JSON.stringify(contexts, undefined, 2));
    } catch (e) {
        throw abortExecution("Failed to write to file '%s': %s", contextsFile, (e as Error)?.message ?? 'Unknown error');
    }
}

async function ensureConfigDirectoryExists() {
    await fs.mkdir(configDir, { recursive: true });
}

export async function getActiveContext(): Promise<Context | undefined> {
    await ensureConfigDirectoryExists();

    let activeContextName: string | undefined;
    try {
        activeContextName = await fs.readFile(activeContextFile, { encoding: 'utf8' });
        // Users opening and saving the file might end up adding a trailing new line character.
        activeContextName = activeContextName.trim();
    } catch (e) {
        if ((e as any)?.code !== 'ENOENT') {
            throw abortExecution("Failed to read file '%s': %s", contextsFile, (e as Error)?.message ?? 'Unknown error');
        }
    }

    const contexts = await getContexts();
    const activeContext: Context | undefined = contexts.find(p => p.name === activeContextName?.trim()) ?? contexts[0];
    return activeContext;
}

export async function setActiveContext(contextName: string): Promise<void> {
    await ensureConfigDirectoryExists();

    try {
        await fs.writeFile(activeContextFile, contextName);
    } catch (e) {
        throw abortExecution(
                "Failed to write to file '%s': %s",
                activeContextFile,
                (e as Error)?.message ?? 'Unknown error'
        );
    }
}
