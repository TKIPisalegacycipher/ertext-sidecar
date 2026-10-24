package translate

import "go.mau.fi/mautrix-gmessages/pkg/libgm/gmproto"

// Stats counts what a message carries, for debug logs. Counts only, never content.
type Stats struct {
	Text, Media, Location, Other int
	// EmptyMediaID counts media (including a location's) with no media_id to download.
	EmptyMediaID int
	// EmbeddedBytes is the total len(MediaData) of the message's media.
	EmbeddedBytes int
}

// MediaStats counts m's MessageInfo entries by type, the media with no media_id, and the bytes
// embedded in the message.
func MediaStats(m *gmproto.Message) Stats {
	var s Stats
	note := func(c *gmproto.MediaContent) {
		if c == nil {
			return
		}
		if c.GetMediaID() == "" {
			s.EmptyMediaID++
		}
		s.EmbeddedBytes += len(c.GetMediaData())
	}
	for _, info := range m.GetMessageInfo() {
		switch d := info.GetData().(type) {
		case *gmproto.MessageInfo_MessageContent:
			s.Text++
		case *gmproto.MessageInfo_MediaContent:
			s.Media++
			note(d.MediaContent)
		case *gmproto.MessageInfo_LocationContent:
			s.Location++
			note(d.LocationContent.GetMediaContent())
		default:
			s.Other++
		}
	}
	return s
}
