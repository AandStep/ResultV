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

package proxy

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"

	// Псевдоним обязателен: пакет здесь сам называется proxy, и импорт под тем
	// же именем читался бы как обращение к себе.
	socksproxy "golang.org/x/net/proxy"

	"resultproxy-wails/internal/verdict"
)

const (
	// smartRelayFlushDelay — задержка перед записью файла rule-set после
	// изменения. Загрузка одной страницы учит десяток имён; писать файл
	// десять раз подряд значит десять раз дёрнуть fswatch ядра.
	smartRelayFlushDelay = 2 * time.Second
	// smartRelayStoreInterval — как часто хешированный стор уходит на диск.
	// Реже, чем rule-set: его никто не читает на ходу, а запись дороже.
	smartRelayStoreInterval = 60 * time.Second
	// smartRelayHeaderTimeout — сколько ждём заголовок CONNECT. Клиент здесь
	// не человек, а аутбаунд ядра: он пишет запрос сразу.
	smartRelayHeaderTimeout = 10 * time.Second
)

// SmartRelayOptions — всё, что реле нужно знать о мире.
type SmartRelayOptions struct {
	// DataDir — корень данных приложения (filesDir). Файлы лягут в его
	// подпапку smart/, рядом со Smart-списком.
	DataDir string
	// ListenPort — порт реле; 0 означает «любой свободный» и нужен тестам.
	ListenPort int
	// TunnelPort — порт loopback-инбаунда движка, через который проходит
	// туннельная нога. 0 означает, что туннеля нет: гонка тогда вырождается в
	// прямой путь, и это честнее, чем мерить прямой путь дважды.
	TunnelPort int
	// MemoryOnly — не писать на диск ничего, кроме пустого скелета rule-set
	// (без файла ядро не стартует).
	MemoryOnly bool
}

// SmartRelay — loopback-прокси, в который маршрутные правила отправляют всё,
// о чём вердикта ещё нет.
type SmartRelay struct {
	opts     SmartRelayOptions
	listener net.Listener
	store    *verdict.Store
	health   *directHealth
	probes   *probeGate

	// probe вынесен полем, чтобы тест мог ответить без сети.
	probe func(ctx context.Context, host string, gate *probeGate) verdict.Decision

	// inflight держит хосты, по которым проба уже идёт. Загрузка страницы
	// открывает десятки соединений к одному имени разом; без этого каждое
	// начало бы свою пробу и одна страница съела бы весь бюджет намордника.
	inflight sync.Map

	dirty  chan struct{}
	closed chan struct{}
	wg     sync.WaitGroup

	closeOnce sync.Once
}

// StartSmartRelay поднимает реле и возвращает его уже слушающим.
func StartSmartRelay(opts SmartRelayOptions) (*SmartRelay, error) {
	storePath := SmartVerdictStorePath(opts.DataDir)
	setPath := SmartDirectSetPath(opts.DataDir)

	// Порядок обязателен. Сперва имена из файла, потом стор, потом Rehydrate:
	// плейнтекст в сторе сессионный, и рендер до восстановления стёр бы файл,
	// из которого имена только что пришли.
	names := ReadSmartDirectSet(setPath)
	store, err := verdict.Load(storePath, nil)
	if err != nil {
		return nil, err
	}
	store.Rehydrate(names)

	if err := EnsureSmartDirectSet(setPath); err != nil {
		return nil, err
	}

	ln, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(opts.ListenPort))
	if err != nil {
		return nil, err
	}

	r := &SmartRelay{
		opts:     opts,
		listener: ln,
		store:    store,
		health:   newDirectHealth(nil),
		probes:   newProbeGate(nil),
		probe:    probeHost,
		dirty:    make(chan struct{}, 1),
		closed:   make(chan struct{}),
	}
	setProbeInboundPort(opts.TunnelPort)

	r.wg.Add(2)
	go r.acceptLoop()
	go r.flushLoop()
	return r, nil
}

// Port — на каком порту реле в итоге слушает. Тесты просят нулевой и узнают
// ответ здесь.
func (r *SmartRelay) Port() int { return r.listener.Addr().(*net.TCPAddr).Port }

// Close останавливает реле и сохраняет выученное.
func (r *SmartRelay) Close() error {
	var err error
	r.closeOnce.Do(func() {
		close(r.closed)
		err = r.listener.Close()
		r.wg.Wait()
		r.persist()
		setProbeInboundPort(0)
	})
	return err
}

func (r *SmartRelay) acceptLoop() {
	defer r.wg.Done()
	for {
		conn, err := r.listener.Accept()
		if err != nil {
			select {
			case <-r.closed:
				return
			default:
			}
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				continue
			}
			return
		}
		go r.serve(conn)
	}
}

// markDirty просит записать выученное. Не блокирует: буфер на единицу, и
// второй сигнал до записи ничего не добавляет.
func (r *SmartRelay) markDirty() {
	select {
	case r.dirty <- struct{}{}:
	default:
	}
}

// flushLoop пишет файлы: rule-set — вскоре после изменения, стор — по таймеру.
func (r *SmartRelay) flushLoop() {
	defer r.wg.Done()
	var pending <-chan time.Time
	storeTicker := time.NewTicker(smartRelayStoreInterval)
	defer storeTicker.Stop()
	for {
		select {
		case <-r.closed:
			return
		case <-r.dirty:
			pending = time.After(smartRelayFlushDelay)
		case <-pending:
			pending = nil
			r.renderSet()
		case <-storeTicker.C:
			r.saveStore()
		}
	}
}

func (r *SmartRelay) renderSet() {
	if r.opts.MemoryOnly {
		return
	}
	if err := RenderSmartDirectSet(SmartDirectSetPath(r.opts.DataDir), directNamesOf(r.store)); err != nil {
		// Молча: файл — кэш. Потеря рендера стоит петли до следующего
		// изменения, отказ реле стоил бы всего трафика.
		_ = err
	}
}

func (r *SmartRelay) saveStore() {
	if r.opts.MemoryOnly {
		return
	}
	_ = r.store.Save(SmartVerdictStorePath(r.opts.DataDir))
}

func (r *SmartRelay) persist() {
	r.renderSet()
	r.saveStore()
}

// serve обслуживает одно соединение от аутбаунда ядра.
func (r *SmartRelay) serve(client net.Conn) {
	defer client.Close()

	_ = client.SetReadDeadline(time.Now().Add(smartRelayHeaderTimeout))
	reader := bufio.NewReader(client)
	target, err := readConnectRequest(reader)
	if err != nil {
		return
	}
	_ = client.SetReadDeadline(time.Time{})

	// Ответ 200 идёт до дозвона, и это осознанно: без него клиент не пришлёт
	// первых байт, а без первых байт гонку не запустить — их нечего послать
	// обеим ногам. Плата в том, что провал дозвона клиент видит закрытым
	// соединением, а не кодом ошибки. Ровно так же он видит его и сегодня,
	// когда сайт не открывается.
	if _, err := io.WriteString(client, "HTTP/1.1 200 Connection established\r\n\r\n"); err != nil {
		return
	}

	host, portStr, err := net.SplitHostPort(target)
	if err != nil {
		return
	}
	addr, _ := netip.ParseAddr(host)

	rec, known := lookupSmart(r.store, host, addr)
	switch choiceFrom(rec, known, r.health.healthy()) {
	case chooseProxy:
		r.pipeVia(client, reader, target, true)
	case chooseDirect:
		r.pipeVia(client, reader, target, false)
	default:
		r.race(client, reader, target, host, portStr)
	}
}

// pipeVia ведёт соединение одной ногой, без гонки: вердикт уже есть.
func (r *SmartRelay) pipeVia(client net.Conn, buffered *bufio.Reader, target string, viaTunnel bool) {
	var (
		up  net.Conn
		err error
	)
	if viaTunnel {
		up, err = r.dialTunnel(context.Background(), target)
	} else {
		up, err = r.dialDirect(context.Background(), target)
	}
	if err != nil {
		return
	}
	defer up.Close()
	splice(client, buffered, up)
}

// race разыгрывает незнакомое назначение.
func (r *SmartRelay) race(client net.Conn, buffered *bufio.Reader, target, host, portStr string) {
	first := make([]byte, smartFirstReadBudget)
	_ = client.SetReadDeadline(time.Now().Add(smartRaceHeadStart))
	n, _ := buffered.Read(first)
	_ = client.SetReadDeadline(time.Time{})
	if n == 0 {
		// Клиент, который молчит, разыграть нельзя: нечего послать и нечем
		// различить ноги. Пусть идёт так, как ходил до появления фичи.
		r.pipeVia(client, buffered, target, false)
		return
	}

	res := runSmartRace(context.Background(), first[:n],
		func(ctx context.Context) (net.Conn, error) { return r.dialDirect(ctx, target) },
		func(ctx context.Context) (net.Conn, error) { return r.dialTunnel(ctx, target) },
	)
	if res.Err != nil {
		r.health.record(host, false)
		return
	}
	defer res.Conn.Close()

	// Выигрыш прямого пути — доказательство, что линия жива; выигрыш туннеля —
	// ещё один сайт, который сам не ответил.
	r.health.record(host, !res.ViaProxy)
	r.learn(host, portStr, res.ViaProxy)
	if !res.ViaProxy {
		// Прямой путь победил по байтам. Байты это сайт или стена — вопрос
		// другой, и отвечает на него только проба.
		r.recheckAsync(host)
	}

	if _, err := client.Write(res.Head); err != nil {
		return
	}
	splice(client, buffered, res.Conn)
}

// learn записывает, что доказала гонка.
func (r *SmartRelay) learn(host, portStr string, viaTunnel bool) {
	if !r.health.healthy() {
		// Линия лежит. Всё записанное сейчас было бы догадкой сроком до недели.
		return
	}
	d := verdict.Direct
	if viaTunnel {
		d = verdict.Proxy
	}
	if addr, err := netip.ParseAddr(host); err == nil {
		// Голый адрес — это Telegram MTProto и голос Discord: имени они не
		// дают никогда, и записывать их приходится по адресу.
		r.store.LearnIP(addr, d)
	} else {
		r.store.Learn(host, d)
	}
	_ = portStr
	if d == verdict.Direct {
		r.markDirty()
	}
}

// recheckAsync перепроверяет имя, которое гонка отдала прямому пути.
func (r *SmartRelay) recheckAsync(host string) {
	if host == "" || r.probe == nil {
		return
	}
	if _, busy := r.inflight.LoadOrStore(host, struct{}{}); busy {
		return
	}
	go func() {
		defer r.inflight.Delete(host)
		d := r.probe(context.Background(), host, r.probes)
		if d == verdict.Unknown {
			return
		}
		r.store.Learn(host, d)
		r.markDirty()
	}()
}

func (r *SmartRelay) dialDirect(ctx context.Context, target string) (net.Conn, error) {
	d := net.Dialer{Timeout: smartDirectDialTimeout}
	return d.DialContext(ctx, "tcp", target)
}

// dialTunnel ходит SOCKS5 в loopback-инбаунд движка — тем же способом, которым
// сегодня ходит MITM (mobile/libbox_filter.go). Адрес передаётся именем, чтобы
// инбаунд увидел домен и применил к нему обычные правила.
func (r *SmartRelay) dialTunnel(ctx context.Context, target string) (net.Conn, error) {
	if r.opts.TunnelPort == 0 {
		return nil, errors.New("smart: туннельный инбаунд не поднят")
	}
	dialer, err := socksproxy.SOCKS5("tcp", fmt.Sprintf("127.0.0.1:%d", r.opts.TunnelPort), nil, socksproxy.Direct)
	if err != nil {
		return nil, err
	}
	if cd, ok := dialer.(socksproxy.ContextDialer); ok {
		return cd.DialContext(ctx, "tcp", target)
	}
	return dialer.Dial("tcp", target)
}

// readConnectRequest читает `CONNECT host:port HTTP/1.1` и его заголовки.
//
// Свой разбор, а не net/http: нам нужно ровно одно поле, а http.ReadRequest
// потащил бы за собой тело, таймауты и аллокации на каждое соединение.
func readConnectRequest(reader *bufio.Reader) (string, error) {
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	parts := strings.Fields(strings.TrimSpace(line))
	if len(parts) < 2 || !strings.EqualFold(parts[0], "CONNECT") {
		return "", errors.New("smart: ожидался CONNECT, получено " + strings.TrimSpace(line))
	}
	for {
		h, hErr := reader.ReadString('\n')
		if hErr != nil {
			return "", hErr
		}
		if strings.TrimSpace(h) == "" {
			break
		}
	}
	return parts[1], nil
}

// splice сводит две стороны. Буфер читателя обязателен: в нём уже могут лежать
// байты клиента, вычитанные вместе с заголовком.
func splice(client net.Conn, buffered *bufio.Reader, up net.Conn) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = io.Copy(up, buffered)
		if c, ok := up.(*net.TCPConn); ok {
			_ = c.CloseWrite()
		}
	}()
	_, _ = io.Copy(client, up)
	if c, ok := client.(*net.TCPConn); ok {
		_ = c.CloseWrite()
	}
	<-done
}
