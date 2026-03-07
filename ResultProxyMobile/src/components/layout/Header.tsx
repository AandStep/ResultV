import React from 'react';
import { View, Text, Image, StyleSheet, Platform } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { useConnectionStore } from '../../store/connectionStore';
import { colors } from '../../theme';

export const Header = () => {
    const insets = useSafeAreaInsets();
    const isConnected = useConnectionStore(s => s.isConnected);
    const isProxyDead = useConnectionStore(s => s.isProxyDead);

    return (
        <View style={[styles.container, { paddingTop: insets.top }]}>
            <View style={styles.content}>
                <View style={styles.left}>
                    <Image
                        source={require('../../assets/logo.png')}
                        style={styles.logo}
                        resizeMode="contain"
                    />
                    <Text style={styles.title}>ResultProxy</Text>
                </View>
                {isConnected && (
                    <View
                        style={[
                            styles.statusBadge,
                            {
                                backgroundColor: isProxyDead ? colors.error : colors.primary,
                            },
                        ]}
                    />
                )}
            </View>
        </View>
    );
};

const styles = StyleSheet.create({
    container: {
        backgroundColor: colors.bg,
        borderBottomWidth: 1,
        borderBottomColor: colors.border,
    },
    content: {
        height: 60,
        flexDirection: 'row',
        alignItems: 'center',
        justifyContent: 'space-between',
        paddingHorizontal: 16,
    },
    left: {
        flexDirection: 'row',
        alignItems: 'center',
        gap: 10,
    },
    logo: {
        width: 28,
        height: 28,
    },
    title: {
        fontSize: 18,
        fontWeight: '700',
        color: colors.text,
    },
    statusBadge: {
        width: 12,
        height: 12,
        borderRadius: 6,
        shadowColor: colors.primary,
        shadowOffset: { width: 0, height: 0 },
        shadowOpacity: 0.5,
        shadowRadius: 5,
        elevation: 5,
    },
});
