#!/usr/bin/env node

import { Command, Option } from 'commander';
import { remove } from '../config/context/remove.js';
import { select } from '../config/context/select.js';
import { list } from '../config/context/list.js';
import { add } from '../config/context/add.js';

const program = new Command();

program
        .command('add')
        .description('Configure a new context (interactively or via options).')
        .addOption(new Option('-n, --name <name>', 'Name of the context'))
        .addOption(new Option('-b, --baseUrl <url>', 'Base URL to be used'))
        .addOption(new Option('-t, --token <token>', 'Auth token'))
        .action(add);
program.command('list').description('List all configured contexts.').action(list);
program.command('ls').description('Alias for list.').action(list);
program.command('remove').description('Interactively remove an existing context.').action(remove);
program.command('select').description('Interactively change the currently active context.').action(select);

program.parseAsync(process.argv);
