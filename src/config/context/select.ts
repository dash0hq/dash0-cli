import inquirer from 'inquirer';

import { setActiveContext, getContexts } from './service.js';
import { abortExecution } from '../../errors.js';

export async function select(): Promise<void> {
    const activeContextName = await promptContextSelection('Choose the new active context:');
    await setActiveContext(activeContextName);
}

export async function promptContextSelection(message: string): Promise<string> {
    const contexts = await getContexts();
    if (contexts.length === 0) {
        throw abortExecution('No contexts configured.');
    }

    const { activeContextName } = await inquirer.prompt([
        {
            type: 'list',
            name: 'activeContextName',
            message,
            choices: contexts.map(p => p.name),
        },
    ]);

    return activeContextName;
}
