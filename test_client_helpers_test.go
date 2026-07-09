package codex

import (
	"context"

	"github.com/openai/codex/sdk/go/protocol"
	"github.com/openai/codex/sdk/go/rpc"
)

func newReplayClient(ctx context.Context, transport rpc.Transport, info protocol.ClientInfo) (*Client, error) {
	return NewClient(ctx, WithTransport(transport), WithClientInfo(info))
}
