// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	sandboxcontract "github.com/coze-dev/coze-studio/backend/pkg/sandboxcontract"
)

const (
	executionCheckpointPlaintextVersion  byte = 1
	maxExecutionCheckpointPlaintextBytes      = 4096
	checkpointAADPurpose                      = "appdev_provider_executions:execution_checkpoint:v1"
)

var ErrExecutionCheckpointCodec = errors.New("execution checkpoint codec failed")

type ExecutionCheckpointBinding struct {
	SpaceID     string
	ProjectID   string
	Generation  uint64
	ProviderKey string
	Scope       domainsandbox.Scope
}

type ExecutionCheckpointProtector interface {
	Seal(ctx context.Context, aad, plaintext []byte) (string, error)
	Open(ctx context.Context, aad []byte, envelope string) ([]byte, error)
}

type ExecutionCheckpointCodec struct {
	protector ExecutionCheckpointProtector
}

func NewExecutionCheckpointCodec(protector ExecutionCheckpointProtector) (*ExecutionCheckpointCodec, error) {
	if nilExecutionCheckpointProtector(protector) {
		return nil, ErrExecutionCheckpointCodec
	}
	return &ExecutionCheckpointCodec{protector: protector}, nil
}

func (c *ExecutionCheckpointCodec) Seal(
	ctx context.Context,
	binding ExecutionCheckpointBinding,
	checkpoint ExecutionCheckpoint,
) (string, error) {
	if c == nil || nilExecutionCheckpointProtector(c.protector) || ctx == nil || ctx.Err() != nil ||
		validateExecutionCheckpointBinding(binding) != nil || validateExecutionCheckpointForBinding(checkpoint, binding) != nil {
		return "", ErrExecutionCheckpointCodec
	}
	plaintext, err := encodeExecutionCheckpoint(checkpoint)
	if err != nil {
		return "", ErrExecutionCheckpointCodec
	}
	defer wipeExecutionCheckpointBytes(plaintext)
	envelope, err := c.protector.Seal(ctx, executionCheckpointAAD(binding), plaintext)
	if err != nil || envelope == "" || len(envelope) > sandboxcontract.MaxExecutionCheckpointEnvelopeBytes {
		return "", ErrExecutionCheckpointCodec
	}
	return envelope, nil
}

func (c *ExecutionCheckpointCodec) Open(
	ctx context.Context,
	binding ExecutionCheckpointBinding,
	envelope string,
) (ExecutionCheckpoint, error) {
	if c == nil || nilExecutionCheckpointProtector(c.protector) || ctx == nil || ctx.Err() != nil ||
		validateExecutionCheckpointBinding(binding) != nil || envelope == "" || len(envelope) > sandboxcontract.MaxExecutionCheckpointEnvelopeBytes {
		return ExecutionCheckpoint{}, ErrExecutionCheckpointCodec
	}
	plaintext, err := c.protector.Open(ctx, executionCheckpointAAD(binding), envelope)
	if err != nil || len(plaintext) == 0 || len(plaintext) > maxExecutionCheckpointPlaintextBytes {
		wipeExecutionCheckpointBytes(plaintext)
		return ExecutionCheckpoint{}, ErrExecutionCheckpointCodec
	}
	defer wipeExecutionCheckpointBytes(plaintext)
	checkpoint, err := decodeExecutionCheckpoint(plaintext)
	if err != nil || validateExecutionCheckpointForBinding(checkpoint, binding) != nil {
		return ExecutionCheckpoint{}, ErrExecutionCheckpointCodec
	}
	return checkpoint, nil
}

func (*ExecutionCheckpointCodec) String() string   { return "[REDACTED execution checkpoint codec]" }
func (*ExecutionCheckpointCodec) GoString() string { return "[REDACTED execution checkpoint codec]" }
func (*ExecutionCheckpointCodec) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "[REDACTED execution checkpoint codec]")
}

func validateExecutionCheckpointBinding(binding ExecutionCheckpointBinding) error {
	spaceID, err := strconv.ParseUint(binding.SpaceID, 10, 64)
	if err != nil || spaceID == 0 || strconv.FormatUint(spaceID, 10) != binding.SpaceID ||
		!validCheckpointBindingIdentifier(binding.ProjectID, 64) || binding.Generation == 0 ||
		domainsandbox.ValidateProviderKey(binding.ProviderKey) != nil || binding.Scope != domainsandbox.ScopeAppDev {
		return ErrExecutionCheckpointCodec
	}
	return nil
}

func validateExecutionCheckpointForBinding(checkpoint ExecutionCheckpoint, binding ExecutionCheckpointBinding) error {
	if checkpoint.providerKey != binding.ProviderKey || checkpoint.scope != binding.Scope ||
		domainsandbox.ValidateProviderKey(checkpoint.providerKey) != nil || !validRouterScope(checkpoint.scope) ||
		!validOpaqueLeaseToken(checkpoint.leaseToken) || !validOpaqueLeaseToken(checkpoint.leaseFence) ||
		checkpoint.leaseExpiryMilli <= 0 || !validRouterExecutionID(checkpoint.executionID) {
		return ErrExecutionCheckpointCodec
	}
	return nil
}

func encodeExecutionCheckpoint(checkpoint ExecutionCheckpoint) ([]byte, error) {
	buffer := bytes.NewBuffer(make([]byte, 0, 256))
	buffer.WriteString("ECP")
	buffer.WriteByte(executionCheckpointPlaintextVersion)
	for _, value := range []string{
		checkpoint.providerKey,
		string(checkpoint.scope),
		checkpoint.leaseToken,
		checkpoint.leaseFence,
		checkpoint.executionID,
	} {
		if err := writeExecutionCheckpointString(buffer, value); err != nil {
			return nil, err
		}
	}
	if err := binary.Write(buffer, binary.BigEndian, checkpoint.leaseExpiryMilli); err != nil || buffer.Len() > maxExecutionCheckpointPlaintextBytes {
		return nil, ErrExecutionCheckpointCodec
	}
	return buffer.Bytes(), nil
}

func decodeExecutionCheckpoint(plaintext []byte) (ExecutionCheckpoint, error) {
	if len(plaintext) < 4 || len(plaintext) > maxExecutionCheckpointPlaintextBytes ||
		!bytes.Equal(plaintext[:3], []byte("ECP")) || plaintext[3] != executionCheckpointPlaintextVersion {
		return ExecutionCheckpoint{}, ErrExecutionCheckpointCodec
	}
	reader := bytes.NewReader(plaintext[4:])
	values := make([]string, 5)
	for index := range values {
		value, err := readExecutionCheckpointString(reader)
		if err != nil {
			return ExecutionCheckpoint{}, ErrExecutionCheckpointCodec
		}
		values[index] = value
	}
	var expiry int64
	if err := binary.Read(reader, binary.BigEndian, &expiry); err != nil || reader.Len() != 0 {
		return ExecutionCheckpoint{}, ErrExecutionCheckpointCodec
	}
	return ExecutionCheckpoint{
		providerKey: values[0], scope: domainsandbox.Scope(values[1]),
		leaseToken: values[2], leaseFence: values[3], executionID: values[4], leaseExpiryMilli: expiry,
	}, nil
}

func writeExecutionCheckpointString(buffer *bytes.Buffer, value string) error {
	if value == "" || len(value) > 1024 {
		return ErrExecutionCheckpointCodec
	}
	if err := binary.Write(buffer, binary.BigEndian, uint16(len(value))); err != nil {
		return ErrExecutionCheckpointCodec
	}
	_, err := buffer.WriteString(value)
	return err
}

func readExecutionCheckpointString(reader *bytes.Reader) (string, error) {
	var length uint16
	if err := binary.Read(reader, binary.BigEndian, &length); err != nil || length == 0 || int(length) > reader.Len() || length > 1024 {
		return "", ErrExecutionCheckpointCodec
	}
	value := make([]byte, int(length))
	if _, err := io.ReadFull(reader, value); err != nil {
		return "", ErrExecutionCheckpointCodec
	}
	return string(value), nil
}

func executionCheckpointAAD(binding ExecutionCheckpointBinding) []byte {
	buffer := bytes.NewBuffer(make([]byte, 0, 192))
	for _, value := range []string{
		checkpointAADPurpose,
		binding.SpaceID,
		binding.ProjectID,
		strconv.FormatUint(binding.Generation, 10),
		binding.ProviderKey,
		string(binding.Scope),
	} {
		_ = binary.Write(buffer, binary.BigEndian, uint16(len(value)))
		buffer.WriteString(value)
	}
	return buffer.Bytes()
}

func validCheckpointBindingIdentifier(value string, limit int) bool {
	if value == "" || len(value) > limit || value != strings.TrimSpace(value) {
		return false
	}
	for index := range value {
		character := value[index]
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || character == '-' || character == '_' || character == '.' {
			continue
		}
		return false
	}
	return true
}

func nilExecutionCheckpointProtector(protector ExecutionCheckpointProtector) bool {
	if protector == nil {
		return true
	}
	value := reflect.ValueOf(protector)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

func wipeExecutionCheckpointBytes(value []byte) {
	for index := range value {
		value[index] = 0
	}
}
