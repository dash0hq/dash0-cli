import {Configuration} from "../types.js";

export interface Context {
    name: string;
    configuration: Pick<Configuration, 'baseUrl' | 'authToken'>;
}
