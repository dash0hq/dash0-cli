import { promptContextSelection } from './select.js';
import { removeContext } from './service.js';

export async function remove(): Promise<void> {
    const contextName = await promptContextSelection('Choose profile to delete:');
    await removeContext(contextName);
}
