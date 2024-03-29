package libraries

import (
	"bytes"
	"reflect"
	"strconv"
	"sync"
	"unsafe"

	jsoniter "github.com/json-iterator/go"
)

var gjson jsoniter.API= jsoniter.ConfigCompatibleWithStandardLibrary
var serializationbufpool = sync.Pool{
	New: func() interface{} {
		return &MsgBuffer{}
	},
}

func S2B(s *string) []byte {
	return *(*[]byte)(unsafe.Pointer((*reflect.SliceHeader)(unsafe.Pointer(s))))
}

func B2S(buf []byte) string {
	return *(*string)(unsafe.Pointer(&buf))
}

//返回map[string]string
func Json_unpack_mps(s interface{}) map[string]string {
	v := json_unpack(s)
	if v == nil {
		return nil
	}
	tmp := make(map[string]string)
	for _k, _v := range v.(map[string]interface{}) {
		tmp[_k] = Initstring(_v)
	}
	return tmp

}

//返回map[int]string
func Json_unpack_mis(s interface{}) map[int]string {
	v := json_unpack(s)
	if v == nil {
		return nil
	}
	tmp := make(map[int]string)
	for _k, _v := range v.(map[string]interface{}) {
		k, _ := strconv.Atoi(_k)
		tmp[k] = Initstring(_v)
	}
	return tmp

}

//返回map[string]interface{}
func Json_unpack_mpi(s interface{}) map[string]interface{} {
	v := json_unpack(s)
	if v == nil {
		return nil
	}
	tmp := make(map[string]interface{})
	for _k, _v := range v.(map[string]interface{}) {
		tmp[_k] = Initresult(_v)
	}
	return tmp
}

//返回interface
func Json_unpack(s interface{}) interface{} {
	return Initresult(json_unpack(s))
}
func json_unpack(s interface{}) interface{} {
	if s == nil {
		return nil
	}
	var (
		v interface{} // value to decode/encode into
	)
	buf := serializationbufpool.Get().(*MsgBuffer)
	buf.Reset()
	var err error
	switch s.(type) {
	case string:
		buf.WriteString(s.(string))
	case []byte:
		buf.Write(s.([]byte))
	}
	err = gjson.NewDecoder(buf).Decode(&v)
	serializationbufpool.Put(buf)
	if err != nil {
		DEBUG("json反序列失败", err)
		return nil
	}
	return v
}
func JsonUnmarshal(b []byte, v interface{}) (err error) {
	buf := serializationbufpool.Get().(*MsgBuffer)
	buf.Reset()
	buf.Write(b)
	err = gjson.NewDecoder(buf).Decode(v)
	serializationbufpool.Put(buf)
	return
}

func Json_pack(s interface{}) string {

	b, err := gjson.Marshal(s)
	if err != nil {
		DEBUG("json序列化失败", err)
		return ""
	}

	return string(b)
}
func Json_pack_b(s interface{}) []byte {
	b, err := gjson.Marshal(s)
	if err != nil {
		DEBUG("json序列化失败", err)
		return nil
	}
	return b
}

func Initresult(v interface{}) interface{} {
	if v == nil {
		return nil
	}
	var result = make(map[string]interface{})
	switch v.(type) {
	case map[interface{}]interface{}:
		return initmap(v.(map[interface{}]interface{}))
	case []interface{}:
		return initslice(v.([]interface{}))
	default:
		return v
	}
	return result
}
func initmap(v map[interface{}]interface{}) interface{} {

	result := make(map[string]interface{})
	for k, value := range v {
		key := initkey(k)
		switch value.(type) {
		case []byte:
			result[key] = B2S(value.([]byte))
		case map[interface{}]interface{}:
			result[key] = Initresult(value)
		case []interface{}:
			result[key] = Initresult(value)
		case string:
			result[key] = value.(string)
		case uint64:
			result[key] = value.(uint64)
		case int64:
			result[key] = value.(int64)
		case nil:
			result[key] = nil
		default:
			t := reflect.TypeOf(value)
			DEBUG("序列化initmap未设置类型", t.Name())
		}
	}
	return result
}
func initslice(v []interface{}) interface{} {
	result := make([]interface{}, len(v))
	for i := 0; i < len(v); i++ {
		switch v[i].(type) {
		case []byte:
			result[i] = B2S(v[i].([]byte))
		case map[interface{}]interface{}:
			result[i] = Initresult(v[i])
		case []interface{}:
			result[i] = Initresult(v[i])
		case string:
			result[i] = v[i].(string)
		case uint64:
			result[i] = v[i].(uint64)
		default:
			t := reflect.TypeOf(v[i])
			DEBUG("序列化initslice未设置类型", t.Name())
		}
	}
	return result
}
func I2s(v interface{}) (result string, ok bool) {
	switch v.(type) {
	case string:
		result = v.(string)
	case uint64:
		result = strconv.FormatUint(v.(uint64), 10)
	case uint32:
		result = strconv.FormatUint(uint64(v.(uint32)), 10)
	case uint16:
		result = strconv.FormatUint(uint64(v.(uint16)), 10)
	case uint8:
		result = strconv.FormatUint(uint64(v.(uint8)), 10)
	case uint:
		result = strconv.FormatUint(uint64(v.(uint)), 10)
	case int64:
		result = strconv.FormatInt(v.(int64), 10)
	case int32:
		result = strconv.FormatInt(int64(v.(int32)), 10)
	case int16:
		result = strconv.FormatInt(int64(v.(int16)), 10)
	case int8:
		result = strconv.FormatInt(int64(v.(int8)), 10)
	case int:
		result = strconv.FormatInt(int64(v.(int)), 10)
	case uintptr:
		result = strconv.FormatInt(int64(v.(uintptr)), 10)
	case float32:
		result = Number_format(v, 10)
	case float64:
		//精度10位小数
		result = Number_format(v, 10)
	case []byte:
		result = string(v.([]byte))
	case bool:
		if v.(bool) {
			result = "1"
		} else {
			result = "0"
		}
	case nil:
		result = ""
	default:
		panic("当你遇到这个错误，很有可能是反序列化解析出来的格式与期望输出的格式不对，比如使用map[string]string去封装解析实际为map[string]map[string]string的值")
		ok = false
		return
	}
	ok = true
	return
}
func Initstring(v interface{}) (result string) {
	result, ok := I2s(v)
	if !ok {
		t := reflect.ValueOf(v)
		DEBUG("序列化未设置string类型", t.Kind())
	}
	return
}
func initkey(k interface{}) (key string) {
	switch k.(type) {
	case string:
		key = k.(string)
	case uint64:
		key = strconv.FormatUint(k.(uint64), 10)
	case int:
		key = strconv.Itoa(k.(int))
	case int64:
		key = strconv.FormatInt(k.(int64), 10)
	default:
		t := reflect.TypeOf(k)
		DEBUG("序列化未设置key类型", t.Name())
	}
	return
}

type Json_encode struct {
	E *jsoniter.Encoder
	B *bytes.Buffer
}
type Json_decode struct {
	D *jsoniter.Decoder
	B *bytes.Buffer
}
