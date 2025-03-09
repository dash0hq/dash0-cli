import {getActiveContext} from './context/service.js';
import { Configuration } from './types.js';

export async function getConfiguration(): Promise<Configuration> {
    let configuration: Configuration;

    const context = await getActiveContext();
    if (context?.configuration) {
        configuration = context.configuration;
    } else {
        configuration = {
            authToken: undefined,
            baseUrl: undefined,
        };
    }

    // Environment arguments take precedence over the global system configuration.
    configuration.authToken = process.env.DASH0_AUTH_TOKEN ?? configuration.authToken;
    configuration.baseUrl = process.env.DASH0_URL ?? configuration.baseUrl;

    // A typical error case is that the baseUrl carries a trailing slash. This is fine
    // in our persisted config files, but we don't want our CLI to internally work with those.
    if (configuration.baseUrl?.endsWith('/')) {
        configuration.baseUrl = configuration.baseUrl.substring(0, configuration.baseUrl.length - 1);
    }

    return configuration;
}
