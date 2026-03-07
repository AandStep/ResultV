import React, { useCallback, useMemo, useState } from 'react';
import {
    View,
    Text,
    Pressable,
    TextInput,
    FlatList,
    StyleSheet,
    Modal,
} from 'react-native';
import { Server, Activity, Pencil, Trash2, Search, Plus, ChevronDown } from 'lucide-react-native';
import { useTranslation } from 'react-i18next';
import { FlagIcon } from '../components/ui/FlagIcon';
import { useConfigStore, ProxyItem } from '../store/configStore';
import { useConnectionStore } from '../store/connectionStore';
import { useLogStore } from '../store/logStore';
import { colors } from '../theme';

const ProxyCard = React.memo(
    ({
        proxy,
        isActive,
        ping,
        onPress,
        onEdit,
        onDelete,
        connectLabel,
        connectedLabel,
    }: {
        proxy: ProxyItem;
        isActive: boolean;
        ping: string;
        onPress: () => void;
        onEdit: () => void;
        onDelete: () => void;
        connectLabel: string;
        connectedLabel: string;
    }) => (
        <Pressable
            onPress={onPress}
            style={[styles.card, isActive && styles.cardActive]}
            android_ripple={{ color: colors.border }}>
            <View style={styles.cardTop}>
                <View style={styles.cardTopLeft}>
                    <View style={styles.flagWrap}>
                        <FlagIcon code={proxy.country} size={24} />
                    </View>
                    <View style={styles.cardInfo}>
                        <Text style={styles.cardName} numberOfLines={1}>{proxy.name}</Text>
                        <Text style={styles.cardAddr} numberOfLines={1}>
                            {proxy.ip}:{proxy.port}
                        </Text>
                    </View>
                </View>
                <View style={styles.typeBadge}>
                    <Text style={styles.typeText}>{proxy.type}</Text>
                </View>
            </View>

            <View style={styles.cardBottom}>
                <View style={styles.pingRow}>
                    <Activity
                        size={14}
                        color={
                            ping === 'Timeout' || ping === 'Error'
                                ? colors.error
                                : colors.textMuted
                        }
                    />
                    <Text
                        style={[
                            styles.pingText,
                            (ping === 'Timeout' || ping === 'Error') && { color: colors.error },
                        ]}>
                        {ping || '...'}
                    </Text>
                </View>
                <View style={styles.cardActions}>
                    <Pressable onPress={onEdit} style={styles.iconBtn} android_ripple={{ color: colors.border }}>
                        <Pencil size={16} color={colors.textSecondary} />
                    </Pressable>
                    <Pressable onPress={onDelete} style={styles.iconBtn} android_ripple={{ color: colors.error + '30' }}>
                        <Trash2 size={16} color={colors.textSecondary} />
                    </Pressable>
                    <Pressable
                        onPress={onPress}
                        style={[styles.connectBtn, isActive && styles.connectBtnActive]}
                        android_ripple={{ color: colors.primaryDark }}>
                        <Text
                            style={[styles.connectText, isActive && styles.connectTextActive]}>
                            {isActive ? connectedLabel : connectLabel}
                        </Text>
                    </Pressable>
                </View>
            </View>
        </Pressable>
    ),
);
ProxyCard.displayName = 'ProxyCard';

export const ProxyListScreen = ({ navigation }: any) => {
    const { t } = useTranslation();
    const [searchQuery, setSearchQuery] = useState('');
    const [sortMode, setSortMode] = useState<'default' | 'newest' | 'oldest' | 'country' | 'type'>('default');
    const proxies = useConfigStore(s => s.proxies);
    const setProxies = useConfigStore(s => s.setProxies);
    const setEditingProxy = useConfigStore(s => s.setEditingProxy);
    const routingRules = useConfigStore(s => s.routingRules);
    const settings = useConfigStore(s => s.settings);

    const isConnected = useConnectionStore(s => s.isConnected);
    const activeProxy = useConnectionStore(s => s.activeProxy);
    const pings = useConnectionStore(s => s.pings);
    const selectAndConnect = useConnectionStore(s => s.selectAndConnect);
    const deleteProxyAction = useConnectionStore(s => s.deleteProxy);
    const addLog = useLogStore(s => s.addLog);

    const filtered = useMemo(() => {
        let result = proxies;
        if (searchQuery) {
            const q = searchQuery.toLowerCase();
            result = result.filter(
                p => p.name.toLowerCase().includes(q) || p.ip.toLowerCase().includes(q),
            );
        }

        switch (sortMode) {
            case 'newest':
                result = [...result].sort((a, b) => b.id - a.id);
                break;
            case 'oldest':
                result = [...result].sort((a, b) => a.id - b.id);
                break;
            case 'country':
                result = [...result].sort((a, b) => (a.country || '').localeCompare(b.country || ''));
                break;
            case 'type':
                result = [...result].sort((a, b) => (a.type || '').localeCompare(b.type || ''));
                break;
        }

        return result;
    }, [proxies, searchQuery, sortMode]);

    const [isSortOpen, setIsSortOpen] = useState(false);
    const sortModes: Array<'default' | 'newest' | 'oldest' | 'country' | 'type'> = [
        'default',
        'newest',
        'oldest',
        'country',
        'type',
    ];

    const handleEdit = useCallback(
        (proxy: ProxyItem) => {
            setEditingProxy(proxy);
            navigation.navigate('AddProxy');
        },
        [setEditingProxy, navigation],
    );

    const handleDelete = useCallback(
        (id: number) => {
            deleteProxyAction(
                id,
                fn => {
                    const newProxies = fn(useConfigStore.getState().proxies);
                    useConfigStore.getState().setProxies(newProxies);
                },
                addLog,
            );
        },
        [deleteProxyAction, addLog],
    );

    const handleConnect = useCallback(
        (proxy: ProxyItem) => {
            selectAndConnect(proxy, routingRules, settings.killswitch, addLog);
        },
        [selectAndConnect, routingRules, settings.killswitch, addLog],
    );

    const renderItem = useCallback(
        ({ item }: { item: ProxyItem }) => (
            <ProxyCard
                proxy={item}
                isActive={isConnected && activeProxy?.id === item.id}
                ping={pings[item.id] || ''}
                onPress={() => handleConnect(item)}
                onEdit={() => handleEdit(item)}
                onDelete={() => handleDelete(item.id)}
                connectLabel={t('proxyList.status.connect')}
                connectedLabel={t('proxyList.status.connected')}
            />
        ),
        [isConnected, activeProxy, pings, handleConnect, handleEdit, handleDelete, t],
    );

    const keyExtractor = useCallback((item: ProxyItem) => String(item.id), []);

    const ListHeader = (
        <>
            <View style={styles.headerSection}>
                <Text style={styles.title}>{t('proxyList.title')}</Text>
                <Text style={styles.desc}>{t('proxyList.desc')}</Text>
            </View>

            {proxies.length > 0 && (
                <>
                    <View style={styles.searchSection}>
                        <View style={styles.searchBar}>
                            <Search size={18} color={colors.textMuted} />
                            <TextInput
                                placeholder={t('proxyList.searchPlaceholder')}
                                placeholderTextColor={colors.textMuted}
                                value={searchQuery}
                                onChangeText={setSearchQuery}
                                style={styles.searchInput}
                            />
                        </View>

                        <View style={styles.sortRow}>
                            <Text style={styles.sortLabel}>{t('proxyList.sortBy')}:</Text>
                            <Pressable
                                onPress={() => setIsSortOpen(true)}
                                style={styles.sortBtn}
                                android_ripple={{ color: colors.border }}>
                                <Text style={styles.sortBtnText}>{t(`proxyList.sort.${sortMode}`)}</Text>
                                <ChevronDown size={14} color={colors.textSecondary} />
                            </Pressable>
                        </View>
                    </View>

                    {/* Sort Dropdown Modal */}
                    <Modal
                        visible={isSortOpen}
                        transparent
                        animationType="fade"
                        onRequestClose={() => setIsSortOpen(false)}>
                        <Pressable style={styles.modalOverlay} onPress={() => setIsSortOpen(false)}>
                            <View style={styles.dropdownModal}>
                                {sortModes.map(mode => (
                                    <Pressable
                                        key={mode}
                                        style={[
                                            styles.dropdownModItem,
                                            sortMode === mode && styles.dropdownModItemActive,
                                        ]}
                                        android_ripple={{ color: colors.border }}
                                        onPress={() => {
                                            setSortMode(mode);
                                            setIsSortOpen(false);
                                        }}>
                                        <Text
                                            style={[
                                                styles.dropdownModText,
                                                sortMode === mode && styles.dropdownModTextActive,
                                            ]}>
                                            {t(`proxyList.sort.${mode}`)}
                                        </Text>
                                    </Pressable>
                                ))}
                            </View>
                        </Pressable>
                    </Modal>
                </>
            )}

            <Pressable
                onPress={() => {
                    setEditingProxy(null);
                    navigation.navigate('AddProxy');
                }}
                style={styles.addCard}
                android_ripple={{ color: colors.border }}>
                <View style={styles.addIcon}>
                    <Plus size={24} color={colors.textSecondary} />
                </View>
                <Text style={styles.addLabel}>{t('add.newServer')}</Text>
            </Pressable>
        </>
    );

    const EmptyList = (
        <View style={styles.empty}>
            <Server size={48} color={colors.borderLight} />
            <Text style={styles.emptyText}>{t('proxyList.empty')}</Text>
        </View>
    );

    return (
        <FlatList
            data={filtered}
            renderItem={renderItem}
            keyExtractor={keyExtractor}
            ListHeaderComponent={ListHeader}
            ListEmptyComponent={proxies.length === 0 ? EmptyList : undefined}
            contentContainerStyle={styles.list}
            style={styles.scrollView}
            removeClippedSubviews
        />
    );
};

const styles = StyleSheet.create({
    scrollView: { flex: 1, backgroundColor: colors.bg },
    list: { padding: 16, gap: 16 },
    headerSection: { marginBottom: 8 },
    title: { fontSize: 30, lineHeight: 36, fontWeight: '700', color: colors.text },
    desc: { fontSize: 16, lineHeight: 24, color: colors.textSecondary, marginTop: 6 },

    searchSection: { marginBottom: 16, gap: 12 },
    searchBar: {
        flexDirection: 'row',
        alignItems: 'center',
        backgroundColor: colors.card,
        borderWidth: 1,
        borderColor: colors.border,
        borderRadius: 12,
        paddingHorizontal: 14,
        gap: 10,
    },
    searchInput: { flex: 1, color: colors.text, fontSize: 16, lineHeight: 24, paddingVertical: 12 },
    sortRow: { flexDirection: 'row', alignItems: 'center', gap: 10 },
    sortLabel: { fontSize: 14, lineHeight: 20, color: colors.textSecondary },
    sortBtn: { flexDirection: 'row', alignItems: 'center', paddingHorizontal: 12, paddingVertical: 8, backgroundColor: colors.card, borderWidth: 1, borderColor: colors.border, borderRadius: 10, gap: 8 },
    sortBtnText: { fontSize: 14, lineHeight: 20, color: colors.text, fontWeight: '500' },

    addCard: {
        backgroundColor: colors.card + '80',
        borderWidth: 1,
        borderColor: colors.border,
        borderStyle: 'dashed',
        borderRadius: 24,
        padding: 24,
        alignItems: 'center',
        justifyContent: 'center',
        minHeight: 100,
    },
    addIcon: {
        width: 44,
        height: 44,
        borderRadius: 12,
        backgroundColor: colors.border + '80',
        borderWidth: 1,
        borderColor: colors.borderLight + '80',
        alignItems: 'center',
        justifyContent: 'center',
        marginBottom: 12,
    },
    addLabel: { fontSize: 20, lineHeight: 28, fontWeight: '700', color: colors.textSecondary },

    card: {
        backgroundColor: colors.card,
        padding: 20,
        borderRadius: 24,
        borderWidth: 1,
        borderColor: colors.border,
    },
    cardActive: { borderColor: colors.primaryLight, elevation: 4 },
    cardTop: {
        flexDirection: 'row',
        justifyContent: 'space-between',
        alignItems: 'flex-start',
        marginBottom: 16,
        gap: 12,
    },
    cardTopLeft: { flexDirection: 'row', alignItems: 'center', gap: 14, flex: 1 },
    flagWrap: {
        width: 44,
        height: 44,
        borderRadius: 12,
        backgroundColor: colors.border + '80',
        borderWidth: 1,
        borderColor: colors.borderLight + '80',
        alignItems: 'center',
        justifyContent: 'center',
    },
    cardInfo: { flex: 1 },
    cardName: { fontSize: 20, lineHeight: 28, fontWeight: '700', color: colors.text },
    cardAddr: { fontSize: 14, lineHeight: 20, color: colors.textSecondary, fontFamily: 'monospace', marginTop: 3 },
    typeBadge: { backgroundColor: colors.border, paddingHorizontal: 8, paddingVertical: 4, borderRadius: 6 },
    typeText: { fontSize: 12, lineHeight: 16, fontWeight: '500', color: colors.textSecondary },

    cardBottom: { flexDirection: 'row', justifyContent: 'space-between', alignItems: 'center', gap: 12 },
    pingRow: { flexDirection: 'row', alignItems: 'center', gap: 4 },
    pingText: { fontSize: 12, lineHeight: 16, color: colors.textMuted },
    cardActions: { flexDirection: 'row', alignItems: 'center', gap: 8 },
    iconBtn: {
        padding: 10,
        backgroundColor: colors.border,
        borderRadius: 12,
    },
    connectBtn: {
        paddingHorizontal: 18,
        paddingVertical: 8,
        borderRadius: 12,
        backgroundColor: colors.primary + '15',
    },
    connectBtnActive: { backgroundColor: colors.primaryLight },
    connectText: { fontSize: 14, lineHeight: 20, fontWeight: '500', color: colors.primaryLight },
    connectTextActive: { color: colors.bg, fontWeight: '700' },

    empty: { alignItems: 'center', paddingVertical: 60, gap: 16 },
    emptyText: { fontSize: 16, lineHeight: 24, color: colors.textSecondary },

    modalOverlay: {
        flex: 1,
        backgroundColor: 'rgba(0,0,0,0.4)',
        justifyContent: 'center',
        alignItems: 'center',
        padding: 24,
    },
    dropdownModal: {
        width: '100%',
        maxWidth: 320,
        backgroundColor: colors.card,
        borderWidth: 1,
        borderColor: colors.border,
        borderRadius: 16,
        padding: 8,
        elevation: 10,
    },
    dropdownModItem: {
        paddingVertical: 14,
        paddingHorizontal: 16,
        borderRadius: 10,
    },
    dropdownModItemActive: {
        backgroundColor: colors.primaryLight + '20',
    },
    dropdownModText: {
        fontSize: 16,
        lineHeight: 24,
        color: colors.text,
        fontWeight: '500',
    },
    dropdownModTextActive: {
        color: colors.primaryLight,
        fontWeight: '700',
    },
});
