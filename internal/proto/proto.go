package proto

// Headers contains basic JSON headers, which
// every message should have.
type Headers struct {
	SrcAddr string `json:"src_addr"` // IP:PORT or path to UNIX-socket.
	Immed   bool   `json:"immed"`    // If true, there is no additional data in message (so put it into queue).
}
