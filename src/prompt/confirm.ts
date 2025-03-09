import {stdout} from "node:process";
import inquirer from 'inquirer';

export interface ConfirmOptions {
    defaultYes?: boolean;
    defaultWhenNonInteractive?: boolean;
}

export async function confirm(
        message: string,
        { defaultYes = true, defaultWhenNonInteractive = true }: ConfirmOptions = {}
): Promise<boolean> {
    if (!stdout.isTTY) {
        return defaultWhenNonInteractive;
    }

    const answers = await inquirer.prompt([
        {
            type: 'confirm',
            name: 'confirm',
            message,
            default: defaultYes,
        },
    ]);

    return answers.confirm;
}
