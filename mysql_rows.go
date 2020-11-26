package libraries

import (
	"reflect"
	"sync"
)

var rows_pool = sync.Pool{New: func() interface{} {
	row := &MysqlRows{Buffer: new(MsgBuffer), Buffer2: new(MsgBuffer), field_m: make(map[string]map[string]*Field_struct)}

	return row
}}

type Field_struct struct {
	Offset  uintptr
	Kind    reflect.Kind
	Field_t reflect.Type
}
type MysqlRows struct {
	Buffer, Buffer2 *MsgBuffer
	field_len       int
	msg_len         []int
	buffer          []byte
	//msg_buffer_no *int
	field      []byte
	field_m    map[string]map[string]*Field_struct
	fields     [][]byte
	result_len int
}

func (row *MysqlRows) Columns(mysql *Mysql_Conn) (columns [][]byte, err error) {
	if cap(row.fields) < row.field_len {
		row.fields = make([][]byte, 0, row.field_len)
	}
	row.result_len = 0
	columns = row.fields[:row.field_len]
	var index uint32
	var msglen, pos, field_index int

	for msglen, err = mysql.readOneMsg(); err == nil; msglen, err = mysql.readOneMsg() {
		data := mysql.readBuffer.Next(msglen)

		if msglen == 5 && data[0] == 0xfe { //EOF
			break
		}
		pos = 0
		msglen, err = ReadLength_Coded_Slice(data, &pos)
		if err != nil {
			return
		}
		pos += msglen

		// Database [len coded string]
		msglen, err = ReadLength_Coded_Slice(data[pos:], &pos)
		if err != nil {
			return
		}

		pos += msglen
		// Table [len coded string]
		msglen, err = ReadLength_Coded_Slice(data[pos:], &pos)
		if err != nil {
			return
		}
		pos += msglen
		// Original table [len coded string]
		msglen, err = ReadLength_Coded_Slice(data[pos:], &pos)
		if err != nil {
			return
		}
		pos += msglen
		// Name [len coded string]
		msglen, err = ReadLength_Coded_Slice(data[pos:], &pos)
		if err != nil {
			return
		}
		if field_index+msglen > len(row.field) {
			row.field = append(row.field, make([]byte, msglen)...)
		}
		columns[index] = row.field[field_index : field_index+msglen]
		copy(columns[index], data[pos:pos+msglen])
		field_index += msglen

		index++
	}
	//libraries.DEBUG(row.Buffer.Bytes())
	row.Buffer.Reset()
	row.msg_len = row.msg_len[:0]
	for msglen, err := mysql.readOneMsg(); err == nil; msglen, err = mysql.readOneMsg() {
		data := mysql.readBuffer.Next(msglen)
		if msglen == 5 && data[0] == 0xfe { //EOF
			return columns, nil
		}
		row.Buffer.Write(data)
		row.result_len++
		row.msg_len = append(row.msg_len, msglen)
	}
	return columns, err
}

func (row *MysqlRows) Scan(a ...*[]byte) error {
	var err error
	for _, v := range a {
		*v, err = ReadLength_Coded_Byte(row.Buffer)
		if err != nil {
			return err
		}
	}
	return nil

}
