export const compareVersions = (v1: string, v2: string): number => {
    if (!v1 || !v2) return 0;

    const v1Parts = v1.split('.').map(Number);
    const v2Parts = v2.split('.').map(Number);

    const length = Math.max(v1Parts.length, v2Parts.length);

    for (let i = 0; i < length; i++) {
        const p1 = v1Parts[i] || 0;
        const p2 = v2Parts[i] || 0;

        if (p1 > p2) return 1;
        if (p1 < p2) return -1;
    }

    return 0;
};
