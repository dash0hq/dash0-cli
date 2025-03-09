import colors from 'colors/safe.js';
import inquirer from 'inquirer';

import { validateNotBlank, validateHttpUrl } from '../../prompt/validation.js';
import { addContext } from './service.js';
import { Context } from './types.js';

const startHelp = `
Contexts enable you to use the CLI without repeatedly providing
passwords or having to remember environment variables. Configuration contexts
are stored in ~/.dash0/contexts.json
`.trim();

const finishHelp = `
${colors.green('Done!')} You can now start using the CLI.`.trim();

interface Options {
    name: string;
    baseUrl?: string;
    token: string;
}

export async function add(options: Options): Promise<void> {
    let context: Context | undefined;
    console.log(options);
    if (options?.name && options?.token) {
        context = {
            name: options.name,
            configuration: {
                baseUrl: options.baseUrl,
                authToken: options.token,
            }
        };
    } else {
        console.clear();
        console.log(startHelp);
        console.log();

        context = await ask();
    }
    await addContext(context);

    console.log();
    console.log(finishHelp);
}

async function ask(): Promise<Context> {
    const answers = await inquirer.prompt([
        {
            type: 'input',
            name: 'name',
            message: 'Context name:',
            validate: validateNotBlank,
        },
        {
            type: 'input',
            name: 'baseUrl',
            message: 'Base URL of the Dash0 API:',
            validate: validateHttpUrl,
        },
        {
            type: 'password',
            name: 'authToken',
            message: 'API access token:',
            validate: validateNotBlank,
        },
    ]);

    return {
        name: answers.name,
        configuration: {
            baseUrl: answers.baseUrl,
            authToken: answers.authToken,
        }
    };
}
