package network

import (
	"context"
	"encoding/json"
)

type CredentialRequest struct {
	Service string `json:"service"`
}

type CredentialsProxy struct {
	transport Transport
}

func NewCredentialsProxy(transport Transport) *CredentialsProxy {
	return &CredentialsProxy{
		transport: transport,
	}
}

func (c *CredentialsProxy) SendRequest(ctx context.Context, req *CredentialRequest) error {
	conn, err := c.transport.Dial(ctx, "")
	if err != nil {
		return err
	}
	defer conn.Close()

	// Send request
	data, err := json.Marshal(req)
	if err != nil {
		return err
	}

	_, err = conn.Write(data)
	return err
}
