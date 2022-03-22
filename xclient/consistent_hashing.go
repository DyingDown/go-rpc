package xclient

type ConsistentHash struct {
	Servers        []string
	VirtualServers []uint32
	correlation    map[string]uint32
}
