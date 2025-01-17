package mbserver

import (
	"net"
	"testing"
	"time"

	"github.com/simonvetter/modbus"
)

func TestTCPServerWithConcurrentConnections(t *testing.T) {
	var coils []bool
	var c1, c2, c3 *modbus.ModbusClient

	server, err := New(new(tcpTestHandler), MaxClients(2))
	if err != nil {
		t.Errorf("failed to create server: %v", err)
	}

	l, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Error(err)
	}
	defer l.Close()

	if err := server.Start(l); err != nil {
		t.Errorf("failed to start server: %v", err)
	}

	// create 3 modbus clients
	c1, err = modbus.NewClient(&modbus.ClientConfiguration{
		URL: "tcp://" + l.Addr().String(),
	})
	if err != nil {
		t.Errorf("failed to create client: %v", err)
	}
	c2, err = modbus.NewClient(&modbus.ClientConfiguration{
		URL: "tcp://" + l.Addr().String(),
	})
	if err != nil {
		t.Errorf("failed to create client: %v", err)
	}
	c3, err = modbus.NewClient(&modbus.ClientConfiguration{
		URL: "tcp://" + l.Addr().String(),
	})
	if err != nil {
		t.Errorf("failed to create client: %v", err)
	}

	// the server should have zero client connections so far
	server.lock.Lock()
	if len(server.tcpClients) != 0 {
		t.Errorf("expected server.tcpClients to hold 0 entries, got: %v", len(server.tcpClients))
	}
	server.lock.Unlock()

	// connect client #1
	if err := c1.Open(); err != nil {
		t.Errorf("c1.Connect() should have succeeded, got: %v", err)
	}
	c1.SetUnitId(9)

	// the server should have 1 client connection at this point
	time.Sleep(time.Millisecond)
	server.lock.Lock()
	if len(server.tcpClients) != 1 {
		t.Errorf("expected server.tcpClients to hold 1 entry, got: %v", len(server.tcpClients))
	}
	server.lock.Unlock()

	// connect client #2
	if err := c2.Open(); err != nil {
		t.Errorf("c2.Connect() should have succeeded, got: %v", err)
	}
	c2.SetUnitId(9)

	time.Sleep(time.Millisecond)
	// the server should now have 2 client connections, its maximum allowed
	server.lock.Lock()
	if len(server.tcpClients) != 2 {
		t.Errorf("expected server.tcpClients to hold 2 entries, got: %v", len(server.tcpClients))
	}
	server.lock.Unlock()

	// connect client #3
	if err := c3.Open(); err != nil {
		t.Errorf("c3.Connect() should have succeeded, got: %v", err)
	}
	c3.SetUnitId(9)

	// since the previous client was rejected, the active connection count
	// should stay at 2
	server.lock.Lock()
	if len(server.tcpClients) != 2 {
		t.Errorf("expected server.tcpClients to hold 2 entries, got: %v", len(server.tcpClients))
	}
	server.lock.Unlock()

	// c1 and c2 should both be able to make requests while c3 should error out
	// as it has been disconnected (conn closed)
	coils, err = c1.ReadCoils(0x0000, 2)
	if err != nil {
		t.Errorf("c1.ReadCoils() should have succeeded, got: %v", err)
	}
	if coils[0] == true || coils[1] == true {
		t.Errorf("expected {false, false}, got: %v", coils)
	}

	coils, err = c2.ReadCoils(0x0003, 5)
	if err != nil {
		t.Errorf("c2.ReadCoils() should have succeeded, got: %v", err)
	}
	if coils[0] != false || coils[1] != false {
		t.Errorf("expected {false, false}, got: %v", coils)
	}

	_, err = c3.ReadCoil(0x0001)
	if err == nil {
		t.Errorf("c3.ReadCoil() should have failed")
	}

	// close c2 and make sure the connection is freed
	c2.Close()
	time.Sleep(time.Millisecond)
	server.lock.Lock()
	if len(server.tcpClients) != 1 {
		t.Errorf("expected server.tcpClients to hold 1 entry, got: %v", len(server.tcpClients))
	}
	server.lock.Unlock()

	// reconnect c2
	if err := c2.Open(); err != nil {
		t.Errorf("c2.Open should have succeeded, got: %v", err)
	}

	// write to the coil at address #1
	err = c2.WriteCoil(0x0001, true)
	if err != nil {
		t.Errorf("c2.WriteCoil() should have succeeded, got: %v", err)
	}

	server.lock.Lock()
	if len(server.tcpClients) != 2 {
		t.Errorf("expected server.tcpClients to hold 2 entries, got: %v", len(server.tcpClients))
	}
	server.lock.Unlock()

	// check the coil value with c1
	coils, err = c1.ReadCoils(0x0000, 2)
	if err != nil {
		t.Errorf("c1.ReadCoils() should have succeeded, got: %v", err)
	}
	if coils[0] != false || coils[1] != true {
		t.Errorf("expected {false, true}, got: %v", coils)
	}

	// close c1 and make sure the connection is freed
	c1.Close()
	time.Sleep(time.Millisecond)
	server.lock.Lock()
	if len(server.tcpClients) != 1 {
		t.Errorf("expected server.tcpClients to hold 1 entry, got: %v", len(server.tcpClients))
	}
	server.lock.Unlock()

	// stopping the server should disconnect all clients
	server.Stop()

	time.Sleep(time.Millisecond)
	server.lock.Lock()
	if len(server.tcpClients) != 0 {
		t.Errorf("expected server.tcpClients to hold 0 entries, got: %v", len(server.tcpClients))
	}
	server.lock.Unlock()

	// c2 should have been disconnected
	_, err = c2.ReadCoils(0x0003, 5)
	if err == nil {
		t.Errorf("c2.ReadCoils() should have failed")
	}
}

func TestTCPServerCoilsAndDiscreteInputs(t *testing.T) {
	var coils []bool
	var dis []bool
	var client *modbus.ModbusClient

	th := &tcpTestHandler{}

	server, err := New(th)
	if err != nil {
		t.Errorf("failed to create server: %v", err)
	}

	l, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Error(err)
	}
	defer l.Close()

	if err := server.Start(l); err != nil {
		t.Errorf("failed to start server: %v", err)
	}

	client, err = modbus.NewClient(&modbus.ClientConfiguration{
		URL: "tcp://" + l.Addr().String(),
	})
	if err != nil {
		t.Errorf("failed to create client: %v", err)
	}

	if err := client.Open(); err != nil {
		t.Errorf("client.Open() should have succeeded, got: %v", err)
	}
	client.SetUnitId(9)

	// make sure both coils and discrete inputs are all false/0
	coils, err = client.ReadCoils(0x0000, 10)
	if err != nil {
		t.Errorf("client.ReadCoils() should have succeeded, got: %v", err)
	}
	for i := 0; i < 10; i++ {
		if coils[i] != false {
			t.Errorf("expected coil at addr 0x%04x to be false", i)
		}
	}

	dis, err = client.ReadDiscreteInputs(0x0000, 10)
	if err != nil {
		t.Errorf("client.ReadDiscreteInputs() should have succeeded, got: %v", err)
	}
	for i := 0; i < 10; i++ {
		if dis[i] != false {
			t.Errorf("expected discrete input at addr 0x%04x to be false", i)
		}
	}

	// set discrete inputs to random values
	th.di = [10]bool{
		false, false, false, true, false, true, true, true, true, true,
	}

	// read the discrete inputs again
	dis, err = client.ReadDiscreteInputs(0x0000, 10)
	if err != nil {
		t.Errorf("client.ReadDiscreteInput() should have succeeded, got: %v", err)
	}
	for i, b := range [10]bool{
		false, false, false, true, false, true, true, true, true, true,
	} {
		if dis[i] != b {
			t.Errorf("expected discrete input at addr 0x%04x to be %v", i, b)
		}
	}

	// reading past the array size should return ErrIllegalDataAddress
	_, err = client.ReadDiscreteInputs(0x000a, 1)
	if err != modbus.ErrIllegalDataAddress {
		t.Errorf("expected ErrIllegalDataAddress, got: %v", err)
	}
	_, err = client.ReadCoils(0x000a, 1)
	if err != modbus.ErrIllegalDataAddress {
		t.Errorf("expected ErrIllegalDataAddress, got: %v", err)
	}
	_, err = client.ReadDiscreteInputs(0x8, 3)
	if err != modbus.ErrIllegalDataAddress {
		t.Errorf("expected ErrIllegalDataAddress, got: %v", err)
	}
	_, err = client.ReadCoils(0x8, 3)
	if err != modbus.ErrIllegalDataAddress {
		t.Errorf("expected ErrIllegalDataAddress, got: %v", err)
	}

	// the coils shouldn't have changed
	coils, err = client.ReadCoils(0x0000, 10)
	if err != nil {
		t.Errorf("client.ReadCoils() should have succeeded, got: %v", err)
	}
	for i := 0; i < 10; i++ {
		if coils[i] != false {
			t.Errorf("expected coil at addr 0x%04x to be false", i)
		}
	}

	// write to a single coil
	err = client.WriteCoil(0x0004, true)
	if err != nil {
		t.Errorf("client.WriteCoil() should have succeeded, got: %v", err)
	}

	// make sure it has been written to
	coils, err = client.ReadCoils(0x0003, 3)
	if err != nil {
		t.Errorf("client.ReadCoils() should have succeeded, got: %v", err)
	}
	for i, v := range []bool{false, true, false} {
		if coils[i] != v {
			t.Errorf("expected coil at addr 0x%04x to be %v", 3+i, v)
		}
	}

	// write to multiple coils at once
	err = client.WriteCoils(0x0005, []bool{
		true, false, true, true,
	})
	if err != nil {
		t.Errorf("client.WriteCoils() should have succeeded, got: %v", err)
	}

	// make sure the write went through
	coils, err = client.ReadCoils(0x0005, 4)
	if err != nil {
		t.Errorf("client.ReadCoils() should have succeeded, got: %v", err)
	}
	for i, v := range []bool{true, false, true, true} {
		if coils[i] != v {
			t.Errorf("expected coil at addr 0x%04x to be %v", 3+i, v)
		}
	}

	// switch to another unit ID and make sure both coil and discrete input operations
	// return ErrIllegalFunction
	client.SetUnitId(5)
	err = client.WriteCoils(0x0005, []bool{
		true, false, true, true,
	})
	if err != modbus.ErrIllegalFunction {
		t.Errorf("client.WriteCoils() should have returned ErrIllegalFunction, got: %v", err)
	}
	err = client.WriteCoil(0x0005, false)
	if err != modbus.ErrIllegalFunction {
		t.Errorf("client.WriteCoil() should have returned ErrIllegalFunction, got: %v", err)
	}
	_, err = client.ReadCoils(0x0005, 1)
	if err != modbus.ErrIllegalFunction {
		t.Errorf("client.ReadCoils() should have returned ErrIllegalFunction, got: %v", err)
	}
	_, err = client.ReadDiscreteInputs(0x0005, 1)
	if err != modbus.ErrIllegalFunction {
		t.Errorf("client.ReadDiscreteInputs() should have returned ErrIllegalFunction, got: %v", err)
	}

	client.Close()
	server.Stop()
}

func TestTCPServerHoldingAndInputRegisters(t *testing.T) {
	var client *modbus.ModbusClient
	var th *tcpTestHandler
	var regs []uint16

	th = &tcpTestHandler{}

	server, err := New(th)
	if err != nil {
		t.Errorf("failed to create server: %v", err)
	}

	l, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Error(err)
	}
	defer l.Close()

	if err := server.Start(l); err != nil {
		t.Errorf("failed to start server: %v", err)
	}

	client, err = modbus.NewClient(&modbus.ClientConfiguration{
		URL: "tcp://" + l.Addr().String(),
	})
	if err != nil {
		t.Errorf("failed to create client: %v", err)
	}

	if err := client.Open(); err != nil {
		t.Errorf("client.Open() should have succeeded, got: %v", err)
	}
	client.SetUnitId(9)

	// all 10 input registers should be 0x0000
	regs, err = client.ReadRegisters(0x0000, 10, modbus.INPUT_REGISTER)
	if err != nil {
		t.Errorf("client.ReadRegisters() should have succeeded, got: %v", err)
	}
	for i := 0; i < 10; i++ {
		if regs[i] != 0x0000 {
			t.Errorf("expected 0x0000 at position %v, got: 0x%04x", i, regs[i])
		}
	}

	// assign some values to the handler's input registers
	for i := range th.input {
		th.input[i] = 0xa710 + uint16(i)
	}

	regs, err = client.ReadRegisters(0x0000, 10, modbus.INPUT_REGISTER)
	if err != nil {
		t.Errorf("client.ReadRegisters() should have succeeded, got: %v", err)
	}
	for i := 0; i < 10; i++ {
		if regs[i] != 0xa710+uint16(i) {
			t.Errorf("expected 0x%04x at position %v, got: 0x%04x", 0xa710+uint16(i), i, regs[i])
		}
	}

	// reading addr 0x0009 (the very last register) should succeed
	regs, err = client.ReadRegisters(0x0009, 1, modbus.INPUT_REGISTER)
	if err != nil {
		t.Errorf("client.ReadRegisters() should have succeeded, got: %v", err)
	}
	if regs[0] != 0xa719 {
		t.Errorf("expected 0xa719 at address 9, saw: 0x%04x", regs[0])
	}

	// reading past address 0x000a should fail
	_, err = client.ReadRegisters(0x0001, 10, modbus.INPUT_REGISTER)
	if err != modbus.ErrIllegalDataAddress {
		t.Errorf("client.ReadRegisters() should have returned ErrIllegalDataAddress, got: %v", err)
	}
	_, err = client.ReadRegisters(0x0000, 11, modbus.INPUT_REGISTER)
	if err != modbus.ErrIllegalDataAddress {
		t.Errorf("client.ReadRegisters() should have returned ErrIllegalDataAddress, got: %v", err)
	}

	// all 10 holding registers should still be 0x0000
	regs, err = client.ReadRegisters(0x0000, 10, modbus.HOLDING_REGISTER)
	if err != nil {
		t.Errorf("client.ReadRegisters() should have succeeded, got: %v", err)
	}
	for i := 0; i < 10; i++ {
		if regs[i] != 0x0000 {
			t.Errorf("expected 0x0000 at position %v, got: 0x%04x", i, regs[i])
		}
	}

	// write to a single valid register (with opcode 0x06)
	err = client.WriteRegister(0x0007, 0xfea1)
	if err != nil {
		t.Errorf("client.WriteRegister() should have succeeded, got: %v", err)
	}

	// make sure it has been written to
	regs, err = client.ReadRegisters(0x0005, 5, modbus.HOLDING_REGISTER)
	if err != nil {
		t.Errorf("client.ReadRegisters() should have succeeded, got: %v", err)
	}
	for i := 0; i < 5; i++ {
		if i != 2 && regs[i] != 0x0000 {
			t.Errorf("expected 0x0000 at position %v, got: 0x%04x", i, regs[i])
		}
		if i == 2 && regs[i] != 0xfea1 {
			t.Errorf("expected 0xfea1 at position %v, got: 0x%04x", i, regs[i])
		}
	}

	// check values in the handler as well
	for i := 0; i < 10; i++ {
		if i != 7 && th.holding[i] != 0x0000 {
			t.Errorf("expected 0x0000 at handler index %v, got: 0x%04x", i, regs[i])
		}
		if i == 7 && th.holding[i] != 0xfea1 {
			t.Errorf("expected 0xfea1 at handler index %v, got: 0x%04x", i, regs[i])
		}
	}

	// write multiple registers at once (with function code 0x10)
	err = client.WriteRegisters(0x0001, []uint16{
		0x0c11, 0x0c22, 0x0c33, 0x0c44,
		0x0c55, 0x0c66, 0x0c77, 0x0c88,
		0x0c99,
	})
	if err != nil {
		t.Errorf("client.WriteRegisters() should have succeeded, got: %v", err)
	}

	// write to a single valid register (with opcode 0x06)
	err = client.WriteRegister(0x0000, 0x0c00)
	if err != nil {
		t.Errorf("client.WriteRegister() should have succeeded, got: %v", err)
	}

	// make sure they have all been written to
	regs, err = client.ReadRegisters(0x0000, 10, modbus.HOLDING_REGISTER)
	if err != nil {
		t.Errorf("client.ReadRegisters() should have succeeded, got: %v", err)
	}
	for i := 0; i < 10; i++ {
		if regs[i] != 0x0c00+uint16(0x11*i) {
			t.Errorf("expected ox%04x at position %v, got: 0x%04x", 0x0c00+uint16(0x11*i), i, regs[i])
		}
	}

	// check values in the handler as well
	for i := 0; i < 10; i++ {
		if th.holding[i] != 0x0c00+uint16(0x11*i) {
			t.Errorf("expected 0xfea1 at handler index %v, got: 0x%04x", i, regs[i])
		}
	}

	// reading addr 0x0009 (the very last register) should succeed
	regs, err = client.ReadRegisters(0x0009, 1, modbus.HOLDING_REGISTER)
	if err != nil {
		t.Errorf("client.ReadRegisters() should have succeeded, got: %v", err)
	}
	if regs[0] != 0x0c99 {
		t.Errorf("expected 0x0c99 at address 9, saw: 0x%04x", regs[0])
	}

	// reading past address 0x000a should fail
	_, err = client.ReadRegisters(0x0001, 10, modbus.HOLDING_REGISTER)
	if err != modbus.ErrIllegalDataAddress {
		t.Errorf("client.ReadRegisters() should have returned ErrIllegalDataAddress, got: %v", err)
	}
	_, err = client.ReadRegisters(0x0000, 11, modbus.HOLDING_REGISTER)
	if err != modbus.ErrIllegalDataAddress {
		t.Errorf("client.ReadRegisters() should have returned ErrIllegalDataAddress, got: %v", err)
	}

	// switch to another unit ID and make sure both holding and input register operations
	// return ErrIllegalFunction
	client.SetUnitId(2)
	err = client.WriteRegisters(0x0005, []uint16{
		0x0000, 0x0001,
	})
	if err != modbus.ErrIllegalFunction {
		t.Errorf("client.WriteRegisters() should have returned ErrIllegalFunction, got: %v", err)
	}
	err = client.WriteRegister(0x0001, 0xffff)
	if err != modbus.ErrIllegalFunction {
		t.Errorf("client.WriteRegister() should have returned ErrIllegalFunction, got: %v", err)
	}
	_, err = client.ReadRegisters(0x0005, 1, modbus.HOLDING_REGISTER)
	if err != modbus.ErrIllegalFunction {
		t.Errorf("client.ReadRegisters() should have returned ErrIllegalFunction, got: %v", err)
	}
	_, err = client.ReadRegisters(0x0005, 1, modbus.INPUT_REGISTER)
	if err != modbus.ErrIllegalFunction {
		t.Errorf("client.ReadRegisters() should have returned ErrIllegalFunction, got: %v", err)
	}

	client.Close()
	server.Stop()
}

type tcpTestHandler struct {
	coils   [10]bool
	di      [10]bool
	input   [10]uint16
	holding [10]uint16
}

func (th *tcpTestHandler) HandleCoils(req *CoilsRequest) (res []bool, err error) {
	if req.UnitId != 9 {
		// only reply to unit ID #9
		err = ErrIllegalFunction
		return
	}

	if req.Addr+req.Quantity > uint16(len(th.coils)) {
		err = ErrIllegalDataAddress
		return
	}

	for i := 0; i < int(req.Quantity); i++ {
		if req.IsWrite {
			th.coils[int(req.Addr)+i] = req.Args[i]
		}
		res = append(res, th.coils[int(req.Addr)+i])
	}

	return
}

func (th *tcpTestHandler) HandleDiscreteInputs(req *DiscreteInputsRequest) (res []bool, err error) {
	if req.UnitId != 9 {
		// only reply to unit ID #9
		err = ErrIllegalFunction
		return
	}

	if req.Addr+req.Quantity > uint16(len(th.di)) {
		err = ErrIllegalDataAddress
		return
	}

	for i := 0; i < int(req.Quantity); i++ {
		res = append(res, th.di[int(req.Addr)+i])
	}

	return
}

func (th *tcpTestHandler) HandleHoldingRegisters(req *HoldingRegistersRequest) (res []uint16, err error) {
	if req.UnitId != 9 {
		// only reply to unit ID #9
		err = ErrIllegalFunction
		return
	}

	if req.Addr+req.Quantity > uint16(len(th.holding)) {
		err = ErrIllegalDataAddress
		return
	}

	for i := 0; i < int(req.Quantity); i++ {
		if req.IsWrite {
			th.holding[int(req.Addr)+i] = req.Args[i]
		}
		res = append(res, th.holding[int(req.Addr)+i])
	}

	return
}

func (th *tcpTestHandler) HandleInputRegisters(req *InputRegistersRequest) (res []uint16, err error) {
	if req.UnitId != 9 {
		// only reply to unit ID #9
		err = ErrIllegalFunction
		return
	}

	if req.Addr+req.Quantity > uint16(len(th.input)) {
		err = ErrIllegalDataAddress
		return
	}

	for i := 0; i < int(req.Quantity); i++ {
		res = append(res, th.input[int(req.Addr)+i])
	}

	return
}
