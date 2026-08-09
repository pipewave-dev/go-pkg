# Wire Protocol v1

This document specifies the binary wire format for the Pipewave WebSocket
envelope, version 1 (`v1`). This is a hand-rolled binary format — not
msgpack, JSON, or protobuf — designed to be byte-identical across all client
implementations (Go, TypeScript, Dart). The conformance vectors in
[`wire-vectors-v1.json`](./wire-vectors-v1.json) are the authoritative test
fixtures every implementation of this spec must reproduce exactly.

## Byte layout

```
[0]      version+flags   (1 byte)  — bits 0-3 version, bits 4-7 field presence
[1]      msgType len     (1 byte)  — 0 = control frame, else UTF-8 byte length
[2..]    msgType bytes   (or 1 control-code byte when len == 0)
         id              (1 byte len + bytes)   if flags & 0x10
         responseToId    (1 byte len + bytes)   if flags & 0x20
         ackId           (1 byte len + bytes)   if flags & 0x40
         error           (2 byte len BE + bytes) if flags & 0x80
         binary          (remainder, may be empty)
```

Fields appear in the payload in bit order: `id`, then `responseToId`, then
`ackId`, then `error`, then `binary`. Each optional field is present in the
byte stream if and only if its corresponding flag bit is set in byte 0.

Multi-byte lengths (the `error` field's 2-byte length prefix) are
**big-endian**.

`binary` has **no length prefix** because it is always the last field in the
frame — its length is simply "everything remaining in the frame after the
last field that was present."

## Byte 0: version + flags

Byte 0 packs the format version into the low nibble and field-presence flags
into the high nibble:

- Bits 0-3 (low nibble): version. `v1` = `0x1`.
- Bits 4-7 (high nibble): field-presence flags.

| Flag           | Bit mask | Meaning                              |
|----------------|----------|---------------------------------------|
| `id`           | `0x10`   | `id` field is present                 |
| `responseToId` | `0x20`   | `responseToId` field is present       |
| `ackId`        | `0x40`   | `ackId` field is present              |
| `error`        | `0x80`   | `error` field is present              |

Fields appear in the payload in bit order (`id`, `responseToId`, `ackId`,
`error`), regardless of the order flags are listed or checked.

A decoder **MUST reject** a frame whose byte-0 low nibble (version) is not a
version it recognizes. For this spec, only `0x1` is valid; any other version
value is a decode error.

## Byte 1 / msgType

Byte 1 is the `msgType` length:

- If byte 1 is `0`, this is a **control frame**: byte 2 is a single
  control-code byte (see below) instead of UTF-8 `msgType` bytes.
- If byte 1 is nonzero, it is the UTF-8 byte length of `msgType` (1-255),
  and that many bytes follow as the UTF-8-encoded `msgType` string.

## Control codes

Reserved control codes, used only when `msgTypeLen == 0`:

| Control  | Code            |
|----------|-----------------|
| heartbeat | `0xCA` (202)   |
| ack       | `0xCB` (203)   |

A control frame is encoded as `msgTypeLen = 0` followed by one control-code
byte. A control frame may still be followed by optional fields and/or
`binary`, exactly as a `msgType` frame would.

## Length limits

| Field           | Max length      | Length prefix        |
|-----------------|-----------------|-----------------------|
| `msgType`       | 255 bytes       | 1 byte                |
| `id`            | 255 bytes       | 1 byte                |
| `responseToId`  | 255 bytes       | 1 byte                |
| `ackId`         | 255 bytes       | 1 byte                |
| `error`         | 65535 bytes     | 2 bytes, big-endian   |
| `binary`        | unbounded       | none (remainder)      |

## Decode error rules

A decoder MUST reject a frame under any of these conditions:

- The frame is shorter than 2 bytes.
- The version nibble (byte 0, bits 0-3) is not a recognized version.
- `msgType`, `id`, `responseToId`, or `ackId` byte length exceeds 255 (not
  representable by a 1-byte prefix).
- `error` byte length exceeds 65535 (not representable by a 2-byte
  big-endian prefix).
- The frame is truncated relative to any declared length prefix (i.e. the
  frame ends before a field's declared length is satisfied).

## Conformance vectors

See [`wire-vectors-v1.json`](./wire-vectors-v1.json) for the canonical set of
encode/decode test vectors. Each vector has:

- `name`: a short identifier for the vector.
- `frame`: the logical frame contents. Keys are any of `msgType`,
  `controlCode`, `id`, `responseToId`, `ackId`, `error`, `binaryHex`.
  `controlCode` and `msgType` are mutually exclusive — a frame is either a
  control frame or a `msgType` frame, never both. `binaryHex` is a lowercase
  hex string representing the `binary` field's bytes, and is `""` when
  `binary` is empty.
- `hex`: the expected full wire encoding of `frame`, as a lowercase hex
  string.

Every implementation of this wire format (Go, TypeScript, Dart) MUST encode
each vector's `frame` to exactly `hex`, and MUST decode `hex` back to a
frame equivalent to `frame`.
