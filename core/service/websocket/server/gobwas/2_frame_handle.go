package gobwas

import (
	"context"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/gobwas/ws"
	"github.com/pipewave-dev/go-pkg/shared/actx"
	"github.com/pipewave-dev/go-pkg/shared/utils/fn"
)

// validateFrame validates a WebSocket frame against the protocol specification.
func (s *NetpollServer) validateFrame(frame ws.Frame) error {
	// RSV bits must be 0 when no extensions are negotiated.
	if frame.Header.Rsv != 0 {
		return fmt.Errorf("non-zero rsv bits with no extension negotiated")
	}

	// Validate opcode.
	switch frame.Header.OpCode {
	case ws.OpContinuation, ws.OpText, ws.OpBinary:
		// Data frames - OK
	case ws.OpClose, ws.OpPing, ws.OpPong:
		// Control frames - OK
	default:
		return fmt.Errorf("use of reserved op code: %d", frame.Header.OpCode)
	}

	// Control frames must have FIN bit set (cannot be fragmented).
	if frame.Header.OpCode >= ws.OpClose && !frame.Header.Fin {
		return fmt.Errorf("control frame is not final")
	}

	return nil
}

// handleFrame dispatches a frame by OpCode.
func (s *NetpollServer) handleFrame(client *GobwasConnection, frame ws.Frame) {
	// Unmask payload if masked (client-to-server frames are always masked).
	payload := frame.Payload
	if frame.Header.Masked {
		ws.Cipher(payload, frame.Header.Mask, 0)
	}

	var err error

	switch frame.Header.OpCode {
	case ws.OpText:
		err = s.handleTextFrame(client, payload, frame.Header.Fin)

	case ws.OpBinary:
		err = s.handleBinaryFrame(client, payload, frame.Header.Fin)

	case ws.OpContinuation:
		err = s.handleContinuationFrame(client, payload, frame.Header.Fin)

	case ws.OpClose:
		err = s.handleCloseFrame(client, payload)

	case ws.OpPing:
		err = s.handlePingFrame(client, payload)

	case ws.OpPong:
		err = s.handlePongFrame(client, payload)

	default:
		// Should not be reached; validated above.
		err = fmt.Errorf("unexpected opcode: %d", frame.Header.OpCode)
	}

	if err != nil {
		s.handleProtocolError(client, err)
	}
}

// handleTextFrame processes a text message frame.
func (s *NetpollServer) handleTextFrame(client *GobwasConnection, payload []byte, fin bool) error {
	aCtx := actx.New()
	aCtx.SetWebsocketAuth(client.Auth())
	aCtx.SetTraceID("textmsg" + fn.NewNanoID(18))

	if fin {
		// Complete text message
		s.onTextMessage(aCtx, string(payload), client.auth, func(ctx context.Context, responsePayload []byte) error {
			return s.send(ctx, client, responsePayload)
		})
	} else {
		// Fragmented message - store fragment
		// Future: Implement message fragmentation handling if needed
		// For now, treat as complete message
		s.onTextMessage(aCtx, string(payload), client.auth, func(ctx context.Context, responsePayload []byte) error {
			return s.send(ctx, client, responsePayload)
		})
	}
	return nil
}

// handleBinaryFrame processes a binary message frame.
func (s *NetpollServer) handleBinaryFrame(client *GobwasConnection, payload []byte, fin bool) error {
	aCtx := actx.New()
	aCtx.SetWebsocketAuth(client.Auth())
	aCtx.SetTraceID("binmsg" + fn.NewNanoID(18))

	if fin {
		// Complete binary message
		s.onBinMessage(aCtx, payload, client.auth, func(ctx context.Context, responsePayload []byte) error {
			return s.send(ctx, client, responsePayload)
		})
	} else {
		// Fragmented message - store fragment
		// Future: Implement message fragmentation handling if needed
		s.onBinMessage(aCtx, payload, client.auth, func(ctx context.Context, responsePayload []byte) error {
			return s.send(ctx, client, responsePayload)
		})
	}
	return nil
}

// handleContinuationFrame processes a continuation frame (part of a fragmented message).
func (s *NetpollServer) handleContinuationFrame(_ *GobwasConnection, payload []byte, fin bool) error {
	// Note: just log and continue. Pipewave client SDK (such as react) does not fragment messages
	fmt.Printf("Received continuation frame, fin=%v, payload_len=%d\n", fin, len(payload))
	return nil
}

// handleCloseFrame processes a close frame.
func (s *NetpollServer) handleCloseFrame(client *GobwasConnection, payload []byte) error {
	client.MarkCloseReceived()

	if len(payload) == 1 {
		return s.writeCloseOnce(client, ws.StatusProtocolError, "invalid close payload")
	}

	closeCode := ws.StatusNormalClosure
	closeReason := ""
	if len(payload) >= 2 {
		closeCode = ws.StatusCode(binary.BigEndian.Uint16(payload[:2]))
		if len(payload) > 2 {
			closeReason = string(payload[2:])
		}
	}

	// Per RFC 6455, if we receive close and have not sent close yet,
	// respond with close and then close the transport.
	if err := s.writeCloseOnce(client, closeCode, closeReason); err != nil {
		client.Close()
		return err
	}

	client.Close()
	return nil
}

// handlePingFrame processes a ping frame.
func (s *NetpollServer) handlePingFrame(client *GobwasConnection, payload []byte) error {
	// Respond with pong frame containing the same payload
	pongFrame := ws.NewPongFrame(payload)
	if err := s.writeFrame(client, pongFrame); err != nil {
		return fmt.Errorf("failed to send pong: %w", err)
	}
	return nil
}

// handlePongFrame processes a pong frame.
func (s *NetpollServer) handlePongFrame(client *GobwasConnection, payload []byte) error {
	// Pong confirms the transport is still alive after a server ping.
	client.notePong(time.Now())
	return nil
}

// handleProtocolError sends a close frame with an error code and removes the client.
func (s *NetpollServer) handleProtocolError(client *GobwasConnection, err error) {
	fmt.Printf("Protocol error: %v\n", err)
	if errWrite := s.writeCloseOnce(client, ws.StatusProtocolError, err.Error()); errWrite != nil {
		_ = errWrite
	}

	// Remove client
	client.Close()
}

func (s *NetpollServer) writeCloseOnce(client *GobwasConnection, code ws.StatusCode, reason string) error {
	if !client.MarkCloseSentIfFirst() {
		return nil
	}

	closeFrame := ws.NewCloseFrame(ws.NewCloseFrameBody(code, reason))
	if err := s.writeFrame(client, closeFrame); err != nil {
		return err
	}

	return nil
}
