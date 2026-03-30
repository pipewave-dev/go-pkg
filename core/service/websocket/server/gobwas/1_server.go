package gobwas

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"github.com/mailru/easygo/netpoll"
	voAuth "github.com/pipewave-dev/go-pkg/core/domain/value-object/auth"
	wsSv "github.com/pipewave-dev/go-pkg/core/service/websocket"
	dostuffs "github.com/pipewave-dev/go-pkg/global/do-stuffs"
	workerpool "github.com/pipewave-dev/go-pkg/pkg/worker-pool"
	configprovider "github.com/pipewave-dev/go-pkg/provider/config-provider"
	healthyprovider "github.com/pipewave-dev/go-pkg/provider/healthy-provider"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

// Check types
var (
	_ wsSv.WebsocketServer = (*NetpollServer)(nil)
	_ wsSv.WebsocketConn   = (*GobwasConnection)(nil)
)

var (
	server *NetpollServer
	once   sync.Once
)

func NewServer(
	c configprovider.ConfigStore,
	workerPool *workerpool.WorkerPool,
	healthy healthyprovider.Healthy,
	onTextMessage wsSv.OnTextMessageFn,
	onBinMessage wsSv.OnBinMessageFn,
	onReadError wsSv.OnReadErrorFn,
	onWriteError wsSv.OnWriteErrorFn,
	onClose wsSv.OnCloseStuffFn,
) *NetpollServer {
	once.Do(func() {
		// Create netpoll poller
		poller, err := netpoll.New(&netpoll.Config{
			OnWaitError: func(err error) {
				log.Printf("Netpoll error: %v", err)
			},
		})
		if err != nil {
			panic(fmt.Errorf("failed to create netpoll: %w", err))
		}

		server = &NetpollServer{
			c:          c,
			poller:     poller,
			healthy:    healthy,
			stats:      &serverStats{StartTime: time.Now()},
			workerPool: workerPool,

			onTextMessage: onTextMessage,
			onBinMessage:  onBinMessage,
			onReadError:   onReadError,
			onWriteError:  onWriteError,
			onClose:       onClose,
		}

		dostuffs.DebugFn.RegTask(server.printStats)
	})
	return server
}

// NewConnection registers a new connection with netpoll.
func (s *NetpollServer) NewConnection(
	conn net.Conn,
	propAuth voAuth.WebsocketAuth,
) (wsConn wsSv.WebsocketConn, aErr aerror.AError) {
	if !s.healthy.IsHealthy() {
		return nil, aerror.New(context.Background(), aerror.ErrServerIsShuttingDown, errors.New("server is shutting down"))
	}

	// Validate connection before proceeding
	if conn == nil {
		return nil, aerror.New(context.Background(), aerror.ErrUnexpectedSyscall, errors.New("connection is nil"))
	}

	// Ensure connection supports file descriptor operations (TCP connections)
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		// Get the underlying file descriptor to verify it's valid
		if file, err := tcpConn.File(); err != nil {
			conn.Close()
			return nil, aerror.New(context.Background(), aerror.ErrUnexpectedSyscall, fmt.Errorf("failed to get file descriptor from TCP connection: %w", err))
		} else {
			// Close the file immediately as we only needed to check if FD is accessible
			file.Close()
		}
	} else {
		// For non-TCP connections, we can't guarantee FD availability
		slog.Warn("Connection is not a TCP connection, may not support netpoll", slog.String("type", fmt.Sprintf("%T", conn)))
	}

	atomic.AddInt64(&s.stats.ConnectionsAccepted, 1)
	atomic.AddInt64(&s.connections, 1)

	client := &GobwasConnection{
		c:      s.c,
		server: s,
		conn:   conn,
		auth:   propAuth,
	}

	// Create netpoll descriptor with better error handling
	desc, err := netpoll.HandleRead(client.conn)
	if err != nil {
		slog.Error("Failed to create netpoll descriptor",
			slog.Any("error", err),
			slog.String("remote_addr", conn.RemoteAddr().String()),
			slog.String("local_addr", conn.LocalAddr().String()),
			slog.String("conn_type", fmt.Sprintf("%T", conn)))

		// Clean up connection before returning error
		conn.Close()
		atomic.AddInt64(&s.connections, -1)
		atomic.AddInt64(&s.stats.ConnectionsClosed, 1)

		return nil, aerror.New(context.Background(), aerror.ErrUnexpectedSyscall, err)
	}
	client.desc = desc

	// Register with netpoll to monitor I/O events.
	// This callback is invoked only when data is actually available to read.
	err = s.poller.Start(desc, func(ev netpoll.Event) {
		if ev&netpoll.EventReadHup != 0 {
			// Connection closed
			client.Close()
			return
		}

		// Only process when real data is available — no goroutines block waiting.
		s.handleClientData(client)
	})
	if err != nil {
		slog.Error("Failed to start netpoll monitoring", slog.Any("error", err))

		client.Close()
		return nil, aerror.New(context.Background(), aerror.ErrUnexpectedSyscall, err)
	}

	return client, nil
}

// handleClientData processes data from a client (called by the netpoll callback).
func (s *NetpollServer) handleClientData(client *GobwasConnection) {
	s.workerPool.Submit(func() {
		s.processClientMessage(client)
	})
}

// send writes a binary frame to the client connection.
func (s *NetpollServer) send(client *GobwasConnection, payload []byte) error {
	conn := client.conn
	if conn == nil {
		return fmt.Errorf("connection is nil")
	}
	// Use a binary frame because payload may be MessagePack/binary, not UTF-8.
	frame := ws.NewBinaryFrame(payload)
	if err := ws.WriteFrame(conn, frame); err != nil {
		s.onWriteError(client.auth, fmt.Errorf("failed to send message: %w", err))
		client.Close()
		return fmt.Errorf("failed to send message: %w", err)
	}
	return nil
}

func (s *NetpollServer) ping(client *GobwasConnection) {
	s.workerPool.Submit(func() {
		conn := client.conn
		if conn == nil {
			return
		}
		err := wsutil.WriteServerMessage(conn, ws.OpPing, nil)
		if err != nil {
			s.onWriteError(client.auth, err)
			client.Close()
			return
		}
	})
}

func (s *NetpollServer) processClientMessage(client *GobwasConnection) {
	conn := client.conn
	if conn == nil {
		return
	}
	header, err := ws.ReadHeader(conn)
	if err != nil {
		if errors.Is(err, io.EOF) {
			// Connection closed normally
			client.Close()
			return
		}
		if errors.Is(err, net.ErrClosed) {
			// Connection closed unexpectedly
			client.Close()
			return
		}
		// Read error
		s.onReadError(client.auth, err)
		return
	}

	const MaxFrameSize = 1 * 1024 * 1024 // 1MB

	if header.Length > MaxFrameSize {
		err = fmt.Errorf("frame size %d exceeds maximum allowed size %d", header.Length, MaxFrameSize)
		s.handleProtocolError(client, err) // send close protocol error
		return
	}

	payload := make([]byte, header.Length)
	if header.Length > 0 {
		if _, err = io.ReadFull(conn, payload); err != nil {
			s.onReadError(client.auth, fmt.Errorf("failed to read frame payload: %w", err))
			client.Close()
			return
		}
	}

	frame := ws.Frame{
		Header:  header,
		Payload: payload,
	}

	// Validate frame before processing
	if err = s.validateFrame(frame); err != nil {
		s.handleProtocolError(client, err)
		return
	}

	// Process frame based on OpCode
	err = s.handleFrame(client, frame)
	if err != nil {
		s.onReadError(client.auth, err)
		client.Close()
		return
	}
}

// removeClient cleans up a client on disconnect.
func (s *NetpollServer) removeClient(client *GobwasConnection) {
	if !atomic.CompareAndSwapInt32(&client.closed, 0, 1) {
		return
	}

	atomic.AddInt64(&s.stats.ConnectionsClosed, 1)
	atomic.AddInt64(&s.connections, -1)

	// Stop netpoll monitoring
	if client.desc != nil {
		_ = s.poller.Stop(client.desc)
		client.desc.Close()
		client.desc = nil
	}

	// Close connection
	if client.conn != nil {
		client.conn.Close()
	}

	// Trigger onClose callback
	s.onClose.Do(client.auth)
}

func (s *NetpollServer) printStats() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	currentConns := atomic.LoadInt64(&s.connections)
	totalAccepted := atomic.LoadInt64(&s.stats.ConnectionsAccepted)
	totalClosed := atomic.LoadInt64(&s.stats.ConnectionsClosed)

	uptime := time.Since(s.stats.StartTime)

	fmt.Printf("=== NETPOLL SERVER STATS (Runtime: %v)\n", uptime.Round(time.Second))
	fmt.Printf("\t Connections: %d active | %d total accepted | %d closed\n",
		currentConns, totalAccepted, totalClosed)
	fmt.Printf("\t Memory Usage:\n")
	fmt.Printf("\t\t   - Allocated: %.2f MB\n", float64(m.Alloc)/1024/1024)
	fmt.Printf("\t\t- System: %.2f MB\n", float64(m.Sys)/1024/1024)
	fmt.Printf("\t\t- Stack: %.2f MB\n", float64(m.StackSys)/1024/1024)
	if currentConns > 0 {
		fmt.Printf(" \t\t- Per Connection: %.2f KB (vs ~8KB in standard)\n",
			float64(m.Alloc)/float64(currentConns)/1024)
	}
	fmt.Printf("\t\t- GC Cycles: %d\n", m.NumGC)
}
