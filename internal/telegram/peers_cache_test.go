package telegram

import (
	"context"
	"testing"

	"github.com/gotd/td/tg"
)

func TestStorePeerThenResolveNumericID(t *testing.T) {
	r := NewPeerResolver(nil)
	r.StorePeer(&tg.InputPeerChannel{ChannelID: 2082511793, AccessHash: 987654}, "school")

	peer, err := r.Resolve(context.Background(), "2082511793")
	if err != nil {
		t.Fatalf("数字 ID 应命中 StorePeer 缓存: %v", err)
	}
	ch, ok := peer.(*tg.InputPeerChannel)
	if !ok || ch.ChannelID != 2082511793 || ch.AccessHash != 987654 {
		t.Fatalf("peer = %#v", peer)
	}
}

func TestResolveMinus100IDUsesCachedRealID(t *testing.T) {
	r := NewPeerResolver(nil)
	r.StorePeer(&tg.InputPeerChannel{ChannelID: 2082511793, AccessHash: 42}, "")
	peer, err := r.Resolve(context.Background(), "-1002082511793")
	if err != nil {
		t.Fatalf("-100 ID 应命中 realID 缓存: %v", err)
	}
	if _, ok := peer.(*tg.InputPeerChannel); !ok {
		t.Fatalf("peer type = %T", peer)
	}
}
