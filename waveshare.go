package waveshare

import (
	"cmp"
	"fmt"
	"image"
	"time"

	"periph.io/x/conn/v3/gpio"
	"periph.io/x/conn/v3/gpio/gpioreg"
	"periph.io/x/conn/v3/physic"
	"periph.io/x/conn/v3/spi"
	"periph.io/x/conn/v3/spi/spireg"
	"periph.io/x/host/v3"
)

const (
	// Width is the horizontal resolution of the display in pixels.
	Width = 400
	// Height is the vertical resolution of the display in pixels.
	Height = 300
)

// Default GPIO pin names for the Waveshare HAT on Raspberry Pi.
const (
	DefaultPinRST  = "GPIO17"
	DefaultPinDC   = "GPIO25"
	DefaultPinCS   = "GPIO8"
	DefaultPinBUSY = "GPIO24"
)

// Config holds the hardware configuration for the display.
type Config struct {
	// SPI bus identifier. Empty string selects the default bus.
	SPIBus string
	// SPI clock frequency. Defaults to 2 MHz if zero.
	SPIFrequency physic.Frequency
	// GPIO pin names. Empty strings use the defaults.
	PinRST  string
	PinDC   string
	PinCS   string
	PinBUSY string
}

// EPD is the e-Paper display driver.
type EPD struct {
	port spi.PortCloser
	conn spi.Conn
	rst  gpio.PinOut
	dc   gpio.PinOut
	cs   gpio.PinOut
	busy gpio.PinIn
}

// New initializes the display with default configuration.
func New() (*EPD, error) {
	return NewWithConfig(Config{})
}

// NewWithConfig initializes the display with a custom configuration.
func NewWithConfig(cfg Config) (*EPD, error) {
	if _, err := host.Init(); err != nil {
		return nil, fmt.Errorf("waveshare: host init: %w", err)
	}

	freq := cmp.Or(cfg.SPIFrequency, 2*physic.MegaHertz)

	port, err := spireg.Open(cfg.SPIBus)
	if err != nil {
		return nil, fmt.Errorf("waveshare: spi open: %w", err)
	}

	conn, err := port.Connect(freq, spi.Mode0, 8)
	if err != nil {
		port.Close()
		return nil, fmt.Errorf("waveshare: spi connect: %w", err)
	}

	pins := map[string]string{
		"RST":  cmp.Or(cfg.PinRST, DefaultPinRST),
		"DC":   cmp.Or(cfg.PinDC, DefaultPinDC),
		"CS":   cmp.Or(cfg.PinCS, DefaultPinCS),
		"BUSY": cmp.Or(cfg.PinBUSY, DefaultPinBUSY),
	}

	rst := gpioreg.ByName(pins["RST"])
	dc := gpioreg.ByName(pins["DC"])
	cs := gpioreg.ByName(pins["CS"])
	busy := gpioreg.ByName(pins["BUSY"])

	for name, pin := range map[string]gpio.PinIO{"RST": rst, "DC": dc, "CS": cs, "BUSY": busy} {
		if pin == nil {
			port.Close()
			return nil, fmt.Errorf("waveshare: GPIO pin %s (%s) not found", name, pins[name])
		}
	}

	rst.Out(gpio.High)
	dc.Out(gpio.Low)
	cs.Out(gpio.High)
	busy.In(gpio.PullUp, gpio.NoEdge)

	epd := &EPD{
		port: port,
		conn: conn,
		rst:  rst,
		dc:   dc,
		cs:   cs,
		busy: busy,
	}

	epd.reset()
	epd.initDisplay()
	return epd, nil
}

// Close puts the display to sleep and releases all hardware resources.
// Always call this when done to protect the display panel.
func (e *EPD) Close() error {
	e.sleep()
	return e.port.Close()
}

// Clear fills the entire display with white.
func (e *EPD) Clear() {
	e.sendCommand(0x10)
	e.sendRawBytes(0xFF, Width*Height/8)
	e.sendCommand(0x13)
	e.sendRawBytes(0xFF, Width*Height/8)
	e.refreshFull()
}

// DisplayImage performs a full-screen refresh with the provided image using 4 grayscales.
// The image must be at least Width x Height pixels.
func (e *EPD) DisplayImage(img image.Image) {
	e.sendCommand(0x10)
	e.writeGrayBits(img, 0x10)

	e.sendCommand(0x13)
	e.writeGrayBits(img, 0x13)

	e.loadGrayLUT()
	e.refreshFull()
}

// --- Low-level helpers (unexported) ---

func (e *EPD) reset() {
	e.rst.Out(gpio.High)
	time.Sleep(200 * time.Millisecond)
	e.rst.Out(gpio.Low)
	time.Sleep(2 * time.Millisecond)
	e.rst.Out(gpio.High)
	time.Sleep(200 * time.Millisecond)
}

func (e *EPD) sendCommand(cmd byte) {
	e.dc.Out(gpio.Low)
	e.cs.Out(gpio.Low)
	e.conn.Tx([]byte{cmd}, nil)
	e.cs.Out(gpio.High)
}

func (e *EPD) sendData(data ...byte) {
	e.dc.Out(gpio.High)
	e.cs.Out(gpio.Low)
	e.conn.Tx(data, nil)
	e.cs.Out(gpio.High)
}

// sendRawBytes sends `count` bytes of `val` as data.
func (e *EPD) sendRawBytes(val byte, count int) {
	buf := make([]byte, count)
	for i := range buf {
		buf[i] = val
	}
	e.dc.Out(gpio.High)
	e.cs.Out(gpio.Low)
	e.conn.Tx(buf, nil)
	e.cs.Out(gpio.High)
}

func (e *EPD) waitIdle() {
	for e.busy.Read() == gpio.Low {
		time.Sleep(10 * time.Millisecond)
	}
}

func (e *EPD) waitIdleTimeout(maxWait time.Duration) {
	deadline := time.Now().Add(maxWait)
	for e.busy.Read() == gpio.Low {
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (e *EPD) initDisplay() {
	e.waitIdle()

	e.sendCommand(0x01) // POWER SETTING
	e.sendData(0x03, 0x00, 0x2b, 0x2b, 0x13)

	e.sendCommand(0x06) // booster soft start
	e.sendData(0x17, 0x17, 0x17)

	e.sendCommand(0x04)                // POWER ON
	time.Sleep(100 * time.Millisecond) // Give the IC time to pull BUSY low
	e.waitIdle()

	e.sendCommand(0x00) // panel setting
	e.sendData(0x3F)    // KW-3f KWR-2F BWROTP 0f BWOTP 1f

	e.sendCommand(0x30) // PLL setting
	e.sendData(0x3c)

	e.sendCommand(0x61)                // resolution setting
	e.sendData(0x01, 0x90, 0x01, 0x2c) // 400x300

	e.sendCommand(0x82) // vcom_DC setting
	e.sendData(0x12)

	e.sendCommand(0x50) // VCOM AND DATA INTERVAL SETTING
	e.sendData(0x97)
}

func (e *EPD) refreshFull() {
	e.sendCommand(0x12)
	e.waitIdle()
}

func (e *EPD) sleep() {
	e.sendCommand(0x50)
	e.sendData(0xF7)

	e.sendCommand(0x02) // Power off
	e.waitIdleTimeout(3 * time.Second)

	e.sendCommand(0x07) // Deep sleep
	e.sendData(0xA5)
}

// writeGrayBits sends image pixel data translated to 4 grayscales.
func (e *EPD) writeGrayBits(img image.Image, reg byte) {
	for y := range Height {
		for x := 0; x < Width; x += 8 {
			var b byte
			for i := range 8 {
				r, g, bl, _ := img.At(x+i, y).RGBA()
				lum := (r*299 + g*587 + bl*114) / 1000

				level := 0
				if lum >= 49151 { // White
					level = 3
				} else if lum >= 32768 { // Light Gray
					level = 2
				} else if lum >= 16384 { // Dark Gray
					level = 1
				} else { // Black
					level = 0
				}

				bit := byte(0)
				switch reg {
				case 0x10:
					if level == 3 || level == 2 {
						bit = 1
					}
				case 0x13:
					if level == 3 || level == 1 {
						bit = 1
					}
				}
				b |= (bit << (7 - i))
			}
			e.sendData(b)
		}
	}
}

// loadGrayLUT updates the lookup tables for 4 grayscale mode.
func (e *EPD) loadGrayLUT() {
	lutVCOM := []byte{0x00, 0x0A, 0x00, 0x00, 0x00, 0x01, 0x60, 0x14, 0x14, 0x00, 0x00, 0x01, 0x00, 0x14, 0x00, 0x00, 0x00, 0x01, 0x00, 0x13, 0x0A, 0x01, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	lutWW := []byte{0x40, 0x0A, 0x00, 0x00, 0x00, 0x01, 0x90, 0x14, 0x14, 0x00, 0x00, 0x01, 0x10, 0x14, 0x0A, 0x00, 0x00, 0x01, 0xA0, 0x13, 0x01, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	lutBW := []byte{0x40, 0x0A, 0x00, 0x00, 0x00, 0x01, 0x90, 0x14, 0x14, 0x00, 0x00, 0x01, 0x00, 0x14, 0x0A, 0x00, 0x00, 0x01, 0x99, 0x0C, 0x01, 0x03, 0x04, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	lutWB := []byte{0x40, 0x0A, 0x00, 0x00, 0x00, 0x01, 0x90, 0x14, 0x14, 0x00, 0x00, 0x01, 0x00, 0x14, 0x0A, 0x00, 0x00, 0x01, 0x99, 0x0B, 0x04, 0x04, 0x01, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	lutBB := []byte{0x80, 0x0A, 0x00, 0x00, 0x00, 0x01, 0x90, 0x14, 0x14, 0x00, 0x00, 0x01, 0x20, 0x14, 0x0A, 0x00, 0x00, 0x01, 0x50, 0x13, 0x01, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}

	e.sendCommand(0x20)
	e.sendData(lutVCOM...)
	e.sendCommand(0x21)
	e.sendData(lutWW...)
	e.sendCommand(0x22)
	e.sendData(lutBW...)
	e.sendCommand(0x23)
	e.sendData(lutWB...)
	e.sendCommand(0x24)
	e.sendData(lutBB...)
	e.sendCommand(0x25)
	e.sendData(lutWW...)
}
