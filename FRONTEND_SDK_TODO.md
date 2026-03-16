# Frontend SDK TODO

Features that require corresponding implementation in the Frontend/Client SDK.

## Message Acknowledgment (ACK)

**Backend behavior:** When server sends a message via `SendToSessionWithAck` or `SendToUserWithAck`, the message includes an `ackId` field. Server maintains a pending ACK map and waits for client response until timeout.

**Frontend SDK needs to:**
1. Detect incoming messages with `ackId` field
2. After processing the message, send back an ACK message with type `__ack__` and the corresponding `ackId`
3. ACK response format: `{ "t": "__ack__", "ackId": "<received_ack_id>" }`

**Example flow:**
```
Server -> Client: { "t": "payment_update", "ackId": "abc123", "b": <payload> }
Client -> Server: { "t": "__ack__", "ackId": "abc123" }
```

**Timeout:** If client does not send ACK within the specified timeout, `SendToUserWithAck` returns `acked=false`.
