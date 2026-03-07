import React, { memo, useMemo } from 'react';
import { StyleSheet, View } from 'react-native';
import Svg, { Polyline } from 'react-native-svg';

type Props = {
    data: number[];
    color: string;
};

export const SpeedChart = memo(({ data, color }: Props) => {
    const points = useMemo(() => {
        if (!data || data.length < 2) return '';
        const max = Math.max(...data, 1024);
        return data
            .map(
                (val, i) =>
                    `${(i / (data.length - 1)) * 100},${25 - (val / max) * 25}`,
            )
            .join(' ');
    }, [data]);

    if (!points) return null;

    return (
        <View style={styles.container}>
            <Svg viewBox="0 0 100 28" style={styles.svg}>
                <Polyline
                    fill="none"
                    stroke={color}
                    strokeWidth="2.5"
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    points={points}
                />
            </Svg>
        </View>
    );
});

SpeedChart.displayName = 'SpeedChart';

const styles = StyleSheet.create({
    container: {
        height: 32,
    },
    svg: {
        width: '100%',
        height: 32,
        opacity: 0.9,
    },
});
