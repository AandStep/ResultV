package proxy

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"strings"
	"testing"
	"time"

	"resultproxy-wails/internal/verdict"
)

// echoServer — «сайт»: пишет обратно то, что ему прислали, с префиксом.
func echoServer(t *testing.T, prefix string) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() {
		for {
			c, aErr := ln.Accept()
			if aErr != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 512)
				n, rErr := c.Read(buf)
				if rErr != nil {
					return
				}
				fmt.Fprintf(c, "%s:%s", prefix, buf[:n])
			}(c)
		}
	}()
	t.Cleanup(func() { ln.Close() })
	return ln
}

// dialThroughRelay говорит с реле как аутбаунд http ядра: CONNECT, затем
// байты вперемешку.
func dialThroughRelay(t *testing.T, relayPort int, target string) net.Conn {
	t.Helper()
	c, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", relayPort))
	if err != nil {
		t.Fatalf("dial relay: %v", err)
	}
	fmt.Fprintf(c, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", target, target)
	resp, err := http.ReadResponse(bufio.NewReader(c), &http.Request{Method: "CONNECT"})
	if err != nil {
		t.Fatalf("read CONNECT response: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("CONNECT дал %d, ожидалось 200", resp.StatusCode)
	}
	return c
}

// Известный direct-вердикт реле отрабатывает без гонки: соединение уходит
// прямым дозвоном, туннель не трогается вовсе.
func TestSmartRelay_KnownDirect_GoesStraightOut(t *testing.T) {
	site := echoServer(t, "direct")
	dir := t.TempDir()

	relay, err := StartSmartRelay(SmartRelayOptions{
		DataDir: dir, ListenPort: 0, TunnelPort: 0, MemoryOnly: true,
	})
	if err != nil {
		t.Fatalf("start relay: %v", err)
	}
	defer relay.Close()
	relay.store.Seed(site.Addr().(*net.TCPAddr).IP.String(), verdict.Direct, verdict.SourceUser)

	c := dialThroughRelay(t, relay.Port(), site.Addr().String())
	defer c.Close()
	fmt.Fprint(c, "привет")
	got := readAll(t, c)
	if !strings.HasPrefix(got, "direct:") {
		t.Fatalf("ответ %q, ожидался прямой путь", got)
	}
}

// Незнакомое имя реле разыгрывает и записывает исход. Туннель здесь отвечает
// быстрее, потому что прямого пути к этому адресу нет вовсе.
func TestSmartRelay_UnknownRaces_AndLearns(t *testing.T) {
	tunnelSite := echoServer(t, "tunnel")
	dir := t.TempDir()

	// Инбаунд «туннеля»: SOCKS5-сервер, который всё ведёт в tunnelSite.
	tunnel := fakeSocksInbound(t, tunnelSite.Addr().String())

	relay, err := StartSmartRelay(SmartRelayOptions{
		DataDir: dir, ListenPort: 0, TunnelPort: tunnel, MemoryOnly: true,
	})
	if err != nil {
		t.Fatalf("start relay: %v", err)
	}
	defer relay.Close()

	// Адрес, который никто не слушает: прямая нога откажет сразу.
	dead := deadAddress(t)
	c := dialThroughRelay(t, relay.Port(), dead)
	defer c.Close()
	fmt.Fprint(c, "привет")
	got := readAll(t, c)
	if !strings.HasPrefix(got, "tunnel:") {
		t.Fatalf("ответ %q, ожидался путь через туннель", got)
	}

	host, _, _ := net.SplitHostPort(dead)
	addr := netip.MustParseAddr(host)
	// Имени тут нет — только адрес, как у Telegram MTProto. Спрашиваем так же,
	// как спросит реле на следующем соединении.
	waitFor(t, func() bool {
		rec, ok := relay.store.LookupIP(addr)
		return ok && rec.Decision == verdict.Proxy
	}, "вердикт proxy не записан")
}

// Файл rule-set обновляется после того, как гонка научилась: иначе выученное
// так и осталось бы ходить через реле.
func TestSmartRelay_LearnedDirect_LandsInRuleSetFile(t *testing.T) {
	dir := t.TempDir()
	relay, err := StartSmartRelay(SmartRelayOptions{
		DataDir: dir, ListenPort: 0, TunnelPort: 0, MemoryOnly: false,
	})
	if err != nil {
		t.Fatalf("start relay: %v", err)
	}
	defer relay.Close()

	relay.store.Learn("clean.example", verdict.Direct)
	relay.markDirty()

	path := SmartDirectSetPath(dir)
	waitFor(t, func() bool {
		for _, n := range ReadSmartDirectSet(path) {
			if n == "clean.example" {
				return true
			}
		}
		return false
	}, "имя не доехало до файла rule-set")
}

// «Только в памяти» значит именно это: ни стор, ни rule-set на диск не идут,
// но скелет остаётся — без него ядро не стартует.
func TestSmartRelay_MemoryOnly_WritesNothingButSkeleton(t *testing.T) {
	dir := t.TempDir()
	relay, err := StartSmartRelay(SmartRelayOptions{
		DataDir: dir, ListenPort: 0, TunnelPort: 0, MemoryOnly: true,
	})
	if err != nil {
		t.Fatalf("start relay: %v", err)
	}
	relay.store.Learn("clean.example", verdict.Direct)
	relay.markDirty()
	// Close сам дописывает всё, что должно было дописаться, — ждать таймера
	// не нужно, и именно поэтому он тут самая строгая проверка.
	if err := relay.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	if names := ReadSmartDirectSet(SmartDirectSetPath(dir)); len(names) != 0 {
		t.Errorf("в памяти-только, а в файле %v", names)
	}
	if _, err := os.Stat(SmartVerdictStorePath(dir)); err == nil {
		t.Error("в памяти-только, а стор записан на диск")
	}
}

// Перезапуск: имена из файла возвращаются в стор, и повторного обучения не
// требуется.
func TestSmartRelay_Restart_RehydratesNamesFromFile(t *testing.T) {
	dir := t.TempDir()
	first, err := StartSmartRelay(SmartRelayOptions{DataDir: dir, ListenPort: 0, TunnelPort: 0})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	first.store.Learn("clean.example", verdict.Direct)
	first.markDirty()
	if err := first.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	second, err := StartSmartRelay(SmartRelayOptions{DataDir: dir, ListenPort: 0, TunnelPort: 0})
	if err != nil {
		t.Fatalf("restart: %v", err)
	}
	defer second.Close()
	if _, ok := second.store.Names()["clean.example"]; !ok {
		t.Fatalf("имя не вернулось в стор после перезапуска: %v", second.store.Names())
	}
}

// Мусор вместо CONNECT соединение закрывает, а не роняет реле.
func TestSmartRelay_GarbageRequest_IsRefused(t *testing.T) {
	relay, err := StartSmartRelay(SmartRelayOptions{
		DataDir: t.TempDir(), ListenPort: 0, TunnelPort: 0, MemoryOnly: true,
	})
	if err != nil {
		t.Fatalf("start relay: %v", err)
	}
	defer relay.Close()

	c, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", relay.Port()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	fmt.Fprint(c, "НЕ CONNECT вовсе\r\n\r\n")
	_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
	buf := make([]byte, 64)
	if n, rErr := c.Read(buf); rErr == nil && n > 0 {
		t.Fatalf("реле ответило %q на мусор, ожидалось закрытие", buf[:n])
	}
	// Реле обязано пережить это и принять следующего.
	if !relayAccepts(t, relay.Port()) {
		t.Fatal("реле перестало принимать соединения после мусорного запроса")
	}
}

// Заголовок без конца обрывается по потолку, а не копится в памяти до
// дедлайна: до этого порта дотянется любое приложение устройства.
func TestSmartRelay_OversizedHeader_IsRefusedBeforeDeadline(t *testing.T) {
	relay, err := StartSmartRelay(SmartRelayOptions{
		DataDir: t.TempDir(), ListenPort: 0, TunnelPort: 0, MemoryOnly: true,
	})
	if err != nil {
		t.Fatalf("start relay: %v", err)
	}
	defer relay.Close()

	c, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", relay.Port()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()

	started := time.Now()
	// Байты без перевода строки. Запись обязана упереться в закрытое реле
	// задолго до smartRelayHeaderTimeout.
	junk := bytes.Repeat([]byte("x"), 4096)
	var wErr error
	for i := 0; i < 64 && wErr == nil; i++ {
		_ = c.SetWriteDeadline(time.Now().Add(5 * time.Second))
		_, wErr = c.Write(junk)
	}
	_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
	buf := make([]byte, 64)
	_, _ = c.Read(buf)
	if elapsed := time.Since(started); elapsed >= smartRelayHeaderTimeout {
		t.Fatalf("реле держало соединение %v — дольше дедлайна, потолок не сработал", elapsed)
	}
	if !relayAccepts(t, relay.Port()) {
		t.Fatal("реле перестало принимать соединения после переростка")
	}
}

// relayAccepts проверяет, что реле живо: обычный CONNECT получает свои 200.
func relayAccepts(t *testing.T, port int) bool {
	t.Helper()
	c, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return false
	}
	defer c.Close()
	fmt.Fprint(c, "CONNECT example.invalid:443 HTTP/1.1\r\nHost: example.invalid:443\r\n\r\n")
	_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
	resp, err := http.ReadResponse(bufio.NewReader(c), &http.Request{Method: "CONNECT"})
	return err == nil && resp.StatusCode == http.StatusOK
}

func readAll(t *testing.T, c net.Conn) string {
	t.Helper()
	_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
	buf := make([]byte, 512)
	n, err := c.Read(buf)
	if err != nil && n == 0 {
		t.Fatalf("read: %v", err)
	}
	return string(buf[:n])
}

func waitFor(t *testing.T, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal(msg)
}

// deadAddress возвращает адрес, который заведомо никто не слушает: порт занят
// и тут же отпущен.
func deadAddress(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	ln.Close()
	return addr
}

// fakeSocksInbound — заглушка инбаунда движка: принимает SOCKS5 CONNECT и
// ведёт всё в один адрес, как это сделал бы туннель.
func fakeSocksInbound(t *testing.T, target string) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, aErr := ln.Accept()
			if aErr != nil {
				return
			}
			go serveFakeSocks(c, target)
		}
	}()
	return ln.Addr().(*net.TCPAddr).Port
}

func serveFakeSocks(c net.Conn, target string) {
	defer c.Close()
	r := bufio.NewReader(c)
	// Приветствие: версия, число методов, методы.
	head := make([]byte, 2)
	if _, err := io.ReadFull(r, head); err != nil {
		return
	}
	if _, err := io.ReadFull(r, make([]byte, int(head[1]))); err != nil {
		return
	}
	if _, err := c.Write([]byte{0x05, 0x00}); err != nil {
		return
	}
	// Запрос: ver, cmd, rsv, atyp, addr, port. Адрес нам не нужен — заглушка
	// всегда ведёт в один и тот же target.
	req := make([]byte, 4)
	if _, err := io.ReadFull(r, req); err != nil {
		return
	}
	switch req[3] {
	case 0x01:
		io.ReadFull(r, make([]byte, 4+2))
	case 0x03:
		l := make([]byte, 1)
		if _, err := io.ReadFull(r, l); err != nil {
			return
		}
		io.ReadFull(r, make([]byte, int(l[0])+2))
	case 0x04:
		io.ReadFull(r, make([]byte, 16+2))
	}
	if _, err := c.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0}); err != nil {
		return
	}
	up, err := net.Dial("tcp", target)
	if err != nil {
		return
	}
	defer up.Close()
	go io.Copy(up, r)
	io.Copy(c, up)
}
