package network

const (
	// OptimalPacketSize for credentials (avoid fragmentation)
	OptimalPacketSize = 4096
)

type PacketOptimizer struct {
	bufferSize int
}

func NewPacketOptimizer() *PacketOptimizer {
	return &PacketOptimizer{
		bufferSize: OptimalPacketSize,
	}
}

func (p *PacketOptimizer) BufferSize() int {
	return p.bufferSize
}
