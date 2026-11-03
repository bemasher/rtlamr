package rtltcp

import (
	"encoding/binary"
	"io"
	"net"
	"sync"
)

// sampleChunkSize matches the buffer size the rtl-sdr library hands to
// rtl_tcp's async callback, so the stream looks the same to a client.
const sampleChunkSize = 16384

// MockServer emulates the TCP server implemented by rtl_tcp: it sends the
// 12 byte dongle info on accept, streams samples continuously, and interprets
// the 5 byte command frames. Commands are never acknowledged, matching
// rtl_tcp, so received values are observed through the On* callbacks, which
// run on the server's goroutine.
//
// Callbacks may be registered or replaced at any time, including from inside
// another callback. Info and Samples are read when a client connects, so set
// them before that.
//
// Unlike rtl_tcp this server does not exit when it cannot make progress, and
// only serves one client at a time.
type MockServer struct {
	// Info is sent verbatim in the handshake.
	Info Info

	// Samples is repeated cyclically as the sample stream. If empty, zeros
	// are sent.
	Samples []byte

	addr *net.TCPAddr
	ln   *net.TCPListener
	done chan struct{}

	mu     sync.Mutex
	conn   *net.TCPConn
	closed bool

	cbMu sync.RWMutex
	cb   callbacks
}

// callbacks holds the handler for each command. dispatch copies it under cbMu
// before calling anything, so a callback may register another without
// deadlocking.
type callbacks struct {
	centerFreq     func(uint32)
	sampleRate     func(uint32)
	gainMode       func(uint32)
	gain           func(uint32)
	freqCorrection func(int32)
	tunerIfGain    func(stage, gain int16)
	testMode       func(uint32)
	agcMode        func(uint32)
	directSampling func(uint32)
	offsetTuning   func(uint32)
	rtlXtalFreq    func(uint32)
	tunerXtalFreq  func(uint32)
	gainByIndex    func(uint32)
	biasTee        func(uint32)
	unknown        func(cmd uint8, param uint32)
}

// NewMockServer returns an unstarted server. A nil addr listens on an
// ephemeral port on 127.0.0.1. The default Info reports an R820T tuner.
func NewMockServer(addr *net.TCPAddr) *MockServer {
	if addr == nil {
		addr = &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)}
	}
	return &MockServer{
		Info: Info{Magic: magic, Tuner: 5, GainCount: 29},
		addr: addr,
		done: make(chan struct{}),
	}
}

// Start begins listening and serving in the background.
func (m *MockServer) Start() (err error) {
	m.ln, err = net.ListenTCP("tcp", m.addr)
	if err != nil {
		return
	}
	m.addr = m.ln.Addr().(*net.TCPAddr)
	go m.serve()
	return
}

// Addr returns the address to connect to, available after Start.
func (m *MockServer) Addr() *net.TCPAddr {
	return m.addr
}

// Close stops accepting connections, drops the active client and waits for
// the server goroutine to exit. Once it returns no callback will run again.
func (m *MockServer) Close() error {
	m.mu.Lock()
	if m.closed || m.ln == nil {
		m.closed = true
		m.mu.Unlock()
		return nil
	}
	m.closed = true
	conn := m.conn
	m.mu.Unlock()

	m.ln.Close()
	if conn != nil {
		conn.Close()
	}
	<-m.done
	return nil
}

// OnCenterFreq sets the callback for command 0x01.
func (m *MockServer) OnCenterFreq(fn func(uint32)) { m.set(func(cb *callbacks) { cb.centerFreq = fn }) }

// OnSampleRate sets the callback for command 0x02.
func (m *MockServer) OnSampleRate(fn func(uint32)) { m.set(func(cb *callbacks) { cb.sampleRate = fn }) }

// OnGainMode sets the callback for command 0x03, the value passed to
// rtlsdr_set_tuner_gain_mode.
func (m *MockServer) OnGainMode(fn func(uint32)) { m.set(func(cb *callbacks) { cb.gainMode = fn }) }

// OnGain sets the callback for command 0x04, in tenths of a dB.
func (m *MockServer) OnGain(fn func(uint32)) { m.set(func(cb *callbacks) { cb.gain = fn }) }

// OnFreqCorrection sets the callback for command 0x05, in ppm.
func (m *MockServer) OnFreqCorrection(fn func(int32)) {
	m.set(func(cb *callbacks) { cb.freqCorrection = fn })
}

// OnTunerIfGain sets the callback for command 0x06, in tenths of a dB.
func (m *MockServer) OnTunerIfGain(fn func(stage, gain int16)) {
	m.set(func(cb *callbacks) { cb.tunerIfGain = fn })
}

// OnTestMode sets the callback for command 0x07.
func (m *MockServer) OnTestMode(fn func(uint32)) { m.set(func(cb *callbacks) { cb.testMode = fn }) }

// OnAGCMode sets the callback for command 0x08.
func (m *MockServer) OnAGCMode(fn func(uint32)) { m.set(func(cb *callbacks) { cb.agcMode = fn }) }

// OnDirectSampling sets the callback for command 0x09.
func (m *MockServer) OnDirectSampling(fn func(uint32)) {
	m.set(func(cb *callbacks) { cb.directSampling = fn })
}

// OnOffsetTuning sets the callback for command 0x0a.
func (m *MockServer) OnOffsetTuning(fn func(uint32)) {
	m.set(func(cb *callbacks) { cb.offsetTuning = fn })
}

// OnRTLXtalFreq sets the callback for command 0x0b.
func (m *MockServer) OnRTLXtalFreq(fn func(uint32)) {
	m.set(func(cb *callbacks) { cb.rtlXtalFreq = fn })
}

// OnTunerXtalFreq sets the callback for command 0x0c.
func (m *MockServer) OnTunerXtalFreq(fn func(uint32)) {
	m.set(func(cb *callbacks) { cb.tunerXtalFreq = fn })
}

// OnGainByIndex sets the callback for command 0x0d. rtl_tcp resolves the
// index to a gain first; that resolution is not observable over the wire.
func (m *MockServer) OnGainByIndex(fn func(uint32)) {
	m.set(func(cb *callbacks) { cb.gainByIndex = fn })
}

// OnBiasTee sets the callback for command 0x0e.
func (m *MockServer) OnBiasTee(fn func(uint32)) { m.set(func(cb *callbacks) { cb.biasTee = fn }) }

// OnUnknown sets the callback for any unrecognized command, which rtl_tcp
// ignores.
func (m *MockServer) OnUnknown(fn func(cmd uint8, param uint32)) {
	m.set(func(cb *callbacks) { cb.unknown = fn })
}

func (m *MockServer) set(apply func(*callbacks)) {
	m.cbMu.Lock()
	defer m.cbMu.Unlock()
	apply(&m.cb)
}

func (m *MockServer) serve() {
	defer close(m.done)
	for {
		conn, err := m.ln.AcceptTCP()
		if err != nil {
			return
		}

		m.mu.Lock()
		if m.closed {
			m.mu.Unlock()
			conn.Close()
			return
		}
		m.conn = conn
		m.mu.Unlock()

		m.handle(conn)

		m.mu.Lock()
		m.conn = nil
		m.mu.Unlock()
		conn.Close()
	}
}

// handle serves one client: handshake, then the sample and command workers
// run until either fails, like rtl_tcp's two worker threads.
func (m *MockServer) handle(conn *net.TCPConn) {
	var hdr [12]byte
	copy(hdr[:], m.Info.Magic[:])
	binary.BigEndian.PutUint32(hdr[4:], uint32(m.Info.Tuner))
	binary.BigEndian.PutUint32(hdr[8:], m.Info.GainCount)
	if _, err := conn.Write(hdr[:]); err != nil {
		return
	}

	chunk := make([]byte, sampleChunkSize)
	for i := range chunk {
		if len(m.Samples) > 0 {
			chunk[i] = m.Samples[i%len(m.Samples)]
		}
	}

	dead := make(chan struct{}, 2)
	go func() {
		for {
			if _, err := conn.Write(chunk); err != nil {
				break
			}
		}
		dead <- struct{}{}
	}()
	go func() {
		var buf [5]byte
		for {
			if _, err := io.ReadFull(conn, buf[:]); err != nil {
				break
			}
			m.dispatch(buf[0], binary.BigEndian.Uint32(buf[1:]))
		}
		dead <- struct{}{}
	}()

	<-dead
	conn.Close()
	<-dead
}

func (m *MockServer) dispatch(cmd uint8, param uint32) {
	m.cbMu.RLock()
	cb := m.cb
	m.cbMu.RUnlock()

	switch cmd {
	case centerFreq:
		if cb.centerFreq != nil {
			cb.centerFreq(param)
		}
	case sampleRate:
		if cb.sampleRate != nil {
			cb.sampleRate(param)
		}
	case tunerGainMode:
		if cb.gainMode != nil {
			cb.gainMode(param)
		}
	case tunerGain:
		if cb.gain != nil {
			cb.gain(param)
		}
	case freqCorrection:
		if cb.freqCorrection != nil {
			cb.freqCorrection(int32(param))
		}
	case tunerIfGain:
		if cb.tunerIfGain != nil {
			cb.tunerIfGain(int16(param>>16), int16(param))
		}
	case testMode:
		if cb.testMode != nil {
			cb.testMode(param)
		}
	case agcMode:
		if cb.agcMode != nil {
			cb.agcMode(param)
		}
	case directSampling:
		if cb.directSampling != nil {
			cb.directSampling(param)
		}
	case offsetTuning:
		if cb.offsetTuning != nil {
			cb.offsetTuning(param)
		}
	case rtlXtalFreq:
		if cb.rtlXtalFreq != nil {
			cb.rtlXtalFreq(param)
		}
	case tunerXtalFreq:
		if cb.tunerXtalFreq != nil {
			cb.tunerXtalFreq(param)
		}
	case gainByIndex:
		if cb.gainByIndex != nil {
			cb.gainByIndex(param)
		}
	case biasTee:
		if cb.biasTee != nil {
			cb.biasTee(param)
		}
	default:
		if cb.unknown != nil {
			cb.unknown(cmd, param)
		}
	}
}
