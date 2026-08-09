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

## Empty vs. absent

The four optional fields `id`, `responseToId`, `ackId`, and `error` use
**empty-string-means-absent** semantics: a zero-length value is never
transmitted on the wire — its flag bit (byte 0) stays clear, exactly as if
the field had not been set at all. Consequently, an intentionally-empty
value and a genuinely-absent value are **indistinguishable on the wire**:
there is no encoding that represents "present but empty" for these fields.

This is intentional, not an oversight, and every implementation of this
spec (Go, TypeScript, Dart) MUST follow it:

- **Encoding:** if one of these fields' logical value is the empty string,
  its flag bit MUST be left clear and no bytes for that field MUST be
  written, regardless of whether the caller considered the field "set to
  empty" or "not set".
- **Decoding:** if a field's flag bit is clear, the decoded value MUST be
  the empty string (not `null`/`None`/absent-as-a-distinct-state, in
  languages where that distinction exists in the target type).

`binary` is exempt from this rule: an empty `binary` is simply zero
trailing bytes and is always representable (and is indistinguishable from
absent `binary`, which is consistent with this same field always being
present as "the remainder").

## Invalid UTF-8

`msgType`, `id`, `responseToId`, `ackId`, and `error` are declared as UTF-8
byte strings, but a decoder MUST NOT assume the bytes on the wire are
well-formed UTF-8, and MUST NOT reject a frame solely because one of these
fields contains invalid UTF-8.

- Decoders MUST either retain the raw bytes as-is (Go, whose `string` type
  is an opaque byte sequence with no UTF-8 validity requirement) or
  substitute the Unicode replacement character (U+FFFD) for invalid
  sequences (TypeScript, Dart, and any language whose string type cannot
  hold arbitrary bytes).
- Decoders MUST NOT throw, panic, or otherwise fail the decode because of
  invalid UTF-8 in these fields.

This means a decode→encode round trip is **not guaranteed to be byte-stable**
for a field containing invalid UTF-8, in a language that substitutes
replacement characters: the substituted U+FFFD sequence re-encodes to
different bytes than the original invalid sequence. This is intentional —
rejecting the frame would be worse, since the field's content is opaque
application data as far as the wire format is concerned.

## Non-canonical frames

A decoder MUST NOT enforce canonical encoding of the optional fields beyond
what "Decode error rules" (below) requires. In particular: a frame in which
one of `id`, `responseToId`, `ackId`, or `error`'s flag bit is **set** but
the field's declared length is **0** MUST be accepted, and MUST decode to
the empty string — identical to the outcome when the flag bit is clear
(field absent). A decoder MUST NOT reject this as malformed or
inconsistent.

Such a frame is non-canonical: a conforming encoder never produces it (see
"Empty vs. absent" above — an encoder always clears the flag bit for an
empty value), but a decoder still has to accept it, since it is fully
parseable and any other behavior would make decoding depend on how the
frame happened to be produced rather than on its bytes. As with invalid
UTF-8, this means decode→encode is not byte-stable for such a frame: it
re-encodes shorter than the original (with the flag bit cleared), since the
canonical encoding of an empty value never includes the field at all.

## Unknown control codes

A decoder MUST NOT reject a control frame (`msgTypeLen == 0`) merely
because its control-code byte is not one it recognizes. It MUST decode the
frame successfully, surfacing the control code and the frame's other fields
(binary, id, etc.) intact, and let the application layer decide what to do
with an unrecognized code (e.g. ignore it, log it, or route it elsewhere).

Only two control codes are defined in v1: `0xCA` (heartbeat) and `0xCB`
(ack). This rule exists so that a future v1.x addition of a new control
code — or a newer peer sending one — degrades gracefully on an older
decoder instead of being rejected outright.

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

## Migration and rollout

This is a **breaking wire-format change** from whatever the server and
clients spoke before v1. There is no dual-read support: a given connection
speaks exactly one version, and there is no negotiation beyond the version
nibble in byte 0.

- **Mixed-version fleets are unsupported.** A server and client on
  different wire versions cannot interoperate on the same connection.
  There is no fallback, no auto-detection beyond rejecting the mismatch,
  and no partial compatibility.
- **The version nibble is the only negotiation mechanism.** It exists to
  make a mismatch fail safely (see "Decode error rules"), not to negotiate
  a common version. Deploying v1 requires coordinating server and client
  rollout — this is an operational cutover, not a protocol-level
  auto-upgrade.
- **Both mismatch directions fail closed, not silently.** An old client
  talking to a new server (or vice versa) always hits a decode error on
  the receiving side rather than misinterpreting bytes as something else
  (see the version-nibble discussion above: msgpack's leading bytes never
  alias a valid v1 version nibble, so corruption is not possible). This
  fails safely, but it still requires a coordinated cutover — it is not a
  substitute for one.
- **A rollback after clients have shipped re-breaks every updated
  client.** If the server rolls back to a pre-v1 wire format after clients
  have already been updated to speak v1, every updated client breaks
  again, the same way it would have broken during a naive forward
  migration. Plan rollback the same way you'd plan the forward cutover —
  as a coordinated, fleet-wide change — not as an independent, low-risk
  escape hatch.
