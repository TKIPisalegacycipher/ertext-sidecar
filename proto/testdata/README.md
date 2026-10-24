# Golden frames

Each `.bin` file is one protobuf-encoded `ertext.v1.Envelope` (no length prefix), made
deterministically by `internal/golden/golden_test.go`. Both implementations must decode them to:

| File | Content |
|---|---|
| `hello.bin` | `event.hello` = { protocol_version: 1, sidecar_version: "golden", libgm_version: "v0.0.0-golden" } |
| `send_message.bin` | `request` id 7, `send_message` = { conversation_id: "c1", outgoing_participant_id: "p-me", sim: { sim_number: 1, payload_kind: 1 }, text: "héllo 👋", reply_to_id: "m0", tmp_id: "tmp_1" } |
| `message_upserted.bin` | `event.message_upserted.message` = { id: "m1", tmp_id: "tmp_1", conversation_id: "c1", sender_participant_id: "p-me", timestamp_us: 1761300000000000, kind: RCS, status: DELIVERED, raw_status: 2, text: "héllo 👋", reply_to_id: "m0", from_me: true, media: [{ media_id: "md1", key: [1,2], thumb_media_id: "md1t", thumb_key: [3], mime: "image/jpeg", name: "pic.jpg", size: 1234, width: 640, height: 480 }], reactions: [{ emoji: "👍", participant_ids: ["p2"] }] }, is_old: false |
| `error_response.bin` | `response` id 9, `error` = { code: UNAUTHORIZED, message: "session invalid" } |

Change these only together with a schema change, and update the Rust test that reads them.
