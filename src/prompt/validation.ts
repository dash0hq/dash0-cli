export function validateNotBlank(input: string): boolean {
    return input != null && input.trim().length > 0;
}

export function validateHttpUrl(input: string): boolean | string {
    try {
        const url = new URL(input);
        if (url.protocol !== 'http:' && url.protocol !== 'https:') {
            return `Unsupported protocol ${url.protocol}. Only http: and https: are supported.`;
        }
        return true;
    } catch {
        return 'Invalid URL. Please specify an absolute URL.';
    }
}
