import {stringify} from 'yaml';

import { getConfiguration } from './index.js';

export async function show(): Promise<void> {
    const configuration = await getConfiguration();
    console.log(stringify(configuration));
}
