/*
 * Copyright (C) 2026 ResultV
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <https://www.gnu.org/licenses/>.
 */

export const formatBytes = (bytes) => {
  if (!bytes || bytes === 0) return "0.00 MB";
  const mb = bytes / (1024 * 1024);
  if (mb < 1024) return mb.toFixed(2) + " MB";
  return (mb / 1024).toFixed(2) + " GB";
};

export const formatSpeed = (bytesPerSec) => {
  if (!bytesPerSec || bytesPerSec === 0) return "0.0 KB/s";
  const kb = bytesPerSec / 1024;
  if (kb < 1024) return kb.toFixed(1) + " KB/s";
  return (kb / 1024).toFixed(1) + " MB/s";
};
