declare module 'react-native-android-installed-apps-unblocking' {
    export interface AppInfo {
        packageName: string;
        versionName: string;
        versionCode: number;
        firstInstallTime: number;
        lastUpdateTime: number;
        appName: string;
        icon: string;
        apkDir: string;
        size: number;
    }

    export function getApps(): Promise<AppInfo[]>;
    export function getNonSystemApps(): Promise<AppInfo[]>;
    export function getSystemApps(): Promise<AppInfo[]>;
}
