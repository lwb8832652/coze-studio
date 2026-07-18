// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package secureaead

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestSealOpenSupportsAESKeySizesAndDeterministicNonce(t *testing.T) {
	for _, keySize := range []int{16, 24, 32} {
		t.Run(string(rune('A'+keySize)), func(t *testing.T) {
			key := bytes.Repeat([]byte{byte(keySize)}, keySize)
			nonce := []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11}
			aad := []byte("synthetic-aad-marker")
			plaintext := []byte("synthetic-plaintext-marker")

			gotNonce, ciphertext, err := Seal(key, aad, plaintext, bytes.NewReader(nonce))
			if err != nil {
				t.Fatalf("Seal() error = %v", err)
			}
			if !bytes.Equal(gotNonce, nonce) {
				t.Fatalf("Seal() nonce mismatch")
			}
			secondNonce, secondCiphertext, err := Seal(key, aad, plaintext, bytes.NewReader(nonce))
			if err != nil {
				t.Fatalf("second Seal() error = %v", err)
			}
			if !bytes.Equal(secondNonce, gotNonce) || !bytes.Equal(secondCiphertext, ciphertext) {
				t.Fatal("deterministic nonce did not produce byte-compatible output")
			}
			decoded, err := Open(key, aad, gotNonce, ciphertext)
			if err != nil {
				t.Fatalf("Open() error = %v", err)
			}
			if !bytes.Equal(decoded, plaintext) {
				t.Fatal("Open() plaintext mismatch")
			}
		})
	}
}

func TestSealRejectsInvalidKeyAndNonceSourceSafely(t *testing.T) {
	marker := "must-not-appear-in-secureaead-errors"
	tests := []struct {
		name   string
		key    []byte
		source *bytes.Reader
		want   error
	}{
		{name: "short key", key: bytes.Repeat([]byte(marker), 1), source: bytes.NewReader(make([]byte, 12)), want: ErrInvalidKey},
		{name: "long key", key: bytes.Repeat([]byte{1}, 33), source: bytes.NewReader(make([]byte, 12)), want: ErrInvalidKey},
		{name: "nil nonce source", key: bytes.Repeat([]byte{1}, 32), want: ErrInvalidNonceSource},
		{name: "short nonce source", key: bytes.Repeat([]byte{1}, 32), source: bytes.NewReader(make([]byte, 11)), want: ErrNonceGeneration},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, _, err := Seal(test.key, []byte(marker), []byte(marker), test.source)
			if !errors.Is(err, test.want) {
				t.Fatalf("Seal() error = %v, want %v", err, test.want)
			}
			assertSecureAEADError(t, err, marker)
		})
	}
}

func TestOpenRejectsMalformedNonceCiphertextAndTamperSafely(t *testing.T) {
	key := bytes.Repeat([]byte{7}, 32)
	nonce := bytes.Repeat([]byte{8}, 12)
	marker := "must-not-appear-in-open-errors"
	_, ciphertext, err := Seal(key, []byte(marker), []byte(marker), bytes.NewReader(nonce))
	if err != nil {
		t.Fatalf("Seal() error = %v", err)
	}
	tampered := append([]byte(nil), ciphertext...)
	tampered[len(tampered)-1] ^= 1
	tests := []struct {
		name       string
		nonce      []byte
		ciphertext []byte
		want       error
	}{
		{name: "short nonce", nonce: nonce[:11], ciphertext: ciphertext, want: ErrInvalidNonce},
		{name: "long nonce", nonce: append(append([]byte(nil), nonce...), 0), ciphertext: ciphertext, want: ErrInvalidNonce},
		{name: "short ciphertext", nonce: nonce, ciphertext: make([]byte, 15), want: ErrInvalidCiphertext},
		{name: "tampered ciphertext", nonce: nonce, ciphertext: tampered, want: ErrOpenFailed},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Open(key, []byte(marker), test.nonce, test.ciphertext)
			if !errors.Is(err, test.want) {
				t.Fatalf("Open() error = %v, want %v", err, test.want)
			}
			assertSecureAEADError(t, err, marker)
		})
	}
}

func assertSecureAEADError(t *testing.T, err error, markers ...string) {
	t.Helper()
	if err == nil || len(err.Error()) > 96 {
		t.Fatalf("error is nil or unbounded")
	}
	for _, marker := range markers {
		if strings.Contains(err.Error(), marker) {
			t.Fatal("error leaked caller-provided data")
		}
	}
}
