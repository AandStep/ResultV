import React, { memo, useCallback } from 'react';
import { View, Text, Pressable, StyleSheet, Animated } from 'react-native';
import { colors } from '../../theme';

type Props = {
    title: string;
    description: string;
    isOn: boolean;
    onToggle: () => void;
};

export const SettingToggle = memo(({ title, description, isOn, onToggle }: Props) => {
    const handlePress = useCallback(() => onToggle(), [onToggle]);

    return (
        <Pressable
            onPress={handlePress}
            style={styles.container}
            android_ripple={{ color: colors.border }}>
            <View style={styles.textContainer}>
                <Text style={styles.title}>{title}</Text>
                <Text style={styles.description}>{description}</Text>
            </View>
            <View
                style={[styles.track, isOn ? styles.trackOn : styles.trackOff]}>
                <View
                    style={[
                        styles.thumb,
                        isOn ? styles.thumbOn : styles.thumbOff,
                    ]}
                />
            </View>
        </Pressable>
    );
});

SettingToggle.displayName = 'SettingToggle';

const styles = StyleSheet.create({
    container: {
        flexDirection: 'row',
        alignItems: 'center',
        justifyContent: 'space-between',
        padding: 20,
        backgroundColor: colors.card,
        borderRadius: 24,
        borderWidth: 1,
        borderColor: colors.border,
    },
    textContainer: {
        flex: 1,
        paddingRight: 20,
    },
    title: {
        fontSize: 16,
        lineHeight: 24,
        fontWeight: '700',
        color: colors.text,
    },
    description: {
        fontSize: 14,
        lineHeight: 20,
        color: colors.textMuted,
        marginTop: 4,
    },
    track: {
        width: 52,
        height: 28,
        borderRadius: 14,
    },
    trackOn: {
        backgroundColor: colors.primary,
    },
    trackOff: {
        backgroundColor: colors.borderLight,
    },
    thumb: {
        width: 20,
        height: 20,
        borderRadius: 10,
        backgroundColor: colors.text,
        position: 'absolute',
        top: 4,
    },
    thumbOn: {
        left: 28,
    },
    thumbOff: {
        left: 4,
    },
});
