import React, { useState, useEffect, useCallback } from 'react';
import {
    Modal,
    View,
    Text,
    TextInput,
    FlatList,
    Pressable,
    StyleSheet,
    Image,
    ActivityIndicator,
} from 'react-native';
import { Search, X, Box, RotateCw } from 'lucide-react-native';
import { useTranslation } from 'react-i18next';
import { getApps } from 'react-native-android-installed-apps-unblocking';
import { colors } from '../../theme';

interface AppInfo {
    packageName: string;
    versionName: string;
    versionCode: number;
    firstInstallTime: number;
    lastUpdateTime: number;
    appName: string;
    icon: string; // Base64 icon
}

interface AppPickerModalProps {
    visible: boolean;
    onClose: () => void;
    onSelect: (packageName: string) => void;
}

// Module-level cache to persist across modal re-opens
let cachedApps: AppInfo[] | null = null;

export const AppPickerModal: React.FC<AppPickerModalProps> = ({
    visible,
    onClose,
    onSelect,
}) => {
    const { t } = useTranslation();
    const [apps, setApps] = useState<AppInfo[]>(cachedApps || []);
    const [filteredApps, setFilteredApps] = useState<AppInfo[]>(cachedApps || []);
    const [loading, setLoading] = useState(false);
    const [search, setSearch] = useState('');

    const fetchApps = useCallback(async (forceRefresh = false) => {
        if (!forceRefresh && cachedApps) {
            setApps(cachedApps);
            setFilteredApps(cachedApps);
            return;
        }

        setLoading(true);
        try {
            const installedApps = await getApps();
            cachedApps = installedApps;
            setApps(installedApps);
            setFilteredApps(installedApps);
        } catch (error) {
            console.error('Failed to fetch apps:', error);
        } finally {
            setLoading(false);
        }
    }, []);

    useEffect(() => {
        if (visible && !cachedApps) {
            fetchApps();
        }
    }, [visible, fetchApps]);

    useEffect(() => {
        if (search.trim() === '') {
            setFilteredApps(apps);
        } else {
            const lowerSearch = search.toLowerCase();
            const filtered = apps.filter(
                (app) =>
                    app.appName.toLowerCase().includes(lowerSearch) ||
                    app.packageName.toLowerCase().includes(lowerSearch)
            );
            setFilteredApps(filtered);
        }
    }, [search, apps]);

    const renderItem = ({ item }: { item: AppInfo }) => (
        <Pressable
            style={styles.appItem}
            onPress={() => {
                onSelect(item.packageName);
                onClose();
            }}
            android_ripple={{ color: colors.borderLight }}>
            {item.icon ? (
                <Image
                    source={{ uri: `data:image/png;base64,${item.icon}` }}
                    style={styles.appIcon}
                />
            ) : (
                <View style={[styles.appIcon, styles.appIconPlaceholder]}>
                    <Box size={24} color={colors.textMuted} />
                </View>
            )}
            <View style={styles.appInfo}>
                <Text style={styles.appName} numberOfLines={1}>
                    {item.appName}
                </Text>
                <Text style={styles.packageName} numberOfLines={1}>
                    {item.packageName}
                </Text>
            </View>
        </Pressable>
    );

    return (
        <Modal
            visible={visible}
            animationType="slide"
            transparent={true}
            onRequestClose={onClose}>
            <View style={styles.modalOverlay}>
                <View style={styles.modalContent}>
                    <View style={styles.header}>
                        <View style={styles.headerLeft}>
                            <Text style={styles.title}>{t('rules.apps.choose_title')}</Text>
                            <Pressable
                                onPress={() => fetchApps(true)}
                                style={({ pressed }) => [styles.refreshBtn, pressed && { opacity: 0.7 }]}
                                disabled={loading}
                            >
                                <RotateCw size={20} color={loading ? colors.textMuted : colors.primaryLight} />
                            </Pressable>
                        </View>
                        <Pressable onPress={onClose} hitSlop={10} style={styles.closeBtn}>
                            <X size={24} color={colors.text} />
                        </Pressable>
                    </View>

                    <View style={styles.searchContainer}>
                        <Search size={20} color={colors.textMuted} style={styles.searchIcon} />
                        <TextInput
                            style={styles.searchInput}
                            placeholder={t('rules.apps.search_placeholder')}
                            placeholderTextColor={colors.textMuted}
                            value={search}
                            onChangeText={setSearch}
                            autoCapitalize="none"
                            autoCorrect={false}
                        />
                    </View>

                    {loading ? (
                        <View style={styles.loadingContainer}>
                            <ActivityIndicator size="large" color={colors.primaryLight} />
                            <Text style={styles.loadingText}>{t('rules.apps.loading')}</Text>
                        </View>
                    ) : (
                        <FlatList
                            data={filteredApps}
                            keyExtractor={(item) => item.packageName}
                            renderItem={renderItem}
                            contentContainerStyle={styles.listContent}
                            ListEmptyComponent={
                                <View style={styles.emptyContainer}>
                                    <Text style={styles.emptyText}>{t('rules.apps.no_apps')}</Text>
                                </View>
                            }
                        />
                    )}
                </View>
            </View>
        </Modal>
    );
};

const styles = StyleSheet.create({
    modalOverlay: {
        flex: 1,
        backgroundColor: 'rgba(0, 0, 0, 0.7)',
        justifyContent: 'flex-end',
    },
    modalContent: {
        backgroundColor: colors.bg,
        borderTopLeftRadius: 32,
        borderTopRightRadius: 32,
        height: '80%',
        paddingTop: 20,
    },
    header: {
        flexDirection: 'row',
        justifyContent: 'space-between',
        alignItems: 'center',
        paddingHorizontal: 24,
        marginBottom: 20,
    },
    headerLeft: {
        flexDirection: 'row',
        alignItems: 'center',
        gap: 12,
    },
    title: {
        fontSize: 24,
        fontWeight: '700',
        color: colors.text,
    },
    refreshBtn: {
        padding: 4,
    },
    closeBtn: {
        padding: 4,
    },
    searchContainer: {
        flexDirection: 'row',
        alignItems: 'center',
        backgroundColor: colors.card,
        borderRadius: 16,
        marginHorizontal: 20,
        paddingHorizontal: 16,
        marginBottom: 16,
        borderWidth: 1,
        borderColor: colors.border,
    },
    searchIcon: {
        marginRight: 12,
    },
    searchInput: {
        flex: 1,
        height: 50,
        color: colors.text,
        fontSize: 16,
    },
    listContent: {
        paddingHorizontal: 20,
        paddingBottom: 40,
    },
    appItem: {
        flexDirection: 'row',
        alignItems: 'center',
        paddingVertical: 12,
        paddingHorizontal: 4,
        borderBottomWidth: 1,
        borderBottomColor: colors.border + '50',
    },
    appIcon: {
        width: 48,
        height: 48,
        borderRadius: 12,
        marginRight: 16,
    },
    appIconPlaceholder: {
        backgroundColor: colors.card,
        justifyContent: 'center',
        alignItems: 'center',
    },
    appInfo: {
        flex: 1,
    },
    appName: {
        fontSize: 16,
        fontWeight: '600',
        color: colors.text,
        marginBottom: 2,
    },
    packageName: {
        fontSize: 13,
        color: colors.textMuted,
    },
    loadingContainer: {
        flex: 1,
        justifyContent: 'center',
        alignItems: 'center',
    },
    loadingText: {
        marginTop: 12,
        color: colors.textSecondary,
        fontSize: 16,
    },
    emptyContainer: {
        marginTop: 60,
        alignItems: 'center',
    },
    emptyText: {
        color: colors.textMuted,
        fontSize: 16,
    },
});
