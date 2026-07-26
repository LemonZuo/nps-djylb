package api

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"image"
	"image/color"
	"image/png"
	"math"
	"strings"
	"sync"
	"time"
)

// A small self-contained captcha. The previous implementation came from beego's
// utils/captcha, which stores its challenges in a beego cache instance and so
// cannot be used without the framework. Rather than take a new dependency for
// four digits and a PNG, the whole thing is here: challenge storage, glyph
// rendering and verification.
//
// The threat model is modest and unchanged from before — it exists to slow down
// scripted login attempts, backed up by the proof-of-work and ban layers that
// do the real work. It is not expected to defeat a determined OCR attack.

const (
	// captchaDigits is how many characters the user must type.
	captchaDigits = 4

	// captchaW and captchaH are the rendered image dimensions. The old login
	// page used 100x50; this is larger because the bitmap font below is coarse
	// and needs the room to stay legible once it has been distorted.
	captchaW = 160
	captchaH = 60

	// captchaTTL is how long a challenge remains solvable.
	captchaTTL = 5 * time.Minute

	// maxCaptchas caps the store; the endpoint issuing them is unauthenticated.
	maxCaptchas = 20000
)

// Challenge is an issued captcha, ready to be handed to the client.
type Challenge struct {
	// ID identifies the challenge on the way back in.
	ID string `json:"id"`
	// Image is a data: URI holding the PNG, so the SPA can drop it straight
	// into an <img src>. Serving it inline avoids a second unauthenticated
	// endpoint and keeps the challenge and its id inseparable.
	Image string `json:"image"`
}

type captchaEntry struct {
	code    string
	expires time.Time
}

type captchaStore struct {
	mu     sync.Mutex
	items  map[string]captchaEntry
	sweep  time.Time
	nowFn  func() time.Time
	maxLen int
}

func newCaptchaStore() *captchaStore {
	return &captchaStore{
		items:  make(map[string]captchaEntry),
		nowFn:  time.Now,
		maxLen: maxCaptchas,
	}
}

var captchas = newCaptchaStore()

// NewCaptcha renders a fresh challenge. It returns an error only if the system
// random source or the PNG encoder fails.
func NewCaptcha() (*Challenge, error) {
	return captchas.issue()
}

// VerifyCaptcha checks a submitted code, consuming the challenge either way so
// that a wrong guess cannot be retried against the same image.
func VerifyCaptcha(id, code string) bool {
	return captchas.verify(id, code)
}

func (s *captchaStore) issue() (*Challenge, error) {
	code, err := randomDigits(captchaDigits)
	if err != nil {
		return nil, err
	}
	id, err := randomID()
	if err != nil {
		return nil, err
	}
	img, err := renderCaptcha(code)
	if err != nil {
		return nil, err
	}

	now := s.nowFn()
	s.mu.Lock()
	s.sweepLocked(now)
	if len(s.items) >= s.maxLen {
		s.mu.Unlock()
		return nil, errStoreFull
	}
	s.items[id] = captchaEntry{code: code, expires: now.Add(captchaTTL)}
	s.mu.Unlock()

	return &Challenge{
		ID:    id,
		Image: "data:image/png;base64," + base64.StdEncoding.EncodeToString(img),
	}, nil
}

func (s *captchaStore) verify(id, code string) bool {
	if id == "" || code == "" {
		return false
	}
	now := s.nowFn()

	s.mu.Lock()
	entry, ok := s.items[id]
	delete(s.items, id)
	s.mu.Unlock()

	if !ok || !now.Before(entry.expires) {
		return false
	}
	// Case folding costs nothing and the glyph set is digits only, but trimming
	// matters: browsers and password managers like to add whitespace.
	return strings.EqualFold(strings.TrimSpace(code), entry.code)
}

func (s *captchaStore) sweepLocked(now time.Time) {
	if len(s.items) < s.maxLen && now.Sub(s.sweep) < time.Minute {
		return
	}
	s.sweep = now
	for k, v := range s.items {
		if !now.Before(v.expires) {
			delete(s.items, k)
		}
	}
}

func (s *captchaStore) len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.items)
}

// errStoreFull is returned when the challenge store is at its cap.
var errStoreFull = &storeFullError{}

type storeFullError struct{}

func (*storeFullError) Error() string { return "captcha store is full" }

func randomID() (string, error) {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// randomDigits builds the answer. crypto/rand is used rather than math/rand
// because a predictable answer defeats the whole control.
func randomDigits(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	out := make([]byte, n)
	for i, b := range buf {
		// A 256-value byte reduced mod 10 is very slightly biased towards the
		// low digits. That bias is irrelevant to a 4-digit human challenge, and
		// rejection sampling here would only add failure modes.
		out[i] = '0' + b%10
	}
	return string(out), nil
}

// glyphs is a 5x7 bitmap font covering the digits. A font file would be a
// dependency and a licence question for something this small.
var glyphs = [10][7]string{
	{".###.", "#...#", "#...#", "#...#", "#...#", "#...#", ".###."}, // 0
	{"..#..", ".##..", "..#..", "..#..", "..#..", "..#..", ".###."}, // 1
	{".###.", "#...#", "....#", "...#.", "..#..", ".#...", "#####"}, // 2
	{"####.", "....#", "....#", ".###.", "....#", "....#", "####."}, // 3
	{"...#.", "..##.", ".#.#.", "#..#.", "#####", "...#.", "...#."}, // 4
	{"#####", "#....", "####.", "....#", "....#", "#...#", ".###."}, // 5
	{"..##.", ".#...", "#....", "####.", "#...#", "#...#", ".###."}, // 6
	{"#####", "....#", "...#.", "..#..", ".#...", ".#...", ".#..."}, // 7
	{".###.", "#...#", "#...#", ".###.", "#...#", "#...#", ".###."}, // 8
	{".###.", "#...#", "#...#", ".####", "....#", "...#.", ".##.."}, // 9
}

const (
	glyphW = 5
	glyphH = 7
)

// renderCaptcha draws code as a PNG. The distortions applied — per-digit
// offset and shear, a sine warp over the whole image, speckle and stray lines —
// are the cheap ones that break naive template matching while leaving the
// digits legible at this size.
func renderCaptcha(code string) ([]byte, error) {
	// All randomness in the drawing comes from one buffer, drawn once, so a
	// failing entropy source is handled in a single place.
	noise := make([]byte, 512)
	if _, err := rand.Read(noise); err != nil {
		return nil, err
	}
	rnd := &byteSource{buf: noise}

	img := image.NewRGBA(image.Rect(0, 0, captchaW, captchaH))

	// A light, slightly random background keeps the PNG from being byte-identical
	// between challenges.
	bg := color.RGBA{
		R: 225 + rnd.next(30),
		G: 225 + rnd.next(30),
		B: 225 + rnd.next(30),
		A: 255,
	}
	for y := 0; y < captchaH; y++ {
		for x := 0; x < captchaW; x++ {
			img.SetRGBA(x, y, bg)
		}
	}

	// Draw onto a scratch layer first so the sine warp can displace finished
	// glyphs rather than being applied per-pixel as they are drawn.
	scratch := image.NewRGBA(img.Rect)
	copy(scratch.Pix, img.Pix)

	n := len(code)
	if n == 0 {
		n = 1
	}
	cellW := captchaW / n
	// Pick the largest whole-pixel scale that leaves a little breathing room
	// between neighbouring digits, so they never merge into one blob.
	scale := (cellW - 6) / glyphW
	if scale < 2 {
		scale = 2
	}
	if maxScale := (captchaH - 10) / glyphH; scale > maxScale && maxScale >= 2 {
		scale = maxScale
	}

	for i, ch := range code {
		d := int(ch - '0')
		if d < 0 || d > 9 {
			continue
		}
		// Dark ink on a light background: enough contrast that the speckle
		// below does not swallow the strokes. Each digit gets its own hue, but
		// all of them stay dark so the noise never competes with them.
		fg := color.RGBA{
			R: 20 + rnd.next(50),
			G: 20 + rnd.next(50),
			B: 20 + rnd.next(50),
			A: 255,
		}
		// Centre the glyph in its cell, then jitter it by a pixel or two.
		baseX := i*cellW + (cellW-glyphW*scale)/2 + int(rnd.next(3)) - 1
		baseY := (captchaH-glyphH*scale)/2 + int(rnd.next(5)) - 2
		// Lean the glyph left or right. The factor is small on purpose: past
		// about 0.15 the taller digits start colliding with their neighbours.
		shear := float64(int(rnd.next(3))-1) * 0.12

		for gy := 0; gy < glyphH; gy++ {
			row := glyphs[d][gy]
			for gx := 0; gx < glyphW; gx++ {
				if row[gx] != '#' {
					continue
				}
				for py := 0; py < scale; py++ {
					offY := gy*scale + py
					lean := int(shear * float64(glyphH*scale-offY))
					for px := 0; px < scale; px++ {
						fillPixel(scratch, baseX+gx*scale+px+lean, baseY+offY, fg)
					}
				}
			}
		}
	}

	// Sine warp: shift each row horizontally. Kept to a couple of pixels over a
	// long period, which ripples the text without shredding it.
	amp := 1.0 + float64(rnd.next(3))/2
	period := float64(captchaH) * (1.2 + float64(rnd.next(5))/10)
	phase := float64(rnd.next(100)) / 100 * 2 * math.Pi
	for y := 0; y < captchaH; y++ {
		shift := int(math.Round(amp * math.Sin(float64(y)/period*2*math.Pi+phase)))
		for x := 0; x < captchaW; x++ {
			src := x - shift
			if src < 0 || src >= captchaW {
				img.SetRGBA(x, y, bg)
				continue
			}
			img.SetRGBA(x, y, scratch.RGBAAt(src, y))
		}
	}

	// One stroke through the text, plus light speckle. Both are mid-grey rather
	// than as dark as the digits, so a human can tell ink from noise.
	lineColor := color.RGBA{R: 120 + rnd.next(60), G: 120 + rnd.next(60), B: 120 + rnd.next(60), A: 255}
	drawLine(img,
		0, captchaH/4+int(rnd.next(uint8(captchaH/2))),
		captchaW-1, captchaH/4+int(rnd.next(uint8(captchaH/2))),
		lineColor)

	for i := 0; i < captchaW*captchaH/40; i++ {
		x := int(rnd.next(uint8(captchaW/2)))*2 + int(rnd.next(2))
		y := int(rnd.next(uint8(captchaH)))
		v := 140 + rnd.next(70)
		fillPixel(img, x, y, color.RGBA{R: v, G: v, B: v, A: 255})
	}

	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestSpeed}
	if err := enc.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func fillPixel(img *image.RGBA, x, y int, c color.RGBA) {
	if x < 0 || y < 0 || x >= captchaW || y >= captchaH {
		return
	}
	img.SetRGBA(x, y, c)
}

// drawLine is Bresenham's algorithm; the standard library has no line drawing.
func drawLine(img *image.RGBA, x0, y0, x1, y1 int, c color.RGBA) {
	dx, dy := abs(x1-x0), -abs(y1-y0)
	sx, sy := 1, 1
	if x0 > x1 {
		sx = -1
	}
	if y0 > y1 {
		sy = -1
	}
	err := dx + dy
	for {
		fillPixel(img, x0, y0, c)
		// Thicken slightly so the stroke survives PNG scaling in the browser.
		fillPixel(img, x0, y0+1, c)
		if x0 == x1 && y0 == y1 {
			return
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x0 += sx
		}
		if e2 <= dx {
			err += dx
			y0 += sy
		}
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// byteSource hands out small bounded values from a fixed random buffer, wrapping
// when exhausted. Reuse is fine here: the values only drive cosmetic jitter, and
// the answer itself comes straight from crypto/rand.
type byteSource struct {
	buf []byte
	pos int
}

func (b *byteSource) next(n uint8) uint8 {
	if n == 0 {
		return 0
	}
	if b.pos >= len(b.buf) {
		b.pos = 0
	}
	v := b.buf[b.pos]
	b.pos++
	return v % n
}
