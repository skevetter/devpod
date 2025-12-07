package network

import "context"

type FallbackTransport struct {
	primary  Transport
	fallback Transport
}

func NewFallbackTransport(primary, fallback Transport) *FallbackTransport {
	return &FallbackTransport{
		primary:  primary,
		fallback: fallback,
	}
}

func (f *FallbackTransport) Dial(ctx context.Context, target string) (Conn, error) {
	conn, err := f.primary.Dial(ctx, target)
	if err == nil {
		return conn, nil
	}

	// Primary failed, try fallback
	return f.fallback.Dial(ctx, target)
}

func (f *FallbackTransport) Close() error {
	_ = f.primary.Close()
	_ = f.fallback.Close()
	return nil
}
