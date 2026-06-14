package receipt

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// ParseMoneyMinor parses a human amount like "12.99", "12,99", "12", or "1299.00"
// into integer minor units. It accepts '.' or ',' as the decimal separator and
// at most two fractional digits, and never uses floating point.
func ParseMoneyMinor(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errors.New("amount is empty")
	}
	s = strings.ReplaceAll(s, ",", ".")

	neg := false
	if strings.HasPrefix(s, "-") {
		neg, s = true, s[1:]
	}

	whole, frac := s, ""
	if i := strings.IndexByte(s, '.'); i >= 0 {
		whole, frac = s[:i], s[i+1:]
	}
	if whole == "" {
		whole = "0"
	}
	if len(frac) > 2 {
		return 0, fmt.Errorf("amount %q has more than two decimal places", s)
	}
	for len(frac) < 2 {
		frac += "0"
	}

	w, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("amount %q is not a number", s)
	}
	f, err := strconv.ParseInt(frac, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("amount %q has an invalid fraction", s)
	}

	minor := w*100 + f
	if neg {
		minor = -minor
	}
	return minor, nil
}

// ClassifyUpload inspects the leading bytes of an upload and returns the content
// type to store, its kind, and whether it is an accepted type. HEIC/HEIF (which
// http.DetectContentType reports as application/octet-stream) is identified by
// its ISO-BMFF "ftyp" box rather than by any client-declared type or filename,
// so the allow-list cannot be bypassed by a spoofed header or extension.
func ClassifyUpload(head []byte) (contentType string, kind AttachmentKind, ok bool) {
	switch http.DetectContentType(head) {
	case "image/jpeg":
		return "image/jpeg", KindImage, true
	case "image/png":
		return "image/png", KindImage, true
	case "application/pdf":
		return "application/pdf", KindPDF, true
	}
	if _, isHEIF := heifBrand(head); isHEIF {
		return "image/heic", KindImage, true
	}
	return "", "", false
}

// heifBrand detects an ISO Base Media File Format "ftyp" box whose major or a
// compatible brand indicates HEIF/HEIC. Layout: [4-byte big-endian box size]
// "ftyp" [4-byte major brand] [4-byte minor version] [compatible brands...].
func heifBrand(b []byte) (string, bool) {
	if len(b) < 12 || string(b[4:8]) != "ftyp" {
		return "", false
	}
	known := map[string]bool{
		"heic": true, "heix": true, "heim": true, "heis": true,
		"hevc": true, "hevx": true, "mif1": true, "msf1": true, "heif": true,
	}
	if major := string(b[8:12]); known[major] {
		return major, true
	}
	size := int(binary.BigEndian.Uint32(b[0:4]))
	if size <= 0 || size > len(b) {
		size = len(b)
	}
	for i := 16; i+4 <= size; i += 4 {
		if brand := string(b[i : i+4]); known[brand] {
			return brand, true
		}
	}
	return "", false
}
