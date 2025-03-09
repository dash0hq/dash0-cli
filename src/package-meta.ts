import { fileURLToPath } from 'node:url';
import { join } from "node:path";
import { readFileSync } from "node:fs";

const __dirname = fileURLToPath(new URL('.', import.meta.url));
const packageJsonPath = join(__dirname, '..', 'package.json');
const packageJson = JSON.parse(readFileSync(packageJsonPath, 'utf-8'));

export const packageName = packageJson.name;
export const packageVersion = packageJson.version;
export const requiredNodejsVersion = packageJson.engines.node;
