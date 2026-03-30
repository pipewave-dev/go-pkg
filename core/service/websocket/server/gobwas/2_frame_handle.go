package gobwas

import (
	"fmt"

	"github.com/gobwas/ws"
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
func (s *NetpollServer) handleFrame(client *GobwasConnection, frame ws.Frame) error {
	// Unmask payload if masked (client-to-server frames are always masked).
	payload := frame.Payload
	if frame.Header.Masked {
		ws.Cipher(payload, frame.Header.Mask, 0)
	}

	switch frame.Header.OpCode {
	case ws.OpText:
		return s.handleTextFrame(client, payload, frame.Header.Fin)

	case ws.OpBinary:
		return s.handleBinaryFrame(client, payload, frame.Header.Fin)

	case ws.OpContinuation:
		return s.handleContinuationFrame(client, payload, frame.Header.Fin)

	case ws.OpClose:
		return s.handleCloseFrame(client, payload)

	case ws.OpPing:
		return s.handlePingFrame(client, payload)

	case ws.OpPong:
		return s.handlePongFrame(client, payload)

	default:
		// Should not be reached; validated above.
		return fmt.Errorf("unexpected opcode: %d", frame.Header.OpCode)
	}
}

// handleTextFrame processes a text message frame.
func (s *NetpollServer) handleTextFrame(client *GobwasConnection, payload []byte, fin bool) error {
	if fin {
		// Complete text message
		s.onTextMessage(string(payload), client.auth, func(responsePayload []byte) error {
			return s.send(client, responsePayload)
		})
	} else {
		// Fragmented message - store fragment
		// Future: Implement message fragmentation handling if needed
		// For now, treat as complete message
		s.onTextMessage(string(payload), client.auth, func(responsePayload []byte) error {
			return s.send(client, responsePayload)
		})
	}
	return nil
}

// handleBinaryFrame processes a binary message frame.
func (s *NetpollServer) handleBinaryFrame(client *GobwasConnection, payload []byte, fin bool) error {
	if fin {
		// Complete binary message
		s.onBinMessage(payload, client.auth, func(responsePayload []byte) error {
			return s.send(client, responsePayload)
		})
	} else {
		// Fragmented message - store fragment
		// Future: Implement message fragmentation handling if needed
		s.onBinMessage(payload, client.auth, func(responsePayload []byte) error {
			return s.send(client, responsePayload)
		})
	}
	return nil
}

// handleContinuationFrame processes a continuation frame (part of a fragmented message).
func (s *NetpollServer) handleContinuationFrame(client *GobwasConnection, payload []byte, fin bool) error {
	// TODO: Implement proper fragmentation handling
	// For now, just log and continue
	fmt.Printf("Received continuation frame, fin=%v, payload_len=%d\n", fin, len(payload))
	return nil
}

// handleCloseFrame processes a close frame.
func (s *NetpollServer) handleCloseFrame(client *GobwasConnection, payload []byte) error {
	// Parse close code and reason if present
	// var closeCode ws.StatusCode = ws.StatusNormalClosure
	// var reason string
	// fmt.Printf("Received close frame: code=%d, reason=%s\n", closeCode, reason)

	// if len(payload) >= 2 {
	// 	closeCode = ws.StatusCode(uint16(payload[0])<<8 | uint16(payload[1]))
	// 	if len(payload) > 2 {
	// 		reason = string(payload[2:])
	// 	}
	// }

	// Send close frame response
	closeFrame := ws.NewCloseFrame(ws.NewCloseFrameBody(ws.StatusNormalClosure, ""))
	if err := ws.WriteFrame(client.conn, closeFrame); err != nil {
		return err
	}

	// Return false to indicate connection should be closed
	return nil
}

// handlePingFrame processes a ping frame.
func (s *NetpollServer) handlePingFrame(client *GobwasConnection, payload []byte) error {
	// Respond with pong frame containing the same payload
	pongFrame := ws.NewPongFrame(payload)
	if err := ws.WriteFrame(client.conn, pongFrame); err != nil {
		return fmt.Errorf("failed to send pong: %w", err)
	}
	return nil
}

// handlePongFrame processes a pong frame.
func (s *NetpollServer) handlePongFrame(client *GobwasConnection, payload []byte) error {
	// Pong frame received - could be response to our ping
	// TODO: Implement ping/pong tracking if needed for keep-alive
	fmt.Printf("Received pong frame with payload length: %d\n", len(payload))
	return nil
}

// handleProtocolError sends a close frame with an error code and removes the client.
func (s *NetpollServer) handleProtocolError(client *GobwasConnection, err error) {
	fmt.Printf("Protocol error: %v\n", err)

	// Send close frame with protocol error status
	closeFrame := ws.NewCloseFrame(ws.NewCloseFrameBody(
		ws.StatusProtocolError,
		err.Error(),
	))

	// Ignore write error — connection may already be closed.
	ws.WriteFrame(client.conn, closeFrame)

	// Remove client
	client.Close()
}
