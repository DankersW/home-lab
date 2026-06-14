package receipt_test

import (
	"testing"

	"github.com/dankers/home-lab/services/receipts/internal/receipt"
	"github.com/stretchr/testify/require"
)

func TestParseMoneyMinor(t *testing.T) {
	tests := []struct {
		in      string
		want    int64
		wantErr bool
	}{
		{in: "12.99", want: 1299},
		{in: "12,99", want: 1299},
		{in: "12", want: 1200},
		{in: "0.05", want: 5},
		{in: "1299.00", want: 129900},
		{in: " 7.5 ", want: 750},
		{in: "-5.50", want: -550},
		{in: "", wantErr: true},
		{in: "12.999", wantErr: true},
		{in: "abc", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			got, err := receipt.ParseMoneyMinor(tc.in)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestClassifyUpload(t *testing.T) {
	heic := []byte{
		0, 0, 0, 0x18, 'f', 't', 'y', 'p', 'h', 'e', 'i', 'c',
		0, 0, 0, 0, 'm', 'i', 'f', '1',
	}
	tests := []struct {
		name     string
		head     []byte
		wantCT   string
		wantKind receipt.AttachmentKind
		wantOK   bool
	}{
		{name: "jpeg", head: []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10}, wantCT: "image/jpeg", wantKind: receipt.KindImage, wantOK: true},
		{name: "png", head: []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}, wantCT: "image/png", wantKind: receipt.KindImage, wantOK: true},
		{name: "pdf", head: []byte("%PDF-1.7\n%abc"), wantCT: "application/pdf", wantKind: receipt.KindPDF, wantOK: true},
		{name: "heic", head: heic, wantCT: "image/heic", wantKind: receipt.KindImage, wantOK: true},
		{name: "plain text spoof", head: []byte("totally not an image, even if named .heic"), wantOK: false},
		{name: "too short", head: []byte{0x01}, wantOK: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ct, kind, ok := receipt.ClassifyUpload(tc.head)
			require.Equal(t, tc.wantOK, ok)
			if tc.wantOK {
				require.Equal(t, tc.wantCT, ct)
				require.Equal(t, tc.wantKind, kind)
			}
		})
	}
}

func TestMoneyString(t *testing.T) {
	require.Equal(t, "12.99 EUR", receipt.Money{AmountMinor: 1299, Currency: "EUR"}.String())
	require.Equal(t, "-5.05 USD", receipt.Money{AmountMinor: -505, Currency: "USD"}.String())
	require.Equal(t, "0.09 EUR", receipt.Money{AmountMinor: 9, Currency: "EUR"}.String())
}
