package main

import (
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"math/cmplx"
	"net"
	"os"
	"os/signal"
	"strconv"
	"time"

	"github.com/bemasher/fftw"
	"github.com/bemasher/rtltcp"
)

const (
	BlockSize = 1 << 14

	SampleRate   = 2.4e6
	DataRate     = 32.768e3
	SymbolLength = SampleRate / DataRate

	PacketSymbols = 192
	PacketLength  = PacketSymbols * SymbolLength

	PreambleDFTSize = 20480

	CenterFreq    = 920299072
	Local         = ***REMOVED***
	RestrictLocal = false

	Preamble     = 0x1F2A60
	PreambleBits = "111110010101001100000"

	GenPoly    = 0x16F63
	MsgLen     = 10
	ErrorCount = 2

	TimeFormat = "2006-01-02T15:04:05.000"
)

var SymLen = IntRound(SymbolLength)
var config Config

type Config struct {
	serverAddr     string
	logFilename    string
	sampleFilename string

	ServerAddr *net.TCPAddr
	CenterFreq uint
	TimeLimit  time.Duration
	LogFile    *os.File
	SampleFile *os.File
}

func (c Config) String() string {
	return fmt.Sprintf("{ServerAddr:%s Freq:%d TimeLimit:%s LogFile:%s SampleFile:%s}",
		c.ServerAddr,
		c.CenterFreq,
		c.TimeLimit,
		c.LogFile.Name(),
		c.SampleFile.Name(),
	)
}

func (c *Config) Parse() (err error) {
	flag.Parse()

	c.ServerAddr, err = net.ResolveTCPAddr("tcp", c.serverAddr)
	if err != nil {
		return
	}

	if c.logFilename == "/dev/stdout" {
		c.LogFile = os.Stdout
	} else {
		c.LogFile, err = os.Create(c.logFilename)
	}
	if err != nil {
		return
	}

	log.SetOutput(c.LogFile)
	log.SetFlags(log.Lshortfile)

	c.SampleFile, err = os.Create(c.sampleFilename)
	if err != nil {
		return
	}

	return
}

func (c Config) Close() {
	c.LogFile.Close()
	c.SampleFile.Close()
}

type Receiver struct {
	rtltcp.SDR

	pd  PreambleDetector
	bch BCH
}

func NewReceiver(blockSize int) (rcvr Receiver) {
	rcvr.pd = NewPreambleDetector()

	rcvr.bch = NewBCH(GenPoly)
	log.Printf("BCH: %+v\n", rcvr.bch)

	if err := rcvr.Connect(config.ServerAddr); err != nil {
		log.Fatal(err)
	}

	rcvr.SetSampleRate(SampleRate)
	rcvr.SetCenterFreq(uint32(config.CenterFreq))
	rcvr.SetOffsetTuning(true)
	rcvr.SetAGCMode(true)
	rcvr.SetGainByIndex(23)
	rcvr.SetFreqCorrection(31)

	return
}

func (rcvr *Receiver) Close() {
	rcvr.SDR.Close()
	rcvr.pd.Close()
}

func (rcvr *Receiver) Run() {
	// Setup signal channel for interruption.
	sigint := make(chan os.Signal, 1)
	signal.Notify(sigint)

	// Allocate sample and demodulated signal buffers.
	block := make([]byte, BlockSize<<1)
	amBuf := make([]float64, BlockSize<<1)

	// Setup time limit channel
	tLimit := make(<-chan time.Time, 1)
	if config.TimeLimit != 0 {
		tLimit = time.After(config.TimeLimit)
	}

	start := time.Now()
	for {
		// Exit on interrupt or time limit, otherwise receive.
		select {
		case <-sigint:
			return
		case <-tLimit:
			fmt.Println("Time Limit Reached:", time.Since(start))
			return
		default:
			// Rotate sample buffer.
			copy(amBuf[:BlockSize], amBuf[BlockSize:])

			// Read new sample block.
			_, err := io.ReadFull(rcvr, block)
			if err != nil {
				log.Fatal("Error reading samples:", err)
			}

			// AM Demodulate
			for i := 0; i < BlockSize; i++ {
				amBuf[BlockSize+i] = Mag(block[i<<1], block[(i<<1)+1])
			}

			// Detect preamble in first half of demod buffer.
			copy(rcvr.pd.r, amBuf)
			align := rcvr.pd.Execute()

			// Bad framing, catch message on next block.
			if align > BlockSize {
				continue
			}

			// Filter signal and bit slice.
			filtered := MatchedFilter(amBuf[align:])
			bits := ""
			for i := range filtered {
				if filtered[i] > 0 {
					bits += "1"
				} else {
					bits += "0"
				}
			}

			// Convert bitstring to bytes for BCH.
			data := make([]byte, 10)
			for i := range data {
				idx := i<<3 + 16
				b, err := strconv.ParseUint(bits[idx:idx+8], 2, 8)
				if err != nil {
					log.Fatal("Error parsing byte:", err)
				}
				data[i] = byte(b)
			}

			// Calculate the syndrome to track which bits were corrected later
			// for logging.
			syn := rcvr.bch.Encode(data)

			// Correct errors
			checksum, corrected := rcvr.bch.Correct(data)

			// If the preamble matches and the corrected checksum is 0 we
			// probably have a message.
			if bits[:21] == PreambleBits && checksum == 0 {
				// Convert back to bitstring for parsing (should probably
				// write a method for parsing from bytes)
				bits = bits[:16]
				for i := range data {
					bits += fmt.Sprintf("%08b", data[i])
				}

				// Parse SCM
				scm, err := ParseSCM(bits)
				if err != nil {
					log.Fatal("Error parsing SCM:", err)
				}

				// Make sure checksum isn't 0 just because the received signal
				// evaluated to zero.
				if scm.ID != 0 {
					// Calculate message bounds.
					lower := align - IntRound(8*SymbolLength)
					if lower < 0 {
						lower = 0
					}
					upper := align + IntRound(PacketLength+8*SymbolLength)

					// Dump message to file.
					err = binary.Write(config.SampleFile, binary.LittleEndian, amBuf[lower:upper])
					if err != nil {
						log.Fatal("Error dumping samples:", err)
					}

					fmt.Fprintf(config.LogFile, "%+v ", scm)

					// If we corrected any errors, print their positions.
					if corrected {
						fmt.Fprintf(config.LogFile, "%d\n", rcvr.bch.Syndromes[syn])
					} else {
						fmt.Fprintln(config.LogFile)
					}
				}
			}
		}
	}
}

// Shift sample from unsigned and normalize.
func Mag(i, q byte) float64 {
	j := (127 - float64(i)) / 127
	k := (127 - float64(q)) / 127
	return math.Hypot(j, k)
}

// Preamble detection uses half-complex dft to convolve signal with preamble
// basis function, argmax of result represents most likely preamble position.
type PreambleDetector struct {
	forward  fftw.HCDFT1DPlan
	backward fftw.HCDFT1DPlan

	r        []float64
	c        []complex128
	template []complex128
}

func NewPreambleDetector() (pd PreambleDetector) {
	// Plan forward and reverse transforms.
	pd.forward = fftw.NewHCDFT1D(PreambleDFTSize, nil, nil, fftw.Forward, fftw.InPlace, fftw.Measure)
	pd.r = pd.forward.Real
	pd.c = pd.forward.Complex
	pd.backward = fftw.NewHCDFT1D(PreambleDFTSize, pd.r, pd.c, fftw.Backward, fftw.PreAlloc, fftw.Measure)

	// Zero out input array.
	for i := range pd.r {
		pd.r[i] = 0
	}

	// Generate the preamble basis function.
	for idx, bit := range PreambleBits {
		// Must account for rounding error.
		sIdx := idx << 1
		lower := IntRound(float64(sIdx) * SymbolLength)
		upper := IntRound(float64(sIdx+1) * SymbolLength)
		for i := 0; i < upper-lower; i++ {
			if bit == '1' {
				pd.r[lower+i] = 1.0
				pd.r[upper+i] = -1.0
			} else {
				pd.r[lower+i] = -1.0
				pd.r[upper+i] = 1.0
			}
		}
	}

	// Transform the preamble basis function.
	pd.forward.Execute()

	// Create the preamble template and store conjugated dft result.
	pd.template = make([]complex128, len(pd.c))
	copy(pd.template, pd.c)
	for i := range pd.template {
		pd.template[i] = cmplx.Conj(pd.template[i])
	}

	return
}

// FFTW plans must be cleaned up.
func (pd *PreambleDetector) Close() {
	pd.forward.Close()
	pd.backward.Close()
}

// Convolves signal with preamble basis function. Returns the most likely
// position of preamble. Assumes data has been copied into real array.
func (pd *PreambleDetector) Execute() int {
	pd.forward.Execute()
	for i := range pd.template {
		pd.backward.Complex[i] = pd.forward.Complex[i] * pd.template[i]
	}
	pd.backward.Execute()

	return pd.ArgMax()
}

// Calculate index of largest element in the real array.
func (pd *PreambleDetector) ArgMax() (idx int) {
	max := 0.0
	for i, v := range pd.backward.Real {
		if max < v {
			max, idx = v, i
		}
	}
	return idx
}

// Matched filter implemented as integrate and dump. Output array is equal to
// the number of manchester coded symbols per packet.
func MatchedFilter(input []float64) (output []float64) {
	output = make([]float64, IntRound(PacketSymbols/2))
	fidx := 0
	for idx := 0.0; fidx < 96; idx += SymbolLength * 2 {
		lower := IntRound(idx)
		upper := IntRound(idx + SymbolLength)
		for i := 0; i < upper-lower; i++ {
			output[fidx] += input[lower+i] - input[upper+i]
		}
		fidx++
	}
	return
}

func ParseUint(raw string) uint64 {
	tmp, _ := strconv.ParseUint(raw, 2, 64)
	return tmp
}

// Standard Consumption Message
type SCM struct {
	ID          uint32
	Type        uint8
	Tamper      Tamper
	Consumption uint32
	Checksum    uint16
}

func (scm SCM) String() string {
	return fmt.Sprintf("{ID:%8d Type:%2d Tamper:%+v Consumption:%8d Checksum:0x%04X}",
		scm.ID, scm.Type, scm.Tamper, scm.Consumption, scm.Checksum,
	)
}

type Tamper struct {
	Phy uint8
	Enc uint8
}

func (t Tamper) String() string {
	return fmt.Sprintf("{Phy:%d Enc:%d}", t.Phy, t.Enc)
}

// Given a string of bits, parse the message.
func ParseSCM(data string) (scm SCM, err error) {
	if len(data) != 96 {
		return scm, errors.New("invalid input length")
	}

	scm.ID = uint32(ParseUint(data[21:23] + data[56:80]))
	scm.Type = uint8(ParseUint(data[26:30]))
	scm.Tamper.Phy = uint8(ParseUint(data[24:26]))
	scm.Tamper.Enc = uint8(ParseUint(data[30:32]))
	scm.Consumption = uint32(ParseUint(data[32:56]))
	scm.Checksum = uint16(ParseUint(data[80:96]))

	return scm, nil
}

// BCH Error Correction
type BCH struct {
	GenPoly   uint
	PolyLen   byte
	Syndromes map[uint][]uint
}

// Given a generator polynomial, calculate the polynomial length and pre-
// compute syndromes for number of errors to be corrected.
func NewBCH(poly uint) (bch BCH) {
	bch.GenPoly = poly

	p := bch.GenPoly
	for ; bch.PolyLen < 32 && p > 0; bch.PolyLen, p = bch.PolyLen+1, p>>1 {
	}
	bch.PolyLen--

	bch.ComputeSyndromes(MsgLen, ErrorCount)

	return
}

func (bch BCH) String() string {
	return fmt.Sprintf("{GenPoly:%X PolyLen:%d Syndromes:%d}", bch.GenPoly, bch.PolyLen, len(bch.Syndromes))
}

// Recursively computes syndromes for number of desired errors.
func (bch *BCH) ComputeSyndromes(msgLen, errCount uint) {
	bch.Syndromes = make(map[uint][]uint)

	data := make([]byte, msgLen)
	bch.computeHelper(msgLen, errCount, nil, data)
}

func (bch *BCH) computeHelper(msgLen, depth uint, prefix []uint, data []byte) {
	if depth == 0 {
		return
	}

	// For all possible bit positions.
	for i := uint(0); i < msgLen<<3; i++ {
		inPrefix := false
		for p := uint(0); p < uint(len(prefix)) && !inPrefix; p++ {
			inPrefix = i == prefix[p]
		}
		if inPrefix {
			continue
		}

		// Toggle the bit
		data[i>>3] ^= 1 << uint(i%8)

		// Calculate the syndrome and store with position if new.
		syn := bch.Encode(data)
		if _, exists := bch.Syndromes[syn]; !exists {
			bch.Syndromes[syn] = append(prefix, i)
		}

		// Recurse.
		bch.computeHelper(msgLen, depth-1, append(prefix, i), data)

		data[i>>3] ^= 1 << uint(i%8)
	}
}

// Syndrome calculation implemented using LSFR (linear feedback shift register).
func (bch BCH) Encode(data []byte) (checksum uint) {
	// For each byte of data.
	for _, b := range data {
		// For each bit of byte.
		for i := byte(0); i < 8; i++ {
			// Rotate register and shift in bit.
			checksum = (checksum << 1) | uint((b>>(7-i))&1)
			// If MSB of register is non-zero XOR with generator polynomial.
			if checksum>>bch.PolyLen != 0 {
				checksum ^= bch.GenPoly
			}
		}
	}

	// Mask to valid length
	checksum &= (1 << bch.PolyLen) - 1
	return
}

// Given data, calculate the syndrome and correct errors if syndrome exists in
// pre-computed syndromes.
func (bch BCH) Correct(data []byte) (checksum uint, corrected bool) {
	// Calculate syndrome.
	syn := bch.Encode(data)
	if syn == 0 {
		return syn, false
	}

	// If the syndrome exists then toggle bits the syndrome was
	// calculated from.
	if pos, exists := bch.Syndromes[syn]; exists {
		for _, b := range pos {
			data[b>>3] ^= 1 << uint(b%8)
		}
	}

	// Calculate syndrome of corrected version. If we corrected anything, indicate so.
	checksum = bch.Encode(data)
	if syn != checksum && checksum == 0 {
		corrected = true
	}

	return
}

func IntRound(i float64) int {
	return int(math.Floor(i + 0.5))
}

func init() {
	flag.StringVar(&config.serverAddr, "server", "127.0.0.1:1234", "address or hostname of rtl_tcp instance")
	flag.StringVar(&config.logFilename, "logfile", "/dev/stdout", "log statement dump file")
	flag.StringVar(&config.sampleFilename, "samplefile", os.DevNull, "received message signal dump file")
	flag.UintVar(&config.CenterFreq, "centerfreq", 920299072, "center frequency to receive on")
	flag.DurationVar(&config.TimeLimit, "duration", 0, "time to run for, 0 for infinite")

	err := config.Parse()
	if err != nil {
		log.Fatal("Error parsing flags:", err)
	}
}

func main() {
	log.Println("Config:", config)
	log.Println("BlockSize:", BlockSize)
	log.Println("SampleRate:", SampleRate)
	log.Println("DataRate:", DataRate)
	log.Println("SymbolLength:", SymbolLength)
	log.Println("PacketSymbols:", PacketSymbols)
	log.Println("PacketLength:", PacketLength)
	log.Println("CenterFreq:", CenterFreq)

	rcvr := NewReceiver(BlockSize)
	defer rcvr.Close()
	defer config.Close()

	log.Println("Running...")
	rcvr.Run()
}
