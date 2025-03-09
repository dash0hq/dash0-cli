import colors from 'colors/safe.js';

import { getActiveContext, getContexts } from './service.js';

export async function list(): Promise<void> {
    const contexts = await getContexts();
    const activeContext = await getActiveContext();

    contexts
            .slice()
            .sort((a, b) => a.name.localeCompare(b.name))
            .forEach(p => {
                const isActive = p.name === activeContext?.name;

                if (isActive) {
                    console.log('* %s', colors.green(p.name));
                } else {
                    console.log('  %s', p.name);
                }
            });
}
