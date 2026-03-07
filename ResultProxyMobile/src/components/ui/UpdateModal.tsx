import React, { memo, useCallback } from 'react';
import {
    View,
    Text,
    Pressable,
    Modal,
    StyleSheet,
    Linking,
} from 'react-native';
import { DownloadCloud, X } from 'lucide-react-native';
import { useTranslation } from 'react-i18next';
import { colors } from '../../theme';

type Props = {
    visible: boolean;
    currentVersion: string;
    latestVersion?: string;
    downloadUrl?: string;
    onClose: () => void;
};

export const UpdateModal = memo(
    ({ visible, currentVersion, latestVersion, downloadUrl, onClose }: Props) => {
        const { t } = useTranslation();

        const handleDownload = useCallback(() => {
            if (downloadUrl) {
                Linking.openURL(downloadUrl);
            }
            onClose();
        }, [downloadUrl, onClose]);

        if (!latestVersion) return null;

        return (
            <Modal
                visible={visible}
                transparent
                animationType="fade"
                onRequestClose={onClose}>
                <View style={styles.overlay}>
                    <View style={styles.modal}>
                        <View style={styles.header}>
                            <View style={styles.headerLeft}>
                                <DownloadCloud size={24} color={colors.primaryLight} />
                                <Text style={styles.title}>
                                    {t('update.title', 'Доступно обновление')}
                                </Text>
                            </View>
                            <Pressable onPress={onClose} hitSlop={12}>
                                <X size={20} color={colors.textMuted} />
                            </Pressable>
                        </View>

                        <Text style={styles.message}>
                            {t(
                                'update.message',
                                'У вас установлена версия {{current}}, доступна новая версия {{latest}}.',
                                { current: currentVersion, latest: latestVersion },
                            )}
                        </Text>

                        <View style={styles.actions}>
                            <Pressable
                                onPress={onClose}
                                style={styles.laterBtn}
                                android_ripple={{ color: colors.border }}>
                                <Text style={styles.laterText}>
                                    {t('update.later', 'Позже')}
                                </Text>
                            </Pressable>
                            <Pressable
                                onPress={handleDownload}
                                style={styles.downloadBtn}
                                android_ripple={{ color: colors.primaryDark }}>
                                <Text style={styles.downloadText}>
                                    {t('update.download', 'Обновить')}
                                </Text>
                            </Pressable>
                        </View>
                    </View>
                </View>
            </Modal>
        );
    },
);

UpdateModal.displayName = 'UpdateModal';

const styles = StyleSheet.create({
    overlay: {
        flex: 1,
        backgroundColor: 'rgba(0,0,0,0.5)',
        justifyContent: 'center',
        alignItems: 'center',
        padding: 16,
    },
    modal: {
        width: '100%',
        maxWidth: 380,
        backgroundColor: colors.bg,
        borderWidth: 1,
        borderColor: colors.border,
        borderRadius: 16,
        padding: 24,
    },
    header: {
        flexDirection: 'row',
        justifyContent: 'space-between',
        alignItems: 'flex-start',
        marginBottom: 16,
    },
    headerLeft: {
        flexDirection: 'row',
        alignItems: 'center',
        gap: 12,
    },
    title: {
        fontSize: 20,
        lineHeight: 28,
        fontWeight: '700',
        color: colors.text,
    },
    message: {
        fontSize: 16,
        color: colors.textSecondary,
        marginBottom: 24,
        lineHeight: 24,
    },
    actions: {
        flexDirection: 'row',
        gap: 12,
    },
    laterBtn: {
        flex: 1,
        paddingVertical: 14,
        borderRadius: 12,
        backgroundColor: colors.card,
        borderWidth: 1,
        borderColor: colors.border,
        alignItems: 'center',
    },
    laterText: {
        fontSize: 14,
        lineHeight: 20,
        fontWeight: '700',
        color: colors.textSecondary,
    },
    downloadBtn: {
        flex: 1,
        paddingVertical: 14,
        borderRadius: 12,
        backgroundColor: colors.primary,
        alignItems: 'center',
    },
    downloadText: {
        fontSize: 14,
        lineHeight: 20,
        fontWeight: '700',
        color: colors.text,
    },
});
