// Package whisper is a thin client around the gRPC Transcriber service.
package whisper

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/santiago-jauregui/logger-bot/internal/whisper/pb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

const (
	maxMsgBytes    = 32 * 1024 * 1024 // 32MB; covers Telegram's 20MB cap
	defaultTimeout = 60 * time.Second
)

type Client struct {
	conn *grpc.ClientConn
	stub pb.TranscriberClient
}

func NewClient(addr string) (*Client, error) {
	conn, err := grpc.NewClient(
		addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(maxMsgBytes),
			grpc.MaxCallSendMsgSize(maxMsgBytes),
		),
		grpc.WithDefaultServiceConfig(`{
			"loadBalancingPolicy": "round_robin",
			"healthCheckConfig": {"serviceName": ""}
		}`),
	)
	if err != nil {
		return nil, fmt.Errorf("dial whisper: %w", err)
	}
	return &Client{conn: conn, stub: pb.NewTranscriberClient(conn)}, nil
}

func (c *Client) Close() error { return c.conn.Close() }

// Transcribe sends audio bytes to the Whisper service and returns the text.
// Generates an x-request-id and propagates it via metadata.
func (c *Client) Transcribe(ctx context.Context, audio []byte, mimeType string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()
	ctx = metadata.AppendToOutgoingContext(ctx, "x-request-id", newRequestID())

	resp, err := c.stub.Transcribe(ctx, &pb.TranscribeRequest{
		Audio:    audio,
		MimeType: mimeType,
	})
	if err != nil {
		return "", err
	}
	return resp.GetText(), nil
}

func newRequestID() string {
	var b [4]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}