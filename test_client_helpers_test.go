package codex

import (
	"context"

	"github.com/mel2oo/codex-sdk-go/protocol"
	"github.com/mel2oo/codex-sdk-go/rpc"
)

func newReplayClient(ctx context.Context, transport rpc.Transport, info protocol.ClientInfo) (*Client, error) {
	return NewClient(ctx, WithTransport(transport), WithClientInfo(info))
}
