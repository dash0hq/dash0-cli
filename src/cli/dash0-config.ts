#!/usr/bin/env node

import { Command } from 'commander';

import { show } from '../config/show.js';

const program = new Command();

program.command('show').description('Show the active CLI configuration. Warning: Prints secrets!').action(show);

program.parseAsync(process.argv);
