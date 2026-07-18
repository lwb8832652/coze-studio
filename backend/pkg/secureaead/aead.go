// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

// Package secureaead provides application-neutral AES-GCM primitives.
package secureaead

import (
	"crypto/aes"
	"crypto/cipher"
	"errors"
	"io"
	"reflect"
)

var (
	ErrInvalidKey         = errors.New("secureaead: invalid AES key")
	ErrInvalidNonceSource = errors.New("secureaead: invalid nonce source")
	ErrNonceGeneration    = errors.New("secureaead: nonce generation failed")
	ErrInvalidNonce       = errors.New("secureaead: invalid nonce")
	ErrInvalidCiphertext  = errors.New("secureaead: invalid ciphertext")
	ErrSealFailed         = errors.New("secureaead: encryption failed")
	ErrOpenFailed         = errors.New("secureaead: decryption failed")
)

// Seal encrypts plaintext with AES-GCM and a nonce read in full from
// nonceSource. The nonce is returned separately from the ciphertext.
func Seal(key, aad, plaintext []byte, nonceSource io.Reader) ([]byte, []byte, error) {
	aead, err := newGCM(key)
	if err != nil {
		return nil, nil, err
	}
	if nilReader(nonceSource) {
		return nil, nil, ErrInvalidNonceSource
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(nonceSource, nonce); err != nil {
		return nil, nil, ErrNonceGeneration
	}
	ciphertext := aead.Seal(nil, nonce, plaintext, aad)
	return nonce, ciphertext, nil
}

// Open authenticates and decrypts an AES-GCM ciphertext.
func Open(key, aad, nonce, ciphertext []byte) ([]byte, error) {
	aead, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	if len(nonce) != aead.NonceSize() {
		return nil, ErrInvalidNonce
	}
	if len(ciphertext) < aead.Overhead() {
		return nil, ErrInvalidCiphertext
	}
	plaintext, err := aead.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return nil, ErrOpenFailed
	}
	return plaintext, nil
}

func newGCM(key []byte) (cipher.AEAD, error) {
	switch len(key) {
	case 16, 24, 32:
	default:
		return nil, ErrInvalidKey
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, ErrInvalidKey
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, ErrInvalidKey
	}
	return aead, nil
}

func nilReader(reader io.Reader) bool {
	if reader == nil {
		return true
	}
	value := reflect.ValueOf(reader)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
