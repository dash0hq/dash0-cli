import colors from 'colors/safe.js';

import { abortExecutionWithOpts } from '../errors.js';
import { getConfiguration } from './index.js';

const configurationMissingHelp = `
No auth token or base URL configuration was found for Dash0 API access.
You can configure both through configuration contexts or
environment variables (${colors.bold('DASH0_AUTH_TOKEN')} / ${colors.bold('DASH0_URL')}). We recommend
configuration contexts for local CLI usage.
You can add a configuration context via:

                  ${colors.bold('dash0 context add')}
`.trim();

export async function ensureApiAccessConfigurationIsAvailable() {
    const config = await getConfiguration();

    if (!config.baseUrl || !config.authToken) {
        throw abortExecutionWithOpts({ colorize: false }, configurationMissingHelp);
    }
}
