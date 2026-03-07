import React, { useCallback, useMemo } from 'react';
import { View, Text, FlatList, StyleSheet } from 'react-native';
import { useTranslation } from 'react-i18next';
import { useLogStore } from '../store/logStore';
import { colors } from '../theme';

type LogEntry = {
    timestamp: number;
    time: string;
    msg: string;
    type: string;
};

const translateLog = (log: string, t: any): string => {
    const map: Record<string, string> = {
        'Конфигурация успешно загружена.': t('logs.configLoaded'),
        'Служба недоступна. Используются базовые настройки.': t('logs.serviceUnavailable'),
        'Отключение...': t('logs.disconnecting'),
        'Отключено успешно.': t('logs.disconnected'),
        'Соединение установлено.': t('logs.connected'),
        'Связь с узлом восстановлена.': t('logs.nodeRestored'),
        'Служба недоступна.': t('logs.serviceDown'),
        'Kill Switch отключен пользователем.': t('logs.killSwitchDisabled'),
        'Ссылка скопирована.': t('logs.linkCopied'),
        'Промокод скопирован.': t('logs.promoCopied'),
    };
    for (const [key, value] of Object.entries(map)) {
        if (log.includes(key)) return value;
    }
    return log;
};

const LOG_BG: Record<string, string> = {
    info: colors.border + '40',
    success: colors.primary + '10',
    error: colors.error + '10',
    warning: '#ca8a04' + '10',
};

const LOG_BORDER: Record<string, string> = {
    info: colors.border,
    success: colors.primary + '30',
    error: colors.error + '30',
    warning: '#ca8a04' + '30',
};

const LOG_COLOR: Record<string, string> = {
    info: colors.textSecondary,
    success: colors.primaryLight,
    error: colors.errorLight,
    warning: '#facc15',
};

const LogItem = React.memo(({ item, t }: { item: LogEntry; t: any }) => (
    <View
        style={[
            styles.logItem,
            {
                backgroundColor: LOG_BG[item.type] || LOG_BG.info,
                borderLeftColor: LOG_BORDER[item.type] || LOG_BORDER.info,
            },
        ]}>
        <Text style={styles.logTime}>{item.time}</Text>
        <Text
            style={[
                styles.logMsg,
                { color: LOG_COLOR[item.type] || LOG_COLOR.info },
            ]}>
            {translateLog(item.msg, t)}
        </Text>
    </View>
));
LogItem.displayName = 'LogItem';

export const LogsScreen = () => {
    const { t } = useTranslation();
    const logs = useLogStore(s => s.logs);
    const backendLogs = useLogStore(s => s.backendLogs);

    const allLogs = useMemo(() => {
        const bl = (backendLogs || []).map(l => ({ ...l, timestamp: l.timestamp || 0 }));
        return [...logs, ...bl].sort((a, b) => b.timestamp - a.timestamp);
    }, [logs, backendLogs]);

    const renderItem = useCallback(
        ({ item }: { item: LogEntry }) => <LogItem item={item} t={t} />,
        [t],
    );

    const keyExtractor = useCallback(
        (item: LogEntry, index: number) => `${item.timestamp}-${index}`,
        [],
    );

    return (
        <View style={styles.container}>
            <View style={styles.headerSection}>
                <Text style={styles.title}>{t('logs.title')}</Text>
                <Text style={styles.desc}>
                    {t('logs.count', { count: allLogs.length })}
                </Text>
            </View>

            <FlatList
                data={allLogs}
                renderItem={renderItem}
                keyExtractor={keyExtractor}
                contentContainerStyle={styles.list}
                removeClippedSubviews
                initialNumToRender={15}
                maxToRenderPerBatch={10}
            />
        </View>
    );
};

const styles = StyleSheet.create({
    container: { flex: 1, backgroundColor: colors.bg, padding: 16, gap: 16 },
    headerSection: {},
    title: { fontSize: 30, lineHeight: 36, fontWeight: '700', color: colors.text },
    desc: { fontSize: 16, lineHeight: 24, color: colors.textSecondary, marginTop: 6 },

    list: { gap: 8, paddingBottom: 16 },
    logItem: {
        padding: 14,
        borderRadius: 12,
        borderLeftWidth: 3,
    },
    logTime: { fontSize: 12, lineHeight: 16, color: colors.textMuted, marginBottom: 4, fontFamily: 'monospace' },
    logMsg: { fontSize: 16, lineHeight: 24 },
});
