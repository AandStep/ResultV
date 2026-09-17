// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package verdict

// Rehydrate возвращает в стор плейнтекст имён, переживших перезапуск снаружи.
//
// На диск уходят только хеши (см. комментарий к Store): после Load стор знает
// вердикт по имени, но не знает самого имени, а значит Names() пуст и рендер
// плейнтекстового rule-set стёр бы файл, из которого имена только что и
// пришли. Rehydrate — обратный ход: по списку имён он находит уже лежащие
// записи и подписывает их.
//
// Новых записей не создаёт. Имя без живой записи — истекшее или чужое — молча
// пропускается, и при следующем рендере само выпадает из файла.
//
// Возвращает число узнанных имён: вызывающая сторона пишет его в лог, потому
// что «ноль из трёхсот» означает сменившуюся соль, а не пустой файл.
func (s *Store) Rehydrate(names []string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	space := s.spaces[s.ns]
	if space == nil {
		return 0
	}
	now := s.now()
	restored := 0
	for _, raw := range names {
		key := NormalizeHost(raw)
		if key == "" {
			continue
		}
		h := s.hash(key)
		rec, ok := space[h]
		if !ok {
			continue
		}
		if !rec.ExpiresAt.IsZero() && !rec.ExpiresAt.After(now) {
			continue
		}
		s.plain[h] = key
		restored++
	}
	return restored
}
