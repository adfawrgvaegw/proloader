package translate

import (
	"github.com/vmihailenco/msgpack/v5"
)

// ConvertStruct 使用MessagePack进行结构体转换
func ConvertStruct(src interface{}, dest interface{}) error {
	data, err := msgpack.Marshal(src)
	if err != nil {
		return err
	}
	return msgpack.Unmarshal(data, dest)
}
