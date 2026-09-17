package verdict

import (
	"path/filepath"
	"testing"
	"time"
)

// Плейнтекст имён не переживает перезапуск: он лежит в store.plain, а на диск
// уходят только хеши. Rehydrate возвращает имена в стор по списку, который
// пережил перезапуск снаружи — в нашем случае в rule-set файле.
func TestRehydrate_ReturnsNamesToNamesMap(t *testing.T) {
	now := func() time.Time { return time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC) }
	path := filepath.Join(t.TempDir(), "verdicts.json")

	first := New([]byte("salt"), now)
	first.Learn("example.com", Direct)
	if err := first.Save(path); err != nil {
		t.Fatalf("save: %v", err)
	}

	second, err := Load(path, now)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got := len(second.Names()); got != 0 {
		t.Fatalf("после загрузки имён быть не должно, получено %d", got)
	}

	if n := second.Rehydrate([]string{"example.com"}); n != 1 {
		t.Fatalf("Rehydrate вернул %d, ожидался 1", n)
	}
	names := second.Names()
	rec, ok := names["example.com"]
	if !ok {
		t.Fatalf("имя не вернулось в Names(), получено %+v", names)
	}
	if rec.Decision != Direct {
		t.Errorf("решение = %v, ожидалось Direct", rec.Decision)
	}
}

// Имя, записи по которому нет (истекла или её не было), в стор не попадает:
// Rehydrate восстанавливает плейнтекст, а не создаёт вердикты.
func TestRehydrate_UnknownNameCreatesNothing(t *testing.T) {
	now := func() time.Time { return time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC) }
	s := New([]byte("salt"), now)

	if n := s.Rehydrate([]string{"nobody.example"}); n != 0 {
		t.Fatalf("Rehydrate вернул %d, ожидался 0", n)
	}
	if _, ok := s.Lookup("nobody.example"); ok {
		t.Error("Rehydrate завёл запись, которой не было")
	}
	if got := len(s.Names()); got != 0 {
		t.Errorf("Names() не пуст: %d", got)
	}
}
