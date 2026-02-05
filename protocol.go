package main

// Modbus 協議常數
const (
	// Modbus 功能碼
	FuncCodeReadCoils              = 0x01
	FuncCodeReadDiscreteInputs     = 0x02
	FuncCodeReadHoldingRegisters   = 0x03
	FuncCodeReadInputRegisters     = 0x04
	FuncCodeWriteSingleCoil        = 0x05
	FuncCodeWriteSingleRegister    = 0x06
	FuncCodeWriteMultipleCoils     = 0x0F
	FuncCodeWriteMultipleRegisters = 0x10

	// Modbus 異常碼
	ExceptionCodeIllegalFunction         = 0x01
	ExceptionCodeIllegalDataAddress      = 0x02
	ExceptionCodeIllegalDataValue        = 0x03
	ExceptionCodeSlaveDeviceFailure      = 0x04
	ExceptionCodeAcknowledge             = 0x05
	ExceptionCodeSlaveDeviceBusy         = 0x06
	ExceptionCodeMemoryParityError       = 0x08
	ExceptionCodeGatewayPathUnavailable  = 0x0A
	ExceptionCodeGatewayTargetNoResponse = 0x0B

	// Modbus TCP 常數
	ModbusTCPHeaderLength = 7  // MBAP Header 長度
	ModbusTCPMaxADULength = 260
	ModbusTCPDefaultPort  = 502

	// 暫存器限制
	MaxCoilsPerRead     = 2000
	MaxRegistersPerRead = 125
	MaxCoilsPerWrite    = 1968
	MaxRegistersPerWrite = 123
)

// RegisterType 暫存器類型
type RegisterType int

const (
	RegisterTypeCoil RegisterType = iota
	RegisterTypeDiscreteInput
	RegisterTypeInputRegister
	RegisterTypeHoldingRegister
)

func (rt RegisterType) String() string {
	switch rt {
	case RegisterTypeCoil:
		return "Coil"
	case RegisterTypeDiscreteInput:
		return "DiscreteInput"
	case RegisterTypeInputRegister:
		return "InputRegister"
	case RegisterTypeHoldingRegister:
		return "HoldingRegister"
	default:
		return "Unknown"
	}
}

// DataType 資料類型 (用於暫存器映射)
type DataType int

const (
	DataTypeUint16 DataType = iota
	DataTypeInt16
	DataTypeUint32
	DataTypeInt32
	DataTypeFloat32
)

func (dt DataType) String() string {
	switch dt {
	case DataTypeUint16:
		return "uint16"
	case DataTypeInt16:
		return "int16"
	case DataTypeUint32:
		return "uint32"
	case DataTypeInt32:
		return "int32"
	case DataTypeFloat32:
		return "float32"
	default:
		return "unknown"
	}
}

// RegisterCount 返回該資料類型佔用的暫存器數量
func (dt DataType) RegisterCount() int {
	switch dt {
	case DataTypeUint32, DataTypeInt32, DataTypeFloat32:
		return 2
	default:
		return 1
	}
}
