import crypto from 'react-native-quick-crypto';
import { Buffer } from '@craftzdog/react-native-buffer';
import DeviceInfo from 'react-native-device-info';
import AsyncStorage from '@react-native-async-storage/async-storage';

const FALLBACK_ID_KEY = 'resultProxy_machine-fallback-id';

class CryptoService {
    private machineId: string = '';
    private encryptionKey: Buffer = Buffer.alloc(0);

    constructor() {
        // We'll initialize it asynchronously since device ID fetching can be async 
        // and we need to handle potential fallbacks from AsyncStorage
    }

    async init() {
        if (this.encryptionKey.length > 0) return;

        this.machineId = await this.getHardwareId();
        this.encryptionKey = crypto
            .createHash('sha256')
            .update(this.machineId + '_ResultProxy_SafeVault_v1')
            .digest();
    }

    private async getHardwareId(): Promise<string> {
        try {
            const id = await DeviceInfo.getUniqueId();
            if (id && id !== 'unknown') return id;
        } catch (e) {
            // Unhandled
        }

        return await this._getOrCreateFallbackId();
    }

    private async _getOrCreateFallbackId(): Promise<string> {
        try {
            const stored = await AsyncStorage.getItem(FALLBACK_ID_KEY);
            if (stored && stored.length >= 32) return stored;

            const newId = crypto.randomUUID();
            await AsyncStorage.setItem(FALLBACK_ID_KEY, newId);
            return newId;
        } catch (e) {
            return crypto.randomUUID();
        }
    }

    encrypt(data: any): string {
        try {
            if (this.encryptionKey.length === 0) {
                // Not initialized yet? Encryption will fail or be skipped if we follow the PC logic
                // But in React Native we'll probably need it early. 
                // We'll assume init() is called by the store before persistence starts.
            }

            const iv = crypto.randomBytes(16);
            const cipher = crypto.createCipheriv(
                'aes-256-gcm',
                this.encryptionKey,
                iv,
            );

            let encrypted = cipher.update(JSON.stringify(data), 'utf8', 'base64');
            encrypted += cipher.final('base64');
            const authTag = cipher.getAuthTag();

            return JSON.stringify(
                {
                    _isSecure: true,
                    iv: iv.toString('base64'),
                    data: encrypted,
                    authTag: authTag.toString('base64'),
                },
                null,
                2,
            );
        } catch (e) {
            return JSON.stringify(data, null, 2);
        }
    }

    decrypt(rawStr: string | null): any {
        if (!rawStr) return null;
        try {
            const parsed = JSON.parse(rawStr);
            if (parsed._isSecure && parsed.data && parsed.iv && parsed.authTag) {
                const decipher = crypto.createDecipheriv(
                    'aes-256-gcm',
                    this.encryptionKey,
                    Buffer.from(parsed.iv, 'base64'),
                );
                decipher.setAuthTag(Buffer.from(parsed.authTag, 'base64'));

                let decrypted = decipher.update(parsed.data, 'base64', 'utf8');
                decrypted += decipher.final('utf8');

                return JSON.parse(decrypted);
            }
            return parsed;
        } catch (e) {
            return null;
        }
    }
}

export const cryptoService = new CryptoService();
