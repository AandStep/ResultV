export const formatBytes = (bytes: number): string => {
    if (!bytes || bytes === 0) return '0.00 MB';
    const mb = bytes / (1024 * 1024);
    if (mb < 1024) return mb.toFixed(2) + ' MB';
    return (mb / 1024).toFixed(2) + ' GB';
};

export const formatSpeed = (bytesPerSec: number): string => {
    if (!bytesPerSec || bytesPerSec === 0) return '0.0 KB/s';
    const kb = bytesPerSec / 1024;
    if (kb < 1024) return kb.toFixed(1) + ' KB/s';
    return (kb / 1024).toFixed(1) + ' MB/s';
};
