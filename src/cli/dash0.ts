#!/usr/bin/env node

import { Command } from 'commander';
import colors from 'colors/safe.js';
import { satisfies } from 'semver';
import process from 'node:process';
import {packageVersion, requiredNodejsVersion} from "../package-meta.js";

const actualNodejsVersion = process.version;

if (!satisfies(actualNodejsVersion, requiredNodejsVersion)) {
    const help = `
Node.js version ${actualNodejsVersion} is not supported. The Dash0 CLI
requires a Node.js version that satisfies the following version range:

                       ${colors.bold(requiredNodejsVersion)}

We recommend to install Node.js via a version manager. For example,
using the Node Version Manager (NVM):

               ${colors.bold('https://github.com/nvm-sh/nvm#readme')}
`;
    console.error(colors.red(help.trim()));
    process.exit(1);
}

new Command()
        // Prefer to load at runtime directly from the package.json to simplify
        // the TypeScript build. Without this, we would have to make the build
        // more complicated to adapt the root dir accordingly.
        // eslint-disable-next-line
        .version(packageVersion)
        .command('config', 'Show the CLI configuration.')
        .command('context', 'Manage configuration contexts.')
        .parseAsync(process.argv);
