import AsyncStorage from '@react-native-async-storage/async-storage';

const DEFAULT_DAEMON_URL = 'http://127.0.0.1:14080';

let cachedToken: string | null = null;
let cachedUrl: string | null = null;

export const getBaseUrl = async (): Promise<string> => {
    if (cachedUrl) return cachedUrl;
    cachedUrl = (await AsyncStorage.getItem('daemonUrl')) ?? DEFAULT_DAEMON_URL;
    return cachedUrl;
};

export const setBaseUrl = async (url: string): Promise<void> => {
    cachedUrl = url;
    await AsyncStorage.setItem('daemonUrl', url);
};

export const setApiToken = async (token: string): Promise<void> => {
    cachedToken = token;
    await AsyncStorage.setItem('apiToken', token);
};

export const apiFetch = async (
    endpoint: string,
    options: RequestInit = {},
): Promise<Response> => {
    if (!cachedToken) {
        cachedToken = (await AsyncStorage.getItem('apiToken')) ?? '';
    }
    const baseUrl = await getBaseUrl();

    return fetch(`${baseUrl}${endpoint}`, {
        ...options,
        headers: {
            ...options.headers,
            Authorization: `Bearer ${cachedToken}`,
        },
    });
};
